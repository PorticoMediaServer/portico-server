/**
 * Plays Playback Protocol v1 sessions through today's `PlaybackService` and its engines (web
 * `<video>` + hls.js, Apple AVPlayer), so the player screens don't change while the apps move to
 * v1 (Plan — Client Playback Migration, core step). `V1PlaybackApi` is the `PlaybackApi` the
 * service calls for a session's progress and stop, spoken as v1: the timeline (with its own
 * monotonic `seq`, generation and a lease keepalive while paused) and `DELETE` with the last
 * position. `toPlaybackSession` turns a v1 session into the service's session shape.
 */
import type {PlaybackApi, PlaybackSession} from '../index.ts';
import {idempotencyKey, PlaybackApiError, type V1Http} from './http.ts';
import {PlaybackOptionsClient} from './options.ts';
import {SessionsClient, type StartOptions, type StartTarget, type Timers} from './sessions.ts';
import type {TimelineReport, TimelineState} from './timeline.ts';
import {parseSessionEnd, type Session, type SessionEnd} from './types.ts';

/** The service's session for a v1 session: stream mode is HLS, direct stays direct. */
export function toPlaybackSession(session: Session, durationSeconds: number, resumeSeconds?: number): PlaybackSession {
  return {
    id: session.id,
    generation: session.presentation.generation,
    streamUrl: session.presentation.url,
    mode: session.presentation.mode === 'stream' ? 'hls' : 'direct',
    duration: Math.max(0, durationSeconds),
    resumeSeconds: resumeSeconds ?? session.presentation.startPositionMs / 1000,
    protocol: 'v1',
    // §18.1 version 2: the device decodes the file itself.
    ...(session.presentation.audio ? {audio: session.presentation.audio} : {}),
  };
}

type PendingReport = {seq?: number; body: Omit<TimelineReport, 'seq'>; at: number; promise: Promise<void>; resolve: () => void; reject: (error: unknown) => void};
const MAX_PENDING_REPORT_FACTS = 64;

type Tracked = {stopping?: boolean; stopPromise?: Promise<void>; ackEndGeneration?: number; retryUntil?: number; failures?: number; active?: PendingReport; pending?: PendingReport[]; reportAbort?: AbortController; seq: number; generation: number; positionMs: number; state: TimelineState; everyMs: number; timer?: unknown;
  /** The last report actually sent: when, where and in which state (the §6 cadence). */
  sentAt?: number; sentPositionMs?: number; sentState?: TimelineState; sentGeneration?: number;
  /** When `positionMs` was last set by the player. */
  positionAt?: number;
  /** A marker skip to carry on the next report (spec §4.2), sent once. */
  skipped?: NonNullable<TimelineReport['skipped']>};

export class V1PlaybackApi implements PlaybackApi {
  readonly sessions: SessionsClient;
  private readonly optionsClient: PlaybackOptionsClient;
  private readonly tracked = new Map<string, Tracked>();
  private readonly setTimer: (fn: () => void, ms: number) => unknown;
  private readonly clearTimer: (t: unknown) => void;
  private readonly key: () => string;
  private readonly now: () => number;
  private readonly random: () => number;
  private readonly endedListeners = new Set<(sessionId: string, end: SessionEnd) => void>();
  /** Sessions a committed audio edge ended on the server (§18.3): their last reports and end
   * notices are dropped, since the audio already belongs to the next session. */
  private readonly retired = new Set<string>();

  constructor(http: V1Http, options: {timers?: Timers; key?: () => string; now?: () => number; random?: () => number} = {}) {
    this.now = options.now ?? (() => Date.now());
    this.random = options.random ?? Math.random;
    this.sessions = new SessionsClient(http, options.timers);
    this.optionsClient = new PlaybackOptionsClient(http);
    this.setTimer = options.timers?.setTimer ?? ((fn, ms) => setTimeout(fn, ms));
    this.clearTimer = options.timers?.clearTimer ?? (t => clearTimeout(t as ReturnType<typeof setTimeout>));
    this.key = options.key ?? (() => idempotencyKey());
  }

