/**
 * Viewer-scoped synchronization, query deduplication, and overload protection.
 *
 * One instance is owned by one authenticated viewer generation. Components
 * lease logical resources and subscriptions; they never own physical retry
 * loops. This keeps remounts, navigation, and focus changes from multiplying
 * requests while preserving strict profile-generation fencing.
 */

import {
  porticoEventFailureIsRetryable,
  porticoEventRetryDelay,
  type PorticoAbortSignal,
  type PorticoEventRetryPolicy,
  type PorticoViewerGeneration
} from "./eventSubscriptions.js";

export type ViewerSyncLifecycleReason = "unauthenticated" | "forbidden";
export type ViewerSyncInvalidationUrgency = "immediate" | "coalesced";
/** Playback continuity always outranks active UI, which outranks hidden reconciliation. */
export type ViewerSyncWorkPriority = "playback-continuity" | "interactive" | "background";

export interface ViewerSyncGenerationFence {
  readonly generation: PorticoViewerGeneration;
  currentGeneration(): PorticoViewerGeneration;
}

export interface ViewerSyncLifecycleEvent {
  reason: ViewerSyncLifecycleReason;
  status: 401 | 403;
  resourceKey: string;
  cause: unknown;
}

export interface ViewerSyncRuntime {
  now(): number;
  random(): number;
  setTimer(callback: () => void, delayMs: number): unknown;
  clearTimer(handle: unknown): void;
}

export interface ViewerSyncOptions {
  generationFence: ViewerSyncGenerationFence;
  onLifecycleEvent(event: ViewerSyncLifecycleEvent): void;
  /** Ordinary invalidations are collected for this long. */
  coalesceWindowMs?: number;
  /** A zero-consumer subscription survives this long to absorb UI remounts. */
  subscriptionLingerMs?: number;
  /** A connection surviving this long resets accumulated retry pressure. */
  subscriptionHealthyAfterMs?: number;
  maximumLogicalSubscriptions?: number;
  maximumResourceRegistrations?: number;
  maximumPendingInvalidationTags?: number;
  maximumInflightQueries?: number;
  /** Kept unavailable to ordinary app-sync queries. */
  reservedPlaybackQuerySlots?: number;
  maximumBackgroundInflightQueries?: number;
  maximumCacheEntries?: number;
  maximumCacheWeight?: number;
  defaultCacheMaxAgeMs?: number;
  defaultStaleIfErrorMs?: number;
  requestRateWindowMs?: number;
  maximumRequestsPerResourceWindow?: number;
  circuitOpenMs?: number;
  subscriptionRetryPolicy?: Partial<PorticoEventRetryPolicy>;
  runtime?: Partial<ViewerSyncRuntime>;
}

export interface ViewerSyncMetricsSnapshot {
  logicalSubscriptionLeases: number;
  physicalSubscriptionStarts: number;
  physicalSubscriptionStops: number;
  subscriptionRetries: number;
  subscriptionAborts: number;
  lifecycleEvents: number;
  invalidationsReceived: number;
  invalidationBatches: number;
  authoritativeRefreshes: number;
  queriesRequested: number;
  queryCacheHits: number;
  queryStaleFallbacks: number;
  querySingleflightJoins: number;
  queryExecutions: number;
  queryFailures: number;
  circuitRejections: number;
  cacheEvictions: number;
  activeLogicalSubscriptions: number;
  activePhysicalSubscriptions: number;
  activeResourceRegistrations: number;
  cacheEntries: number;
  cacheWeight: number;
  inflightQueries: number;
}

export interface ViewerSyncResourceMetrics {
  subscriptionStarts: number;
  subscriptionRetries: number;
  lifecycleEvents: number;
  refreshes: number;
  queries: number;
  cacheHits: number;
  staleFallbacks: number;
  singleflightJoins: number;
  executions: number;
  failures: number;
  circuitRejections: number;
}

export interface ViewerSyncInvalidationBatch {
  tags: ReadonlySet<string>;
  /** Monotonic within this coordinator/generation. */
  sequence: number;
  isCurrent(): boolean;
}

export interface ViewerSyncResourceRegistration {
  key: string;
  tags: readonly string[];
  priority?: Exclude<ViewerSyncWorkPriority, "playback-continuity">;
  /** Latest-wins authoritative refresh. It may safely read from query(). */
  refresh(batch: ViewerSyncInvalidationBatch, signal: PorticoAbortSignal): void | Promise<void>;
}

export interface ViewerSyncSubscriptionRegistration {
  key: string;
  priority?: ViewerSyncWorkPriority;
  /**
   * Runs one physical connection lifetime. A normally resolved connection is
   * treated as lost continuity and retried; it must stay pending while healthy.
   */
  start(signal: PorticoAbortSignal): Promise<void>;
}

export interface ViewerSyncQuery<T> {
  key: string;
  priority?: ViewerSyncWorkPriority;
  load(signal: PorticoAbortSignal): Promise<T>;
  maxAgeMs?: number;
  staleIfErrorMs?: number;
  weight?: number;
  signal?: PorticoAbortSignal;
}

