import test from 'node:test';
import assert from 'node:assert/strict';
import {PreferencesReader} from '@core/preferences-reader.ts';
import type {PreferenceSnapshot} from '@core/preferences.ts';
import type {SettingsConfiguration} from '@core/settings-configuration.ts';
import {settingsSpec} from '@core/presentation/settings-spec.ts';
import {SettingsController, type SettingsControllerOptions} from '../src/screens/settings/controller.ts';

const serverId = 'captured-server', fence = 'a'.repeat(64);
const section = settingsSpec.find(section => section.scope === 'server' && section.id === 'integrations')!;
const configuration: SettingsConfiguration = {revision: 4, registryRevision: 'actual-registry', path: '/actual/portico.toml', fields: [
  {id: 'integrations.maxWebhooks', key: 'integrations.max_webhooks', current: 12, effective: 10, default: 8, source: 'file', configOnly: true, applicationMode: 'next-operation'},
]};
const preferences: PreferenceSnapshot = {deviceClass: 'web', registry: {revision: 'test', fields: []}, documents: [], effective: {}, effectiveSource: {}, clampedFields: [], registryRevision: 'test'};

function fixture(input: {owner?: boolean; readConfiguration?: SettingsControllerOptions['readConfiguration']; hold?: boolean; foreign?: boolean} = {}) {
  const calls: string[] = [], signals: AbortSignal[] = [];
  let release!: () => void, current = true, configurationSignal: AbortSignal | undefined;
  const gate = input.hold ? new Promise<void>(resolve => {release = resolve;}) : Promise.resolve();
  const api: SettingsControllerOptions['api'] = {request: async <T>(path: string, _method?: string, _body?: unknown, signal?: AbortSignal) => {
    calls.push(path);if (signal) signals.push(signal);
    const scope = {serverId: input.foreign ? 'another-server' : serverId, viewerFence: fence};
    if (path === '/v1/admin/settings/configuration') {configurationSignal = signal;await gate;return {scope, data: configuration} as T;}
    const name = path.split('/').at(-1)!;
    return {scope, data: {name, observedAt: 1000, freshUntil: 31000, facts: {}}} as T;
  }};
  const reader = new PreferencesReader(api, {serverId, viewerId: 'captured-viewer'});
  const controller = new SettingsController({api, serverId, viewerId: 'captured-viewer', role: input.owner === false ? 'member' : 'owner', mainProfile: true,
    capabilities: {manageServer: input.owner !== false, managePeople: false, manageSignIn: false}, preferences, reader,
    readConfiguration: input.readConfiguration, isCurrent: () => current, reportError: error => {throw error;},
    environment: {platform: 'web', os: 'browser'}, eligibility: {hasEngine: () => false, library: () => undefined},
    reading: {read: () => {throw new Error('Unexpected Reading read');}, patch: () => {throw new Error('Unexpected Reading write');}},
  });
  return {controller, calls, signals, configurationSignal: () => configurationSignal, release: () => release(), retire: () => {current = false;}, dispose: () => {controller.dispose();reader.dispose();}};
}

test('Settings reads canonical configuration through its captured ConsoleClient, once per successful scope', async () => {
  const f = fixture();
  try {
    assert.deepEqual(f.calls, []);
    await f.controller.load(section);
    const matches = f.controller.configurationMatches('integrations.max_webhooks');
    assert.equal(matches?.length, 1);
    assert.deepEqual(matches?.[0], {...configuration.fields[0], label: matches?.[0].label, path: configuration.path});
    assert.equal(f.controller.getSnapshot(section).loading, false);
    await f.controller.load(section);
    assert.equal(f.calls.filter(path => path === '/v1/admin/settings/configuration').length, 1);
  } finally {f.dispose();}
});

test('foreign configuration fails the real section read and is never searchable', async () => {
  const f = fixture({foreign: true});
  try {await f.controller.load(section);assert.ok(f.controller.getSnapshot(section).error);assert.deepEqual(f.controller.configurationMatches('integrations.max_webhooks'), []);}
  finally {f.dispose();}
});

test('disposal aborts the same console request and refuses a late configuration result', async () => {
  const f = fixture({hold: true}), pending = f.controller.load(section);
  await new Promise(resolve => setImmediate(resolve));
  f.dispose();f.release();await pending;
  assert.equal(f.configurationSignal()?.aborted, true);
  assert.deepEqual(f.controller.configurationMatches('integrations.max_webhooks'), []);
});

test('denied members start no owner reads; an injected reader retains the same lifetime boundary', async () => {
  const denied = fixture({owner: false});
  try {await denied.controller.load(section);assert.deepEqual(denied.calls, []);} finally {denied.dispose();}
  let signal: AbortSignal | undefined, release!: (value: typeof configuration) => void;
  const f = fixture({readConfiguration: async captured => {signal = captured;return new Promise(resolve => {release = resolve;});}});
  const pending = f.controller.load(section);await new Promise(resolve => setImmediate(resolve));
  f.retire();release(configuration);await pending;
  assert.deepEqual(f.controller.configurationMatches('integrations.max_webhooks'), []);
  f.dispose();assert.equal(signal?.aborted, true);
});
