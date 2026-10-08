import {ascii, Cursor, DemuxError, u16be, type ByteReader, type Demuxer, type Packet, type TrackConfig} from './types';

/**
 * Native FLAC (`fLaC` stream). Frames are found by their sync code and confirmed by the header's
 * CRC-8 and the frame's CRC-16, so a sync-like byte pair inside audio data is never a boundary.
 */
const CRC8 = new Uint8Array(256), CRC16 = new Uint16Array(256);
for (let i = 0; i < 256; i++) {
  let c = i;
  for (let k = 0; k < 8; k++) c = (c & 0x80 ? (c << 1) ^ 0x07 : c << 1) & 0xff;
  CRC8[i] = c;
  let d = i << 8;
  for (let k = 0; k < 8; k++) d = (d & 0x8000 ? (d << 1) ^ 0x8005 : d << 1) & 0xffff;
  CRC16[i] = d;
}
const crc8 = (b: Uint8Array, from: number, to: number) => { let c = 0; for (let i = from; i < to; i++) c = CRC8[c ^ b[i]!]!; return c; };
export const crc16 = (b: Uint8Array, from: number, to: number) => { let c = 0; for (let i = from; i < to; i++) c = ((c << 8) & 0xffff) ^ CRC16[(c >> 8) ^ b[i]!]!; return c; };

export type StreamInfo = Readonly<{minBlock: number; maxBlock: number; maxFrame: number; sampleRate: number; channels: number; bits: number; totalFrames: number; body: Uint8Array}>;
export type SeekPoint = Readonly<{sample: number; offset: number; frames: number}>;
export type FrameHeader = Readonly<{variable: boolean; blockSize: number; number: number; channels: number; length: number}>;

/** A frame header at `o` with a valid CRC-8, or undefined. */
export function flacFrameHeader(b: Uint8Array, o: number, info: StreamInfo): FrameHeader | undefined {
  if (o + 6 > b.length || b[o] !== 0xff || (b[o + 1]! & 0xfe) !== 0xf8) return undefined;
  const variable = (b[o + 1]! & 1) === 1;
  const sizeCode = b[o + 2]! >> 4, rateCode = b[o + 2]! & 0xf;
  const channelCode = b[o + 3]! >> 4, bitsCode = (b[o + 3]! >> 1) & 7;
  if (sizeCode === 0 || rateCode === 15 || channelCode > 10 || bitsCode === 3 || (b[o + 3]! & 1)) return undefined;
  // UTF-8-style coded frame or sample number.
  let p = o + 4, number = b[p]!, extra = 0;
  if (number < 0x80) extra = 0;
  else if ((number & 0xe0) === 0xc0) { extra = 1; number &= 0x1f; }
  else if ((number & 0xf0) === 0xe0) { extra = 2; number &= 0x0f; }
  else if ((number & 0xf8) === 0xf0) { extra = 3; number &= 0x07; }
  else if ((number & 0xfc) === 0xf8) { extra = 4; number &= 0x03; }
  else if ((number & 0xfe) === 0xfc) { extra = 5; number &= 0x01; }
  else if (number === 0xfe) { extra = 6; number = 0; }
  else return undefined;
  p++;
  if (p + extra + 3 > b.length) return undefined;
  for (let i = 0; i < extra; i++) { const x = b[p++]!; if ((x & 0xc0) !== 0x80) return undefined; number = number * 64 + (x & 0x3f); }
  let blockSize = 0;
  if (sizeCode === 1) blockSize = 192;
  else if (sizeCode <= 5) blockSize = 576 << (sizeCode - 2);
  else if (sizeCode === 6) blockSize = b[p++]! + 1;
  else if (sizeCode === 7) { blockSize = u16be(b, p) + 1; p += 2; }
  else blockSize = 256 << (sizeCode - 8);
  if (rateCode === 12) p += 1; else if (rateCode === 13 || rateCode === 14) p += 2;
  if (p >= b.length || crc8(b, o, p) !== b[p]) return undefined;
  const channels = channelCode < 8 ? channelCode + 1 : 2;
  if (channels !== info.channels || blockSize > info.maxBlock && info.maxBlock > 0) return undefined;
  return {variable, blockSize, number, channels, length: p + 1 - o};
}

