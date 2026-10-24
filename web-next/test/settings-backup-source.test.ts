import test from 'node:test';
import assert from 'node:assert/strict';
import type {BackupTransport} from '@core/backups.ts';
import type {BackupsDocument} from '@core/administration.ts';
import {SettingsBackupSource} from '../src/screens/settings/backup-source.ts';

const serverId = 'captured-server';
const settings = {backupFolder: '/actual/backups', includeArtwork: true, passphraseProtected: false, passphraseSet: false};
const backup = {id: 'listed', createdAt: '2026-10-23T12:00:00Z', kind: 'manual', schemaVersion: 1, serverVersion: '1', bytes: 100, path: '/actual/backups/listed', encrypted: true};
const envelope = (result: unknown) => ({protocolVersion: '1.0', serverId, result});
type Request = (path: string, method: string, body: unknown, signal?: AbortSignal) => Promise<unknown>;
function fixture(request?: Request) {
  let current = true;
  const calls: {path: string; method: string; body: unknown; signal?: AbortSignal}[] = [];
  const api: BackupTransport = {request: async <T>(path: string, method = 'GET', body?: unknown, signal?: AbortSignal) => {
    calls.push({path, method, body, signal});
    return (request ? await request(path, method, body, signal) : envelope(path.endsWith('/settings') ? settings : {backups: [backup]})) as T;
  }};
  return {source: new SettingsBackupSource(api, serverId, () => current, () => {}), calls, retire: () => {current = false;}};
}

test('Backup constructors are inert and confirmed snapshots retain only safe settings and real list flags', async () => {
  const f = fixture(async path => envelope(path.endsWith('/settings') ? settings : {backups: [backup]}));
  try {
    assert.deepEqual(f.calls, []);await f.source.load();
    assert.deepEqual(f.source.getSnapshot().settings, settings);assert.equal(f.source.getSnapshot().document?.backups[0].encrypted, true);
    f.retire();assert.deepEqual(f.source.getSnapshot(), {busy: false});await f.source.load();assert.equal(f.calls.length, 2);
  } finally {f.source.dispose();}
});

test('secret settings writes return only the canonical safe readback, with no optimistic or retained passphrase', async () => {
  let release!: () => void;
  const f = fixture(async (path, method, body) => {
    if (method === 'PUT') {
      assert.deepEqual(body, {passphraseProtected: true, passphrase: 'temporary test phrase'});
      await new Promise<void>(resolve => {release = resolve;});
      return envelope({...settings, passphraseProtected: true, passphraseSet: true});
    }
    return envelope(path.endsWith('/settings') ? settings : {backups: [backup]});
  });
  try {
    await f.source.load();const pending = f.source.save({passphraseProtected: true, passphrase: 'temporary test phrase'});
    assert.equal(f.source.getSnapshot().settings?.passphraseProtected, false);assert.equal(f.source.getSnapshot().busy, true);
    assert.equal(await f.source.save({includeArtwork: false}), false);release();assert.equal(await pending, true);
    assert.equal(f.source.getSnapshot().settings?.passphraseSet, true);
    assert.doesNotMatch(JSON.stringify(f.source.getSnapshot()), /temporary test phrase|"passphrase":/);
  } finally {f.source.dispose();}
});

test('inspection uses actual content flags, refuses missing list members and aborts dialog-retired replies', async () => {
  let hold = false, release!: () => void, captured: AbortSignal | undefined;
  const f = fixture(async (path, _method, body, signal) => {
    if (path.endsWith('/inspect')) {
      captured = signal;if (hold) await new Promise<void>(resolve => {release = resolve;});
      return envelope({encrypted: (body as {source: {path?: string}}).source.path?.endsWith('.db') === true});
    }
    return envelope(path.endsWith('/settings') ? settings : {backups: [backup]});
  });
  try {
    await f.source.load();const live = new AbortController();
    assert.deepEqual(await f.source.inspect({path: '/actual/plain.pba'}, live.signal), {encrypted: false});
    assert.deepEqual(await f.source.inspect({path: '/actual/protected.db'}, live.signal), {encrypted: true});
    const count = f.calls.length;await assert.rejects(f.source.inspect({backupId: 'not-listed'}, live.signal));assert.equal(f.calls.length, count);
    hold = true;const pending = f.source.inspect({backupId: 'listed'}, live.signal);
    live.abort();release();await assert.rejects(pending, {name: 'AbortError'});assert.equal(captured?.aborted, true);
  } finally {f.source.dispose();}
});

