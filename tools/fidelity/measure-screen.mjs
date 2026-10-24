/** Run inside the measured document. Returns measurements, never DOM/source/art bytes.
 * The caller supplies a full-screen root and records its controlled state separately. */
export async function measureScreen({selector, expectedViewport, anchors = {}} = {}) {
  const root = document.querySelector(selector);
  if (!root) throw new Error('Fidelity screen root is missing');
  await document.fonts.ready;
  await Promise.allSettled([...root.querySelectorAll('img')].map(image => image.decode()));
  await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
  const bounds = root.getBoundingClientRect();
  if (!bounds.width || !bounds.height) throw new Error('Fidelity screen root has no layout');
  const round = number => Object.is(number, -0) ? 0 : number;
  const box = rect => ({x: round(rect.x - bounds.x), y: round(rect.y - bounds.y), width: round(rect.width), height: round(rect.height)});
  const properties = [
    'display', 'position', 'boxSizing', 'overflowX', 'overflowY', 'opacity', 'zIndex',
    'color', 'backgroundColor', 'backgroundImage', 'backgroundSize', 'backgroundPosition',
    'fontFamily', 'fontSize', 'fontWeight', 'fontStyle', 'fontStretch', 'fontFeatureSettings',
    'fontVariationSettings', 'lineHeight', 'letterSpacing', 'textAlign', 'textTransform',
    'whiteSpace', 'textDecorationLine', 'textDecorationColor', 'textShadow',
    'borderTopWidth', 'borderRightWidth', 'borderBottomWidth', 'borderLeftWidth',
    'borderTopColor', 'borderRightColor', 'borderBottomColor', 'borderLeftColor',
    'borderTopStyle', 'borderRightStyle', 'borderBottomStyle', 'borderLeftStyle',
    'borderTopLeftRadius', 'borderTopRightRadius', 'borderBottomRightRadius', 'borderBottomLeftRadius',
    'paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft',
    'marginTop', 'marginRight', 'marginBottom', 'marginLeft', 'gap', 'rowGap', 'columnGap',
    'alignItems', 'justifyContent', 'flexDirection', 'flexGrow', 'flexShrink', 'flexBasis',
    'boxShadow', 'filter', 'backdropFilter', 'transform', 'transformOrigin', 'fill', 'stroke', 'strokeWidth', 'objectFit', 'objectPosition', 'caretColor',
  ];
  const scrubURLs = value => value.replace(/url\((?:"[^"]*"|'[^']*'|[^)]*)\)/g, 'url([asset])');
  const style = computed => Object.fromEntries(properties.map(key => [key, scrubURLs(computed[key] ?? '')]));
  const visible = element => {
    for (let current = element; current; current = current.parentElement) {
      const value = getComputedStyle(current);
      if (value.display === 'none' || Number(value.opacity) === 0 || current === element && ['hidden', 'collapse'].includes(value.visibility)) return false;
      if (current === root) break;
    }
    return true;
  };
  const textValue = (text, computed) => computed.whiteSpace.startsWith('pre') ? text : text.replace(/[\t\n\f\r ]+/g, ' ').trim();
  const anchorMap = new Map(), anchorIssues = [];
  for (const [name, query] of Object.entries(anchors)) {
    const matches = query === '$root' ? [root] : [...root.querySelectorAll(query)];
    if (matches.length !== 1) anchorIssues.push({name, matches: matches.length});
    else anchorMap.set(matches[0], name);
  }
  const paths = new Map();
  const indexPaths = (element, path) => {paths.set(element, path.join('.')); [...element.children].forEach((child, index) => indexPaths(child, [...path, index]));};
  indexPaths(root, [0]);
  const elements = [], strings = [];
  const visit = async (element, path) => {
    if (!visible(element)) return;
    const rect = element.getBoundingClientRect(), computed = getComputedStyle(element);
    const id = path.join('.'), tag = element.localName;
    if (rect.width > 0 && rect.height > 0) {
      const aria = Object.fromEntries([...element.attributes].filter(attribute => attribute.name.startsWith('aria-')).sort((a, b) => a.name.localeCompare(b.name)).map(attribute =>
        [attribute.name, ['aria-controls', 'aria-labelledby', 'aria-describedby', 'aria-owns'].includes(attribute.name) ? attribute.value.split(/\s+/).map(target => paths.get(document.getElementById(target)) ?? '[outside-screen]') : attribute.value]));
      const ownText = [...element.childNodes].filter(child => child.nodeType === Node.TEXT_NODE).map(child => textValue(child.textContent ?? '', computed)).filter(Boolean).join(' ');
      const record = {id, parent: paths.get(element.parentElement) ?? null, tag, classes: [...element.classList].sort(),
        frame: anchorMap.get(element) ?? element.getAttribute('data-frame'), ownText, aria, box: box(rect), style: style(computed),
        role: element.getAttribute('role'), label: element.getAttribute('aria-label') ?? element.getAttribute('alt'),
        testId: element.getAttribute('data-testid'), focused: element === document.activeElement,
        disabled: 'disabled' in element ? Boolean(element.disabled) : element.getAttribute('aria-disabled') === 'true'};
      if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement || element instanceof HTMLSelectElement) {
        const secret = element instanceof HTMLInputElement && (element.type === 'password' || /password|secret|token|otp|pin/i.test(element.name) || /password|one-time-code/.test(element.autocomplete));
        record.input = {type: element.type, name: element.name, value: secret ? null : element.value,
          redacted: secret, length: element.value.length, placeholder: element.getAttribute('placeholder'),
          labels: [...(element.labels ?? [])].map(label => label.innerText)};
        if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement) {
          record.input.selectionStart = element.selectionStart; record.input.selectionEnd = element.selectionEnd;
        }
        if (!secret && element.value) strings.push({id: `${id}:value`, owner: id, frame: record.frame, kind: 'value', text: element.value, box: box(rect), style: style(computed)});
        if (record.input.placeholder) strings.push({id: `${id}:placeholder`, owner: id, frame: record.frame, kind: 'placeholder', text: record.input.placeholder});
      }
      if (element instanceof HTMLImageElement) record.image = {kind: 'image', loaded: element.complete && element.naturalWidth > 0, fit: computed.objectFit};
      else if (tag === 'svg') record.image = {kind: 'icon', fit: computed.objectFit};
      elements.push(record);
      for (const pseudo of ['::before', '::after']) {
        const value = getComputedStyle(element, pseudo);
        if (value.content !== 'none' && value.content !== 'normal' && value.display !== 'none')
          elements.push({id: id + pseudo, parent: id, tag: pseudo, frame: record.frame ? record.frame + pseudo : null, style: style(value), content: scrubURLs(value.content)});
      }
      let textIndex = 0;
      for (const child of element.childNodes) {
        if (child.nodeType !== Node.TEXT_NODE) continue;
        const text = textValue(child.textContent ?? '', computed);
        if (!text) continue;
        const range = document.createRange(); range.selectNodeContents(child);
        const rects = [...range.getClientRects()].filter(value => value.width > 0 && value.height > 0).map(box);
        if (rects.length) strings.push({id: `${id}:text:${textIndex++}`, owner: id, frame: record.frame, kind: 'text', text, rects, style: style(computed)});
      }
      if (record.label) strings.push({id: `${id}:label`, owner: id, frame: record.frame, kind: 'accessible-label', text: record.label});
    }
    if (['svg', 'canvas', 'img'].includes(tag)) return;
    for (let index = 0; index < element.children.length; index++) await visit(element.children[index], [...path, index]);
  };
  await visit(root, [0]);
  const viewport = {width: innerWidth, height: innerHeight, deviceScaleFactor: devicePixelRatio};
  return {version: 1, scope: 'screen', viewport, root: {x: round(bounds.x), y: round(bounds.y), width: round(bounds.width), height: round(bounds.height)},
    expectedViewport: expectedViewport ?? null,
    fullViewport: bounds.x === 0 && bounds.y === 0 && bounds.width === innerWidth && bounds.height === innerHeight,
    fontStatus: document.fonts.status, fontFaces: [...document.fonts].map(face => ({family: face.family, weight: face.weight, style: face.style, status: face.status})).sort((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b))),
    documentFocused: document.hasFocus(), anchorIssues, elements, strings};
}
