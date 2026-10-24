import test from 'node:test';
import assert from 'node:assert/strict';
import {deflateSync} from 'node:zlib';
import {comparePixels, crc32, decodePNG} from './png.mjs';

function png(filter, colorType = 6) {
  const chunk = (name, value) => {
    const bytes = Buffer.alloc(12 + value.length); bytes.writeUInt32BE(value.length);
    bytes.write(name, 4); value.copy(bytes, 8); bytes.writeUInt32BE(crc32(bytes.subarray(4, 8 + value.length)), 8 + value.length); return bytes;
  };
  const header = Buffer.alloc(13); header.writeUInt32BE(1); header.writeUInt32BE(1, 4); header[8] = 8; header[9] = colorType;
  return Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk('IHDR', header),
    chunk('IDAT', deflateSync(Buffer.from([filter, 46, 63, 73, ...(colorType === 6 ? [255] : [])]))), chunk('IEND', Buffer.alloc(0))]);
}
test('PNG decoder accepts every standard first-pixel filter and opaque RGB/RGBA', () => {
  for (const filter of [0, 1, 2, 3, 4]) for (const type of [2, 6]) assert.deepEqual(decodePNG(png(filter, type)), {width: 1, height: 1, rgba: Buffer.from([46, 63, 73, 255])});
});
test('malformed or unbounded raster cannot produce a comparison result', () => {
  const corrupted = png(0); corrupted[30] ^= 1;
  assert.throws(() => decodePNG(corrupted), /checksum/);
  assert.throws(() => decodePNG(png(5)), /filter/);
  assert.throws(() => decodePNG(png(0).subarray(0, 30)), /Truncated/);
  assert.throws(() => comparePixels({width: 1, height: 1, rgba: Buffer.alloc(4)}, {width: 2, height: 1, rgba: Buffer.alloc(8)}), /dimensions differ/);
});
test('one changed channel in the full frame is measured without a tolerance or crop', () => {
  const reference = {width: 2, height: 1, rgba: Buffer.from([46, 63, 73, 255, 46, 63, 73, 255])};
  const actual = {...reference, rgba: Buffer.from(reference.rgba)}; actual.rgba[5]++;
  assert.deepEqual(comparePixels(reference, actual), {totalPixels: 2, changedPixels: 1, exactPixels: 1, exactPixelPercent: 50,
    meanAbsoluteChannelError: .125, maximumChannelError: 1, differenceBounds: {x: 1, y: 0, width: 1, height: 1}});
});
