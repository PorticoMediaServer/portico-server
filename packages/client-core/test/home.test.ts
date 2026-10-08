import test from 'node:test';
import assert from 'node:assert/strict';
import {fetchHome, fetchHomeRow, resetHomeLayout, saveHomeLayout, validateHomeDocument, validateHomeLayout, validateHomeRow, type HomeApi} from '../src/home.ts';
import {fetchItemRecommendations, fetchSuggestions, moreLikeThisRow, validateItemRecommendations, validateSuggestions} from '../src/recommendations.ts';

function entry(id = 'm1', kind = 'movie') {
  return {id, kind, title: `Title ${id}`, libraryId: 'movies', available: true, navigation: {view: 'item', entityId: id}, playback: {itemId: id}};
}
function row(changes: Record<string, unknown> = {}): any {
  return {
    id: 'recent_movies', title: 'Recently Added in Movies', kind: 'recent', artworkShape: 'poster', endpoint: '/v1/home/rows/recent_movies',
    libraryId: 'movies', privacySensitivity: 'catalog', policyState: 'available', priority: 100, cacheTtlSeconds: 120,
    required: false, hideable: true, reorderable: true, critical: false, cursorCapable: true,
    entries: [entry('m1'), entry('m2')], total: 5, start: 0, limit: 2, hasMore: true, nextCursor: 'opaque.signature',
    revision: {catalog: 3, viewer: 1}, ...changes,
  };
}
function document(changes: Record<string, unknown> = {}): any {
  return {
    serverId: 'server', viewerFence: 'fence', rows: [row()], layout: {revision: 2, rowOrder: ['recent_movies'], hiddenRowIds: []},
    revision: {catalog: 3, viewer: 1}, generatedAt: '2026-09-16T12:00:00.000Z', ...changes,
  };
}
function api(handler: (path: string, method?: string, body?: unknown) => unknown): HomeApi {
  return {request: async <T>(path: string, method?: string, body?: unknown) => handler(path, method, body) as T};
}

test('a home document keeps the rows, policy and paging shape the server published', () => {
  const parsed = validateHomeDocument(document());
  assert.equal(parsed.rows.length, 1);
  assert.equal(parsed.rows[0].artworkShape, 'poster');
  assert.equal(parsed.rows[0].entries[1].id, 'm2');
  assert.equal(parsed.rows[0].cursorCapable, true);
  assert.equal(parsed.layout.revision, 2);
  assert.throws(() => (parsed.rows as unknown as unknown[]).push(row()));
});

test('an unknown card kind drops only that Home entry and retains the published row total', () => {
  const parsed = validateHomeDocument(document({rows: [row({entries: [entry('m1'), entry('future', 'interactive'), entry('m2')]})]}));
  assert.deepEqual(parsed.rows[0].entries.map(card => card.id), ['m1', 'm2']);
  assert.equal(parsed.rows[0].total, 5);
  assert.equal(parsed.rows[0].nextCursor, 'opaque.signature');
});

test('malformed home payloads are refused rather than half-rendered', () => {
  const cases: Record<string, unknown>[] = [
    document({rows: [row({artworkShape: 'circle'})]}),
    document({rows: [row({required: true, hideable: true})]}),
    document({rows: [row({entries: [entry('m1'), entry('m1')]})]}),
    document({rows: [row({total: 1})]}),
    document({rows: [row(), row()]}),
    document({rows: [row({revision: {catalog: -1, viewer: 0}})]}),
    document({generatedAt: ''}),
    document({layout: {revision: 2, rowOrder: ['a', 'a'], hiddenRowIds: []}}),
    // The server never publishes a row the viewer hid, nor an empty non-critical row.
    document({layout: {revision: 2, rowOrder: [], hiddenRowIds: ['recent_movies']}}),
    document({rows: [row({entries: [], total: 0, critical: false, hasMore: false})]}),
  ];
  for (const payload of cases) assert.throws(() => validateHomeDocument(payload), {code:'invalid_home'});
  assert.doesNotThrow(() => validateHomeDocument(document({rows: [row({entries: [], total: 0, critical: true, hasMore: false, id: 'continue', required: true, hideable: false})]})));
});

