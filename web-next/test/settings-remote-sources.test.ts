import test from 'node:test';
import assert from 'node:assert/strict';
import {registerHooks} from 'node:module';
import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {transformTsx} from './transform-tsx.mjs';

registerHooks({load(url, context, next) {
  if (url.endsWith('.module.css')) return {format: 'module', source: 'export default new Proxy({}, {get: (_, key) => String(key)});', shortCircuit: true};
  if (url.endsWith('.tsx')) return {format: 'module', source: transformTsx(url), shortCircuit: true};
  return next(url, context);
}});
const {defaultI18n} = await import('../../packages/i18n/src/index.ts');
const {sourceReadPreset, sourceReadPresetName} = await import('../../packages/client-core/src/remote-sources.ts');
const {RemoteSourcesPage, remoteErrorText} = await import('../src/screens/settings/remote-sources/RemoteSourcesPage.tsx');

test('the four reading choices are the server SourceReadPreset values', () => {
  assert.deepEqual(sourceReadPreset('list'), {scanDepth: 'file_list_only', wholeFileReads: false, connections: 2, requestsPerSecond: 2, bytesPerSecond: 0});
  assert.deepEqual(sourceReadPreset('once'), {scanDepth: 'basic', wholeFileReads: false, connections: 4, requestsPerSecond: 4, bytesPerSecond: 0});
  assert.deepEqual(sourceReadPreset('everything'), {scanDepth: 'complete', wholeFileReads: true, connections: 8, requestsPerSecond: 10, bytesPerSecond: 0});
  assert.deepEqual(sourceReadPreset('ask'), {scanDepth: 'basic', wholeFileReads: false, connections: 2, requestsPerSecond: 2, bytesPerSecond: 0});
  assert.equal(sourceReadPresetName(sourceReadPreset('everything')), 'everything');
  assert.equal(sourceReadPresetName({...sourceReadPreset('once'), connections: 6}), undefined);
});

test('the Remote sources page draws the add action and its empty state', () => {
  const markup = renderToStaticMarkup(createElement(RemoteSourcesPage, {
    api: {request: async () => ({})}, serverId: 'server', viewerId: 'owner', isCurrent: () => true,
  }));
  assert.match(markup, /Add remote source/);
  assert.match(markup, /No remote sources yet/);
  assert.match(markup, /Folder browsing inside a remote source is not available yet/);
});

test('the source states and the blocked sentence have their drawn words', () => {
  assert.equal(defaultI18n.t('next.remoteSources.state.unreachable'), 'Can’t be reached');
  assert.equal(defaultI18n.t('next.remoteSources.state.credentials'), 'Needs its sign-in again');
  assert.equal(defaultI18n.t('next.remoteSources.state.waiting'), 'Waiting');
  assert.equal(defaultI18n.t('next.remoteSources.state.limited'), 'Limited');
  assert.match(defaultI18n.t('next.remoteSources.problem.range_unsupported'), /read parts of a file/);
});

test('a hostile transport message is never shown; only a bounded source error is', () => {
  const hostile = Object.assign(new Error('https://private-user:private-password@private.example/media/'), {code: 'remote_offline'});
  const text = remoteErrorText(hostile);
  assert.equal(text, defaultI18n.t('next.remoteSources.error.generic'));
  assert.doesNotMatch(text, /private-user|private-password|private\.example/);
  assert.equal(remoteErrorText(Object.assign(new Error('x'), {code: 'remote_credentials_required'})), defaultI18n.t('next.remoteSources.problem.credentials_required'));
  assert.equal(remoteErrorText(Object.assign(new Error('x'), {code: 'refresh_failed'})), defaultI18n.t('next.remoteSources.error.refreshFailed'));
});
