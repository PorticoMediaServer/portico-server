import test from 'node:test';
import assert from 'node:assert/strict';
import {retainLayout} from '../src/kit/layout/retention.ts';
import {resolveRemovalFocus} from '../src/kit/layout/focus-context.ts';

// UNRUN source regression against the public History composition 4af76987.
// Helper coverage only: the real retention helper and the real removal-focus
// resolver. Nothing here is a second History state machine, a clock, a portal, a
// product hook or a render/inert/pointer/overlay integration.
//
// A retention call only keeps carrying exits when the previous result and this
// call share the exact same scope object; a different object resets the paint.

type Row = Readonly<{id: string; label: string}>;
const row = (id: string, label = id): Row => ({id, label});
const keyOf = (item: Row) => item.id;
const scope = Object.freeze({surface: 'history'});
const base = (live: readonly Row[], exits: ReadonlyMap<string, unknown> = new Map()) =>
  ({live, present: live, exits, scope} as never);

test('retention keeps every live row even when more than forty are outgoing', () => {
  const live = Array.from({length: 60}, (_, index) => row('live-' + index));
  const next = retainLayout(base(live), live, keyOf, 0, false, 180, 160,
    {maxRetained: 40, staggerMs: 40, scope});
  assert.equal(next.live.length, 60, 'live rows are never capped');
});

test('one shared bound admits at most forty actually visible outgoing rows', () => {
  const before = Array.from({length: 50}, (_, index) => row('r' + index));
  const visible = new Set(before.map(item => item.id));
  const first = retainLayout(base(before), [], keyOf, 0, false, 180, 160,
    {maxRetained: 40, staggerMs: 40, visibleOnly: true, scope}, visible);
  assert.equal(first.exits.size, 40, 'no more than forty visible rows leave at once');
  const noneVisible = retainLayout(base(before), [], keyOf, 0, false, 180, 160,
    {maxRetained: 40, staggerMs: 40, visibleOnly: true, scope}, new Set<string>());
  assert.equal(noneVisible.exits.size, 0, 'only actually visible rows are admitted');
});

test('an admitted exit keeps its original delay and deadline before its expiry, and the bound stays shared', () => {
  const firstBatch = Array.from({length: 30}, (_, index) => row('a' + index));
  const secondBatch = Array.from({length: 20}, (_, index) => row('b' + index));
  const afterFirst = retainLayout(base([...firstBatch, ...secondBatch]), secondBatch, keyOf, 1000, false, 180, 160,
    {maxRetained: 40, staggerMs: 40, scope});
  assert.equal(afterFirst.exits.size, 30);
  const admitted = afterFirst.exits.get('a0')!;
  assert.equal(admitted.admittedAt, 1000);
  assert.equal(admitted.delay, 0);
  assert.equal(admitted.deadline, 1180);
  // A second cohort arrives while the first cohort is still legitimately alive.
  const afterSecond = retainLayout(afterFirst, [], keyOf, 1100, false, 180, 160,
    {maxRetained: 40, staggerMs: 40, scope});
  assert.deepEqual(afterSecond.exits.get('a0'), admitted, 'a later update cannot extend or rewrite the admitted exit');
  assert.equal(afterSecond.exits.size, 40, 'overlapping cohorts share the same forty-row bound');
});

test('reduction only shortens the effective retirement', () => {
  const first = retainLayout(base([row('a')]), [], keyOf, 1000, false, 180, 160,
    {maxRetained: 40, staggerMs: 40, scope});
  const reduced = retainLayout(first, [], keyOf, 1000, true, 180, 160,
    {maxRetained: 40, staggerMs: 40, scope});
  assert.equal(reduced.exits.get('a')!.retireAt, 1160, 'reduced retention never exceeds the original cohort');
  assert.equal(reduced.exits.get('a')!.reduced, true);
});

test('a cohort keeps its layout slot until the final stagger finishes', () => {
  const rows = [row('a'), row('b')];
  const next = retainLayout(base(rows), [], keyOf, 0, false, 180, 160,
    {maxRetained: 40, staggerMs: 40, scope});
  assert.equal(next.exits.get('b')!.delay, 40);
  assert.equal(next.exits.get('b')!.cohortDeadline, 220, '180 plus one 40 ms stagger');
  assert.deepEqual(next.present.map(item => item.id), ['a', 'b'], 'faded predecessors keep their slots');
  // After the first row's own deadline but before the cohort finishes, its slot is still present.
  const kept = retainLayout(next, [], keyOf, 200, false, 180, 160,
    {maxRetained: 40, staggerMs: 40, scope});
  assert.deepEqual(kept.present.map(item => item.id), ['a', 'b']);
});

