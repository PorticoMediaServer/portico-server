/** Shared measured facts, not a renderer or a source of layout defaults. */
export const contractVersion = 1;
/** Curated scalar input styles only; these never substitute for measured style. */
export const diagnosticStyleProperties = Object.freeze([
  'fontFamily', 'fontSize', 'fontWeight', 'fontStyle', 'lineHeight', 'letterSpacing', 'textAlign', 'textTransform',
  'color', 'backgroundColor', 'opacity', 'borderRadius', 'borderTopLeftRadius', 'borderTopRightRadius', 'borderBottomLeftRadius', 'borderBottomRightRadius',
  'borderWidth', 'borderTopWidth', 'borderRightWidth', 'borderBottomWidth', 'borderLeftWidth', 'borderColor', 'borderTopColor', 'borderRightColor', 'borderBottomColor', 'borderLeftColor',
  'shadowColor', 'shadowOpacity', 'shadowRadius', 'elevation', 'overflow', 'display', 'position', 'zIndex',
  'width', 'height', 'minWidth', 'minHeight', 'maxWidth', 'maxHeight', 'top', 'right', 'bottom', 'left',
  'padding', 'paddingHorizontal', 'paddingVertical', 'paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft',
  'margin', 'marginHorizontal', 'marginVertical', 'marginTop', 'marginRight', 'marginBottom', 'marginLeft',
  'gap', 'rowGap', 'columnGap', 'alignItems', 'alignSelf', 'justifyContent', 'flexDirection', 'flexGrow', 'flexShrink', 'flexBasis',
]);
const finite = value => typeof value === 'number' && Number.isFinite(value);
const rect = value => value && ['x', 'y', 'width', 'height'].every(key => finite(value[key])) && value.width >= 0 && value.height >= 0;
export function usesLogicalCoordinates(screen) {
  const value = screen?.measurement;
  return value?.renderer === 'dom' && value.coordinateUnit === 'css-px' || value?.renderer === 'react-native' &&
    (value.platform === 'android' && value.coordinateUnit === 'dp' || ['ios', 'tvos'].includes(value.platform) && value.coordinateUnit === 'pt');
}

export function validateMeasurement(screen) {
  const errors = [];
  const need = (condition, path) => {if (!condition) errors.push(path);};
  need(screen?.version === contractVersion, 'version');
  need(screen?.scope === 'screen', 'scope');
  need(rect(screen?.root), 'root');
  need(screen?.fullViewport === true, 'fullViewport');
  need(finite(screen?.viewport?.width) && screen.viewport.width > 0 && finite(screen?.viewport?.height) && screen.viewport.height > 0 && finite(screen?.viewport?.deviceScaleFactor) && screen.viewport.deviceScaleFactor > 0, 'viewport');
  need(screen?.root?.x === 0 && screen?.root?.y === 0 && screen?.root?.width === screen?.viewport?.width && screen?.root?.height === screen?.viewport?.height, 'root.viewport');
  if (screen?.measurement) {
    need(['dom', 'react-native'].includes(screen.measurement.renderer), 'measurement.renderer');
    need(['css-px', 'dp', 'pt'].includes(screen.measurement.coordinateUnit), 'measurement.coordinateUnit');
    need(screen.measurement.renderer !== 'react-native' || ['android', 'ios', 'tvos'].includes(screen.measurement.platform), 'measurement.platform');
    need(usesLogicalCoordinates(screen), 'measurement.coordinateMapping');
  }
  need(Array.isArray(screen?.elements), 'elements');
  need(Array.isArray(screen?.strings), 'strings');
  need(Array.isArray(screen?.anchorIssues), 'anchorIssues');
  need(Array.isArray(screen?.repeatedLists), 'repeatedLists');
  if (screen?.unavailable !== undefined) need(Array.isArray(screen.unavailable) && screen.unavailable.every(fact => typeof fact?.property === 'string' && typeof fact.reason === 'string' && fact.reason.length > 0 && (fact.element === undefined || typeof fact.element === 'string')), 'unavailable');
  const ids = new Set();
  for (const [index, node] of (Array.isArray(screen?.elements) ? screen.elements : []).entries()) {
    const path = `elements.${index}`;
    const declared = property => Array.isArray(screen.unavailable) && screen.unavailable.some(fact => fact.element === node.id && fact.property === property && typeof fact.reason === 'string' && fact.reason.length > 0);
    need(typeof node.id === 'string' && !ids.has(node.id), path + '.id'); ids.add(node.id);
    if (node.diagnostics !== undefined) {
      need(node.diagnostics && typeof node.diagnostics === 'object' && !Array.isArray(node.diagnostics) && Object.keys(node.diagnostics).every(key => key === 'flattenedStyle'), path + '.diagnostics');
      const style = node.diagnostics?.flattenedStyle;
      if (style !== undefined) need(style && typeof style === 'object' && !Array.isArray(style) && Object.entries(style).every(([key, value]) => diagnosticStyleProperties.includes(key) && (finite(value) || typeof value === 'string' && !/url\s*\(|(?:https?|file|data):/i.test(value))), path + '.diagnostics.flattenedStyle');
    }
    // Pseudo-elements have no separate box or semantic focus target.
    if (node.tag?.startsWith('::')) continue;
    need(rect(node.box), path + '.box');
    need(typeof node.ownText === 'string' && typeof node.normalizedText === 'string', path + '.text');
    need(typeof node.focused === 'boolean' || node.focused == null && declared('focused'), path + '.focused');
    need(node.style && typeof node.style === 'object', path + '.style');
    need(finite(node.effectiveOpacity) && node.effectiveOpacity >= 0 && node.effectiveOpacity <= 1 || node.effectiveOpacity == null && declared('effectiveOpacity'), path + '.effectiveOpacity');
    if (screen?.measurement?.renderer === 'react-native') {
      need(typeof node.effectiveBackgroundColor === 'string' || declared('effectiveBackgroundColor'), path + '.effectiveBackgroundColor');
      if (node.ownText && !node.input) {
        need(Array.isArray(node.textRects) && node.textRects.length > 0 && node.textRects.every(rect) || declared('textRects'), path + '.textRects');
        for (const property of ['fontFamily', 'fontSize', 'fontWeight', 'color']) need(typeof node.style?.[property] === 'string' || declared(property), path + '.style.' + property);
      }
    }
  }
  return errors;
}
