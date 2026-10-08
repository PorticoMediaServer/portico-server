import test from 'node:test';
import assert from 'node:assert/strict';
import {fetchHomeLayout, validateHomeLayoutView, type HomeApi} from '../src/home.ts';
import {validateContentEntry} from '../src/library-content.ts';

function layoutRow(changes: Record<string, unknown> = {}): any {
  return {
    id: 'continue', title: 'Continue Watching', kind: 'continue', artworkShape: 'landscape',
    required: true, hideable: false, reorderable: true, hidden: false, ...changes,
  };
}
function layoutView(changes: Record<string, unknown> = {}): any {
  return {
    revision: 3, rowOrder: ['continue', 'watchlist'], hiddenRowIds: ['trending_now'],
    rows: [
      layoutRow(),
      layoutRow({id: 'recent_movies', title: 'Recently Added in Movies', kind: 'recent', artworkShape: 'poster', libraryId: 'movies', required: false, hideable: true, reorderable: true, hidden: false}),
      layoutRow({id: 'trending_now', title: 'Trending Now', kind: 'community', artworkShape: 'poster', required: false, hideable: true, reorderable: true, hidden: true}),
    ],
    ...changes,
  };
}
function api(handler: (path: string, method?: string, body?: unknown) => unknown): HomeApi {
  return {request: async <T>(path: string, method?: string, body?: unknown) => handler(path, method, body) as T};
}

test('a layout view round-trips and freezes', () => {
  const parsed = validateHomeLayoutView(layoutView());
  assert.equal(parsed.revision, 3);
  assert.deepEqual([...parsed.rowOrder], ['continue', 'watchlist']);
  assert.deepEqual([...parsed.hiddenRowIds], ['trending_now']);
  assert.equal(parsed.rows.length, 3);
  assert.equal(parsed.rows[1].libraryId, 'movies');
  assert.equal(parsed.rows[2].hidden, true);
  assert.throws(() => (parsed.rows as unknown as unknown[]).push(layoutRow({id: 'x'})), TypeError);
  assert.throws(() => ((parsed.rows[0] as unknown as Record<string, unknown>).title = 'x'), TypeError);
});

test('layout view rejections', () => {
  const cases: unknown[] = [
    layoutView({rows: 'nope'}),
    layoutView({rows: Array.from({length: 65}, (_, i) => layoutRow({id: `r${i}`}))}),
    layoutView({rows: [layoutRow({id: ''})]}),
    layoutView({rows: [layoutRow({title: '' .padEnd(1025, 'x')})]}),
    layoutView({rows: [layoutRow({kind: ''})]}),
    layoutView({rows: [layoutRow({artworkShape: 'circle'})]}),
    layoutView({rows: [layoutRow({libraryId: ''})]}),
    layoutView({rows: [layoutRow({required: 'yes'})]}),
    layoutView({rows: [layoutRow(), layoutRow()]}),
    layoutView({rows: [layoutRow({id: 'a', required: true, hideable: true, hidden: false})]}),
    layoutView({rows: [layoutRow({id: 'a', required: true, hideable: false, hidden: true})]}),
    layoutView({rows: [layoutRow({id: 'a', required: false, hideable: false, hidden: true})]}),
    layoutView({revision: -1}),
    layoutView({rowOrder: ['a', 'a']}),
  ];
  for (const payload of cases) assert.throws(() => validateHomeLayoutView(payload), {code: 'invalid_home'});
});

test('fetchHomeLayout hits GET /v1/home/layout with no query', async () => {
  const seen: string[] = [];
  const client = api(path => {
    seen.push(path);
    return layoutView({revision: 3, rowOrder: ['continue', 'watchlist'], hiddenRowIds: ['trending_now']});
  });
  const view = await fetchHomeLayout(client);
  assert.deepEqual(seen, ['/v1/home/layout']);
  assert.equal(view.revision, 3);
});

test('new entry fields are accepted', () => {
  const parsed = validateContentEntry({id: 'm1', kind: 'movie', title: 'Title', year: 2024, contentRating: 'PG-13', genres: ['Drama', 'Crime'], watchlisted: true});
  assert.equal(parsed.year, 2024);
  assert.equal(parsed.contentRating, 'PG-13');
  assert.deepEqual([...(parsed.genres ?? [])], ['Drama', 'Crime']);
  assert.equal(parsed.watchlisted, true);
  assert.throws(() => ((parsed.genres as unknown as unknown[]).push('x')), TypeError);
  const absent = validateContentEntry({id: 'm1', kind: 'movie', title: 'Title'});
  assert.equal(absent.year, undefined);
  assert.equal(absent.watchlisted, undefined);
});

test('new entry fields are rejected', () => {
  const bad: unknown[] = [
    {id: 'm1', kind: 'movie', title: 'T', year: 0},
    {id: 'm1', kind: 'movie', title: 'T', year: -5},
    {id: 'm1', kind: 'movie', title: 'T', year: 2024.5},
    {id: 'm1', kind: 'movie', title: 'T', year: '2024'},
    {id: 'm1', kind: 'movie', title: 'T', contentRating: 13},
    {id: 'm1', kind: 'movie', title: 'T', contentRating: 'x'.repeat(65)},
    {id: 'm1', kind: 'movie', title: 'T', watchlisted: 'yes'},
    {id: 'm1', kind: 'movie', title: 'T', watchlisted: 1},
  ];
  for (const payload of bad) assert.throws(() => validateContentEntry(payload), {code: 'invalid_projection'});
});

// §4a and be/pages (artists carry three genres, albums more): genres degrade, never fail the entry.
test('genres keep the readable names and drop the rest', () => {
  assert.deepEqual([...validateContentEntry({id: 'm1', kind: 'movie', title: 'T', genres: ['a', 'b', 'c']}).genres!], ['a', 'b', 'c']);
  assert.deepEqual([...validateContentEntry({id: 'm1', kind: 'movie', title: 'T', genres: ['Drama', 123, '']}).genres!], ['Drama']);
  assert.equal(validateContentEntry({id: 'm1', kind: 'movie', title: 'T', genres: 'Drama'}).genres, undefined);
  assert.equal(validateContentEntry({id: 'm1', kind: 'movie', title: 'T', genres: [123]}).genres, undefined);
});

test('an episode entry carries its own still; a malformed still fails like any art path', () => {
  const base = {id: 'e1', kind: 'episode', title: 'Pilot', backdropUrl: '/v1/metadata/show/s1/art/backdrop?v=1'};
  assert.equal(validateContentEntry({...base, stillUrl: '/v1/metadata/item/e1/art/still?v=2&w=640'}).stillUrl, '/v1/metadata/item/e1/art/still?v=2&w=640');
  assert.equal(validateContentEntry(base).stillUrl, undefined);
  assert.throws(() => validateContentEntry({...base, stillUrl: 42}));
});
