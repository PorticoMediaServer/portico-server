import assert from 'node:assert/strict';
import {test} from 'node:test';
import {ApiError} from '@core/index.ts';
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

type PortSession = {account: {id: string; username: string; displayName: string}; familyId: string; accessToken: string};

/** The canonical CredentialSessionService contract: a browser is either signed out
 * (phase signedOut, no session) or signed in (phase ready, one family); subscribe
 * publishes every transition; only the coordinator admits a signed-out opening; and
 * logoutIfCurrent retires just the family it is handed, if that family is still
 * current. Tests that exercise the signed-out signup opening ask for it explicitly. */
function accountPort({signedIn = true}: {signedIn?: boolean} = {}) {
  const family = (familyId: string): PortSession => ({account: {id: 'account_a', username: 'account', displayName: 'Account'}, familyId, accessToken: 'access'});
  let session: PortSession | undefined = signedIn ? family('family_a') : undefined;
  let phase: 'signedOut' | 'ready' = signedIn ? 'ready' : 'signedOut';
  const listeners = new Set<() => void>();
  const publish = () => {for (const listener of [...listeners]) listener();};
  const calls: unknown[] = [];
  const retired: unknown[] = [];
  let adopted = false;
  const account = {
    client: {
      request: async (...args: unknown[]) => {
        calls.push(args);
        const body = args[2] as {token?: unknown} | undefined;
        // The public confirm capability answers the shape its mailed token proves.
        const kind = typeof body?.token === 'string' ? verificationLink(body.token)?.kind : undefined;
        return kind === 'email_change'
          ? {kind: 'email_change', status: 'email_changed', accountId: session?.account.id ?? 'account_a'}
          : {kind: 'registration', status: 'confirmed'};
      },
      registration: async () => null,
      verifyRegistration: async () => family('family_a'),
    },
    service: {
      synchronize: async () => {},
      accessSession: async () => {
        if (!session) throw new ApiError(401, 'account_signed_out', 'account_signed_out', false);
        return session;
      },
      getSnapshot: () => ({phase, session, pendingRevocations: 0}),
      subscribe: (listener: () => void) => {
        listeners.add(listener);
        return () => {listeners.delete(listener);};
      },
      authenticateIfSignedOut: async (work: (signal: AbortSignal) => Promise<PortSession>, _context: unknown = {},
        callerCurrent: () => boolean = () => true) => {
        if (session) throw new ApiError(409, 'account_changed', 'account_changed', false);
        if (!callerCurrent()) throw Object.assign(new Error('cancelled'), {code: 'cancelled'});
        adopted = true;
        session = await work(new AbortController().signal);
        phase = 'ready';
        publish();
        return session;
      },
      logoutIfCurrent: async (expected: {accountId: string; familyId: string}) => {
        if (!session || session.account.id !== expected.accountId || session.familyId !== expected.familyId) return false;
        session = undefined;
        phase = 'signedOut';
        publish();
        retired.push(expected);
        return true;
      },
    },
  } as unknown as ServicesAccount;
  return {account, calls, retired, adopted: () => adopted, change: () => {
    if (!session) return;
    session = {...session, familyId: 'family_b'};
    publish();
  }};
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
  // The signup completion is admitted only for an opening that began signed out.
  const port = accountPort({signedIn: false});
  port.account.client.registration = async () => ({registrationId: 'registration_123'} as never);
  assert.equal(await verifyEmail(port.account, token(), new AbortController().signal), 'confirmed');
  assert.equal(port.adopted(), true);
  assert.deepEqual(port.calls.map(call => call.slice(0, 2)), [['/v1/email-links/confirm', 'POST']]);
  assert.equal(port.account.service.getSnapshot().session?.familyId, 'family_a');
});

test('an email change retires only the reviewed family through the canonical coordinator', async () => {
  const port = accountPort();
  assert.equal(await verifyEmail(port.account, token('email_change'), new AbortController().signal), 'changed');
  // A mailed email change uses the same public capability as any other link ACK.
  assert.deepEqual(port.calls.map(call => call.slice(0, 2)), [['/v1/email-links/confirm', 'POST']]);
  assert.deepEqual(port.retired, [{accountId: 'account_a', familyId: 'family_a'}]);
  assert.equal(port.adopted(), false);
});

test('an email confirmation cannot sign out a family that replaced the initiating account', async () => {
  const port = accountPort();
  port.account.client.request = async () => {
    port.change();
    return {kind: 'email_change', status: 'email_changed', accountId: 'account_a'};
  };
  // The change that landed first is the current family; the held ACK keeps it.
  assert.equal(await verifyEmail(port.account, token('email_change'), new AbortController().signal), 'changed');
  assert.deepEqual(port.retired, []);
  assert.equal(port.account.service.getSnapshot().session?.familyId, 'family_b');
});
