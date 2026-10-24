/** Shared measured facts, not a renderer or a source of layout defaults. */
export const contractVersion = 1;
const finite = value => typeof value === 'number' && Number.isFinite(value);
const rect = value => value && ['x', 'y', 'width', 'height'].every(key => finite(value[key])) && value.width >= 0 && value.height >= 0;

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
  }
  need(Array.isArray(screen?.elements), 'elements');
  need(Array.isArray(screen?.strings), 'strings');
  need(Array.isArray(screen?.anchorIssues), 'anchorIssues');
  need(Array.isArray(screen?.repeatedLists), 'repeatedLists');
  const ids = new Set();
  for (const [index, node] of (Array.isArray(screen?.elements) ? screen.elements : []).entries()) {
    const path = `elements.${index}`;
    need(typeof node.id === 'string' && !ids.has(node.id), path + '.id'); ids.add(node.id);
    // Pseudo-elements have no separate box or semantic focus target.
    if (node.tag?.startsWith('::')) continue;
    need(rect(node.box), path + '.box');
    need(typeof node.ownText === 'string' && typeof node.normalizedText === 'string', path + '.text');
    need(typeof node.focused === 'boolean', path + '.focused');
    need(node.style && typeof node.style === 'object', path + '.style');
    need(finite(node.effectiveOpacity) && node.effectiveOpacity >= 0 && node.effectiveOpacity <= 1, path + '.effectiveOpacity');
    if (screen?.measurement?.renderer === 'react-native') {
      const declared = property => screen.unavailable?.some(fact => fact.element === node.id && fact.property === property && typeof fact.reason === 'string' && fact.reason.length > 0);
      need(typeof node.effectiveBackgroundColor === 'string' || declared('effectiveBackgroundColor'), path + '.effectiveBackgroundColor');
      if (node.ownText && !node.input) {
        need(Array.isArray(node.textRects) && node.textRects.length > 0 && node.textRects.every(rect) || declared('textRects'), path + '.textRects');
        for (const property of ['fontFamily', 'fontSize', 'fontWeight', 'color']) need(typeof node.style?.[property] === 'string' || declared(property), path + '.style.' + property);
      }
    }
  }
  return errors;
}
