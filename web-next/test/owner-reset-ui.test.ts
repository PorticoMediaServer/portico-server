import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import type {AccountEntryState} from '../src/platform/account-entry.ts';
import {finishOwnerResetAndSignIn} from '../src/screens/accounts/owner-reset.ts';
import {componentModule, hooks} from '../../web/test/helpers/component-harness.mjs';

function fixture() {
  const server = {id: 'server', name: 'Actual server', version: '1', signIn: 'local' as const, move: null, setupRequired: false, hostedConfigured: false, hostedAttached: false};
  const pin = Object.freeze({serverId: server.id, publicKey: 'key', fingerprint: 'fingerprint'});
  let state: AccountEntryState = {phase: 'owner-reset-password', server, origin: 'https://server.example', pin, ownerReset: {username: 'actual_owner', twoStepEnabled: true}};
  const calls: unknown[][] = [];
  const entry = {
    getSnapshot: () => state,
    finishLocalOwnerResetPassword: async (password: string, turnOff: boolean) => {calls.push(['finish', password, turnOff]);state = {...state, phase: 'password', ownerReset: undefined};return true;},
  };
  const form = {signIn: async (username: string, password: string) => {calls.push(['ordinary-sign-in', username, password]);}};
  return {entry, form, calls, set: (value: Partial<AccountEntryState>) => {state = {...state, ...value};}};
}

test('owner reset finishes once then uses ordinary sign-in to the original verified server', async () => {
  const f = fixture();
  await finishOwnerResetAndSignIn(f.entry, f.form, 'private-new-password', false, () => true);
  assert.deepEqual(f.calls, [['finish', 'private-new-password', false], ['ordinary-sign-in', 'actual_owner', 'private-new-password']]);
  assert.doesNotMatch(JSON.stringify(f.entry.getSnapshot()), /private-new-password/);
});

test('failed atomic reset never starts ordinary sign-in or retries the spent file code', async () => {
  const f = fixture();f.entry.finishLocalOwnerResetPassword = async () => false;
  await finishOwnerResetAndSignIn(f.entry, f.form, 'private-new-password', true, () => true);
  assert.deepEqual(f.calls, []);
});

test('explicit close or a changed origin/pin after completion cannot retarget ordinary sign-in', async () => {
  for (const change of ['closed', 'origin', 'pin']) {
    const f = fixture();let current = true;
    f.entry.finishLocalOwnerResetPassword = async () => {
      f.set({phase: 'password', ownerReset: undefined});
      if (change === 'closed') current = false;
      if (change === 'origin') f.set({origin: 'https://other.example'});
      if (change === 'pin') f.set({pin: {serverId: 'server', publicKey: 'different', fingerprint: 'different'}});
      return true;
    };
    await finishOwnerResetAndSignIn(f.entry, f.form, 'private-new-password', true, () => current);
    assert.deepEqual(f.calls, [], change);
  }
});

test('an unproved or retired phase never reaches the atomic password command', async () => {
  const f = fixture();f.set({phase: 'password', ownerReset: undefined});
  await finishOwnerResetAndSignIn(f.entry, f.form, 'private-new-password', false, () => true);
  assert.deepEqual(f.calls, []);
});

async function surface(phone = false) {
  const h = hooks();
  const m = await componentModule(new URL('../src/kit/accounts/OwnerPasswordRecovery.tsx', import.meta.url), {
    react: {...h.react, useId: () => 'strength'}, '@i18n': {defaultI18n},
    '../controls': {Button: 'button', TextField: 'field', Switch: 'switch'},
    './AuthStage': {AuthStage: 'stage', AuthPasswordField: 'password', AuthError: 'error'},
    './ServicesAuth': {ServicesStrength: 'strength'}, '../Icon': {Icon: 'icon'}, './accounts.module.css': {default: {}},
  });
  const previous = globalThis.matchMedia;
  globalThis.matchMedia = (() => ({matches: phone})) as typeof matchMedia;
  return {m, render: (props: unknown) => h.render(() => m.OwnerResetPassword(props)), restore: () => {globalThis.matchMedia = previous;}};
}
const nodes = (node: any): any[] => Array.isArray(node) ? node.flatMap(nodes) : node && typeof node === 'object' ? [node, ...nodes(node.props?.children)] : [];

test('owner20c keeps factor switch absent unless actual enabled, and desktop confirmation guards submit', async () => {
  const f = await surface(), calls: unknown[][] = [];
  const props = {serverName: 'Actual server', username: 'actual_owner', twoStepEnabled: false, onSubmit: (...args: unknown[]) => calls.push(args)};
  try {
    let tree = nodes(f.render(props));
    assert.equal(tree.some(node => node.type === 'switch'), false);
    tree.find(node => node.type === 'password').props.onChange({currentTarget: {value: 'a-strong-new-owner-password'}});
    tree = nodes(f.render(props));tree.find(node => node.type === 'form').props.onSubmit({preventDefault() {}});
    assert.deepEqual(calls, []);
    tree.find(node => node.type === 'field').props.onChange({currentTarget: {value: 'a-strong-new-owner-password'}});
    tree = nodes(f.render(props));tree.find(node => node.type === 'form').props.onSubmit({preventDefault() {}});
    assert.deepEqual(calls, [['a-strong-new-owner-password', false]]);
    assert.doesNotMatch(JSON.stringify(f.render(props)), /a-strong-new-owner-password/);
  } finally {f.restore();}
});

test('owner20c factor choice starts off; phone uses its one actual password field', async () => {
  const f = await surface(true), calls: unknown[][] = [];
  const props = {serverName: 'Actual server', username: 'actual_owner', twoStepEnabled: true, onSubmit: (...args: unknown[]) => calls.push(args)};
  try {
    let tree = nodes(f.render(props));
    assert.equal(tree.find(node => node.type === 'switch').props.checked, false);
    assert.equal(tree.some(node => node.type === 'field'), false);
    tree.find(node => node.type === 'switch').props.onCheckedChange(true);
    tree.find(node => node.type === 'password').props.onChange({currentTarget: {value: 'a-strong-new-owner-password'}});
    tree = nodes(f.render(props));tree.find(node => node.type === 'form').props.onSubmit({preventDefault() {}});
    assert.deepEqual(calls, [['a-strong-new-owner-password', true]]);
  } finally {f.restore();}
});

test('reset-file display uses the actual returned directory and never infers an install path', async () => {
  const f = await surface();
  try {
    assert.equal(f.m.resetFilePath({systemKind: 'linux', fileDirectory: '/actual/custom/location', fileName: 'reset-code.txt'}), '/actual/custom/location/reset-code.txt');
    assert.equal(f.m.resetFilePath({systemKind: 'windows', fileDirectory: 'D:\\Actual\\State\\', fileName: 'reset-code.txt'}), 'D:\\Actual\\State\\reset-code.txt');
  } finally {f.restore();}
});

// The disclosure documents the approved install-guide paths; it is not the
// actual server path (that always comes from ResetFileLocation).
test('reset disclosure preserves the exact Windows path without doubled separators', () => {
  assert.equal(defaultI18n.t('accounts.recovery.windowsPath'), String.raw`C:\ProgramData\Portico Media Server\reset-code.txt`);
});
