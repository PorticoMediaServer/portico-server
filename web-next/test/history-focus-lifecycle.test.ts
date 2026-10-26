import test from 'node:test';
import assert from 'node:assert/strict';
import {retainLayout} from '../src/kit/layout/retention.ts';
import {resolveRemovalFocus} from '../src/kit/layout/focus-context.ts';

// UNRUN source regression against the public History composition 4af76987: the
// real retention helper and the real removal-focus resolver. Nothing here is a
// second History state machine, a clock or a product hook; the fake nodes only
// supply the DOM facts the resolver itself reads.

type Row = Readonly<{id: string; label: string}>;
const row = (id: string, label = id): Row => ({id, label});
const keyOf = (item: Row) => item.id;

test('retention keeps every live row even when more than forty are outgoing', () => {
  const live = Array.from({length: 60}, (_, index) => row('live-' + index));
  const next = retainLayout({live, present: live, exits: new Map()}, live, keyOf, 0, false, 180, 160,
    {maxRetained: 40, staggerMs: 40, scope: {}});
  assert.equal(next.live.length, 60, 'live rows are never capped');
});

test('one shared bound admits at most forty actually visible outgoing rows', () => {
  const before = Array.from({length: 50}, (_, index) => row('r' + index));
  const visible = new Set(Array.from({length: 50}, (_, index) => 'r' + index));
  const first = retainLayout({live: before, present: before, exits: new Map()}, [], keyOf, 0, false, 180, 160,
    {maxRetained: 40, staggerMs: 40, visibleOnly: true, scope: {}}, visible);
  assert.equal(first.exits.size, 40, 'no more than forty visible rows leave at once');
  const hidden = new Set<string>();
  const noneVisible = retainLayout({live: before, present: before, exits: new Map()}, [], keyOf, 0, false, 180, 160,
    {maxRetained: 40, staggerMs: 40, visibleOnly: true, scope: {}}, hidden);
  assert.equal(noneVisible.exits.size, 0, 'only actually visible rows are admitted');
});

test('an admitted exit keeps its original delay and deadline across later cohorts', () => {
  const a = row('a');
  const first = retainLayout({live: [a], present: [a], exits: new Map()}, [], keyOf, 1000, false, 180, 160,
    {maxRetained: 40, staggerMs: 40, scope: {}});
  const exit = first.exits.get('a')!;
  assert.equal(exit.admittedAt, 1000);
  assert.equal(exit.delay, 0);
  assert.equal(exit.deadline, 1180);
  const second = retainLayout(first, [], keyOf, 1400, false, 180, 160, {maxRetained: 40, staggerMs: 40, scope: {}});
  assert.deepEqual(second.exits.get('a'), exit, 'a later update cannot extend the admitted deadline');
});

test('reduction only shortens the effective retirement', () => {
  const a = row('a');
  const first = retainLayout({live: [a], present: [a], exits: new Map()}, [], keyOf, 1000, false, 180, 160,
    {maxRetained: 40, staggerMs: 40, scope: {}});
  const reduced = retainLayout(first, [], keyOf, 1000, true, 180, 160, {maxRetained: 40, staggerMs: 40, scope: {}});
  assert.equal(reduced.exits.get('a')!.retireAt, 1160, 'reduced retention never exceeds the original cohort');
  assert.equal(reduced.exits.get('a')!.reduced, true);
});

test('a cohort keeps its layout slot until the final stagger finishes', () => {
  const rows = [row('a'), row('b')];
  const next = retainLayout({live: rows, present: rows, exits: new Map()}, [], keyOf, 0, false, 180, 160,
    {maxRetained: 40, staggerMs: 40, scope: {}});
  assert.equal(next.exits.get('b')!.delay, 40);
  assert.equal(next.exits.get('b')!.cohortDeadline, 260);
  assert.deepEqual(next.present.map(item => item.id), ['a', 'b'], 'faded predecessors keep their slots');
});

test('a new source scope drops the old paint instead of inheriting it', () => {
  const a = row('a');
  const first = retainLayout({live: [a], present: [a], exits: new Map()}, [], keyOf, 0, false, 180, 160,
    {maxRetained: 40, staggerMs: 40, scope: {generation: 1}});
  assert.equal(first.exits.size, 1);
  const replaced = retainLayout(first, [row('b')], keyOf, 10, false, 180, 160,
    {maxRetained: 40, staggerMs: 40, scope: {generation: 2}});
  assert.equal(replaced.exits.size, 0, 'a replaced source drops the previous row paint');
});

// Minimal node fixture: only the facts resolveRemovalFocus reads.
const primary = '[data-layout-primary],button:not(:disabled),a[href],[tabindex="0"]';
function node(id: string, options: {rects?: number; visibility?: string; disabled?: boolean; tabindex?: string | null} = {}) {
  const value = {id, focused: false, isConnected: true,
    matches: (selector: string) => selector === primary,
    hasAttribute: (name: string) => name === 'tabindex' && options.tabindex !== null,
    querySelectorAll: () => [],
    closest: () => null,
    getClientRects: () => Array(options.rects ?? 1).fill({}),
    getAttribute: (name: string) => name === 'aria-disabled' && options.disabled ? 'true' : null,
    focus() {value.focused = true;},
  };
  return value;
}

(globalThis as unknown as {getComputedStyle: (node: unknown) => {visibility: string}}).getComputedStyle =
  (value: unknown) => ({visibility: (value as {visibility?: string}).visibility ?? 'visible'});

const root = node('root');
function context(order: ReturnType<typeof node>[], surviving: ReturnType<typeof node>[], sectionStarts: ReturnType<typeof node>[] = []) {
  return {root, order, surviving, sectionStarts};
}

test('removal focus goes to the next surviving row, then the previous one', () => {
  const [a, b, c] = [node('a'), node('b'), node('c')];
  assert.equal(resolveRemovalFocus(context([a, b, c], [a, c]), b).element, c, 'next surviving row');
  assert.equal(resolveRemovalFocus(context([a, b, c], [a, b]), c).element, b, 'previous when the last row goes');
});

test('an outgoing row is never a legal target, and a same-key successor is not confused with it', () => {
  const [a, b] = [node('a'), node('b')];
  const successor = node('b');
  const target = resolveRemovalFocus(context([a, b, successor], [a, successor]), b);
  assert.equal(target.element, successor, 'the captured identity decides, not the key');
  assert.notEqual(target.element, b);
});

test('a surviving section start is used before any region fallback, and an empty context returns none', () => {
  const [a] = [node('a')];
  const section = node('section');
  assert.equal(resolveRemovalFocus(context([a], [], [section]), a).kind, 'section');
  const detached = node('detached');
  assert.equal(resolveRemovalFocus(context([a], [], []), a).kind, 'region');
  assert.equal(resolveRemovalFocus({root, order: [], surviving: [], sectionStarts: []}, detached).kind, 'none');
});

test('a non-eligible surviving row is skipped instead of stealing focus', () => {
  const a = node('a');
  const hidden = node('hidden');
  hidden.isConnected = false;
  assert.equal(resolveRemovalFocus(context([a, hidden], [hidden]), a).kind, 'region');
});
