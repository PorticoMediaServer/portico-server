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
    api: {request: async () => ({})}, serverId: 'server', viewerId: 'owner', isCurrent: () => true, reportError: () => {},
  }));
  assert.match(markup, /Add remote source/);
  assert.match(markup, /No remote sources yet/);
  assert.match(markup, /give Portico a folder on another computer/);
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

test('saved Add handlers cannot adopt a reopened form lifetime', async () => {
  const React = await import('react');
  const {AddSourceDialog} = await import('../src/screens/settings/remote-sources/RemoteSourcesPage.tsx');
  // Execute the real dialog hooks and effects, retaining callbacks from open A.
  const internals = (React as any).__CLIENT_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE;
  const previous = internals.H;
  const slots: any[] = []; let cursor = 0, effects: (() => void)[] = [], dirty = false;
  const dispatcher = {
    useState(initial: any) {const i = cursor++;if (!(i in slots)) slots[i] = typeof initial === 'function' ? initial() : initial;
      return [slots[i], (value: any) => {slots[i] = typeof value === 'function' ? value(slots[i]) : value;dirty = true;}];},
    useRef(initial: any) {const i = cursor++;return slots[i] ??= {current: initial};},
    useEffect(effect: () => void, deps: any[]) {const i = cursor++, old = slots[i];
      if (!old || deps.some((value, index) => value !== old[index])) {slots[i] = deps;effects.push(effect);}},
  };
  let io = 0, completions = 0, open = true;
  const service = {testConnection: async () => {io++;return {origin: 'https://a.example', rangeSupport: 'unknown'};},
    configure: async () => {io++;}, getSnapshot: () => ({})} as any;
  const render = () => {let tree: any;do {dirty = false;cursor = 0;effects = [];internals.H = dispatcher;
    try {tree = AddSourceDialog({service, open, isCurrent: () => true, reportError: () => completions++, onClose: () => completions++, onCreated: () => completions++});}
    finally {internals.H = previous;}for (const effect of effects) effect();
  } while (dirty);return tree;};
  const elements = (node: any): any[] => Array.isArray(node) ? node.flatMap(elements) : node && typeof node === 'object' && node.props ? [node, ...elements(node.props.children), ...elements(node.props.footer)] : [];
  const button = (tree: any, label: string) => elements(tree).find(node => node.props.children === defaultI18n.t(label as any));
  let tree = render();
  for (const [label, value] of [['name', 'A'], ['address', 'https://a.example'], ['password', 'secret-A']]) {
    elements(tree).find(node => node.props.label === defaultI18n.t(`next.remoteSources.${label}` as any)).props.onChange({target: {value}});tree = render();
  }
  const oldTest = button(tree, 'next.remoteSources.test').props.onClick;
  oldTest();await new Promise(resolve => setImmediate(resolve));tree = render();
  const oldCreate = button(tree, 'next.remoteSources.saveAndContinue').props.onClick;
  assert.equal(button(tree, 'next.remoteSources.saveAndContinue').props.disabled, false);
  button(tree, 'next.remoteSources.cancel').props.onClick();open = false;render();open = true;tree = render();
  const before = {io, completions, state: JSON.stringify(slots)};
  oldTest();await oldCreate();await new Promise(resolve => setImmediate(resolve));
  assert.equal(io, before.io, 'old form sends no provider test or mutation');
  assert.equal(completions, before.completions, 'old form cannot close or complete successor');
  assert.equal(JSON.stringify(slots), before.state, 'successor hook state stays unchanged');
});

test('the imported rclone kind has its drawn words and its bound program error', () => {
  assert.equal(defaultI18n.t('next.remoteSources.kindRclone'), 'Imported rclone source');
  assert.match(defaultI18n.t('next.remoteSources.connectRclone'), /Paste a configuration/);
  assert.match(defaultI18n.t('next.remoteSources.configurationHelp'), /never shows it again/);
  assert.match(defaultI18n.t('next.remoteSources.bundledProgram'), /comes with Portico Server/);
  assert.equal(remoteErrorText(Object.assign(new Error('x'), {code: 'administration_unavailable'})), defaultI18n.t('next.remoteSources.problem.administration_unavailable'));
  assert.equal(remoteErrorText(Object.assign(new Error('x'), {code: 'remote_administration_unavailable'})), defaultI18n.t('next.remoteSources.problem.administration_unavailable'));
});

test('the account-configuration kind has its drawn words', () => {
  assert.equal(defaultI18n.t('next.remoteSources.connectFromConfig'), 'From my config');
  assert.match(defaultI18n.t('next.remoteSources.configPath'), /name and kind/);
  assert.match(defaultI18n.t('next.remoteSources.configEncrypted'), /encrypted/);
  assert.match(defaultI18n.t('next.remoteSources.configChecksOnSave'), /tests the remote when you save/);
});

test('the S3 kind has its drawn fields and its own refusal words', () => {
  assert.equal(defaultI18n.t('next.remoteSources.connectS3'), 'S3 storage');
  assert.match(defaultI18n.t('next.remoteSources.s3EndpointHelp'), /https/);
  assert.match(defaultI18n.t('next.remoteSources.s3Checks'), /nothing is typed in rclone/);
  assert.match(defaultI18n.t('next.remoteSources.s3Invalid'), /Check the endpoint/);
});
