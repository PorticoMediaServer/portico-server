import { afterEach, describe, expect, it } from 'vitest';
import {
  LOCAL_SESSION_QUARANTINE_KEY,
  clearLocalSessionAfterVerifiedAuthentication,
  localSessionRestoreStatus,
  markLocalSessionSignedOut,
} from './localSessionQuarantine';

const scope = {
  authority: 'local' as const,
  accountId: 'account-1',
  serverId: 'server-1',
  profileId: 'profile-1',
  authorizationRevision: 'policy-1',
};

afterEach(() => window.localStorage.clear());

describe('local session quarantine', () => {
  it('read-verifies a durable sign-out marker and clears it only for verified Local Auth', () => {
    expect(markLocalSessionSignedOut(scope)).toBe(true);
    expect(localSessionRestoreStatus()).toMatchObject({
      trustedForRestore: true,
      quarantined: true,
    });
    expect(clearLocalSessionAfterVerifiedAuthentication({...scope, authority: 'hosted'})).toBe(false);
    expect(localSessionRestoreStatus().quarantined).toBe(true);
    expect(clearLocalSessionAfterVerifiedAuthentication(scope)).toBe(true);
    expect(localSessionRestoreStatus()).toEqual({trustedForRestore: true, quarantined: false});
  });

  it('fails restore closed for malformed durable state', () => {
    window.localStorage.setItem(LOCAL_SESSION_QUARANTINE_KEY, '{not-json');
    expect(localSessionRestoreStatus()).toEqual({trustedForRestore: false, quarantined: true});
  });
});
