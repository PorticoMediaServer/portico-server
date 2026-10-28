import test from 'node:test';
import assert from 'node:assert/strict';
import {shellResyncRecheck} from '../src/shell/bootstrap.ts';

function deferred<T>() { let resolve!: (value: T) => void, reject!: (error: unknown) => void; const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; }); return {promise, resolve, reject}; }

function harness(key = 'same') {
  const calls = {refresh: 0, read: 0, moved: 0};
  const reads: ReturnType<typeof deferred<string>>[] = [];
  let current = true;
  const recheck = shellResyncRecheck({key, isCurrent: () => current,
    refresh: () => { calls.refresh++; }, moved: () => { calls.moved++; },
    read: () => { calls.read++; const next = deferred<string>(); reads.push(next); return next.promise; }});
  return {calls, reads, recheck, retire: () => { current = false; }};
}

test('a resync whose facts did not move refreshes in place and never restarts the shell', async () => {
  const {calls, reads, recheck} = harness();
  const done = recheck();
  reads[0]!.resolve('same');
  await done;
  assert.deepEqual(calls, {refresh: 1, read: 1, moved: 0});
});

test('a resync that finds the libraries or server facts moved restarts once, as the missed hint would have', async () => {
  const {calls, reads, recheck} = harness();
  const done = recheck();
  reads[0]!.resolve('moved');
  await done;
  assert.deepEqual(calls, {refresh: 1, read: 1, moved: 1});
});

test('resyncs during a read share it and one follow-up read, not one read each', async () => {
  const {calls, reads, recheck} = harness();
  const first = recheck();
  await recheck(); await recheck(); await recheck();
  assert.equal(calls.read, 1);
  reads[0]!.resolve('same');
  await new Promise(resolve => setTimeout(resolve, 0));
  assert.equal(calls.read, 2, 'one follow-up covers every resync that arrived meanwhile');
  reads[1]!.resolve('same');
  await first;
  assert.deepEqual(calls, {refresh: 2, read: 2, moved: 0});
});

test('a failed read keeps the shell, and the next resync reads again', async () => {
  const {calls, reads, recheck} = harness();
  const failed = recheck();
  reads[0]!.reject(new Error('offline'));
  await failed;
  assert.equal(calls.moved, 0);
  const retried = recheck();
  reads[1]!.resolve('moved');
  await retried;
  assert.deepEqual(calls, {refresh: 2, read: 2, moved: 1});
});

test('a shell retired during the read is not restarted by it', async () => {
  const {calls, reads, recheck, retire} = harness();
  const done = recheck();
  retire();
  reads[0]!.resolve('moved');
  await done;
  assert.equal(calls.moved, 0);
});
