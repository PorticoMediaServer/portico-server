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

test('a FLAC is started part-way from the frames’ own positions, exactly, without reading what comes before', async () => {
  const {flacSeek, openFlac} = await import('../src/bridge/audio/flac.ts');
  const flacs = sidecars.filter(x => x.codec === 'flac');
  assert.ok(flacs.length > 0);
  let sought = 0;
  for (const s of flacs) {
    const bytes = new Uint8Array(readFileSync(new URL(s.file, dir)));
    // The whole file walked once: where every frame is and which sample it starts at.
    const walked: {offset: number; raw: number; frames: number}[] = [];
    let raw = 0;
    const whole = await openFlac(fileReader(bytes));
    for await (const p of whole.packets()) { walked.push({offset: p.offset, raw, frames: p.frames!}); raw += p.frames!; }
    const block = whole.info.maxBlock || 4096;
    for (const fraction of [0.5, 0.75, 0.97]) {
      const target = Math.floor(s.rawFrames * fraction);
      const touched: number[] = [];
      const counting = {size: bytes.length, read: async (o: number, n: number) => { touched.push(o); return bytes.slice(o, Math.min(bytes.length, o + n)); }};
      // The fixtures are a few seconds long: "close enough" is scaled down to them.
      // No seek table is given, so the place is found by proportion and the frames' own numbers.
      const found = await flacSeek(counting, whole.firstFrame, whole.info, [], target, 0, undefined, {near: block * 2});
      assert.ok(found, `${s.file} at ${fraction}: a start was confirmed`);
      const first = await found!.packets.next();
      assert.equal(first.done, false);
      const same = walked.find(w => w.offset === first.value!.offset);
      assert.ok(same, `${s.file}: the start is a real frame`);
      assert.equal(found!.rawStart, same!.raw, `${s.file}: its position is exact`);
      assert.ok(found!.rawStart <= target, 'the frame holding the position is not skipped');
      assert.ok(target - found!.rawStart <= block * 8, `${s.file}: and the start is close to it (${target - found!.rawStart} frames early)`);
      assert.ok(touched.length <= 12, `${s.file}: a bounded number of reads (${touched.length})`);
      await found!.packets.return(undefined);
      sought++;
      // With a seek table, its point is used once the frame found there confirms it; a wrong point is not trusted.
      const at = walked[Math.floor(walked.length * 0.6)]!;
      const good = await flacSeek(fileReader(bytes), whole.firstFrame, whole.info, [{sample: at.raw, offset: at.offset - whole.firstFrame, frames: at.frames}], at.raw + block, 0, undefined, {near: block * 4});
      assert.equal(good?.rawStart, at.raw);
      const bad = await flacSeek(fileReader(bytes), whole.firstFrame, whole.info, [{sample: at.raw + 12_345, offset: at.offset - whole.firstFrame + 7, frames: at.frames}], at.raw + 12_345 + block, 0, undefined, {near: block * 4});
      if (bad) assert.ok(walked.some(w => w.raw === bad.rawStart), 'a wrong point never yields a wrong position');
    }
  }
  assert.ok(sought >= 3);
});
