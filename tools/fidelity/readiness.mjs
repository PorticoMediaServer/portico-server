/** Shared font readiness facts; no guessed fallback or font-source URLs. */
export function fontReadiness(screen, side) {
  const differences = [];
  const required = new Map(screen.strings.filter(value => value.style).map(value => [value.style.fontFamily + '|' + value.style.fontWeight, value.style]));
  for (const style of required.values()) {
    const family = style.fontFamily.split(',')[0].trim().replace(/^['"]|['"]$/g, ''), weight = Number(style.fontWeight);
    if (['system-ui', 'sans-serif', 'serif', 'monospace'].includes(family)) continue;
    if (!screen.fontFaces.some(face => {
      const name = face.family.replace(/^['"]|['"]$/g, ''), range = face.weight.split(/\s+/).map(Number);
      return name === family && face.status === 'loaded' && (range.length === 1 ? range[0] === weight : weight >= range[0] && weight <= range[1]);
    })) differences.push({kind: 'font-not-loaded', label: `${side} ${family} ${weight}`});
  }
  return differences;
}
