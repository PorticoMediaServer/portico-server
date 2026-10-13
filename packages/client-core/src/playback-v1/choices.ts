/**
 * What a viewer chooses for one playback (quality, version, audio track) and the Quality menu's
 * model. This is the one place those choices become protocol requests: the player screens and the
 * settings speak in `QualityChoice`; `qualityRequest` turns one into the session's `quality` field.
 *
 * The menu, in order: Original (the default: the file as it is, converted only when the device
 * cannot play it), Automatic (starts at Original and moves down, and back up, as the connection
 * requires; see `AutomaticQuality`), then the fixed qualities below the source.
 */
import type {QualityRequest} from './quality.ts';

/** Choices that travel with a start or a change of the live session. Absent fields are left as they are. */
export type PlaybackChoices = Readonly<{
  quality?: QualityRequest;
  /** The version of the title to play (an edition, another copy). */
  versionId?: string;
  /** The audio track, by its playback-options id (`a1`, `a2`…). */
  audioTrackId?: string;
}>;

export type FixedQualityId = '2160' | '1080' | '720' | '480' | '360';
export type QualityChoice = 'original' | 'automatic' | FixedQualityId;

/** A fixed quality: the tallest picture and the video bit rate a stream at that height is given. */
export type FixedQuality = Readonly<{id: FixedQualityId; height: number; videoKbps: number; audioKbps?: number}>;

/** Fixed qualities, best first. */
export const fixedQualities: readonly FixedQuality[] = Object.freeze([
  Object.freeze({id: '2160' as const, height: 2160, videoKbps: 40_000}),
  Object.freeze({id: '1080' as const, height: 1080, videoKbps: 8000}),
  Object.freeze({id: '720' as const, height: 720, videoKbps: 4000}),
  Object.freeze({id: '480' as const, height: 480, videoKbps: 1500, audioKbps: 128}),
  Object.freeze({id: '360' as const, height: 360, videoKbps: 800, audioKbps: 96}),
]);

export const qualityChoiceOrder: readonly QualityChoice[] = Object.freeze(['original', 'automatic', ...fixedQualities.map(q => q.id)]);

export function isQualityChoice(value: unknown): value is QualityChoice {
  return typeof value === 'string' && (qualityChoiceOrder as readonly string[]).includes(value);
}

export function fixedQuality(id: string | undefined): FixedQuality | undefined {
  return fixedQualities.find(q => q.id === id);
}

/**
 * The fixed qualities worth offering for a source: those that are really lower than it. A step at
 * the source's own height is offered only when the source's bit rate is well above the step's (a
 * 1080p disc copy at 30 Mbps has a meaningful "1080p" below it); with nothing known, all are offered.
 */
export function offeredQualities(source: Readonly<{height?: number; bitrateKbps?: number}> = {}): readonly FixedQuality[] {
  const height = source.height && source.height > 0 ? source.height : undefined;
  const kbps = source.bitrateKbps && source.bitrateKbps > 0 ? source.bitrateKbps : undefined;
  if (height === undefined && kbps === undefined) return fixedQualities;
  return fixedQualities.filter(q => {
    // Sources are rarely exactly 1080 lines tall (a 1920x800 film is "1080p"): compare by class.
    const sourceClass = height === undefined ? Infinity : heightClass(height);
    if (q.height < sourceClass) return true;
    if (q.height > sourceClass) return false;
    return kbps !== undefined && kbps >= q.videoKbps * 1.5;
  });
}

/** The fixed-quality class a picture height belongs to (800 lines of a 1920-wide film is 1080). */
export function heightClass(height: number): number {
  if (height > 1200) return 2160;
  if (height > 780) return 1080;
  if (height > 520) return 720;
  if (height > 400) return 480;
  return 360;
}

/**
 * The session `quality` for a choice. Automatic starts at Original; `step` is the fixed quality
 * Automatic has moved down to, when it has.
 */
export function qualityRequest(choice: QualityChoice, step?: FixedQuality): QualityRequest {
  const fixed = choice === 'automatic' ? step : fixedQuality(choice);
  if (!fixed) return Object.freeze({mode: 'original'});
  return Object.freeze({mode: 'limit', maxHeight: fixed.height, maxVideoBitrateKbps: fixed.videoKbps, ...(fixed.audioKbps ? {maxAudioBitrateKbps: fixed.audioKbps} : {})});
}

/** The fixed quality a session's quality request corresponds to, if any (for the menu's tick). */
export function fixedQualityOf(request: QualityRequest | undefined): FixedQuality | undefined {
  if (!request || request.mode !== 'limit' || !request.maxHeight) return undefined;
  return fixedQualities.find(q => q.height === request.maxHeight) ?? fixedQualities.find(q => q.height <= request.maxHeight!);
}

/** The audio request for a track id, or for a legacy absolute stream index. */
export function audioTrackId(stream: number | string | undefined): string | undefined {
  if (typeof stream === 'string') return /^a\d{1,4}$/.test(stream) ? stream : undefined;
  return typeof stream === 'number' && Number.isInteger(stream) && stream >= 0 && stream <= 9999 ? 'a' + stream : undefined;
}

/** The stream index of a playback-options audio id (`a3` → 3). */
export function audioStreamIndex(trackId: string | undefined): number | undefined {
  const m = trackId ? /^a(\d{1,4})$/.exec(trackId) : null;
  return m ? Number(m[1]) : undefined;
}

// ── Automatic ───────────────────────────────────────────────────────────────

