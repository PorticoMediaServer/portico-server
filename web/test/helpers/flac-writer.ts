import {crc16} from '../../src/bridge/audio/flac.ts';

/** A minimal FLAC encoder for tests: 16-bit stereo, VERBATIM subframes, fixed block size. */
export function encodeFlac(left: Float32Array, right: Float32Array, rate: number, block = 4096): Uint8Array {
  const out: number[] = [...'fLaC'].map(c => c.charCodeAt(0));
  const total = left.length;
  const si = new Uint8Array(34);
  si[0] = block >> 8; si[1] = block & 255; si[2] = block >> 8; si[3] = block & 255;
  si[10] = rate >> 12; si[11] = (rate >> 4) & 255; si[12] = ((rate & 15) << 4) | (1 << 1) | 0; si[13] = (15 << 4) | Math.floor(total / 2 ** 32);
  si[14] = (total >>> 24) & 255; si[15] = (total >>> 16) & 255; si[16] = (total >>> 8) & 255; si[17] = total & 255;
  out.push(0x80, 0, 0, 34, ...si);
  const crc8 = (b: number[]) => { let c = 0; for (const x of b) { c ^= x; for (let k = 0; k < 8; k++) c = c & 0x80 ? ((c << 1) ^ 7) & 255 : (c << 1) & 255; } return c; };
  const utf8 = (n: number) => n < 0x80 ? [n] : n < 0x800 ? [0xc0 | (n >> 6), 0x80 | (n & 63)] : [0xe0 | (n >> 12), 0x80 | ((n >> 6) & 63), 0x80 | (n & 63)];
  for (let f = 0, at = 0; at < total; f++, at += block) {
    const n = Math.min(block, total - at);
    const header = [0xff, 0xf8, (7 << 4) | 0, (1 << 4) | (4 << 1), ...utf8(f), (n - 1) >> 8, (n - 1) & 255];
    header.push(crc8(header));
    const frame = [...header];
    for (const ch of [left, right]) {
      frame.push(0b00000010);
      for (let i = 0; i < n; i++) { const v = Math.max(-32768, Math.min(32767, Math.round(ch[at + i]! * 32768))) & 0xffff; frame.push(v >> 8, v & 255); }
    }
    const bytes = Uint8Array.from(frame), c = crc16(bytes, 0, bytes.length);
    out.push(...frame, c >> 8, c & 255);
  }
  return Uint8Array.from(out);
}
