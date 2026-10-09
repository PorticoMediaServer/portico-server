/**
 * Playback sessions (spec §5) and the controller that plays one on a `MediaPlayer`.
 *
 * `SessionsClient` is the wire: idempotent start (201, or 202 while preparing: the same request
 * is replayed with the same `Idempotency-Key` after `retryAfterMs`), `PATCH` with `If-Match`
 * (412 carries the current session), `DELETE`, timeline.
 *
 * `PlaybackSessionController` is what a platform wires (phase 2): it owns the generation gate,
 * the timeline reporter and the player port, serialises changes, recovers from 412 by adopting
 * the server's session and re-applying only the newest intent, and ends cleanly when the server
 * ends the session (stop elsewhere, transfer, admin terminate).
 */
import {GenerationGate} from './generation.ts';
import {PlaybackApiError, call, enc, header, idempotencyKey, type V1Http} from './http.ts';
import type {MediaPlayer, PlayerEvent, PlayerObservation} from './port.ts';
import type {QualityRequest} from './quality.ts';
import {TimelineReporter, type TimelineReport, type TimelineReporterOptions} from './timeline.ts';
import {parseSession, parseSessionEnd, type Session, type SessionEnd} from './types.ts';

export type StartTarget =
  | Readonly<{itemId: string}>
  | Readonly<{channelId: string}>
  | Readonly<{queue: Readonly<{queueId: string; entryId: string}>}>
  | Readonly<{transferId: string}>;

export type StartOptions = Readonly<{
  versionId?: string;
  partIndex?: number;
  startPositionMs?: number;
  startFrom?: 'resume' | 'beginning' | 'live' | 'startOver';
  state?: 'playing' | 'paused';
  quality?: QualityRequest;
  audio?: Readonly<{trackId: string; channels?: number}>;
  subtitles?: Readonly<{trackId: string | null; delivery?: 'auto' | 'sidecar' | 'embeddedClient' | 'burn'}>;
  network?: 'local' | 'remote' | 'cellular';
  replacesSessionId?: string;
}>;

export type SessionChange = Readonly<{
  state?: 'playing' | 'paused';
  versionId?: string;
  partIndex?: number;
  audio?: Readonly<{trackId: string; channels?: number}>;
  subtitles?: Readonly<{trackId: string | null; delivery?: 'auto' | 'sidecar' | 'embeddedClient' | 'burn'}>;
  quality?: QualityRequest;
  seek?: Readonly<{positionMs: number; id: string}>;
  subtitleOffsetMs?: number;
}>;

export type Timers = Readonly<{now?: () => number; setTimer?: (fn: () => void, ms: number) => unknown; clearTimer?: (t: unknown) => void}>;

/** How long a 202 "preparing" start may take before giving up. */
const MAX_PREPARE_MS = 120_000;

export class SessionsClient {
  private http: V1Http;
  private setTimer: (fn: () => void, ms: number) => unknown;
  constructor(http: V1Http, timers: Timers = {}) {
    this.http = http;
    this.setTimer = timers.setTimer ?? ((fn, ms) => setTimeout(fn, ms));
  }

  /** Start (or replay a start). `onPreparing` reports each 202 so the UI can say "Getting it ready". */
  async start(target: StartTarget, options: StartOptions, key: string, extra: {signal?: AbortSignal; onPreparing?: (retryAfterMs: number) => void} = {}): Promise<Session> {
    const body = {...target, ...options};
    let waited = 0;
    for (;;) {
      const r = await call(this.http, {method: 'POST', path: '/v1/playback/sessions', headers: {'Idempotency-Key': key}, body, signal: extra.signal}, [201, 200, 202]);
      if (r.status !== 202) return parseSession(r.body);
      const b = r.body as {retryAfterMs?: unknown} | undefined;
      const wait = Math.min(10_000, Math.max(250, typeof b?.retryAfterMs === 'number' ? b.retryAfterMs : Number(header(r, 'Retry-After') ?? 1) * 1000));
      waited += wait;
      if (waited > MAX_PREPARE_MS) throw new PlaybackApiError(504, 'preparation_timeout', 'same_request');
      extra.onPreparing?.(wait);
      await new Promise<void>((resolve, reject) => {
        if (extra.signal?.aborted) { reject(Object.assign(new Error('aborted'), {name: 'AbortError'})); return; }
        this.setTimer(resolve, wait);
        extra.signal?.addEventListener('abort', () => reject(Object.assign(new Error('aborted'), {name: 'AbortError'})), {once: true});
      });
    }
  }

  async get(id: string, signal?: AbortSignal): Promise<Session> {
    return parseSession((await call(this.http, {method: 'GET', path: `/v1/playback/sessions/${enc(id)}`, signal})).body);
  }

