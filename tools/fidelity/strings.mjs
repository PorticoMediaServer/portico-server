/** Measured strings are data. This tool neither adds copy nor edits catalogues. */
export function stringInventory(screen) {
  if (screen?.scope !== 'screen' || !Array.isArray(screen.strings)) throw new Error('A screen measurement is required');
  const inventory = new Map();
  for (const value of screen.strings) {
    if (typeof value.text !== 'string' || typeof value.id !== 'string') throw new Error('Invalid measured string');
    // Input values and DOM text are both visible copy. Accessibility labels and
    // placeholders stay distinct so a label cannot excuse missing drawn text.
    const channel = value.kind === 'text' || value.kind === 'value' ? 'visible' : value.kind;
    const key = JSON.stringify([channel, value.text]);
    if (!inventory.has(key)) inventory.set(key, {channel, text: value.text, occurrences: []});
    inventory.get(key).occurrences.push(value.id);
  }
  return [...inventory.values()];
}

export function compareStrings(reference, actual) {
  const expected = stringInventory(reference), observed = stringInventory(actual);
  const key = value => JSON.stringify([value.channel, value.text]);
  const actualByKey = new Map(observed.map(value => [key(value), value]));
  const expectedByKey = new Map(expected.map(value => [key(value), value]));
  const differences = [];
  let required = 0, matched = 0;
  for (const value of expected) {
    const count = value.occurrences.length, actualCount = actualByKey.get(key(value))?.occurrences.length ?? 0;
    required += count; matched += Math.min(count, actualCount);
    if (actualCount < count) differences.push({kind: 'missing-string', channel: value.channel, text: value.text, count: count - actualCount});
  }
  for (const value of observed) {
    const count = value.occurrences.length, expectedCount = expectedByKey.get(key(value))?.occurrences.length ?? 0;
    if (count > expectedCount) differences.push({kind: 'extra-string', channel: value.channel, text: value.text, count: count - expectedCount});
  }
  return {required, matched, differences};
}

/** Exact catalogue matches only: no punctuation, case or Unicode normalization. */
export function catalogueMatches(screen, catalogue) {
  const values = new Map();
  for (const [key, text] of Object.entries(catalogue)) {
    if (typeof text !== 'string') throw new Error('A flat string catalogue is required');
    const keys = values.get(text) ?? []; keys.push(key); values.set(text, keys);
  }
  return stringInventory(screen).map(value => ({...value, catalogueKeys: values.get(value.text) ?? []}));
}
