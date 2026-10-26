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

function clock(keepCleared = false) {
  let current = 0;
  let timers: Timer[] = [];
  return {
    now: () => current,
    setTimer: (callback: () => void, waitMs: number) => {timers.push({callback, at: current + waitMs}); return timers[timers.length - 1];},
    clearTimer: (timer: unknown) => {if (!keepCleared) timers = timers.filter(entry => entry !== timer);},
    all: () => timers.slice(),
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

// The owning consumer supplies its own scheduler identity; these are two different
// consumers that happen to use the same group name.
const scopeA = {}, scopeB = {};

test('a mounted predecessor blocks its later sibling even when the sibling decodes first', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  // The row mounts C, A, B (visibility order) before any of them decodes.
  const c = paint.mount({scope: scopeA, group: 'row', order: 3});
  const a = paint.mount({scope: scopeA, group: 'row', order: 1});
  const b = paint.mount({scope: scopeA, group: 'row', order: 2});
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
  const first = paint.mount({scope: scopeA, group: 'row', order: 1});
  const second = paint.mount({scope: scopeA, group: 'row', order: 2});
  first.retire();                       // failed, offscreen, replaced or unmounted
  second.ready(() => seen.push('second'));
  assert.deepEqual(seen, ['second'], 'the next ready slot paints without waiting for a retired one');
  assert.equal(paint.size(), 0);
});

test('a slot with no real group paints independently and never joins a row', () => {
  const {paint} = admission();
  const seen: string[] = [];
  const rowHead = paint.mount({scope: scopeA, group: 'row', order: 1});
  const loose = paint.mount({order: 1});     // no group metadata: no invented global group
  const unscoped = paint.mount({group: 'row', order: 1});  // grouped, but no owning scope
  loose.ready(() => seen.push('loose'));
  unscoped.ready(() => seen.push('unscoped'));
  assert.deepEqual(seen, ['loose', 'unscoped'], 'a group without the owning scope never joins a queue');
  rowHead.ready(() => seen.push('row'));
  assert.deepEqual(seen, ['loose', 'unscoped', 'row']);
  assert.equal(paint.size(), 0);
});

test('a missing or non-finite order never fabricates a reading position', () => {
  const {paint} = admission();
  const seen: string[] = [];
  const rowHead = paint.mount({scope: scopeA, group: 'row', order: 1});
  const unordered = paint.mount({scope: scopeA, group: 'row'});
  const notFinite = paint.mount({scope: scopeA, group: 'row', order: Number.NaN});
  unordered.ready(() => seen.push('unordered'));
  notFinite.ready(() => seen.push('not-finite'));
  assert.deepEqual(seen, ['unordered', 'not-finite'], 'an unorderable slot paints on its own, not at a made-up position');
  rowHead.ready(() => seen.push('row'));
  assert.deepEqual(seen, ['unordered', 'not-finite', 'row']);
});

test('a stale queued timer callback cannot clear or reschedule a replacement wait', () => {
  // The host already delivered the first callback; clearTimer therefore cannot
  // remove it. The callback must still refuse the replaced timer slot.
  const time = clock(true);
  const paint = createPaintAdmission({stepMs: 40, now: time.now, setTimer: time.setTimer, clearTimer: time.clearTimer});
  const seen: string[] = [];
  paint.mount({scope: scopeA, group: 'row', order: 1}).ready(() => seen.push('first'));
  const second = paint.mount({scope: scopeA, group: 'row', order: 2});
  second.ready(() => seen.push('second'));
  const stale = time.all()[0];
  assert.ok(stale, 'the paced wait was issued');
  const third = paint.mount({scope: scopeA, group: 'row', order: 3});
  third.ready(() => seen.push('third'));
  second.retire();                       // its wait is retired and a replacement wait is issued
  stale.callback();                      // the old, already-queued callback fires late
  assert.deepEqual(seen, ['first'], 'the stale callback admits nothing on its own');
  time.advance(40);
  assert.deepEqual(seen, ['first', 'third'], 'the replacement wait still admits the next sibling exactly once');
});

test('two consumers with the same group name never share a queue or a pace', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  // Both consumers' visible heads are mounted before either decodes.
  const aFirst = paint.mount({scope: scopeA, group: 'row', order: 1});
  const bFirst = paint.mount({scope: scopeB, group: 'row', order: 1});
  aFirst.ready(() => seen.push('a-first'));
  // A second scheduler's same-named row is a different run: it starts its own pace.
  bFirst.ready(() => seen.push('b-first'));
  assert.deepEqual(seen, ['a-first', 'b-first'], 'the scopes are isolated, not merged into one FIFO');
  paint.mount({scope: scopeB, group: 'row', order: 2}).ready(() => seen.push('b-second'));
  assert.deepEqual(seen, ['a-first', 'b-first'], 'scope B has its own pacing step');
  time.advance(40);
  assert.deepEqual(seen, ['a-first', 'b-first', 'b-second']);
});

test('admissions in one group are paced by exactly one 40 ms step', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  // Both visible slots are mounted before either decodes, as the consumer does.
  const first = paint.mount({scope: scopeA, group: 'row', order: 1});
  const second = paint.mount({scope: scopeA, group: 'row', order: 2});
  first.ready(() => seen.push('first'));
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
  const stale = paint.mount({scope: scopeA, group: 'row', order: 1});
  stale.retire();                        // same-key image replaced, or the slot unmounted
  const fresh = paint.mount({scope: scopeA, group: 'row', order: 1});
  stale.ready(() => seen.push('stale'));
  assert.deepEqual(seen, [], 'a retired slot never paints in a successor slot');
  fresh.ready(() => seen.push('fresh'));
  assert.deepEqual(seen, ['fresh']);
});

test('a slot reports at most one paint even if ready is called twice', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  paint.mount({scope: scopeA, group: 'row', order: 1}).ready(() => seen.push('first'));
  const slot = paint.mount({scope: scopeA, group: 'row', order: 2});
  slot.ready(() => seen.push('second'));
  slot.ready(() => seen.push('second-again'));   // ignored: the first ready closure is the issued one
  time.advance(40);
  assert.deepEqual(seen, ['first', 'second'], 'a repeated ready never replaces the first paint closure');
});

test('an emptied group is pruned and a later slot starts a fresh group', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  paint.mount({scope: scopeA, group: 'row', order: 1}).ready(() => seen.push('first'));
  const second = paint.mount({scope: scopeA, group: 'row', order: 2});
  second.ready(() => seen.push('second'));
  assert.equal(time.pending(), 1);
  second.retire();                       // the whole row empties before its paced admission
  assert.equal(time.pending(), 0, 'the retired wait is cleared');
  assert.equal(paint.size(), 0);
  const later = paint.mount({scope: scopeA, group: 'row', order: 1});
  later.ready(() => seen.push('later'));
  assert.deepEqual(seen, ['first', 'later'], 'a fresh group admits its own first slot immediately');
  assert.equal(time.pending(), 0);
});

test('dispose invalidates every mounted slot and clears every wait', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  paint.mount({scope: scopeA, group: 'row', order: 1}).ready(() => seen.push('first'));
  const queued = paint.mount({scope: scopeA, group: 'row', order: 2});
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