export type AutomaticOptions = Readonly<{
  /** The fixed qualities this source offers, best first (`offeredQualities`). */
  steps: readonly FixedQuality[];
  /** The bit rate of the original, when known: what Original needs from the connection. */
  originalKbps?: number;
  now?: () => number;
  /** A shortfall must last this long before stepping down. */
  shortfallMs?: number;
  /** Headroom must last this long before stepping up. */
  headroomMs?: number;
  /** Stalls inside this window count together. */
  stallWindowMs?: number;
  /** The least time between two changes: a change costs a rebuffer. */
  holdMs?: number;
}>;

/**
 * Automatic quality for one playback. It starts at Original and is told what the connection
 * manages (`sample`, from media downloads) and when the picture ran dry (`stalled`). `decide`
 * answers with the step to move to, or nothing.
 *
 * Down: the measured throughput stays under what the current quality needs for `shortfallMs`, or
 * the picture stalls twice inside `stallWindowMs`. It then moves to the best step the connection
 * carries with a fifth to spare (always at least one step down).
 * Up: the throughput stays half as much again above what the next quality up needs for
 * `headroomMs`, with no stall in that time. One step at a time.
 * Not flapping: at least `holdMs` between changes; a step up that stalls soon afterwards doubles
 * the wait before the next attempt to go up (to at most ten minutes), and that wait relaxes again
 * after a step up that holds.
 */
export class AutomaticQuality {
  private readonly steps: readonly FixedQuality[];
  private readonly originalKbps?: number;
  private readonly now: () => number;
  private readonly shortfallMs: number;
  private readonly headroomMs: number;
  private readonly stallWindowMs: number;
  private readonly holdMs: number;
  /** -1 is Original; otherwise an index into `steps`. */
  private index = -1;
  private estimateKbps?: number;
  private shortSince?: number;
  private roomSince?: number;
  private stalls: number[] = [];
  private changedAt = -Infinity;
  private wentUpAt = -Infinity;
  private upHoldMs: number;

  constructor(options: AutomaticOptions) {
    this.steps = options.steps;
    this.originalKbps = options.originalKbps && options.originalKbps > 0 ? options.originalKbps : undefined;
    this.now = options.now ?? Date.now;
    this.shortfallMs = options.shortfallMs ?? 10_000;
    this.headroomMs = options.headroomMs ?? 30_000;
    this.stallWindowMs = options.stallWindowMs ?? 30_000;
    this.holdMs = options.holdMs ?? 15_000;
    this.upHoldMs = this.headroomMs;
  }

  /** The step in force: undefined is Original. */
  get step(): FixedQuality | undefined { return this.index < 0 ? undefined : this.steps[this.index]; }
  get estimate(): number | undefined { return this.estimateKbps; }

  private needs(index: number): number | undefined {
    return index < 0 ? this.originalKbps : this.steps[index]?.videoKbps;
  }

  /** One media download: how many kilobits a second it managed. Smoothed, falling faster than it rises. */
  sample(kbps: number): void {
    if (!Number.isFinite(kbps) || kbps <= 0) return;
    const previous = this.estimateKbps;
    this.estimateKbps = previous === undefined ? kbps : kbps < previous ? previous * 0.5 + kbps * 0.5 : previous * 0.8 + kbps * 0.2;
    const at = this.now(), estimate = this.estimateKbps, need = this.needs(this.index);
    if (need !== undefined && estimate < need) this.shortSince ??= at; else this.shortSince = undefined;
    const above = this.index >= 0 ? this.needs(this.index - 1) : undefined;
    if (this.index >= 0 && above !== undefined && estimate >= above * 1.5) this.roomSince ??= at; else this.roomSince = undefined;
  }

  /** The picture ran out of media while it should have been playing. */
  stalled(): void {
    const at = this.now();
    this.stalls = [...this.stalls.filter(t => at - t <= this.stallWindowMs), at];
    this.roomSince = undefined;
    // A step up that cannot hold: wait longer before trying again.
    if (at - this.wentUpAt <= this.stallWindowMs) { this.upHoldMs = Math.min(600_000, this.upHoldMs * 2); this.wentUpAt = -Infinity; }
  }

  /** The step to change to now (`null` is back to Original), or undefined to stay. */
  decide(): FixedQuality | null | undefined {
    const at = this.now();
    if (at - this.changedAt < this.holdMs) return undefined;
    this.stalls = this.stalls.filter(t => at - t <= this.stallWindowMs);
    const short = this.shortSince !== undefined && at - this.shortSince >= this.shortfallMs;
    if ((short || this.stalls.length >= 2) && this.index < this.steps.length - 1) {
      let next = this.index + 1;
      const usable = this.estimateKbps === undefined ? undefined : this.estimateKbps * 0.8;
      if (usable !== undefined) while (next < this.steps.length - 1 && this.steps[next]!.videoKbps > usable) next++;
      return this.move(next, at);
    }
    if (this.index >= 0 && this.roomSince !== undefined && at - this.roomSince >= this.upHoldMs && this.stalls.length === 0) {
      // A step up that held relaxes the wait again.
      if (this.wentUpAt !== -Infinity && at - this.wentUpAt > this.stallWindowMs) this.upHoldMs = this.headroomMs;
      this.wentUpAt = at;
      return this.move(this.index - 1, at);
    }
    return undefined;
  }

  private move(index: number, at: number): FixedQuality | null {
    this.index = index;
    this.changedAt = at;
    this.shortSince = undefined;
    this.roomSince = undefined;
    this.stalls = [];
    return index < 0 ? null : this.steps[index]!;
  }
}
