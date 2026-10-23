import test from 'node:test';
import assert from 'node:assert/strict';
import {SettingsSectionReads} from '../src/screens/settings/section-loading.ts';

test('an unrelated failed section leaves the loaded account section and shared successful documents intact', async () => {
  const source = new SettingsSectionReads(() => true, () => {});
  let configurationReads = 0, logReads = 0;
  const configuration = {id: 'configuration', load: async () => {configurationReads++;}};
  await source.load('account/general', []);
  const account = source.snapshot('account/general');
  const failure = new Error('unavailable');
  await source.load('server/logs', [configuration, {id: 'logs', load: async () => {logReads++;throw failure;}}]);
  assert.equal(source.snapshot('account/general'), account);
  assert.equal(source.snapshot('server/logs').error, failure);
  await source.load('server/general', [configuration]);
  assert.deepEqual(source.snapshot('server/general'), {loading: false, saveState: undefined});
  assert.equal(configurationReads, 1);
  assert.equal(logReads, 1);
});

test('retry repeats a failed dependency and does not reread successful shared dependencies', async () => {
  const source = new SettingsSectionReads(() => true, () => {});
  let configurationReads = 0, logReads = 0;
  const reads = [{id: 'configuration', load: async () => {configurationReads++;}}, {id: 'logs', load: async () => {if (++logReads === 1) throw new Error('unavailable');}}];
  await source.load('server/logs', reads);
  await source.load('server/logs', reads);
  assert.equal(source.snapshot('server/logs').loading, false);
  assert.equal(source.snapshot('server/logs').error, undefined);
  assert.equal(configurationReads, 1);
  assert.equal(logReads, 2);
});

test('overlapping sections deduplicate the same document read while keeping independent loading states', async () => {
  const source = new SettingsSectionReads(() => true, () => {});
  let finish!: () => void, reads = 0;
  const document = {id: 'shared', load: async () => {reads++;await new Promise<void>(resolve => {finish = resolve;});}};
  const general = source.load('server/general', [document]);
  const streaming = source.load('server/streaming', [document]);
  assert.equal(source.load('server/general', [document]), general);
  await Promise.resolve();
  assert.equal(reads, 1);
  assert.equal(source.snapshot('server/general').loading, true);
  assert.equal(source.snapshot('server/streaming').loading, true);
  finish();await Promise.all([general, streaming]);
  assert.equal(source.snapshot('server/general').loading, false);
  assert.equal(source.snapshot('server/streaming').loading, false);
});

test('retiring authority erases snapshots and a late read cannot publish or restore them', async () => {
  let current = true, notifications = 0, finish!: () => void;
  const source = new SettingsSectionReads(() => current, () => {notifications++;});
  const pending = source.load('server/general', [{id: 'runtime', load: () => new Promise<void>(resolve => {finish = resolve;})}]);
  await Promise.resolve();
  current = false;source.dispose();
  const notified = notifications;
  finish();await pending;
  assert.deepEqual(source.snapshot('server/general'), {loading: true});
  assert.equal(notifications, notified);
  let requests = 0;
  await source.load('server/general', [{id: 'runtime', load: async () => {requests++;}}]);
  assert.equal(requests, 0);
});
