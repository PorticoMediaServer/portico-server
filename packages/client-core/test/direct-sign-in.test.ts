import {test} from 'node:test';
import assert from 'node:assert/strict';
import {completeDirectSignInChallenge, directSignIn, parseDirectSignIn} from '../src/profile-management.ts';

// The exact documented direct sign-in shapes (`DirectSignIn` and `Envelope`). No accountToken/accountExpiresAt; one device-bound
// session that names its device and installation.
const profile = (id: string, over: Record<string, unknown> = {}) => ({id, name: id, primary: id === 'p1', position: id === 'p1' ? 0 : 1, art: 'blue', revision: 1, pinRevision: 1, pinRequired: false, allowedLibraries: [], ...over});
const snapshot = (profiles: unknown[]) => ({authority: 'local', serverId: 'srv', account: {id: 'acct', username: 'owner', primaryProfileId: 'p1', role: 'owner', revision: 1, disabled: false, allowedLibraries: []}, profiles, canManage: true, deviceId: 'dev', installationId: 'inst'});
const session = (role: string) => ({accessToken: 'a'.repeat(43), refreshToken: 'r'.repeat(43), deviceId: 'dev', installationId: 'inst', expiresAt: '2099-01-01T00:00:00Z', viewer: {accountId: 'acct', profileId: 'p1', serverId: 'srv', authority: 'local', role}, sessionFamilyId: 'fam', tokenGeneration: '1', authorizationHorizon: '2099-03-01T00:00:00Z'});

test('several profiles: an account-scoped session, and a profile is chosen next', () => {
  const r = parseDirectSignIn({...snapshot([profile('p1'), profile('p2')]), session: session('account')});
  assert.equal(r.kind, 'signed-in');
  if (r.kind !== 'signed-in') return;
  assert.equal(r.accountScoped, true);
  assert.equal(r.passwordChangeRequired, false);
  assert.equal(r.session.accessToken, 'a'.repeat(43));
  assert.equal(r.session.installationId, 'inst');
  assert.equal(r.session.deviceId, 'dev');
  assert.equal(r.snapshot.profiles.length, 2);
});

test('one unprotected profile: the session is the viewer', () => {
  const r = parseDirectSignIn({...snapshot([profile('p1')]), session: session('owner')});
  assert.ok(r.kind === 'signed-in' && !r.accountScoped);
});

test('a viewer session is refused where a profile must be chosen or a password changed', () => {
  assert.throws(() => parseDirectSignIn({...snapshot([profile('p1'), profile('p2')]), session: session('owner')}));
  assert.throws(() => parseDirectSignIn({...snapshot([profile('p1', {pinRequired: true})]), session: session('owner')}));
  assert.throws(() => parseDirectSignIn({...snapshot([profile('p1')]), passwordChangeRequired: true, session: session('owner')}));
  const forced = parseDirectSignIn({...snapshot([profile('p1')]), passwordChangeRequired: true, session: session('account')});
  assert.ok(forced.kind === 'signed-in' && forced.passwordChangeRequired && forced.accountScoped);
});

test('the old account-token shape is no longer accepted (a session is required)', () => {
  assert.throws(() => parseDirectSignIn({...snapshot([profile('p1'), profile('p2')]), accountToken: 't'.repeat(43), accountExpiresAt: '2099-03-01T00:00:00Z'}));
});

test('CD-34: direct admin profile management keeps owner/admin/member', () => {
  for (const role of ['owner', 'admin', 'member']) {
    const r = parseDirectSignIn({...snapshot([profile('p1')]), account: {...(snapshot([profile('p1')] as any) as any).account, role}, session: session('owner')});
    assert.equal(r.kind, 'signed-in');
    if (r.kind !== 'signed-in') continue;
    assert.equal(r.snapshot.account.role, role);
  }
  assert.throws(() => parseDirectSignIn({...snapshot([profile('p1')]), account: {...(snapshot([profile('p1')] as any) as any).account, role: 'viewer'}, session: session('owner')}));
});

test('two-step sign-in returns a challenge, finished with a code', async () => {
  const r = parseDirectSignIn({challenge: {token: 'c'.repeat(43), expiresAt: '2099-01-01T00:00:00Z'}});
  assert.deepEqual(r, {kind: 'challenge', token: 'c'.repeat(43), expiresAt: '2099-01-01T00:00:00Z'});
  const calls: unknown[] = [];
  const api = {request: async <T,>(path: string, method?: string, body?: unknown): Promise<T> => { calls.push([path, method, body]); return {...snapshot([profile('p1'), profile('p2')]), session: session('account')} as T; }};
  const done = await completeDirectSignInChallenge(api, 'c'.repeat(43), ' 123456 ');
  assert.equal(done.kind, 'signed-in');
  assert.deepEqual(calls, [['/v1/auth/two-factor/challenge', 'POST', {token: 'c'.repeat(43), code: '123456'}]]);
  await assert.rejects(completeDirectSignInChallenge(api, 'c'.repeat(43), ''));
});

test('a device waiting for approval gets no session', async () => {
  assert.deepEqual(parseDirectSignIn({deviceApprovalPending: true, deviceId: 'dev', installationId: 'inst'}), {kind: 'device-pending', deviceId: 'dev', installationId: 'inst'});
  assert.throws(() => parseDirectSignIn({deviceApprovalPending: true, deviceId: 'dev', installationId: 'inst', session: session('account')}));
  const api = {request: async <T,>(): Promise<T> => ({deviceApprovalPending: true, deviceId: 'dev', installationId: 'inst'}) as T};
  assert.equal((await directSignIn(api, 'sam', 'pw')).kind, 'device-pending');
});
