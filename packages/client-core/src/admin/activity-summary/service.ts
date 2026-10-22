import {routePath} from '../../../../contracts/src/routes.ts';
import {utcOrderKey} from '../../local-session.ts';
import {call} from '../../playback-v1/http.ts';
import type {EventsClient} from '../../playback-v1/events.ts';
import {parseActivitySummary} from './parser.ts';
import type {ActivitySummaryErrorCode, ActivitySummaryScope, ActivitySummarySnapshot, ActivitySummarySource, ActivitySummarySourceStore, ActivitySummaryTimers, OwnerActivitySummary} from './types.ts';

const hintTypes = ['admin.sessions', 'library.scan.updated', 'job.updated', 'operation.updated', 'server.update'] as const;
const scopeKeys = ['serverId', 'accountId', 'profileId', 'authority', 'role', 'sessionFamilyId', 'authorizationEpoch'] as const;
const unavailable: ActivitySummarySnapshot = Object.freeze({phase: 'unavailable'});
const defaults: ActivitySummaryTimers = {
  now: Date.now,
  setTimer: (fn, ms) => setTimeout(fn, ms),
  clearTimer: timer => clearTimeout(timer as ReturnType<typeof setTimeout>),
};
function owner(source: ActivitySummarySource | null): source is ActivitySummarySource {
  return !!source && typeof source.http?.send === 'function' && !!source.scope && scopeKeys.every(k => typeof source.scope[k] === 'string' && source.scope[k].length > 0 && source.scope[k].length <= 1024 && !/[\x00-\x1f\x7f]/.test(source.scope[k])) && source.scope.role === 'owner' && ['local', 'hosted'].includes(source.scope.authority);
}
const same = (a?: ActivitySummarySource | null, b?: ActivitySummarySource | null) => a === b || !!a && !!b && a.http === b.http && scopeKeys.every(k => a.scope[k] === b.scope[k]);
const failure = (code: ActivitySummaryErrorCode) => Object.assign(new Error(code), {code});
function safeError(error: unknown): NonNullable<ActivitySummarySnapshot['error']> {
  const value = error !== null && typeof error === 'object' ? error as {status?: unknown; code?: unknown; retryAfterMs?: unknown} : {};
  const denied = value.status === 401 || value.status === 403 || ['unauthorized', 'forbidden', 'not_permitted'].includes(String(value.code));
  const code: ActivitySummaryErrorCode = denied ? 'not_permitted' : ['invalid_activity_summary', 'permission_changed', 'request_timeout'].includes(String(value.code)) ? value.code as ActivitySummaryErrorCode : 'request_failed';
  return Object.freeze({code, ...(typeof value.retryAfterMs === 'number' && Number.isSafeInteger(value.retryAfterMs) && value.retryAfterMs > 0 ? {retryAfterMs: value.retryAfterMs} : {})});
}

/**
 * The desktop owner's private summary. This owns no feed, polling loop or credential cache.
 * The shell supplies its current observable session scope and existing authenticated transport.
 */
export class ActivitySummaryService {
  private readonly source: ActivitySummarySourceStore;
  private readonly timers: ActivitySummaryTimers;
  private readonly timeoutMs: number;
  private releaseSource: () => void;
  private listeners = new Set<() => void>();
  private state: ActivitySummarySnapshot = unavailable;
  private current?: ActivitySummarySource;
  private active = false;
  private disposed = false;
  private epoch = 0;
  private fence?: string;
  private observed?: string;
  private blocked = false;
  private reading?: {controller: AbortController; promise: Promise<void>};
  private pending = false;
  private timer?: unknown;
  private lastStart = -Infinity;
  private retryAt = -Infinity;
  private releases: (() => void)[] = [];

  constructor(options: {source: ActivitySummarySourceStore; timers?: ActivitySummaryTimers; timeoutMs?: number}) {
    this.source = options.source;
    this.timers = options.timers ?? defaults;
    this.timeoutMs = options.timeoutMs ?? 10_000;
    if (!Number.isSafeInteger(this.timeoutMs) || this.timeoutMs < 1 || this.timeoutMs > 60_000) throw new Error('invalid_activity_timeout');
    this.syncSource();
    this.releaseSource = this.source.subscribe(() => {
      if (this.syncSource() && this.active && this.current) this.invalidate();
    });
  }