  /** A title's duration from its playback options (the default version's), in seconds. */
  async duration(itemId: string, versionId?: string, signal?: AbortSignal): Promise<number> {
    try {
      const o = await this.optionsClient.options(itemId, {}, signal);
      const v = o.versions.find(x => x.id === (versionId ?? o.preferred?.versionId)) ?? o.versions[0];
      return v ? v.durationMs / 1000 : 0;
    } catch {
      return 0; // The engine reports the duration once it has the media.
    }
  }

  /** Starts a session and returns it in the service's shape; the session is then tracked. */
  async start(target: StartTarget, options: StartOptions, key: string = this.key(), durationSeconds?: number, signal?: AbortSignal): Promise<{session: Session; playback: PlaybackSession}> {
    const session = await this.sessions.start(target, options, key, {signal});
    return {session, playback: this.adopt(session, durationSeconds ?? await this.duration(session.itemId ?? '', session.versionId, signal))};
  }

  /** Tracks a session another request started (a queue create with startPlayback, an advance). */
  adopt(session: Session, durationSeconds: number): PlaybackSession {
    const prior = this.tracked.get(session.id);
    // A presentation change keeps the same session and its accepted facts.
    // Replacing this object would cancel a queued marker or end before its ack.
    const tracked = prior ?? {seq: 0, generation: session.presentation.generation, positionMs: session.presentation.startPositionMs, state: 'playing' as TimelineState, everyMs: session.lease.reportEveryMs};
    tracked.generation = session.presentation.generation;
    tracked.positionMs = session.presentation.startPositionMs;
    tracked.positionAt = this.now();
    tracked.state = session.state === 'paused' ? 'paused' : 'playing';
    tracked.everyMs = session.lease.reportEveryMs;
    this.tracked.set(session.id, tracked);
    return toPlaybackSession(session, durationSeconds);
  }

  createPlayback(itemId: string, requestId: string, signal?: AbortSignal): Promise<PlaybackSession> {
    return this.start({itemId}, {startFrom: 'resume', state: 'playing'}, requestId, undefined, signal).then(r => r.playback);
  }

  stopPlayback(id: string, signal?: AbortSignal): Promise<void> {
    const t = this.tracked.get(id);
    if (!t) return this.sessions.stop(id, undefined, signal);
    if (t.stopPromise) return t.stopPromise;
    t.stopping = true;
    if (t.timer !== undefined && !t.pending?.length) { this.clearTimer(t.timer); t.timer = undefined; }
    // A normal stop must not overtake already accepted marker/end/state facts.
    // Their retry queue survives the caller's cleanup deadline; explicit forget
    // remains the cancellation boundary for a replaced viewer/session.
    const facts = [t.active, ...(t.pending ?? [])].filter((p): p is PendingReport => !!p);
    const work = Promise.all(facts.map(p => p.promise)).then(async () => {
      if (this.tracked.get(id) !== t) return;
      await this.sessions.stop(id, t.positionMs, signal);
      if (this.tracked.get(id) === t) this.forget(id);
    });
    t.stopPromise = work.finally(() => { if (this.tracked.get(id) === t) t.stopPromise = undefined; });
    return t.stopPromise;
  }

  async progressPlayback(id: string, payload: {generation: number; sequence: number; positionSeconds: number; state: 'playing' | 'paused' | 'ended'}): Promise<void> {
    if (this.retired.has(id)) return;
    const t = this.tracked.get(id) ?? {seq: 0, generation: payload.generation, positionMs: 0, state: 'playing' as TimelineState, everyMs: 10_000};
    this.tracked.set(id, t);
    if (t.stopping) return;
    t.generation = payload.generation;
    t.positionMs = Math.max(0, Math.round(payload.positionSeconds * 1000));
    t.positionAt = this.now();
    // The end is reported once: a second "ended" for the same generation (the engine's own report,
    // then the queue's completion check) would reach a session the first one already ended (410)
    // and stall the queue on its way to the next entry.
    t.state = payload.state;
    if (payload.state === 'ended') {
      const pendingEnd = [t.active, ...(t.pending ?? [])].find(r => r?.body.state === 'ended' && r.body.generation === payload.generation);
      if (pendingEnd) return pendingEnd.promise;
      if (t.ackEndGeneration === payload.generation) return;
    }
    // PERF-24, spec §6 cadence: while playing steadily, the report timer carries the latest
    // position every `Report-Every-Ms` (10 s); a state or generation change, the first report and a
    // seek (the position no longer follows the clock) are sent at once.
    if (this.steady(t)) return;
    try {
      await this.report(id, t);
    } catch (e) {
      // An end the server already has (the session is over, 404/410) is confirmed, not a failure.
      if (payload.state === 'ended' && e instanceof PlaybackApiError && (e.status === 404 || e.status === 410 || e.code === 'session_ended')) return;
      throw e;
    }
  }

