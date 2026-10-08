/** A season's episodes page by cursor; the workspace seed covers page 0. */
import test from 'node:test';
import assert from 'node:assert/strict';
import {EPISODE_PAGE, episodeSource} from '../src/app/episodes.ts';

const scope = {serverId: 'server', viewerId: 'hosted/account/profile'};
const TOTAL = 100;
const ep = (i: number) => ({id: `ep${i}`, kind: 'episode', title: `Episode ${i}`, navigation: {view: 'item', entityId: `ep${i}`}, playback: {itemId: `ep${i}`, startSeconds: 0}});
const cursorOf = (page: number) => ((page + 1) * EPISODE_PAGE < TOTAL ? `c${page + 1}` : '');

function projection(page: number) {
  const entries = Array.from({length: Math.min(EPISODE_PAGE, TOTAL - page * EPISODE_PAGE)}, (_, k) => ep(page * EPISODE_PAGE + k));
  return {
    scope: {serverId: 'server', libraryId: 'lib', libraryKind: 'tv', view: 'season', entityId: 's1', viewerFence: 'fence'},
    revision: {catalog: 1, viewer: 1},
    heading: {key: 'entity.title', fallback: 'Show · Episodes'},
    navigation: [],
    query: {sort: 'episode', direction: 'asc', category: '', q: '', limit: EPISODE_PAGE, searchMode: 'none'},
    sorts: [{id: 'episode', labelKey: 'sort.episode', directions: ['asc']}],
    filters: [],
    sections: [{id: 'episodes', type: 'list', heading: {key: 'content.episodes', fallback: 'Episodes'}, entries, totalCount: TOTAL, nextCursor: cursorOf(page)}],
  };
}

function api(log: string[] = []) {
  return {
    log,
    request: async <T,>(path: string): Promise<T> => {
      log.push(path);
      const cursor = new URL(path, 'https://server.test').searchParams.get('cursor');
      const page = cursor ? Number(cursor.slice(1)) : 0;
      return {episodes: projection(page)} as T;
    },
  };
}

const where = {libraryId: 'lib', showId: 'show1', seasonId: 's1'};
const signal = () => new AbortController().signal;
const seed = {entries: projection(0).sections[0].entries, nextCursor: 'c1', total: TOTAL};

test('the seeded first page costs no request; the next page follows its cursor', async () => {
  const a = api();
  const fetch = episodeSource(a as never, scope, where, seed as never);
  const first = await fetch(0, EPISODE_PAGE, signal());
  assert.equal(first.items.length, EPISODE_PAGE);
  assert.equal(first.total, TOTAL);
  assert.equal(a.log.length, 0);
  const second = await fetch(EPISODE_PAGE, EPISODE_PAGE, signal());
  assert.equal((second.items[0] as {id: string}).id, `ep${EPISODE_PAGE}`);
  assert.equal(a.log.length, 1);
  assert.match(a.log[0]!, /showId=show1.*seasonId=s1.*cursor=c1/);
});

test('jumping ahead walks forward from the nearest known cursor', async () => {
  const a = api();
  const fetch = episodeSource(a as never, scope, where);
  const last = await fetch(2 * EPISODE_PAGE, EPISODE_PAGE, signal());
  assert.equal((last.items[0] as {id: string}).id, 'ep80');
  assert.equal(last.items.length, 20);
  assert.equal(a.log.length, 3, 'pages 0 and 1 are walked to learn page 2’s cursor');
  assert.ok(!a.log[0]!.includes('cursor='), 'page 0 reads without a cursor');
});

test('a finished list answers empty without another request', async () => {
  const a = api();
  const fetch = episodeSource(a as never, scope, where);
  await fetch(2 * EPISODE_PAGE, EPISODE_PAGE, signal());
  const calls = a.log.length;
  const beyond = await fetch(3 * EPISODE_PAGE, EPISODE_PAGE, signal());
  assert.deepEqual(beyond.items, []);
  assert.equal(a.log.length, calls, 'the null cursor short-circuits the read');
});

test('a projection the core refuses fails the read', async () => {
  const bad = {request: async <T,>(): Promise<T> => ({episodes: {bogus: true}} as T)};
  await assert.rejects(episodeSource(bad as never, scope, where)(0, EPISODE_PAGE, signal()));
});
