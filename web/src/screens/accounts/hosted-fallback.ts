import type {DirectoryFallback} from '../../kit/accounts/DirectoryStatus';
import type {DirectoryItem, DirectorySnapshot} from '../../services/directory-flow';

/** The single way to the server's own page, worded for the case (P2-66 D-4). Its address is
 * one this browser already proved, or the account's verified direct address. */
export function fallbackFor(state: Pick<DirectorySnapshot, 'status'>, target: DirectoryItem, saved: ReadonlyMap<string, string>, open: (address: string) => void): DirectoryFallback | undefined {
  const lastGood = saved.get(target.id), local = lastGood && isLocalRoute(lastGood) ? lastGood : undefined;
  const go = (address: string) => () => open(new URL('/', address).href);
  if (state.status === 'update') return target.baseUrl ? {kind: 'direct', onPress: go(target.baseUrl)} : undefined;
  if (state.status === 'account-waiting' && target.ownedByMe) return local ? {kind: 'local', onPress: go(local)} : target.baseUrl ? {kind: 'direct', onPress: go(target.baseUrl)} : undefined;
  if ((state.status === 'waiting' || state.status === 'unreachable') && local) return {kind: 'local', onPress: go(local)};
  return undefined;
}
/** A server's own home-network address: its LAN name under the Portico certificate, or a private IP. */
export function isLocalRoute(origin: string): boolean {
  try {
    const host = new URL(origin).hostname.replace(/^\[|\]$/g, '');
    return /^l-[a-z0-9-]+\.ptc-[a-z0-9]+\.direct\.getportico\.tv$/i.test(host) || /^(10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.)/.test(host) || /^f[cd][0-9a-f]{2}:/i.test(host) || host.endsWith('.local');
  } catch {return false;}
}
