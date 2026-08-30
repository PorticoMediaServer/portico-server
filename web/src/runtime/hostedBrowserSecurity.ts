let hostedCSRF = '';

/**
 * Contract for the durable installation ID owned by hostedConnectionVault.
 * It binds browser-scoped sessions/profile trust to this origin; it is not a
 * reusable credential. Normal sign-out and account removal revoke account
 * state but retain this binding. A client-only reset is intentionally not
 * supported until a server-backed revoke-and-rebind contract exists.
 */
export const HOSTED_BROWSER_INSTALLATION_IDENTITY_POLICY = {
  purpose: 'bind-browser-installation-to-server-session-and-profile-trust',
  storage: 'origin-scoped-indexeddb-metadata',
  retention: 'retain-across-sign-out-and-account-removal',
  reset: 'requires-server-revocation-and-rebind',
} as const;

export function hostedCSRFToken(): string {
	return hostedCSRF;
}

export function rememberHostedCSRFToken(token: string): void {
  hostedCSRF = token.trim();
}
