/**
 * WEB-05: disposing the artwork store ends the work it owns.
 *
 * The store fetches authorized image bytes and hands them to the DOM as object
 * URLs, which pin the encoded bytes for the lifetime of the document. A URL
 * created after disposal has no owner and no later disposal can find it, so the
 * bytes stay pinned — which is what these tests are really about.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {ArtworkStore, defaultConcurrency} from '../src/ui/artwork-store.ts';

/** Node has no object-URL registry; this one also counts what leaked. */
function objectURLs() {
  let next = 0;
  const live = new Set<string>();
  const created: string[] = [];
  (globalThis as any).URL.createObjectURL = () => {
    const url = 'blob:artwork/' + ++next;
    live.add(url);
    created.push(url);
    return url;
  };
  (globalThis as any).URL.revokeObjectURL = (url: string) => void live.delete(url);
  return {live, created};
}

const image = () =>
  new Response(new Blob([new Uint8Array([1, 2, 3])], {type: 'image/png'}), {
    status: 200,
    headers: {'content-type': 'image/png'},
  });

/** An api whose every artwork read can be released, failed or left pending. */
function transport() {
  const pending: {resolve: (r: Response) => void; reject: (e: unknown) => void; signal: AbortSignal; url: string}[] = [];
  const api = {
    baseUrl: 'http://server.test/',
    routeFetch: (url: string, init: RequestInit) =>
      new Promise<Response>((resolve, reject) => {
        const signal = init.signal as AbortSignal;
        // A real fetch rejects when its signal aborts; so must this one, or the
        // test would be asserting against a transport that cannot be cancelled.
        signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), {once: true});
        pending.push({resolve, reject, signal, url});
      }),
  };
  return {api, pending, releaseAll: () => pending.splice(0).forEach(p => p.resolve(image()))};
}

const settled = async (p: Promise<unknown>) => p.then(() => 'resolved', () => 'rejected');
const tick = () => new Promise(done => setTimeout(done, 0));

test('WEB-05: work in flight at disposal is aborted and its promise settles', async () => {
  const urls = objectURLs();
  const {api, pending} = transport();
  const store = new ArtworkStore(api as never, 'token', {concurrency: 6});

  const load = store.load('/art/one.png');
  await tick();
  assert.equal(pending.length, 1, 'the request started');

  store.dispose();
  assert.equal(pending[0]!.signal.aborted, true, 'disposal aborts the request it owns');
  assert.equal(await settled(load), 'rejected', 'and the caller is not left waiting forever');
  assert.equal(urls.created.length, 0);
});

test('WEB-05: a response that wins the race with disposal does not leak its URL', async () => {
  const urls = objectURLs();
  const {api, pending} = transport();
  const store = new ArtworkStore(api as never, 'token', {concurrency: 6});

  const load = store.load('/art/one.png');
  await tick();
  store.dispose();
  // The bytes were already on the way and arrive after disposal.
  pending[0]!.resolve(image());

  assert.equal(await settled(load), 'rejected');
  await tick();
  assert.deepEqual([...urls.live], [], 'any URL created across the race is revoked immediately');
});

test('WEB-05: queued work never starts after disposal, and every caller settles', async () => {
  const urls = objectURLs();
  const {api, pending} = transport();
  const store = new ArtworkStore(api as never, 'token', {concurrency: 6});

  // Six is the concurrency limit, so the seventh is still queued. Outcomes are
  // observed at creation, the way a caller that renders an image does.
  const loads = ['a', 'b', 'c', 'd', 'e', 'f', 'g'].map(n => settled(store.load(`/art/${n}.png`)));
  await tick();
  assert.equal(pending.length, 6, 'six active, one waiting');

  store.dispose();
  await tick();

  assert.equal(pending.length, 6, 'the queued request never started');
  const outcomes = await Promise.all(loads);
  assert.deepEqual(outcomes, Array(7).fill('rejected'), 'all seven callers settle');
  assert.deepEqual([...urls.live], []);
});

