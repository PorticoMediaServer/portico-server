import test from 'node:test';
import assert from 'node:assert/strict';
import {applyRemovalFocus, captureRemovalFocus, removalFocusTarget} from '../src/kit/layout/removal-focus.ts';

/** Ordered, connected fake nodes with the exact DOM surface the helper reads. */
class FocusNode {
  parentElement: FocusNode | null = null;
  readonly children: FocusNode[] = [];
  readonly attributes = new Map<string, string>();
  isConnected = true;
  hidden = false;
  inert = false;
  disabled = false;
  hasBox = true;
  focusCalls = 0;
  constructor(readonly name: string) {}
  append(...nodes: FocusNode[]) {
    for (const node of nodes) { node.parentElement = this; node.connect(this.isConnected); this.children.push(node); }
    return this;
  }
  connect(connected: boolean) {
    this.isConnected = connected;
    for (const child of this.children) child.connect(connected);
  }
  contains(node: FocusNode | null): boolean {
    for (let current = node; current; current = current.parentElement) if (current === this) return true;
    return false;
  }
  detach() {
    const parent = this.parentElement;
    if (parent) {
      const index = parent.children.indexOf(this);
      if (index >= 0) parent.children.splice(index, 1);
    }
    this.parentElement = null;
    this.connect(false);
  }
  getAttribute(name: string) { return this.attributes.get(name) ?? null; }
  hasAttribute(name: string) { return this.attributes.has(name); }
  getClientRects() { return this.hasBox ? [{} as DOMRect] : []; }
  focus() { this.focusCalls += 1; }
  compareDocumentPosition(other: FocusNode) {
    const top = (node: FocusNode) => { let current = node; while (current.parentElement) current = current.parentElement; return current; };
    if (top(this) !== top(other)) return 1;
    const ordered: FocusNode[] = [];
    const visit = (node: FocusNode) => { ordered.push(node); node.children.forEach(visit); };
    visit(top(this));
    const mine = ordered.indexOf(this), theirs = ordered.indexOf(other);
    if (mine === theirs) return 0;
    return theirs < mine ? 2 : 4;
  }
}

const html = (node: FocusNode) => node as unknown as HTMLElement;

function fixture() {
  const root = new FocusNode('root'), toolbar = new FocusNode('toolbar'), sectionA = new FocusNode('sectionA'), sectionB = new FocusNode('sectionB');
  root.append(toolbar, sectionA, sectionB);
  const before = new FocusNode('before'), sectionStart = new FocusNode('sectionStart');
  toolbar.append(before, sectionStart);
  const a1 = new FocusNode('a1'), a2 = new FocusNode('a2'), a3 = new FocusNode('a3');
  sectionA.append(a1, a2, a3);
  const after = new FocusNode('after'), b1 = new FocusNode('b1');
  sectionB.append(after, b1);
  return {root, toolbar, sectionA, sectionB, before, sectionStart, after, a1, a2, a3, b1};
}

function capture(f: ReturnType<typeof fixture>, focused: FocusNode, items = [f.a1, f.a2, f.a3, f.b1]) {
  return captureRemovalFocus({root: html(f.root), items: items.map(html), focused: html(focused),
    section: html(f.sectionStart), starts: [f.before, f.sectionStart, f.after].map(html)});
}

function expectFocus(result: ReturnType<typeof applyRemovalFocus>, node: FocusNode, source: string) {
  assert.equal(result.action, 'focus');
  if (result.action !== 'focus') return;
  assert.equal(result.node, html(node));
  assert.equal(result.source, source);
}

test('next eligible item wins before previous, section and start fallbacks', () => {
  const f = fixture(), context = capture(f, f.a1)!;
  f.a1.detach();
  expectFocus(applyRemovalFocus(context, [html(f.a1)]), f.a2, 'next');
  assert.equal(f.a2.focusCalls, 1);
  assert.equal(f.sectionStart.focusCalls + f.before.focusCalls + f.after.focusCalls, 0);
});

test('previous is used only after no next item is eligible', () => {
  const f = fixture(), context = capture(f, f.a2)!;
  f.a2.detach();
  f.a3.inert = true;
  expectFocus(applyRemovalFocus(context, [html(f.a2)]), f.a1, 'previous');
});

