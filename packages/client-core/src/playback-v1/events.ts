/**
 * The one event feed (spec §15; style guide amendment 8; Public API ARCH-API-07): `/v1/events` as
 * SSE, with a long-poll fallback (`GET /v1/events?after=<id>&waitSeconds=25` → `{events, nextAfter}`).
 *
 * - One subscription per device; features subscribe by type (`session.updated`) or prefix
 *   (`group.`), never open their own streams or poll.
 * - The cursor is the last event id; a reconnect resumes from it (`Last-Event-ID` or `after`).
 * - `stream.resync` (the cursor fell out of the server's ring, or the client was too slow) tells
 *   every subscriber to refetch what it shows. `stream.closed` reconnects after `reconnectAfterMs`.
 * - Streaming falls back to long-poll when the platform can't stream, the server refuses
 *   (`409 stream_exists`, 404, 405), or the stream keeps failing.
 * - Redelivered events (after a reconnect) are dropped by id.
 */
import {call, type V1Http} from './http.ts';
import {parseEvent, type ServerEvent} from './types.ts';

/** A raw SSE message (`readEventStream` in `sse.ts` produces these on web; Apple's XHR adapter too). */
export type RawStreamEvent = Readonly<{id?: string; event?: string; data: string}>;

/**
 * Opens the SSE stream from `lastEventId`, calls `onEvent` for each message, and resolves when the
 * stream ends or rejects on failure (with `status` for an HTTP refusal).
 */
export type StreamOpener = (lastEventId: string | undefined, onEvent: (event: RawStreamEvent) => void, signal: AbortSignal) => Promise<void>;

export type EventsClientOptions = Readonly<{
  http: V1Http;
  /** Omit to use long-poll only (constrained clients). */
  openStream?: StreamOpener;
  cursor?: string;
  waitSeconds?: number;
  backoffMs?: readonly number[];
  /** Consecutive stream failures before switching to long-poll for this run. */
  streamFailuresBeforePoll?: number;
  setTimer?: (fn: () => void, ms: number) => unknown;
  clearTimer?: (timer: unknown) => void;
  /** Retry jitter source; injectable for deterministic tests. */
  random?: () => number;
}>;

export type EventsStatus = Readonly<{mode: 'stream' | 'poll' | 'stopped'; connected: boolean; cursor?: string; failures: number}>;

type Listener = (event: ServerEvent) => void;

export class EventsClient {
  private readonly o: EventsClientOptions;
  private listeners = new Map<string, Set<Listener>>();
  private resyncListeners = new Set<() => void>();
  private statusListeners = new Set<() => void>();
  private recent: string[] = [];
  private recentSet = new Set<string>();
  private controller?: AbortController;
  private running = false;
  private status: EventsStatus;
  private streamFailures = 0;
  private setTimer: (fn: () => void, ms: number) => unknown;
  private clearTimer: (t: unknown) => void;
  private sleeping?: {timer: unknown; resolve: () => void};

  constructor(options: EventsClientOptions) {
    this.o = options;
    this.status = Object.freeze({mode: 'stopped', connected: false, cursor: options.cursor, failures: 0});
    this.setTimer = options.setTimer ?? ((fn, ms) => setTimeout(fn, ms));
    this.clearTimer = options.clearTimer ?? (t => clearTimeout(t as ReturnType<typeof setTimeout>));
  }

  getStatus = (): EventsStatus => this.status;
  subscribeStatus = (fn: () => void) => { this.statusListeners.add(fn); return () => { this.statusListeners.delete(fn); }; };
  private setStatus(p: Partial<EventsStatus>) { this.status = Object.freeze({...this.status, ...p}); for (const l of [...this.statusListeners]) l(); }

  /** Listen for one type (`session.updated`) or a prefix ending in a dot (`group.`). */
  on(typeOrPrefix: string, listener: Listener): () => void {
    let set = this.listeners.get(typeOrPrefix);
    if (!set) { set = new Set(); this.listeners.set(typeOrPrefix, set); }
    set.add(listener);
    return () => { set!.delete(listener); };
  }

  /** Refetch what you show: the server couldn't deliver everything since the cursor. */
  onResync(listener: () => void): () => void { this.resyncListeners.add(listener); return () => { this.resyncListeners.delete(listener); }; }

  start(): void {
    if (this.running) return;
    this.running = true;
    this.streamFailures = 0;
    this.controller = new AbortController();
    void this.loop(this.controller.signal);
  }

  stop(): void {
    this.running = false;
    this.controller?.abort();
    if (this.sleeping) { this.clearTimer(this.sleeping.timer); this.sleeping.resolve(); this.sleeping = undefined; }
    this.setStatus({mode: 'stopped', connected: false});
  }

  /** Deliver one envelope (also used by tests and by platforms that receive events elsewhere). */
  deliver(event: ServerEvent): void {
    if (this.recentSet.has(event.id)) return;
    this.recent.push(event.id); this.recentSet.add(event.id);
    if (this.recent.length > 512) this.recentSet.delete(this.recent.shift()!);
    this.setStatus({cursor: event.id});
    if (event.type === 'stream.resync') { for (const l of [...this.resyncListeners]) l(); return; }
    const exact = this.listeners.get(event.type);
    if (exact) for (const l of [...exact]) l(event);
    for (const [key, set] of this.listeners) {
      if (key.endsWith('.') && event.type.startsWith(key) && key !== event.type) for (const l of [...set]) l(event);
    }
  }

