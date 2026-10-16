import test from 'node:test';
import assert from 'node:assert/strict';
import {manualMoves, movedOrder, moveRequest, moveTarget} from '../src/presentation/manual-order.ts';

test('a move sends the entry and the one it follows, never the whole order', () => {
  const ids = ['a', 'b', 'c', 'd'];
  assert.deepEqual(moveRequest(ids, 2, 0), {entryId: 'c', afterEntryId: ''}, 'to the front');
  assert.deepEqual(moveRequest(ids, 0, 1), {entryId: 'a', afterEntryId: 'b'}, 'down one');
  assert.deepEqual(moveRequest(ids, 3, 1), {entryId: 'd', afterEntryId: 'a'}, 'a drag up');
  assert.deepEqual(moveRequest(ids, 1, 3), {entryId: 'b', afterEntryId: 'd'}, 'to the end');
  assert.equal(moveRequest(ids, 1, 1), undefined);
  assert.equal(moveRequest(ids, 9, 0), undefined);
  for (const [from, to] of [[2, 0], [0, 1], [3, 1], [1, 3], [0, 3]] as const) {
    const req = moveRequest(ids, from, to)!;
    const after = movedOrder(ids, from, to);
    assert.equal(after.indexOf(req.entryId), req.afterEntryId === '' ? 0 : after.indexOf(req.afterEntryId) + 1, `${from}→${to}: the server's anchor gives the order shown`);
  }
});

test('the moves offered for an entry; to the bottom only with the whole list loaded', () => {
  assert.deepEqual(manualMoves(0, 3, true), ['down', 'bottom']);
  assert.deepEqual(manualMoves(1, 3, true), ['top', 'up', 'down', 'bottom']);
  assert.deepEqual(manualMoves(1, 3, false), ['top', 'up', 'down']);
  assert.deepEqual(manualMoves(2, 3, true), ['top', 'up']);
  assert.deepEqual(manualMoves(0, 1, true), []);
  assert.equal(moveTarget('up', 0, 3), 0);
  assert.equal(moveTarget('bottom', 0, 3), 2);
});
