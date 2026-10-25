import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {fakeContext} from './helpers/fake-audio.ts';

/**
 * DecodeAudioRender against the playback service contract, in node: a fake AudioContext whose
 * clock the test moves, the §18.8 FLAC fixture served with Range, and a recording service.
 */
const dir = new URL('../../fixtures/audio-gapless/', import.meta.url);
const file = (name: string) => new Uint8Array(readFileSync(new URL(name, dir)));
const sidecar = (name: string) => JSON.parse(readFileSync(new URL(name, dir), 'utf8'));

function setup() {
  const f = fakeContext(44100);
  const requests: string[] = [];
  const files: Record<string, Uint8Array> = {'/a': file('album-flac-01.flac'), '/b': file('album-flac-02.flac')};
  (globalThis as any).AudioContext = class { constructor() { Object.assign(this, f.ctx); (this as any).suspend = async () => {}; (this as any).resume = async () => {}; (this as any).close = async () => {}; return f.ctx; } };
  f.ctx.suspend = async () => { f.ctx.state = 'suspended'; }; f.ctx.resume = async () => { f.ctx.state = 'running'; }; f.ctx.close = async () => {};
  f.ctx.addEventListener = () => {}; f.ctx.removeEventListener = () => {};
  (globalThis as any).document = {visibilityState: 'visible', addEventListener() {}, removeEventListener() {}};
  (globalThis as any).fetch = async (url: string, init: RequestInit) => {
    requests.push(url);
    const bytes = files[url]!, [a, b] = String((init.headers as Record<string, string>).Range).replace('bytes=', '').split('-').map(Number);
    const last = Math.min(bytes.length - 1, b!);
    return new Response(bytes.slice(a!, last + 1), {status: 206, headers: {'Content-Range': `bytes ${a}-${last}/${bytes.length}`}});
  };
  const plan = (id: string, s: any) => ({version: 2, mode: 'direct', id, url: '/' + id, container: 'flac', codec: 'flac', sampleRate: 44100, channels: 2, bytes: files['/' + id]!.length, prefetchBytes: 1 << 20, durationFrames: s.durationFrames, trim: s.trim});
  const a = plan('a', sidecar('album-flac-01.json')), b = plan('b', sidecar('album-flac-02.json'));
  const calls: unknown[][] = [];
  let snapshot: any = {session: {id: 's1', generation: 1, audio: a}, itemId: 'i1', phase: 'ready', intent: 'playing', intentId: 1, positionSeconds: 0, audioEffects: {rendering: true, rate: 1, settings: {gapless: true, crossfadeSeconds: 0, normalization: 'off'}, notice: ''}};
  const service: any = new Proxy({getSnapshot: () => snapshot}, {get: (t, k) => k in t ? (t as any)[k] : (...args: unknown[]) => { calls.push([k, ...args]); }});
  return {f, requests, a, b, calls, service, set: (patch: any) => { snapshot = {...snapshot, ...patch, audioEffects: {...snapshot.audioEffects, ...(patch.audioEffects ?? {})}}; return snapshot; }, get: () => snapshot};
}
const settle = async () => { for (let i = 0; i < 20; i++) await new Promise(r => setTimeout(r, 0)); };
/** The renderer keeps a 50 ms timer until it is disposed. Every test that makes one disposes it in
 * `after`, which also runs when the test fails or times out; the timeout means a read or decode that
 * never answers fails the test instead of keeping the process (and the whole suite) alive. */
const bounded = {timeout: 30_000};

test('legacy processing facts do not turn a saved leveling choice into an applied gain', bounded, async ctx => {
  const {DecodeAudioRender} = await import('../src/bridge/DecodeAudioRender.ts');
  const t = setup(), render = new DecodeAudioRender(t.service, path => path, {addEventListener() {}, removeEventListener() {}, volume: 1, muted: false} as any);
  ctx.after(() => render.dispose());
  render.apply(t.set({audioEffects: {settings: {...t.get().audioEffects.settings, normalization: 'track'}}})); await settle();
  (render as any).tick();
  assert.deepEqual(t.calls.filter(call => call[0] === 'audioProcessingApplied').at(-1), ['audioProcessingApplied', {intentId: 1, sessionId: 's1', generation: 1}, {eqApplied: false, levelingApplied: false}]);
  render.apply(t.set({session: {id: 's2', generation: 2, audio: {...t.a, gain: {trackDb: -6, trackPeak: 0.5}}}})); await settle();
  (render as any).tick();
  assert.deepEqual(t.calls.filter(call => call[0] === 'audioProcessingApplied').at(-1), ['audioProcessingApplied', {intentId: 1, sessionId: 's2', generation: 2}, {eqApplied: false, levelingApplied: true}]);
});