export async function openFlac(reader: ByteReader): Promise<Demuxer & {info: StreamInfo; seekTable: readonly SeekPoint[]; firstFrame: number}> {
  const head = await reader.read(0, 1 << 16);
  let o = 0;
  if (ascii(head, 0, 3) === 'ID3') o = 10 + (((head[6]! & 0x7f) << 21) | ((head[7]! & 0x7f) << 14) | ((head[8]! & 0x7f) << 7) | (head[9]! & 0x7f));
  const c = new Cursor(reader, o);
  if (!(await c.need(4)) || ascii(c.view, 0, 4) !== 'fLaC') throw new DemuxError('Not a FLAC stream.');
  c.skip(4);
  let info: StreamInfo | undefined;
  const seekTable: SeekPoint[] = [];
  for (;;) {
    if (!(await c.need(4))) throw new DemuxError('Truncated FLAC metadata.');
    const v = c.view, last = (v[0]! & 0x80) !== 0, type = v[0]! & 0x7f, length = (v[1]! << 16) | (v[2]! << 8) | v[3]!;
    c.skip(4);
    if (!(await c.need(length))) throw new DemuxError('Truncated FLAC metadata.');
    const body = c.view.slice(0, length);
    if (type === 0 && length >= 34) {
      const rate = (body[10]! << 12) | (body[11]! << 4) | (body[12]! >> 4);
      const channels = ((body[12]! >> 1) & 7) + 1, bits = (((body[12]! & 1) << 4) | (body[13]! >> 4)) + 1;
      const total = (body[13]! & 0xf) * 2 ** 32 + ((body[14]! << 24) >>> 0) + (body[15]! << 16) + (body[16]! << 8) + body[17]!;
      info = {minBlock: u16be(body, 0), maxBlock: u16be(body, 2), maxFrame: (body[7]! << 16) | (body[8]! << 8) | body[9]!, sampleRate: rate, channels, bits, totalFrames: total, body: body.slice(0, 34)};
    } else if (type === 3) {
      for (let i = 0; i + 18 <= length; i += 18) {
        const hi = ((body[i]! << 24) >>> 0) * 2 ** 32 + ((body[i + 4]! << 24) >>> 0) + (body[i + 5]! << 16) + (body[i + 6]! << 8) + body[i + 7]!;
        if (body[i] === 0xff && body[i + 1] === 0xff) continue; // placeholder point
        const off = ((body[i + 8]! << 24) >>> 0) * 2 ** 32 + ((body[i + 12]! << 24) >>> 0) + (body[i + 13]! << 16) + (body[i + 14]! << 8) + body[i + 15]!;
        seekTable.push({sample: hi, offset: off, frames: u16be(body, i + 16)});
      }
    }
    c.skip(length);
    if (last) break;
  }
  if (!info || !info.sampleRate || !info.channels) throw new DemuxError('FLAC STREAMINFO is missing.');
  const firstFrame = c.pos;
  // WebCodecs wants "fLaC" and a STREAMINFO block (header included) as the description.
  const description = new Uint8Array(42);
  description.set([0x66, 0x4c, 0x61, 0x43, 0x80, 0, 0, 34]);
  description.set(info.body, 8);
  const config: TrackConfig = Object.freeze({codec: 'flac', family: 'flac', sampleRate: info.sampleRate, numberOfChannels: info.channels, description});
  const streamInfo = info;
  return {
    config, info, seekTable, firstFrame,
    packets: (signal?: AbortSignal) => flacFrames(reader, firstFrame, streamInfo, signal),
  };
}

/** Frames from `from` (which must be a frame start). */
export async function* flacFrames(reader: ByteReader, from: number, info: StreamInfo, signal?: AbortSignal): AsyncGenerator<Packet & {header: FrameHeader}> {
  const c = new Cursor(reader, from, 1 << 17, signal);
  const reach = Math.max(info.maxFrame || 0, 1 << 16) + 32;
  while (await c.need(6)) {
    const h = flacFrameHeader(c.view, 0, info);
    if (!h) { c.skip(1); continue; }
    await c.need(reach);
    const v = c.view;
    // The frame ends where the next valid frame begins and the CRC-16 of what lies between matches.
    let end = -1;
    for (let p = h.length + 2; p + 1 < v.length; p++) {
      if (v[p] !== 0xff || (v[p + 1]! & 0xfe) !== 0xf8) continue;
      const next = flacFrameHeader(v, p, info);
      if (next && next.variable === h.variable && crc16(v, 0, p - 2) === u16be(v, p - 2)) { end = p; break; }
    }
    if (end < 0) {
      if (!c.eof) {
        // A frame longer than the window (a stream without max frame size): widen and look again.
        if (!(await c.need(c.view.length * 2))) { /* at the end */ }
        if (!c.eof) continue;
      }
      // The last frame runs to the end of the file (less any trailing junk the CRC rejects).
      const tail = c.view;
      for (let len = tail.length; len >= h.length + 2; len--) {
        if (crc16(tail, 0, len - 2) === u16be(tail, len - 2)) { end = len; break; }
        if (tail.length - len > 256) break;
      }
      if (end < 0) return;
    }
    const offset = c.pos;
    const data = c.view.slice(0, end);
    c.skip(end);
    yield {data, frames: h.blockSize, offset, header: h};
  }
}
