import test from 'node:test';
import assert from 'node:assert/strict';
import type {SettingsDevicePort} from '../src/platform/settings-platform.ts';
import {readDeviceSettingsBinding, writeDeviceSettingsBinding} from '../src/screens/settings/device-bindings.ts';

test('the native facade supplies defaults and exact values through the existing binding codec', () => {
  const port: SettingsDevicePort = {subscribe: () => () => {}, getSnapshot: () => ({facts: {}, document: {revision: 'native-17', values: {bufferMs: 2500, enabled: false}, defaults: {bufferMs: 1000, enabled: true}}}), apply: async () => {}};
  const value = readDeviceSettingsBinding({form: 'device', key: 'bufferMs', scale: 1000}, port, {}, 'fixture.buffer');
  assert.equal(value?.current, 2.5);
  assert.equal(value?.default, 1);
  assert.equal(readDeviceSettingsBinding({form: 'device', key: 'enabled'}, port, {}, 'fixture.enabled')?.current, false);
  assert.equal(readDeviceSettingsBinding({form: 'runtime', key: 'bufferMs'}, port, {}, 'fixture.buffer'), undefined);
  assert.equal(readDeviceSettingsBinding({form: 'device', key: 'missing'}, port, {}, 'fixture.missing'), undefined);
});
test('missing native defaults stay unknown and no server configuration value substitutes for a device facade', () => {
  const port: SettingsDevicePort = {subscribe: () => () => {}, getSnapshot: () => ({facts: {}, document: {revision: 'native-18', values: {output: 'actual-device-id'}, defaults: {}}}), apply: async () => {}};
  const source = {configuration: {revision: 1, registryRevision: 'fixture', path: 'fixture', fields: [{id: 'device.output', key: 'output', current: 'server-id', effective: 'server-id', default: 'guessed', source: 'server' as const, configOnly: false, applicationMode: 'immediate'}]}};
  const value = readDeviceSettingsBinding({form: 'device', key: 'output'}, port, source, 'device.output');
  assert.equal(value?.current, 'actual-device-id');
  assert.equal(value?.default, undefined);
  assert.equal(readDeviceSettingsBinding({form: 'device', key: 'output'}, undefined, source, 'device.output'), undefined);
});
test('writes use the latest native revision and a late completion cannot report success for a retired viewer', async () => {
  let current = true, revision = 'native-1', complete!: () => void;
  const received: unknown[] = [];
  const port: SettingsDevicePort = {subscribe: () => () => {}, getSnapshot: () => ({facts: {}, document: {revision, values: {}, defaults: {}}}), apply: async (expectedRevision, patch) => {received.push({expectedRevision, patch});await new Promise<void>(resolve => {complete = resolve;});}};
  revision = 'native-2';
  const write = writeDeviceSettingsBinding(port, {enabled: false}, new AbortController().signal, () => current);
  assert.deepEqual(received, [{expectedRevision: 'native-2', patch: {enabled: false}}]);
  current = false; complete();
  await assert.rejects(write, {name: 'AbortError'});
  await assert.rejects(writeDeviceSettingsBinding(port, {enabled: true}, new AbortController().signal, () => current), {name: 'AbortError'});
  assert.equal(received.length, 1);
});