test('WEB-05: disposal revokes what it holds, and repeating it is safe', async () => {
  const urls = objectURLs();
  const {api, releaseAll} = transport();
  const store = new ArtworkStore(api as never, 'token', {concurrency: 6});

  const load = store.load('/art/one.png');
  await tick();
  releaseAll();
  assert.equal(await settled(load), 'resolved');
  assert.equal(urls.live.size, 1, 'a settled image is held');

  store.dispose();
  assert.equal(urls.live.size, 0, 'and released on disposal');
  store.dispose();
  assert.equal(urls.live.size, 0, 'a second disposal is a no-op, not an error');
});

test('WEB-05: a disposed store refuses new work instead of starting it', async () => {
  objectURLs();
  const {api, pending} = transport();
  const store = new ArtworkStore(api as never, 'token', {concurrency: 6});
  store.dispose();

  assert.equal(await settled(store.load('/art/one.png')), 'rejected');
  await tick();
  assert.equal(pending.length, 0, 'nothing was requested');
});

test('WEB-05: ordinary loading and eviction still work', async () => {
  const urls = objectURLs();
  const {api, releaseAll} = transport();
  const store = new ArtworkStore(api as never, 'token', {concurrency: 6});

  const load = store.load('/art/one.png');
  await tick();
  releaseAll();
  const url = await load;
  assert.match(url, /^blob:artwork\//);
  assert.equal(store.peek('/art/one.png'), url, 'a settled entry is served from cache');
  assert.equal(await store.load('/art/one.png'), url, 'without a second request');
  assert.equal(urls.created.length, 1);
});

/* WEB-SYS-01: a slow or busy server never becomes a permanent blank. */

/** Manual clock: timers fire only when the test says so. */
function clock() {
  let now = 0;
  const timers: {at: number; fn: () => void; id: number}[] = [];
  let id = 0;
  return {
    now: () => now,
    setTimer: (fn: () => void, ms: number) => { const t = {at: now + ms, fn, id: ++id}; timers.push(t); return t.id as unknown as ReturnType<typeof setTimeout>; },
    clearTimer: (t: ReturnType<typeof setTimeout>) => { const i = timers.findIndex(x => x.id === (t as unknown as number)); if (i >= 0) timers.splice(i, 1); },
    advance(ms: number) { now += ms; for (const t of timers.filter(x => x.at <= now)) { timers.splice(timers.indexOf(t), 1); t.fn(); } },
    pending: () => timers.map(t => t.at - now),
  };
}
const status = (code: number, headers: Record<string, string> = {}) => new Response(null, {status: code, headers});

test('WEB-SYS-01: a 404 with Retry-After is retried after the server’s delay and the same caller gets the image', async () => {
  objectURLs();
  const c = clock();
  const {api, pending} = transport();
  const store = new ArtworkStore(api as never, 'token', {...c, concurrency: 6});
  const load = store.load('/art/show.png');
  await tick();
  pending.shift()!.resolve(status(404, {'Retry-After': '20'}));
  await tick(); await tick();
  assert.equal(pending.length, 0, 'no immediate hammering');
  assert.deepEqual(c.pending(), [20000], 'waits the longer of Retry-After (20 s) and the first backoff (5 s)');
  c.advance(20000);
  await tick();
  assert.equal(pending.length, 1, 'asks again');
  pending.shift()!.resolve(image());
  assert.match(await load, /^blob:artwork\//, 'the original caller resolves — the card fills in without a remount');
});

test('WEB-SYS-01: timeouts back off 5 s, 15 s, 45 s, then fail; a plain 404 fails at once', async () => {
  objectURLs();
  const c = clock();
  const {api, pending} = transport();
  const store = new ArtworkStore(api as never, 'token', {...c, concurrency: 6});
  const load = settled(store.load('/art/slow.png'));
  for (const wait of [5000, 15000, 45000]) {
    await tick();
    pending.shift()!.reject(new TypeError('network'));
    await tick(); await tick();
    assert.deepEqual(c.pending(), [wait]);
    c.advance(wait);
  }
  await tick();
  pending.shift()!.reject(new TypeError('network'));
  assert.equal(await load, 'rejected', 'gives up after three retries');

  const missing = settled(store.load('/art/missing.png'));
  await tick();
  pending.shift()!.resolve(status(404));
  assert.equal(await missing, 'rejected', 'a 404 without Retry-After is not retried');
  assert.deepEqual(c.pending(), []);
});

test('WEB-SYS-01: a retry wait does not hold a slot, so queued posters are not blocked', async () => {
  objectURLs();
  const c = clock();
  const {api, pending} = transport();
  const store = new ArtworkStore(api as never, 'token', {...c, concurrency: 6});
  const loads = Array.from({length: 7}, (_, i) => store.load(`/art/${i}.png`));
  await tick();
  assert.equal(pending.length, 6);
  for (const p of pending.splice(0)) p.resolve(status(503, {'Retry-After': '5'}));
  await tick(); await tick(); await tick();
  assert.equal(pending.length, 1, 'the seventh starts while the first six wait to retry');
  pending.shift()!.resolve(image());
  assert.match(await loads[6]!, /^blob:/);
  void loads;
});

test('WEB-SYS-01: priority work jumps the queue and may use a reserved slot', async () => {
  objectURLs();
  const {api, pending} = transport();
  const store = new ArtworkStore(api as never, 'token', {concurrency: 6});
  for (let i = 0; i < 8; i++) void store.load(`/art/poster-${i}.png`).catch(() => {});
  await tick();
  assert.equal(pending.length, 6, 'six posters in flight, two queued');
  void store.load('/art/hero-backdrop.jpg', {priority: 'high'}).catch(() => {});
  await tick();
  assert.equal(pending.length, 7, 'the hero starts at once in a reserved slot');
  store.dispose();
});

test('WEB-SYS-01: queued work nobody waits for is dropped before it starts', async () => {
  objectURLs();
  const {api, pending} = transport();
  const store = new ArtworkStore(api as never, 'token', {concurrency: 6});
  for (let i = 0; i < 6; i++) void store.load(`/art/busy-${i}.png`).catch(() => {});
  const leaving = new AbortController();
  const gone = settled(store.load('/art/scrolled-away.png', {signal: leaving.signal}));
  await tick();
  leaving.abort();
  assert.equal(await gone, 'rejected');
  pending.shift()!.resolve(image());
  await tick(); await tick();
  assert.equal(pending.length, 5, 'the freed slot was not spent on the abandoned card');
  store.dispose();
});

test('artwork concurrency follows the HTTP version: HTTP/1.1 leaves connections for API calls', () => {
  assert.equal(defaultConcurrency('http/1.1'), 4);
  assert.equal(defaultConcurrency('h2'), 10);
  assert.equal(defaultConcurrency('h3'), 10);
  assert.equal(defaultConcurrency(''), 4);
});

test('artwork starts in screen order (row by row, left to right), not request order', async () => {
  objectURLs();
  const {api, pending} = transport();
  const store = new ArtworkStore(api as never, 'token', {concurrency: 1});
  // One render pass asks bottom row first; the queue still starts top-left.
  for (const [name, order] of [['row2-col0', 2000], ['row1-col1', 1001], ['row1-col0', 1000], ['lookahead', 1_002_000]] as const) void store.load(`/art/${name}.png`, {order, group: `row:${Math.floor(order / 1000)}`}).catch(() => {});
  const started: string[] = [];
  for (let i = 0; i < 4; i++) {
    await tick();
    const next = pending.shift()!;
    started.push(new URL(next.url).pathname.slice(5, -4));
    next.resolve(image());
  }
  assert.deepEqual(started, ['row1-col0', 'row1-col1', 'row2-col0', 'lookahead']);
  store.dispose();
});

test('PERF-22: an evicted URL that is still on screen is revoked only when it unmounts', async () => {
  const {retainArtworkUrl, releaseArtworkUrl} = await import('../src/ui/artwork-store.ts');
  const revoked: string[] = [];
  const original = (globalThis as any).URL.revokeObjectURL;
  (globalThis as any).URL.revokeObjectURL = (url: string) => { revoked.push(url); };
  try {
    const first = retainArtworkUrl('blob:shown'), second = retainArtworkUrl('blob:shown');
    releaseArtworkUrl('blob:shown');
    assert.deepEqual(revoked, []);
    first();
    assert.deepEqual(revoked, []);
    second();
    assert.deepEqual(revoked, ['blob:shown']);
    releaseArtworkUrl('blob:offscreen');
    assert.deepEqual(revoked, ['blob:shown', 'blob:offscreen']);
  } finally { (globalThis as any).URL.revokeObjectURL = original; }
});
