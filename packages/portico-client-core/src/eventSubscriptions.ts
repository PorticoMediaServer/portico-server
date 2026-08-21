/** Transport-neutral subscriptions for Portico's authorized viewer event surfaces. */

export const PORTICO_EVENT_ENVELOPE_VERSION = "v1" as const;
export const PORTICO_LONG_POLL_DEFAULT_WAIT_SECONDS = 20;
export const PORTICO_LONG_POLL_MAXIMUM_WAIT_SECONDS = 25;

export type PorticoEventTransport = "sse" | "long-poll";
export type PorticoViewerGeneration = string | number;

/**
 * Portable subset of the web AbortSignal contract used by Client Core.
 * Native AbortSignal instances satisfy this shape, while consumers do not
 * need the DOM declaration library merely to import subscription types.
 */
export interface PorticoAbortSignal {
  readonly aborted: boolean;
  addEventListener(type: "abort", listener: () => void, options?: boolean | { once?: boolean }): void;
  removeEventListener(type: "abort", listener: () => void, options?: boolean | { capture?: boolean }): void;
}

export interface PorticoEventPublicationFence {
  /** Generation captured when the logical subscription is created. */
  generation: PorticoViewerGeneration;
  /** Synchronous current generation, advanced before viewer teardown starts. */
  currentGeneration(): PorticoViewerGeneration;
}

export interface PorticoLongPollEnvelope<TEvent> {
  version: typeof PORTICO_EVENT_ENVELOPE_VERSION;
  cursor: string;
  serverTime: string;
  resetRequired: boolean;
  hasMore: boolean;
  events: TEvent[];
}

export interface PorticoLongPollRequest {
  cursor?: string;
  waitSeconds: number;
}

export interface PorticoEventResetContext {
  transport: PorticoEventTransport;
  cursor: string;
  serverTime: string;
  /** Recheck immediately before publishing the authoritative refetch result. */
  isCurrent(): boolean;
}

export interface PorticoEventRetryPolicy {
  initialDelayMs: number;
  maximumDelayMs: number;
  maximumRetryAfterMs: number;
  jitterRatio: number;
  maximumImmediateDrains: number;
}

export interface PorticoEventSubscriptionOptions<TEvent> {
  /** Deliberate selection. Callers must not downgrade after ordinary failures. */
  transport: PorticoEventTransport;
  signal: PorticoAbortSignal;
  publicationFence: PorticoEventPublicationFence;
  onEvent(event: TEvent): void;
  /** Refetches the authoritative route-specific state after continuity is lost. */
  onResetRequired(context: PorticoEventResetContext): void | Promise<void>;
  waitSeconds?: number;
  retryPolicy?: Partial<PorticoEventRetryPolicy>;
}

export interface PorticoEventSubscriptionDriver<TEvent> {
  /**
   * Opens one SSE response. onConnected runs after the authorized response is
   * established but before any queued frame is published. This lets Client
   * Core repair authoritative state after a reconnect without leaving a race
   * between the repair and the newly-open stream.
   */
  stream(signal: PorticoAbortSignal, onEvent: (event: TEvent) => void, onConnected?: () => void | Promise<void>): Promise<void>;
  /** Performs one JSON long-poll request. */
  poll(request: PorticoLongPollRequest, signal: PorticoAbortSignal): Promise<unknown>;
  /** Validates one route-specific logical event. */
  parseEvent(value: unknown): TEvent;
  /** Optional idempotent identity used to suppress replay after reconnect/retry. */
  eventIdentity?(event: TEvent): string | number | undefined;
}

export interface PorticoEventSubscriptionRuntime {
  random(): number;
  sleep(delayMs: number, signal: PorticoAbortSignal): Promise<void>;
}

const defaultRetryPolicy: PorticoEventRetryPolicy = Object.freeze({
  initialDelayMs: 500,
  maximumDelayMs: 30_000,
  maximumRetryAfterMs: 120_000,
  jitterRatio: 0.2,
  maximumImmediateDrains: 32
});

function abortError(): Error {
  if (typeof DOMException !== "undefined") return new DOMException("The operation was aborted.", "AbortError");
  const error = new Error("The operation was aborted.");
  error.name = "AbortError";
  return error;
}

function isAbortError(reason: unknown): boolean {
  return Boolean(reason && typeof reason === "object" && (reason as { name?: unknown }).name === "AbortError");
}

