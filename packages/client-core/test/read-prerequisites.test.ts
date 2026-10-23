import test from 'node:test';
import assert from 'node:assert/strict';
import {parseLibraryMediaCapabilities, libraryVisibleOnTV} from '../src/read-prerequisites.ts';
import {parseLibraryList} from '../src/library-list.ts';
import {parseBrowseCapabilities} from '../src/browse.ts';
import {shapeAspect, shapeFor} from '../src/presentation/card-shape.ts';
import {decode} from '../../contracts/src/runtime.ts';
import {schemas} from '../../contracts/src/schemas.ts';
import {parsePinnedLibraries} from '../src/library-pins.ts';
import {readLibrarySummaries} from '../src/library-summary.ts';

test('directory consumes producer capability facts without inferring Read from audiobook identity', () => {
  const read = {tv: false, reading: false, cardShape: 'portrait'};
  const listen = {tv: true, reading: false, cardShape: 'square'};
  const {items} = parseLibraryList({items: [
    {id: 'r', name: 'Books', kind: 'book', capabilities: read},
    {id: 'a', name: 'Audiobooks', kind: 'audiobook', capabilities: listen},
    {id: 'future', name: 'Unknown', kind: 'new-kind', capabilities: {...read, tv: true}},
  ]});
  assert.deepEqual(items.map(libraryVisibleOnTV), [false, true, true]);
  assert.deepEqual(items.map(l => l.capabilities?.reading), [false, false, false]);
  assert.equal(items[0].capabilities?.cardShape, 'portrait');
  assert.equal(shapeAspect('portrait'), 3 / 4);
  assert.equal(shapeFor('book'), 'square');
  assert.equal(shapeFor('audiobook_file'), 'square');
  assert.equal(shapeFor('category', 'grid', 'read'), 'portrait');
});

test('unreadable media capabilities confer no TV or reading capability', () => {
  for (const raw of [null, [], {}, {tv: 'true', reading: false, cardShape: 'portrait'}, {tv: true, reading: true, cardShape: 'poster'}, {tv: true, reading: 1, cardShape: 'poster'}, {tv: true, reading: false, cardShape: 'future'}]) {
    assert.equal(parseLibraryMediaCapabilities(raw), undefined);
    const {items} = parseLibraryList({items: [{id: 'x', name: 'X', kind: 'book', capabilities: raw}]});
    assert.equal(items[0].capabilities, undefined);
    assert.equal(libraryVisibleOnTV(items[0]), false);
  }
  assert.equal(libraryVisibleOnTV({}), false);
});

test('browse retains the current library capability without inventing one across libraries', () => {
  const raw = {library: {id: 'a', name: 'Audiobooks', kind: 'audiobook', capabilities: {tv: true, reading: false, cardShape: 'square'}}, pivots: [], fields: [], sorts: [], queryLimits: {maximumDepth: 5, maximumClauses: 40, maximumBytes: 65536, maximumSorts: 1, defaultLimit: 40, maximumLimit: 100, cursorTtlMs: 1800000}};
  assert.deepEqual(parseBrowseCapabilities(raw).library.capabilities, raw.library.capabilities);
  assert.equal(parseBrowseCapabilities({...raw, library: {id: '', kind: '', capabilities: raw.library.capabilities}}).library.capabilities, undefined);
});

test('the generated library schema describes current producer support and decodes its primitive types', () => {
  const library = {id: 'r', name: 'Books', kind: 'book', defaultView: 'browse', pinned: false, capabilities: {tv: false, reading: false, cardShape: 'portrait'}};
  assert.doesNotThrow(() => decode('GetLibrariesResponse', {items: [library]}));
  const fields = schemas.GetLibrariesResponse.properties.items.items.properties.capabilities.properties;
  assert.deepEqual(fields.reading.enum, [false]);
  assert.deepEqual(fields.cardShape.enum, ['poster', 'square', 'portrait']);
  assert.throws(() => decode('GetLibrariesResponse', {items: [{...library, capabilities: {...library.capabilities, reading: 'false'}}]}));
  assert.throws(() => decode('GetLibrariesResponse', {items: [{...library, capabilities: {...library.capabilities, cardShape: 3}}]}));
});

test('pinned and summary consumers retain the same capability authority as the directory', async () => {
  const capabilities = {tv: false, reading: false, cardShape: 'portrait'};
  const row = {id: 'r', name: 'Books', kind: 'book', defaultView: 'browse', pinned: true, capabilities, summary: {counts: [], coverUrls: [], latestArtwork: '', firstScan: false, empty: true, emptyKnown: true}};
  assert.deepEqual(parsePinnedLibraries({items: [row]})[0].capabilities, capabilities);
  const api = {request: async <T,>(): Promise<T> => ({items: [row]} as T)};
  const summaries = await readLibrarySummaries(api);
  assert.deepEqual(summaries[0].capabilities, capabilities);
  assert.equal(libraryVisibleOnTV(summaries[0]), false);
  assert.deepEqual(summaries[0].summary.counts, []);
});
