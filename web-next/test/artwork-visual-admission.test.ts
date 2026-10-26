import test from 'node:test';
import assert from 'node:assert/strict';
import {createPaintAdmission} from '../src/kit/artwork/visual-admission.ts';

// Pure source regression for the paint admission helper. UNRUN here: no test
// runner, network or scheduler is started. Every wait is a controlled clock.
//
// The real mechanism: the owner MOUNTS a visible slot before its image decodes,
// then reports ready or retire on that same slot. Mount order is visibility order,
// never decode order.

type Timer = {callback: () => void; at: number};

function clock() {
  let current = 0;
  let timers: Timer[] = [];
  return {
    now: () => current,
    setTimer: (callback: () => void, waitMs: number) => {timers.push({callback, at: current + waitMs}); return timers[timers.length - 1];},
    clearTimer: (timer: unknown) => {timers = timers.filter(entry => entry !== timer);},
    advance(milliseconds: number) {
      current += milliseconds;
      for (;;) {
        const due = timers.filter(entry => entry.at <= current).sort((left, right) => left.at - right.at)[0];
        if (!due) return;
        timers = timers.filter(entry => entry !== due);
        due.callback();
      }
    },
    pending: () => timers.length,
  };
}

function admission() {
  const time = clock();
  return {time, paint: createPaintAdmission({stepMs: 40, now: time.now, setTimer: time.setTimer, clearTimer: time.clearTimer})};
}

test('a mounted predecessor blocks its later sibling even when the sibling decodes first', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  // The row mounts C, A, B (visibility order) before any of them decodes.
  const c = paint.mount({group: 'row', order: 3});
  const a = paint.mount({group: 'row', order: 1});
  const b = paint.mount({group: 'row', order: 2});
  c.ready(() => seen.push('c'));
  assert.deepEqual(seen, [], 'C decoded first but its mounted predecessors are still pending');
  assert.equal(time.pending(), 0, 'waiting for a mounted predecessor is not a timer fact');
  a.ready(() => seen.push('a'));
  assert.deepEqual(seen, ['a'], 'the earliest mounted slot paints as soon as it is ready');
  b.ready(() => seen.push('b'));
  assert.deepEqual(seen, ['a'], 'the next sibling waits one bounded step');
  time.advance(40);
  assert.deepEqual(seen, ['a', 'b']);
  time.advance(40);
  assert.deepEqual(seen, ['a', 'b', 'c']);
  assert.equal(time.pending(), 0, 'the group is pruned with its last admission');
  assert.equal(paint.size(), 0);
});

test('a retired mounted predecessor is skipped immediately, never inferred', () => {
  const {paint} = admission();
  const seen: string[] = [];
  const first = paint.mount({group: 'row', order: 1});
  const second = paint.mount({group: 'row', order: 2});
  first.retire();                       // failed, offscreen, replaced or unmounted
  second.ready(() => seen.push('second'));
  assert.deepEqual(seen, ['second'], 'the next ready slot paints without waiting for a retired one');
  assert.equal(paint.size(), 0);
});

test('a slot with no real group paints independently and never joins a row', () => {
  const {paint} = admission();
  const seen: string[] = [];
  const rowHead = paint.mount({group: 'row', order: 1});
  const loose = paint.mount({order: 1});     // no group metadata: no invented global group
  loose.ready(() => seen.push('loose'));
  assert.deepEqual(seen, ['loose'], 'an ungrouped slot paints on its own');
  rowHead.ready(() => seen.push('row'));
  assert.deepEqual(seen, ['loose', 'row']);
  assert.equal(paint.size(), 0);
});

test('admissions in one group are paced by exactly one 40 ms step', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  paint.mount({group: 'row', order: 1}).ready(() => seen.push('first'));
  const second = paint.mount({group: 'row', order: 2});
  second.ready(() => seen.push('second'));
  assert.deepEqual(seen, ['first']);
  assert.equal(time.pending(), 1, 'the paced sibling holds one bounded wait');
  time.advance(39);
  assert.deepEqual(seen, ['first'], 'not before the step');
  time.advance(1);
  assert.deepEqual(seen, ['first', 'second']);
});

test('a re-created slot cannot paint from a stale ready callback', () => {
  const {paint} = admission();
  const seen: string[] = [];
  const stale = paint.mount({group: 'row', order: 1});
  stale.retire();                        // same-key image replaced, or the slot unmounted
  const fresh = paint.mount({group: 'row', order: 1});
  stale.ready(() => seen.push('stale'));
  assert.deepEqual(seen, [], 'a retired slot never paints in a successor slot');
  fresh.ready(() => seen.push('fresh'));
  assert.deepEqual(seen, ['fresh']);
});

test('a slot reports at most one paint even if ready is called twice', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  paint.mount({group: 'row', order: 1}).ready(() => seen.push('first'));
  const slot = paint.mount({group: 'row', order: 2});
  slot.ready(() => seen.push('second'));
  slot.ready(() => seen.push('second-again'));
  time.advance(40);
  assert.deepEqual(seen, ['first', 'second']);
});

test('an emptied group is pruned and a later slot starts a fresh group', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  paint.mount({group: 'row', order: 1}).ready(() => seen.push('first'));
  const second = paint.mount({group: 'row', order: 2});
  second.ready(() => seen.push('second'));
  assert.equal(time.pending(), 1);
  second.retire();                       // the whole row empties before its paced admission
  assert.equal(time.pending(), 0, 'the retired wait is cleared');
  assert.equal(paint.size(), 0);
  const later = paint.mount({group: 'row', order: 1});
  later.ready(() => seen.push('later'));
  assert.deepEqual(seen, ['first', 'later'], 'a recreated group is not paced by the old timer');
  assert.equal(time.pending(), 0);
});

test('dispose invalidates every mounted slot and clears every wait', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  paint.mount({group: 'row', order: 1}).ready(() => seen.push('first'));
  const queued = paint.mount({group: 'row', order: 2});
  queued.ready(() => seen.push('second'));
  const independent = paint.mount({order: 1});
  assert.equal(time.pending(), 1);
  paint.dispose();
  queued.ready(() => seen.push('late'));
  independent.ready(() => seen.push('late-independent'));
  time.advance(200);
  assert.deepEqual(seen, ['first'], 'nothing paints after dispose');
  assert.equal(time.pending(), 0);
  assert.equal(paint.size(), 0);
});
