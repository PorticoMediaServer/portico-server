/**
 * Quality (spec §5.2). The default is the original file. A viewer may instead pick a fixed
 * quality (one step of the server's ladder, a ceiling: a file already within it still plays as it
 * is) or Automatic, which starts at the original and moves down, and back up, as the connection
 * requires. The owner's ceilings, the member's caps and the upload budget always apply on top.
 *
 * A viewer has two saved defaults on the server, `quality.home` and `quality.remote` (the
 * preference registry): a start that names no quality gets the one for where it comes from, as
 * the server decides it from the owner's home networks. The player can name another quality for
 * one session, at start or with a change.
 *
 * Automatic is driven by the client: it measures its connection and asks the server to step
 * (`{mode: 'automatic', step: 'down' | 'up'}`); each step re-plans the same session at the
 * current position. `AutomaticQuality` below decides when to ask.
 */

export type QualityId = '2160p' | '1440p' | '1080p' | '720p' | '480p' | '360p';
/** The values of the saved defaults `quality.home` and `quality.remote`. */
export type SavedQuality = 'original' | 'automatic' | QualityId;
export const savedQualities: readonly SavedQuality[] = Object.freeze(['original', 'automatic', '2160p', '1440p', '1080p', '720p', '480p', '360p']);

/** The `quality` field of a start, a change and the options preview. */
export type QualityRequest =
  | Readonly<{mode: 'original'}>
  /** One ladder step, by id. */
  | Readonly<{mode: 'fixed'; id: QualityId}>
  /** Automatic. `id` jumps to a step; `step` moves one step from where the session is (a change only). */
  | Readonly<{mode: 'automatic'; id?: QualityId; step?: 'down' | 'up'}>
  /** Ceilings in numbers, for a client that has its own. */
  | Readonly<{mode: 'limit'; maxVideoBitrateKbps?: number; maxHeight?: number; maxAudioBitrateKbps?: number}>;

/** The quality in force on a presentation: never a `step`; automatic's `id` is the step it is on (absent: the original). */
export type QualityInForce = Readonly<{mode: 'original' | 'fixed' | 'automatic' | 'limit'; id?: string; maxVideoBitrateKbps?: number; maxHeight?: number; maxAudioBitrateKbps?: number}>;

/** One entry of a title's quality menu (playback options `qualities`): the original, automatic, then the ladder steps no taller than the picture. */
export type QualityOption = Readonly<{mode: 'original' | 'automatic' | 'fixed'; id?: QualityId; maxHeight?: number; maxVideoBitrateKbps?: number; maxAudioBitrateKbps?: number}>;

/** The server's ladder (`GET /v1/playback/delivery-policy` `ladder`), widest first: what each fixed quality allows. */
export const qualityLadder: readonly Readonly<{id: QualityId; height: number; videoKbps: number; audioKbps: number}>[] = Object.freeze([
  {id: '2160p', height: 2160, videoKbps: 20_000, audioKbps: 192},
  {id: '1440p', height: 1440, videoKbps: 12_000, audioKbps: 192},
  {id: '1080p', height: 1080, videoKbps: 8000, audioKbps: 192},
  {id: '720p', height: 720, videoKbps: 4000, audioKbps: 128},
  {id: '480p', height: 480, videoKbps: 2000, audioKbps: 96},
  {id: '360p', height: 360, videoKbps: 1000, audioKbps: 96},
]);

const isQualityId = (v: unknown): v is QualityId => qualityLadder.some(r => r.id === v);

/** The request a saved default (or a menu choice by name) stands for. Anything unknown is the original. */
export function qualityRequest(saved: string | undefined): QualityRequest {
  if (saved === 'automatic') return Object.freeze({mode: 'automatic'});
  if (isQualityId(saved)) return Object.freeze({mode: 'fixed', id: saved});
  return Object.freeze({mode: 'original'});
}

