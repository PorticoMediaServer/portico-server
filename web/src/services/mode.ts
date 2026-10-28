/** Portico Account pages at web.getportico.tv. Each is its own Services route. */
export const servicesPaths = ['/account', '/device', '/reset', '/reset-password', '/verify-email', '/claim', '/join', '/sign-in', '/app/account-return'] as const;
const continuation = /^#(signIn=|join$|setup$|local$)/;

/** Which entry a web.getportico.tv document starts. The account pages, and a root address
 * carrying an account continuation, are Services. A root address with no server selected in
 * this browser is the Portico Account front door (sign in, or straight on to the chooser).
 * Every other address is the app. A storage failure reads as "nothing selected": the front
 * door asks again rather than guessing a server. */
export async function hostedServicesPage(page: Pick<Location, 'pathname' | 'search' | 'hash'>,
  selected: () => Promise<unknown> = async () => (await import('../platform/server-connections')).browserAppSessions().selected()): Promise<boolean> {
  if ((servicesPaths as readonly string[]).includes(page.pathname)) return true;
  if (page.pathname !== '/') return false;
  const search = new URLSearchParams(page.search);
  if (continuation.test(page.hash) || search.has('mode') || search.has('identityError')) return true;
  try {return !await selected();} catch {return true;}
}