test('row paging asks for a cursor or an anchored range, never both', async () => {
  const requested: string[] = [];
  const client = api(path => {
    requested.push(path);
    return row({start: 2, entries: [entry('m3')], limit: 1});
  });
  await fetchHomeRow(client, 'recent_movies', {limit: 1, cursor: 'opaque.signature'});
  await fetchHomeRow(client, 'recent_movies', {limit: 1, start: 2, revision: '3:1', anchorId: 'm3'});
  assert.equal(requested[0], '/v1/home/rows/recent_movies?limit=1&cursor=opaque.signature');
  assert.equal(requested[1], '/v1/home/rows/recent_movies?limit=1&start=2&revision=3%3A1&anchorId=m3');
  await assert.rejects(() => fetchHomeRow(client, 'recent_movies', {cursor: 'c', start: 1}), /cursor or by anchored range/);
  await assert.rejects(() => fetchHomeRow(api(() => row({id: 'other'})), 'recent_movies'), {code:'invalid_home'});
});

test('home and layout mutations use the published endpoints', async () => {
  const seen: {path: string; method?: string; body?: unknown}[] = [];
  const client = api((path, method, body) => {
    seen.push({path, method, body});
    return path.startsWith('/v1/home?') ? document() : {revision: 3, rowOrder: ['favorites'], hiddenRowIds: ['trending']};
  });
  await fetchHome(client, 6);
  const layout = await saveHomeLayout(client, {expectedRevision: 2, rowOrder: ['favorites'], hiddenRowIds: ['trending'], idempotencyKey: 'k'});
  assert.equal(layout.revision, 3);
  await resetHomeLayout(client, 'k2');
  assert.deepEqual(seen.map(call => `${call.method} ${call.path}`), ['GET /v1/home?limit=6', 'PUT /v1/home/layout', 'POST /v1/home/layout/reset']);
  assert.deepEqual(seen[1].body, {expectedRevision: 2, idempotencyKey: 'k', rowOrder: ['favorites'], hiddenRowIds: ['trending']});
});

test('suggestions carry a reason, a source and a bounded score', () => {
  const payload = {items: [{entry: entry('m4'), reason: 'Because you watched Movie m1', source: 'because_you_watched', score: 0.75}], total: 1, revision: {catalog: 3, viewer: 1}, generatedAt: '2026-09-16T12:00:00.000Z'};
  const parsed = validateSuggestions(payload);
  assert.equal(parsed.items[0].source, 'because_you_watched');
  for (const broken of [{...payload, items: [{...payload.items[0], reason: ''}]}, {...payload, items: [{...payload.items[0], score: 0}]}, {...payload, items: [{...payload.items[0], score: 2}]}, {...payload, total: 2}]) {
    assert.throws(() => validateSuggestions(broken), {code:'invalid_recommendations'});
  }
});

test('per-item recommendation rows must be explained and must exclude their own item', async () => {
  const related = row({id: 'genre:tmdb:16', title: 'More Animation', kind: 'related', relation: 'genre', provider: 'tmdb', evidenceId: '16', entries: [entry('m2')], total: 1, limit: 1, hasMore: false, nextCursor: '', cursorCapable: false, reorderable: false, priority: 0, cacheTtlSeconds: 300, endpoint: '/v1/items/m1/recommendations'});
  const parsed = validateItemRecommendations({itemId: 'm1', rows: [related], revision: {catalog: 3, viewer: 1}}, 'm1');
  assert.equal(parsed.rows[0].relation, 'genre');
  assert.throws(() => validateItemRecommendations({itemId: 'm1', rows: [{...related, relation: undefined}], revision: {catalog: 3, viewer: 1}}, 'm1'), {code:'invalid_recommendations'});
  assert.throws(() => validateItemRecommendations({itemId: 'm1', rows: [{...related, entries: [entry('m1')], total: 1}], revision: {catalog: 3, viewer: 1}}, 'm1'), {code:'invalid_recommendations'});
  assert.throws(() => validateItemRecommendations({itemId: 'm1', rows: [related], revision: {catalog: 3, viewer: 1}}, 'other'), {code:'invalid_recommendations'});
  const paths: string[] = [];
  const client = api(path => {
    paths.push(path);
    return path.startsWith('/v1/suggestions')
      ? {items: [], total: 0, revision: {catalog: 3, viewer: 1}, generatedAt: '2026-09-16T12:00:00.000Z'}
      : {itemId: 'm1', rows: [related], revision: {catalog: 3, viewer: 1}};
  });
  await fetchSuggestions(client, 4);
  await fetchItemRecommendations(client, 'm1', 6);
  assert.deepEqual(paths, ['/v1/suggestions?limit=4', '/v1/items/m1/recommendations?limit=6']);
});

test('a row descriptor without entries still validates its own shape', () => {
  const empty = validateHomeRow(row({id: 'continue', required: true, hideable: false, critical: true, entries: [], total: 0, hasMore: false, nextCursor: ''}));
  assert.equal(empty.entries.length, 0);
  assert.equal(empty.required, true);
});

