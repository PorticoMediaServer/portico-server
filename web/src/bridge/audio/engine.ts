import {Resampler} from './resampler';
import type {Planar} from './decoder';

/**
 * The two-deck scheduler (§9.6 rules 1, 4, 7, 8). Every deck plays one track's exact frames on the
 * context's sample clock, from an integer start frame, in fixed-size buffers scheduled back to
 * back; nothing is timed by timers. A gapless successor starts at the frame after the current
 * track's last frame; a crossfade overlaps them with equal-power curves. Each deck decodes at most
 * `aheadSeconds` past the playhead, so memory is bounded however long the track or queue.
 *
 * Works on any BaseAudioContext, so an OfflineAudioContext renders the same schedule for tests.
 */
export interface PcmSource {
  readonly sampleRate: number;
  readonly channels: number;
  read(max: number): Promise<Planar | undefined>;
  close(): void;
}

export type DeckOptions = Readonly<{
  /** Trimmed frames of the track this deck plays (from its start frame to its end), at the file's rate. */
  frames: number;
  /** Linear gain (normalization, already peak-limited). */
  gain: number;
  /** Diagnostics and tests: a label. */
  label?: string;
}>;

const now = (c: BaseAudioContext) => c.currentTime;

export class Deck {
  readonly source: PcmSource;
  readonly label: string;
  readonly norm: GainNode;
  readonly fade: GainNode;
  /** Output frames at the context rate. */
  readonly outFrames: number;
  /** Integer context frame of the first output frame; undefined until scheduled. */
  startFrame?: number;
  /** Output frames decoded so far (scheduled or held). */
  decoded = 0;
  /** Output frames scheduled on the clock. */
  scheduled = 0;
  closed = false;
  ended = false;
  /** Held buffers before the deck has a start frame (a candidate), in order. */
  private held: AudioBuffer[] = [];
  private chunk: Float32Array[] | undefined;
  private chunkFill = 0;
  private readonly chunkFrames: number;
  private readonly resampler?: Resampler;
  private readonly context: BaseAudioContext;
  private readonly nodes = new Set<AudioBufferSourceNode>();
  private pumping = false;
  private exhausted = false;
  failure?: unknown;

  constructor(context: BaseAudioContext, destination: AudioNode, source: PcmSource, o: DeckOptions) {
    this.context = context;
    this.source = source;
    this.label = o.label ?? '';
    this.norm = context.createGain();
    this.fade = context.createGain();
    this.norm.gain.value = o.gain;
    this.gainValue = o.gain;
    this.norm.connect(this.fade);
    this.fade.connect(destination);
    const rate = context.sampleRate;
    if (source.sampleRate !== rate) this.resampler = new Resampler(source.sampleRate, rate, source.channels);
    this.outFrames = this.resampler ? Resampler.outputFrames(o.frames, source.sampleRate, rate) : o.frames;
    this.chunkFrames = Math.round(rate); // one second per buffer
  }

  get rate() { return this.context.sampleRate; }
  /** Context time of the frame after this deck's last. */
  get endFrame(): number | undefined { return this.startFrame === undefined ? undefined : this.startFrame + this.outFrames; }
  /** Seconds decoded past the playhead (or held, before starting). */
  get aheadSeconds(): number {
    if (this.startFrame === undefined) return this.decoded / this.rate;
    return Math.max(0, (this.startFrame + this.decoded) / this.rate - now(this.context));
  }
  /** Frames of this track played so far, at the context rate. */
  playedFrames(): number {
    if (this.startFrame === undefined) return 0;
    return Math.max(0, Math.min(this.outFrames, Math.floor(now(this.context) * this.rate) - this.startFrame));
  }

  /** Decodes until `seconds` of audio lie ahead of the playhead (or are held), then stops. */
  async pump(seconds: number): Promise<void> {
    if (this.pumping || this.closed) return;
    this.pumping = true;
    try {
      while (!this.closed && !this.exhausted && this.aheadSeconds < seconds) {
        const block = await this.source.read(Math.round(this.source.sampleRate / 4));
        if (this.closed) return;
        if (!block) {
          if (this.resampler) this.append(this.resampler.flush());
          this.exhausted = true;
          this.finishChunk();
          break;
        }
        this.append(this.resampler ? this.resampler.process(block.channels, block.frames) : block);
      }
    } catch (e) {
      this.failure = e;
    } finally {
      this.pumping = false;
    }
  }

  private append(block: {channels: readonly Float32Array[]; frames: number}) {
    let at = 0;
    const channels = block.channels;
    while (at < block.frames) {
      const room = Math.min(this.chunkFrames - this.chunkFill, this.outFrames - (this.decoded + this.chunkFill));
      if (room <= 0) { this.finishChunk(); if (this.decoded >= this.outFrames) return; continue; }
      if (!this.chunk) { this.chunk = channels.map(() => new Float32Array(this.chunkFrames)); this.chunkFill = 0; }
      const n = Math.min(room, block.frames - at);
      for (let ch = 0; ch < this.chunk.length; ch++) this.chunk[ch]!.set(channels[ch]!.subarray(at, at + n), this.chunkFill);
      this.chunkFill += n; at += n;
      if (this.chunkFill === this.chunkFrames) this.finishChunk();
    }
  }

