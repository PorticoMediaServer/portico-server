import test from 'node:test';
import assert from 'node:assert/strict';
import {createWindowedCollection} from '@core/collections/index.ts';
import {createStaticCollection} from '../src/ui/static-collection.ts';
import {clearListWindows, listWindowKey, MAX_REMEMBERED_LISTS, recallListWindow, saveListWindow} from '../src/app/list-position.ts';
import {browsePageSource} from '../src/app/browse-page-source.ts';
import {formatBrowseAnchor} from '@core/browse.ts';

const tick = (ms = 5) => new Promise(r => setTimeout(r, ms));

test('PERF-S04: position memory keeps the last N lists keyed by route plus query', () => {
  clearListWindows();
  assert.equal(MAX_REMEMBERED_LISTS, 5);
  const key = (pivot: string, query: string) => listWindowKey({serverId: 's1', viewerId: 'v1', libraryId: 'lib', pivot, query});
  assert.equal(key('movies', '[["a"]]'), key('movies', '[["a"]]'));
  assert.notEqual(key('movies', '[["a"]]'), key('movies', '[["b"]]'));
  assert.notEqual(key('movies', 'q'), key('albums', 'q'));
  for (let i = 0; i < 5; i++) saveListWindow(key(`p${i}`, 'q'), {first: i * 60, items: [`e${i}`], total: 1000, scrollY: i});
  assert.deepEqual(recallListWindow(key('p0', 'q')), {first: 0, items: ['e0'], total: 1000, scrollY: 0, at: recallListWindow(key('p0', 'q'))!.at});
  // k0 was recalled (most recent now); saving a sixth list evicts k1 instead.
  saveListWindow(key('p5', 'q'), {first: 300, items: ['e5'], total: 1000, scrollY: 5});
  assert.ok(recallListWindow(key('p0', 'q')));
  assert.equal(recallListWindow(key('p1', 'q')), undefined);
  assert.ok(recallListWindow(key('p5', 'q')));
  // An empty window is not worth remembering.
  saveListWindow(key('empty', 'q'), {first: 0, items: [], total: 0, scrollY: 0});
  assert.equal(recallListWindow(key('empty', 'q')), undefined);
  clearListWindows();
});

test('PERF-S04: scrolling up after a letter jump loads the previous page in one request', async () => {
  // The browse engine pages by offset (`range.start`, app/browse.ts); this pins
  // the client half: a backward ensureRange fetches exactly the needed page.
  const calls: {start: number; count: number}[] = [];
  const collection = createWindowedCollection<string>({
    fetchPage: async (start, count) => {
      calls.push({start, count});
      return {items: Array.from({length: count}, (_, i) => `e${start + i}`), total: 25000};
    },
    pageSize: 60, maxResidentPages: 10, prefetchPages: 0, keyOf: s => s,
  });
  // Jump to W.
  collection.ensureRange(14400, 14459);
  await tick();
  assert.ok(collection.itemAt(14400));
  calls.length = 0;
  // Scroll up into V: its page arrives in exactly one range request (plus the
  // collection's one travel-direction prefetch), never a walk from the top.
  collection.ensureRange(14340, 14399);
  await tick();
  assert.equal(calls.filter(c => c.start === 14340).length, 1);
  assert.ok(calls.length <= 2);
  assert.equal(collection.itemAt(14340), 'e14340');
  collection.dispose();
});

test('PERF-S04: a remembered window serves its range with the whole-list total', async () => {
  const {collection, setItems} = createStaticCollection<string>({keyOf: s => s, base: 180});
  const items = Array.from({length: 60}, (_, i) => `e${180 + i}`);
  setItems(items, 1000);
  collection.ensureRange(180, 239);
  await tick();
  assert.equal(collection.getSnapshot().total, 1000);
  assert.equal(collection.itemAt(180), 'e180');
  assert.equal(collection.itemAt(239), 'e239');
  // Outside the remembered window: placeholders, not invented items.
  assert.equal(collection.itemAt(179), undefined);
  assert.equal(collection.slotAt(179).kind, 'placeholder');
  assert.equal(collection.slotAt(999).kind, 'placeholder');
  collection.dispose();
});

test('Q4: an expired revision reloads at the current first visible index, not at the top', async () => {
  // browsePageSource pins the first page's revision; a later page carrying a
  // stale revision must clear it and reload the same window once.
  const seen: {start: number; revision: string | undefined}[] = [];
  const entry = (id: string) => ({id, kind: 'movie', title: `Title ${id}`});
  const page = (start: number, revision: string) => ({
    pivot: 'movies',
    applied: {query: null, sort: [{field: 'title', direction: 'asc'}], seek: null},
    entries: Array.from({length: 60}, (_, i) => entry(`e${start + i}`)),
    pageInfo: {start, total: 1000, revision, hasMore: start + 60 < 1000, nextCursor: ''},
    positionIndex: [],
  });
  const api = {
    request: async (_path: string, _method?: string, body?: unknown) => {
      const range = (body as {range?: {start?: number; revision?: string}}).range ?? {};
      const start = range.start ?? 0;
      seen.push({start, revision: range.revision});
      if (seen.length === 1) return page(start, 'rev-one');
      if (range.revision === 'rev-one') throw Object.assign(new Error('expired'), {code: 'stale_continuation', status: 409});
      return page(start, 'rev-two');
    },
  };
  const source = browsePageSource(api, {libraryId: 'lib', pivot: 'movies', predicates: [], sort: {field: 'title', direction: 'asc'}});
  const signal = new AbortController().signal;
  const first = await source(0, 60, signal);
  assert.equal(first.items[0]!.id, 'e0');
  assert.deepEqual(seen[0], {start: 0, revision: undefined});
  // The visible window is at 480 when the revision expires.
  const recovered = await source(480, 60, signal);
  assert.equal(recovered.items[0]!.id, 'e480');
  assert.equal(recovered.items.length, 60);
  // The stale attempt kept its window (480 with rev-one); the retry reloaded
  // the same window (480) without a revision — never a reset to 0.
  assert.deepEqual(seen[1], {start: 480, revision: 'rev-one'});
  assert.deepEqual(seen[2], {start: 480, revision: undefined});
  assert.ok(!seen.some(s => s.start === 0 && s.revision === undefined && seen.indexOf(s) > 0));
  // The fresh revision is pinned for the next window.
  await source(540, 60, signal);
  assert.deepEqual(seen[3], {start: 540, revision: 'rev-two'});
});

