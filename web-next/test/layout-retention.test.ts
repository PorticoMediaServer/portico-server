import assert from 'node:assert/strict';
import test from 'node:test';
import {retainLayout, type LayoutRetention} from '../src/kit/layout/retention.ts';
const keyOf = (item: {id: string}) => item.id;
const rows = (...ids: string[]) => ids.map(id => Object.freeze({id}));
const initial = (live = rows()): LayoutRetention<{id: string}> => ({live, present: live, exits: new Map()});
const policy = {maxRetained: 40, staggerMs: 40};
// Source vectors only, UNRUN. These exercise the consumed retention model;
// actual focus and DOM paint remain separate owner/worker qualification.
test('only visible original rows acquire outgoing slots and every live row remains', () => {
  const before = initial(rows('a', 'b', 'c')), incoming = rows(...Array.from({length: 100}, (_, i) => `new-${i}`));
  const next = retainLayout(before, incoming, keyOf, 0, false, 180, 160, {...policy, visibleOnly: true}, new Set(['b']));
  assert.deepEqual([...next.exits.keys()], ['b']);assert.equal(next.present.length, 101);assert.equal(next.exits.get('b')!.item, before.live[1]);
  assert.ok(incoming.every(item => next.present.includes(item)));
});
test('expired paint cannot be admitted again by a timer or unrelated update', () => {
  const before = initial(rows('a')), first = retainLayout(before, [], keyOf, 0, false, 180, 160, policy);
  const expired = retainLayout(first, [], keyOf, 180, false, 180, 160, policy);
  assert.equal(expired.exits.size, 0);assert.equal(retainLayout(expired, [], keyOf, 1000, false, 180, 160, policy).present.length, 0);
});
test('a returned key withdraws its old exit and a later real removal gets a new token', () => {
  const before = initial(rows('a')), first = retainLayout(before, [], keyOf, 0, false, 180, 160, policy), current = rows('a');
  const returned = retainLayout(first, current, keyOf, 20, false, 180, 160, policy);
  assert.equal(returned.exits.size, 0);assert.equal(returned.present[0], current[0]);
  const again = retainLayout(returned, [], keyOf, 30, false, 180, 160, policy);
  assert.notEqual(again.exits.get('a')!.token, first.exits.get('a')!.token);assert.equal(again.exits.get('a')!.item, current[0]);
});
test('non-finite retention limits refuse extra paint rather than silently making it unbounded', () => {
  const next = retainLayout(initial(rows('a')), [], keyOf, 0, false, 180, 160, {maxRetained: NaN});assert.equal(next.present.length, 0);
});
test('overlapping batches never replace an older retained source with a successor value', () => {
  const before = initial(rows('a', 'b')), first = retainLayout(before, rows('new'), keyOf, 0, false, 180, 160, policy);
  const original = first.exits.get('b')!, next = retainLayout(first, rows('later'), keyOf, 20, false, 180, 160, policy);
  assert.equal(next.exits.get('b'), original);assert.equal(original.item, before.live[1]);assert.equal(original.delay, 40);
});