  private finishChunk() {
    if (!this.chunk || this.chunkFill === 0) { this.chunk = undefined; return; }
    const buffer = this.context.createBuffer(this.chunk.length, this.chunkFill, this.rate);
    for (let ch = 0; ch < this.chunk.length; ch++) buffer.copyToChannel(this.chunk[ch]!.subarray(0, this.chunkFill) as Float32Array<ArrayBuffer>, ch);
    this.chunk = undefined;
    const frames = this.chunkFill;
    this.chunkFill = 0;
    if (this.startFrame === undefined) this.held.push(buffer);
    else this.play(buffer);
    this.decoded += frames;
  }

  private play(buffer: AudioBuffer) {
    const node = this.context.createBufferSource();
    node.buffer = buffer;
    node.connect(this.norm);
    const frame = this.startFrame! + this.scheduled;
    node.start(frame / this.rate);
    this.scheduled += buffer.length;
    this.nodes.add(node);
    node.onended = () => { this.nodes.delete(node); node.disconnect(); if (this.scheduled >= this.outFrames && this.nodes.size === 0) this.ended = true; };
  }

  /** Puts the deck on the clock at integer context frame `frame`. */
  start(frame: number) {
    if (this.startFrame !== undefined || this.closed) return;
    this.startFrame = frame;
    const held = this.held;
    this.held = [];
    for (const b of held) this.play(b);
  }

  /** The audio is exhausted and every scheduled frame has been handed to the clock. */
  get complete() { return this.exhausted && !this.chunk && this.held.length === 0 && this.scheduled >= this.decoded; }

  /** Ramps the normalization gain (20 ms, §9.6 rule 7). */
  private gainValue?: number;
  setGain(value: number) {
    if (value === this.gainValue) return;
    this.gainValue = value;
    const t = now(this.context), g = this.norm.gain;
    g.cancelScheduledValues(t);
    g.setValueAtTime(g.value, t);
    g.linearRampToValueAtTime(value, t + 0.02);
  }

  close() {
    if (this.closed) return;
    this.closed = true;
    for (const n of this.nodes) { n.onended = null; try { n.stop(); } catch { /* not started */ } n.disconnect(); }
    this.nodes.clear();
    this.held = [];
    this.chunk = undefined;
    this.norm.disconnect();
    this.fade.disconnect();
    this.source.close();
  }
}

/** Equal-power curves for a crossfade of `n` points. */
export function equalPower(n = 64): {fadeIn: Float32Array; fadeOut: Float32Array} {
  const fadeIn = new Float32Array(n), fadeOut = new Float32Array(n);
  for (let i = 0; i < n; i++) { const x = i / (n - 1); fadeIn[i] = Math.sin(x * Math.PI / 2); fadeOut[i] = Math.cos(x * Math.PI / 2); }
  return {fadeIn, fadeOut};
}

/**
 * Where the next track starts (context frames), gapless or crossfaded (§9.6 rule 4). The fade is
 * clamped to half the shorter track; same-album neighbors join gaplessly.
 */
export function edgeFrames(current: {endFrame: number; outFrames: number}, next: {outFrames: number}, o: {crossfadeSeconds: number; sameAlbum: boolean; rate: number}): {start: number; fade: number} {
  if (o.sameAlbum || o.crossfadeSeconds <= 0) return {start: current.endFrame, fade: 0};
  const fade = Math.max(0, Math.min(Math.round(o.crossfadeSeconds * o.rate), Math.floor(current.outFrames / 2), Math.floor(next.outFrames / 2)));
  return {start: current.endFrame - fade, fade};
}

/** Schedules `next` after `current` (both on one context). Returns where it starts. A link made
 * after its edge (`earliest`, the first frame still schedulable) starts late rather than cutting
 * frames from the incoming track, and shortens the fade to what's left. */
export function linkDecks(context: BaseAudioContext, current: Deck, next: Deck, o: {crossfadeSeconds: number; sameAlbum: boolean; earliest?: number}): {start: number; fade: number; late: boolean} {
  const end = current.endFrame;
  if (end === undefined) throw new Error('The current track is not on the clock.');
  const planned = edgeFrames({endFrame: end, outFrames: current.outFrames}, next, {...o, rate: context.sampleRate});
  const late = o.earliest !== undefined && planned.start < o.earliest;
  const edge = late ? {start: o.earliest!, fade: Math.max(0, Math.min(planned.fade, end - o.earliest!))} : planned;
  if (edge.fade > 0) {
    const {fadeIn, fadeOut} = equalPower();
    const t = edge.start / context.sampleRate, d = edge.fade / context.sampleRate;
    next.fade.gain.setValueAtTime(0, 0);
    next.fade.gain.setValueCurveAtTime(fadeIn, t, d);
    current.fade.gain.setValueCurveAtTime(fadeOut, t, d);
  }
  next.start(edge.start);
  return {...edge, late};
}
