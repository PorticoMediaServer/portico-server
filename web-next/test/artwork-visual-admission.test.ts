import test from 'node:test';
import assert from 'node:assert/strict';
import {createPaintAdmission} from '../src/kit/artwork/visual-admission.ts';

// Pure source regression for the paint admission helper. UNRUN here: no runner,
// network or scheduler is started. Every wait is a controlled clock.
//
// The real mechanism: the consumer MOUNTS every visible slot before any of them
// decodes, then reports ready or retire on the same slot. A paced run therefore
// always has all of its siblings mounted before the first ready; a sibling mounted
// after a group has painted is a new group and legitimately admits at once.

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

function admission(keepCleared = false) {
  const time = clock(keepCleared);
  return {time, paint: createPaintAdmission({stepMs: 40, now: time.now, setTimer: time.setTimer, clearTimer: time.clearTimer})};
}

// The owning consumer supplies its own scheduler identity; these are two different
// consumers that happen to use the same group name.
const scopeA = {}, scopeB = {};

test('a mounted predecessor blocks its later sibling even when the sibling decodes first', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  const c = paint.mount({scope: scopeA, group: 'row', order: 3});
  const a = paint.mount({scope: scopeA, group: 'row', order: 1});
  const b = paint.mount({scope: scopeA, group: 'row', order: 2});
  c.ready(() => seen.push('c'));
  assert.deepEqual(seen, [], 'C decoded first but its mounted predecessors are still pending');
  assert.equal(time.pending(), 0, 'waiting for a mounted predecessor is not a timer fact');
  a.ready(() => seen.push('a'));
  assert.deepEqual(seen, ['a']);
  b.ready(() => seen.push('b'));
  assert.deepEqual(seen, ['a'], 'the next sibling waits one bounded step');
  time.advance(40);
  assert.deepEqual(seen, ['a', 'b']);
  time.advance(40);
  assert.deepEqual(seen, ['a', 'b', 'c']);
  assert.equal(paint.size(), 0);
});

test('a retired mounted predecessor is skipped immediately', () => {
  const {paint} = admission();
  const seen: string[] = [];
  const first = paint.mount({scope: scopeA, group: 'row', order: 1});
  const second = paint.mount({scope: scopeA, group: 'row', order: 2});
  first.retire();
  second.ready(() => seen.push('second'));
  assert.deepEqual(seen, ['second']);
  assert.equal(paint.size(), 0);
});

test('a slot with no real group or no owning scope paints independently', () => {
  const {paint} = admission();
  const seen: string[] = [];
  const rowHead = paint.mount({scope: scopeA, group: 'row', order: 1});
  const loose = paint.mount({order: 1});
  const unscoped = paint.mount({group: 'row', order: 1});
  loose.ready(() => seen.push('loose'));
  unscoped.ready(() => seen.push('unscoped'));
  assert.deepEqual(seen, ['loose', 'unscoped'], 'neither joins a queue');
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
  assert.deepEqual(seen, ['unordered', 'not-finite'], 'an unorderable slot paints on its own');
  rowHead.ready(() => seen.push('row'));
  assert.deepEqual(seen, ['unordered', 'not-finite', 'row']);
});

test('admissions in one group are paced by exactly one 40 ms step', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
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

test('two consumers with the same group name never share a queue or a pace', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  // Both consumers' paced pairs are mounted before either decodes.
  const aFirst = paint.mount({scope: scopeA, group: 'row', order: 1});
  const aSecond = paint.mount({scope: scopeA, group: 'row', order: 2});
  const bFirst = paint.mount({scope: scopeB, group: 'row', order: 1});
  const bSecond = paint.mount({scope: scopeB, group: 'row', order: 2});
  aFirst.ready(() => seen.push('a-first'));
  aSecond.ready(() => seen.push('a-second'));
  bFirst.ready(() => seen.push('b-first'));
  bSecond.ready(() => seen.push('b-second'));
  assert.deepEqual(seen, ['a-first', 'b-first'], 'each scope admits its own first immediately');
  assert.equal(time.pending(), 2, 'each scope holds its own bounded wait');
  time.advance(40);
  assert.deepEqual(seen, ['a-first', 'b-first', 'a-second', 'b-second']);
});

