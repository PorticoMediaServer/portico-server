import test from 'node:test';
import assert from 'node:assert/strict';
import {Deck, edgeFrames, linkDecks} from '../src/bridge/audio/engine.ts';
import {fakeContext, sineSource} from './helpers/fake-audio.ts';

test('(j) memory is bounded: a 2-hour 24/96 track holds at most the ahead window decoded', async () => {
  const rate = 96000, f = fakeContext(rate), frames = 2 * 3600 * rate;
  const deck = new Deck(f.ctx, {} as AudioNode, sineSource(frames, rate), {frames, gain: 1});
  await deck.pump(2);
  deck.start(Math.ceil(0.08 * rate));
  let worst = 0;
  // Ten minutes of playback in half-second steps, pumping as the tick does.
  for (let t = 0; t < 600; t += 0.5) {
    await deck.pump(30);
    worst = Math.max(worst, f.unplayed() / rate);
    f.advance(0.5);
  }
  assert.ok(worst <= 31.1, `decoded ahead ${worst.toFixed(2)} s`);
  assert.ok(deck.decoded < 700 * rate, 'only what the clock reached plus the window was decoded');
  deck.close();
});

test('(b) a gapless successor starts at the frame after the current track’s last frame', async () => {
  const rate = 44100, f = fakeContext(rate);
  const a = new Deck(f.ctx, {} as AudioNode, sineSource(110000, rate), {frames: 110000, gain: 1});
  const b = new Deck(f.ctx, {} as AudioNode, sineSource(117919, rate), {frames: 117919, gain: 1});
  await a.pump(100); await b.pump(10);
  a.start(3528);
  const edge = linkDecks(f.ctx, a, b, {crossfadeSeconds: 6, sameAlbum: true});
  assert.equal(edge.start, 3528 + 110000);
  assert.equal(edge.fade, 0, 'same-album neighbors never crossfade');
  const aFrames = f.scheduled.filter(s => s.node.buffer.length && s.start < edge.start);
  assert.equal(aFrames.reduce((n, s) => n + s.frames, 0), 110000, 'exactly the track’s frames');
  assert.equal(Math.max(...aFrames.map(s => s.start + s.frames)), edge.start, 'no gap and no overlap');
  assert.equal(Math.min(...f.scheduled.filter(s => s.start >= edge.start).map(s => s.start)), edge.start);
});

test('(c) crossfade: starts crossfadeSeconds before the end, clamped to half the shorter track; late links start late', () => {
  const rate = 48000;
  assert.deepEqual(edgeFrames({endFrame: 1_000_000, outFrames: 600_000}, {outFrames: 600_000}, {crossfadeSeconds: 6, sameAlbum: false, rate}), {start: 1_000_000 - 288_000, fade: 288_000});
  assert.deepEqual(edgeFrames({endFrame: 1_000_000, outFrames: 200_000}, {outFrames: 600_000}, {crossfadeSeconds: 6, sameAlbum: false, rate}), {start: 900_000, fade: 100_000}, 'half the shorter track');
  assert.deepEqual(edgeFrames({endFrame: 1_000_000, outFrames: 600_000}, {outFrames: 600_000}, {crossfadeSeconds: 6, sameAlbum: true, rate}), {start: 1_000_000, fade: 0});
  assert.deepEqual(edgeFrames({endFrame: 1_000_000, outFrames: 600_000}, {outFrames: 600_000}, {crossfadeSeconds: 0, sameAlbum: false, rate}), {start: 1_000_000, fade: 0});
});

test('(c) the crossfade is equal-power on both decks', async () => {
  const rate = 48000, f = fakeContext(rate);
  const a = new Deck(f.ctx, {} as AudioNode, sineSource(rate * 20, rate), {frames: rate * 20, gain: 1});
  const b = new Deck(f.ctx, {} as AudioNode, sineSource(rate * 20, rate), {frames: rate * 20, gain: 1});
  await a.pump(1); await b.pump(1);
  a.start(0);
  const edge = linkDecks(f.ctx, a, b, {crossfadeSeconds: 6, sameAlbum: false});
  assert.equal(edge.start, rate * 14);
  const curveIn = (b.fade.gain as any).events.find((e: unknown[]) => e[0] === 'curve'), curveOut = (a.fade.gain as any).events.find((e: unknown[]) => e[0] === 'curve');
  assert.deepEqual(curveIn.slice(1), [0, 1, 14, 6]);
  assert.ok(Math.abs(curveOut[1] - 1) < 1e-6 && Math.abs(curveOut[2]) < 1e-6);
  assert.deepEqual(curveOut.slice(3), [14, 6]);
});

test('(d) normalization ramps over 20 ms, and only when the value changes', () => {
  const f = fakeContext(48000);
  const deck = new Deck(f.ctx, {} as AudioNode, sineSource(48000, 48000), {frames: 48000, gain: 0.5});
  assert.equal(deck.norm.gain.value, 0.5);
  f.ctx.currentTime = 1;
  deck.setGain(0.25);
  deck.setGain(0.25);
  const ramps = (deck.norm.gain as any).events.filter((e: unknown[]) => e[0] === 'ramp');
  assert.deepEqual(ramps, [['ramp', 0.25, 1.02]]);
});

test('the in-tab byte cache empties on sign-out and on a profile or server switch', async () => {
  const {ByteCache} = await import('../src/bridge/audio/bytes.ts');
  const cache = new ByteCache(1 << 20);
  cache.put('m1:0', new Uint8Array(1000)); cache.put('m2:0', new Uint8Array(500));
  cache.clear();
  assert.equal(cache.size, 0);
  assert.equal(cache.get('m1:0'), undefined);
  const {readFileSync} = await import('node:fs');
  const session = readFileSync(new URL('../src/app/session.tsx', import.meta.url), 'utf8');
  assert.match(session, /const viewerKey = sessionIdentity\(scope\.session\);[\s\S]{0,200}audioByteCache\.clear\(\)/, 'keyed on who is signed in (server, account, profile, sign-in)');
});
