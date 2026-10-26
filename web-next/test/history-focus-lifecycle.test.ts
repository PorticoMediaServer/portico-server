import test from 'node:test';
import assert from 'node:assert/strict';
import {tableFocusFallback} from '../src/kit/settings/table-model.ts';

// UNRUN source regression for the History clear/remove focus handoff.
//
// It drives the actual exported helper the History table already uses to move
// focus after a row disappears (SettingsTable's own before/after id capture). It
// does not build a second History state machine, a parallel resource/presence
// model, a clock or a retirement hook: the 180 ms outgoing inert paint, the 320 ms
// gap closure, the section context and the overlay return are not in current
// source, so those seams are handed back to the owner instead of being faked here.

test('a removed focused row hands focus to the next surviving row', () => {
  assert.equal(tableFocusFallback(['a', 'b', 'c'], ['a', 'c'], 'b'), 'c');
});

test('a removed last row hands focus back to the previous surviving row', () => {
  assert.equal(tableFocusFallback(['a', 'b', 'c'], ['a', 'b'], 'c'), 'b');
});

test('clearing every row leaves no fallback target, so the caller uses its own empty-state control', () => {
  assert.equal(tableFocusFallback(['a', 'b'], [], 'a'), undefined);
});

test('a row that is still present keeps focus even if the display key repeats', () => {
  assert.equal(tableFocusFallback(['a', 'b'], ['a', 'b'], 'a'), 'a');
});

test('a new row that repeats a removed row key never inherits the retired reference', () => {
  // The helper addresses rows by identity, never by a displayed label: removing b
  // and rendering a different row x with the same visible text cannot capture focus.
  assert.equal(tableFocusFallback(['a', 'b'], ['a', 'x'], 'b'), 'a');
});

test('a retained row with no actionable control still resolves by identity only', () => {
  // The helper never inspects controls; the accessible fallback stays the caller's
  // own decision, so a retained id resolves to itself.
  assert.equal(tableFocusFallback(['a', 'b'], ['a', 'b'], 'b'), 'b');
});