test('(e) pause, seek or a rate change drops a prepared edge: nothing is committed, the next track never plays', bounded, async ctx => {
  const {DecodeAudioRender} = await import('../src/bridge/DecodeAudioRender.ts');
  const t = setup();
  const render = new DecodeAudioRender(t.service, (p: string) => p, {addEventListener() {}, removeEventListener() {}, volume: 1, muted: false} as any);
  ctx.after(() => render.dispose());
  render.apply(t.get()); await settle();
  assert.ok(t.calls.some(c => c[0] === 'ready'), 'the first track started');
  render.apply(t.set({audioEffects: {prepared: {token: 'pn_1', expiresAtMs: Date.now() + 60000, audio: t.b}}})); await settle();
  assert.ok(t.calls.some(c => c[0] === 'audioPrepared' && c[1] === 'pn_1'));
  // The service clears the preparation on pause (and on seek or rate).
  render.apply(t.set({intent: 'paused', audioEffects: {prepared: undefined}})); await settle();
  t.f.advance(3);
  (render as any).tick();
  assert.equal(t.calls.filter(c => c[0] === 'audioCommit').length, 0, 'no commit after the edge was dropped');
  assert.equal((render as any).candidate, undefined);
});

test('(f) a commit that fails falls back: the track ends and says so, for :advance', bounded, async ctx => {
  const {DecodeAudioRender} = await import('../src/bridge/DecodeAudioRender.ts');
  const t = setup();
  const render = new DecodeAudioRender(t.service, (p: string) => p, {addEventListener() {}, removeEventListener() {}, volume: 1, muted: false} as any);
  ctx.after(() => render.dispose());
  render.apply(t.get()); await settle();
  render.apply(t.set({audioEffects: {prepared: {token: 'pn_1', expiresAtMs: Date.now() + 60000, audio: t.b}}})); await settle();
  // Near the end: the engine commits.
  for (let i = 0; i < 30 && !t.calls.some(c => c[0] === 'audioCommit'); i++) { t.f.advance(0.1); (render as any).tick(); await settle(); }
  assert.ok(t.calls.some(c => c[0] === 'audioCommit' && c[1] === 'pn_1'), 'committed within crossfade + 2 s of the edge');
  // The commit fails: the queue discards the preparation.
  render.apply(t.set({audioEffects: {prepared: undefined}})); await settle();
  for (let i = 0; i < 60 && !t.calls.some(c => c[0] === 'fact' && c[3] === 'ended'); i++) { t.f.advance(0.1); (render as any).tick(); await settle(); }
  assert.ok(t.calls.some(c => c[0] === 'fact' && c[3] === 'ended'), 'the track ends, so the queue advances with reason completion');
});

test('(b)(i) a committed edge promotes at the boundary; the engine keeps a constant number of decks however many edges pass', bounded, async ctx => {
  const {DecodeAudioRender} = await import('../src/bridge/DecodeAudioRender.ts');
  const t = setup();
  const render = new DecodeAudioRender(t.service, (p: string) => p, {addEventListener() {}, removeEventListener() {}, volume: 1, muted: false} as any);
  ctx.after(() => render.dispose());
  render.apply(t.get()); await settle();
  let current = t.a, next = t.b, n = 1;
  for (let edge = 0; edge < 6; edge++, n++) {
    const token = 'pn_' + edge;
    render.apply(t.set({audioEffects: {prepared: {token, expiresAtMs: Date.now() + 60000, audio: next}}})); await settle();
    for (let i = 0; i < 40 && !t.calls.some(c => c[0] === 'audioCommit' && c[1] === token); i++) { t.f.advance(0.1); (render as any).tick(); await settle(); }
    const committedSession = {id: 's' + (n + 1), generation: 1, audio: next};
    render.apply(t.set({audioEffects: {committed: {token, itemId: 'i' + (n + 1), session: committedSession}}})); await settle();
    const edgeFrame = (render as any).incoming.edge;
    assert.equal(edgeFrame, (render as any).primary.deck.endFrame, 'gapless: the next track starts at the frame after the last');
    for (let i = 0; i < 40 && !t.calls.some(c => c[0] === 'audioBoundary' && c[1] === token); i++) { t.f.advance(0.1); (render as any).tick(); await settle(); }
    assert.ok(t.calls.some(c => c[0] === 'audioBoundary' && c[1] === token));
    render.apply(t.set({session: committedSession, itemId: 'i' + (n + 1), audioEffects: {prepared: undefined, committed: undefined}})); await settle();
    const open = ['primary', 'candidate', 'incoming', 'retiring'].filter(k => (render as any)[k]).length;
    assert.ok(open <= 2, `edge ${edge}: ${open} decks open`);
    [current, next] = [next, current];
  }
  assert.equal(t.calls.filter(c => c[0] === 'ready').length, 1, 'no restart at any edge');
});

