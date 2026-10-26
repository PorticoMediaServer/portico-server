import test from 'node:test';
import assert from 'node:assert/strict';
import {createPaintAdmission} from '../src/kit/artwork/visual-admission.ts';

// Pure source regression: the paint admission helper only decides when an already
// decoded image may show its decoded paint. No I/O, no scheduler, no network and no
// live timer: a controlled clock fires every wait.

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

test('siblings reveal in their real reading order, not their decode order', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  // Decodes finish c, a, b while the row's order is a=1, b=2, c=3.
  for (const [order, label] of [[3, 'c'], [1, 'a'], [2, 'b']] as const) {
    paint.request({group: 'row', order, paint: () => seen.push(label)});
  }
  assert.deepEqual(seen, ['a'], 'the lowest ordered ready sibling paints at once');
  time.advance(40);
  assert.deepEqual(seen, ['a', 'b'], 'the next sibling waits one bounded step');
  time.advance(40);
  assert.deepEqual(seen, ['a', 'b', 'c']);
  assert.equal(time.pending(), 0, 'no wait survives the last admission');
  assert.equal(paint.size(), 0);
});

test('the first admission is immediate and leaves no timer behind', () => {
  const {time, paint} = admission();
  let painted = 0;
  paint.request({group: 'row', order: 1, paint: () => {painted++;}});
  assert.equal(painted, 1);
  assert.equal(time.pending(), 0, 'a cached caller paints immediately and this module holds no wait');
});

test('a retired refusal never strands the ready siblings behind it', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  paint.request({group: 'row', order: 1, paint: () => seen.push('first')});
  const second = paint.request({group: 'row', order: 2, paint: () => seen.push('second')});
  paint.request({group: 'row', order: 3, paint: () => seen.push('third')});
  paint.retire(second);                 // the second image failed or left the viewport
  assert.deepEqual(seen, ['first']);
  time.advance(40);
  assert.deepEqual(seen, ['first', 'third'], 'the next ready sibling paints on the bounded step');
  assert.equal(paint.size(), 0);
});

test('a retired token never paints even when its entry is still queued', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  paint.request({group: 'row', order: 1, paint: () => seen.push('first')});
  const late = paint.request({group: 'row', order: 2, paint: () => seen.push('retired')});
  paint.retire(late);
  time.advance(40);
  assert.deepEqual(seen, ['first'], 'a retired image never paints in a successor slot');
  assert.equal(time.pending(), 0, 'retiring the only queued entry clears its wait');
});

test('entries with the same order keep their registration order', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  paint.request({group: 'row', order: 1, paint: () => seen.push('first')});
  paint.request({group: 'row', order: 2, paint: () => seen.push('second-a')});
  paint.request({group: 'row', order: 2, paint: () => seen.push('second-b')});
  time.advance(40);
  time.advance(40);
  assert.deepEqual(seen, ['first', 'second-a', 'second-b']);
});

test('separate groups pace separately and never block each other', () => {
  const {paint} = admission();
  const seen: string[] = [];
  paint.request({group: 'row-a', order: 1, paint: () => seen.push('a1')});
  paint.request({group: 'row-b', order: 1, paint: () => seen.push('b1')});
  assert.deepEqual(seen, ['a1', 'b1'], 'each run admits its own first sibling immediately');
});

test('dispose clears every wait and admits nothing afterwards', () => {
  const {time, paint} = admission();
  const seen: string[] = [];
  paint.request({group: 'row', order: 1, paint: () => seen.push('first')});
  paint.request({group: 'row', order: 2, paint: () => seen.push('second')});
  assert.equal(time.pending(), 1);
  paint.dispose();
  time.advance(200);
  assert.deepEqual(seen, ['first']);
  assert.equal(time.pending(), 0);
  assert.equal(paint.request({group: 'row', order: 3, paint: () => seen.push('after')}), 0);
  assert.deepEqual(seen, ['first']);
});

test('a single-row lifecycle retires every entry on unmount', () => {
  const {time, paint} = admission();
  const tokens = [1, 2, 3].map(order => paint.request({group: 'row', order, paint: () => {}}));
  assert.equal(paint.size(), 2, 'one admitted entry leaves the two queued');
  for (const token of tokens) paint.retire(token);
  assert.equal(paint.size(), 0);
  assert.equal(time.pending(), 0);
});
