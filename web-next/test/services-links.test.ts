import assert from 'node:assert/strict';
import {test} from 'node:test';
import {takeLinkFragment, verificationLink} from '../src/services/fragments.ts';
import {currentAccountWork} from '../src/services/session.ts';
import {resetPassword, verifyEmail} from '../src/services/link-effects.ts';
import type {ServicesAccount} from '../src/services/session.ts';

test('link secrets are scrubbed synchronously, including duplicate, unexpected and malformed fragments', () => {
  for (const hash of ['#token=abc_123', '#token=a&token=b', '#token=a&return=https://bad.example', '#token=%00', '#unknown=a']) {
    const calls: unknown[] = [];
    const result = takeLinkFragment({hash, pathname: '/reset', search: ''}, {state: {keep: true}, replaceState: (...args) => { calls.push(args); }});
    assert.deepEqual(calls, [[{keep: true}, '', '/reset']]);
    assert.deepEqual(result, hash === '#token=abc_123' ? {token: 'abc_123'} : {});
  }
  assert.deepEqual(takeLinkFragment({hash: '#token=abc', pathname: '/reset', search: '?token=secret'}, {state: null, replaceState() {}}), {});
});

const token = (kind = 'registration', code = '482917') => btoa(JSON.stringify(['1', kind, 'registration_123', code])).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
test('verification tuple accepts the two current kinds and refuses retired provider mail and unbounded input', () => {
  assert.deepEqual(verificationLink(token()), {kind: 'registration', resource: 'registration_123', code: '482917'});
  assert.equal(verificationLink(token('oidc_contact')), undefined);
  assert.equal(verificationLink(token('registration', '12345')), undefined);
  assert.equal(verificationLink('a'.repeat(2049)), undefined);
});

function accountPort() {
  let session = {account: {id: 'account_a', username: 'account', displayName: 'Account'}, familyId: 'family_a', accessToken: 'access'};
  const calls: unknown[] = [];
  const retired: unknown[] = [];
  let adopted = false;
  const account = {
    client: {
      request: async (...args: unknown[]) => { calls.push(args); return {kind: 'registration', status: 'confirmed'}; },
      registration: async () => null,
      verifyRegistration: async () => session,
    },
    service: {
      synchronize: async () => {},
      accessSession: async () => session,
      getSnapshot: () => ({session}),
      authenticateIfSignedOut: async (work: (signal: AbortSignal) => Promise<unknown>) => { adopted = true; return work(new AbortController().signal); },
      logoutIfCurrent: async (expected: unknown) => { retired.push(expected); return true; },
    },
  } as unknown as ServicesAccount;
  return {account, calls, retired, adopted: () => adopted, change: () => { session = {...session, familyId: 'family_b'}; }};
}

test('account work rejects a changed family even when the account id is unchanged', async () => {
  const port = accountPort();
  await assert.rejects(currentAccountWork(port.account, async () => { port.change(); return 'stale'; }), {code: 'account_changed'});
  await assert.rejects(currentAccountWork(port.account, async () => 'unused', {accountId: 'account_a', familyId: 'family_a'}), {code: 'account_changed'});
});

test('password reset uses the unauthenticated recovery endpoint and never adopts a session', async () => {
  const port = accountPort();
  await resetPassword(port.account, 'token', 'a long new password', ' 482917 ', new AbortController().signal);
  assert.equal(port.calls.length, 1);
  assert.deepEqual(port.calls[0].slice(0, 3), ['/v1/account/recovery/complete', 'POST', {token: 'token', newPassword: 'a long new password', factor: '482917'}]);
  assert.equal(port.adopted(), false);
});

test('a link on another device confirms the address without creating a session', async () => {
  const port = accountPort();
  assert.equal(await verifyEmail(port.account, token(), new AbortController().signal), 'elsewhere');
  assert.equal(port.adopted(), false);
  assert.equal(port.calls[0][0], '/v1/email-links/confirm');
});

test('only a matching registration can finish its browser session', async () => {
  const port = accountPort();
  port.account.client.registration = async () => ({registrationId: 'registration_123'} as never);
  assert.equal(await verifyEmail(port.account, token(), new AbortController().signal), 'confirmed');
  assert.equal(port.adopted(), true);
  assert.equal(port.calls.length, 0);
});

test('an email change retires only the reviewed family through the canonical coordinator', async () => {
  const port = accountPort();
  assert.equal(await verifyEmail(port.account, token('email_change'), new AbortController().signal), 'changed');
  assert.equal(port.calls[0][0], '/v1/account/security/email/confirm');
  assert.deepEqual(port.retired, [{accountId: 'account_a', familyId: 'family_a'}]);
  assert.equal(port.adopted(), false);
});

test('an email confirmation cannot sign out a family that replaced the initiating account', async () => {
  const port = accountPort();
  port.account.client.request = async () => { port.change(); return {} as never; };
  await assert.rejects(verifyEmail(port.account, token('email_change'), new AbortController().signal), {code: 'account_changed'});
  assert.deepEqual(port.retired, []);
});