  /** The viewer skipped a marker (spec §4.2): reported at once, with the session's next report. */
  async markerSkipped(id: string, skipped: NonNullable<TimelineReport['skipped']>): Promise<void> {
    const t = this.tracked.get(id);
    if (!t || this.retired.has(id)) return;
    if ((t.pending?.length ?? 0) + (t.active ? 1 : 0) >= MAX_PENDING_REPORT_FACTS) throw new PlaybackApiError(429, 'timeline_pending_full', 'same_request');
    t.skipped = skipped;
    await this.report(id, t);
  }

  private steady(t: Tracked): boolean {
    const latest = t.pending?.at(-1) ?? t.active;
    const at = latest?.at ?? t.sentAt;
    const state = latest?.body.state ?? t.sentState;
    const generation = latest?.body.generation ?? t.sentGeneration;
    const position = latest?.body.positionMs ?? t.sentPositionMs ?? 0;
    if (at === undefined || (!latest && t.timer === undefined) || state !== t.state || generation !== t.generation) return false;
    const elapsed = this.now() - at;
    if (elapsed >= t.everyMs) return false;
    return Math.abs(t.positionMs - position - (t.state === 'playing' ? elapsed : 0)) <= 2000;
  }

  /** Keep timeline facts ordered while a slow server has one request outstanding.
   * Steady progress stays in the tracked position and is carried by the cadence timer;
   * state changes, seeks, generations and marker skips retain their own reports. */
  private report(id: string, t: Tracked): Promise<void> {
    if (this.tracked.get(id) !== t) return Promise.resolve();
    if ((t.pending?.length ?? 0) + (t.active ? 1 : 0) >= MAX_PENDING_REPORT_FACTS) return Promise.reject(new PlaybackApiError(429, 'timeline_pending_full', 'same_request'));
    if (t.timer !== undefined) this.clearTimer(t.timer);
    t.timer = undefined;
    const at = this.now();
    const positionMs = t.positionMs + (t.state === 'playing' && t.positionAt !== undefined ? Math.max(0, Math.min(5000, at - t.positionAt)) : 0);
    let resolve!: () => void, reject!: (error: unknown) => void;
    const promise = new Promise<void>((yes, no) => { resolve = yes; reject = no; });
    const body = {generation: t.generation, state: t.state, positionMs, rate: 1, ...(t.skipped ? {skipped: t.skipped} : {})};
    t.skipped = undefined;
    (t.pending ??= []).push({body, at, promise, resolve, reject});
    this.drain(id, t);
    return promise;
  }