function defaultSleep(delayMs: number, signal: PorticoAbortSignal): Promise<void> {
  if (signal.aborted) return Promise.reject(abortError());
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      signal.removeEventListener("abort", onAbort);
      resolve();
    }, Math.max(0, delayMs));
    const onAbort = () => {
      clearTimeout(timer);
      signal.removeEventListener("abort", onAbort);
      reject(abortError());
    };
    signal.addEventListener("abort", onAbort, { once: true });
  });
}

const defaultRuntime: PorticoEventSubscriptionRuntime = Object.freeze({
  random: () => Math.random(),
  sleep: defaultSleep
});

/**
 * A transport adapter normally observes the signal itself, but the logical
 * subscription must still settle when an adapter or reset handler does not.
 * The underlying promise is deliberately left running; its callbacks are
 * fenced by the signal and the adapter owns any platform-specific teardown.
 */
function abortable<T>(operation: PromiseLike<T>, signal: PorticoAbortSignal): Promise<T> {
  const promise = Promise.resolve(operation);
  if (signal.aborted) {
    void promise.catch(() => undefined);
    return Promise.reject(abortError());
  }
  return new Promise<T>((resolve, reject) => {
    let settled = false;
    const cleanup = () => signal.removeEventListener("abort", onAbort);
    const onAbort = () => {
      if (settled) return;
      settled = true;
      cleanup();
      reject(abortError());
    };
    signal.addEventListener("abort", onAbort, { once: true });
    void promise.then(value => {
      if (settled) return;
      settled = true;
      cleanup();
      resolve(value);
    }, reason => {
      if (settled) return;
      settled = true;
      cleanup();
      reject(reason);
    });
  });
}

function startAbortable<T>(start: () => Promise<T>, signal: PorticoAbortSignal): Promise<T> {
  if (signal.aborted) return Promise.reject(abortError());
  let operation: Promise<T>;
  try {
    operation = Promise.resolve(start());
  } catch (reason) {
    operation = Promise.reject(reason);
  }
  return abortable(operation, signal);
}

async function sleepAbortably(
  runtime: PorticoEventSubscriptionRuntime,
  delayMs: number,
  signal: PorticoAbortSignal
): Promise<boolean> {
  try {
    await startAbortable(() => runtime.sleep(delayMs, signal), signal);
    return true;
  } catch (reason) {
    if (signal.aborted || isAbortError(reason)) return false;
    throw reason;
  }
}

/** A malformed event response is a contract failure, not a reconnect signal. */
export class PorticoEventProtocolError extends Error {
  constructor(message: string, cause?: unknown) {
    super(message, cause === undefined ? undefined : { cause });
    this.name = "PorticoEventProtocolError";
  }
}

class PorticoEventConsumerError extends Error {
  constructor(cause: unknown) {
    super("Portico event consumer rejected a publication", { cause });
    this.name = "PorticoEventConsumerError";
  }
}

function finiteInteger(value: number, name: string, minimum: number, maximum: number): number {
  if (!Number.isInteger(value) || value < minimum || value > maximum) throw new RangeError(`${name} must be an integer from ${minimum} through ${maximum}`);
  return value;
}

function normalizedRetryPolicy(input: Partial<PorticoEventRetryPolicy> | undefined): PorticoEventRetryPolicy {
  const policy = { ...defaultRetryPolicy, ...input };
  if (!Number.isFinite(policy.initialDelayMs) || policy.initialDelayMs < 0) throw new RangeError("initial event retry delay is invalid");
  if (!Number.isFinite(policy.maximumDelayMs) || policy.maximumDelayMs < policy.initialDelayMs) throw new RangeError("maximum event retry delay is invalid");
  if (!Number.isFinite(policy.maximumRetryAfterMs) || policy.maximumRetryAfterMs < 0) throw new RangeError("maximum Retry-After delay is invalid");
  if (!Number.isFinite(policy.jitterRatio) || policy.jitterRatio < 0 || policy.jitterRatio > 1) throw new RangeError("event retry jitter is invalid");
  finiteInteger(policy.maximumImmediateDrains, "maximum immediate event drains", 1, 1_000);
  return policy;
}

function isPublicationCurrent(fence: PorticoEventPublicationFence): boolean {
  return Object.is(fence.generation, fence.currentGeneration());
}

