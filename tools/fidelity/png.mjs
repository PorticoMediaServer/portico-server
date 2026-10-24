import {inflateSync} from 'node:zlib';

const signature = Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]);
const table = Array.from({length: 256}, (_, n) => {
  for (let bit = 0; bit < 8; bit++) n = n & 1 ? 0xedb88320 ^ (n >>> 1) : n >>> 1;
  return n >>> 0;
});
export function crc32(bytes) {
  let crc = 0xffffffff;
  for (const byte of bytes) crc = table[(crc ^ byte) & 255] ^ (crc >>> 8);
  return (crc ^ 0xffffffff) >>> 0;
}
const paeth = (a, b, c) => {
  const p = a + b - c, da = Math.abs(p - a), db = Math.abs(p - b), dc = Math.abs(p - c);
  return da <= db && da <= dc ? a : db <= dc ? b : c;
};

/** Bounded Chromium RGB/RGBA PNG decoder; buffers stay in memory, never evidence. */
export function decodePNG(bytes) {
  if (!Buffer.isBuffer(bytes) || bytes.length > 64 * 1024 * 1024 || !bytes.subarray(0, 8).equals(signature)) throw new Error('Invalid PNG input');
  let width, height, channels, offset = 8, ended = false;
  const compressed = [];
  while (offset < bytes.length) {
    if (offset + 12 > bytes.length) throw new Error('Truncated PNG chunk');
    const size = bytes.readUInt32BE(offset), type = bytes.toString('ascii', offset + 4, offset + 8);
    if (offset + size + 12 > bytes.length) throw new Error('Truncated PNG data');
    const data = bytes.subarray(offset + 8, offset + 8 + size);
    if (crc32(bytes.subarray(offset + 4, offset + 8 + size)) !== bytes.readUInt32BE(offset + 8 + size)) throw new Error('PNG checksum mismatch');
    if (type === 'IHDR') {
      if (width !== undefined || offset !== 8 || size !== 13) throw new Error('Invalid PNG header');
      width = data.readUInt32BE(0); height = data.readUInt32BE(4);
      if (!width || !height || width * height > 16_000_000 || data[8] !== 8 || ![2, 6].includes(data[9]) || data[10] || data[11] || data[12]) throw new Error('Unsupported PNG layout');
      channels = data[9] === 6 ? 4 : 3;
    } else if (type === 'IDAT') {
      if (width === undefined) throw new Error('PNG data precedes header');
      compressed.push(data);
    } else if (type === 'IEND') {
      if (size || offset + 12 !== bytes.length) throw new Error('Invalid PNG end');
      ended = true; break;
    } else if (type[0] === type[0].toUpperCase()) throw new Error('Unsupported critical PNG chunk');
    offset += size + 12;
  }
  if (!ended || !channels || !compressed.length) throw new Error('Incomplete PNG');
  const stride = width * channels, expected = (stride + 1) * height;
  const raw = inflateSync(Buffer.concat(compressed), {maxOutputLength: expected});
  if (raw.length !== expected) throw new Error('PNG scanline length mismatch');
  const rgb = Buffer.alloc(width * height * channels);
  for (let y = 0; y < height; y++) {
    const filter = raw[y * (stride + 1)];
    if (filter > 4) throw new Error('Invalid PNG filter');
    for (let x = 0; x < stride; x++) {
      const index = y * stride + x, left = x >= channels ? rgb[index - channels] : 0;
      const up = y ? rgb[index - stride] : 0, upperLeft = y && x >= channels ? rgb[index - stride - channels] : 0;
      const predictor = [0, left, up, Math.floor((left + up) / 2), paeth(left, up, upperLeft)][filter];
      rgb[index] = (raw[y * (stride + 1) + x + 1] + predictor) & 255;
    }
  }
  const rgba = Buffer.alloc(width * height * 4, 255);
  for (let pixel = 0; pixel < width * height; pixel++) for (let channel = 0; channel < channels; channel++) rgba[pixel * 4 + channel] = rgb[pixel * channels + channel];
  return {width, height, rgba};
}

/** Exact full-frame comparison. No implicit thresholds, blur or crop. */
export function comparePixels(reference, actual) {
  if (reference.width !== actual.width || reference.height !== actual.height) throw new Error('Full-screen raster dimensions differ');
  const totalPixels = reference.width * reference.height;
  if (reference.rgba.length !== totalPixels * 4 || actual.rgba.length !== totalPixels * 4) throw new Error('Invalid pixel buffer');
  let changedPixels = 0, absoluteChannelError = 0, maximumChannelError = 0;
  let left = reference.width, top = reference.height, right = -1, bottom = -1;
  for (let pixel = 0; pixel < totalPixels; pixel++) {
    let changed = false;
    for (let channel = 0; channel < 4; channel++) {
      const error = Math.abs(reference.rgba[pixel * 4 + channel] - actual.rgba[pixel * 4 + channel]);
      changed ||= error !== 0; absoluteChannelError += error; maximumChannelError = Math.max(maximumChannelError, error);
    }
    if (changed) {
      changedPixels++;
      const x = pixel % reference.width, y = Math.floor(pixel / reference.width);
      left = Math.min(left, x); right = Math.max(right, x); top = Math.min(top, y); bottom = Math.max(bottom, y);
    }
  }
  return {totalPixels, changedPixels, exactPixels: totalPixels - changedPixels,
    exactPixelPercent: 100 * (totalPixels - changedPixels) / totalPixels,
    meanAbsoluteChannelError: absoluteChannelError / (totalPixels * 4), maximumChannelError,
    differenceBounds: changedPixels ? {x: left, y: top, width: right - left + 1, height: bottom - top + 1} : null};
}