export interface ViewerSyncSemanticAdapter<TEvent> {
  /** Stable logical stream name, such as `application` or `notifications`. */
  subscriptionKey: string;
  /** Maps transport events to the shared resource-tag vocabulary. */
  tagsForEvent(event: TEvent): readonly string[];
  /** Authorization and identity changes bypass ordinary coalescing. */
  urgencyForEvent?(event: TEvent): ViewerSyncInvalidationUrgency;
}

export interface ViewerSyncLease {
  release(): void;
}

export class ViewerSyncCircuitOpenError extends Error {
  readonly retryAt: number;

  constructor(resourceKey: string, retryAt: number) {
    super(`Portico temporarily paused repeated requests for ${resourceKey}`);
    this.name = "ViewerSyncCircuitOpenError";
    this.retryAt = retryAt;
  }
}

export class ViewerSyncOverloadedError extends Error {
  constructor(message = "Portico is already refreshing the maximum number of resources") {
    super(message);
    this.name = "ViewerSyncOverloadedError";
  }
}

type MutableMetrics = Omit<ViewerSyncMetricsSnapshot,
  "activeLogicalSubscriptions" | "activePhysicalSubscriptions" |
  "activeResourceRegistrations" | "cacheEntries" | "cacheWeight" | "inflightQueries">;

type ResourceRegistration = {
  id: number;
  key: string;
  tags: Set<string>;
  refresh: ViewerSyncResourceRegistration["refresh"];
  priority: Exclude<ViewerSyncWorkPriority, "playback-continuity">;
  running: boolean;
  pending?: ViewerSyncInvalidationBatch;
  executing?: ViewerSyncInvalidationBatch;
  controller?: AbortController;
};

type SubscriptionState = {
  key: string;
  start: ViewerSyncSubscriptionRegistration["start"];
  priority: ViewerSyncWorkPriority;
  leases: number;
  controller?: AbortController;
  task?: Promise<void>;
  lingerTimer?: unknown;
  retryTimer?: unknown;
  failedAttempts: number;
  nextRetryAt: number;
  halted: boolean;
};

type CacheEntry = {
  value: unknown;
  storedAt: number;
  lastAccessedAt: number;
  weight: number;
};

type CircuitState = {
  requestStarts: number[];
  openUntil: number;
  lastAccessedAt: number;
};

type InflightQuery = { promise: Promise<unknown>; controller: AbortController; priority: ViewerSyncWorkPriority };

const defaultRuntime: ViewerSyncRuntime = Object.freeze({
  now: () => Date.now(),
  random: () => Math.random(),
  setTimer: (callback: () => void, delayMs: number) => setTimeout(callback, delayMs),
  clearTimer: (handle: unknown) => clearTimeout(handle as ReturnType<typeof setTimeout>)
});

const VIEWER_GLOBAL_FORBIDDEN_CODES = new Set([
  "profile_access_revoked",
  "profile_revoked",
  "server_session_revoked",
  "viewer_access_revoked"
]);

const terminalStatus = (reason: unknown): 401 | 403 | undefined => {
  if (!reason || typeof reason !== "object") return undefined;
  const { status, code } = reason as { status?: unknown; code?: unknown };
  if (status === 401) return status;
  return status === 403 && typeof code === "string" && VIEWER_GLOBAL_FORBIDDEN_CODES.has(code.trim().toLowerCase())
    ? status
    : undefined;
};

const isAbortError = (reason: unknown): boolean => Boolean(
  reason && typeof reason === "object" && (reason as { name?: unknown }).name === "AbortError"
);

function normalizedKey(value: string, name: string): string {
  const result = value.trim();
  if (!result || result.length > 512) throw new TypeError(`${name} is invalid`);
  return result;
}

function normalizedTag(value: string): string {
  const tag = value.trim().toLowerCase();
  if (tag === "*") return tag;
  if (!tag || tag.length > 128 || !/^[a-z0-9][a-z0-9:._/-]*$/.test(tag)) {
    throw new TypeError("viewer sync resource tag is invalid");
  }
  return tag;
}

function boundedNumber(value: number | undefined, fallback: number, minimum: number, maximum: number, name: string): number {
  const result = value ?? fallback;
  if (!Number.isFinite(result) || result < minimum || result > maximum) throw new RangeError(`${name} is invalid`);
  return result;
}

function abortedError(): Error {
  const error = new Error("The operation was aborted.");
  error.name = "AbortError";
  return error;
}

/**
 * Portable adapter helper: feed an already parsed event into one coordinator.
 * Web, React Native, Roku bridges, and future constrained clients can share the
 * same semantic tag mapping without sharing a rendering/query library.
 */
export function publishViewerSyncEvent<TEvent>(
  coordinator: ViewerSyncCoordinator,
  adapter: ViewerSyncSemanticAdapter<TEvent>,
  event: TEvent
): void {
  coordinator.invalidate(adapter.tagsForEvent(event), adapter.urgencyForEvent?.(event) ?? "coalesced");
}

