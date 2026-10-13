import test from 'node:test';
import assert from 'node:assert/strict';
import {parseBrowseCapabilities,parseBrowseResult,parseBrowseFacets,validateBrowseQuery,formatBrowseAnchor,BrowseQueryBuilder,BrowseQueryError,type BrowseCapabilities} from '../src/browse.ts';
import {parsePinOrder,parseLibraryNavigationPins,parsePinnedLibraries,pinOrderRequest,libraryNavigationRequest} from '../src/library-pins.ts';

function capabilitiesPayload(changes: Record<string, unknown> = {}): any {
  return {
    library: {id: 'lib', name: 'Movies', kind: 'movie', defaultView: 'discover', pinned: false},
    pivots: [
      {id: 'discover', labelKey: 'library.discover', entityKinds: ['movie'], defaultSort: [{field: 'title', direction: 'asc'}], supportedViews: ['rail'], browsable: false},
      {id: 'movies', labelKey: 'library.movies', entityKinds: ['movie'], defaultSort: [{field: 'title', direction: 'asc'}], supportedViews: ['grid', 'list'], browsable: true},
    ],
    resolvedPivot: {id: 'movies', labelKey: 'library.movies', entityKinds: ['movie'], defaultSort: [{field: 'title', direction: 'asc'}], supportedViews: ['grid', 'list'], browsable: true},
    fields: [
      {id: 'title', labelKey: 'browse.field.title', type: 'string', operators: ['equals', 'starts-with', 'contains'], controlHint: 'text', complexity: 'quick', cost: 'indexed', applicableKinds: ['movie']},
      {id: 'year', labelKey: 'browse.field.year', type: 'number', operators: ['equals', 'between', 'at-least'], controlHint: 'number-range', complexity: 'quick', cost: 'indexed', applicableKinds: ['movie'], facetSource: {endpoint: '/v1/libraries/{id}/facets', field: 'year'}},
      {id: 'playState', labelKey: 'browse.field.playState', type: 'enum', operators: ['equals'], controlHint: 'select', complexity: 'quick', cost: 'indexed-join', applicableKinds: ['movie'], allowedValues: ['unplayed', 'in-progress', 'played']},
      {id: 'favorite', labelKey: 'browse.field.favorite', type: 'boolean', operators: ['equals'], controlHint: 'toggle', complexity: 'quick', cost: 'indexed-join', applicableKinds: ['movie']},
      {id: 'genre', labelKey: 'browse.field.genre', type: 'identity-set', operators: ['contains', 'contains-any'], controlHint: 'facet-multi-select', complexity: 'quick', cost: 'indexed-join', applicableKinds: ['movie'], facetSource: {endpoint: '/v1/libraries/{id}/facets', field: 'genre'}},
    ],
    sorts: [
      {id: 'title', labelKey: 'sort.title', directions: ['asc', 'desc'], defaultDirection: 'asc', expensive: false, applicableKinds: ['movie']},
      {id: 'year', labelKey: 'sort.year', directions: ['asc', 'desc'], defaultDirection: 'desc', expensive: false, applicableKinds: ['movie']},
    ],
    quickFilters: [{id: 'favorites', labelKey: 'filter.favorites', query: {field: 'favorite', operator: 'equals', value: true}}],
    queryLimits: {maximumDepth: 5, maximumClauses: 40, maximumBytes: 65536, maximumSorts: 3, defaultLimit: 40, maximumLimit: 100, cursorTtlSeconds: 1800},
    ...changes,
  };
}
const capabilities: BrowseCapabilities = parseBrowseCapabilities(capabilitiesPayload());

function resultPayload(changes: Record<string, unknown> = {}): any {
  return {
    pivot: 'movies',
    applied: {query: {field: 'genre', operator: 'contains', value: 'Action'}, sort: [{field: 'title', direction: 'asc'}], seek: null},
    entries: [{id: 'm1', kind: 'movie', title: 'Alpha', navigation: {view: 'item', entityId: 'm1'}}],
    pageInfo: {start: 0, total: 3, revision: 'browse_abc', hasMore: true, nextCursor: 'cursor'},
    positionIndex: [{key: '#', index: 0}, {key: 'A', index: 1}],
    ...changes,
  };
}