  /** `PATCH` with `If-Match`. A 412 throws `PlaybackApiError` whose `current` is the parsed session. */
  async patch(id: string, revision: string, change: SessionChange, signal?: AbortSignal): Promise<Session> {
    try {
      return parseSession((await call(this.http, {method: 'PATCH', path: `/v1/playback/sessions/${enc(id)}`, headers: {'If-Match': revision, 'Content-Type': 'application/merge-patch+json'}, body: change, signal})).body);
    } catch (e) {
      if (e instanceof PlaybackApiError && e.status === 412 && e.current !== undefined) {
        let current: Session | undefined;
        try { current = parseSession(e.current); } catch { current = undefined; }
        throw new PlaybackApiError(412, e.code, 'after_refresh', current, e.retryAfterMs);
      }
      throw e;
    }
  }

  async stop(id: string, positionMs?: number, signal?: AbortSignal): Promise<void> {
    await call(this.http, {method: 'DELETE', path: `/v1/playback/sessions/${enc(id)}`, ...(signal ? {signal} : {}), ...(positionMs !== undefined ? {body: {positionMs: Math.max(0, Math.round(positionMs))}} : {})}, [204, 200, 404, 410]);
  }

  async timeline(id: string, report: TimelineReport, signal?: AbortSignal): Promise<{reportEveryMs?: number}> {
    const r = await call(this.http, {method: 'POST', path: `/v1/playback/sessions/${enc(id)}/timeline`, body: report, ...(signal ? {signal} : {})});
    const every = Number(header(r, 'Report-Every-Ms'));
    return Number.isFinite(every) && every > 0 ? {reportEveryMs: every} : {};
  }
}

export type ControllerPhase = 'idle' | 'starting' | 'preparing' | 'active' | 'ended' | 'error';

export type ControllerSnapshot = Readonly<{
  phase: ControllerPhase;
  session?: Session;
  /** The last observation of the current generation (position, state). */
  observation?: PlayerObservation;
  error?: PlaybackApiError | Error;
  /** Why the server ended the session (phase "ended"): "terminated" by an administrator with
   * their message, "transferred", "lease_expired"… Absent when this device stopped it. Show it with
   * `sessionEndMessage`. */
  ended?: SessionEnd;
  /** A change is being sent. */
  changing: boolean;
}>;

export type ControllerOptions = Readonly<{
  http: V1Http;
  player: MediaPlayer;
  timers?: Timers;
  key?: () => string;
  reporter?: Partial<Omit<TimelineReporterOptions, 'send' | 'onEnded' | 'onLeaseRisk'>>;
  /** Called when the server ended the session (transfer committed, admin terminate, stop elsewhere). */
  onServerEnded?: (session: Session) => void;
}>;

/**
 * Whether a seek must go through the server (spec §5.7): only when something is transcoded or
 * burned, since the server must restart production. Direct play and stream copy seek locally.
 */
export function seekNeedsServer(session: Session): boolean {
  const d = session.presentation.decision;
  return [d.video, d.audio, d.subtitles].some(x => x?.action === 'transcode' || x?.action === 'burn');
}

export class PlaybackSessionController {
  private readonly client: SessionsClient;
  private readonly player: MediaPlayer;
  private readonly gate = new GenerationGate();
  private readonly key: () => string;
  private readonly timers: Timers;
  private readonly reporterOptions: ControllerOptions['reporter'];
  private readonly onServerEnded?: (session: Session) => void;
  private reporter?: TimelineReporter;
  private state: ControllerSnapshot = Object.freeze({phase: 'idle', changing: false});
  private listeners = new Set<() => void>();
  private offPlayer: () => void;
  private startToken = 0;
  private changeChain: Promise<unknown> = Promise.resolve();
  private changeSerial = 0;
  private leaseAtRisk = false;
  private lastStart?: {target: StartTarget; options: StartOptions};
  private carriedFor?: number;

  constructor(o: ControllerOptions) {
    this.client = new SessionsClient(o.http, o.timers);
    this.player = o.player;
    this.key = o.key ?? (() => idempotencyKey());
    this.timers = o.timers ?? {};
    this.reporterOptions = o.reporter;
    this.onServerEnded = o.onServerEnded;
    this.offPlayer = o.player.onEvent(e => this.playerEvent(e));
  }

  getSnapshot = (): ControllerSnapshot => this.state;
  subscribe = (fn: () => void) => { this.listeners.add(fn); return () => { this.listeners.delete(fn); }; };
  private publish(p: Partial<ControllerSnapshot>) { this.state = Object.freeze({...this.state, ...p}); for (const l of [...this.listeners]) l(); }

