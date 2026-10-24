import {colorDifference} from './color.mjs';

export const policy = Object.freeze({boxPixels: 2, opacity: .02, deltaE2000: 2, colorAlpha: .02});
const textStyles = ['fontFamily', 'fontSize', 'fontWeight', 'fontStyle', 'lineHeight', 'letterSpacing', 'textTransform', 'textAlign', 'textDecorationLine', 'textShadow'];
const paintStyles = ['backgroundImage', 'backgroundSize', 'backgroundPosition', 'borderTopWidth', 'borderRightWidth', 'borderBottomWidth', 'borderLeftWidth',
  'borderTopStyle', 'borderRightStyle', 'borderBottomStyle', 'borderLeftStyle', 'borderTopLeftRadius', 'borderTopRightRadius', 'borderBottomRightRadius', 'borderBottomLeftRadius',
  'boxShadow', 'filter', 'backdropFilter', 'transform', 'transformOrigin', 'objectFit'];
const colors = ['color', 'backgroundColor', 'borderTopColor', 'borderRightColor', 'borderBottomColor', 'borderLeftColor', 'textDecorationColor', 'fill', 'stroke'];
const unscored = ['classes', 'tag', 'role', 'aria', 'display', 'position', 'zIndex', 'objectPosition', 'fill', 'stroke', 'boxSizing', 'overflowX', 'overflowY', 'paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft', 'marginTop', 'marginRight', 'marginBottom', 'marginLeft', 'gap', 'rowGap', 'columnGap', 'alignItems', 'justifyContent', 'flexDirection', 'flexGrow', 'flexShrink', 'flexBasis'];
const identityTransform = value => value === undefined || value === 'none' || value.replaceAll(' ', '') === 'matrix(1,0,0,1,0,0)';
const opaque = value => value && value !== 'transparent' && value !== 'rgba(0, 0, 0, 0)';
function significant(node) {
  const s = node.style ?? {};
  return Boolean(node.frame || node.ownText || node.input || node.image ||
    opaque(s.backgroundColor) || s.backgroundImage && s.backgroundImage !== 'none' || s.boxShadow && s.boxShadow !== 'none' ||
    ['Top', 'Right', 'Bottom', 'Left'].some(side => parseFloat(s[`border${side}Width`]) > 0 && s[`border${side}Style`] !== 'none'));
}
const text = node => node.input && !node.input.redacted ? node.input.value : node.ownText ?? '';
const role = node => node.role || (/^h[1-6]$/.test(node.tag) ? 'heading' : ['button', 'input', 'a', 'img', 'svg'].includes(node.tag) ? node.tag : 'text');
const colorTokens = /rgba?\([^)]+\)|color\(srgb [^)]+\)/g;
function samePaint(a, b) {
  if (a === b) return true;
  const left = [...String(a).matchAll(colorTokens)].map(match => match[0]), right = [...String(b).matchAll(colorTokens)].map(match => match[0]);
  return left.length > 0 && left.length === right.length && String(a).replace(colorTokens, '[color]') === String(b).replace(colorTokens, '[color]') &&
    left.every((value, index) => {const difference = colorDifference(value, right[index]); return difference && difference.deltaE2000 <= policy.deltaE2000 && difference.alpha <= policy.colorAlpha;});
}

