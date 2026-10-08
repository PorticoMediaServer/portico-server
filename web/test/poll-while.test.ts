import test from 'node:test';
import assert from 'node:assert/strict';
import {pollWhile} from '../src/admin/poll-while.ts';

function fakeEnv() {
  const timers: {fn: () => void; ms: number}[] = [];
  let hidden = false; const listeners = new Set<() => void>();
  return {
    timers, setHidden(h: boolean) { hidden = h; for (const fn of [...listeners]) fn(); },
    fire() { const t = timers.shift(); t?.fn(); return t?.ms; },
    env: {setTimer: (fn: () => void, ms: number) => { const t = {fn, ms}; timers.push(t); return t; }, clearTimer: (t: unknown) => { const i = timers.indexOf(t as never); if (i >= 0) timers.splice(i, 1); }, hidden: () => hidden, onVisibility: (fn: () => void) => { listeners.add(fn); return () => listeners.delete(fn); }},
  };
}

test('polls with 2, 4, 8, 16, 30, 30 s backoff while active, and stops when it settles', () => {
  const f = fakeEnv(); let active = true, reloads = 0;
  pollWhile(() => active, () => { reloads++; }, f.env);
  const delays = [f.fire(), f.fire(), f.fire(), f.fire(), f.fire(), f.fire()];
  assert.deepEqual(delays, [2000, 4000, 8000, 16000, 30000, 30000]);
  assert.equal(reloads, 6);
  active = false;
  f.fire();
  assert.equal(reloads, 6);
  assert.equal(f.timers.length, 0);
});

test('pauses while the page is hidden and checks straight away when it is shown', () => {
  const f = fakeEnv(); let reloads = 0;
  const stop = pollWhile(() => true, () => { reloads++; }, f.env);
  f.setHidden(true);
  assert.equal(f.timers.length, 0);
  f.setHidden(false);
  assert.equal(reloads, 1);
  assert.equal(f.timers[0]?.ms, 2000);
  stop();
  assert.equal(f.timers.length, 0);
});
