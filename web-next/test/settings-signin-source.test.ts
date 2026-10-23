import test from 'node:test';
import assert from 'node:assert/strict';
import {SignInSettingsSource} from '../src/screens/settings/signin-source.ts';

const capabilities = {manageAccount: true, managePeople: true, manageServer: true, manageSignIn: true};
const document = (signIn: 'local' | 'portico') => ({signIn, owner: {accountId: 'account-actual', username: 'actual-owner', displayName: 'Actual owner', ...(signIn === 'portico' ? {email: 'owner@example.org'} : {})}, move: null, webPath: '/settings?scope=server&section=signin'});
test('Sign-in facts are inert until read and use actual owner data, with no invented Local email line', async () => {
  let requests = 0;
  const source = new SignInSettingsSource({request: async <T>() => {requests++;return document('local') as T;}}, {serverId: 'server', signIn: 'local', isCurrent: () => true, capabilities: () => capabilities, onAuthorityChanged() {}});
  assert.equal(requests, 0);
  assert.deepEqual(source.rowState('signin.methodLocal'), {present: false});
  await source.load();
  assert.equal(source.rowState('signin.methodLocal')?.displayValue, 'Local accounts');
  assert.equal(source.rowState('signin.ownerLocal')?.displayValue, 'actual-owner');
  assert.equal(source.rowState('signin.ownerLocal')?.help, undefined);
  assert.deepEqual(source.rowState('signin.method'), {present: false});
  assert.equal(source.rowState('signin.switchToPortico'), undefined);
  await source.load();assert.equal(requests, 1);source.dispose();
});
test('Portico owner details use the returned username and email and disappear synchronously with capability loss', async () => {
  let manageSignIn = true;
  const source = new SignInSettingsSource({request: async <T>() => document('portico') as T}, {serverId: 'server', signIn: 'portico', isCurrent: () => true, capabilities: () => ({...capabilities, manageSignIn}), onAuthorityChanged() {}});
  await source.load();
  assert.equal(source.rowState('signin.owner')?.displayValue, 'actual-owner');
  assert.equal(source.rowState('signin.owner')?.help, 'Portico Account owner@example.org');
  manageSignIn = false;
  assert.equal(source.getSnapshot().document, undefined);
  assert.deepEqual(source.rowState('signin.owner'), {present: false});source.dispose();
});
test('a changed kind or authority refusal erases facts before asking the parent to reread authority', async () => {
  for (const result of [document('portico'), Object.assign(new Error('forbidden'), {status: 403})]) {
    let refreshed = 0;
    const source = new SignInSettingsSource({request: async <T>() => {if (result instanceof Error) throw result;return result as T;}}, {serverId: 'server', signIn: 'local', isCurrent: () => true, capabilities: () => capabilities, onAuthorityChanged() {refreshed++;assert.equal(source.getSnapshot().document, undefined);}});
    await source.load();assert.equal(refreshed, 1);assert.equal(source.getSnapshot().document, undefined);
  }
});
test('disposal aborts the guarded read and its late reply cannot publish private owner facts', async () => {
  let finish!: (value: unknown) => void, signal: AbortSignal | undefined, notifications = 0;
  const source = new SignInSettingsSource({request: <T>(_path: string, _method?: string, _body?: unknown, next?: AbortSignal) => {signal = next;return new Promise(resolve => {finish = resolve;}) as Promise<T>;}}, {serverId: 'server', signIn: 'local', isCurrent: () => true, capabilities: () => capabilities, onAuthorityChanged() {}});
  source.subscribe(() => {notifications++;});
  const loading = source.load();source.dispose();const before = notifications;
  assert.equal(signal?.aborted, true);finish(document('local'));await loading;
  assert.equal(notifications, before);assert.equal(source.getSnapshot().document, undefined);
});