  /**
   * Play something. A session already playing on this device is replaced atomically
   * (`replacesSessionId`), so queue advance, version switch and channel surfing keep one slot.
   */
  async start(target: StartTarget, options: StartOptions = {}, signal?: AbortSignal): Promise<Session | undefined> {
    const token = ++this.startToken;
    const previous = this.state.session;
    const body: StartOptions = {...options, ...(previous && this.state.phase === 'active' && !options.replacesSessionId ? {replacesSessionId: previous.id} : {})};
    this.lastStart = {target, options};
    this.retireReporter();
    this.publish({phase: 'starting', error: undefined, ended: undefined});
    try {
      const session = await this.client.start(target, body, this.key(), {signal, onPreparing: () => { if (token === this.startToken) this.publish({phase: 'preparing'}); }});
      if (token !== this.startToken) return undefined;
      this.gate.reset();
      this.adopt(session, options.state !== 'paused');
      return session;
    } catch (e) {
      if (token === this.startToken) this.publish({phase: 'error', error: e as Error});
      throw e;
    }
  }

  /** Adopt a server session (start, 412 recovery, `session.updated`): load a new generation's URL. */
  private adopt(session: Session, autoplay: boolean) {
    const fresh = this.gate.advance(session.presentation.generation);
    if (!this.reporter) this.reporter = this.newReporter(session.id);
    this.publish({session, phase: session.state === 'ended' ? 'ended' : 'active', ended: session.state === 'ended' ? session.end ?? {reason: 'stopped'} : undefined});
    if (!fresh) return;
    this.reporter.setGeneration(session.presentation.generation);
    this.player.load({
      url: session.presentation.url, generation: session.presentation.generation, mode: session.presentation.mode,
      startPositionMs: session.presentation.startPositionMs, subtitles: session.presentation.subtitles, autoplay,
      sessionId: session.id, ...(session.itemId ? {itemId: session.itemId} : {}),
    });
  }

  private newReporter(sessionId: string): TimelineReporter {
    return new TimelineReporter({
      ...this.reporterOptions,
      now: this.timers.now, setTimer: this.timers.setTimer, clearTimer: this.timers.clearTimer,
      send: report => this.client.timeline(sessionId, report),
      onLeaseRisk: () => { this.leaseAtRisk = true; },
      onEnded: () => void this.serverEnded(),
    });
  }

  private retireReporter() {
    this.reporter?.stop();
    this.reporter = undefined;
  }

  /** The server ended our session. Why comes with the event, or from re-reading the session. */
  private async serverEnded(end?: SessionEnd) {
    const session = this.state.session;
    this.retireReporter();
    if (!end && session) end = await this.client.get(session.id).then(s => s.end, () => undefined);
    // Only a lapsed lease is resumed; an administrator's stop or a move elsewhere is final.
    if (this.leaseAtRisk && this.lastStart && session && 'itemId' in this.lastStart.target && (!end || end.reason === 'lease_expired')) {
      // The lease lapsed while we couldn't report (offline): start again where we were.
      this.leaseAtRisk = false;
      const at = this.state.observation?.positionMs ?? 0;
      await this.start(this.lastStart.target, {...this.lastStart.options, startPositionMs: at, startFrom: undefined, replacesSessionId: undefined}).catch(() => {});
      return;
    }
    this.player.pause();
    this.publish({phase: 'ended', ended: end ?? {reason: 'stopped'}});
    if (session) this.onServerEnded?.(session);
  }

  private playerEvent(e: PlayerEvent) {
    const o = e.observation;
    if (!this.gate.accepts(o.generation)) return;
    this.publish({observation: o});
    if (e.type === 'error') { this.reporter?.failed(o.generation, e.code, e.detail); return; }
    // PiP or AirPlay can't show an overlay the app draws: ask for the same track in the manifest,
    // which the system renders wherever the picture is (Plan §8.3). Once per generation.
    if (o.subtitlesCarried === false && o.subtitleTrackId && this.carriedFor !== o.generation) {
      this.carriedFor = o.generation;
      void this.change({subtitles: {trackId: o.subtitleTrackId, delivery: 'embeddedClient'}}).catch(() => {});
    }
    this.reporter?.observe(o, e.type === 'time' ? 'time' : e.type === 'seeked' ? 'seeked' : 'state');
  }

