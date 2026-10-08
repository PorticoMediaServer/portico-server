/**
 * PERF-22 + PERF-S16: the web artwork store is capped by bytes, refreshes
 * recency on `peek()`, keeps large images in a separate pool of 4, never
 * revokes a mounted URL, and serves every card from one shared observer.
 *
 * Blob bytes are faked with sized `Blob`s; the store counts each blob's
 * `size` toward the budget (16 MB when `deviceMemory ≤ 2`, else 48 MB —
 * 48 MB under Node, which has no `deviceMemory`).
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {
  ARTWORK_MAX_BYTES,
  ARTWORK_MAX_LARGE,
  isLargeArtworkPath,
  observeArtwork,
  retainArtworkUrl,
  ArtworkStore,
} from '../src/ui/artwork-store.ts';

/** Node has no object-URL registry; this one also counts what leaked. */
function objectURLs() {
  let next = 0;
  const live = new Set<string>();
  (globalThis as any).URL.createObjectURL = () => {
    const url = 'blob:artwork/' + ++next;
    live.add(url);
    return url;
  };
  (globalThis as any).URL.revokeObjectURL = (url: string) => void live.delete(url);
  return {live};
}

const image = (size: number) =>
  new Response(new Blob([new Uint8Array(size)], {type: 'image/png'}), {
    status: 200,
    headers: {'content-type': 'image/png'},
  });

/** An api that answers every artwork read at once with `size` bytes. */
function autoTransport(size: number | ((url: string) => number)) {
  let started = 0;
  const api = {
    baseUrl: 'http://server.test/',
    routeFetch: (url: string, _init: RequestInit) => {
      started++;
      const n = typeof size === 'function' ? size(url) : size;
      return Promise.resolve(image(n));
    },
  };
  return {api, started: () => started};
}

const settled = async (p: Promise<unknown>) => p.then(() => 'resolved', () => 'rejected');

test('PERF-22: blob bytes stay under budget over a 25,000-poster scroll', async () => {
  assert.equal(ARTWORK_MAX_BYTES, 48 * 1024 * 1024, 'no deviceMemory under Node, so the 48 MB budget');
  objectURLs();
  // 25,000 × 100 KB = ~2.5 GB of posters through a 48 MB budget. At 100 KB the
  // byte budget binds long before the 600-entry count backstop (≈ 480 kept).
  const {api} = autoTransport(100 * 1024);
  const store = new ArtworkStore(api as never, 'token', {concurrency: 64});
  const loads = Array.from({length: 25_000}, (_, i) => settled(store.load(`/v1/items/${i}/art/poster?w=400`)));
  const outcomes = await Promise.all(loads);
  assert.ok(outcomes.every(o => o === 'resolved'), 'every poster resolves');
  const stats = store.stats();
  assert.ok(stats.entries < 600, `the byte budget binds first (${stats.entries} entries kept)`);
  assert.ok(stats.bytes <= ARTWORK_MAX_BYTES, `bytes ${stats.bytes} stay under the ${ARTWORK_MAX_BYTES} budget`);
  assert.ok(stats.bytes > ARTWORK_MAX_BYTES - 4 * 1024 * 1024, `the budget is actually used (${stats.bytes})`);
  store.dispose();
});

test('PERF-22: a mounted URL survives eviction pressure and is revoked on unmount', async () => {
  const urls = objectURLs();
  const {api} = autoTransport(32 * 1024);
  const store = new ArtworkStore(api as never, 'token', {concurrency: 8, maxBytes: 64 * 1024});
  const first = await store.load('/v1/items/a/art/poster?w=400');
  const release = retainArtworkUrl(first);
  // 320 KB of newer posters against a 64 KB budget: everything unmounted must go.
  await Promise.all(Array.from({length: 10}, (_, i) => store.load(`/v1/items/n${i}/art/poster?w=400`)));
  assert.equal(store.peek('/v1/items/a/art/poster?w=400'), first, 'the mounted poster is still cached');
  assert.ok(urls.live.has(first), 'and its bytes are not revoked');
  assert.ok(store.stats().bytes <= 64 * 1024, 'the budget still holds');
  release();
  store.dispose();
  assert.ok(!urls.live.has(first), 'unmount after eviction revokes the deferred URL');
});

