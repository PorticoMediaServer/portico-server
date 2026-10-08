/**
 * The timeline reporter (spec §6): one POST that is progress, lease renewal and observation.
 *
 * - `seq` rises with every report sent for the session; the server ignores lower ones.
 * - A report is always built from the *latest* observation at send time, and only one is in
 *   flight: a slow or failing network never builds a queue, it just sends the newest state next.
 * - Cadence: 10 s while playing (or the server's `Report-Every-Ms`), 30 s paused, immediately on
 *   a state change, a completed seek, a skip or an error; nothing periodic once ended.
 * - Generation discipline: observations from an older presentation are dropped, and a switch
 *   clears the latest observation, so a stale-generation report can never be sent (invariant 3).
 * - Failures back off (1, 2, 5, 10, 30 s by default) and coalesce. A session the server no longer
 *   knows (404/410 or `session_ended`) stops the reporter and says so. Long failure risks the
 *   120 s lease: `onLeaseRisk` fires once so the controller can restart from the last position.
 */
import {GenerationGate} from './generation.ts';
import type {PlayerObservation, PlayerState} from './port.ts';

export type TimelineState = 'playing' | 'paused' | 'buffering' | 'ended' | 'error';
export type MarkerType = 'intro' | 'recap' | 'credits' | 'preview' | 'commercial';

export type TimelineReport = Readonly<{
  seq: number;
  generation: number;
  state: TimelineState;
  positionMs: number;
  partIndex?: number;
  rate: number;
  bufferedMs?: number;
  bandwidthKbps?: number;
  droppedFrames?: number;
  volume?: number;
  muted?: boolean;
  audioTrackId?: string;
  subtitleTrackId?: string | null;
  /** A marker the viewer skipped (spec §4.2): its id, whether the skip was the viewer's `auto`
   * preference or their own action, and where it left from. */
  skipped?: Readonly<{markerId: string; mode: 'automatic' | 'manual'; positionMs: number}>;
  error?: Readonly<{code: string; detail?: string}>;
}>;

/** Sends one report; resolves with the server's `Report-Every-Ms`, when given. */
export type TimelineSend = (report: TimelineReport) => Promise<{reportEveryMs?: number} | void>;

export type FailureKind = 'retry' | 'offline' | 'ended';

export type TimelineReporterOptions = Readonly<{
  send: TimelineSend;
  playingEveryMs?: number;
  pausedEveryMs?: number;
  backoffMs?: readonly number[];
  leaseMs?: number;
  /** How a failed send should be treated. Default: 404/410/`session_ended` end it; no status means offline; else retry. */
  classify?: (error: unknown) => FailureKind;
  onEnded?: () => void;
  onLeaseRisk?: () => void;
  now?: () => number;
  setTimer?: (fn: () => void, ms: number) => unknown;
  clearTimer?: (timer: unknown) => void;
}>;

export type TimelineSnapshot = Readonly<{
  seq: number;
  generation?: number;
  lastSentState?: TimelineState;
  lastAcknowledgedAt?: number;
  failures: number;
  offline: boolean;
  stopped: boolean;
}>;

const reportable: Record<PlayerState, TimelineState | undefined> = {idle: undefined, loading: 'buffering', playing: 'playing', paused: 'paused', buffering: 'buffering', ended: 'ended', error: 'error'};

function defaultClassify(error: unknown): FailureKind {
  const e = error as {status?: unknown; code?: unknown} | null;
  if (e?.code === 'session_ended' || e?.status === 404 || e?.status === 410) return 'ended';
  return typeof e?.status === 'number' ? 'retry' : 'offline';
}