  /**
   * Send a change. Changes are serialised; on 412 the controller adopts the server's session and
   * re-applies this change only if no newer change is waiting (newest intent wins, invariant 2).
   */
  change(change: SessionChange): Promise<Session | undefined> {
    const serial = ++this.changeSerial;
    const run = async (): Promise<Session | undefined> => {
      const session = this.state.session;
      if (!session || this.state.phase === 'ended') return undefined;
      this.publish({changing: true});
      try {
        let updated: Session;
        try {
          updated = await this.client.patch(session.id, session.revision, change);
        } catch (e) {
          if (!(e instanceof PlaybackApiError) || e.status !== 412 || !e.current) throw e;
          this.adopt(e.current as Session, this.state.observation?.state !== 'paused');
          if (serial !== this.changeSerial) return this.state.session;
          updated = await this.client.patch((e.current as Session).id, (e.current as Session).revision, change);
        }
        this.adopt(updated, updated.state !== 'paused');
        return updated;
      } catch (e) {
        if (e instanceof PlaybackApiError && (e.status === 404 || e.status === 410)) { void this.serverEnded(); return undefined; }
        this.publish({error: e as Error});
        throw e;
      } finally {
        if (serial === this.changeSerial) this.publish({changing: false});
      }
    };
    const p = this.changeChain.then(run, run);
    this.changeChain = p.catch(() => {});
    return p;
  }

  play() { this.player.play(); }
  pause() { this.player.pause(); }
  setRate(rate: number) { this.player.setRate(Math.max(0.25, Math.min(4, rate))); }

  /** Seek locally when bytes are direct or copied; through the server when it must re-produce. */
  seek(positionMs: number): Promise<unknown> | void {
    const session = this.state.session;
    if (!session) return;
    if (seekNeedsServer(session)) return this.change({seek: {positionMs: Math.max(0, Math.round(positionMs)), id: this.key()}});
    this.player.seek(Math.max(0, positionMs));
  }

  /**
   * Subtitles already offered as a sidecar switch locally; others need the server (new generation).
   * A local switch is reported at once (`subtitleTrackId` in the timeline, spec §17).
   */
  setSubtitles(trackId: string | null): Promise<unknown> | void {
    const session = this.state.session;
    if (!session) return;
    if (trackId === null || session.presentation.subtitles.some(s => s.trackId === trackId)) {
      this.player.selectSidecarSubtitle(trackId);
      const o = this.player.observe();
      if (this.gate.accepts(o.generation)) { this.publish({observation: o}); this.reporter?.observe(o, 'state'); }
      return;
    }
    return this.change({subtitles: {trackId}});
  }

  setAudio(trackId: string) { return this.change({audio: {trackId}}); }
  setQuality(quality: QualityRequest) { return this.change({quality}); }
  setVersion(versionId: string) { return this.change({versionId}); }

  /** Skips a marker (spec §4.2): a seek to its end plus a `skipped` note in the next timeline
   * report. `automatic` is for the viewer's `auto` preference, and only on a marker the server
   * marks `automaticSafe`; the server ignores anything else as evidence. */
  skip(marker: Readonly<{id: string; endMs: number}>, mode: 'automatic' | 'manual' = 'manual') {
    if (marker.id) this.reporter?.skipped({markerId: marker.id, mode, positionMs: Math.max(0, Math.round(this.state.observation?.positionMs ?? 0))});
    return this.seek(marker.endMs);
  }

  /** A `session.updated` event for our session (with its payload when the app has it): an end
   * with its reason is taken as is; otherwise the session is re-read if its revision moved. */
  async sessionUpdated(sessionId: string, revision?: string, data?: unknown): Promise<void> {
    const session = this.state.session;
    if (!session || session.id !== sessionId || this.state.phase === 'ended' || (revision !== undefined && revision === session.revision)) return;
    const end = data && typeof data === 'object' && (data as {state?: unknown}).state === 'ended' ? parseSessionEnd(data) : undefined;
    if (end) { void this.serverEnded(end); return; }
    try {
      const fresh = await this.client.get(sessionId);
      if (fresh.state === 'ended') { void this.serverEnded(fresh.end ?? {reason: 'stopped'}); return; }
      this.adopt(fresh, fresh.state !== 'paused');
    } catch (e) {
      if (e instanceof PlaybackApiError && (e.status === 404 || e.status === 410)) void this.serverEnded();
    }
  }

  /** Stop playing: report the final position, end the session, pause the player. Idempotent. */
  async stop(): Promise<void> {
    const session = this.state.session;
    this.startToken++;
    const at = this.state.observation?.positionMs;
    if (this.reporter) { await this.reporter.flush().catch(() => {}); }
    this.retireReporter();
    this.player.pause();
    this.publish({phase: 'ended'});
    if (session) await this.client.stop(session.id, at).catch(() => {});
  }

  dispose() {
    this.retireReporter();
    this.offPlayer();
    this.listeners.clear();
  }
}
