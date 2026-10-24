import {colorDifference} from './color.mjs';
import {usesLogicalCoordinates, sameLogicalExtent, componentFitsViewport} from './contract.mjs';

export const policy = Object.freeze({boxPixels: 2, opacity: .02, deltaE2000: 2, colorAlpha: .02});
const textStyles = ['fontFamily', 'fontSize', 'fontWeight', 'fontStyle', 'letterSpacing', 'textTransform', 'textDecorationLine', 'textShadow'];
const paintStyles = ['backgroundImage', 'backgroundSize', 'backgroundPosition', 'borderTopWidth', 'borderRightWidth', 'borderBottomWidth', 'borderLeftWidth',
  'borderTopStyle', 'borderRightStyle', 'borderBottomStyle', 'borderLeftStyle', 'borderTopLeftRadius', 'borderTopRightRadius', 'borderBottomRightRadius', 'borderBottomLeftRadius',
  'boxShadow', 'filter', 'backdropFilter', 'transform', 'transformOrigin', 'objectFit'];
const colors = ['color', 'borderTopColor', 'borderRightColor', 'borderBottomColor', 'borderLeftColor', 'textDecorationColor', 'fill', 'stroke'];
const unscored = ['diagnostics', 'classes', 'tag', 'role', 'aria', 'display', 'position', 'zIndex', 'objectPosition', 'lineHeight', 'textAlign', 'backgroundColor', 'opacity', 'fill', 'stroke', 'boxSizing', 'overflowX', 'overflowY', 'paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft', 'marginTop', 'marginRight', 'marginBottom', 'marginLeft', 'gap', 'rowGap', 'columnGap', 'alignItems', 'justifyContent', 'flexDirection', 'flexGrow', 'flexShrink', 'flexBasis'];
const identityTransform = value => value === undefined || value === 'none' || value.replaceAll(' ', '') === 'matrix(1,0,0,1,0,0)';
const opaque = value => value && value !== 'transparent' && value !== 'rgba(0, 0, 0, 0)';
function significant(node) {
  const s = node.style ?? {};
  return Boolean(node.frame || node.leaf !== false && node.ownText || ['button', 'a', 'label'].includes(node.tag) && node.normalizedText || node.input || node.image ||
    opaque(s.backgroundColor) || s.backgroundImage && s.backgroundImage !== 'none' || s.boxShadow && s.boxShadow !== 'none' ||
    ['Top', 'Right', 'Bottom', 'Left'].some(side => parseFloat(s[`border${side}Width`]) > 0 && s[`border${side}Style`] !== 'none'));
}
const text = node => node.input && !node.input.redacted ? node.input.value : node.frame || node.repeat || ['button', 'a', 'label'].includes(node.tag) ? node.normalizedText ?? node.ownText ?? '' : node.leaf === false ? '' : node.ownText ?? '';
const textRectangles = node => node.frame || node.repeat || ['button', 'a', 'label'].includes(node.tag) ? node.descendantTextRects ?? node.textRects : node.textRects;
const role = node => node.repeat?.role || node.role || (/^h[1-6]$/.test(node.tag) ? 'heading' : ['button', 'input', 'a', 'img', 'svg'].includes(node.tag) ? node.tag : 'text');
const within = (id, roots) => typeof id === 'string' && roots.some(root => root && (id === root || id.startsWith(root + '.')));
const colorTokens = /rgba?\([^)]+\)|color\(srgb [^)]+\)/g;
function samePaint(a, b) {
  if (a === b) return true;
  const left = [...String(a).matchAll(colorTokens)].map(match => match[0]), right = [...String(b).matchAll(colorTokens)].map(match => match[0]);
  return left.length > 0 && left.length === right.length && String(a).replace(colorTokens, '[color]') === String(b).replace(colorTokens, '[color]') &&
    left.every((value, index) => {const difference = colorDifference(value, right[index]); return difference && difference.deltaE2000 <= policy.deltaE2000 && difference.alpha <= policy.colorAlpha;});
}

