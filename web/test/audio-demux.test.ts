import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync, readdirSync} from 'node:fs';
import {openDemuxer} from '../src/bridge/audio/demux.ts';

const dir = new URL('../../fixtures/audio-gapless/', import.meta.url);
export const fileReader = (bytes: Uint8Array) => ({size: bytes.length, read: async (o: number, n: number) => bytes.slice(o, Math.min(bytes.length, o + n))});
const sidecars = readdirSync(dir).filter(f => f.endsWith('.json') && f !== 'album.json').map(f => JSON.parse(readFileSync(new URL(f, dir), 'utf8')));

test('every §18.8 fixture demuxes into packets whose frames add up to the raw decoded length', async () => {
  for (const s of sidecars) {
    const bytes = new Uint8Array(readFileSync(new URL(s.file, dir)));
    const d = await openDemuxer(s.container, fileReader(bytes));
    assert.equal(d.config.sampleRate, s.sampleRate, s.file);
    assert.equal(d.config.numberOfChannels, s.channels, s.file);
    let frames = 0, packets = 0, unknown = 0;
    for await (const p of d.packets()) { packets++; if (p.frames === undefined) unknown++; else frames += p.frames; }
    assert.ok(packets > 0, s.file);
    assert.equal(unknown, 0, s.file);
    assert.equal(frames, s.rawFrames, `${s.file}: ${packets} packets`);
  }
});

test('the JavaScript FLAC decoder reproduces every FLAC fixture exactly', async () => {
  const {JsFlacDecoder} = await import('../src/bridge/audio/flac-decoder.ts');
  for (const s of sidecars.filter(x => x.codec === 'flac')) {
    const d = await openDemuxer(s.container, fileReader(new Uint8Array(readFileSync(new URL(s.file, dir)))));
    const decoder = new JsFlacDecoder(d.config);
    let n = 0, worst = 0;
    const {amplitude: a, frequency: f, offset} = s.signal;
    for await (const p of d.packets()) {
      decoder.decode(p);
      for (const block of decoder.take()) {
        assert.equal(block.channels.length, s.channels);
        for (let i = 0; i < block.frames; i++, n++) {
          const want = a * Math.sin(2 * Math.PI * f * (n + offset) / s.sampleRate);
          worst = Math.max(worst, Math.abs(block.channels[0]![i]! - want), Math.abs(block.channels[1]![i]! - want));
        }
      }
    }
    assert.equal(n, s.durationFrames, s.file);
    assert.ok(worst <= s.tolerance, `${s.file}: ${worst}`);
  }
});
