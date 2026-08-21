import { afterEach, describe, expect, it } from 'vitest';
import { hostedCSRFToken, HOSTED_BROWSER_INSTALLATION_IDENTITY_POLICY, rememberHostedCSRFToken } from './hostedBrowserSecurity';

afterEach(() => {
  rememberHostedCSRFToken('');
  document.cookie = 'portico_hosted_csrf=; Max-Age=0; Path=/';
});

describe('hosted browser security', () => {
  it('makes installation identity retention and reset semantics explicit', () => {
    expect(HOSTED_BROWSER_INSTALLATION_IDENTITY_POLICY).toEqual({
      purpose: 'bind-browser-installation-to-server-session-and-profile-trust',
      storage: 'origin-scoped-indexeddb-metadata',
      retention: 'retain-across-sign-out-and-account-removal',
      reset: 'requires-server-revocation-and-rebind',
    });
  });

  it('moves from the rolling-upgrade cookie to API-origin request verification in memory', () => {
    document.cookie = 'portico_hosted_csrf=legacy%2Etoken; Path=/';
    expect(hostedCSRFToken()).toBe('legacy.token');
    rememberHostedCSRFToken(' token.value ');
    expect(hostedCSRFToken()).toBe('token.value');
  });
});
