import test from 'node:test';
import assert from 'node:assert/strict';
import {routePath} from '../../packages/contracts/src/routes.ts';
import {SearchGenresController, type SearchGenreSlot} from '../src/screens/search/genres.ts';

const artwork = (id: string, role: 'backdrop' | 'poster') => ({kind: 'item', id,
  path: `${routePath('get_metadata_kind_id_art_role', {kind: 'item', id, role})}?v=${'a'.repeat(64)}&w=${role === 'backdrop' ? 1920 : 400}`});
const genre = (value: string, count: number, backdrop = true, covers = 2) => ({value, label: value, count,
  ...(backdrop ? {backdrop: artwork(value + '-backdrop', 'backdrop')} : {}),
  covers: Array.from({length: covers}, (_, index) => artwork(value + '-cover-' + index, 'poster'))});
const page = (genres: readonly unknown[], nextCursor?: string, revision = 'genre-only-opaque') => ({revision, genres, ...(nextCursor ? {nextCursor} : {})});
function deferred<T>() {let resolve!: (value: T) => void;const promise = new Promise<T>(done => {resolve = done;});return {promise, resolve};}
function setup(handler: (path: string, signal?: AbortSignal) => Promise<unknown>, slot: SearchGenreSlot = 'backdrop', limit = 2) {
  let current = true, requests = 0;
  const parent = new AbortController();
  const controller = new SearchGenresController({api: {request: async<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal) => {
    assert.equal(method, 'GET');assert.equal(body, undefined);requests++;return await handler(path, signal) as T;
  }}, isCurrent: () => current, signal: parent.signal, slot, limit});
  return {controller, parent, requests: () => requests, retire: () => {current = false;}};
}

test('genre factory is inert and filters the drawn art slot before taking the server prefix', async () => {
  const owner = setup(async () => page([genre('no-art', 100, false), genre('first', 90), genre('second', 80), genre('later', 70)], 'real-more'));
  try {
    assert.equal(owner.requests(), 0);
    await owner.controller.load();
    const state = owner.controller.getSnapshot();
    assert.deepEqual(state.genres.map(({value, count}) => [value, count]), [['first', 90], ['second', 80]]);
    assert.equal(state.revision, 'genre-only-opaque');assert.equal(state.nextCursor, 'real-more');assert.equal(state.canContinue, false);
    assert(Object.isFrozen(state.genres));assert(!('total' in state));
    await owner.controller.next();assert.equal(owner.requests(), 1, 'a filled prefix must not scan later pages');
  } finally {owner.controller.dispose();}
});

test('phone cover slots require two genuine covers and retain metadata label spelling', async () => {
  const owner = setup(async () => page([genre('single', 100, true, 1), {...genre('science-fiction', 80, false), label: 'Sci-Fi & Fantasy'}]), 'covers');
  try {
    await owner.controller.load();
    assert.deepEqual(owner.controller.getSnapshot().genres.map(({value, label}) => [value, label]), [['science-fiction', 'Sci-Fi & Fantasy']]);
    assert.equal(owner.controller.getSnapshot().canContinue, false);
    assert.equal(owner.controller.getSnapshot().genres[0].backdrop, undefined);
  } finally {owner.controller.dispose();}
});

test('a real continuation fills only the remaining prefix with one coalesced request', async () => {
  const second = deferred<unknown>(), paths: string[] = [];
  const owner = setup(async path => {paths.push(path);return paths.length === 1 ? page([genre('first', 90), genre('missing', 80, false)], 'opaque/+=') : second.promise;});
  try {
    await owner.controller.load();const first = owner.controller.getSnapshot().genres[0];
    assert.equal(owner.controller.getSnapshot().canContinue, true);
    const read = owner.controller.next();assert.equal(owner.controller.next(), read);assert.equal(owner.requests(), 2);
    assert.equal(new URL(paths[1], 'https://local.invalid').searchParams.get('cursor'), 'opaque/+=');
    second.resolve(page([genre('second', 70), genre('third', 60)], 'next-unused'));await read;
    assert.equal(owner.controller.getSnapshot().genres[0], first);
    assert.deepEqual(owner.controller.getSnapshot().genres.map(genre => genre.value), ['first', 'second']);
    assert.equal(owner.controller.getSnapshot().canContinue, false);
  } finally {owner.controller.dispose();}
});

