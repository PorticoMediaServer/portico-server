import test from 'node:test';
import assert from 'node:assert/strict';
import {compressedPosition} from '../src/ui/compressed-scroll.ts';

// A 10 M-title grid: 600 M px of rows in an 8 M px box, a 1,000 px window.
const maxScrolled = 8_000_000 - 1000;
const maxVirtual = 600_000_000 - 1000;
const jump = 3000;

/** Scrolls the box from `start` to `end` in `step` px moves, as a wheel would. */
function wheel(start: {top: number; scrolled: number}, end: number, step: number) {
  let {top, scrolled} = start;
  const seen: number[] = [];
  while (scrolled !== end) {
    const next = step > 0 ? Math.min(end, scrolled + step) : Math.max(end, scrolled + step);
    const moved = compressedPosition({from: top, scrolled: next, previousScrolled: scrolled, maxScrolled, maxVirtual, jump});
    seen.push(moved.position);
    top = moved.top;
    scrolled = next;
  }
  return {top, scrolled, seen};
}

test('a wheel moves a compressed list 1:1 from where it is', () => {
  const at = compressedPosition({from: 0, scrolled: 4_000_000, previousScrolled: 0, maxScrolled, maxVirtual, jump});
  assert.equal(at.top, 4_000_000 * (maxVirtual / maxScrolled), 'a scrollbar drag maps proportionally');
  const step = compressedPosition({from: at.top, scrolled: 4_000_100, previousScrolled: 4_000_000, maxScrolled, maxVirtual, jump});
  assert.equal(step.top - at.top, 100);
});

test('wheeling to either end of a compressed list arrives without a jump', () => {
  // From close to the end, wheel down to the very end, then back up to the top.
  const near = compressedPosition({from: 0, scrolled: maxScrolled - 20_000, previousScrolled: 0, maxScrolled, maxVirtual, jump});
  const down = wheel({top: near.top, scrolled: maxScrolled - 20_000}, maxScrolled, 120);
  assert.equal(down.top, maxVirtual, 'the bottom of the box shows the last row');
  const largestDown = Math.max(...down.seen.map((p, i) => (i ? p - down.seen[i - 1] : 0)));
  assert.ok(largestDown <= 120 * 2 * (maxVirtual / maxScrolled) + 1, `no step jumps (largest ${largestDown})`);

  const up = wheel(down, maxScrolled - 30_000, -120);
  const back = compressedPosition({from: up.top, scrolled: 30_000, previousScrolled: up.scrolled, maxScrolled, maxVirtual, jump});
  const top = wheel({top: back.top, scrolled: 30_000}, 0, -120);
  assert.equal(top.top, 0, 'the top of the box shows the first row');
  for (let i = 1; i < top.seen.length; i++) assert.ok(top.seen[i - 1] - top.seen[i] <= 120 * 2 * (maxVirtual / maxScrolled) + 1);
  assert.ok(top.seen.every((p, i) => i === 0 || p <= top.seen[i - 1]), 'moving up never moves the list down');
});

test('past either end the box itself moves', () => {
  assert.equal(compressedPosition({from: 0, scrolled: -40, previousScrolled: 0, maxScrolled, maxVirtual, jump}).position, -40);
  assert.equal(compressedPosition({from: maxVirtual, scrolled: maxScrolled + 30, previousScrolled: maxScrolled, maxScrolled, maxVirtual, jump}).position, maxVirtual + 30);
});