export class TimelineReporter {
  private readonly gate = new GenerationGate();
  private latest?: PlayerObservation;
  private pendingSkip?: TimelineReport['skipped'];
  private pendingError?: TimelineReport['error'];
  private seq = 0;
  private lastSent?: TimelineState;
  private lastAck?: number;
  private started?: number;
  private failures = 0;
  private offline = false;
  private stopped = false;
  private inFlight = false;
  private dirty = false;
  private timer?: unknown;
  private reportEveryMs?: number;
  private leaseWarned = false;
  private readonly o: Required<Pick<TimelineReporterOptions, 'playingEveryMs' | 'pausedEveryMs' | 'backoffMs' | 'leaseMs'>> & TimelineReporterOptions;
  private readonly now: () => number;
  private readonly setTimer: (fn: () => void, ms: number) => unknown;
  private readonly clearTimer: (timer: unknown) => void;

  constructor(options: TimelineReporterOptions) {
    this.o = {playingEveryMs: 10_000, pausedEveryMs: 30_000, backoffMs: [1000, 2000, 5000, 10_000, 30_000], leaseMs: 120_000, ...options};
    this.now = options.now ?? Date.now;
    this.setTimer = options.setTimer ?? ((fn, ms) => setTimeout(fn, ms));
    this.clearTimer = options.clearTimer ?? (t => clearTimeout(t as ReturnType<typeof setTimeout>));
  }

  snapshot(): TimelineSnapshot {
    return Object.freeze({seq: this.seq, generation: this.gate.value, lastSentState: this.lastSent, lastAcknowledgedAt: this.lastAck, failures: this.failures, offline: this.offline, stopped: this.stopped});
  }

  /** A new presentation is in force: forget everything observed about older ones. */
  setGeneration(generation: number): void {
    if (this.stopped || !this.gate.advance(generation)) return;
    if (this.started === undefined) this.started = this.now();
    this.latest = undefined;
    this.pendingError = undefined;
  }

  /** A player reading. `cause` says whether it must be reported now. */
  observe(observation: PlayerObservation, cause: 'time' | 'state' | 'seeked' = 'time'): void {
    if (this.stopped || !this.gate.accepts(observation.generation) || !reportable[observation.state]) return;
    const previous = this.latest;
    this.latest = observation;
    const changed = !previous || reportable[previous.state] !== reportable[observation.state] || reportable[observation.state] !== this.lastSent;
    // A track switch (a local sidecar subtitle, say) is reported at once (spec §17).
    const tracks = !!previous && (previous.audioTrackId !== observation.audioTrackId || previous.subtitleTrackId !== observation.subtitleTrackId);
    if (cause === 'seeked' || (cause === 'state' && changed) || tracks || (this.seq === 0 && !this.inFlight)) this.reportNow();
    else if (this.timer === undefined && !this.inFlight) this.schedule();
  }

  /** The viewer skipped a marker (spec §4.2): sent with the next report, immediately. */
  skipped(marker: NonNullable<TimelineReport['skipped']>): void {
    if (this.stopped) return;
    this.pendingSkip = marker;
    this.reportNow();
  }

  /** A client-side failure for the current presentation (spec §6 `error`). */
  failed(generation: number, code: string, detail?: string): void {
    if (this.stopped || !this.gate.accepts(generation)) return;
    this.pendingError = {code: code.slice(0, 64), ...(detail ? {detail: detail.slice(0, 500)} : {})};
    if (this.latest) this.latest = {...this.latest, state: 'error'};
    this.reportNow();
  }

  /** Send the newest state now (before a stop, on app backgrounding). */
  flush(): Promise<void> {
    this.reportNow();
    return this.idle();
  }

  /** No more reports for this session. */
  stop(): void {
    this.stopped = true;
    this.cancel();
    this.release();
  }

  private idleWaiters: (() => void)[] = [];
  private idle(): Promise<void> {
    if (!this.inFlight && !this.dirty) return Promise.resolve();
    return new Promise(resolve => this.idleWaiters.push(resolve));
  }

  private cancel() {
    if (this.timer !== undefined) this.clearTimer(this.timer);
    this.timer = undefined;
  }

  private reportNow() {
    if (this.stopped) return;
    this.cancel();
    if (this.inFlight) { this.dirty = true; return; }
    void this.send();
  }