test('a new source scope drops the old paint instead of inheriting it', () => {
  const first = retainLayout(base([row('a')]), [], keyOf, 0, false, 180, 160,
    {maxRetained: 40, staggerMs: 40, scope});
  assert.equal(first.exits.size, 1);
  const replaced = retainLayout(first, [row('b')], keyOf, 10, false, 180, 160,
    {maxRetained: 40, staggerMs: 40, scope: Object.freeze({surface: 'history', generation: 2})});
  assert.equal(replaced.exits.size, 0, 'a replaced source drops the previous row paint');
});

// Minimal element fixture: exactly the DOM facts the resolver reads.
const primary = '[data-layout-primary],button:not(:disabled),a[href],[tabindex="0"]';
type Node = ReturnType<typeof node>;
function node(id: string, options: {rects?: number; visibility?: string; disabled?: boolean; tabindex?: boolean; connected?: boolean} = {}) {
  const value = {
    id, focused: false, isConnected: options.connected ?? true,
    visibility: options.visibility ?? 'visible',
    matches: (selector: string) => selector === primary && options.disabled !== true ? true : selector === primary ? false : false,
    hasAttribute: (name: string) => name === 'tabindex' && options.tabindex !== false,
    querySelectorAll: () => [] as unknown[],
    closest: () => null,
    getClientRects: () => Array(options.rects ?? 1).fill({}),
    getAttribute: (name: string) => name === 'aria-disabled' && options.disabled ? 'true' : null,
    contains: (candidate: {isConnected?: boolean}) => candidate.isConnected !== false,
    focus() {value.focused = true;},
  };
  return value;
}

const root = node('root');
const originalStyle = (globalThis as unknown as {getComputedStyle?: unknown}).getComputedStyle;
test.after(() => {
  if (originalStyle === undefined) delete (globalThis as unknown as {getComputedStyle?: unknown}).getComputedStyle;
  else (globalThis as unknown as {getComputedStyle: unknown}).getComputedStyle = originalStyle;
});

function context(order: Node[], surviving: Node[], sectionStarts: Node[] = []) {
  (globalThis as unknown as {getComputedStyle: (value: {visibility?: string}) => {visibility: string}}).getComputedStyle =
    value => ({visibility: value.visibility ?? 'visible'});
  return {root, order, surviving, sectionStarts} as never;
}

test('removal focus goes to the next surviving row, then the previous one', () => {
  const [a, b, c] = [node('a'), node('b'), node('c')];
  assert.equal(resolveRemovalFocus(context([a, b, c], [a, c]), b as never).element, c, 'next surviving row');
  assert.equal(resolveRemovalFocus(context([a, b, c], [a, b]), c as never).element, b, 'previous when the last row goes');
});

test('an outgoing row is never a legal target, and a same-key successor is not confused with it', () => {
  const [a, b] = [node('a'), node('b')];
  const successor = node('b');
  const target = resolveRemovalFocus(context([a, b, successor], [a, successor]), b as never);
  assert.equal(target.element, successor, 'the captured identity decides, not the key');
  assert.notEqual(target.element, b);
});

test('a surviving section start is used before any region fallback, and an empty context returns none', () => {
  const a = node('a');
  const section = node('section');
  assert.equal(resolveRemovalFocus(context([a], [], [section]), a as never).kind, 'section');
  assert.equal(resolveRemovalFocus(context([a], [], []), a as never).kind, 'region');
  assert.equal(resolveRemovalFocus({root, order: [], surviving: [], sectionStarts: []} as never, node('detached') as never).kind, 'none');
});

test('a non-eligible survivor is skipped instead of stealing focus', () => {
  const a = node('a');
  const hidden = node('hidden', {connected: false});
  assert.equal(resolveRemovalFocus(context([a, hidden], [hidden]), a as never).kind, 'region');
});

test('a hidden-but-connected survivor is skipped by its own visibility fact', () => {
  const a = node('a');
  const invisible = node('invisible', {visibility: 'hidden'});
  assert.equal(resolveRemovalFocus(context([a, invisible], [invisible]), a as never).kind, 'region');
});
