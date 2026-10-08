import test from 'node:test';
import assert from 'node:assert/strict';
import {createWindowedCollection, type FetchPage} from '../src/collections/index.ts';

type Item = {id: string; n: number};
const settle = async () => { for (let i = 0; i < 10; i++) await new Promise(setImmediate); };

/** A synthetic server: items are computed from the offset, so nothing here is O(total) either. */
function server(total: number, options: {prefix?: string; delay?: boolean; positionIndex?: Record<string, number>; fail?: (start: number) => boolean} = {}) {
  const calls: {start: number; count: number; signal: AbortSignal}[] = [];
  const held: {resolve: () => void}[] = [];
  const fetchPage: FetchPage<Item> = async (start, count, signal) => {
    calls.push({start, count, signal});
    if (options.delay) await new Promise<void>(resolve => held.push({resolve}));
    if (signal.aborted) throw Object.assign(new Error('aborted'), {name: 'AbortError'});
    if (options.fail?.(start)) throw new Error('boom');
    const n = Math.max(0, Math.min(count, total - start));
    const items: Item[] = [];
    for (let i = 0; i < n; i++) items.push({id: `${options.prefix ?? 'i'}${start + i}`, n: start + i});
    return {items, total, positionIndex: options.positionIndex};
  };
  return {fetchPage, calls, release: () => { for (const h of held.splice(0)) h.resolve(); }};
}

test('the first page sizes the collection; placeholders have stable keys', async () => {
  const s = server(1_000_000);
  const c = createWindowedCollection({fetchPage: s.fetchPage, pageSize: 50, maxResidentPages: 8, keyOf: i => i.id});
  assert.equal(c.getSnapshot().total, undefined);
  assert.equal(c.keyAt(10), 'placeholder:10');
  c.ensureRange(0, 30);
  await settle();
  assert.equal(c.getSnapshot().total, 1_000_000);
  assert.equal(c.getSnapshot().status, 'ready');
  assert.equal(c.itemAt(10)?.n, 10);
  assert.equal(c.keyAt(10), 'i10');
  assert.equal(c.keyAt(999_999), 'placeholder:999999');
  assert.deepEqual(c.slotAt(999_998), {kind: 'placeholder', index: 999_998, key: 'placeholder:999998'});
  assert.equal(s.calls[0]!.start, 0);
});

test('1M items with random jumps: resident items stay within the budget and nothing is O(total)', async () => {
  const total = 1_000_000, pageSize = 60, maxResidentPages = 6;
  const s = server(total);
  // Catch any O(total) allocation through the array constructors.
  const RealArray = Array;
  const realFrom = Array.from;
  let biggest = 0;
  (globalThis as {Array: ArrayConstructor}).Array = new Proxy(RealArray, {construct(target, args) { if (typeof args[0] === 'number') biggest = Math.max(biggest, args[0]); return Reflect.construct(target, args); }, apply(target, self, args) { if (typeof args[0] === 'number') biggest = Math.max(biggest, args[0]); return Reflect.apply(target, self, args); }});
  RealArray.from = ((value: {length?: number}, ...rest: unknown[]) => { if (value && typeof value.length === 'number') biggest = Math.max(biggest, value.length); return (realFrom as (...a: unknown[]) => unknown[]).call(RealArray, value, ...rest); }) as typeof Array.from;
  try {
    const c = createWindowedCollection({fetchPage: s.fetchPage, pageSize, maxResidentPages, keyOf: i => i.id});
    let seed = 7;
    const random = () => (seed = (seed * 1103515245 + 12345) % 2147483648) / 2147483648;
    for (let jump = 0; jump < 200; jump++) {
      const first = Math.floor(random() * (total - 40));
      c.ensureRange(first, first + 39);
      await settle();
      const {items, pages} = c.resident();
      assert.ok(items <= maxResidentPages * pageSize, `jump ${jump}: ${items} resident items`);
      assert.ok(pages <= maxResidentPages);
      assert.equal(c.itemAt(first)?.n, first, `item at ${first}`);
      assert.equal(c.itemAt(first + 39)?.n, first + 39);
    }
    assert.ok(biggest < total, `an array of length ${biggest} was created`);
    assert.ok(s.calls.length < 200 * 6, `${s.calls.length} page requests`);
  } finally {
    (globalThis as {Array: ArrayConstructor}).Array = RealArray;
    RealArray.from = realFrom;
  }
});

test('a billion-item collection costs the same as a small one', async () => {
  const s = server(1_000_000_000);
  const c = createWindowedCollection({fetchPage: s.fetchPage, pageSize: 100, maxResidentPages: 5, keyOf: i => i.id});
  const before = process.memoryUsage().heapUsed;
  for (const first of [0, 999_999_900, 500_000_000, 123_456_789]) { c.ensureRange(first, first + 50); await settle(); }
  assert.equal(c.itemAt(123_456_800)?.n, 123_456_800);
  assert.ok(c.resident().items <= 5 * 100);
  assert.ok(process.memoryUsage().heapUsed - before < 20 * 1024 * 1024);
});