function eventHTTPStatus(reason: unknown): number | undefined {
  if (!reason || typeof reason !== "object") return undefined;
  const status = (reason as { status?: unknown }).status;
  return typeof status === "number" && Number.isFinite(status) ? status : undefined;
}

function eventErrorCode(reason: unknown): string | undefined {
  if (!reason || typeof reason !== "object") return undefined;
  const code = (reason as { code?: unknown }).code;
  return typeof code === "string" ? code : undefined;
}

function isRecoverableLongPollCursorFailure(reason: unknown): boolean {
  return eventHTTPStatus(reason) === 400 && eventErrorCode(reason) === "invalid_poll_cursor";
}

function retryAfterMilliseconds(reason: unknown): number | undefined {
  if (!reason || typeof reason !== "object") return undefined;
  const value = (reason as { retryAfterMs?: unknown }).retryAfterMs;
  return typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : undefined;
}

/** Authentication, authorization, and permanent client errors are lifecycle signals, not retry loops. */
export function porticoEventFailureIsRetryable(reason: unknown): boolean {
  if (isAbortError(reason)) return false;
  if (reason instanceof PorticoEventProtocolError || reason instanceof PorticoEventConsumerError) return false;
  const status = eventHTTPStatus(reason);
  if (status === undefined) return true;
  if (status === 408 || status === 425 || status === 429) return true;
  if (status >= 500) return true;
  return status < 400;
}

export function porticoEventRetryDelay(
  reason: unknown,
  failedAttempts: number,
  input: Partial<PorticoEventRetryPolicy> | undefined = undefined,
  random: () => number = defaultRuntime.random
): number {
  const policy = normalizedRetryPolicy(input);
  const status = eventHTTPStatus(reason);
  const retryAfter = retryAfterMilliseconds(reason);
  if (retryAfter !== undefined && (status === 429 || (status !== undefined && status >= 500))) {
    return Math.min(policy.maximumRetryAfterMs, retryAfter);
  }
  const exponent = Math.max(0, Math.min(30, Math.trunc(failedAttempts)));
  const base = Math.min(policy.maximumDelayMs, policy.initialDelayMs * (2 ** exponent));
  const rawSample = random();
  const sample = Number.isFinite(rawSample) ? Math.max(0, Math.min(1, rawSample)) : 0.5;
  const factor = 1 - policy.jitterRatio + (2 * policy.jitterRatio * sample);
  return Math.min(policy.maximumDelayMs, Math.max(0, Math.round(base * factor)));
}

function nonEmptyBoundedString(value: unknown, name: string, maximum: number): string {
  if (typeof value !== "string" || !value.trim() || value.length > maximum) throw new TypeError(`${name} is invalid`);
  return value;
}

/** Strict common-envelope parsing; route-specific event parsing happens before cursor publication. */
export function parsePorticoLongPollEnvelope(value: unknown): PorticoLongPollEnvelope<unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new TypeError("long-poll envelope is invalid");
  const source = value as Record<string, unknown>;
  if (source.version !== PORTICO_EVENT_ENVELOPE_VERSION) throw new TypeError("long-poll envelope version is unsupported");
  const cursor = nonEmptyBoundedString(source.cursor, "long-poll cursor", 4_096);
  const serverTime = nonEmptyBoundedString(source.serverTime, "long-poll server time", 64);
  if (!Number.isFinite(Date.parse(serverTime))) throw new TypeError("long-poll server time is invalid");
  if (typeof source.resetRequired !== "boolean" || typeof source.hasMore !== "boolean") throw new TypeError("long-poll continuity fields are invalid");
  if (!Array.isArray(source.events) || source.events.length > 500) throw new TypeError("long-poll events are invalid");
  return {
    version: PORTICO_EVENT_ENVELOPE_VERSION,
    cursor,
    serverTime,
    resetRequired: source.resetRequired,
    hasMore: source.hasMore,
    events: source.events
  };
}

class EventStreamEndedError extends Error {
  constructor() {
    super("Portico event stream ended");
    this.name = "EventStreamEndedError";
  }
}

class ImmediateDrainLimitError extends Error {
  constructor() {
    super("Portico long-poll drain limit was exceeded");
    this.name = "ImmediateDrainLimitError";
  }
}

