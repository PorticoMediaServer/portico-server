import {ascii, Cursor, DemuxError, type ByteReader, type Demuxer, type Packet, type TrackConfig} from './types';

/**
 * MPEG audio (Layer III, and Layer II) in a raw `.mp3`. The first frame is dropped when it is a
 * Xing/Info or VBRI header, as FFmpeg does: it carries no audio, and the plan's `trim` counts raw
 * frames without it (§18.1, the fixtures' `rawFrames`).
 */
const BITRATES: Record<string, readonly number[]> = {
  // [version][layer] → kbit/s by index (index 0 = free, 15 = bad).
  '1-3': [0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320],
  '1-2': [0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384],
  '2-3': [0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160],
  '2-2': [0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160],
};
const RATES: Record<number, readonly number[]> = {3: [44100, 48000, 32000], 2: [22050, 24000, 16000], 0: [11025, 12000, 8000]};

export type Mp3Header = Readonly<{version: number; layer: number; sampleRate: number; channels: number; length: number; frames: number; crc: boolean}>;

export function mp3Header(b: Uint8Array, o: number): Mp3Header | undefined {
  if (o + 4 > b.length || b[o] !== 0xff || (b[o + 1]! & 0xe0) !== 0xe0) return undefined;
  const versionBits = (b[o + 1]! >> 3) & 3, layerBits = (b[o + 1]! >> 1) & 3;
  if (versionBits === 1 || layerBits === 0) return undefined;
  const layer = 4 - layerBits;
  if (layer === 1) return undefined; // Layer I is not music we play.
  const crc = (b[o + 1]! & 1) === 0;
  const bitrateIndex = b[o + 2]! >> 4, rateIndex = (b[o + 2]! >> 2) & 3, padding = (b[o + 2]! >> 1) & 1;
  if (bitrateIndex === 0 || bitrateIndex === 15 || rateIndex === 3) return undefined;
  const mpeg1 = versionBits === 3;
  const kbps = BITRATES[`${mpeg1 ? 1 : 2}-${layer}`]![bitrateIndex]!;
  const sampleRate = RATES[versionBits]![rateIndex]!;
  const frames = layer === 3 && !mpeg1 ? 576 : 1152;
  const length = Math.floor((frames / 8) * kbps * 1000 / sampleRate) + padding;
  const channels = (b[o + 3]! >> 6) === 3 ? 1 : 2;
  return {version: versionBits, layer, sampleRate, channels, length, frames, crc};
}

/** Whether a frame is a Xing/Info or VBRI header frame (no audio). */
export function isInfoFrame(b: Uint8Array, o: number, h: Mp3Header): boolean {
  if (h.layer !== 3) return false;
  const side = h.version === 3 ? (h.channels === 1 ? 17 : 32) : (h.channels === 1 ? 9 : 17);
  const at = o + 4 + (h.crc ? 2 : 0) + side;
  const tag = ascii(b, at, 4);
  return tag === 'Xing' || tag === 'Info' || ascii(b, o + 36, 4) === 'VBRI';
}

/** Skips an ID3v2 tag at `o`; returns the offset after it. */
export function afterId3(b: Uint8Array, o: number): number {
  if (ascii(b, o, 3) !== 'ID3' || o + 10 > b.length) return o;
  const size = ((b[o + 6]! & 0x7f) << 21) | ((b[o + 7]! & 0x7f) << 14) | ((b[o + 8]! & 0x7f) << 7) | (b[o + 9]! & 0x7f);
  return o + 10 + size + ((b[o + 5]! & 0x10) ? 10 : 0);
}

export async function openMp3(reader: ByteReader): Promise<Demuxer> {
  const head = await reader.read(0, 1 << 16);
  let o = afterId3(head, 0);
  // Tags larger than the first block: read from after them.
  const probe = o + 8192 > head.length ? await reader.read(o, 8192) : head.subarray(o);
  const base = o;
  let first: Mp3Header | undefined, at = 0;
  for (; at + 4 <= probe.length; at++) {
    const h = mp3Header(probe, at);
    // A header is real when the next frame's header agrees with it.
    if (h && h.length > 4) {
      const next = mp3Header(probe, at + h.length);
      if (next && next.sampleRate === h.sampleRate && next.layer === h.layer && next.version === h.version) { first = h; break; }
    }
  }
  if (!first) throw new DemuxError('No MPEG audio frames were found.');
  const start = base + at;
  const config: TrackConfig = Object.freeze({codec: 'mp3', family: 'mp3', sampleRate: first.sampleRate, numberOfChannels: first.channels});
  return {
    config,
    async *packets(signal?: AbortSignal): AsyncGenerator<Packet> {
      const c = new Cursor(reader, start, 1 << 16, signal);
      let index = 0;
      while (await c.need(4)) {
        const v = c.view;
        const h = mp3Header(v, 0);
        if (!h || h.sampleRate !== first!.sampleRate || h.layer !== first!.layer) {
          // Trailing tags (ID3v1, APE) or junk: resynchronize, or stop at the end.
          if (ascii(v, 0, 3) === 'TAG' || ascii(v, 0, 8) === 'APETAGEX') return;
          c.skip(1);
          continue;
        }
        if (!(await c.need(h.length))) return; // a truncated last frame is not audio
        const data = c.view.slice(0, h.length);
        const offset = c.pos;
        c.skip(h.length);
        if (index++ === 0 && isInfoFrame(data, 0, h)) continue;
        yield {data, frames: h.frames, offset};
      }
    },
  };
}