test('capabilities parse only what the server can execute', () => {
  assert.equal(capabilities.resolvedPivot?.id, 'movies');
  assert.equal(capabilities.queryLimits.maximumDepth, 5);
  assert.equal(capabilities.fields.find(f => f.id === 'year')?.facetSource?.field, 'year');
  // The library, the query limits and the bounds of the lists stay exact.
  assert.throws(() => parseBrowseCapabilities(capabilitiesPayload({library: {id: '', name: 'Movies', kind: 'movie', defaultView: 'discover'}})), {code:'invalid_browse'});
  assert.throws(() => parseBrowseCapabilities(capabilitiesPayload({fields: Array.from({length: 129}, (_, i) => ({id: `f${i}`, labelKey: 'k', type: 'string', operators: ['equals'], controlHint: 'text', complexity: 'quick', cost: 'indexed', applicableKinds: []}))})), {code:'invalid_browse'});
  assert.throws(() => parseBrowseCapabilities(capabilitiesPayload({pivots: 'nope'})), {code:'invalid_browse'});
  assert.throws(() => parseBrowseCapabilities(capabilitiesPayload({resolvedPivot: {id: 'ghost', labelKey: 'k', entityKinds: [], defaultSort: [], supportedViews: [], browsable: true}})), {code:'invalid_browse'});
  assert.throws(() => parseBrowseCapabilities(capabilitiesPayload({queryLimits: {maximumDepth: 5, maximumClauses: 40, maximumBytes: 1, maximumSorts: 3, defaultLimit: 40, maximumLimit: 10, cursorTtlSeconds: 1}})), {code:'invalid_browse'});
});

test('a filter, sort, pivot or quick filter this app cannot read is not offered and the rest are kept', () => {
  const field = {id: 'title', labelKey: 'k', type: 'string', operators: ['equals'], controlHint: 'text', complexity: 'quick', cost: 'indexed', applicableKinds: ['movie']};
  const parsed = parseBrowseCapabilities(capabilitiesPayload({
    fields: [
      field, {...field, labelKey: 'again'},
      {...field, id: 'unknownOperators', operators: ['nonsense']},
      {...field, id: 'someOperators', operators: ['equals', 'resembles']},
      {...field, id: 'unknownControl', controlHint: 'dial'},
      {...field, id: 'unknownType', type: 'colour'},
      {...field, id: 'hints', complexity: 'expert', cost: 'full-scan', future: {a: 1}},
      {...field, id: 'brokenFacet', facetSource: {endpoint: 7}},
      'nope',
    ],
    sorts: [
      {id: 'title', labelKey: 'sort.title', directions: ['asc', 'sideways'], defaultDirection: 'sideways', applicableKinds: ['movie']},
      {id: 'unknownDirections', labelKey: 'k', directions: ['sideways'], defaultDirection: 'sideways', expensive: false, applicableKinds: []},
      {id: 'year', labelKey: 'sort.year', directions: ['asc', 'desc'], defaultDirection: 'desc', expensive: true, applicableKinds: ['movie']},
    ],
    pivots: [
      {id: 'movies', labelKey: 'library.movies', entityKinds: ['movie', 7], defaultSort: [{field: 'title', direction: 'asc'}, {field: 'year', direction: 'sideways'}], supportedViews: ['grid'], browsable: true},
      {id: 'unsure', labelKey: 'library.unsure'},
      {labelKey: 'no id'},
    ],
    quickFilters: [{id: 'favorites', labelKey: 'filter.favorites', query: {field: 'favorite', operator: 'equals', value: true}}, {id: 'future', labelKey: 'k', query: {field: 'mood', operator: 'resembles', value: 'calm'}}],
  }));
  assert.deepEqual(parsed.fields.map(f => f.id), ['title', 'someOperators', 'hints']);
  assert.equal(parsed.fields[0].labelKey, 'k', 'a repeated id keeps its first appearance');
  assert.deepEqual(parsed.fields[1].operators, ['equals']);
  assert.deepEqual([parsed.fields[2].complexity, parsed.fields[2].cost, 'future' in parsed.fields[2]], ['advanced', 'indexed-join', false]);
  assert.deepEqual(parsed.sorts.map(v => [v.id, v.directions, v.defaultDirection, v.expensive]), [['title', ['asc'], 'asc', false], ['year', ['asc', 'desc'], 'desc', true]]);
  assert.deepEqual(parsed.pivots, [
    {id: 'movies', labelKey: 'library.movies', entityKinds: ['movie'], defaultSort: [{field: 'title', direction: 'asc'}], supportedViews: ['grid'], browsable: true},
    {id: 'unsure', labelKey: 'library.unsure', entityKinds: [], defaultSort: [], supportedViews: [], browsable: false},
  ]);
  assert.deepEqual(parsed.quickFilters.map(f => f.id), ['favorites']);
});

