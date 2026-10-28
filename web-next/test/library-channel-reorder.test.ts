import {strict as assert} from 'node:assert';
import {test} from 'node:test';
import type {LibraryChannel} from '../../packages/client-core/src/library-channels.ts';
import {moveLibraryChannelTo} from '../src/screens/settings/server/channels/reorder-library-channels.ts';
const rows = () => ['a', 'b', 'c', 'd'].map(id => ({config: {id}, revision: 1}) as LibraryChannel);

test('drag reorder uses the newly read revision for every adjacent move', async () => {
  let list = rows();const revisions: number[] = [];
  const result = await moveLibraryChannelTo(list[0]!, 3, {current: () => true, rows: () => list, move: async (row, direction) => {
    revisions.push(row.revision);const at = list.indexOf(row);assert.notEqual(at, -1);assert.equal(direction, 1);
    const next = [...list];[next[at], next[at + direction]] = [next[at + direction]!, next[at]!];
    list = next.map(value => ({...value, revision: value.revision + 1}));return true;
  }});
  assert.equal(result, true);assert.deepEqual(list.map(row => row.config.id), ['b', 'c', 'd', 'a']);assert.deepEqual(revisions, [1, 2, 3]);
});

test('a concurrent order change stops a multi-position drag', async () => {
  let list = rows(), calls = 0;
  assert.equal(await moveLibraryChannelTo(list[0]!, 3, {current: () => true, rows: () => list, move: async () => {calls++;list = [list[1]!, list[0]!, list[3]!, list[2]!];return true;}}), false);
  assert.equal(calls, 1);
});

test('lost receipts and retired authority never trigger the next move', async () => {
  const list = rows();let calls = 0, current = true;
  const ports = {current: () => current, rows: () => list, move: async () => {calls++;return false;}};
  assert.equal(await moveLibraryChannelTo(list[0]!, 3, ports), false);assert.equal(calls, 1);
  current = false;assert.equal(await moveLibraryChannelTo(list[0]!, 3, ports), false);assert.equal(calls, 1);
});

test('invalid destinations and stale row identities cannot reorder', async () => {
  const list = rows();let calls = 0;
  const ports = {current: () => true, rows: () => list, move: async () => {calls++;return true;}};
  for (const index of [-1, 4, 1.5, NaN]) assert.equal(await moveLibraryChannelTo(list[0]!, index, ports), false);
  assert.equal(await moveLibraryChannelTo({...list[0]!}, 1, ports), false);
  assert.equal(await moveLibraryChannelTo(list[0]!, 0, ports), true);assert.equal(calls, 0);
});
