import test from 'node:test';
import assert from 'node:assert/strict';
import {bindSettingsDownloads, installSettingsPlatform, settingsPlatform, type SettingsDownloadsPort, type SettingsPlatform, type SettingsViewerScope} from '../src/platform/settings-platform.ts';

const scope: SettingsViewerScope = {serverId: 'server-a', authority: 'local', accountId: 'person-a', profileId: 'profile-a', sessionFamilyId: 'family-a'};
test('native platform installation is explicit and retired cleanup cannot clear a replacement', () => {
  const first: SettingsPlatform = {environment: {platform: 'desktop', os: 'windows'}};
  const second: SettingsPlatform = {environment: {platform: 'desktop', os: 'linux'}};
  const offFirst = installSettingsPlatform(first), offSecond = installSettingsPlatform(second);
  try { offFirst(); assert.equal(settingsPlatform(), second); } finally { offSecond(); }
  assert.equal(settingsPlatform(), undefined);
});
test('an actual signed-in scope is captured and private storage facts disappear before late notifications', async () => {
  let active = true, notifications = 0, disposed = 0, listener = () => {}, opens = 0;
  const native: SettingsDownloadsPort = {scope, getSnapshot: () => ({available: true, freeBytes: 12000}), subscribe: fn => {listener = fn; return () => {listener = () => {};};}, open: async () => {opens++;}, dispose: () => {disposed++;}};
  const platform: SettingsPlatform = {environment: {platform: 'desktop', os: 'windows'}, bindDownloads: received => {assert.deepEqual(received, scope); return native;}};
  const port = bindSettingsDownloads(platform, scope, () => active)!;
  try {
    port.subscribe(() => notifications++);
    assert.equal(port.getSnapshot().freeBytes, 12000);
    await port.open(new AbortController().signal);
    assert.equal(opens, 1);
    active = false; listener();
    assert.equal(notifications, 0);
    assert.deepEqual(port.getSnapshot(), {available: false});
    await assert.rejects(port.open(new AbortController().signal), /settings_download_scope_closed/);
    assert.equal(opens, 1);
  } finally {port.dispose();port.dispose();}
  assert.equal(disposed, 1);
});
test('cross-viewer adapters close immediately and no server-global fallback is supplied', () => {
  assert.equal(bindSettingsDownloads(undefined, scope, () => true), undefined);
  let disposed = 0;
  const platform: SettingsPlatform = {environment: {platform: 'desktop', os: 'macos'}, bindDownloads: () => ({scope: {...scope, profileId: 'another-profile'}, getSnapshot: () => ({available: true, freeBytes: 999}), subscribe: () => () => {}, open: async () => {}, dispose: () => {disposed++;}})};
  assert.throws(() => bindSettingsDownloads(platform, scope, () => true), /settings_download_scope_mismatch/);
  assert.equal(disposed, 1);
});
test('disposing the shared door aborts an in-flight native open', async () => {
  let signal: AbortSignal | undefined;
  const platform: SettingsPlatform = {environment: {platform: 'desktop', os: 'linux'}, bindDownloads: () => ({scope, getSnapshot: () => ({available: true}), subscribe: () => () => {}, open: async value => {signal = value; await new Promise<void>((_resolve, reject) => value.addEventListener('abort', () => reject(value.reason), {once: true}));}, dispose: () => {}})};
  const port = bindSettingsDownloads(platform, scope, () => true)!;
  const opened = port.open(new AbortController().signal);
  port.dispose();
  await assert.rejects(opened);
  assert.equal(signal?.aborted, true);
});