test('Backup Now waits for its returned job ID across idle and older successful results', async () => {
  let started = false, reads = 0;
  const results: BackupsDocument[] = [{backups: [], lastBackup: {jobId: 'older', outcome: 'completed'}}, {backups: []}, {backups: [], lastBackup: {jobId: 'started-here', outcome: 'completed'}}];
  const f = fixture(async (path, method) => {
    if (path.endsWith('/settings')) return envelope(settings);
    if (method === 'POST') {started = true;return envelope({jobId: 'started-here'});}
    return envelope(started ? results[reads++] : {backups: []});
  });
  try {
    await f.source.load();assert.equal(await f.source.backupNow({intervalMs: 0, timeoutMs: 100}), true);
    assert.equal(reads, 3);assert.equal(f.source.getSnapshot().document?.lastBackup?.jobId, 'started-here');
  } finally {f.source.dispose();}
});

test('a matching failed job rejects, and a disposed start cannot begin polling or publish success', async () => {
  let started = false;
  const failed = fixture(async (path, method) => {
    if (path.endsWith('/settings')) return envelope(settings);
    if (method === 'POST') {started = true;return envelope({jobId: 'job'});}
    return envelope(started ? {backups: [], lastBackup: {jobId: 'job', outcome: 'failed', errorCode: 'insufficient_disk'}} : {backups: []});
  });
  try {await failed.source.load();await assert.rejects(failed.source.backupNow({intervalMs: 0}), {code: 'insufficient_disk'});assert.equal(failed.source.getSnapshot().busy, false);}
  finally {failed.source.dispose();}
  let release!: () => void, signal: AbortSignal | undefined;
  const late = fixture(async (path, method, _body, captured) => {
    if (method === 'POST') {signal = captured;await new Promise<void>(resolve => {release = resolve;});return envelope({jobId: 'late'});}
    return envelope(path.endsWith('/settings') ? settings : {backups: []});
  });
  await late.source.load();const pending = late.source.backupNow();late.source.dispose();release();
  await assert.rejects(pending, {name: 'AbortError'});assert.equal(signal?.aborted, true);assert.equal(late.calls.length, 3);
  assert.deepEqual(late.source.getSnapshot(), {busy: false});
});

test('an owner refusal clears retained facts and closes all later commands immediately', async () => {
  for (const operation of ['load', 'save', 'inspect', 'start']) {
    let denied = false;
    const failure = Object.assign(new Error('denied'), {status: 403});
    const f = fixture(async path => {
      if (denied) throw failure;
      return envelope(path.endsWith('/settings') ? settings : {backups: [backup]});
    });
    try {
      await f.source.load();denied = true;
      const pending = operation === 'load' ? f.source.load() : operation === 'save' ? f.source.save({includeArtwork: false}) : operation === 'inspect' ? f.source.inspect({backupId: 'listed'}, new AbortController().signal) : f.source.backupNow();
      await assert.rejects(pending, failure);
      assert.deepEqual(f.source.getSnapshot(), {busy: false});
      const count = f.calls.length;await assert.rejects(f.source.load(), {name: 'AbortError'});assert.equal(await f.source.save({includeArtwork: true}), false);assert.equal(await f.source.backupNow(), false);
      assert.equal(f.calls.length, count);
    } finally {f.source.dispose();}
  }
});
