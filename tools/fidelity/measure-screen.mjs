/** Run inside the measured document. Returns measurements, never DOM/source/art bytes.
 * The caller supplies a full-screen root and records its controlled state separately. */
export async function measureScreen({selector, expectedViewport} = {}) {
  const root = document.querySelector(selector);
  if (!root) throw new Error('Fidelity screen root is missing');
  await document.fonts.ready;
  await Promise.allSettled([...root.querySelectorAll('img')].map(image => image.decode()));
  await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
  const bounds = root.getBoundingClientRect();
  if (!bounds.width || !bounds.height) throw new Error('Fidelity screen root has no layout');
  const round = number => Math.round(number * 1000) / 1000;
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
    'boxShadow', 'filter', 'backdropFilter', 'transform', 'transformOrigin', 'fill', 'stroke', 'strokeWidth',
  ];
  const scrubURLs = value => value.replace(/url\((?:"[^"]*"|'[^']*'|[^)]*)\)/g, 'url([asset])');
  const style = computed => Object.fromEntries(properties.map(key => [key, scrubURLs(computed[key] ?? '')]));
  const visible = element => {
    for (let current = element; current; current = current.parentElement) {
      const value = getComputedStyle(current);
      if (value.display === 'none' || value.visibility === 'hidden' || value.visibility === 'collapse' || Number(value.opacity) === 0) return false;
      if (current === root) break;
    }
    return true;
  };
  const digest = async value => {
    if (!globalThis.crypto?.subtle) return null;
    const bytes = await crypto.subtle.digest('SHA-256', typeof value === 'string' ? new TextEncoder().encode(value) : value);
    return [...new Uint8Array(bytes)].map(value => value.toString(16).padStart(2, '0')).join('');
  };
  const textValue = (text, computed) => computed.whiteSpace.startsWith('pre') ? text : text.replace(/[\t\n\f\r ]+/g, ' ').trim();
  const elements = [], strings = [];
  const visit = async (element, path) => {
    if (!visible(element)) return;
    const rect = element.getBoundingClientRect(), computed = getComputedStyle(element);
    const id = path.join('.'), tag = element.localName;
    if (rect.width > 0 && rect.height > 0) {
      const record = {id, tag, box: box(rect), style: style(computed),
        role: element.getAttribute('role'), label: element.getAttribute('aria-label'),
        testId: element.getAttribute('data-testid'), focused: element === document.activeElement,
        disabled: 'disabled' in element ? Boolean(element.disabled) : element.getAttribute('aria-disabled') === 'true'};
      if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement || element instanceof HTMLSelectElement) {
        const secret = element instanceof HTMLInputElement && (element.type === 'password' || /password|secret|token|otp|pin/i.test(element.name) || /password|one-time-code/.test(element.autocomplete));
        record.input = {type: element.type, name: element.name, value: secret ? null : element.value,
          redacted: secret, length: element.value.length, placeholder: element.getAttribute('placeholder'),
          labels: [...(element.labels ?? [])].map(label => label.innerText)};
        if (!secret && element.value) strings.push({id: `${id}:value`, kind: 'value', text: element.value, box: box(rect)});
      }
      if (element instanceof HTMLImageElement) {
        let pixels = null, unavailable;
        try {
          const canvas = document.createElement('canvas'); canvas.width = Math.max(1, Math.round(rect.width)); canvas.height = Math.max(1, Math.round(rect.height));
          if (canvas.width * canvas.height > 4_000_000) throw new Error('Image measurement limit');
          const context = canvas.getContext('2d', {willReadFrequently: true});
          context.drawImage(element, 0, 0, canvas.width, canvas.height);
          const rgba = context.getImageData(0, 0, canvas.width, canvas.height).data;
          pixels = await digest(rgba);
        } catch {unavailable = 'image-pixels-unavailable';}
        record.image = {complete: element.complete, naturalWidth: element.naturalWidth, naturalHeight: element.naturalHeight, pixels, ...(unavailable ? {unavailable} : {})};
      }
      if (element instanceof SVGUseElement) {
        const href = element.href.baseVal;
        const symbol = href.startsWith('#') ? document.getElementById(href.slice(1)) : null;
        // Shape facts are digested in the document; no private SVG path/art is exported.
        const geometry = symbol ? [...symbol.querySelectorAll('path,circle,rect,line,polyline,polygon,ellipse')].map(child =>
          [child.localName, ...['d', 'cx', 'cy', 'r', 'x', 'y', 'width', 'height', 'x1', 'x2', 'y1', 'y2', 'points', 'rx', 'ry'].map(name => child.getAttribute(name))]) : null;
        record.glyph = {shape: geometry ? await digest(JSON.stringify(geometry)) : null};
      }
      elements.push(record);
      for (const pseudo of ['::before', '::after']) {
        const value = getComputedStyle(element, pseudo);
        if (value.content !== 'none' && value.content !== 'normal' && value.display !== 'none')
          elements.push({id: id + pseudo, tag: pseudo, owner: id, style: style(value), content: value.content});
      }
      let textIndex = 0;
      for (const child of element.childNodes) {
        if (child.nodeType !== Node.TEXT_NODE) continue;
        const text = textValue(child.textContent ?? '', computed);
        if (!text) continue;
        const range = document.createRange(); range.selectNodeContents(child);
        const rects = [...range.getClientRects()].filter(value => value.width > 0 && value.height > 0).map(box);
        if (rects.length) strings.push({id: `${id}:text:${textIndex++}`, kind: 'text', text, rects, style: style(computed)});
      }
      if (record.label) strings.push({id: `${id}:label`, kind: 'accessible-label', text: record.label});
    }
    for (let index = 0; index < element.children.length; index++) await visit(element.children[index], [...path, index]);
  };
  await visit(root, [0]);
  const viewport = {width: innerWidth, height: innerHeight, deviceScaleFactor: devicePixelRatio};
  return {version: 1, scope: 'screen', viewport, root: {x: round(bounds.x), y: round(bounds.y), width: round(bounds.width), height: round(bounds.height)},
    expectedViewport: expectedViewport ?? null,
    fullViewport: bounds.x === 0 && bounds.y === 0 && bounds.width === innerWidth && bounds.height === innerHeight,
    fontStatus: document.fonts.status, elements, strings};
}