test('PERF-S14: a row paged sideways keeps at most HOME_ROW_MAX_ENTRIES and then stops offering more', async () => {
  const {appendHomeRowPage, HOME_ROW_MAX_ENTRIES} = await import('../src/home.ts');
  const entry = (i: number) => ({id: `e${i}`, kind: 'movie', title: `Title ${i}`, navigation: {view: 'item', entityId: `e${i}`}});
  const page = (from: number, n: number, cursor: string) => validateHomeRow(row({id: 'recent_movies', entries: Array.from({length: n}, (_, k) => entry(from + k)), total: 1_000_000, hasMore: true, nextCursor: cursor}));
  let current = page(0, 24, 'c1');
  for (let i = 1; current.nextCursor; i++) {
    assert.ok(i < 20, 'paging ends');
    current = appendHomeRowPage(current, page(i * 24 - 1, 24, `c${i + 1}`));
  }
  assert.equal(current.entries.length, HOME_ROW_MAX_ENTRIES);
  assert.equal(new Set(current.entries.map(e => e.id)).size, HOME_ROW_MAX_ENTRIES, 'an overlapping page adds no duplicate');
  assert.equal(current.nextCursor, '');
  assert.equal(current.hasMore, true, 'the server still has more; See all is the way there');
  assert.equal(current.total, 1_000_000);
});

test('CD-31/CD-32: Home stays bounded at 64 rows and 64 layout IDs including DVR rows', () => {
  const makeRows = (n: number) => Array.from({length: n}, (_, i) => row({id: `recent_lib${i}`}));
  // 64 populated rows validate; the 65th fails closed rather than half-rendering.
  assert.doesNotThrow(() => validateHomeDocument(document({rows: makeRows(64), layout: {revision: 2, rowOrder: makeRows(64).map(r => r.id), hiddenRowIds: []}})));
  assert.throws(() => validateHomeDocument(document({rows: makeRows(65)})), {code: 'invalid_home'});
  // Saving 41–64 row layouts succeeds, including a DVR row (recent_<64hex>).
  const dvrRow = 'recent_' + 'a'.repeat(64);
  const order41 = [...Array.from({length: 40}, (_, i) => `recent_lib${i}`), dvrRow];
  assert.equal(order41.length, 41);
  assert.doesNotThrow(() => validateHomeLayout({revision: 3, rowOrder: order41, hiddenRowIds: []}));
  const order64 = [...Array.from({length: 63}, (_, i) => `recent_lib${i}`), dvrRow];
  assert.equal(order64.length, 64);
  assert.doesNotThrow(() => validateHomeLayout({revision: 3, rowOrder: order64, hiddenRowIds: []}));
  assert.throws(() => validateHomeLayout({revision: 3, rowOrder: [...order64, 'extra'], hiddenRowIds: []}), {code: 'invalid_home'});
  // Required rows stay protected: a required row cannot be hideable.
  assert.throws(() => validateHomeDocument(document({rows: [row({id: 'continue', required: true, hideable: true})]})), {code: 'invalid_home'});
});

test('a movie\'s More like this row is the similar-titles row under the catalogue heading, else the first row', () => {
  const base = {kind: 'related', provider: 'local', total: 1, limit: 12, hasMore: false, nextCursor: '', cursorCapable: false, reorderable: false, priority: 0, cacheTtlSeconds: 300, endpoint: '/v1/items/m1/recommendations'};
  const genre = row({...base, id: 'genre:tmdb:16', title: 'More Animation', relation: 'genre', evidenceId: '16', entries: [entry('m2')]});
  const similar = row({...base, id: 'because_you_watched:local:m1', title: 'Because you watched Title m1', relation: 'because_you_watched', evidenceId: 'm1', entries: [entry('m3')]});
  const parsed = (rows: any[]) => validateItemRecommendations({itemId: 'm1', rows, revision: {catalog: 3, viewer: 1}}, 'm1');
  assert.deepEqual(moreLikeThisRow(parsed([genre, similar]), 'More like this'), {id: 'because_you_watched:local:m1', title: 'More like this', entries: parsed([similar]).rows[0].entries});
  assert.equal(moreLikeThisRow(parsed([genre]), 'More like this')?.title, 'More Animation');
  assert.equal(moreLikeThisRow(parsed([]), 'More like this'), undefined);
  assert.equal(moreLikeThisRow(null, 'More like this'), undefined);
});