for (const [label, block] of [
  ['inert', (node: FocusNode) => { node.inert = true; }],
  ['hidden', (node: FocusNode) => { node.hidden = true; }],
  ['aria-hidden', (node: FocusNode) => { node.attributes.set('aria-hidden', 'true'); }],
  ['disabled', (node: FocusNode) => { node.disabled = true; }],
] as const) {
  test(`skips a ${label} next item and uses previous`, () => {
    const f = fixture(), context = capture(f, f.a2)!;
    f.a2.detach();
    block(f.a3);
    expectFocus(applyRemovalFocus(context, [html(f.a2)]), f.a1, 'previous');
  });
}

test('a removed neighbour is not a focus candidate', () => {
  const f = fixture(), context = capture(f, f.a1)!;
  f.a1.detach();
  f.a2.detach();
  expectFocus(applyRemovalFocus(context, [html(f.a1), html(f.a2)]), f.a3, 'next');
});

test('the focused section start follows next and previous', () => {
  const f = fixture(), context = capture(f, f.a1, [f.a1])!;
  f.a1.detach();
  expectFocus(applyRemovalFocus(context, [html(f.a1)]), f.sectionStart, 'section');
});

test('nearest surviving start is preceding first, then following', () => {
  const f = fixture(), context = capture(f, f.a1, [f.a1])!;
  f.a1.detach();
  f.sectionStart.inert = true;
  expectFocus(applyRemovalFocus(context, [html(f.a1)]), f.before, 'start');

  const g = fixture(), second = capture(g, g.a1, [g.a1])!;
  g.a1.detach();
  g.sectionStart.inert = true;
  g.before.hidden = true;
  expectFocus(applyRemovalFocus(second, [html(g.a1)]), g.after, 'start');
});

test('unrelated removal cancels the move without stealing focus', () => {
  const f = fixture(), context = capture(f, f.a1)!;
  f.a2.detach();
  assert.deepEqual(applyRemovalFocus(context, [html(f.a2)]), {action: 'ignore'});
  assert.equal(f.a1.focusCalls + f.a2.focusCalls, 0);
});

test('a focused control outside the ordered items yields no captured context', () => {
  const f = fixture();
  const context = captureRemovalFocus({root: html(f.root), items: [html(f.a1), html(f.a2)], focused: html(f.before),
    section: html(f.sectionStart), starts: [html(f.before)]});
  assert.equal(context, null);
  assert.deepEqual(applyRemovalFocus(context, [html(f.a1)]), {action: 'none'});
});

test('a detached original root is refused before and during apply', () => {
  const f = fixture(), context = capture(f, f.a1)!;
  f.root.detach();
  assert.deepEqual(applyRemovalFocus(context, [html(f.a1)]), {action: 'none'});
  assert.equal(f.a1.focusCalls + f.a2.focusCalls, 0);
  assert.equal(captureRemovalFocus({root: html(f.root), items: [html(f.a1)], focused: html(f.a1)}), null);
});

test('a new element reusing the removed item key is never chosen', () => {
  const f = fixture();
  const context = captureRemovalFocus({root: html(f.root), items: [html(f.a1)], focused: html(f.a1)})!;
  f.a1.detach();
  const replacement = new FocusNode('replacement');
  f.sectionA.append(replacement);
  assert.deepEqual(removalFocusTarget(context, [html(f.a1)]), {action: 'none'});
  assert.deepEqual(applyRemovalFocus(context, [html(f.a1)]), {action: 'none'});
  assert.equal(replacement.focusCalls, 0);
});

test('a focused target moved to another root is not followed', () => {
  const f = fixture(), context = capture(f, f.a1)!;
  f.a1.detach();
  const other = new FocusNode('other');
  other.append(f.a1);
  assert.deepEqual(applyRemovalFocus(context, [html(f.a1)]), {action: 'ignore'});
  assert.equal(f.a2.focusCalls, 0);
});

test('capture follows the real active element when no focused target is supplied', () => {
  const f = fixture();
  Object.assign(f.root, {ownerDocument: {activeElement: html(f.a2)}});
  const context = captureRemovalFocus({root: html(f.root), items: [html(f.a1), html(f.a2)]});
  assert.equal(context?.focused, html(f.a2));
});