test('browse pages parse position index and page info; identity, what was applied and bounds stay exact', () => {
  const page = parseBrowseResult(resultPayload());
  assert.equal(page.pageInfo.total, 3);
  assert.deepEqual([...page.positionIndex], [{key: '#', index: 0}, {key: 'A', index: 1}]);
  assert.equal(page.applied.sort[0].field, 'title');
  assert.throws(() => parseBrowseResult(resultPayload({applied: {query: {field: 'genre', operator: 'nonsense', value: 'x'}, sort: [], seek: null}})), {code:'invalid_browse'});
  assert.throws(() => parseBrowseResult(resultPayload({pivot: ''})), {code:'invalid_browse'});
  assert.throws(() => parseBrowseResult(resultPayload({pageInfo: {start: 0, total: 3, revision: '', hasMore: true, nextCursor: 'cursor'}})), {code:'invalid_browse'});
  assert.throws(() => parseBrowseResult(resultPayload({pageInfo: {start: 0, total: 3, revision: 'r', hasMore: true, nextCursor: 7}})), {code:'invalid_browse'});
  assert.throws(() => parseBrowseResult(resultPayload({entries: 'nope'})), {code:'invalid_browse'});
  assert.throws(() => parseBrowseResult(resultPayload({entries: Array.from({length: 201}, (_, i) => ({id: `m${i}`, kind: 'movie', title: 'A'})), pageInfo: {start: 0, total: 500, revision: 'r', hasMore: true, nextCursor: ''}})), {code:'invalid_browse'});
  // A page none of whose entries can be read is refused rather than shown as empty.
  assert.throws(() => parseBrowseResult(resultPayload({entries: [{id: 'm1', kind: 'hologram', title: 'A'}]})), {code:'invalid_browse'});
});

test('one browse entry that cannot be read is left out, and the total and hasMore follow the rows the server sent', () => {
  const movie = (id: string) => ({id, kind: 'movie', title: id});
  const page = parseBrowseResult(resultPayload({entries: [movie('m1'), movie('m1'), {id: 'x', kind: 'hologram', title: 'Future'}, {id: 'broken'}, {...movie('m2'), year: 0, future: true}], pageInfo: {start: 0, total: 9, revision: 'r', hasMore: true, nextCursor: 'cursor'}}));
  assert.deepEqual(page.entries.map(e => e.id), ['m1', 'm2']);
  assert.deepEqual(page.entries[1], movie('m2'));
  assert.deepEqual([page.pageInfo.total, page.pageInfo.hasMore], [9, true]);
  // The total is at least where the page ends; hasMore says whether rows remain after it.
  const short = parseBrowseResult(resultPayload({entries: [movie('m1'), movie('m2'), movie('m3')], pageInfo: {start: 5, total: 1, revision: 'r', hasMore: true}}));
  assert.deepEqual([short.pageInfo.total, short.pageInfo.hasMore, short.pageInfo.nextCursor], [8, false, '']);
  // Anchors out of order or past the total are left out of the rail.
  assert.deepEqual([...parseBrowseResult(resultPayload({positionIndex: [{key: 'B', index: 2}, {key: 'A', index: 1}, {key: 'Z', index: 3}]})).positionIndex], [{key: 'B', index: 2}]);
});

test('M20: anchors parse on every sort, with empty keys and unknown shapes dropped', () => {
  const entry = (id: string) => ({id, kind: 'movie', title: `Title ${id}`});
  const ids = ['m1', 'm2', 'm3', 'm4'];
  // A year-sorted page: years plus an empty bucket for rows without a value.
  const year = parseBrowseResult(resultPayload({
    applied: {query: null, sort: [{field: 'year', direction: 'desc'}], seek: null},
    entries: ids.map(entry),
    pageInfo: {start: 0, total: 4, revision: 'r', hasMore: false, nextCursor: ''},
    positionIndex: [{key: '1995', index: 0}, {key: '1988', index: 2}, {key: '', index: 3}],
  }));
  assert.deepEqual([...year.positionIndex], [{key: '1995', index: 0}, {key: '1988', index: 2}, {key: '', index: 3}]);
  // The index is optional: continuation pages and older servers carry none.
  for (const missing of [undefined, null]) {
    const page = parseBrowseResult(resultPayload({
      entries: ids.map(entry),
      pageInfo: {start: 0, total: 4, revision: 'r', hasMore: false, nextCursor: ''},
      positionIndex: missing,
    }));
    assert.deepEqual([...page.positionIndex], []);
  }
  // Entries with an unknown shape are dropped; the page still parses.
  const dropped = parseBrowseResult(resultPayload({
    entries: ids.slice(0, 3).map(entry),
    pageInfo: {start: 0, total: 3, revision: 'r', hasMore: false, nextCursor: ''},
    positionIndex: [{key: '1995', index: 0}, {key: 99, index: 1}, null, {index: 2}, {key: '', index: 2}],
  }));
  assert.deepEqual([...dropped.positionIndex], [{key: '1995', index: 0}, {key: '', index: 2}]);
  // An anchor that is out of order is left out of the rail; the page is kept.
  assert.deepEqual([...parseBrowseResult(resultPayload({
    entries: ids.map(entry),
    pageInfo: {start: 0, total: 4, revision: 'r', hasMore: false, nextCursor: ''},
    positionIndex: [{key: '1995', index: 2}, {key: '1988', index: 1}],
  })).positionIndex], [{key: '1995', index: 2}]);
});