/**
 * Runs one logical subscription. Transport selection is immutable for its
 * lifetime; failures never silently downgrade SSE to long polling.
 */
export async function runPorticoEventSubscription<TEvent>(
  options: PorticoEventSubscriptionOptions<TEvent>,
  driver: PorticoEventSubscriptionDriver<TEvent>,
  runtime: Partial<PorticoEventSubscriptionRuntime> = {}
): Promise<void> {
  const waitSeconds = finiteInteger(options.waitSeconds ?? PORTICO_LONG_POLL_DEFAULT_WAIT_SECONDS, "long-poll waitSeconds", 0, PORTICO_LONG_POLL_MAXIMUM_WAIT_SECONDS);
  const retryPolicy = normalizedRetryPolicy(options.retryPolicy);
  const subscriptionRuntime: PorticoEventSubscriptionRuntime = {
    random: runtime.random ?? defaultRuntime.random,
    sleep: runtime.sleep ?? defaultRuntime.sleep
  };
  let failedAttempts = 0;
  let cursor: string | undefined;
  let drainImmediately = false;
  let immediateDrains = 0;
  let sseContinuityLost = false;
  const publishedIdentities = new Set<string | number>();
  const publishedIdentityOrder: (string | number)[] = [];
  const clearPublishedIdentities = () => {
    publishedIdentities.clear();
    publishedIdentityOrder.length = 0;
  };
  const reset = (context: PorticoEventResetContext) => startAbortable(
    () => Promise.resolve(options.onResetRequired(context)),
    options.signal
  );
  const publish = (event: TEvent) => {
    if (options.signal.aborted || !isPublicationCurrent(options.publicationFence)) return;
    const identity = driver.eventIdentity?.(event);
    if (identity !== undefined) {
      if (publishedIdentities.has(identity)) return;
      publishedIdentities.add(identity);
      publishedIdentityOrder.push(identity);
      if (publishedIdentityOrder.length > 1_024) {
        const expired = publishedIdentityOrder.shift();
        if (expired !== undefined) publishedIdentities.delete(expired);
      }
    }
    try {
      options.onEvent(event);
    } catch (reason) {
      throw new PorticoEventConsumerError(reason);
    }
  };

  while (!options.signal.aborted && isPublicationCurrent(options.publicationFence)) {
    try {
      if (options.transport === "sse") {
        await startAbortable(() => driver.stream(options.signal, publish, async () => {
          if (sseContinuityLost) {
            await reset({
              transport: "sse",
              cursor: "",
              serverTime: new Date().toISOString(),
              isCurrent: () => !options.signal.aborted && isPublicationCurrent(options.publicationFence)
            });
            if (options.signal.aborted || !isPublicationCurrent(options.publicationFence)) return;
            clearPublishedIdentities();
            sseContinuityLost = false;
          }
          // A successfully authorized/opened stream is a healthy retry
          // boundary even if it later fails without returning normally.
          failedAttempts = 0;
        }), options.signal);
        if (options.signal.aborted || !isPublicationCurrent(options.publicationFence)) return;
        throw new EventStreamEndedError();
      }

      const rawEnvelope = await startAbortable(
        () => driver.poll({ cursor, waitSeconds: drainImmediately ? 0 : waitSeconds }, options.signal),
        options.signal
      );
      if (options.signal.aborted || !isPublicationCurrent(options.publicationFence)) return;
      let envelope: PorticoLongPollEnvelope<unknown>;
      let events: TEvent[];
      try {
        envelope = parsePorticoLongPollEnvelope(rawEnvelope);
        events = envelope.events.map(driver.parseEvent);
      } catch (reason) {
        throw new PorticoEventProtocolError("Portico event response did not match the advertised contract", reason);
      }
      if (envelope.resetRequired) {
        await reset({
          transport: "long-poll",
          cursor: envelope.cursor,
          serverTime: envelope.serverTime,
          isCurrent: () => !options.signal.aborted && isPublicationCurrent(options.publicationFence)
        });
        if (options.signal.aborted || !isPublicationCurrent(options.publicationFence)) return;
        // A reset may represent a new server boot whose event IDs legitimately
        // restart. The authoritative repair is the dedupe boundary.
        clearPublishedIdentities();
      } else {
        for (const event of events) {
          if (options.signal.aborted || !isPublicationCurrent(options.publicationFence)) return;
          publish(event);
        }
      }
      // Do not advance past a reset until its authoritative refetch succeeds.
      // If onResetRequired rejects, retrying with the prior cursor forces the
      // server to advertise the lost-continuity condition again.
      cursor = envelope.cursor;
      failedAttempts = 0;

      drainImmediately = envelope.hasMore;
      if (drainImmediately) {
        immediateDrains += 1;
        if (immediateDrains > retryPolicy.maximumImmediateDrains) throw new ImmediateDrainLimitError();
      } else {
        immediateDrains = 0;
      }
      // Empty timeout responses and hasMore drains are both reissued without delay.
    } catch (reason) {
      if (options.signal.aborted || isAbortError(reason) || !isPublicationCurrent(options.publicationFence)) return;
      if (options.transport === "long-poll" && isRecoverableLongPollCursorFailure(reason)) {
        // A cursor is integrity-bound to one server boot. A legitimate restart
        // is indistinguishable from a tampered old cursor once the ephemeral
        // signing key changes, so recover only by discarding it and performing
        // an authoritative refetch before opening a fresh poll sequence.
        try {
          await reset({
            transport: "long-poll",
            cursor: "",
            serverTime: new Date().toISOString(),
            isCurrent: () => !options.signal.aborted && isPublicationCurrent(options.publicationFence)
          });
        } catch (resetReason) {
          if (options.signal.aborted || isAbortError(resetReason) || !isPublicationCurrent(options.publicationFence)) return;
          if (!porticoEventFailureIsRetryable(resetReason)) throw resetReason;
          const delay = porticoEventRetryDelay(resetReason, failedAttempts, retryPolicy, subscriptionRuntime.random);
          failedAttempts += 1;
          if (!await sleepAbortably(subscriptionRuntime, delay, options.signal)) return;
          continue;
        }
        if (options.signal.aborted || !isPublicationCurrent(options.publicationFence)) return;
        cursor = undefined;
        clearPublishedIdentities();
        failedAttempts = 0;
        drainImmediately = false;
        immediateDrains = 0;
        continue;
      }
      if (!porticoEventFailureIsRetryable(reason)) throw reason;
      if (options.transport === "sse") sseContinuityLost = true;
      const delay = porticoEventRetryDelay(reason, failedAttempts, retryPolicy, subscriptionRuntime.random);
      failedAttempts += 1;
      drainImmediately = false;
      immediateDrains = 0;
      if (!await sleepAbortably(subscriptionRuntime, delay, options.signal)) return;
    }
  }
}

