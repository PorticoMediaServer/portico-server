import test from 'node:test';
import assert from 'node:assert/strict';
import {parseBackupsDocument, parseRestoreStaged, type RestoreRecovery} from '../src/administration.ts';
import {restoreBackup, restoreRecoveryNotice, type BackupTransport} from '../src/backups.ts';

const staged = (recovery: unknown) => ({staged: true, restartRequired: true, validation: {schemaVersion: 41, integrity: 'recovered', recovery}});

test('recovery notices distinguish an index repair, full salvage and either kind of loss', () => {
  const cases: readonly [RestoreRecovery | undefined, ReturnType<typeof restoreRecoveryNotice>][] = [
    [undefined, undefined], [{}, undefined], [{indexesRebuilt: false, salvaged: false}, undefined],
    [{indexesRebuilt: true}, 'repaired'], [{indexesRebuilt: true, salvaged: false}, 'repaired'],
    [{salvaged: true}, 'recovered'], [{indexesRebuilt: true, salvaged: true, damagedTables: [], removedRows: 0}, 'recovered'],
    [{salvaged: true, damagedTables: ['media']}, 'partial'], [{salvaged: true, removedRows: 1}, 'partial'],
    [{indexesRebuilt: true, salvaged: true, damagedTables: ['media'], removedRows: 2}, 'partial'],
  ];
  for (const [recovery, notice] of cases) assert.equal(restoreRecoveryNotice(recovery), notice);
});

test('the backups document carries manual and scheduled kinds and both recovery reports', () => {
  const recovery = {indexesRebuilt: true, salvaged: true, damagedTables: ['media'], removedRows: 2};
  const running = {jobId: 'backup-1', phase: 'copy', bytesDone: 1, bytesTotal: 2};
  for (const kind of ['manual', 'scheduled'] as const) {
    const doc = parseBackupsDocument({backups: [], running: {...running, kind},
      lastBackup: {jobId: 'backup-0', outcome: 'failed', errorCode: 'backup_busy'},
      restore: {...running, jobId: 'restore-1', phase: 'staged', result: staged(recovery)},
      lastRestore: {at: '2026-10-28', outcome: 'restored', recovery}});
    assert.equal(doc.running?.kind, kind);
    assert.equal(doc.lastBackup?.errorCode, 'backup_busy');
    assert.deepEqual(doc.restore?.result?.validation.recovery, recovery);
    assert.deepEqual(doc.lastRestore?.recovery, recovery);
    assert(Object.isFrozen(doc.lastRestore?.recovery));
    assert(Object.isFrozen(doc.restore?.result?.validation.recovery?.damagedTables));
  }
  for (const kind of [undefined, 'future']) assert.equal(parseBackupsDocument({backups: [], running: {...running, kind}}).running?.kind, undefined);
});

test('recovered receipts retain validated recovery details and reject unreadable reports', () => {
  const recovery = {indexesRebuilt: true};
  assert.deepEqual(parseRestoreStaged(staged(recovery)).validation, {schemaVersion: 41, integrity: 'recovered', recovery});
  assert.equal(parseRestoreStaged({staged: true, restartRequired: true, validation: {schemaVersion: 41, integrity: 'recovered'}}).validation.integrity, 'recovered');
  for (const invalid of [null, [], {indexesRebuilt: 1}, {salvaged: 'true'}, {damagedTables: 'media'}, {damagedTables: [1]}, {removedRows: -1}, {removedRows: 0.5}]) {
    assert.throws(() => parseRestoreStaged(staged(invalid)), {code: 'invalid_administration_response'});
  }
});

test('missing the staged response during restart preserves the completed recovery report', async () => {
  for (const recovery of [{indexesRebuilt: true}, {salvaged: true}, {salvaged: true, removedRows: 2}]) {
    const api: BackupTransport = {request: async <T,>(_path: string, method?: string) => ({protocolVersion: '1.0', serverId: 'server-1',
      result: method === 'POST' ? {jobId: 'restore-1'} : {backups: [], lastRestore: {jobId: 'restore-1', schemaVersion: 41, at: '2026-10-28', outcome: 'restored', recovery}}}) as T};
    const result = await restoreBackup(api, 'server-1', 'operation-1', {path: '/backup/portico.db'});
    assert.equal(result.validation.integrity, 'recovered');
    assert.deepEqual(result.validation.recovery, recovery);
  }
});
