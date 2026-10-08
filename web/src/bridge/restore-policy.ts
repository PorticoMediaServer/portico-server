/**
 * What a failed launch restore means (the rule shared with Apple, client-core 440a731):
 *
 * - **reset**: only when the renewal itself was refused (`SessionRefreshError` with `signOut`:
 *   401/403, revoked or unknown family), the server refused this sign-in access after a renewal
 *   (`access_refused`), or the server at the saved address now reports a different server id
 *   (`/v1/system`). The sign-in has ended: the app opens the sign-in screen for that server.
 * - **retry**: everything else. Network and route failures, 5xx and 429, a 400 (a malformed
 *   renewal keeps the credential), a 404, a saved connection that no longer matches this tab.
 *   The saved sign-in is kept and the shell shows "Can't reach" while Portico keeps trying.
 */
import {ApiError} from '@core/index.ts';
import {ACCESS_REFUSED, SessionRefreshError} from '@core/server-connections.ts';

export type RestoreOutcome = 'reset' | 'retry';

/** The renewal was refused, or the server refuses this sign-in access (the sign-in has ended). */
export function refreshRefused(error: unknown): boolean {
  if (error instanceof SessionRefreshError) return error.signOut;
  const e = error as {name?: unknown; code?: unknown} | null;
  return !!e && typeof e === 'object' && e.name === 'RouteError' && e.code === ACCESS_REFUSED;
}

/** Whether the server at `serverUrl` now reports another server id. Any failure answers "no". */
export async function serverReplaced(serverUrl: string, serverId: string, fetcher: typeof fetch = (input, init) => fetch(input, init), timeoutMs = 5000): Promise<boolean> {
  const abort = new AbortController();
  const timer = setTimeout(() => abort.abort(), timeoutMs);
  try {
    const response = await fetcher(serverUrl.replace(/\/+$/, '') + '/v1/system', {signal: abort.signal, headers: {Accept: 'application/json'}, credentials: 'omit', redirect: 'error'});
    if (!response.ok) return false;
    const info = (await response.json()) as {id?: unknown; serverId?: unknown};
    const reported = typeof info.serverId === 'string' ? info.serverId : typeof info.id === 'string' ? info.id : undefined;
    return !!reported && reported !== serverId;
  } catch {
    return false;
  } finally {
    clearTimeout(timer);
  }
}

/** Classify a failed restore of the saved viewer. */
export async function restoreOutcome(error: unknown, saved: {serverUrl: string; serverId: string} | undefined, fetcher?: typeof fetch): Promise<RestoreOutcome> {
  if (refreshRefused(error)) return 'reset';
  if (saved && (await serverReplaced(saved.serverUrl, saved.serverId, fetcher))) return 'reset';
  return 'retry';
}

/** Transport-level failures (no answer from the server); kept for callers that only need this. */
export function retryableServerRestore(error: unknown): boolean {
  if (error instanceof TypeError) return true; // Browser fetch transport failure.
  if (!error || typeof error !== 'object') return false;
  if (error instanceof ApiError) return error.status === 408 || error.status === 429 || error.status >= 500;
  const value = error as {name?: unknown; code?: unknown};
  return value.name === 'RouteError' && value.code === 'route_unavailable';
}
