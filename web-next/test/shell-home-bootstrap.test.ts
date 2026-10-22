import test from 'node:test';
import assert from 'node:assert/strict';
import {hostedShellProfile, ShellBootstrap, sameShellAuthority, type ShellAssembler} from '../src/shell/bootstrap.ts';
import {CredentialSessionService, type CredentialRecord, type HostedSession, type HttpLocalApi, type LocalSession} from '@core/index.ts';
import type {SelectedViewerRecord} from '@core/viewer-selection.ts';
import {homeFixtures as f} from '../../packages/i18n/src/catalog/en-US/next/dev/home-fixtures.ts';

const unavailableAssembly: ShellAssembler = async () => { throw new Error('Unexpected assembly.'); };
const selected = {session: {sessionFamilyId: 'family-a'}} as SelectedViewerRecord;
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(done => { resolve = done; }); return {promise, resolve}; }

test('an absent protected selection reaches the registered signed-out flow without reading a server', async () => {
  let reads = 0;
  const bootstrap = new ShellBootstrap(unavailableAssembly, {selected: {read: async () => null}, restore: async () => { reads++; return undefined; }});
  await bootstrap.start();
  assert.equal(bootstrap.getSnapshot().phase, 'signed-out');
  assert.equal(reads, 0);
  bootstrap.dispose();
});

test('retiring while the vault is opening prevents a later selection from restoring authority', async () => {
  const vault = deferred<SelectedViewerRecord | null>(); let reads = 0;
  const bootstrap = new ShellBootstrap(unavailableAssembly, {selected: {read: () => vault.promise}, restore: async () => { reads++; return undefined; }});
  const work = bootstrap.start(); bootstrap.dispose(); vault.resolve(selected); await work;
  assert.equal(bootstrap.getSnapshot().phase, 'signed-out'); assert.equal(reads, 0);
});

test('a route restored after retirement is disposed and never assembled', async () => {
  const route = deferred<{api: HttpLocalApi; session: LocalSession} | undefined>(); let disposed = 0, assembling = 0;
  const bootstrap = new ShellBootstrap(async context => { assembling++; return unavailableAssembly(context); }, {
    selected: {read: async () => selected}, restore: () => route.promise,
  });
  const work = bootstrap.start(); await Promise.resolve(); bootstrap.dispose();
  route.resolve({api: {getRouteConnection: () => ({dispose: () => { disposed++; }})} as unknown as HttpLocalApi, session: selected.session}); await work;
  assert.equal(disposed, 1); assert.equal(assembling, 0); assert.equal(bootstrap.getSnapshot().phase, 'signed-out');
});

test('a failed protected read exposes retry state and a later explicit retry can restore signed-out state', async () => {
  let attempt = 0, observed = 0;
  const bootstrap = new ShellBootstrap(unavailableAssembly, {selected: {read: async () => { if (attempt++ === 0) throw new Error('vault_busy'); return null; }}, restore: async () => undefined});
  const off = bootstrap.subscribe(() => { observed++; });
  await bootstrap.start(); assert.equal(bootstrap.getSnapshot().phase, 'error');
  await bootstrap.start(); assert.equal(bootstrap.getSnapshot().phase, 'signed-out'); assert.ok(observed >= 4);
  off(); const before = observed; bootstrap.dispose(); assert.equal(observed, before);
});

test('token rotation retains a scope but every authority or family replacement retires it', () => {
  const session = {sessionFamilyId: 'family-a', accessToken: 'test-token-a', viewer: {serverId: 'server-a', accountId: 'account-a', profileId: 'profile-a', authority: 'local', role: 'owner'}} as LocalSession;
  assert.equal(sameShellAuthority(session, {...session, accessToken: 'test-token-b'}), true);
  assert.equal(sameShellAuthority(session, {...session, sessionFamilyId: 'family-b'}), false);
  for (const key of ['serverId', 'accountId', 'profileId', 'authority', 'role'] as const) {
    assert.equal(sameShellAuthority(session, {...session, viewer: {...session.viewer, [key]: 'replacement'}} as LocalSession), false, key);
  }
});

function hostedRecord(accountId = 'account', profileId = 'profile'): CredentialRecord<{familyId: string}> {
  const session: HostedSession = {accessToken: '', familyId: 'family', expiresAt: '2026-10-22T12:00:00Z', refreshExpiresAt: '2027-01-20T12:00:00Z',
    account: {id: accountId, username: f.account, displayName: f.account}, profiles: [{id: profileId, name: f.profile}]};
  return {version: 1, revocations: [], active: {session, authority: {familyId: 'family'}, context: {}}};
}
const unexpectedAccountApi = {
  authorityFromSession: () => { throw new Error('Unexpected authentication.'); },
  refresh: async (): Promise<never> => { throw new Error('Unexpected account renewal.'); },
  revoke: async () => { throw new Error('Unexpected account revocation.'); },
};
test('hosted profile restore waits for public synchronization and projects the current selected account without renewing it', async () => {
  const opening = deferred<CredentialRecord<{familyId: string}> | undefined>();
  let reads = 0, synchronized = hostedRecord();
  const service = new CredentialSessionService(unexpectedAccountApi, {load: async () => ++reads === 1 ? opening.promise : synchronized,
    save: async () => { throw new Error('Unexpected account write.'); }});
  const profile = hostedShellProfile(service, {accountId: 'account', profileId: 'profile'});
  assert.equal(service.getSnapshot().phase, 'restoring'); assert.equal(reads, 1);
  // A different tab replaced the initial metadata before the coordinator reload.
  opening.resolve(hostedRecord('old-account'));
  const restored = await profile;
  assert.equal(reads, 2); assert.equal(restored.profileName, f.profile);
  assert.deepEqual(restored.profiles, [{id: 'profile', name: f.profile, initial: Array.from(f.profile)[0]}]);
  assert.deepEqual(restored.accountFacts, [{id: 'account', text: f.account, role: 'identity'}]);
  assert.equal(restored.canManageProfiles, false);
  synchronized = hostedRecord('replacement-account');
  await assert.rejects(hostedShellProfile(service, {accountId: 'account', profileId: 'profile'}), /profile is unavailable/);
  synchronized = hostedRecord('account', 'replacement-profile');
  await assert.rejects(hostedShellProfile(service, {accountId: 'account', profileId: 'profile'}), /profile is unavailable/);
});
test('hosted synchronization failure or a signed-out snapshot cannot supply profile display authority', async () => {
  const failure = new Error('protected account state unavailable');
  const damaged = new CredentialSessionService(unexpectedAccountApi, {load: async () => { throw failure; }, save: async () => {}});
  await assert.rejects(hostedShellProfile(damaged, {accountId: 'account', profileId: 'profile'}), error => error === failure);
  const signedOut = new CredentialSessionService(unexpectedAccountApi, {load: async () => undefined, save: async () => {}});
  await assert.rejects(hostedShellProfile(signedOut, {accountId: 'account', profileId: 'profile'}), /profile is unavailable/);
});