test('PERF-22 + PERF-S16: peek() refreshes recency, so a revisited poster survives', async () => {
  objectURLs();
  const {api} = autoTransport(32 * 1024);
  const store = new ArtworkStore(api as never, 'token', {concurrency: 8, maxBytes: 96 * 1024});
  const paths = ['/v1/items/a/art/poster?w=400', '/v1/items/b/art/poster?w=400', '/v1/items/c/art/poster?w=400'];
  for (const p of paths) await store.load(p);
  assert.equal(store.peek(paths[0]), store.peek(paths[0]), 'revisit the first poster');
  await store.load('/v1/items/d/art/poster?w=400');
  assert.ok(store.peek(paths[0]), 'the revisited poster survives');
  assert.equal(store.peek(paths[1]), undefined, 'the least recently used poster is evicted instead');
  assert.ok(store.peek(paths[2]), 'newer posters survive');
  assert.ok(store.stats().bytes <= 96 * 1024);
  store.dispose();
});

test('PERF-22: the large pool is capped at 4 and outside the byte budget', async () => {
  objectURLs();
  const {api} = autoTransport(2 * 1024 * 1024);
  const store = new ArtworkStore(api as never, 'token', {concurrency: 8, maxBytes: 64 * 1024});
  assert.ok(isLargeArtworkPath('/v1/items/a/art/backdrop'), 'an unsized backdrop is large');
  assert.ok(isLargeArtworkPath('/v1/items/a/trickplay/set/tiles?at=10'), 'a trickplay sheet is large');
  assert.ok(!isLargeArtworkPath('/v1/items/a/art/poster?w=400'), 'a sized thumbnail is not');
  assert.ok(!isLargeArtworkPath('/v1/items/a/art/poster?size=thumbnail'), 'a thumbnail variant is not');
  const large = Array.from({length: 10}, (_, i) => `/v1/items/backdrop${i}/art/backdrop`);
  await Promise.all(large.map(p => store.load(p, {large: true})));
  const stats = store.stats();
  assert.equal(stats.large, ARTWORK_MAX_LARGE, `exactly ${ARTWORK_MAX_LARGE} large images are kept`);
  assert.equal(stats.bytes, 0, 'large images are not counted toward the thumbnail budget');
  // Thumbnail churn must not evict the large pool, and backdrops must not evict thumbnails.
  await Promise.all(Array.from({length: 20}, (_, i) => store.load(`/v1/items/t${i}/art/poster?w=400`)));
  assert.equal(store.stats().large, ARTWORK_MAX_LARGE, 'the large pool survives thumbnail churn');
  assert.ok(store.peek(large[9]!), 'the newest backdrop survives');
  assert.equal(store.peek(large[0]), undefined, 'the oldest backdrop is evicted');
  store.dispose();
});

test('PERF-22: every card shares one IntersectionObserver owned by the store', () => {
  let instances = 0;
  const observed: unknown[] = [];
  (globalThis as any).IntersectionObserver = class {
    callback: (entries: unknown[]) => void;
    options: unknown;
    constructor(callback: (entries: unknown[]) => void, options: unknown) {
      this.callback = callback;
      this.options = options;
      instances++;
    }
    observe = (el: unknown) => void observed.push(el);
    unobserve = (el: unknown) => void observed.splice(observed.indexOf(el), 1);
    disconnect = () => {};
  };
  try {
    const cleanups = [
      observeArtwork({} as Element, () => {}),
      observeArtwork({} as Element, () => {}),
      observeArtwork({} as Element, () => {}),
    ];
    assert.equal(instances, 1, 'one shared observer for every card');
    assert.equal(observed.length, 3, 'all three frames are watched');
    for (const stop of cleanups) stop();
    assert.equal(observed.length, 0, 'unmounting unwatches');
  } finally {
    delete (globalThis as any).IntersectionObserver;
  }
});

test('PERF-22: a peek() hit starts no work, so a remount paints synchronously', async () => {
  objectURLs();
  const {api, started} = autoTransport(1024);
  const store = new ArtworkStore(api as never, 'token', {concurrency: 4});
  const url = await store.load('/v1/items/a/art/poster?w=400');
  const before = started();
  assert.equal(store.peek('/v1/items/a/art/poster?w=400'), url, 'the remount reads its URL synchronously');
  assert.equal(started(), before, 'with no new request');
  store.dispose();
});