/** Deterministic anchor -> role/text/order matching. Never widens policy to pass. */
export function compareScreens(reference, actual, {referenceOnly = [], actualContentOnly = [], textHandledBySlots = [], contentSlots = [], focus} = {}, {referenceSelf = false} = {}) {
  const differences = [], diagnostics = [], ambiguous = [], matches = [], used = new Set(), itemMatches = new Map();
  if (actual.diagnostics?.fontScale !== undefined) diagnostics.push({label: 'screen', property: 'diagnostics.fontScale', actual: actual.diagnostics.fontScale, scored: false, reason: 'Actual system font scale is diagnostic metadata; measured font/run facts are never multiplied, divided or normalized by it.'});
  const unavailable = (screen, node, property) => screen.unavailable?.some(fact => fact.element === node?.id && fact.property === property);
  const scoredTextStyles = textStyles.map(property => property === 'fontFamily' && actual.measurement?.renderer === 'react-native' ? 'resolvedFontFamily' : property);
  const observedTextStyle = (screen, node, property, side) => {
    if (property !== 'resolvedFontFamily' || typeof node.style?.[property] === 'string') return true;
    if (!unavailable(screen, node, property)) differences.push({kind: 'unavailable-measurement', label: `${side} ${node.id}`, property,
      reason: 'Active rendered font identity has not been observed; a requested family chain is not a substitute'});
    return false;
  };
  const cascadeRoots = {reference: [], actual: []};
  const invalidAnchors = new Set([...(reference.anchorIssues ?? []), ...(actual.anchorIssues ?? [])].map(issue => issue.name));
  for (const name of invalidAnchors) {
    const a = reference.anchorIssues?.find(issue => issue.name === name), b = actual.anchorIssues?.find(issue => issue.name === name);
    differences.push({kind: 'anchor-cardinality', label: name, expected: a?.matches ?? 1, actual: b?.matches ?? 1});
    cascadeRoots.reference.push(...reference.elements.filter(node => node.frame === name).map(node => node.id));
    cascadeRoots.actual.push(...actual.elements.filter(node => node.frame === name).map(node => node.id));
  }
  for (const group of reference.repeatedLists ?? []) {
    const other = actual.repeatedLists?.find(item => item.name === group.name);
    if (!invalidAnchors.has(group.name) && group.count !== (other?.count ?? 0)) {
      differences.push({kind: 'list-cardinality', label: group.name, expected: group.count, actual: other?.count ?? 0});
      if (group.container) cascadeRoots.reference.push(group.container);
      if (other?.container) cascadeRoots.actual.push(other.container);
    }
  }
  const exclude = new Set(referenceOnly);
  const expected = reference.elements.filter(node => significant(node) && !exclude.has(node.frame) && !within(node.id, cascadeRoots.reference));
  const observed = actual.elements.filter(node => significant(node) && !actualContentOnly.includes(node.frame) && !within(node.id, cascadeRoots.actual));
  const actualLogical = usesLogicalCoordinates(actual);
  const referenceSelfCanvas = referenceSelf && reference.measurement?.sourceRole === 'reference-frame' && actual.measurement?.sourceRole === 'reference-frame' &&
    JSON.stringify(reference.referencePresentation) === JSON.stringify(actual.referencePresentation) && JSON.stringify(reference.viewport) === JSON.stringify(actual.viewport);
  const component = reference.scope === 'component' && actual.scope === 'component';
  const selfComponent = referenceSelf && component && reference.measurement?.sourceRole === 'reference-component' && actual.measurement?.sourceRole === 'reference-component' &&
    JSON.stringify(reference.componentPresentation) === JSON.stringify(actual.componentPresentation) && JSON.stringify(reference.viewport) === JSON.stringify(actual.viewport);
  if (component ? reference.viewport.deviceScaleFactor !== 1 || !selfComponent && (!componentFitsViewport(actual) || actual.measurement?.sourceRole?.startsWith('reference-')) :
    reference.scope !== 'screen' || actual.scope !== 'screen' || !actual.fullViewport && !referenceSelfCanvas || !sameLogicalExtent(reference.root.width, actual.root.width, actual.measurement) || !sameLogicalExtent(reference.root.height, actual.root.height, actual.measurement) || reference.viewport.deviceScaleFactor !== 1 || (!actualLogical && actual.viewport.deviceScaleFactor !== 1))
    differences.push({kind: 'screen-state', label: component ? 'Complete measured component' : 'Complete same-size measured logical screen', expected: reference.root, actual: actual.root});
  for (const [side, screen] of [['reference', reference], ['actual', actual]]) for (const fact of screen.unavailable ?? []) differences.push({kind: 'unavailable-measurement', label: `${side} ${fact.element ?? 'screen'}`, property: fact.property, reason: fact.reason});
  if (focus && !actual.elements.some(node => unavailable(actual, node, 'focused'))) {
    const focused = actual.elements.filter(node => node.focused === true), target = actual.elements.filter(node => node.frame === focus.frame);
    if (typeof focus.frame !== 'string' || target.length !== 1 || target[0]?.focused !== true || focused.length !== 1)
      differences.push({kind: 'focus-state', label: focus.frame ?? 'Drawn focus target', expected: {frame: focus.frame, count: 1}, actual: {frames: focused.map(node => node.frame ?? node.id), count: focused.length}});
  }
  const referenceNodes = new Map(reference.elements.map(node => [node.id, node])), actualNodes = new Map(actual.elements.map(node => [node.id, node]));
  const anchoredMatches = new Map();
  const descendantOf = (node, ancestor) => {
    const seen = new Set();
    for (let parent = node.parent; parent && !seen.has(parent); parent = actualNodes.get(parent)?.parent) {
      if (parent === ancestor) return true;
      seen.add(parent);
    }
    return false;
  };
  for (const node of expected) {
    let candidates = observed.filter(other => !used.has(other.id)), method;
    if (referenceSelf) {candidates = candidates.filter(other => other.id === node.id); method = 'same-observation-id';}
    else if (node.repeat) {
      candidates = candidates.filter(other => other.repeat?.list === node.repeat.list && other.repeat?.part === node.repeat.part);
      if (node.repeat.part) candidates = candidates.filter(other => other.repeat.item === itemMatches.get(node.repeat.item));
      else {
        const semantic = candidates.filter(other => role(other) === role(node) && text(other) === text(node));
        candidates = semantic.length ? semantic : candidates.filter(other => role(other) === role(node));
      }
      method = 'list-role-text-order';
    } else if (node.frame) {candidates = candidates.filter(other => other.frame === node.frame); method = 'frame-anchor';}
    else {
      // Wrapper depth may differ, but an unanchored child belongs to its nearest
      // already matched semantic anchor/list part, not an unrelated global row.
      const seen = new Set();
      for (let parent = node.parent; parent && !seen.has(parent); parent = referenceNodes.get(parent)?.parent) {
        seen.add(parent);
        if (anchoredMatches.has(parent)) {candidates = candidates.filter(other => descendantOf(other, anchoredMatches.get(parent))); break;}
      }
      const exact = candidates.filter(other => role(other) === role(node) && text(other) === text(node));
      const copy = candidates.filter(other => text(node) && text(other) === text(node));
      candidates = exact.length ? exact : copy.length ? copy : candidates.filter(other => role(other) === role(node));
      method = exact.length ? 'role-text-order' : copy.length ? 'text-order' : 'role-order';
    }
    const label = node.frame || text(node) || `${node.image?.kind ?? node.tag} ${node.id}`;
    if (!candidates.length) {differences.push({kind: 'missing-element', label, reference: node.id}); continue;}
    if (candidates.length > 1 && !node.repeat) ambiguous.push({label, reference: node.id, candidates: candidates.map(value => value.id), selectedBy: 'DOM order'});
    const other = candidates[0]; used.add(other.id); matches.push({label, reference: node.id, actual: other.id, method});
    if (node.frame || node.repeat) anchoredMatches.set(node.id, other.id);
    if (node.repeat && !node.repeat.part) itemMatches.set(node.repeat.item, other.repeat.item);
    const add = (property, left, right, detail) => differences.push({kind: 'measured-difference', label, property, expected: left, actual: right, ...(detail ? {detail} : {})});
    const paintOnly = textHandledBySlots.includes(node.frame);
    const containsCascade = cascadeRoots.reference.some(root => root.startsWith(node.id + '.')) || cascadeRoots.actual.some(root => root.startsWith(other.id + '.'));
    const unknownText = [node, other].some((value, index) => ['ownText', 'normalizedText'].some(property => unavailable(index ? actual : reference, value, property)));
    if (!paintOnly && !containsCascade && !unknownText && text(node) !== text(other)) add('text', text(node), text(other));
    if (node.box && other.box) for (const property of ['x', 'y', 'width', 'height']) if (Math.abs(node.box[property] - other.box[property]) > policy.boxPixels) add(`box.${property}`, node.box[property], other.box[property]);
    if (!paintOnly && (node.ownText || other.ownText)) for (const property of scoredTextStyles) {
      const left = observedTextStyle(reference, node, property, 'reference'), right = observedTextStyle(actual, other, property, 'actual');
      if (left && right && !unavailable(reference, node, property) && !unavailable(actual, other, property) && node.style?.[property] !== other.style?.[property]) add(property, node.style?.[property], other.style?.[property]);
    }
    if (!paintOnly && !containsCascade && (node.ownText || other.ownText || node.frame && !node.anchoredDescendants)) {
      const a = textRectangles(node), b = textRectangles(other);
      if (a && b) {
        if (a.length !== b.length) add('textRun.count', a.length, b.length);
        else a.forEach((rect, index) => {for (const property of ['x', 'y', 'width', 'height']) if (Math.abs(rect[property] - b[index][property]) > policy.boxPixels) add(`textRun.${index}.${property}`, rect[property], b[index][property]);});
      }
    }
    for (const property of paintStyles) {
      if (property === 'transformOrigin' && node.style?.transform !== undefined && other.style?.transform !== undefined && identityTransform(node.style.transform) && identityTransform(other.style.transform)) continue;
      if (unavailable(reference, node, property) || unavailable(actual, other, property)) continue;
      if (['transform', 'transformOrigin'].includes(property) && identityTransform(node.style?.transform) && identityTransform(other.style?.transform)) continue;
      if (node.style?.[property] !== other.style?.[property] && (node.style?.[property] == null || other.style?.[property] == null)) {
        differences.push({kind: 'unavailable-measurement', label, property, reason: 'Paint style was not observed on both sides'}); continue;
      }
      if (!samePaint(node.style?.[property], other.style?.[property])) add(property, node.style?.[property], other.style?.[property]);
    }
    if (!unavailable(reference, node, 'effectiveOpacity') && !unavailable(actual, other, 'effectiveOpacity') && Math.abs(Number(node.effectiveOpacity ?? node.style?.opacity ?? 1) - Number(other.effectiveOpacity ?? other.style?.opacity ?? 1)) > policy.opacity) add('effectiveOpacity', node.effectiveOpacity ?? node.style?.opacity, other.effectiveOpacity ?? other.style?.opacity);
    const backgroundA = node.effectiveBackgroundColor ?? node.style?.backgroundColor, backgroundB = other.effectiveBackgroundColor ?? other.style?.backgroundColor;
    if (!unavailable(reference, node, 'effectiveBackgroundColor') && !unavailable(actual, other, 'effectiveBackgroundColor') && backgroundA !== backgroundB) {const difference = colorDifference(backgroundA, backgroundB); if (!difference || difference.deltaE2000 > policy.deltaE2000 || difference.alpha > policy.colorAlpha) add('effectiveBackgroundColor', backgroundA, backgroundB, difference);}
    for (const property of colors) {
      if (unavailable(reference, node, property) || unavailable(actual, other, property)) continue;
      if (property === 'color' && (paintOnly || !(node.ownText || other.ownText || node.image?.kind === 'icon'))) continue;
      if (property.startsWith('border') && property.endsWith('Color') && !(parseFloat(node.style?.[property.replace('Color', 'Width')]) > 0 || parseFloat(other.style?.[property.replace('Color', 'Width')]) > 0)) continue;
      if (['fill', 'stroke'].includes(property) || property === 'textDecorationColor' && node.style?.textDecorationLine === 'none' && other.style?.textDecorationLine === 'none') continue;
      const a = node.style?.[property], b = other.style?.[property];
      if (a === b || a === undefined || b === undefined) continue;
      const difference = colorDifference(a, b);
      if (!difference || difference.deltaE2000 > policy.deltaE2000 || difference.alpha > policy.colorAlpha) add(property, a, b, difference);
    }
    if (node.image?.kind !== other.image?.kind) add('image-slot.kind', node.image?.kind, other.image?.kind);
    if (node.image?.kind === 'image' && node.image.loaded !== other.image?.loaded) add('image-slot.loaded', node.image.loaded, other.image?.loaded);
    if (node.image?.kind === 'icon' || other.image?.kind === 'icon') diagnostics.push({label, property: 'icon.identity/contents', scored: false, reason: 'Icon slot geometry/style is measured; SVG paths, sprite references and artwork contents are excluded. A matching slot does not establish glyph identity.'});
    for (const property of unscored) {
      const a = node[property] ?? node.style?.[property], b = other[property] ?? other.style?.[property];
      if (JSON.stringify(a) !== JSON.stringify(b)) diagnostics.push({label, property, expected: a, actual: b, scored: false});
    }
  }
  for (const node of observed.filter(node => !used.has(node.id))) differences.push({kind: 'extra-element', label: node.frame || text(node) || `${node.image?.kind ?? node.tag} ${node.id}`, actual: node.id});
  for (const slot of contentSlots) {
    const a = reference.elements.find(node => node.frame === slot.reference), b = actual.elements.find(node => node.frame === slot.actual);
    if (within(a?.id, cascadeRoots.reference) || within(b?.id, cascadeRoots.actual)) continue;
    if (!a || !b) {differences.push({kind: 'missing-content-slot', label: slot.reference}); continue;}
    if (!b.input?.redacted && text(a) !== text(b)) differences.push({kind: 'measured-difference', label: slot.reference, property: 'text', expected: text(a), actual: text(b)});
    for (const property of scoredTextStyles.filter(property => !slot.nativeInput || property !== 'textShadow')) {
      const left = observedTextStyle(reference, a, property, 'reference'), right = observedTextStyle(actual, b, property, 'actual');
      if (!left || !right) continue;
      if (unavailable(reference, a, property) || unavailable(actual, b, property)) continue;
      if (a.style[property] !== b.style[property]) differences.push({kind: 'measured-difference', label: slot.reference, property, expected: a.style[property], actual: b.style[property]});
    }
    const color = colorDifference(a.style.color, b.style.color);
    if (!unavailable(reference, a, 'color') && !unavailable(actual, b, 'color') && a.style.color !== b.style.color && (!color || color.deltaE2000 > policy.deltaE2000 || color.alpha > policy.colorAlpha)) differences.push({kind: 'measured-difference', label: slot.reference, property: 'color', expected: a.style.color, actual: b.style.color, detail: color});
    if (!slot.nativeInput) {
      const left = a.descendantTextRects ?? a.textRects, right = b.descendantTextRects ?? b.textRects;
      if (left && right) {
        if (left.length !== right.length) differences.push({kind: 'measured-difference', label: slot.reference, property: 'textRun.count', expected: left.length, actual: right.length});
        else left.forEach((rect, index) => {for (const property of ['x', 'y', 'width', 'height']) if (Math.abs(rect[property] - right[index][property]) > policy.boxPixels) differences.push({kind: 'measured-difference', label: slot.reference, property: `textRun.${index}.${property}`, expected: rect[property], actual: right[index][property]});});
      }
    }
    if (slot.nativeInput) diagnostics.push({label: slot.reference, property: 'native-input-glyph-rectangles/lineHeight', scored: false, reason: 'Native input exposes host/value/font facts, not glyph rectangles; never substitute host width for text width.'});
  }
  for (const item of ambiguous) differences.push({kind: 'ambiguous-match', ...item});
  return {policy, matchedElements: matches.length, expectedElements: expected.length, actualElements: observed.length, matches, differences, diagnostics,
    unscoredProperties: [...unscored, 'platformFontFamily', ...(actual.measurement?.renderer === 'react-native' ? [] : ['resolvedFontFamily'])], referenceOnly, actualContentOnly, cascadeRoots, verdict: differences.length ? 'FAIL' : 'PASS'};
}