test('(h) an unavailable plan keeps the ordinary source and exposes availability without diagnostic text', bounded, async ctx => {
  const {PlaybackService, renderable} = await import('../../packages/client-core/src/index.ts');
  const {audioEffectsAvailability} = await import('../../packages/client-core/src/music-preferences.ts');
  const {DecodeAudioRender} = await import('../src/bridge/DecodeAudioRender.ts');
  const t = setup();
  const audio = {version: 2 as const, mode: 'unavailable' as const, id: 'm', reason: 'The owner doesn’t allow converting audio, and this device can’t decode ALAC.'};
  const session = {id: 'ordinary', generation: 1, streamUrl: '/ordinary', mode: 'direct' as const, duration: 120, resumeSeconds: 0, audio};
  const service = new PlaybackService({createPlayback: async () => session, stopPlayback: async () => {}, progressPlayback: async () => {}}, () => 'request-unavailable');
  const render = new DecodeAudioRender(service, path => path, {addEventListener() {}, removeEventListener() {}, volume: 1, muted: false} as any);
  ctx.after(async () => {render.dispose();await service.stop();});
  await service.play('item', 0);
  const snapshot = service.getSnapshot();
  assert.equal(renderable(snapshot.session), false);
  assert.equal(snapshot.audioEffects.rendering, false, 'the real service leaves decoding with the ordinary player');
  render.apply(snapshot);await settle();
  assert.equal(service.getSnapshot().session?.streamUrl, '/ordinary');
  assert.deepEqual(t.requests, [], 'the effects renderer never requests or decodes unavailable-plan bytes');
  assert.deepEqual(audioEffectsAvailability(snapshot.session), {available: false}, 'transport diagnostic reasons stay out of display state');
});

test('the position is what is heard: the output path’s latency is taken off the sample clock (A-17.26)', bounded, async ctx => {
  const {DecodeAudioRender} = await import('../src/bridge/DecodeAudioRender.ts');
  const t = setup();
  t.f.ctx.outputLatency = 0.15; t.f.ctx.baseLatency = 0.05;
  const render = new DecodeAudioRender(t.service, (p: string) => p, {addEventListener() {}, removeEventListener() {}, volume: 1, muted: false} as any);
  ctx.after(() => render.dispose());
  render.apply(t.get()); await settle();
  t.f.advance(2); (render as any).tick();
  const start = (render as any).primary.deck.startFrame as number;
  const facts = t.calls.filter(c => c[0] === 'fact' && c[3] === 'playing');
  const heard = facts.at(-1)![2] as number;
  assert.equal(heard, (Math.floor((t.f.ctx.currentTime - 0.2) * 44100) - start) / 44100, 'scheduled frames the speakers have not played yet are not counted');
});

test('an MP3 started part-way asks for the server’s seek index once and never for a conversion', bounded, async ctx => {
  const {DecodeAudioRender} = await import('../src/bridge/DecodeAudioRender.ts');
  const t = setup();
  const mp3 = file('album-mp3-01.mp3'), rate = 44100, total = rate * 120, from = rate * 40;
  const asked: string[] = [];
  (globalThis as any).fetch = async (url: string, init: RequestInit) => {
    asked.push(url);
    if (url === '/long.idx') return new Response(JSON.stringify({version: 1, offsets: [0, 4096], frames: [0, 1152]}), {headers: {'Content-Type': 'application/json'}});
    const [a, b] = String((init.headers as Record<string, string>).Range).replace('bytes=', '').split('-').map(Number), last = Math.min(mp3.length - 1, b!);
    return new Response(mp3.slice(a!, last + 1), {status: 206, headers: {'Content-Range': `bytes ${a}-${last}/${mp3.length}`}});
  };
  const plan = {version: 2, mode: 'direct', id: 'long', url: '/long.mp3', container: 'mp3', codec: 'mp3', sampleRate: rate, channels: 2, bytes: mp3.length, durationFrames: total, trim: {startFrames: 1105, endFrames: 0, source: 'lame'}, seekIndex: {url: '/long.idx'}};
  const render = new DecodeAudioRender(t.service, (p: string) => p, {addEventListener() {}, removeEventListener() {}, volume: 1, muted: false} as any);
  ctx.after(() => render.dispose());
  const feed = (render as any).reader(plan, false);
  // No decoder for MP3 in this runtime: the start is the ordinary player's, after the index was read.
  assert.equal(await feed.open(from), undefined);
  assert.equal(await feed.open(from + rate), undefined);
  feed.close();
  assert.equal(asked.filter(u => u === '/long.idx').length, 1, 'the index is read once per presentation');
  assert.ok(!asked.some(u => u.includes('audio-converted') || u.includes('fromFrame')), asked.join(' '));
  // A start at the beginning needs no index.
  asked.length = 0;
  const fresh = (render as any).reader(plan, false);
  await fresh.open(0);
  fresh.close();
  assert.ok(!asked.includes('/long.idx'));
});
