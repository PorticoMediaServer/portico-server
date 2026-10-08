/**
 * Switching Portico Accounts in this browser (INT follow-up to gate 2). A Portico Account member's
 * server sign-in is a direct session on that server, made with the account's identity: it belongs
 * to that Portico Account. When another Portico Account is signed in here, the previous account's
 * server sign-in (its name, profile, server) must not stay on screen.
 *
 * `owner` is the Portico Account the server sign-in was made with: the server's own record of the
 * member (`hostedAccountId`), else the account its routes were fetched with. A password sign-in has
 * neither and is never touched; with no Portico Account signed in nothing is decided here (Hosted
 * being away never signs anyone out).
 */
export function belongsToAnotherAccount(owner: string | undefined, signedIn: string | undefined): boolean {
  return !!owner && !!signedIn && owner !== signedIn;
}

/** A server list answered for one account is never shown while another is signed in. */
export function listIsCurrent(owner: string, signedIn: string | undefined): boolean {
  return owner === signedIn;
}
