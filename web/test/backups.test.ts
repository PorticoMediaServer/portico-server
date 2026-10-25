import test from 'node:test';
import assert from 'node:assert/strict';
import {RESTORE_REFUSAL_CODES, isRestoreRefusalCode} from '../../packages/client-core/src/administration.ts';
import {enUS, type MessageId} from '../../packages/i18n/src/index.ts';
import {componentModule} from './helpers/component-harness.mjs';

const load = () => componentModule(new URL('../src/screens/server/Backups.tsx', import.meta.url), {
  react: {default: {}, useEffect: () => {}, useMemo: (f: () => unknown) => f(), useRef: (v: unknown) => ({current: v}), useState: (v: unknown) => [v, () => {}]},
  '@core/administration.ts': {}, '@core/backups.ts': {}, '../../admin/console': {}, './LibrarySettings': {}, './operation-ids': {}, '../../app/session': {}, '../../app/i18n': {}, '../../ui': {},
}) as Promise<any>;

test('every restore refusal code has catalogue copy and nothing else maps', async () => {
  const {RESTORE_REFUSAL_MESSAGE_IDS} = await load();
  const expected = ['insufficient_disk', 'restore_integrity_failed', 'restore_lock_unavailable', 'restore_not_portico', 'restore_pre_release', 'restore_source_invalid'];
  assert.deepEqual([...RESTORE_REFUSAL_CODES].sort(), expected);
  assert.deepEqual(Object.keys(RESTORE_REFUSAL_MESSAGE_IDS).sort(), expected);
  for (const code of expected) assert.equal(isRestoreRefusalCode(code), true, code);
  for (const code of ['restore_schema_newer', 'request_failed', 'internal', '']) {
    assert.equal(isRestoreRefusalCode(code), false, code);
    assert.equal(Object.hasOwn(RESTORE_REFUSAL_MESSAGE_IDS, code), false, code);
  }
  // All baseline/schema mismatches use this current refusal; the retired
  // schema-newer code must not return through a stale presentation map.
  assert.equal(RESTORE_REFUSAL_MESSAGE_IDS.restore_pre_release, 'web.maintenance.restorePreRelease');
  for (const id of Object.values(RESTORE_REFUSAL_MESSAGE_IDS)) {
    assert.match(id as string, /^web\.maintenance\./);
    assert.equal(typeof enUS[id as MessageId], 'string', id as string);
  }
});

test('the three backup kinds have catalogue copy', async () => {
  const {backupKindMessageId} = await load();
  assert.equal(backupKindMessageId('scheduled'), 'web.maintenance.backupKindScheduled');
  assert.equal(backupKindMessageId('manual'), 'web.maintenance.backupKindManual');
  assert.equal(backupKindMessageId('pre-restore'), 'web.maintenance.backupKindPreRestore');
});

test('only .db files are offered as bare-database restore sources', async () => {
  const {isBackupSourceFile} = await load();
  assert.equal(isBackupSourceFile('/media/portico.db'), true);
  assert.equal(isBackupSourceFile('/media/PORTICO.DB'), true);
  for (const other of ['/media/backups/2026-09-24T013000Z', '/media/portico.db-journal', '/media/portico.sqlite', '/media/notes.db.txt', '']) assert.equal(isBackupSourceFile(other), false);
});