/** One synchronization owner for one authenticated viewer generation. */
export class ViewerSyncCoordinator {
  readonly #fence: ViewerSyncGenerationFence;
  readonly #onLifecycleEvent: ViewerSyncOptions["onLifecycleEvent"];
  readonly #runtime: ViewerSyncRuntime;
  readonly #coalesceWindowMs: number;
  readonly #subscriptionLingerMs: number;
  readonly #subscriptionHealthyAfterMs: number;
  readonly #maximumLogicalSubscriptions: number;
  readonly #maximumResourceRegistrations: number;
  readonly #maximumPendingInvalidationTags: number;
  readonly #maximumInflightQueries: number;
  readonly #reservedPlaybackQuerySlots: number;
  readonly #maximumBackgroundInflightQueries: number;
  readonly #maximumCacheEntries: number;
  readonly #maximumCacheWeight: number;
  readonly #defaultCacheMaxAgeMs: number;
  readonly #defaultStaleIfErrorMs: number;
  readonly #requestRateWindowMs: number;
  readonly #maximumRequestsPerResourceWindow: number;
  readonly #circuitOpenMs: number;
  readonly #subscriptionRetryPolicy: Partial<PorticoEventRetryPolicy>;
  readonly #resources = new Map<number, ResourceRegistration>();
  readonly #resourcesByTag = new Map<string, Set<number>>();
  readonly #subscriptions = new Map<string, SubscriptionState>();
  readonly #cache = new Map<string, CacheEntry>();
  readonly #invalidationEpochs = new Map<string, number>();
  readonly #inflight = new Map<string, InflightQuery>();
  readonly #circuits = new Map<string, CircuitState>();
  readonly #resourceMetrics = new Map<string, ViewerSyncResourceMetrics>();
  readonly #metrics: MutableMetrics = {
    logicalSubscriptionLeases: 0,
    physicalSubscriptionStarts: 0,
    physicalSubscriptionStops: 0,
    subscriptionRetries: 0,
    subscriptionAborts: 0,
    lifecycleEvents: 0,
    invalidationsReceived: 0,
    invalidationBatches: 0,
    authoritativeRefreshes: 0,
    queriesRequested: 0,
    queryCacheHits: 0,
    queryStaleFallbacks: 0,
    querySingleflightJoins: 0,
    queryExecutions: 0,
    queryFailures: 0,
    circuitRejections: 0,
    cacheEvictions: 0
  };
  #foreground = true;
  #online = true;
  #playbackContinuityActive = false;
  #closed = false;
  #lifecycleTerminal = false;
  readonly #generationAbortController = new AbortController();
  #nextResourceID = 1;
  #invalidationSequence = 0;
  #pendingTags = new Set<string>();
  #coalesceTimer?: unknown;
  #cacheWeight = 0;
  #activeInteractiveRefreshes = 0;
  readonly #deferredBackgroundResources = new Set<number>();

  constructor(options: ViewerSyncOptions) {
    this.#fence = options.generationFence;
    this.#onLifecycleEvent = options.onLifecycleEvent;
    this.#runtime = { ...defaultRuntime, ...options.runtime };
    this.#coalesceWindowMs = boundedNumber(options.coalesceWindowMs, 120, 0, 5_000, "coalesce window");
    this.#subscriptionLingerMs = boundedNumber(options.subscriptionLingerMs, 1_500, 0, 60_000, "subscription linger");
    this.#subscriptionHealthyAfterMs = boundedNumber(options.subscriptionHealthyAfterMs, 30_000, 0, 3_600_000, "subscription healthy interval");
    this.#maximumLogicalSubscriptions = Math.trunc(boundedNumber(options.maximumLogicalSubscriptions, 16, 1, 1_000, "maximum logical subscriptions"));
    this.#maximumResourceRegistrations = Math.trunc(boundedNumber(options.maximumResourceRegistrations, 512, 1, 100_000, "maximum resource registrations"));
    this.#maximumPendingInvalidationTags = Math.trunc(boundedNumber(options.maximumPendingInvalidationTags, 256, 1, 10_000, "maximum pending invalidation tags"));
    this.#maximumInflightQueries = Math.trunc(boundedNumber(options.maximumInflightQueries, 32, 1, 10_000, "maximum inflight queries"));
    this.#reservedPlaybackQuerySlots = Math.trunc(boundedNumber(options.reservedPlaybackQuerySlots, 4, 0, this.#maximumInflightQueries, "reserved playback query slots"));
    this.#maximumBackgroundInflightQueries = Math.trunc(boundedNumber(options.maximumBackgroundInflightQueries, 4, 0, this.#maximumInflightQueries, "maximum background inflight queries"));
    this.#maximumCacheEntries = Math.trunc(boundedNumber(options.maximumCacheEntries, 256, 0, 100_000, "maximum cache entries"));
    this.#maximumCacheWeight = boundedNumber(options.maximumCacheWeight, 16 * 1024 * 1024, 0, Number.MAX_SAFE_INTEGER, "maximum cache weight");
    this.#defaultCacheMaxAgeMs = boundedNumber(options.defaultCacheMaxAgeMs, 30_000, 0, 86_400_000, "default cache max age");
    this.#defaultStaleIfErrorMs = boundedNumber(options.defaultStaleIfErrorMs, 300_000, 0, 86_400_000, "default stale-if-error");
    this.#requestRateWindowMs = boundedNumber(options.requestRateWindowMs, 10_000, 1_000, 300_000, "request-rate window");
    this.#maximumRequestsPerResourceWindow = Math.trunc(boundedNumber(options.maximumRequestsPerResourceWindow, 12, 1, 10_000, "maximum requests per resource window"));
    this.#circuitOpenMs = boundedNumber(options.circuitOpenMs, 15_000, 100, 300_000, "circuit open interval");
    this.#subscriptionRetryPolicy = options.subscriptionRetryPolicy ?? {};
  }

  get isCurrent(): boolean {
    const current = !this.#closed && Object.is(this.#fence.generation, this.#fence.currentGeneration());
    if (!current && !this.#closed) this.#abortForGenerationChange();
    return current;
  }

  /** Background/offline state stops physical streams but retains logical leases and retry history. */
  setRuntimeState(state: { foreground?: boolean; online?: boolean }): void {
    const wasRunnable = this.#canRun();
    if (state.foreground !== undefined) this.#foreground = state.foreground;
    if (state.online !== undefined) this.#online = state.online;
    if (!this.#canRun()) {
      for (const subscription of this.#subscriptions.values()) this.#stopPhysical(subscription, false);
      return;
    }
    if (wasRunnable) return;
    for (const subscription of this.#subscriptions.values()) {
      if (subscription.leases > 0) this.#scheduleSubscription(subscription);
    }
    // Reconcile all mounted resources once when continuity resumes. The normal
    // coalescer folds overlapping foreground and network events together.
    if (this.#resources.size > 0) this.invalidate(["runtime:reconcile"], "immediate", true);
  }

  /**
   * Protects an established player from app-sync competition. Playback-owned
   * subscriptions continue; background streams and refreshes reconcile later.
   */
  setPlaybackContinuityActive(active: boolean): void {
    if (this.#playbackContinuityActive === active) return;
    this.#playbackContinuityActive = active;
    if (active) {
      for (const subscription of this.#subscriptions.values()) {
        if (subscription.priority === "background") this.#stopPhysical(subscription, false);
      }
      for (const resource of this.#resources.values()) {
        if (resource.priority !== "background" || !resource.running) continue;
        if (!resource.pending && resource.executing) resource.pending = resource.executing;
        resource.controller?.abort();
        this.#deferredBackgroundResources.add(resource.id);
      }
      return;
    }
    for (const subscription of this.#subscriptions.values()) {
      if (subscription.leases > 0) this.#scheduleSubscription(subscription);
    }
    this.#drainDeferredBackgroundResources();
  }

  registerResource(registration: ViewerSyncResourceRegistration): ViewerSyncLease {
    this.#assertCurrent();
    if (this.#resources.size >= this.#maximumResourceRegistrations) throw new ViewerSyncOverloadedError("Portico has too many active resource registrations");
    const key = normalizedKey(registration.key, "viewer sync resource key");
    const tags = new Set(registration.tags.map(normalizedTag));
    if (tags.size === 0) throw new TypeError("viewer sync resource requires at least one tag");
    const id = this.#nextResourceID++;
    const resource: ResourceRegistration = {
      id,
      key,
      tags,
      refresh: registration.refresh,
      priority: registration.priority ?? "interactive",
      running: false
    };
    this.#resources.set(id, resource);
    for (const tag of tags) {
      const resources = this.#resourcesByTag.get(tag) ?? new Set<number>();
      resources.add(id);
      this.#resourcesByTag.set(tag, resources);
    }
    let released = false;
    return { release: () => {
      if (released) return;
      released = true;
      resource.controller?.abort();
      this.#resources.delete(id);
      this.#deferredBackgroundResources.delete(id);
      for (const tag of tags) {
        const resources = this.#resourcesByTag.get(tag);
        resources?.delete(id);
        if (resources?.size === 0) this.#resourcesByTag.delete(tag);
      }
    } };
  }

  /**
   * Invalidates matching resources. `runtime:reconcile` deliberately refreshes
   * every active resource after continuity loss without pretending every
   * domain changed.
   */
  invalidate(tags: readonly string[], urgency: ViewerSyncInvalidationUrgency = "coalesced", reconcileAll = false): void {
    if (!this.isCurrent || this.#lifecycleTerminal || tags.length === 0) return;
    for (const tag of tags) {
      if (this.#pendingTags.size >= this.#maximumPendingInvalidationTags) {
        this.#pendingTags.clear();
        this.#pendingTags.add("runtime:reconcile");
        break;
      }
      this.#pendingTags.add(normalizedTag(tag));
    }
    if (reconcileAll) this.#pendingTags.add("runtime:reconcile");
    this.#metrics.invalidationsReceived += 1;
    if (urgency === "immediate" || this.#coalesceWindowMs === 0) {
      this.#flushInvalidations();
      return;
    }
    if (this.#coalesceTimer === undefined) {
      this.#coalesceTimer = this.#runtime.setTimer(() => {
        this.#coalesceTimer = undefined;
        this.#flushInvalidations();
      }, this.#coalesceWindowMs);
    }
  }

  leaseSubscription(registration: ViewerSyncSubscriptionRegistration): ViewerSyncLease {
    this.#assertCurrent();
    const key = normalizedKey(registration.key, "viewer sync subscription key");
    let state = this.#subscriptions.get(key);
    if (!state) {
      if (this.#subscriptions.size >= this.#maximumLogicalSubscriptions) throw new ViewerSyncOverloadedError("Portico has too many logical subscriptions");
      state = { key, start: registration.start, priority: registration.priority ?? "background", leases: 0, failedAttempts: 0, nextRetryAt: 0, halted: false };
      this.#subscriptions.set(key, state);
    } else {
      const priority = registration.priority ?? "background";
      if (state.priority !== priority) throw new TypeError(`viewer sync subscription ${key} changed workload priority`);
      // Render frameworks commonly recreate function identities on remount.
      // Keep the active physical connection; use the newest driver next time.
      state.start = registration.start;
    }
    if (state.lingerTimer !== undefined) {
      this.#runtime.clearTimer(state.lingerTimer);
      state.lingerTimer = undefined;
    }
    // A completely released logical lease is a new consumer attempt. It may
    // retry a subscription that the prior lease halted on a permanent,
    // endpoint-scoped failure; an active lease must never spin on that failure.
    if (state.leases === 0) state.halted = false;
    state.leases += 1;
    this.#metrics.logicalSubscriptionLeases += 1;
    this.#scheduleSubscription(state);
    let released = false;
    return { release: () => {
      if (released) return;
      released = true;
      const current = this.#subscriptions.get(key);
      if (!current) return;
      current.leases = Math.max(0, current.leases - 1);
      if (current.leases > 0) return;
      current.lingerTimer = this.#runtime.setTimer(() => {
        current.lingerTimer = undefined;
        if (current.leases > 0) return;
        this.#stopPhysical(current, true);
        this.#subscriptions.delete(key);
      }, this.#subscriptionLingerMs);
    } };
  }

  async query<T>(request: ViewerSyncQuery<T>): Promise<T> {
    this.#assertCurrent();
    const key = normalizedKey(request.key, "viewer sync query key");
    const priority = request.priority ?? "interactive";
    if (request.signal?.aborted) throw abortedError();
    const now = this.#runtime.now();
    const maxAgeMs = boundedNumber(request.maxAgeMs, this.#defaultCacheMaxAgeMs, 0, 86_400_000, "query max age");
    const staleIfErrorMs = boundedNumber(request.staleIfErrorMs, this.#defaultStaleIfErrorMs, 0, 86_400_000, "query stale-if-error");
    const weight = boundedNumber(request.weight, 1, 0, Number.MAX_SAFE_INTEGER, "query cache weight");
    this.#metrics.queriesRequested += 1;
    this.#bumpResource(key, "queries");
    const cached = this.#cache.get(key);
    if (cached && now - cached.storedAt <= maxAgeMs) {
      cached.lastAccessedAt = now;
      this.#metrics.queryCacheHits += 1;
      this.#bumpResource(key, "cacheHits");
      return cached.value as T;
    }
    const existing = this.#inflight.get(key);
    if (existing) {
      this.#metrics.querySingleflightJoins += 1;
      this.#bumpResource(key, "singleflightJoins");
      return this.#joinWithAbort(existing.promise as Promise<T>, request.signal);
    }
    const backgroundInflight = [...this.#inflight.values()].filter(entry => entry.priority === "background").length;
    const ordinaryCapacity = this.#maximumInflightQueries - this.#reservedPlaybackQuerySlots;
    const priorityBlocked = (priority === "background" && (this.#playbackContinuityActive || backgroundInflight >= this.#maximumBackgroundInflightQueries))
      || (priority !== "playback-continuity" && this.#inflight.size >= ordinaryCapacity)
      || this.#inflight.size >= this.#maximumInflightQueries;
    if (priorityBlocked) {
      if (cached && now - cached.storedAt <= maxAgeMs + staleIfErrorMs) {
        cached.lastAccessedAt = now;
        this.#metrics.queryStaleFallbacks += 1;
        this.#bumpResource(key, "staleFallbacks");
        return cached.value as T;
      }
      throw new ViewerSyncOverloadedError();
    }
    try {
      this.#admitRequest(key, now);
    } catch (reason) {
      if (reason instanceof ViewerSyncCircuitOpenError && cached && now - cached.storedAt <= maxAgeMs + staleIfErrorMs) {
        cached.lastAccessedAt = now;
        this.#metrics.queryStaleFallbacks += 1;
        this.#bumpResource(key, "staleFallbacks");
        return cached.value as T;
      }
      throw reason;
    }
    const controller = new AbortController();
    const invalidationEpoch = this.#invalidationEpochs.get(key) ?? 0;
    const task = (async () => {
      this.#metrics.queryExecutions += 1;
      this.#bumpResource(key, "executions");
      try {
        const value = await request.load(controller.signal);
        if (!this.isCurrent) throw abortedError();
        if ((this.#invalidationEpochs.get(key) ?? 0) === invalidationEpoch) {
          this.#storeCache(key, value, weight);
        }
        return value;
      } catch (reason) {
        this.#metrics.queryFailures += 1;
        this.#bumpResource(key, "failures");
        const status = terminalStatus(reason);
        if (status !== undefined) {
          this.#publishLifecycle(status, key, reason);
          throw reason;
        }
        if (!this.isCurrent || this.#lifecycleTerminal) throw abortedError();
        const fallback = this.#cache.get(key);
        if (fallback && this.#runtime.now() - fallback.storedAt <= maxAgeMs + staleIfErrorMs) {
          fallback.lastAccessedAt = this.#runtime.now();
          this.#metrics.queryStaleFallbacks += 1;
          this.#bumpResource(key, "staleFallbacks");
          return fallback.value as T;
        }
        throw reason;
      } finally {
        this.#inflight.delete(key);
        this.#invalidationEpochs.delete(key);
      }
    })();
    this.#inflight.set(key, { promise: task, controller, priority });
    return this.#joinWithAbort(task, request.signal);
  }

  invalidateQuery(key: string): void {
    const normalized = normalizedKey(key, "viewer sync query key");
    this.#invalidationEpochs.set(normalized, (this.#invalidationEpochs.get(normalized) ?? 0) + 1);
    const entry = this.#cache.get(normalized);
    if (!entry) return;
    this.#cache.delete(normalized);
    this.#cacheWeight -= entry.weight;
  }

  invalidateQueries(prefix?: string): void {
    if (prefix === undefined) {
      for (const key of this.#inflight.keys()) this.invalidateQuery(key);
      this.#cache.clear();
      this.#cacheWeight = 0;
      return;
    }
    const normalized = normalizedKey(prefix, "viewer sync query prefix");
    const candidates = new Set([...this.#cache.keys(), ...this.#inflight.keys()]);
    for (const key of candidates) {
      if (key.startsWith(normalized)) this.invalidateQuery(key);
    }
  }

  close(): void {
    if (this.#closed) return;
    this.#closed = true;
    this.#generationAbortController.abort();
    if (this.#coalesceTimer !== undefined) this.#runtime.clearTimer(this.#coalesceTimer);
    this.#coalesceTimer = undefined;
    for (const subscription of this.#subscriptions.values()) {
      if (subscription.lingerTimer !== undefined) this.#runtime.clearTimer(subscription.lingerTimer);
      if (subscription.retryTimer !== undefined) this.#runtime.clearTimer(subscription.retryTimer);
      this.#stopPhysical(subscription, true);
    }
    for (const query of this.#inflight.values()) query.controller.abort();
    for (const resource of this.#resources.values()) resource.controller?.abort();
    this.#subscriptions.clear();
    this.#resources.clear();
    this.#resourcesByTag.clear();
    this.#cache.clear();
    this.#invalidationEpochs.clear();
    this.#cacheWeight = 0;
    this.#pendingTags.clear();
    this.#deferredBackgroundResources.clear();
  }

  metrics(): ViewerSyncMetricsSnapshot {
    let activePhysicalSubscriptions = 0;
    let activeLogicalSubscriptions = 0;
    for (const state of this.#subscriptions.values()) {
      if (state.leases > 0) activeLogicalSubscriptions += 1;
      if (state.task) activePhysicalSubscriptions += 1;
    }
    return Object.freeze({
      ...this.#metrics,
      activeLogicalSubscriptions,
      activePhysicalSubscriptions,
      activeResourceRegistrations: this.#resources.size,
      cacheEntries: this.#cache.size,
      cacheWeight: this.#cacheWeight,
      inflightQueries: this.#inflight.size
    });
  }

  /** Bounded diagnostic counters suitable for platform telemetry adapters. */
  resourceMetrics(): Readonly<Record<string, Readonly<ViewerSyncResourceMetrics>>> {
    return Object.freeze(Object.fromEntries(
      [...this.#resourceMetrics].map(([key, value]) => [key, Object.freeze({ ...value })])
    ));
  }

  #canRun(): boolean {
    return this.isCurrent && !this.#lifecycleTerminal && this.#foreground && this.#online;
  }

  #abortForGenerationChange(): void {
    this.#generationAbortController.abort();
    if (this.#coalesceTimer !== undefined) this.#runtime.clearTimer(this.#coalesceTimer);
    this.#coalesceTimer = undefined;
    this.#pendingTags.clear();
    for (const state of this.#subscriptions.values()) this.#stopPhysical(state, true);
    for (const query of this.#inflight.values()) query.controller.abort();
    for (const resource of this.#resources.values()) resource.controller?.abort();
  }

  #canRunSubscription(state: SubscriptionState): boolean {
    return this.#canRun() && !(this.#playbackContinuityActive && state.priority === "background");
  }

  #assertCurrent(): void {
    if (!this.isCurrent || this.#lifecycleTerminal) throw abortedError();
  }

  #flushInvalidations(): void {
    if (this.#coalesceTimer !== undefined) this.#runtime.clearTimer(this.#coalesceTimer);
    this.#coalesceTimer = undefined;
    if (!this.isCurrent || this.#pendingTags.size === 0) return;
    const tags = new Set(this.#pendingTags);
    this.#pendingTags.clear();
    const batch: ViewerSyncInvalidationBatch = Object.freeze({
      tags,
      sequence: ++this.#invalidationSequence,
      isCurrent: () => this.isCurrent && batch.sequence === this.#invalidationSequence
    });
    this.#metrics.invalidationBatches += 1;
    const ids = new Set<number>();
    if (tags.has("runtime:reconcile")) {
      for (const id of this.#resources.keys()) ids.add(id);
    } else {
      for (const tag of tags) for (const id of this.#resourcesByTag.get(tag) ?? []) ids.add(id);
      for (const id of this.#resourcesByTag.get("*") ?? []) ids.add(id);
    }
    const resources = [...ids].map(id => this.#resources.get(id)).filter((resource): resource is ResourceRegistration => Boolean(resource));
    resources.sort((left, right) => left.priority === right.priority ? 0 : left.priority === "interactive" ? -1 : 1);
    for (const resource of resources) {
      this.#queueResourceRefresh(resource, batch);
    }
  }

  #queueResourceRefresh(resource: ResourceRegistration, batch: ViewerSyncInvalidationBatch): void {
    resource.pending = batch;
    if (resource.running) return;
    if (resource.priority === "background" && (this.#playbackContinuityActive || this.#activeInteractiveRefreshes > 0)) {
      this.#deferredBackgroundResources.add(resource.id);
      return;
    }
    resource.running = true;
    void (async () => {
      try {
        while (resource.pending && this.isCurrent && !this.#lifecycleTerminal && this.#resources.has(resource.id)) {
          if (resource.priority === "background" && (this.#playbackContinuityActive || this.#activeInteractiveRefreshes > 0)) {
            this.#deferredBackgroundResources.add(resource.id);
            break;
          }
          const current = resource.pending;
          resource.pending = undefined;
          resource.executing = current;
          const controller = new AbortController();
          resource.controller = controller;
          this.#metrics.authoritativeRefreshes += 1;
          this.#bumpResource(resource.key, "refreshes");
          if (resource.priority === "interactive") this.#activeInteractiveRefreshes += 1;
          try {
            await resource.refresh(current, controller.signal);
          } catch (reason) {
            const status = terminalStatus(reason);
            if (status !== undefined) this.#publishLifecycle(status, resource.key, reason);
            // Ordinary fetch failures remain with the platform's visible stale
            // state; a later event or foreground reconcile retries them.
          } finally {
            resource.executing = undefined;
            resource.controller = undefined;
            if (resource.priority === "interactive") this.#activeInteractiveRefreshes -= 1;
          }
        }
      } finally {
        resource.running = false;
        this.#drainDeferredBackgroundResources();
      }
    })();
  }

  #drainDeferredBackgroundResources(): void {
    if (this.#playbackContinuityActive || this.#activeInteractiveRefreshes > 0 || !this.isCurrent) return;
    for (const id of [...this.#deferredBackgroundResources]) {
      const resource = this.#resources.get(id);
      if (!resource?.pending) {
        this.#deferredBackgroundResources.delete(id);
        continue;
      }
      this.#deferredBackgroundResources.delete(id);
      this.#queueResourceRefresh(resource, resource.pending);
    }
  }

  #scheduleSubscription(state: SubscriptionState): void {
    if (!this.#canRunSubscription(state) || state.halted || state.leases === 0 || state.task || state.retryTimer !== undefined) return;
    const delay = Math.max(0, state.nextRetryAt - this.#runtime.now());
    if (delay > 0) {
      state.retryTimer = this.#runtime.setTimer(() => {
        state.retryTimer = undefined;
        this.#scheduleSubscription(state);
      }, delay);
      return;
    }
    const controller = new AbortController();
    const startedAt = this.#runtime.now();
    state.controller = controller;
    this.#metrics.physicalSubscriptionStarts += 1;
    this.#bumpResource(state.key, "subscriptionStarts");
    state.task = (async () => {
      try {
        await state.start(controller.signal);
        if (controller.signal.aborted || !this.#canRunSubscription(state) || state.leases === 0) return;
        throw new Error("Portico subscription ended without an abort");
      } catch (reason) {
        if (controller.signal.aborted || isAbortError(reason) || !this.#canRunSubscription(state) || state.leases === 0) return;
        const status = terminalStatus(reason);
        if (status !== undefined) {
          this.#publishLifecycle(status, state.key, reason);
          return;
        }
        if (!porticoEventFailureIsRetryable(reason)) {
          state.halted = true;
          return;
        }
        if (this.#runtime.now() - startedAt >= this.#subscriptionHealthyAfterMs) state.failedAttempts = 0;
        const delay = porticoEventRetryDelay(reason, state.failedAttempts, this.#subscriptionRetryPolicy, this.#runtime.random);
        state.failedAttempts += 1;
        state.nextRetryAt = this.#runtime.now() + delay;
        this.#metrics.subscriptionRetries += 1;
        this.#bumpResource(state.key, "subscriptionRetries");
      } finally {
        if (controller.signal.aborted && this.#runtime.now() - startedAt >= this.#subscriptionHealthyAfterMs) {
          state.failedAttempts = 0;
          state.nextRetryAt = 0;
        }
        state.controller = undefined;
        state.task = undefined;
        this.#metrics.physicalSubscriptionStops += 1;
        if (this.#canRunSubscription(state) && state.leases > 0) this.#scheduleSubscription(state);
      }
    })();
  }

  #stopPhysical(state: SubscriptionState, countAbort: boolean): void {
    if (state.retryTimer !== undefined) {
      this.#runtime.clearTimer(state.retryTimer);
      state.retryTimer = undefined;
    }
    if (state.controller && !state.controller.signal.aborted) {
      state.controller.abort();
      if (countAbort) this.#metrics.subscriptionAborts += 1;
    }
  }

  #publishLifecycle(status: 401 | 403, resourceKey: string, cause: unknown): void {
    if (!this.isCurrent || this.#lifecycleTerminal) return;
    this.#lifecycleTerminal = true;
    this.#metrics.lifecycleEvents += 1;
    this.#bumpResource(resourceKey, "lifecycleEvents");
    if (this.#coalesceTimer !== undefined) this.#runtime.clearTimer(this.#coalesceTimer);
    this.#coalesceTimer = undefined;
    this.#pendingTags.clear();
    // A terminal viewer failure must not leave other resource loops running.
    for (const state of this.#subscriptions.values()) this.#stopPhysical(state, true);
    for (const query of this.#inflight.values()) query.controller.abort();
    for (const resource of this.#resources.values()) resource.controller?.abort();
    this.#deferredBackgroundResources.clear();
    this.#onLifecycleEvent({ reason: status === 401 ? "unauthenticated" : "forbidden", status, resourceKey, cause });
  }

  #admitRequest(key: string, now: number): void {
    const circuit = this.#circuits.get(key) ?? { requestStarts: [], openUntil: 0, lastAccessedAt: now };
    circuit.lastAccessedAt = now;
    circuit.requestStarts = circuit.requestStarts.filter(start => now - start < this.#requestRateWindowMs);
    if (circuit.openUntil > now) {
      this.#circuits.set(key, circuit);
      this.#metrics.circuitRejections += 1;
      this.#bumpResource(key, "circuitRejections");
      throw new ViewerSyncCircuitOpenError(key, circuit.openUntil);
    }
    if (circuit.requestStarts.length >= this.#maximumRequestsPerResourceWindow) {
      circuit.openUntil = now + this.#circuitOpenMs;
      circuit.requestStarts = [];
      this.#circuits.set(key, circuit);
      this.#metrics.circuitRejections += 1;
      this.#bumpResource(key, "circuitRejections");
      throw new ViewerSyncCircuitOpenError(key, circuit.openUntil);
    }
    circuit.openUntil = 0;
    circuit.requestStarts.push(now);
    this.#circuits.set(key, circuit);
    while (this.#circuits.size > Math.max(256, this.#maximumCacheEntries * 2)) {
      let oldestKey: string | undefined;
      let oldest = Number.POSITIVE_INFINITY;
      for (const [candidateKey, candidate] of this.#circuits) {
        if (candidate.lastAccessedAt < oldest) {
          oldest = candidate.lastAccessedAt;
          oldestKey = candidateKey;
        }
      }
      if (oldestKey === undefined) break;
      this.#circuits.delete(oldestKey);
    }
  }

  #storeCache(key: string, value: unknown, weight: number): void {
    const previous = this.#cache.get(key);
    if (previous) this.#cacheWeight -= previous.weight;
    const now = this.#runtime.now();
    this.#cache.delete(key);
    this.#cache.set(key, { value, storedAt: now, lastAccessedAt: now, weight });
    this.#cacheWeight += weight;
    while (this.#cache.size > this.#maximumCacheEntries || this.#cacheWeight > this.#maximumCacheWeight) {
      let oldestKey: string | undefined;
      let oldest = Number.POSITIVE_INFINITY;
      for (const [candidateKey, entry] of this.#cache) {
        if (entry.lastAccessedAt < oldest) {
          oldest = entry.lastAccessedAt;
          oldestKey = candidateKey;
        }
      }
      if (oldestKey === undefined) break;
      const entry = this.#cache.get(oldestKey)!;
      this.#cache.delete(oldestKey);
      this.#cacheWeight -= entry.weight;
      this.#metrics.cacheEvictions += 1;
    }
  }

  #joinWithAbort<T>(promise: Promise<T>, signal: PorticoAbortSignal | undefined): Promise<T> {
    const signals: PorticoAbortSignal[] = [this.#generationAbortController.signal];
    if (signal) signals.push(signal);
    if (signals.some(candidate => candidate.aborted)) return Promise.reject(abortedError());
    return new Promise<T>((resolve, reject) => {
      const cleanup = () => {
        for (const candidate of signals) candidate.removeEventListener("abort", onAbort);
      };
      const onAbort = () => {
        cleanup();
        reject(abortedError());
      };
      for (const candidate of signals) candidate.addEventListener("abort", onAbort, { once: true });
      promise.then(value => {
        cleanup();
        resolve(value);
      }, reason => {
        cleanup();
        reject(reason);
      });
    });
  }

  #bumpResource(key: string, metric: keyof ViewerSyncResourceMetrics): void {
    let value = this.#resourceMetrics.get(key);
    if (!value) {
      value = {
        subscriptionStarts: 0,
        subscriptionRetries: 0,
        lifecycleEvents: 0,
        refreshes: 0,
        queries: 0,
        cacheHits: 0,
        staleFallbacks: 0,
        singleflightJoins: 0,
        executions: 0,
        failures: 0,
        circuitRejections: 0
      };
      this.#resourceMetrics.set(key, value);
      while (this.#resourceMetrics.size > 1_024) {
        const oldestKey = this.#resourceMetrics.keys().next().value as string | undefined;
        if (oldestKey === undefined) break;
        this.#resourceMetrics.delete(oldestKey);
      }
    }
    value[metric] += 1;
  }
}
