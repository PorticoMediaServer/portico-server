/**
 * ONB-09: Portico Account invitation links arrive as `https://web.getportico.tv/#invite=<id>.<secret>`
 * (every kind of invitation). Before the router mounts, move them to `/join`, which names the
 * server and inviter and accepts in one step. The secret stays in the fragment (never the query,
 * history entries or storage); `/join` scrubs it after reading.
 */
export function routeInvitationLink(loc: Pick<Location, 'pathname' | 'hash'> = location, replace: (url: string) => void = url => history.replaceState(null, '', url)): boolean {
  if (!loc.hash.startsWith('#invite=') || loc.pathname === '/join') return false;
  replace('/join' + loc.hash);
  return true;
}

if (typeof location !== 'undefined') routeInvitationLink();
