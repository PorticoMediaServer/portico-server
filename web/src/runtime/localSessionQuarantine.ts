import type {ViewerScope} from '@porticomediaserver/client-core';
import {
  LEGACY_LOCAL_SESSION_QUARANTINE_KEY,
  ambientCookieRestoreStatus,
  beginAmbientCookieMutation,
  bindAmbientCookieMutationToViewer,
  clearAmbientCookieAfterVerifiedAuthentication,
} from './ambientCookieQuarantine';

// This barrier protects local session restore and is deliberately separate
// from the durable Hosted installation identity. Sign-out and verified local
// reauthentication may change the barrier state, but neither operation may
// silently regenerate or clear the browser installation binding.
export const LOCAL_SESSION_QUARANTINE_KEY = LEGACY_LOCAL_SESSION_QUARANTINE_KEY;

type LocalSessionQuarantineMarker = {
  version: 1;
  authority: 'local';
  accountId: string;
  serverId: string;
  signedOutAt: string;
};

export type LocalSessionRestoreStatus = {
  trustedForRestore: boolean;
  quarantined: boolean;
  marker?: LocalSessionQuarantineMarker;
};

export type LocalSessionQuarantineScope = Pick<ViewerScope, 'authority' | 'accountId' | 'serverId'>
  & Partial<Pick<ViewerScope, 'profileId' | 'authorizationRevision'>>;

export function localSessionRestoreStatus(): LocalSessionRestoreStatus {
  const status = ambientCookieRestoreStatus();
  return { trustedForRestore: status.trustedForRestore, quarantined: status.quarantined };
}

/** Publishes and read-verifies the durable barrier before any sign-out await. */
export function markLocalSessionSignedOut(scope: LocalSessionQuarantineScope): boolean {
  if (scope.authority !== 'local' || !scope.serverId) return false;
  return Boolean(beginAmbientCookieMutation('logout', {state: 'signed-out'}));
}

/** Clears the barrier only after an explicit login has produced a verified local scope. */
export function clearLocalSessionAfterVerifiedAuthentication(scope: ViewerScope): boolean {
  if (scope.authority !== 'local' || !scope.serverId) return false;
  const status = ambientCookieRestoreStatus();
  const existing = status.marker;
  if (existing?.intent.state === 'authenticated') {
    return clearAmbientCookieAfterVerifiedAuthentication(existing, scope);
  }
  const marker = beginAmbientCookieMutation('profile-session', {
    state: 'authenticated',
    expected: scope,
  });
  const exact = marker ? bindAmbientCookieMutationToViewer(marker, scope) : undefined;
  return Boolean(exact && clearAmbientCookieAfterVerifiedAuthentication(exact, scope));
}
