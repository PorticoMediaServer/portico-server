import type { ViewerScope } from '@porticomediaserver/client-core';
import { beforeEach, expect, it } from 'vitest';
import {
  AMBIENT_COOKIE_QUARANTINE_KEY,
  ambientCookieRestoreStatus,
  beginAmbientCookieMutation,
  bindAmbientCookieMutationToViewer,
  claimAmbientCookieMutation,
  clearAmbientCookieAfterVerifiedAuthentication,
  ownsAmbientCookieMutation,
  releaseAmbientCookieMutationReservation,
  reserveAmbientCookieMutation,
} from './ambientCookieQuarantine';

const hostedScope: ViewerScope = {
  authority: 'hosted',
  accountId: 'hosted-account',
  serverId: 'server-one',
  profileId: 'profile-one',
  authorizationRevision: 'revision-one',
};

beforeEach(() => window.localStorage.clear());

it('blocks restart for Hosted and Local ambient-cookie mutations alike', () => {
  const hosted = beginAmbientCookieMutation('browser-account-switch', {
    state: 'authenticated',
    expected: { authority: 'hosted', accountId: 'hosted-account' },
  });
  expect(hosted).toBeDefined();
  expect(ambientCookieRestoreStatus()).toMatchObject({
    trustedForRestore: true,
    quarantined: true,
    marker: { mutationKind: 'browser-account-switch' },
  });

  const local = beginAmbientCookieMutation('profile-session', {
    state: 'authenticated',
    expected: { authority: 'local', accountId: 'local-account', serverId: 'local-server', profileId: 'local-profile' },
  });
  expect(local).toBeDefined();
  expect(ambientCookieRestoreStatus().marker).toMatchObject({
    mutationKind: 'profile-session',
    intent: { state: 'authenticated', expected: { authority: 'local' } },
  });
});

it('clears only after the owning marker is narrowed and final /me matches exactly', () => {
  const marker = beginAmbientCookieMutation('browser-account-switch', {
    state: 'authenticated',
    expected: { authority: 'hosted', accountId: 'hosted-account' },
  });
  expect(marker).toBeDefined();
  const exact = bindAmbientCookieMutationToViewer(marker!, hostedScope);
  expect(exact).toBeDefined();
  expect(clearAmbientCookieAfterVerifiedAuthentication(exact!, {
    ...hostedScope,
    profileId: 'wrong-profile',
  })).toBe(false);
  expect(ambientCookieRestoreStatus().quarantined).toBe(true);
  expect(clearAmbientCookieAfterVerifiedAuthentication(exact!, hostedScope)).toBe(true);
  expect(ambientCookieRestoreStatus()).toEqual({ trustedForRestore: true, quarantined: false });
});

it('never clears an intentional signed-out marker', () => {
  const marker = beginAmbientCookieMutation('browser-account-sign-out-all', { state: 'signed-out' });
  expect(marker).toBeDefined();
  expect(clearAmbientCookieAfterVerifiedAuthentication(marker!, hostedScope)).toBe(false);
  expect(ambientCookieRestoreStatus().quarantined).toBe(true);
});

it('does not let an older mutation narrow or clear a newer marker', () => {
  const older = beginAmbientCookieMutation('browser-account-switch', {
    state: 'authenticated',
    expected: {authority: 'hosted', accountId: 'hosted-account'},
  });
  expect(older).toBeDefined();
  const newer = beginAmbientCookieMutation('profile-session', {
    state: 'authenticated',
    expected: {authority: 'local', accountId: 'local-account', serverId: 'local-server', profileId: 'local-profile'},
  });
  expect(newer).toBeDefined();

  expect(ownsAmbientCookieMutation(older!)).toBe(false);
  expect(bindAmbientCookieMutationToViewer(older!, hostedScope)).toBeUndefined();
  expect(clearAmbientCookieAfterVerifiedAuthentication({...older!, intent: {state: 'authenticated', expected: hostedScope}}, hostedScope)).toBe(false);
  expect(ownsAmbientCookieMutation(newer!)).toBe(true);
  expect(ambientCookieRestoreStatus().marker?.mutationId).toBe(newer!.mutationId);
});

it('keeps cross-context reservations independent while the newest lock owner claims quarantine', () => {
  const older = reserveAmbientCookieMutation('browser-account-switch', {
    state: 'authenticated',
    expected: {authority: 'hosted', accountId: 'hosted-account'},
  });
  expect(older).toBeDefined();
  expect(claimAmbientCookieMutation(older!)).toBe(true);
  const newer = reserveAmbientCookieMutation('profile-session', {
    state: 'authenticated',
    expected: {authority: 'local', accountId: 'local-account', serverId: 'local-server', profileId: 'local-profile'},
  });
  expect(newer).toBeDefined();

  expect(ownsAmbientCookieMutation(older!)).toBe(false);
  expect(bindAmbientCookieMutationToViewer(older!, hostedScope)).toBeUndefined();
  expect(clearAmbientCookieAfterVerifiedAuthentication({...older!, intent: {state: 'authenticated', expected: hostedScope}}, hostedScope)).toBe(false);
  expect(releaseAmbientCookieMutationReservation(older!)).toBe(true);
  expect(claimAmbientCookieMutation(newer!)).toBe(true);
  expect(ownsAmbientCookieMutation(newer!)).toBe(true);
});

it('treats malformed durable state as untrusted', () => {
  window.localStorage.setItem(AMBIENT_COOKIE_QUARANTINE_KEY, '{not-json');
  expect(ambientCookieRestoreStatus()).toEqual({ trustedForRestore: false, quarantined: true });
});
