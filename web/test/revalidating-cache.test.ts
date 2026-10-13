import test from 'node:test';
import assert from 'node:assert/strict';
import {RevalidatingCache} from '../src/app/revalidating-cache.ts';

const listener = (seen: unknown[], errors: unknown[] = []) => ({value: (v: unknown) => { seen.push(v); }, error: (e: unknown) => { errors.push(e); }});
const tick = () => new Promise(resolve => setTimeout(resolve, 0));

test('what was read last is shown at once and checked again on the next use; a changed answer replaces it', async () => {
  const cache = new RevalidatingCache<{sorts: string[]}>();
  let answer = {sorts: ['title']};
  let reads = 0;
  const load = async () => { reads++; return answer; };
  const first: unknown[] = [];
  const stop = cache.use('ana', 'movies:movies', load, listener(first));
  await tick();
  stop();
  assert.deepEqual(first, [{sorts: ['title']}]);

  answer = {sorts: ['title', 'rating']};
  const second: unknown[] = [];
  cache.use('ana', 'movies:movies', load, listener(second));
  assert.deepEqual(second, [{sorts: ['title']}], 'the last answer shows without waiting');
  await tick();
  assert.deepEqual(second, [{sorts: ['title']}, {sorts: ['title', 'rating']}], 'and the server’s new answer replaces it');
  assert.equal(reads, 2);
});

test('an unchanged answer is not delivered twice, and a failed check keeps what is shown', async () => {
  const cache = new RevalidatingCache<{n: number}>();
  let fail = false;
  const load = async () => { if (fail) throw new Error('busy'); return {n: 1}; };
  const seen: unknown[] = [], errors: unknown[] = [];
  cache.use('ana', 'k', load, listener(seen, errors));
  await tick();
  cache.use('ana', 'k', load, listener(seen, errors));
  await tick();
  assert.deepEqual(seen, [{n: 1}, {n: 1}], 'once for each listener from memory, never again for the same answer');
  fail = true;
  const later: unknown[] = [], laterErrors: unknown[] = [];
  cache.use('ana', 'k', load, listener(later, laterErrors));
  await tick();
  assert.deepEqual(later, [{n: 1}]);
  assert.deepEqual(laterErrors, []);
});

test('a first read that fails reports it and is tried again next time', async () => {
  const cache = new RevalidatingCache<number>();
  let fail = true;
  const load = async () => { if (fail) throw new Error('offline'); return 7; };
  const seen: unknown[] = [], errors: unknown[] = [];
  cache.use('ana', 'k', load, listener(seen, errors));
  await tick();
  assert.equal(errors.length, 1);
  fail = false;
  cache.use('ana', 'k', load, listener(seen, errors));
  await tick();
  assert.deepEqual(seen, [7]);
});

test('another sign-in starts empty, and the cache is bounded', async () => {
  const cache = new RevalidatingCache<string>({max: 2});
  const seen: unknown[] = [];
  cache.use('ana', 'a', async () => 'ana-a', listener(seen))();
  await tick();
  const other: unknown[] = [];
  const stop = cache.use('ben', 'a', async () => 'ben-a', listener(other));
  assert.deepEqual(other, [], 'nothing of the previous viewer is shown');
  await tick();
  stop();
  assert.deepEqual(other, ['ben-a']);
  cache.use('ben', 'b', async () => 'b', listener([]))();
  cache.use('ben', 'c', async () => 'c', listener([]))();
  assert.equal(cache.size, 2);
});

test('a late answer for a previous sign-in is dropped', async () => {
  const cache = new RevalidatingCache<string>();
  let release!: (v: string) => void;
  const seen: unknown[] = [];
  cache.use('ana', 'a', () => new Promise<string>(resolve => { release = resolve; }), listener(seen));
  cache.use('ben', 'a', async () => 'ben', listener([]));
  release('ana');
  await tick();
  assert.deepEqual(seen, []);
});