test('prefetches one page either side, two ahead in the direction of travel; dedupes in-flight pages', async () => {
  const s = server(10_000, {delay: true});
  const c = createWindowedCollection({fetchPage: s.fetchPage, pageSize: 100, maxResidentPages: 20, keyOf: i => i.id, maxConcurrent: 10});
  c.ensureRange(0, 50);
  c.ensureRange(0, 60);
  assert.deepEqual(s.calls.map(x => x.start), [0], 'before the total is known only the visible page is requested, once');
  s.release(); await settle();
  assert.deepEqual(s.calls.map(x => x.start), [0, 100], 'once the total is known, one page ahead');
  s.release(); await settle();
  let before = s.calls.length;
  c.ensureRange(1000, 1050); // page 10, moving forward
  await settle();
  assert.deepEqual(s.calls.slice(before).map(x => x.start / 100).sort((a, b) => a - b), [9, 10, 11, 12]);
  s.release(); await settle();
  before = s.calls.length;
  c.ensureRange(500, 550); // page 5, moving backward
  await settle();
  assert.deepEqual(s.calls.slice(before).map(x => x.start / 100).sort((a, b) => a - b), [3, 4, 5, 6]);
});

test('pages that leave interest are aborted', async () => {
  const s = server(100_000, {delay: true});
  const c = createWindowedCollection({fetchPage: s.fetchPage, pageSize: 100, maxResidentPages: 10, keyOf: i => i.id});
  c.ensureRange(0, 10);
  s.release(); await settle();
  c.ensureRange(50_000, 50_050);
  await settle();
  const far = s.calls.filter(x => x.start >= 49_000);
  assert.ok(far.length > 0);
  c.ensureRange(90_000, 90_050);
  await settle();
  assert.ok(far.every(x => x.signal.aborted), 'abandoned pages were aborted');
  s.release(); await settle();
  assert.equal(c.itemAt(90_000)?.n, 90_000);
  assert.equal(c.itemAt(50_000), undefined);
});

test('per-page subscriptions fire only for their page', async () => {
  const s = server(10_000);
  const c = createWindowedCollection({fetchPage: s.fetchPage, pageSize: 100, maxResidentPages: 10, keyOf: i => i.id, prefetchPages: 0});
  let page0 = 0, page7 = 0;
  c.subscribePage(0, () => page0++);
  c.subscribePage(7, () => page7++);
  const v0 = c.getPageVersion(0);
  c.ensureRange(0, 10); await settle();
  assert.ok(page0 > 0);
  assert.equal(page7, 0);
  assert.notEqual(c.getPageVersion(0), v0);
  const v0Loaded = c.getPageVersion(0);
  c.ensureRange(700, 710); await settle();
  assert.ok(page7 > 0);
  assert.equal(c.getPageVersion(0), v0Loaded, 'page 0 is unchanged while page 7 loads');
});

test('invalidate keeps the old generation on screen until the new first page arrives, then swaps atomically', async () => {
  const s = server(10_000, {prefix: 'old'});
  const c = createWindowedCollection({fetchPage: s.fetchPage, pageSize: 100, maxResidentPages: 10, keyOf: i => i.id});
  c.ensureRange(200, 250); await settle();
  assert.equal(c.itemAt(200)?.id, 'old200');
  const next = server(500, {prefix: 'new', delay: true});
  let coarse = 0;
  c.subscribe(() => coarse++);
  c.invalidate(next.fetchPage);
  await settle();
  assert.equal(c.itemAt(200)?.id, 'old200', 'no flash to empty');
  assert.equal(c.getSnapshot().refreshing, true);
  assert.equal(c.getSnapshot().total, 10_000);
  assert.equal(next.calls[0]!.start, 200, 'the new generation starts where the viewer is');
  next.release(); await settle();
  assert.equal(c.itemAt(200)?.id, 'new200');
  assert.equal(c.getSnapshot().total, 500);
  assert.equal(c.getSnapshot().generation, 1);
  assert.equal(c.getSnapshot().refreshing, false);
  assert.ok(coarse > 0);
});

test('letter jumps use the position index', async () => {
  const s = server(26_000, {positionIndex: {A: 0, M: 12_000, Z: 25_000}});
  const c = createWindowedCollection({fetchPage: s.fetchPage, pageSize: 100, maxResidentPages: 10, keyOf: i => i.id});
  c.ensureRange(0, 20); await settle();
  const m = c.indexOfLetter('m');
  assert.equal(m, 12_000);
  c.ensureRange(m!, m! + 20); await settle();
  assert.equal(c.itemAt(12_000)?.n, 12_000);
  assert.equal(c.indexOfLetter('Q'), undefined);
});

test('a failed visible page reports an error, keeps other pages and retries', async () => {
  let failing = true;
  const s = server(1000, {fail: start => failing && start === 100});
  let clock = 0;
  const c = createWindowedCollection({fetchPage: s.fetchPage, pageSize: 100, maxResidentPages: 10, keyOf: i => i.id, prefetchPages: 0, now: () => clock});
  c.ensureRange(0, 10); await settle();
  c.ensureRange(100, 110); await settle();
  assert.equal(c.getSnapshot().status, 'error');
  assert.ok(c.getSnapshot().error instanceof Error);
  failing = false;
  c.ensureRange(100, 110); await settle();
  assert.equal(c.getSnapshot().status, 'error', 'not retried before retryAfterMs');
  clock = 6000;
  c.ensureRange(100, 110); await settle();
  assert.equal(c.getSnapshot().status, 'ready');
  assert.equal(c.itemAt(100)?.n, 100);
});

test('dispose aborts work in flight and stops notifying', async () => {
  const s = server(1000, {delay: true});
  const c = createWindowedCollection({fetchPage: s.fetchPage, pageSize: 100, maxResidentPages: 10, keyOf: i => i.id});
  let calls = 0;
  c.subscribe(() => calls++);
  c.ensureRange(0, 10);
  c.dispose();
  assert.ok(s.calls[0]!.signal.aborted);
  const before = calls;
  s.release(); await settle();
  assert.equal(calls, before);
});