/** Deterministic anchor -> role/text/order matching. Never widens policy to pass. */
export function compareScreens(reference, actual, {referenceOnly = [], actualContentOnly = [], textHandledBySlots = [], contentSlots = []} = {}) {
  const differences = [], diagnostics = [], ambiguous = [], matches = [], used = new Set();
  const exclude = new Set(referenceOnly);
  const expected = reference.elements.filter(node => significant(node) && !exclude.has(node.frame));
  const observed = actual.elements.filter(node => significant(node) && !actualContentOnly.includes(node.frame));
  if (reference.scope !== 'screen' || actual.scope !== 'screen' || !actual.fullViewport || reference.root.width !== actual.root.width || reference.root.height !== actual.root.height || reference.viewport.deviceScaleFactor !== 1 || actual.viewport.deviceScaleFactor !== 1)
    differences.push({kind: 'screen-state', label: 'Complete same-size DPR1 screen', expected: reference.root, actual: actual.root});
  for (const issue of [...(reference.anchorIssues ?? []), ...(actual.anchorIssues ?? [])]) differences.push({kind: 'anchor-cardinality', label: issue.name, matches: issue.matches});
  for (const node of expected) {
    let candidates = observed.filter(other => !used.has(other.id)), method;
    if (node.frame) {candidates = candidates.filter(other => other.frame === node.frame); method = 'frame-anchor';}
    else {
      const exact = candidates.filter(other => role(other) === role(node) && text(other) === text(node));
      const copy = candidates.filter(other => text(node) && text(other) === text(node));
      candidates = exact.length ? exact : copy.length ? copy : candidates.filter(other => role(other) === role(node));
      method = exact.length ? 'role-text-order' : copy.length ? 'text-order' : 'role-order';
    }
    const label = node.frame || text(node) || `${node.image?.kind ?? node.tag} ${node.id}`;
    if (!candidates.length) {differences.push({kind: 'missing-element', label, reference: node.id}); continue;}
    if (candidates.length > 1) ambiguous.push({label, reference: node.id, candidates: candidates.map(value => value.id), selectedBy: 'DOM order'});
    const other = candidates[0]; used.add(other.id); matches.push({label, reference: node.id, actual: other.id, method});
    const add = (property, left, right, detail) => differences.push({kind: 'measured-difference', label, property, expected: left, actual: right, ...(detail ? {detail} : {})});
    const paintOnly = textHandledBySlots.includes(node.frame);
    if (!paintOnly && text(node) !== text(other)) add('text', text(node), text(other));
    if (node.box && other.box) for (const property of ['x', 'y', 'width', 'height']) if (Math.abs(node.box[property] - other.box[property]) > policy.boxPixels) add(`box.${property}`, node.box[property], other.box[property]);
    if (!paintOnly && (text(node) || text(other))) for (const property of textStyles) if (node.style?.[property] !== other.style?.[property]) add(property, node.style?.[property], other.style?.[property]);
    for (const property of paintStyles) {
      if (['transform', 'transformOrigin'].includes(property) && identityTransform(node.style?.transform) && identityTransform(other.style?.transform)) continue;
      if (!samePaint(node.style?.[property], other.style?.[property])) add(property, node.style?.[property], other.style?.[property]);
    }
    if (Math.abs(Number(node.style?.opacity ?? 1) - Number(other.style?.opacity ?? 1)) > policy.opacity) add('opacity', node.style?.opacity, other.style?.opacity);
    for (const property of colors) {
      if (property === 'color' && (paintOnly || !(text(node) || text(other) || node.image?.kind === 'icon'))) continue;
      if (property.startsWith('border') && property.endsWith('Color') && !(parseFloat(node.style?.[property.replace('Color', 'Width')]) > 0 || parseFloat(other.style?.[property.replace('Color', 'Width')]) > 0)) continue;
      if (['fill', 'stroke'].includes(property) || property === 'textDecorationColor' && node.style?.textDecorationLine === 'none' && other.style?.textDecorationLine === 'none') continue;
      const a = node.style?.[property], b = other.style?.[property];
      if (a === b || a === undefined || b === undefined) continue;
      const difference = colorDifference(a, b);
      if (!difference || difference.deltaE2000 > policy.deltaE2000 || difference.alpha > policy.colorAlpha) add(property, a, b, difference);
    }
    if (node.image?.kind !== other.image?.kind) add('image-slot.kind', node.image?.kind, other.image?.kind);
    if (node.image?.kind === 'image' && node.image.loaded !== other.image?.loaded) add('image-slot.loaded', node.image.loaded, other.image?.loaded);
    for (const property of unscored) {
      const a = node[property] ?? node.style?.[property], b = other[property] ?? other.style?.[property];
      if (JSON.stringify(a) !== JSON.stringify(b)) diagnostics.push({label, property, expected: a, actual: b, scored: false});
    }
  }
  for (const node of observed.filter(node => !used.has(node.id))) differences.push({kind: 'extra-element', label: node.frame || text(node) || `${node.image?.kind ?? node.tag} ${node.id}`, actual: node.id});
  for (const slot of contentSlots) {
    const a = reference.elements.find(node => node.frame === slot.reference), b = actual.elements.find(node => node.frame === slot.actual);
    if (!a || !b) {differences.push({kind: 'missing-content-slot', label: slot.reference}); continue;}
    for (const property of textStyles.filter(property => !slot.nativeInput || !['lineHeight', 'textShadow', 'textAlign'].includes(property))) {
      if (a.style[property] !== b.style[property]) differences.push({kind: 'measured-difference', label: slot.reference, property, expected: a.style[property], actual: b.style[property]});
    }
    if (slot.nativeInput) diagnostics.push({label: slot.reference, property: 'native-input-glyph-rectangles/lineHeight', scored: false, reason: 'Native input exposes host/value/font facts, not glyph rectangles; never substitute host width for text width.'});
  }
  for (const item of ambiguous) differences.push({kind: 'ambiguous-match', ...item});
  return {policy, matchedElements: matches.length, expectedElements: expected.length, actualElements: observed.length, matches, differences, diagnostics,
    unscoredProperties: unscored, referenceOnly, actualContentOnly, verdict: differences.length ? 'FAIL' : 'PASS'};
}
