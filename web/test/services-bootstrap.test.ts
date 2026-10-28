import assert from 'node:assert/strict';
import {test} from 'node:test';
import {ServicesBootstrap} from '../src/services/bootstrap.ts';
import {hostedServicesPage} from '../src/services/mode.ts';
import type {ServicesAccount} from '../src/services/session.ts';
import type {ServicesLinkStore} from '../src/services/intake.ts';

test('web.getportico.tv starts Services only for account pages and the signed-out front door', async () => {
  const none = async () => undefined, chosen = async () => ({serverId: 'a'}), broken = async () => {throw new Error('locked');};
  const page = (pathname: string, search = '', hash = '') => ({pathname, search, hash});
  for (const path of ['/account', '/device', '/reset', '/reset-password', '/verify-email', '/claim', '/join', '/sign-in', '/app/account-return'])
    assert.equal(await hostedServicesPage(page(path), chosen), true, path);
  for (const path of ['/servers', '/settings', '/item/x', '/profiles', '/library/a'])
    assert.equal(await hostedServicesPage(page(path), none), false, path);
  assert.equal(await hostedServicesPage(page('/'), none), true);
  assert.equal(await hostedServicesPage(page('/'), broken), true);
  assert.equal(await hostedServicesPage(page('/'), chosen), false);
  assert.equal(await hostedServicesPage(page('/', '', '#signIn=abc'), chosen), true);
  assert.equal(await hostedServicesPage(page('/', '', '#join'), chosen), true);
  assert.equal(await hostedServicesPage(page('/', '?mode=forgot'), chosen), true);
  assert.equal(await hostedServicesPage(page('/', '?identityError=google'), chosen), true);
  assert.equal(await hostedServicesPage(page('/', '', '#other'), chosen), false);
});

function ports(signedIn = false) {
  const calls: string[] = [];
  const account = {service: {
    synchronize: async () => { calls.push('cookie-synchronize'); },
    getSnapshot: () => ({session: signedIn ? {account: {id: 'current'}, familyId: 'family'} : undefined}),
    accessSession: async () => { calls.push('cookie-renew'); },
  }} as unknown as Pick<ServicesAccount, 'service'>;
  const store: ServicesLinkStore = {read: async () => { calls.push('protected-read'); return undefined; }, change: async () => { calls.push('protected-write'); }};
  return {account, store, calls};
}

test('signed-out startup restores only the protected link and canonical cookie coordinator', async () => {
  const p = ports(), boot = new ServicesBootstrap(p.account, () => p.store);
  await boot.start();
  assert.deepEqual(p.calls, ['protected-read', 'cookie-synchronize']);
  assert.equal(boot.getSnapshot().phase, 'ready'); boot.dispose();
});

test('a captured link is saved before cookie work and existing sessions renew through the singleton', async () => {
  const p = ports(true), link = {version: 1, kind: 'device', value: '7KQM-4TXP', createdAt: Date.now()} as const;
  const boot = new ServicesBootstrap(p.account, () => p.store, {kind: 'device', link});
  await boot.start();
  assert.deepEqual(p.calls, ['protected-write', 'cookie-synchronize', 'cookie-renew']);
  assert.deepEqual(boot.getSnapshot().link, link); boot.dispose();
});

test('a protected-store failure cannot fall back to plaintext or start account effects', async () => {
  const p = ports(), failure = new Error('storage unavailable');
  const boot = new ServicesBootstrap(p.account, () => { throw failure; });
  await boot.start();
  assert.equal(boot.getSnapshot().phase, 'error'); assert.equal(boot.getSnapshot().error, failure);
  assert.deepEqual(p.calls, []); boot.dispose();
});

test('disposed startup cannot renew or publish a late restored continuation', async () => {
  const p = ports(true); let resolve!: () => void;
  p.store.read = () => new Promise(done => { resolve = () => done(undefined); });
  const boot = new ServicesBootstrap(p.account, () => p.store), flight = boot.start();
  const before = boot.getSnapshot(); boot.dispose(); resolve(); await flight;
  assert.equal(boot.getSnapshot(), before); assert.deepEqual(p.calls, []);
});
