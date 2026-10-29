import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {browseImporting, browseResultCount} from '../src/screens/browse/importing.ts';

const text = (key: string, count: number) => `${key}:${count}`;

test('P2-59: an importing Reading library counts nothing; a lower bound is "at least"; otherwise exact', () => {
  assert.equal(browseImporting({importing: true}), true);
  assert.equal(browseImporting({capabilities: {importing: true} as never}), true);
  assert.equal(browseImporting({}), false);
  assert.equal(browseResultCount({total: 120, totalIsLowerBound: true}, true, text), undefined, 'no count, not even a lower bound, while importing');
  assert.equal(browseResultCount({total: 120, totalIsLowerBound: false}, true, text), undefined, 'never an exact count while importing');
  assert.equal(browseResultCount({total: 120, totalIsLowerBound: true}, false, text), 'lib.countAtLeast:120');
  assert.equal(browseResultCount({total: 120, totalIsLowerBound: false}, false, text), 'lib.count:120');
  assert.equal(browseResultCount({}, false, text), undefined);
});

test('P2-59: the A–Z rail draws only from anchors, and every count surface goes through the importing rule', () => {
  const results = readFileSync(new URL('../src/screens/browse/BrowseResults.tsx', import.meta.url), 'utf8');
  assert.match(results, /anchors\.length > 0 && !tiles && <AlphabetRail/, 'no anchors (an importing page): no rail, empty or otherwise');
  const library = readFileSync(new URL('../src/screens/browse/Library.tsx', import.meta.url), 'utf8');
  assert.match(library, /browseResultCount\(snapshot, browseImporting\(state\)/);
  const chips = readFileSync(new URL('../src/screens/browse/ScopeChips.tsx', import.meta.url), 'utf8');
  assert.match(chips, /importing \? undefined :/);
  const route = readFileSync(new URL('../src/screens/browse/Library.route.tsx', import.meta.url), 'utf8');
  assert.match(route, /importing \? undefined : readingHeader\.counts/);
});