test('M20: anchor keys read as rail labels for every sort', () => {
  assert.equal(formatBrowseAnchor('1995', 'year', 'No value'), '1995');
  assert.equal(formatBrowseAnchor('1990s', 'year', 'No value'), '1990s');
  assert.equal(formatBrowseAnchor('D', 'title', 'No value'), 'D');
  assert.equal(formatBrowseAnchor('2026-09', 'added', 'No value'), 'Sep 2026');
  assert.equal(formatBrowseAnchor('2026-01', 'lastPlayed', 'No value'), 'Jan 2026');
  assert.equal(formatBrowseAnchor('2026', 'added', 'No value'), '2026');
  assert.equal(formatBrowseAnchor('8', 'communityRating', 'No value'), '8★');
  assert.equal(formatBrowseAnchor('0', 'personalRating', 'No value'), '0★');
  assert.equal(formatBrowseAnchor('90', 'duration', 'No value'), '1h 30m');
  assert.equal(formatBrowseAnchor('30', 'duration', 'No value'), '30m');
  assert.equal(formatBrowseAnchor('60', 'duration', 'No value'), '1h');
  assert.equal(formatBrowseAnchor('0', 'duration', 'No value'), '0m');
  assert.equal(formatBrowseAnchor('', 'year', 'No value'), 'No value');
  assert.equal(formatBrowseAnchor('', 'added', 'No value'), 'No value');
  // Unknown sorts and unparseable keys pass through untouched, never fatal.
  assert.equal(formatBrowseAnchor('1995', 'nonsense', 'No value'), '1995');
  assert.equal(formatBrowseAnchor('soon', 'duration', 'No value'), 'soon');
  assert.equal(formatBrowseAnchor('2026-13', 'added', 'No value'), '2026-13');
});

test('facet pages keep the values they can read; the field asked for and the bound stay exact', () => {
  const page = parseBrowseFacets({field: 'genre', values: [{value: 'Action', label: 'Action', count: 2}]}, 'genre');
  assert.equal(page.values[0].count, 2);
  assert.throws(() => parseBrowseFacets({field: 'genre', values: []}, 'year'), {code:'invalid_browse'});
  assert.throws(() => parseBrowseFacets({field: 'genre', values: 'nope'}), {code:'invalid_browse'});
  assert.throws(() => parseBrowseFacets({field: 'genre', values: Array.from({length: 201}, (_, i) => ({value: `v${i}`, label: 'x', count: 1}))}), {code:'invalid_browse'});
  const loose = parseBrowseFacets({field: 'genre', future: 1, values: [{value: 'a', label: 'A', count: 1}, {value: 'a', label: 'Again', count: 2}, {label: 'no value', count: 1}, {value: 'b'}, {value: 'c', label: 'C', count: -1, future: true}, 7]});
  assert.deepEqual(loose.values, [{value: 'a', label: 'A', count: 1}, {value: 'b', label: 'b', count: 0}, {value: 'c', label: 'C', count: 0}]);
});

test('queries are validated against the published capabilities before they are sent', () => {
  validateBrowseQuery(capabilities, {field: 'year', operator: 'between', value: [1990, 1999]});
  for (const [node, field] of [
    [{field: 'studio', operator: 'equals', value: 'x'}, 'query.field'],
    [{field: 'title', operator: 'at-least', value: 'x'}, 'query.operator'],
    [{field: 'year', operator: 'equals', value: 'x'}, 'query.value'],
    [{field: 'playState', operator: 'equals', value: 'halfway'}, 'query.value'],
    [{field: 'favorite', operator: 'equals', value: 'yes'}, 'query.value'],
    [{field: 'year', operator: 'between', value: [1]}, 'query.value'],
    [{field: 'year', operator: 'between', value: 1990}, 'query.value'],
  ] as const) {
    assert.throws(() => validateBrowseQuery(capabilities, node as any), (e: unknown) => e instanceof BrowseQueryError && e.field === field);
  }
  let deep: any = {field: 'title', operator: 'equals', value: 'x'};
  for (let level = 0; level < 6; level += 1) deep = {not: deep};
  assert.throws(() => validateBrowseQuery(capabilities, deep), (e: unknown) => e instanceof BrowseQueryError && e.field.startsWith('query.not'));
});

