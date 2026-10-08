import type {HostedServer} from '@core/index.ts';
import type {ServerListRecord, ServerListStorage} from '@core/portico-servers.ts';

/**
 * The Portico Account's server list kept between visits (Spec — Hosted at Scale): the list itself
 * and the version it was fetched at. A launch shows the kept list and asks Hosted only when the
 * version check is due (`ServerListWatcher.foreground`), so opening the app is not a Hosted call.
 *
 * Signed route documents are not kept: they go stale, and connecting to a server asks for fresh
 * ones. No credentials are stored here; the watch token only reveals whether the list changed.
 */
const key = (accountId: string, part: 'list' | 'version') => `portico.serverList.${part}.${accountId}`;
function read(k: string): unknown {
  try { return JSON.parse(localStorage.getItem(k) ?? 'null'); } catch { return null; }
}
function write(k: string, value: unknown) {
  try { if (value === null) localStorage.removeItem(k); else localStorage.setItem(k, JSON.stringify(value)); } catch { /* blocked storage: no memory between visits */ }
}

export function serverListStorage(accountId: string): ServerListStorage {
  return {
    load: () => {
      const v = read(key(accountId, 'version')) as ServerListRecord | null;
      return v && typeof v === 'object' && typeof v.version === 'string' && typeof v.watch === 'string' && Number.isFinite(v.checkedAt) && Number.isFinite(v.nextCheckAt) ? v : null;
    },
    save: record => write(key(accountId, 'version'), record),
  };
}

export function cachedServerList(accountId: string): HostedServer[] | undefined {
  const v = read(key(accountId, 'list'));
  if (!Array.isArray(v)) return undefined;
  return v.filter((x): x is HostedServer => !!x && typeof x === 'object' && typeof (x as HostedServer).id === 'string' && typeof (x as HostedServer).name === 'string');
}

export function storeServerList(accountId: string, items: readonly HostedServer[] | null) {
  write(key(accountId, 'list'), items === null ? null : items.map(server => {
    const {routes: _routes, ...rest} = server as HostedServer & {routes?: unknown};
    return rest;
  }));
}

/** The Portico server this account last opened here (Justin: with several servers, open the last one used). */
const lastKey = (accountId: string) => `portico.lastServer.v1:${accountId}`;
export function lastServer(accountId: string): string | undefined {
  try { return localStorage.getItem(lastKey(accountId)) ?? undefined; } catch { return undefined; }
}
export function rememberLastServer(accountId: string, serverId: string) {
  try { localStorage.setItem(lastKey(accountId), serverId); } catch { /* blocked storage */ }
}
