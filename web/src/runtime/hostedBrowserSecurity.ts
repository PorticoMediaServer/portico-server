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
  if (hostedCSRF) return hostedCSRF;
  // Rolling-upgrade bridge only: the prior Cloud release stored this token in
  // a parent-domain cookie. Once Cloud emits the API-origin response header,
  // the in-memory value wins and the server deletes this retired cookie.
  if (typeof document === 'undefined') return '';
  const prefix = 'portico_hosted_csrf=';
  const encoded = document.cookie.split(';').map((part) => part.trim())
    .find((part) => part.startsWith(prefix))?.slice(prefix.length) ?? '';
  if (!encoded) return '';
  try {
    return decodeURIComponent(encoded);
  } catch {
    return encoded;
  }
}

export function rememberHostedCSRFToken(token: string): void {
  hostedCSRF = token.trim();
}