  getSnapshot = (): ActivitySummarySnapshot => { this.syncSource(); return this.state; };
  subscribe = (listener: () => void): (() => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private publish(state: ActivitySummarySnapshot): void {
    this.state = Object.freeze(state);
    for (const listener of [...this.listeners]) listener();
  }
  private releaseFeed(): void { for (const release of this.releases) release(); this.releases = []; }
  private stopRead(): void {
    this.epoch++;
    this.reading?.controller.abort(); this.reading = undefined;
    if (this.timer !== undefined) this.timers.clearTimer(this.timer);
    this.timer = undefined; this.pending = false;
  }
  private syncSource(): boolean {
    if (this.disposed) return false;
    const value = this.source.getSnapshot();
    const next = owner(value) ? value : undefined;
    if (same(this.current, next)) return false;
    this.stopRead(); this.releaseFeed();
    this.current = next ? Object.freeze({http: next.http, scope: Object.freeze({...next.scope})}) : undefined;
    this.fence = undefined; this.observed = undefined; this.blocked = false;
    this.lastStart = -Infinity; this.retryAt = -Infinity;
    this.publish(this.active && this.current ? {phase: 'idle'} : unavailable);
    return true;
  }
  /** Fail closed until the desktop owner surface is mounted and active. Hiding it clears private facts. */
  setActive(active: boolean): void {
    this.syncSource();
    if (this.disposed || active === this.active) return;
    this.active = active;
    if (!active) { this.stopRead(); this.releaseFeed(); this.publish(unavailable); return; }
    if (this.current && !this.blocked) this.publish({phase: 'idle'});
  }
  /** Bind the current session's existing feed. It is never started, stopped or replaced here. */
  attach(events: Pick<EventsClient, 'on' | 'onResync'> | null): void {
    this.syncSource(); this.releaseFeed();
    if (this.disposed || !this.active || !this.current || this.blocked || !events) return;
    const source = this.current, epoch = this.epoch;
    const hint = () => {
      this.syncSource();
      if (this.epoch === epoch && same(source, this.current)) this.invalidate();
    };
    this.releases = [...hintTypes.map(type => events.on(type, hint)), events.onResync(hint)];
  }
  private eligible(): boolean {
    this.syncSource();
    return !this.disposed && this.active && !!this.current && !this.blocked;
  }
  private invalidate(): void {
    if (!this.eligible()) return;
    this.pending = true;
    this.schedule();
  }
  private schedule(): void {
    if (!this.pending || this.reading || this.timer !== undefined || !this.eligible()) return;
    const delay = Math.max(0, this.lastStart + 1000 - this.timers.now(), this.retryAt - this.timers.now());
    this.timer = this.timers.setTimer(() => { this.timer = undefined; if (this.pending) void this.load(); }, delay);
  }
  /** One explicit read. Concurrent callers share it. Hints during the read schedule one trailing read. */
  load = (): Promise<void> => this.read();
  retry = (): Promise<void> => this.read();
  /** Compatible with the existing freshness document protocol; no content is retained after refusal. */
  revalidate = (context?: {signal?: AbortSignal; settle?: () => Promise<void>}): Promise<void> => this.read(context);

  private read(context?: {signal?: AbortSignal; settle?: () => Promise<void>}): Promise<void> {
    if (!this.eligible()) return Promise.resolve();
    if (context?.signal?.aborted) { this.cancel(); return Promise.resolve(); }
    if (this.reading) return this.reading.promise;
    if (this.timers.now() < this.retryAt) { this.pending = true; this.schedule(); return Promise.resolve(); }
    if (this.timer !== undefined) this.timers.clearTimer(this.timer);
    this.timer = undefined; this.pending = false;
    const source = this.current!, epoch = this.epoch, controller = new AbortController();
    this.lastStart = this.timers.now();
    let timer: unknown, abort = () => {};
    const stop = () => { controller.abort(); if (this.reading?.controller === controller) this.cancel(); };
    context?.signal?.addEventListener('abort', stop, {once: true});
    const isCurrent = () => { this.syncSource(); return !this.disposed && this.active && epoch === this.epoch && same(source, this.current); };
    const promise = Promise.resolve().then(async () => {
      try {
        if (!isCurrent()) return;
        if (controller.signal.aborted) { this.publish({phase: 'idle'}); return; }
        this.publish({...this.state, phase: 'loading', error: undefined});
        if (!isCurrent()) return;
        const interrupted = new Promise<never>((_, reject) => {
          abort = () => reject(failure('request_failed'));
          controller.signal.addEventListener('abort', abort, {once: true});
          timer = this.timers.setTimer(() => { reject(failure('request_timeout')); controller.abort(); }, this.timeoutMs);
        });
        const document = await Promise.race([
          (async () => {
            const response = await call(source.http, {method: 'GET', path: routePath('get_admin_activity'), headers: {'Cache-Control': 'no-cache, no-store'}, signal: controller.signal}, [200]);
            return parseActivitySummary(response.body);
          })(),
          interrupted,
        ]);
        if (!isCurrent()) return;
        await Promise.race([context?.settle?.(), interrupted]);
        if (!isCurrent()) return;
        if (controller.signal.aborted) { this.publish({phase: 'idle'}); return; }
        this.admit(document, source.scope);
        this.publish({phase: 'ready', document});
      } catch (error) {
        if (!isCurrent()) return;
        if (controller.signal.aborted && context?.signal?.aborted) { this.publish({phase: 'idle'}); return; }
        const safe = safeError(error);
        if (safe.code === 'not_permitted' || safe.code === 'permission_changed') {
          this.blocked = true; this.pending = false; this.releaseFeed();
        }
        this.retryAt = this.timers.now() + (safe.retryAfterMs ?? 0);
        // Private bodies and opaque error payloads never survive a failed or stale read.
        this.publish({phase: 'error', error: safe});
      } finally {
        if (timer !== undefined) this.timers.clearTimer(timer);
        controller.signal.removeEventListener('abort', abort);
        context?.signal?.removeEventListener('abort', stop);
        if (this.reading?.controller === controller) { this.reading = undefined; this.schedule(); }
      }
    });
    this.reading = {controller, promise};
    return promise;
  }
  private admit(document: OwnerActivitySummary, scope: ActivitySummaryScope): void {
    if (document.scope.serverId !== scope.serverId || this.fence !== undefined && document.scope.viewerFence !== this.fence) throw failure('permission_changed');
    const observed = utcOrderKey(document.observedAt);
    if (this.observed !== undefined && observed < this.observed) throw failure('invalid_activity_summary');
    this.fence = document.scope.viewerFence; this.observed = observed;
  }
  /** Cancel and erase; an ignored AbortSignal can never release its late private answer. */
  cancel(): void {
    if (this.disposed) return;
    this.syncSource(); this.stopRead(); this.releaseFeed();
    this.publish(this.active && this.current && !this.blocked ? {phase: 'idle'} : unavailable);
  }
  dispose(): void {
    if (this.disposed) return;
    this.stopRead(); this.releaseFeed(); this.releaseSource();
    this.current = undefined; this.fence = undefined; this.observed = undefined;
    this.disposed = true; this.publish(unavailable); this.listeners.clear();
  }
}
