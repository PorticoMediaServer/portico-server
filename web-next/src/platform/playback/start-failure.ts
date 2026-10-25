import {call, PlaybackApiError, type V1Http, type V1Response} from '@core/playback-v1/http.ts';
import {playbackFailurePermanent} from '@core/playback-messages.ts';
import {playbackControlsService, type RemotePlaybackControls} from '@core/playback-controls.ts';
import type {HttpLocalApi} from '@core/index.ts';
import type {EventsClient} from '@core/playback-v1/events.ts';
import {routePath} from '../../../../packages/contracts/src/routes.ts';

export type WebPlaybackStartFailure = Readonly<{
  code: string; scope: 'account' | 'server' | 'unknown'; retryable: boolean;
  streamLimit?: Readonly<{scope: 'account'; playing: number; limit: number}>;
  isCurrent(): boolean;
  retry(): Promise<void>;
  cancel(): void;
  remote(): Promise<Readonly<{items: readonly RemotePlaybackControls[]; nextCursor: string}>>;
  stop(row: RemotePlaybackControls): Promise<void>;
}>;
const retired = () => new DOMException('Playback start retired', 'AbortError');
const starts: ReadonlySet<string> = new Set([routePath('start_playback_session'), routePath('create_queue')]);

/** A first start stays with its original core caller. The HTTP response is held at
 * this seam while its real failure is drawn; retry resumes that same request, body,
 * key and revision, rather than calling Play against a newer runtime or selection.
 * This sits outside capacityRetryHttp, whose directed conversion retries stay intact. */
