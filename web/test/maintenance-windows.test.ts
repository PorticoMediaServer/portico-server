import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

const load = () => componentModule(new URL('../src/screens/server/MaintenanceWindows.tsx', import.meta.url), {
  react: {default: {}, useEffect: () => {}, useMemo: (f: () => unknown) => f(), useState: (v: unknown) => [v, () => {}]},
  '@core/administration.ts': {}, '../../admin/console': {}, '../../app/session': {}, '../../app/i18n': {}, '../../ui': {}, './operation-ids': {createOperationIds: () => ({forPayload: (k) => k, release: () => {}})},
}) as Promise<any>;

test('CD-11: broadcasts carry a nonempty dedupe key stable across retries of one send', async () => {
  const {broadcastBody} = await load();
  const first = broadcastBody('op-one', {audience: 'profile', severity: 'info', title: '  Down Saturday  ', body: '  Details  '});
  assert.ok(first.dedupeKey.length >= 1, 'the server requires a nonempty identifier');
  assert.equal(first.dedupeKey, 'broadcast:op-one');
  assert.equal(first.operationId, 'op-one');
  assert.equal(first.title, 'Down Saturday');
  assert.equal(first.body, 'Details');
  // A retry of the same send reuses the ID, so the key — and the server record — is identical.
  assert.equal(broadcastBody('op-one', {audience: 'profile', severity: 'info', title: 'Down Saturday', body: 'Other words'}).dedupeKey, first.dedupeKey);
  // Two distinct broadcasts never share a key.
  assert.notEqual(broadcastBody('op-two', {audience: 'profile', severity: 'info', title: 'Down Saturday', body: 'Details'}).dedupeKey, first.dedupeKey);
  assert.throws(() => broadcastBody('', {audience: 'profile', severity: 'info', title: 'T', body: ''}), 'no send without an operation ID');
});

test('Background priority defaults to lower and the save never pauses work', async () => {
  const {backgroundPriorityOrDefault, maintenanceSaveSettings} = await load();
  assert.equal(backgroundPriorityOrDefault(undefined), 'lower');
  assert.equal(backgroundPriorityOrDefault(''), 'lower');
  assert.equal(backgroundPriorityOrDefault('paused'), 'lower');
  assert.equal(backgroundPriorityOrDefault('normal'), 'normal');
  assert.equal(backgroundPriorityOrDefault('lower'), 'lower');
  const saved = maintenanceSaveSettings({windows: [], retention: {}, backupKeepCount: 7, backgroundTaskPriority: 'normal'});
  assert.equal(saved.backgroundTaskPriority, 'normal');
  assert.equal('pauseHeavyWorkOutsideWindows' in saved, false, 'the save never carries a pause field: background work is de-prioritized, never paused');
  const missing = maintenanceSaveSettings({windows: [], retention: {}, backupKeepCount: 7} as never);
  assert.equal(missing.backgroundTaskPriority, 'lower');
  assert.equal('pauseHeavyWorkOutsideWindows' in missing, false);
});
test('CD-46: broadcast titles and bodies validate UTF-8 bytes, trimmed as sent', async () => {
  const {broadcastProblem, utf8Length} = await load();
  assert.equal(utf8Length('hello'), 5);
  assert.equal(utf8Length('日'), 3);
  assert.equal(utf8Length('😀'), 4);
  assert.equal(broadcastProblem('Down Saturday', 'Details'), '');
  // CJK and emoji at and over each byte boundary.
  assert.equal(broadcastProblem('日'.repeat(66) + 'ab', ''), '', '66 CJK + 2 ASCII is exactly 200 bytes');
  assert.equal(broadcastProblem('日'.repeat(66) + 'abc', ''), 'title-too-long', '201 bytes refuses');
  assert.equal(broadcastProblem('😀'.repeat(50), ''), '', '50 emoji is exactly 200 bytes');
  assert.equal(broadcastProblem('😀'.repeat(50) + 'x', ''), 'title-too-long');
  assert.equal(broadcastProblem('ok', '日'.repeat(333) + 'x'), '', '333 CJK + x is exactly 1000 bytes');
  assert.equal(broadcastProblem('ok', '日'.repeat(333) + 'xy'), 'body-too-long');
  assert.equal(broadcastProblem('ok', '😀'.repeat(250)), '', '250 emoji is exactly 1000 bytes');
  assert.equal(broadcastProblem('ok', '😀'.repeat(251)), 'body-too-long');
  // Padding trims away before measuring.
  assert.equal(broadcastProblem('  ok  ', '  '), '');
});
