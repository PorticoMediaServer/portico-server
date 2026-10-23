import test from 'node:test';
import assert from 'node:assert/strict';
import {settingsRowIndex, settingsSpec} from '../../packages/client-core/src/presentation/settings-spec.ts';
import {createSettingsBindings} from '../src/screens/settings/bindings.ts';
import type {SettingsController, SettingsControllerOptions} from '../src/screens/settings/controller.ts';
import {SignInSettingsSource} from '../src/screens/settings/signin-source.ts';
import type {ShellIdentity} from '../src/shell/Shell.tsx';

const signIn = settingsSpec.find(section => section.scope === 'server' && section.id === 'signin')!;
const general = settingsSpec.find(section => section.scope === 'server' && section.id === 'general')!;
const capabilities = {manageAccount: true, managePeople: true, manageServer: true, manageSignIn: true};
const owner = {accountId: 'actual-account', username: 'actual-owner', displayName: 'Actual owner'};

function setup(read: () => Promise<unknown>) {
  const loads: string[] = [], generic = Object.freeze({loading: false}), listeners = new Set<() => void>();
  const controller = {
    subscribe(listener: () => void) {listeners.add(listener);return () => {listeners.delete(listener);};},
    getSnapshot: () => generic, sectionVisible: () => true, eligible: () => false,
    rowState: () => ({present: false}), load: async (section: typeof general) => {loads.push(section.id);},
  } as unknown as SettingsController;
  const source = new SignInSettingsSource({request: async <T>() => await read() as T}, {serverId: 'actual-server', signIn: 'local', isCurrent: () => true, capabilities: () => capabilities, onAuthorityChanged() {}});
  const options = {role: 'owner', environment: {platform: 'web', os: 'browser', role: 'owner'}} as SettingsControllerOptions;
  const identity = {profileName: 'Actual profile', serverName: 'Actual server', signIn: 'local'} as ShellIdentity;
  const binding = createSettingsBindings(controller, options, identity, {signIn: source, openHome() {}, special: () => null});
  return {binding, source, generic, loads, listeners};
}

test('Sign-in loads its actual guarded source without fetching unrelated Server configuration', async () => {
  let reads = 0;
  const state = setup(async () => {reads++;return {signIn: 'local', owner, move: null, webPath: '/settings?scope=server&section=signin'};});
  const row = settingsRowIndex.get('signin.ownerLocal')!.row;
  assert.equal(reads, 0);
  assert.equal(state.binding.eligible(row, {section: signIn}), false);
  await state.binding.load(signIn);
  assert.equal(reads, 1);assert.deepEqual(state.loads, []);
  assert.equal(state.binding.getSnapshot(signIn).loading, false);
  assert.equal(state.binding.rowState(row, {section: signIn}).displayValue, 'actual-owner');
  assert.equal(state.binding.eligible(row, {section: signIn}), true);
  await state.binding.load(general);
  assert.deepEqual(state.loads, ['general']);
  assert.equal(state.binding.getSnapshot(general), state.generic);
  state.source.dispose();
});

test('Sign-in failures stay on that page and its source notifications detach with the binding subscription', async () => {
  const failure = new Error('actual_transport_failure'), state = setup(async () => {throw failure;});
  let notifications = 0;
  const off = state.binding.subscribe(() => {notifications++;});
  await state.binding.load(signIn);
  assert.equal(state.binding.getSnapshot(signIn).error, failure);
  assert.equal(state.binding.getSnapshot(general), state.generic);
  assert.ok(notifications > 0);assert.equal(state.listeners.size, 1);
  off();const before = notifications;
  assert.equal(state.listeners.size, 0);
  await state.binding.load(signIn);
  assert.equal(notifications, before);
  state.source.dispose();
});
