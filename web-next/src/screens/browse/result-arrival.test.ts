import test from 'node:test';
import assert from 'node:assert/strict';
import {resultArrival, resultWitness} from './result-arrival.ts';

const settled = (generation: number, total: number | undefined, refreshing = false, status = 'ready') =>
  resultWitness({generation, total, refreshing, status});

test('the first accepted result paints immediately', () => {
  assert.equal(resultArrival(undefined, settled(1, 80), false), undefined);
});

test('a pending replacement starts no crossfade', () => {
  assert.equal(resultArrival(settled(1, 80), settled(1, 80, true), false), undefined);
  assert.equal(resultArrival(settled(1, 80, true), settled(1, 80, true), false), undefined);
});

test('a landed answer cross-fades over the authored 180', () => {
  assert.deepEqual(resultArrival(settled(1, 80), settled(2, 16), false), {durationMs: 180, easing: 'ease'});
});

test('a new accepted generation with the same total still arrives', () => {
  assert.deepEqual(resultArrival(settled(1, 16), settled(2, 16), false), {durationMs: 180, easing: 'ease'});
});

test('app or system reduction uses 160 linear opacity-only', () => {
  assert.deepEqual(resultArrival(settled(1, 80), settled(2, 16), true), {durationMs: 160, easing: 'linear'});
});

test('a quiet update that keeps generation and total does not restart', () => {
  assert.equal(resultArrival(settled(2, 16), settled(2, 16), false), undefined);
});

test('rapid A to B to C keeps one bounded arrival per landed answer', () => {
  const a = settled(1, 80), b = settled(2, 16), c = settled(3, 0);
  assert.deepEqual(resultArrival(a, b, false), {durationMs: 180, easing: 'ease'});
  assert.deepEqual(resultArrival(b, c, false), {durationMs: 180, easing: 'ease'});
  assert.equal(resultArrival(c, c, false), undefined);
});