  private async sleep(ms: number, signal: AbortSignal): Promise<void> {
    let remaining = ms;
    do {
      if (signal.aborted) return;
      // Long Retry-After values must not overflow a platform timer into an immediate retry.
      const chunk = Math.min(remaining, 2_147_483_647);
      await new Promise<void>(resolve => {
        const timer = this.setTimer(() => { this.sleeping = undefined; resolve(); }, chunk);
        this.sleeping = {timer, resolve};
      });
      remaining -= chunk;
    } while (remaining > 0 && !signal.aborted);
  }

  private backoff(failures: number): number {
    const b = this.o.backoffMs ?? [1000, 2000, 5000, 10_000, 30_000];
    const base = Math.max(1, b[Math.min(failures - 1, b.length - 1)] ?? 30_000);
    const random = Math.max(0, Math.min(1, (this.o.random ?? Math.random)()));
    return Math.round(base * (0.75 + random * 0.5));
  }

  private async loop(signal: AbortSignal): Promise<void> {
    let failures = 0;
    const pollOnly = () => !this.o.openStream || this.streamFailures >= (this.o.streamFailuresBeforePoll ?? 3);
    while (this.running && !signal.aborted) {
      const streaming = !pollOnly();
      this.setStatus({mode: streaming ? 'stream' : 'poll'});
      try {
        const reconnectAfter = streaming ? await this.streamOnce(signal) : await this.pollOnce(signal);
        if (signal.aborted) break;
        failures = 0;
        this.setStatus({failures: 0});
        if (reconnectAfter) await this.sleep(reconnectAfter, signal);
      } catch (e) {
        if (signal.aborted) break;
        const status = (e as {status?: unknown} | null)?.status;
        if (streaming) this.streamFailures = status === 409 || status === 404 || status === 405 ? Infinity : this.streamFailures + 1;
        failures++;
        this.setStatus({connected: false, failures});
        const directed = (e as {retryAfterMs?: unknown} | null)?.retryAfterMs;
        const minimum = typeof directed === 'number' && Number.isFinite(directed) && directed > 0 ? directed : 0;
        if (streaming && pollOnly() && !minimum) continue; // unsupported streams can fall back immediately
        await this.sleep(Math.max(this.backoff(failures), minimum), signal);
      }
    }
  }

  /** One SSE connection. Resolves with a delay before reconnecting (`stream.closed`), or 0. */
  private async streamOnce(signal: AbortSignal): Promise<number> {
    let reconnectAfter: number | undefined;
    const inner = new AbortController();
    const abort = () => inner.abort();
    signal.addEventListener('abort', abort, {once: true});
    try {
      await this.o.openStream!(this.status.cursor, raw => {
        if (signal.aborted || inner.signal.aborted) return;
        this.setStatus({connected: true});
        if (signal.aborted) return;
        this.streamFailures = 0;
        let parsed: unknown;
        try { parsed = JSON.parse(raw.data); } catch { return; }
        const event = parseEvent(parsed);
        if (!event) return;
        if (event.type === 'stream.closed') {
          const after = (event.data as {reconnectAfterMs?: unknown} | undefined)?.reconnectAfterMs;
          reconnectAfter = typeof after === 'number' && Number.isFinite(after) && after >= 0 ? Math.max(1000, after) : 1000;
          inner.abort();
          return;
        }
        this.deliver(event);
      }, inner.signal);
    } catch (e) {
      if (!inner.signal.aborted || signal.aborted) throw e;
    } finally {
      signal.removeEventListener('abort', abort);
      if (!signal.aborted) this.setStatus({connected: false});
    }
    // A proxy/platform may close SSE without a stream.closed envelope. Treat it as a failure
    // so repeated quiet EOFs back off and eventually fall back instead of spinning.
    if (reconnectAfter === undefined && !signal.aborted) throw new Error('event_stream_closed');
    return reconnectAfter ?? 0;
  }

  /** One long-poll round. */
  private async pollOnce(signal: AbortSignal): Promise<number> {
    const wait = Math.max(1, Math.min(this.o.waitSeconds ?? 25, 55));
    const cursor = this.status.cursor;
    const r = await call(this.o.http, {method: 'GET', path: `/v1/events?waitSeconds=${wait}${cursor ? `&after=${encodeURIComponent(cursor)}` : ''}`, signal});
    if (signal.aborted) return 0;
    this.setStatus({connected: true});
    const b = r.body as {events?: unknown; nextAfter?: unknown} | undefined;
    for (const raw of Array.isArray(b?.events) ? b!.events : []) { if (signal.aborted) return 0; const e = parseEvent(raw); if (e) this.deliver(e); }
    if (!signal.aborted && typeof b?.nextAfter === 'string' && b.nextAfter) this.setStatus({cursor: b.nextAfter});
    return 0;
  }
}
