/**
 * What a failed launch restore means (the rule shared with Apple, client-core 440a731):
 *
 * - **reset**: only when the renewal itself was refused (`SessionRefreshError` with `signOut`:
 *   a 401, or the 403 of a renewal the server revoked; revoked or unknown family), the server
 *   refused this sign-in access after a renewal (`access_refused`), or the saved connection is
 *   damaged for good (`invalid_saved_connection`: waiting never mends it, and signing in
 *   replaces it). The sign-in has ended: the app opens the sign-in screen for that server.
 * - **replaced**: something at the saved address now reports a different server id
 *   (`/v1/system`). That answer is not signed, so it proves nothing: another machine may have
 *   been given the address, or a proxy may be misrouting. The saved sign-in is kept and Portico
 *   keeps trying; the person is told another server is answering and may choose to sign in again.
 * - **retry**: everything else. Network and route failures, 5xx and 429, a 400 (a malformed
 *   renewal keeps the credential), a 404, a saved connection that no longer matches this tab or
 *   cannot be read right now. The saved sign-in is kept and the shell shows "Can't reach" while
 *   Portico keeps trying.
 */
import {ApiError} from '@core/index.ts';
import {ACCESS_REFUSED, SessionRefreshError, savedConnectionDamaged} from '@core/server-connections.ts';

export type RestoreOutcome = 'reset' | 'retry' | 'replaced';

/** The renewal was refused, the server refuses this sign-in access, or the saved connection is damaged for good (the sign-in has ended). */
export function refreshRefused(error: unknown): boolean {
  if (error instanceof SessionRefreshError) return error.signOut;
  if (savedConnectionDamaged(error)) return true;
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
  if (saved && (await serverReplaced(saved.serverUrl, saved.serverId, fetcher))) return 'replaced';
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