test('changed revisions, repeated cursors and repeated selected facets never combine responses', async () => {
  for (const next of [page([genre('second', 80)], undefined, 'different'), page([genre('second', 80)], 'cursor'), page([genre('first', 90)])]) {
    let reads = 0;
    const owner = setup(async () => ++reads === 1 ? page([genre('first', 90)], 'cursor') : next);
    try {
      await owner.controller.load();
      await assert.rejects(owner.controller.next(), {code: 'stale_continuation'});
      assert.deepEqual(owner.controller.getSnapshot().genres, []);
      assert.equal(owner.controller.getSnapshot().phase, 'error');assert.equal(owner.controller.getSnapshot().nextCursor, undefined);
    } finally {owner.controller.dispose();}
  }
});

test('principal retirement and parent abort refuse late reads and erase private genre state', async () => {
  for (const abort of [false, true]) {
    const response = deferred<unknown>();let signal: AbortSignal | undefined;
    const owner = setup(async (_path, provided) => {signal = provided;return response.promise;});
    const read = owner.controller.load();
    if (abort) {owner.parent.abort();assert.equal(signal?.aborted, true);} else owner.retire();
    response.resolve(page([genre('private', 10)]));await assert.rejects(read, {name: 'AbortError'});
    assert.deepEqual(owner.controller.getSnapshot().genres, []);
    await assert.rejects(owner.controller.next(), {name: 'AbortError'});assert.equal(owner.requests(), 1);
    owner.controller.dispose();
  }
});

test('a cancelled older read cannot erase a newer genre response', async () => {
  const first = deferred<unknown>(), second = deferred<unknown>();let reads = 0;
  const owner = setup(async () => ++reads === 1 ? first.promise : second.promise);
  try {
    const old = assert.rejects(owner.controller.load(), {name: 'AbortError'}), current = owner.controller.load();
    second.resolve(page([genre('current', 10)]));await current;
    first.resolve(page([genre('old', 10)]));await old;
    assert.deepEqual(owner.controller.getSnapshot().genres.map(genre => genre.value), ['current']);
  } finally {owner.controller.dispose();}
});

test('only the current actual facet can capture the canonical video-title browse target', async () => {
  const owner = setup(async () => page([genre('actual-value', 10)]));
  try {
    await owner.controller.load();const actual = owner.controller.getSnapshot().genres[0];
    assert.deepEqual(owner.controller.browseTarget(actual), {libraryId: '*', pivot: 'video-titles', query: {field: 'genre', operator: 'equals', value: 'actual-value'}, sort: [{field: 'title', direction: 'asc'}]});
    assert.throws(() => owner.controller.browseTarget({...actual}), {name: 'AbortError'});
    owner.controller.cancel();assert.throws(() => owner.controller.browseTarget(actual), {name: 'AbortError'});
  } finally {owner.controller.dispose();}
});


test('All genres uses the original authorized page, includes art-less genres, and does not scan', async () => {
  const pages = [page([genre('first', 10), genre('without-art', 4, false)], 'next'), page([genre('later', 2, false)], 'more')];
  const owner = setup(async () => pages.shift(), 'backdrop', 1);
  try {
    await owner.controller.load(); const original = owner.controller.getSnapshot();
    assert.deepEqual(original.genres.map(x => x.value), ['first']);
    owner.controller.showAll(original);
    assert.equal(owner.requests(), 1, 'the checked first page is already in hand');
    assert.deepEqual(owner.controller.getSnapshot().genres.map(x => x.value), ['first', 'without-art']);
    assert.equal(owner.controller.getSnapshot().canContinue, true);
    assert.throws(() => owner.controller.showAll(original), {name: 'AbortError'}, 'stale preview cannot borrow a later window');
    await owner.controller.next();
    assert.equal(owner.requests(), 2, 'only the explicit Next reads the next bounded page');
    assert.deepEqual(owner.controller.getSnapshot().genres.map(x => x.value), ['later'], 'directory pages replace rather than accumulate');
    assert.equal(owner.controller.getSnapshot().all, true);
    owner.retire(); assert.throws(() => owner.controller.showAll(owner.controller.getSnapshot()), {name: 'AbortError'});
  } finally {owner.controller.dispose();}
});


test('an art-less first window still exposes the real All genres entry', async () => {
  const owner = setup(async () => page([genre('without-art', 4, false)]));
  try {
    await owner.controller.load(); const preview = owner.controller.getSnapshot();
    assert.equal(preview.genres.length, 0); assert.equal(preview.hasGenres, true);
    owner.controller.showAll(preview);
    assert.deepEqual(owner.controller.getSnapshot().genres.map(x => x.value), ['without-art']);
  } finally {owner.controller.dispose();}
});