test('a stale queued timer callback cannot clear a replacement wait', () => {
  // A host that already delivered the callback cannot remove it, so the replacement
  // wait must be protected by the timer slot itself.
  const {time, paint} = admission(true);
  const seen: string[] = [];
  const a = paint.mount({scope: scopeA, group: 'row', order: 1});
  const b = paint.mount({scope: scopeA, group: 'row', order: 2});
  const c = paint.mount({scope: scopeA, group: 'row', order: 3});
  a.ready(() => seen.push('a'));
  b.ready(() => seen.push('b'));            // issues the paced wait T for B
  const stale = time.all()[0];
  assert.ok(stale, 'the paced wait was issued');
  // A real pending predecessor arrives and becomes the head: flush retires T.
  const predecessor = paint.mount({scope: scopeA, group: 'row', order: 1.5});
  predecessor.ready(() => seen.push('predecessor')); // issues the replacement wait
  stale.callback();                          // the old, already-queued callback fires late
  assert.deepEqual(seen, ['a'], 'the stale callback admits nothing');
  time.advance(40);
  assert.deepEqual(seen, ['a', 'predecessor'], 'the replacement wait still admits the new head');
  time.advance(40);
  assert.deepEqual(seen, ['a', 'predecessor', 'b']);
  time.advance(40);
  assert.deepEqual(seen, ['a', 'predecessor', 'b', 'c'], 'the original run completes in reading order');
});

test('a re-created slot cannot paint from a stale ready callback', () => {
  const {paint} = admission();
  const seen: string[] = [];
  const stale = paint.mount({scope: scopeA, group: 'row', order: 1});
  stale.retire();
  const fresh = paint.mount({scope: scopeA, group: 'row', order: 1});
  stale.ready(() => seen.push('stale'));
  assert.deepEqual(seen, [], 'a retired slot never paints in a successor slot');
  fresh.ready(() => seen.push('fresh'));
  assert.deepEqual(seen, ['fresh']);
});

test('a repeated ready never replaces the first paint closure', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  const first = paint.mount({scope: scopeA, group: 'row', order: 1});
  const slot = paint.mount({scope: scopeA, group: 'row', order: 2});
  first.ready(() => seen.push('first'));
  slot.ready(() => seen.push('second'));
  slot.ready(() => seen.push('second-again'));
  time.advance(40);
  assert.deepEqual(seen, ['first', 'second'], 'the issued closure is the only paint');
});

test('an emptied group is pruned and its own wait cleared', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  const first = paint.mount({scope: scopeA, group: 'row', order: 1});
  const second = paint.mount({scope: scopeA, group: 'row', order: 2});
  first.ready(() => seen.push('first'));
  second.ready(() => seen.push('second'));
  assert.equal(time.pending(), 1);
  second.retire();
  assert.equal(time.pending(), 0, 'the retired wait is cleared with the emptied group');
  assert.equal(paint.size(), 0);
  const later = paint.mount({scope: scopeA, group: 'row', order: 1});
  later.ready(() => seen.push('later'));
  assert.deepEqual(seen, ['first', 'later'], 'a fresh group admits its own first slot immediately');
});

test('dispose invalidates every mounted slot and clears every wait', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  const first = paint.mount({scope: scopeA, group: 'row', order: 1});
  const queued = paint.mount({scope: scopeA, group: 'row', order: 2});
  const independent = paint.mount({order: 1});
  first.ready(() => seen.push('first'));
  queued.ready(() => seen.push('second'));
  assert.equal(time.pending(), 1);
  paint.dispose();
  queued.ready(() => seen.push('late'));
  independent.ready(() => seen.push('late-independent'));
  time.advance(200);
  assert.deepEqual(seen, ['first'], 'nothing paints after dispose');
  assert.equal(time.pending(), 0);
  assert.equal(paint.size(), 0);
});