/** The request a menu entry stands for. */
export function qualityOptionRequest(option: QualityOption): QualityRequest {
  if (option.mode === 'fixed' && option.id) return Object.freeze({mode: 'fixed', id: option.id});
  return Object.freeze({mode: option.mode === 'automatic' ? 'automatic' : 'original'});
}

/** Whether a menu entry is the quality in force (Automatic matches whatever step it is on). */
export function qualityOptionSelected(option: QualityOption, inForce: QualityInForce | undefined): boolean {
  const mode = inForce?.mode ?? 'original';
  if (option.mode !== mode) return false;
  return option.mode !== 'fixed' || option.id === inForce?.id;
}

/** Whether two requests are the same request (so a change would do nothing). */
export function sameQuality(a: QualityRequest, b: QualityRequest): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

/** What the current presentation needs the connection to carry, in kbps; undefined when unknown. */
export type AutomaticInput = Readonly<{
  /** The client's estimate of what its connection to this server carries. */
  throughputKbps: number;
  /** What the presentation in force delivers (`presentation.bitrateKbps` for a conversion, the file's bitrate for an original). */
  currentKbps?: number;
  /** The player ran out of data since the last decision. */
  stalled?: boolean;
  /** Milliseconds of media buffered ahead. */
  bufferedMs?: number;
}>;

/**
 * Decides when an Automatic session should ask the server to step. A step down is asked for at
 * once when the player stalls, or when the stream needs more than the connection carries (more
 * than four fifths of the estimate). A step up is asked for only after the connection has had a
 * quarter of headroom over the next step for `upAfterMs`, with a healthy buffer, so a noisy
 * estimate does not flap. Each decision is followed by a pause of `settleMs` while the new
 * presentation proves itself.
 */
export class AutomaticQuality {
  private readonly now: () => number;
  private readonly upAfterMs: number;
  private readonly settleMs: number;
  private headroomSince?: number;
  private quietUntil = 0;
  constructor(options: {now?: () => number; upAfterMs?: number; settleMs?: number} = {}) {
    this.now = options.now ?? (() => Date.now());
    this.upAfterMs = options.upAfterMs ?? 30_000;
    this.settleMs = options.settleMs ?? 10_000;
  }
  /** Forget what was observed (a new presentation, a seek). */
  reset() { this.headroomSince = undefined; this.quietUntil = this.now() + this.settleMs; }
  /** `'down'` or `'up'` when the session should step now, else undefined. `inForce` is the presentation's quality. */
  decide(inForce: QualityInForce | undefined, input: AutomaticInput): 'down' | 'up' | undefined {
    if (inForce?.mode !== 'automatic' || !(input.throughputKbps > 0)) return undefined;
    const now = this.now();
    if (now < this.quietUntil) return undefined;
    const usable = input.throughputKbps * 0.8;
    const at = qualityLadder.findIndex(r => r.id === inForce.id);
    const lowest = at === qualityLadder.length - 1;
    if (!lowest && (input.stalled === true || input.currentKbps !== undefined && input.currentKbps > usable)) {
      this.headroomSince = undefined;
      this.quietUntil = now + this.settleMs;
      return 'down';
    }
    if (at < 0) return undefined; // On the original: there is nothing above it.
    // The step above (or the original, whose need is unknown: twice the current step stands in).
    const above = at > 0 ? qualityLadder[at - 1]! : undefined;
    const needKbps = above ? above.videoKbps + above.audioKbps : (qualityLadder[at]!.videoKbps + qualityLadder[at]!.audioKbps) * 2;
    if (usable >= needKbps * 1.25 && (input.bufferedMs === undefined || input.bufferedMs >= 10_000) && input.stalled !== true) {
      this.headroomSince ??= now;
      if (now - this.headroomSince >= this.upAfterMs) {
        this.headroomSince = undefined;
        this.quietUntil = now + this.settleMs;
        return 'up';
      }
    } else this.headroomSince = undefined;
    return undefined;
  }
}
