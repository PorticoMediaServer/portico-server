/** O-161/O-162: one shareable link format everywhere (Justin, 29 Oct), Plex-style:
 * `https://web.getportico.tv/s/<serverId>/<screen>`. `<serverId>` is the server's stable pinned
 * ID, never an address. Web reads and writes it in the address bar; the apps open it as a
 * universal/app link and run the same flow (switch server, sign in, choose a profile, continue,
 * or say the link cannot be opened). This grammar is frozen: change it only with every client. */

/** The canonical origin of shared links. `getportico.tv` is accepted for the same paths. */
export const SERVER_LINK_ORIGIN = 'https://web.getportico.tv';
const linkHosts: readonly string[] = ['web.getportico.tv', 'getportico.tv'];

/** Screens that show a server's content or a place on it. Account, server choice, setup,
 * sign-in, settings, device, help and about are never scoped. */
const scopedScreen = /^\/(?:|search|search\/reading|libraries|library\/[^/]+|live|saved|item\/[^/]+|home\/rows\/[^/]+|player|person\/[^/]+|read\/[^/]+|watch-together|dashboard|notifications)$/;
const serverIdPattern = /^[A-Za-z0-9._~-]{1,128}$/;

export const isScopedScreenPath = (pathname: string): boolean => scopedScreen.test(pathname);
export const validServerId = (value: unknown): value is string => typeof value === 'string' && serverIdPattern.test(value) && value !== '.' && value !== '..';

export type ServerLinkPath = Readonly<{serverId: string; pathname: string}>;
/** `/s/<serverId>` + a scoped screen path, else undefined. `/s/<id>` and `/s/<id>/` are Home. */
export function parseServerLinkPath(pathname: string): ServerLinkPath | undefined {
  const match = /^\/s\/([^/]+)(\/.*)?$/.exec(pathname);
  if (!match) return undefined;
  let serverId: string;
  try {serverId = decodeURIComponent(match[1]!);} catch {return undefined;}
  const rest = !match[2] || match[2] === '/' ? '/' : match[2];
  return validServerId(serverId) && isScopedScreenPath(rest) ? Object.freeze({serverId, pathname: rest}) : undefined;
}
/** The scoped path of a screen path on `serverId`; a path that is not a scoped screen is unchanged. */
export function scopedPathname(pathname: string, serverId: string): string {
  if (!isScopedScreenPath(pathname) || !validServerId(serverId)) return pathname;
  return `/s/${encodeURIComponent(serverId)}${pathname === '/' ? '' : pathname}`;
}

export type ServerLink = ServerLinkPath & Readonly<{search: string; hash: string}>;
/** A whole shared link: https, one of the link hosts, no user, port or credentials, a scoped path.
 * Anything else is not a Portico server link (an app hands it back to the browser). */
export function parseServerLink(href: string): ServerLink | undefined {
  let url: URL;
  try {url = new URL(href);} catch {return undefined;}
  if (url.protocol !== 'https:' || !linkHosts.includes(url.hostname.toLowerCase()) || url.port || url.username || url.password) return undefined;
  const path = parseServerLinkPath(url.pathname);
  return path ? Object.freeze({...path, search: url.search, hash: url.hash}) : undefined;
}
/** The shareable link to a screen on a server; `search` and `hash` are kept as given. */
export function serverLink(serverId: string, pathname: string, search = '', hash = '', origin = SERVER_LINK_ORIGIN): string | undefined {
  if (!validServerId(serverId) || !isScopedScreenPath(pathname)) return undefined;
  return origin + scopedPathname(pathname, serverId) + (search && !search.startsWith('?') ? '?' + search : search) + (hash && !hash.startsWith('#') ? '#' + hash : hash);
}