test('M20: a year-sorted library jumps to 1995 in one range request; scrolling back is one more', async () => {
  // The position rail's anchors are server bucket positions; selecting one jumps with
  // `range.start`, and scrolling up afterwards loads the previous window in one request.
  const calls: {start: number}[] = [];
  const anchors = [{key: '1995', index: 480}, {key: '1988', index: 540}, {key: '', index: 600}];
  const entry = (id: string) => ({id, kind: 'movie', title: `Title ${id}`});
  const api = {
    request: async (_path: string, _method?: string, body?: unknown) => {
      const range = (body as {range?: {start?: number; revision?: string}}).range ?? {};
      const start = range.start ?? 0;
      calls.push({start});
      const first = calls.length === 1;
      return {
        pivot: 'movies',
        applied: {query: null, sort: [{field: 'year', direction: 'desc'}], seek: null},
        entries: Array.from({length: 60}, (_, i) => entry(`e${start + i}`)),
        pageInfo: {start, total: 1000, revision: 'rev', hasMore: start + 60 < 1000, nextCursor: ''},
        positionIndex: first ? anchors : [],
      };
    },
  };
  const source = browsePageSource(api, {libraryId: 'lib', pivot: 'movies', predicates: [], sort: {field: 'year', direction: 'desc'}});
  const signal = new AbortController().signal;
  const first = await source(0, 60, signal);
  assert.deepEqual(first.positionIndex, {'1995': 480, '1988': 540, '': 600});
  calls.length = 0;
  // Selecting the 1995 anchor jumps with range.start in exactly one request.
  const jumped = await source(480, 60, signal);
  assert.equal(jumped.items[0]!.id, 'e480');
  assert.deepEqual(calls, [{start: 480}]);
  // Scrolling up loads the previous window (start − limit) in one request.
  calls.length = 0;
  const back = await source(420, 60, signal);
  assert.equal(back.items[0]!.id, 'e420');
  assert.deepEqual(calls, [{start: 420}]);
});

test('M20: an empty anchor key is labelled from the catalogue', () => {
  assert.equal(formatBrowseAnchor('', 'year', 'No value'), 'No value');
  assert.equal(formatBrowseAnchor('', 'added', 'No value'), 'No value');
  assert.equal(formatBrowseAnchor('2026-09', 'added', 'No value'), 'Sep 2026');
  assert.equal(formatBrowseAnchor('8', 'communityRating', 'No value'), '8★');
  assert.equal(formatBrowseAnchor('90', 'duration', 'No value'), '1h 30m');
});

test('a window belongs to its viewer: another viewer never recalls it, and a late save from the old one is ignored', () => {
  const a = listWindowKey({serverId: 's1', viewerId: 'adult', libraryId: 'lib', query: 'q'});
  saveListWindow(a, {first: 0, items: ['Adult title'], total: 1, scrollY: 0});
  assert.equal(recallListWindow<string>(a)?.items[0], 'Adult title');
  const kid = listWindowKey({serverId: 's1', viewerId: 'kid', libraryId: 'lib', query: 'q'});
  assert.equal(recallListWindow(kid), undefined);
  // The adult screen unmounting after the switch saves under its old key.
  saveListWindow(a, {first: 0, items: ['Adult title'], total: 1, scrollY: 0});
  assert.equal(recallListWindow(a), undefined);
  // Back to the adult viewer: the memory was cleared at the switch.
  const again = listWindowKey({serverId: 's1', viewerId: 'adult', libraryId: 'lib', query: 'q'});
  assert.equal(recallListWindow(again), undefined);
  // The same ids on another server are another scope.
  const other = listWindowKey({serverId: 's2', viewerId: 'adult', libraryId: 'lib', query: 'q'});
  assert.notEqual(other, again);
});

test('a remembered window that starts mid-page keeps every item at its own index', async () => {
  const items = Array.from({length: 120}, (_, i) => `e${170 + i}`);
  const {collection, setItems} = createStaticCollection<string>({keyOf: s => s, base: 170});
  setItems(items, 1000);
  collection.ensureRange(120, 300);
  await tick(20);
  // Every item that shows is at its own index; the page that begins before the
  // window (120–179) stays a placeholder rather than taking shifted items.
  for (const index of [180, 200, 289]) assert.equal(collection.itemAt(index), `e${index}`, `index ${index}`);
  for (const index of [120, 150, 170, 179]) assert.equal(collection.itemAt(index), undefined, `index ${index}`);
  collection.dispose();
});