test('the builder produces a request the server contract accepts', () => {
  const body = new BrowseQueryBuilder(capabilities, 'movies')
    .where('genre', 'contains', 'Action')
    .quickFilter('favorites')
    .sort('title', 'asc')
    .limit(25)
    .seek('d')
    .build();
  assert.deepEqual(body, {pivot: 'movies', query: {all: [{field: 'genre', operator: 'contains', value: 'Action'}, {field: 'favorite', operator: 'equals', value: true}]}, sort: [{field: 'title', direction: 'asc'}], limit: 25, seek: {prefix: 'd'}});
  assert.throws(() => new BrowseQueryBuilder(capabilities, 'discover'), (e: unknown) => e instanceof BrowseQueryError);
  assert.throws(() => new BrowseQueryBuilder(capabilities, 'songs'), (e: unknown) => e instanceof BrowseQueryError);
  assert.throws(() => new BrowseQueryBuilder(capabilities).limit(500), (e: unknown) => e instanceof BrowseQueryError && e.field === 'limit');
  assert.throws(() => new BrowseQueryBuilder(capabilities).sort('title').sort('title'), (e: unknown) => e instanceof BrowseQueryError);
  assert.throws(() => new BrowseQueryBuilder(capabilities).sort('nonsense'), (e: unknown) => e instanceof BrowseQueryError);
  assert.throws(() => new BrowseQueryBuilder(capabilities).sort('year', 'desc').seek('a'), (e: unknown) => e instanceof BrowseQueryError && e.field === 'seek');
  assert.throws(() => new BrowseQueryBuilder(capabilities).cursor('c').range({start: 0}), (e: unknown) => e instanceof BrowseQueryError);
  const ranged = new BrowseQueryBuilder(capabilities).range({start: 40, revision: 'browse_abc', anchorId: 'm4'}).build();
  assert.deepEqual(ranged.range, {start: 40, revision: 'browse_abc', anchorId: 'm4'});
});

test('pin arrangements are read and written as server state', () => {
  const order = parsePinOrder({serverId: 'server', viewerFence: 'fence', revision: 2, order: [{kind: 'view', id: 'a'}, {kind: 'collection', id: 'b'}]}, 'server');
  assert.equal(order.order[0].id, 'a');
  assert.throws(() => parsePinOrder({serverId: 'other', viewerFence: 'f', revision: 1, order: []}, 'server'), {code:'invalid_pins'});
  assert.throws(() => parsePinOrder({serverId: 'server', viewerFence: 'f', revision: 1, order: [{kind: 'view', id: 'a'}, {kind: 'view', id: 'a'}]}, 'server'), {code:'invalid_pins'});
  assert.throws(() => parsePinOrder({serverId: 'server', viewerFence: 'f', revision: 1, order: [{kind: 'library', id: 'a'}]}, 'server'), {code:'invalid_pins'});

  const pins = parseLibraryNavigationPins({pinnedLibraryIds: ['a', 'b'], revision: 3});
  assert.deepEqual([...pins.pinnedLibraryIds], ['a', 'b']);
  assert.throws(() => parseLibraryNavigationPins({pinnedLibraryIds: ['a', 'a'], revision: 1}), {code:'invalid_pins'});

  const libraries = parsePinnedLibraries({items: [{id: 'b', name: 'B', kind: 'tv', defaultView: 'browse', pinned: true}, {id: 'a', name: 'A', kind: 'movie', defaultView: 'discover', pinned: false}]});
  assert.equal(libraries[0].id, 'b');
  assert.throws(() => parsePinnedLibraries({items: [{id: 'a', name: 'A', kind: 'movie', defaultView: 'discover', pinned: false}, {id: 'b', name: 'B', kind: 'tv', defaultView: 'browse', pinned: true}]}), {code:'invalid_pins'});

  assert.deepEqual(pinOrderRequest(2, [{kind: 'view', id: 'a'}]), {expectedRevision: 2, order: [{kind: 'view', id: 'a'}]});
  assert.throws(() => pinOrderRequest(-1, []), /cannot be saved/i);
  assert.deepEqual(libraryNavigationRequest(0, ['a']), {expectedRevision: 0, pinnedLibraryIds: ['a']});
  assert.throws(() => libraryNavigationRequest(0, ['a', 'a']), /cannot be saved/i);
});