type ActiveSubscription = {
  controller: AbortController;
  done: Promise<void>;
};

/** Supersedes an older subscription before starting another request for the same logical key. */
export class PorticoEventSubscriptionCoordinator {
  readonly #active = new Map<string, ActiveSubscription>();

  async run(key: string, signal: PorticoAbortSignal, start: (signal: PorticoAbortSignal) => Promise<void>): Promise<void> {
    const normalizedKey = key.trim();
    if (!normalizedKey) throw new TypeError("event subscription key is required");
    if (signal.aborted) return;

    const previous = this.#active.get(normalizedKey);
    previous?.controller.abort();
    const controller = new AbortController();
    const onAbort = () => controller.abort();
    signal.addEventListener("abort", onAbort, { once: true });

    let resolveDone!: () => void;
    let rejectDone!: (reason: unknown) => void;
    const done = new Promise<void>((resolve, reject) => {
      resolveDone = resolve;
      rejectDone = reject;
    });
    const entry: ActiveSubscription = { controller, done };
    this.#active.set(normalizedKey, entry);

    void (async () => {
      try {
        await previous?.done.catch(() => undefined);
        if (controller.signal.aborted || this.#active.get(normalizedKey) !== entry) return;
        await start(controller.signal);
      } catch (reason) {
        if (!controller.signal.aborted) throw reason;
      }
    })().then(resolveDone, rejectDone).finally(() => {
      signal.removeEventListener("abort", onAbort);
      if (this.#active.get(normalizedKey) === entry) this.#active.delete(normalizedKey);
    });

    return done;
  }

  cancel(key: string): void {
    this.#active.get(key.trim())?.controller.abort();
  }

  cancelAll(): void {
    for (const subscription of this.#active.values()) subscription.controller.abort();
  }

  get activeCount(): number {
    return this.#active.size;
  }
}
