import test from 'node:test';
import assert from 'node:assert/strict';
import {onServerChanged, serverChanged} from '../src/app/server-changes.ts';

test('subscribers hear every change; leaving stops the calls', () => {
  const seen: string[] = [];
  const offA = onServerChanged(() => seen.push('a'));
  const offB = onServerChanged(() => seen.push('b'));
  serverChanged();
  assert.deepEqual(seen, ['a', 'b']);
  offA();
  serverChanged();
  assert.deepEqual(seen, ['a', 'b', 'b']);
  offB();
  serverChanged();
  assert.deepEqual(seen, ['a', 'b', 'b']);
});

test('a listener that leaves from inside the emit does not break the round', () => {
  const seen: string[] = [];
  let off: () => void = () => {};
  off = onServerChanged(() => {
    seen.push('once');
    off();
  });
  onServerChanged(() => seen.push('always'));
  serverChanged();
  serverChanged();
  assert.deepEqual(seen, ['once', 'always', 'always']);
});