  private schedule(delayMs?: number) {
    if (this.stopped) return;
    this.cancel();
    const state = this.latest ? reportable[this.latest.state] : undefined;
    if (delayMs === undefined) {
      if (!state || state === 'ended' || state === 'error') return;
      delayMs = state === 'paused' ? Math.max(this.o.pausedEveryMs, this.reportEveryMs ?? 0) : this.reportEveryMs ?? this.o.playingEveryMs;
    }
    this.timer = this.setTimer(() => { this.timer = undefined; this.reportNow(); }, delayMs);
  }

  private build(): TimelineReport | undefined {
    const o = this.latest, generation = this.gate.value;
    if (!o || generation === undefined || o.generation !== generation) return undefined;
    const state = reportable[o.state];
    if (!state) return undefined;
    return Object.freeze({
      seq: ++this.seq, generation, state, positionMs: Math.max(0, Math.round(o.positionMs)), rate: o.rate,
      ...(o.partIndex !== undefined ? {partIndex: o.partIndex} : {}),
      ...(o.bufferedMs !== undefined ? {bufferedMs: Math.round(o.bufferedMs)} : {}),
      ...(o.bandwidthKbps !== undefined ? {bandwidthKbps: Math.round(o.bandwidthKbps)} : {}),
      ...(o.droppedFrames !== undefined ? {droppedFrames: o.droppedFrames} : {}),
      ...(o.volume !== undefined ? {volume: o.volume} : {}),
      ...(o.muted !== undefined ? {muted: o.muted} : {}),
      ...(o.audioTrackId !== undefined ? {audioTrackId: o.audioTrackId} : {}),
      ...(o.subtitleTrackId !== undefined ? {subtitleTrackId: o.subtitleTrackId} : {}),
      ...(this.pendingSkip ? {skipped: this.pendingSkip} : {}),
      ...(this.pendingError ? {error: this.pendingError} : {}),
    });
  }

  private async send() {
    const report = this.build();
    if (!report) { this.settle(); return; }
    this.inFlight = true;
    this.dirty = false;
    const skip = this.pendingSkip, error = this.pendingError;
    try {
      const answer = await this.o.send(report);
      if (this.stopped) return;
      this.lastSent = report.state;
      this.lastAck = this.now();
      this.failures = 0;
      this.offline = false;
      this.leaseWarned = false;
      if (this.pendingSkip === skip) this.pendingSkip = undefined;
      if (this.pendingError === error) this.pendingError = undefined;
      const every = answer && typeof answer.reportEveryMs === 'number' && answer.reportEveryMs > 0 ? answer.reportEveryMs : undefined;
      if (every) this.reportEveryMs = Math.max(1000, every);
      if (report.state === 'ended') this.stop();
    } catch (e) {
      if (this.stopped) return;
      const kind = (this.o.classify ?? defaultClassify)(e);
      if (kind === 'ended') { this.stop(); this.o.onEnded?.(); return; }
      this.failures++;
      this.offline = kind === 'offline';
      this.dirty = true;
      const since = this.now() - (this.lastAck ?? this.started ?? this.now());
      if (!this.leaseWarned && since >= this.o.leaseMs * 0.75) { this.leaseWarned = true; this.o.onLeaseRisk?.(); }
      const wait = this.o.backoffMs[Math.min(this.failures - 1, this.o.backoffMs.length - 1)] ?? 30_000;
      this.inFlight = false;
      this.schedule(wait);
      // A flush waits for an attempt, not for the network to come back.
      this.release();
      return;
    } finally {
      this.inFlight = false;
    }
    if (this.dirty) { this.dirty = false; void this.send(); return; }
    this.schedule();
    this.settle();
  }

  private settle() {
    if (this.inFlight || this.dirty) return;
    this.release();
  }

  private release() {
    for (const w of this.idleWaiters.splice(0)) w();
  }
}