  private drain(id: string, t: Tracked) {
    if (t.active || this.tracked.get(id) !== t) return;
    if (!t.pending?.length) return;
    const wait = (t.retryUntil ?? 0) - this.now();
    if (wait > 0) {
      t.timer = this.setTimer(() => { t.timer = undefined; this.drain(id, t); }, Math.min(wait, 2147483647));
      return;
    }
    const next = t.pending.shift()!;
    t.active = next;
    t.sentAt = next.at; t.sentPositionMs = next.body.positionMs; t.sentState = next.body.state; t.sentGeneration = next.body.generation;
    const controller = new AbortController(); t.reportAbort = controller;
    const deadline = this.setTimer(() => controller.abort(), 20000);
    void this.sessions.timeline(id, {seq: next.seq ??= ++t.seq, ...next.body}, controller.signal).then(r => {
      if (r.reportEveryMs) t.everyMs = r.reportEveryMs;
      t.failures = 0; t.retryUntil = undefined;
      if (next.body.state === 'ended') t.ackEndGeneration = next.body.generation;
      next.resolve();
    }, e => {
      t.failures = (t.failures ?? 0) + 1;
      const directed = e instanceof PlaybackApiError && Number.isFinite(e.retryAfterMs) && e.retryAfterMs! > 0 ? e.retryAfterMs! : 0;
      const delay = Math.max(directed, Math.min(30000, 1000 * 2 ** Math.min(t.failures - 1, 5)));
      t.retryUntil = this.now() + delay + Math.floor(delay * Math.max(0, Math.min(1, this.random())) * .2);
      if (e instanceof PlaybackApiError && (e.status === 404 || e.status === 410 || e.code === 'session_ended') && this.tracked.get(id) === t) void this.learnEnd(id);
      // A transient refusal did not acknowledge this fact. Retain the snapshot
      // (including marker/end) ahead of newer facts until backoff permits retry.
      if (this.tracked.get(id) === t && (!(e instanceof PlaybackApiError) || e.retry === 'same_request')) t.pending!.unshift(next);
      else next.reject(e);
    }).finally(() => {
      this.clearTimer(deadline);
      if (t.reportAbort === controller) t.reportAbort = undefined;
      t.active = undefined;
      if (this.tracked.get(id) !== t) return;
      if (t.pending?.length) { this.drain(id, t); return; }
      if (t.state === 'ended' || t.stopping) return;
      t.timer = this.setTimer(() => {
        t.timer = undefined;
        if (this.tracked.get(id) === t) void this.report(id, t).catch(() => {});
      }, Math.min(2147483647, Math.max(1000, t.everyMs, (t.retryUntil ?? 0) - this.now())));
    });
  }

  /**
   * The server ended a session this service is playing: an administrator terminated it, it
   * moved to another device, its lease lapsed. `PlaybackService` subscribes and stops with the
   * reason. Learned from a forwarded `session.updated` event, or, when a timeline report finds the
   * session gone, from re-reading it.
   */
  onServerEnded(listener: (sessionId: string, end: SessionEnd) => void): () => void {
    this.endedListeners.add(listener);
    return () => { this.endedListeners.delete(listener); };
  }

  /** A `session.updated` event (apps forward `EventsClient`'s): an ended session is reported with
   * its reason; one without a reason in the payload is re-read for it. */
  sessionUpdated(sessionId: string, data?: unknown): void {
    if (!this.tracked.has(sessionId) || this.retired.has(sessionId)) return;
    if (data !== undefined && (data === null || typeof data !== 'object' || (data as {state?: unknown}).state !== 'ended')) return;
    const end = parseSessionEnd(data);
    if (end) this.serverEnded(sessionId, end);
    else void this.learnEnd(sessionId);
  }

  private async learnEnd(id: string): Promise<void> {
    try {
      const session = await this.sessions.get(id);
      if (session.state === 'ended') this.serverEnded(id, session.end ?? {reason: 'stopped'});
    } catch (e) {
      if (e instanceof PlaybackApiError && (e.status === 404 || e.status === 410)) this.serverEnded(id, {reason: 'stopped'});
    }
  }

  private serverEnded(id: string, end: SessionEnd) {
    if (!this.tracked.has(id)) return;
    this.forget(id);
    for (const listener of this.endedListeners) listener(id, end);
  }

  /** A session a committed audio edge ended (§18.3): no more reports, no end notice. */
  retire(id: string) {
    this.forget(id);
    this.retired.add(id);
    if (this.retired.size > 64) this.retired.delete(this.retired.values().next().value!);
  }

  /** Stops tracking (the session ended or was replaced) without a request. */
  forget(id: string) {
    const t = this.tracked.get(id);
    if (t?.timer !== undefined) this.clearTimer(t.timer);
    t?.reportAbort?.abort();
    for (const pending of t?.pending ?? []) pending.resolve();
    if (t) t.pending = [];
    this.tracked.delete(id);
  }

  /** The highest timeline `seq` sent for a session (0 when none): a native owner that takes the
   * session over continues from it (plan §9.3 C3). */
  reportedSeq(id: string): number {
    return this.tracked.get(id)?.seq ?? 0;
  }

  /** The last position the service reported for a session, in ms. */
  positionMs(id: string): number | undefined {
    return this.tracked.get(id)?.positionMs;
  }
}