export function startFailureHttp(input: Readonly<{
  http: V1Http; api: HttpLocalApi; events: Pick<EventsClient, 'on'>; life: AbortSignal;
  isCurrent(): boolean; isInitial(): boolean; captureCurrent(): (() => boolean) | undefined;
  now?: () => number; publish(failure: WebPlaybackStartFailure | undefined): void;
  cancelStart(): void;
}>): V1Http {
  let generation = 0, active: WebPlaybackStartFailure | undefined;
  const now = input.now ?? Date.now;
  return {async send(request) {
    if (request.method !== 'POST' || !starts.has(request.path) || !input.isInitial()
      || !Object.entries(request.headers ?? {}).some(([key, value]) => key.toLowerCase() === 'idempotency-key' && !!value)) return input.http.send(request);
    const originalCurrent = input.captureCurrent();
    if (!originalCurrent) return input.http.send(request);
    const serial = ++generation, controller = new AbortController();
    const signal = AbortSignal.any([input.life, controller.signal, ...(request.signal ? [request.signal] : [])]);
    const captured = {...request, headers: {...request.headers}, body: request.body === undefined ? undefined : JSON.parse(JSON.stringify(request.body)), signal};
    const current = () => {
      if (signal.aborted) return false;
      let valid = false;
      try {valid = serial === generation && input.isCurrent() && originalCurrent();} catch { /* Retired original caller. */ }
      // End the held HTTP continuation as well as refusing its callbacks. Merely
      // hiding a stale card must not leave the original screen's Play pending.
      if (!valid) controller.abort();
      return valid;
    };
    const assert = () => {if (!current()) throw retired();};
    let mine: WebPlaybackStartFailure | undefined, automaticRetries = 0, firstRecovery: number | undefined;
    try {
      for (;;) {
        assert();
        let response: V1Response | undefined, error: PlaybackApiError;
        try {
          response = await input.http.send(captured);
          // A late success still belongs to the original core, which can clean up
          // its admitted session. Never turn it into another start or a failure.
          if (response.status < 400 || !current() || response.status === 401 || response.status === 403) return response;
          try {await call({send: async () => response!}, captured);} catch (failure) {
            if (!(failure instanceof PlaybackApiError)) throw failure;
            error = failure;
          }
        } catch (failure) {
          if (!current() || signal.aborted || failure instanceof DOMException && failure.name === 'AbortError') throw failure;
          // Only an actual transport failure is presented as a lost connection.
          // Identity/guard/decoder errors retain their original typed error path.
          if (!(failure instanceof TypeError || failure instanceof DOMException && failure.name === 'TimeoutError')) throw failure;
          error = new PlaybackApiError(0, 'server_unreachable', 'same_request');
        }
        assert();
        // A revision, identity or validation refusal must reach its existing owner;
        // retaining it would disguise a required refresh as a replayable start.
        if (response && (response.status === 412 || ['idempotency_key_reused', 'invalid_request', 'revision_mismatch', 'revision_required', 'factor_setup_required', 'access_revoked'].includes(error!.code))) return response;
        const scope = error!.code !== 'stream_limit_reached' ? 'unknown'
          : error!.reason === 'owner_server_cap' ? 'server' : error!.reason === 'owner_account_cap' || error!.streamLimit ? 'account' : 'unknown';
        const limit = scope === 'account' ? error!.streamLimit : undefined;
        let wake: (() => void) | undefined, requested = false, busy = false, readSerial = 0;
        let offered: readonly RemotePlaybackControls[] = Object.freeze([]);
        const service = playbackControlsService(<T,>(path: string, method?: string, body?: unknown) => input.api.request<T>(path, method, body, signal));
        const shown = () => current() && active === failure;
        const check = () => {if (!shown()) throw retired();};
        const completeRead = async () => {
          check();const rows: RemotePlaybackControls[] = [], cursors = new Set<string>();let cursor = '';
          // An incomplete page can never prove a stream ended. Bound the read,
          // refuse a looping cursor, and never infer capacity from a partial list.
          for (let page = 0; page < 20; page++) {
            const value = await service.remote(cursor, 100);check();rows.push(...value.items);
            if (!value.nextCursor) {
              if (new Set(rows.map(row => row.sessionId)).size !== rows.length) throw retired();
              return Object.freeze(rows);
            }
            if (cursors.has(value.nextCursor)) throw retired();
            cursors.add(value.nextCursor);cursor = value.nextCursor;
          }
          throw retired();
        };
        const resume = (automatic: boolean) => {
          check();if (requested) throw retired();
          if (automatic) {
            const at = now();firstRecovery ??= at;
            if (automaticRetries >= 5 || at - firstRecovery > 30_000) throw retired();
            automaticRetries++;
          }
          requested = true;active = undefined;input.publish(undefined);wake?.();
        };
        const confirmFreed = async (row: RemotePlaybackControls) => {
          const remaining = await completeRead();check();
          // A complete fresh account read after the original offered row's Stop
          // acknowledgement (or its newer ended event) is the capacity evidence.
          if (!remaining.some(value => value.sessionId === row.sessionId)) resume(true);
        };
        const failure: WebPlaybackStartFailure = Object.freeze({code: error!.code, scope,
          retryable: !playbackFailurePermanent(error!), ...(limit ? {streamLimit: limit} : {}), isCurrent: shown,
          async retry() {check();if (scope === 'account' || playbackFailurePermanent(error!)) throw retired();resume(false);},
          cancel() {if (shown()) {controller.abort();input.cancelStart();}},
          async remote() {
            check();if (scope !== 'account') throw retired();const ticket = ++readSerial;
            const rows = await completeRead();check();if (ticket !== readSerial || busy) throw retired();
            offered = Object.freeze(rows.filter(row => row.actions.includes('stop') && row.state !== 'ended'));
            return Object.freeze({items: offered, nextCursor: ''});
          },
          async stop(row) {
            check();if (scope !== 'account' || busy || !offered.includes(row) || !row.actions.includes('stop')) throw retired();
            busy = true;++readSerial;
            try {await service.stopAccount(row.sessionId);check();await confirmFreed(row);} finally {busy = false;}
          },
        });
        mine = active = failure;
        // A revisioned ended event is only a hint to reread. It is never permission
        // to retry a session not present in this failure's own scoped offered rows.
        const off = input.events.on('session.updated', event => {
          if (scope !== 'account' || !shown() || busy || event.resource?.kind !== 'session') return;
          const row = offered.find(value => value.sessionId === event.resource?.id);
          const data = event.data as {state?: unknown} | undefined;
          if (!row || data?.state !== 'ended' || !/^\d{1,32}$/.test(event.revision ?? '') || BigInt(event.revision!) <= BigInt(row.revision)) return;
          busy = true;++readSerial;
          void confirmFreed(row).catch(() => {}).finally(() => {busy = false;});
        });
        try {
          input.publish(failure);
          assert();
          await new Promise<void>((resolve, reject) => {
            const abort = () => {signal.removeEventListener('abort', abort);reject(retired());};
            wake = () => {signal.removeEventListener('abort', abort);resolve();};
            signal.addEventListener('abort', abort, {once: true});
            if (signal.aborted) abort();else if (requested) wake();
          });
        } finally {off();}
      }
    } finally {if (active === mine && mine) {active = undefined;input.publish(undefined);}}
  }};
}
