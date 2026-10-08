import {createDecoder, type PacketDecoder, type Planar} from './decoder';
import {openDemuxer} from './demux';
import {JsFlacDecoder} from './flac-decoder';
import {DemuxError, u16le, type ByteReader, type Demuxer, type Packet, type TrackConfig} from './types';

/**
 * One track, decoded and trimmed exactly (§18.1, §9.6 rule 2): `read()` returns the track's frames
 * [fromFrame, durationFrames) at the file's own rate, and nothing else. The raw decoded output is
 * counted from the packets fed; a platform decoder that drops leading frames by itself (Safari's
 * MP3 decoder drops its 529-frame delay; every Opus decoder applies the pre-skip) is accounted for
 * from the conformance table, so nothing is trimmed twice.
 */
export type TrackPlan = Readonly<{id: string; container: string; codec: string; sampleRate: number; channels: number; durationFrames: number; trim: Readonly<{startFrames: number; endFrames: number}>; bytes?: number}>;

export type BrowserEngine = 'blink' | 'webkit' | 'gecko' | 'other';
export function browserEngine(ua = typeof navigator === 'undefined' ? '' : navigator.userAgent): BrowserEngine {
  if (/Firefox\//.test(ua)) return 'gecko';
  if (/Chrome\/|Chromium\/|Edg\//.test(ua)) return 'blink';
  if (/AppleWebKit\//.test(ua)) return 'webkit';
  return 'other';
}

/**
 * Leading frames each engine's WebCodecs decoder drops by itself, per codec family, as measured on
 * the §18.8 fixtures (Chrome 152, Safari 27; `src/dev/audio-conformance.ts`). `preskip` means the
 * Opus header's pre-skip. A family missing here isn't used through WebCodecs on that engine.
 */
export const WEBCODECS_SKIP: Readonly<Record<BrowserEngine, Readonly<Record<string, number | 'preskip'>>>> = {
  blink: {mp3: 0, aac: 0, flac: 0, opus: 'preskip'},
  webkit: {mp3: 529, aac: 0, opus: 'preskip'},
  gecko: {},
  other: {},
};

/** Packets a decoder must see before the one it should output from, after a seek. */
const PREROLL: Record<string, number> = {mp3: 3, aac: 2, opus: 4, vorbis: 2};

export type DecoderChoice = Readonly<{decoder: PacketDecoder; skip: number; via: string}>;

export async function chooseDecoder(config: TrackConfig, engine = browserEngine()): Promise<DecoderChoice | undefined> {
  if (config.family === 'pcm') { const d = await createDecoder(config); return d && {decoder: d, skip: 0, via: 'pcm'}; }
  const known = WEBCODECS_SKIP[engine][config.family];
  if (known !== undefined) {
    const d = await createDecoder(config);
    if (d) return {decoder: d, skip: known === 'preskip' ? u16le(config.description ?? new Uint8Array(12), 10) : known, via: `webcodecs-${engine}`};
  }
  if (config.family === 'flac') return {decoder: new JsFlacDecoder(config), skip: 0, via: 'js-flac'};
  return undefined;
}

export class TrackStream {
  readonly sampleRate: number;
  readonly channels: number;
  readonly via: string;
  /** Trimmed frames still to come. */
  private remaining: number;
  private raw: number;
  private readonly from: number;
  private readonly to: number;
  private readonly decoder: PacketDecoder;
  private readonly packets: AsyncGenerator<Packet>;
  private pending: Planar[] = [];
  private ended = false;
  private closed = false;
  private readonly pad: boolean;
  /** Trimmed frames returned so far. */
  delivered = 0;

  private constructor(o: {plan: TrackPlan; choice: DecoderChoice; packets: AsyncGenerator<Packet>; rawStart: number; fromFrame: number; pad: boolean}) {
    this.pad = o.pad;
    this.sampleRate = o.plan.sampleRate;
    this.channels = o.plan.channels;
    this.via = o.choice.via;
    this.decoder = o.choice.decoder;
    this.packets = o.packets;
    this.raw = o.rawStart + o.choice.skip;
    this.from = o.plan.trim.startFrames + o.fromFrame;
    this.to = o.plan.trim.startFrames + o.plan.durationFrames;
    this.remaining = Math.max(0, o.plan.durationFrames - o.fromFrame);
  }

  /** Opens `plan` from trimmed frame `fromFrame` (a seek). Undefined when no decoder here can play it. */
  static async open(plan: TrackPlan, reader: ByteReader, fromFrame = 0, signal?: AbortSignal, o: {pad?: boolean} = {}): Promise<TrackStream | undefined> {
    const demuxer = await openDemuxer(plan.container, reader);
    if (demuxer.config.sampleRate !== plan.sampleRate) throw new DemuxError('The file’s sample rate isn’t the plan’s.');
    const choice = await chooseDecoder(demuxer.config);
    if (!choice) return undefined;
    const target = plan.trim.startFrames + Math.max(0, Math.min(fromFrame, plan.durationFrames));
    const {packets, rawStart} = await startAt(demuxer, target - choice.skip, PREROLL[demuxer.config.family] ?? 0, signal);
    return new TrackStream({plan, choice, packets, rawStart, fromFrame: Math.max(0, fromFrame), pad: o.pad !== false});
  }

  get done() { return this.remaining === 0; }

  /** Up to `max` frames of the track, or undefined at its end. */
  async read(max: number): Promise<Planar | undefined> {
    while (!this.closed && this.remaining > 0) {
      const block = this.pending.shift();
      if (block) {
        const start = this.raw, end = start + block.frames;
        this.raw = end;
        const a = Math.max(start, this.from), b = Math.min(end, this.to);
        if (b <= a) continue;
        const take = Math.min(b - a, max, this.remaining);
        const channels = block.channels.map(c => c.subarray(a - start, a - start + take));
        this.remaining -= take;
        this.delivered += take;
        if (a - start + take < block.frames) {
          // Keep the rest of this block for the next read.
          const rest = a - start + take;
          this.raw = start + rest;
          this.pending.unshift({channels: block.channels.map(c => c.subarray(rest)), frames: block.frames - rest});
        }
        return {channels, frames: take};
      }
      if (this.ended) {
        // A stream that ended early (a converted stream cut at a prepared presentation's budget):
        // the caller continues it from `delivered`.
        if (!this.pad) return undefined;
        // The decoder produced less than the plan says: pad with silence so every later edge stays
        // exact (a short file is a server fact error, not a reason to shift the album).
        const take = Math.min(max, this.remaining);
        this.remaining -= take;
        this.delivered += take;
        return {channels: Array.from({length: this.channels}, () => new Float32Array(take)), frames: take};
      }
      const next = await this.packets.next();
      if (next.done) { this.pending.push(...await this.decoder.flush()); this.ended = true; continue; }
      this.decoder.decode(next.value);
      this.pending.push(...await this.decoder.drain(8));
    }
    return undefined;
  }

  close() {
    if (this.closed) return;
    this.closed = true;
    this.decoder.close();
    void this.packets.return(undefined).catch(() => {});
    this.pending = [];
  }
}

/**
 * Packets from the one that lets the decoder output raw frame `target`, `preroll` packets earlier
 * so it has settled. `rawStart` is the raw index of the first packet's first frame. Packets are
 * skipped by their known lengths without decoding.
 */
async function startAt(demuxer: Demuxer, target: number, preroll: number, signal?: AbortSignal): Promise<{packets: AsyncGenerator<Packet>; rawStart: number}> {
  if (target <= 0) return {packets: demuxer.packets(signal), rawStart: 0};
  if (demuxer.seek) return demuxer.seek(target, preroll, signal);
  const all = demuxer.packets(signal);
  const recent: {packet: Packet; raw: number}[] = [];
  let raw = 0;
  for (;;) {
    const next = await all.next();
    if (next.done) break;
    const p = next.value;
    if (p.frames === undefined) throw new DemuxError('This stream can’t be started part-way.');
    recent.push({packet: p, raw});
    if (recent.length > preroll + 1) recent.shift();
    if (raw + p.frames > target) break;
    raw += p.frames;
  }
  const first = recent[0];
  const held = recent.map(r => r.packet);
  async function* chain(): AsyncGenerator<Packet> { yield* held; yield* all; }
  return {packets: chain(), rawStart: first?.raw ?? raw};
}
