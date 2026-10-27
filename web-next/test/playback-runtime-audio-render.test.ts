import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
/** A fake BaseAudioContext for node: records what the engine schedules and plays it back as
 * `advance()` moves the clock (onended fires for buffers that finished). */
export function fakeContext(sampleRate = 48000) {
  const scheduled: {start: number; frames: number; node: any}[] = [];
  const param = (value = 1) => ({value, events: [] as unknown[], setValueAtTime(v: number, t: number) { this.events.push(['set', v, t]); }, linearRampToValueAtTime(v: number, t: number) { this.events.push(['ramp', v, t]); this.value = v; }, cancelScheduledValues() {}, setValueCurveAtTime(c: Float32Array, t: number, d: number) { this.events.push(['curve', c[0], c[c.length - 1], t, d]); }, setTargetAtTime(v: number) { this.value = v; }});
  const listeners = new Map<string, Set<(event: Event) => void>>();
  const ctx: any = {
    sampleRate, currentTime: 0, state: 'running', destination: {channelCount: 2},
    addEventListener(type: string, callback: (event: Event) => void) { if (!listeners.has(type)) listeners.set(type, new Set()); listeners.get(type)!.add(callback); },
    removeEventListener(type: string, callback: (event: Event) => void) { listeners.get(type)?.delete(callback); },
    dispatchEvent(event: Event) { for (const callback of listeners.get(event.type) ?? []) callback(event); return true; },
    createGain: () => ({gain: param(), connect() {}, disconnect() {}}),
    createBuffer: (channels: number, length: number, rate: number) => ({numberOfChannels: channels, length, sampleRate: rate, data: Array.from({length: channels}, () => new Float32Array(length)), copyToChannel(src: Float32Array, ch: number) { this.data[ch].set(src); }}),
    createBufferSource: () => {
      const node: any = {buffer: null, onended: null, started: undefined, connect() {}, disconnect() {}, start(t: number, offset = 0, duration?: number) { node.started = t; node.offset = Math.round(offset * sampleRate); scheduled.push({start: Math.round(Math.max(ctx.currentTime, t) * sampleRate), frames: duration === undefined ? node.buffer.length : Math.round(duration * sampleRate), node}); }, stop() { node.stopped = true; }};
      return node;
    },
  };
  return {
    ctx, scheduled,
    advance(seconds: number) {
      ctx.currentTime += seconds;
      const frame = Math.round(ctx.currentTime * sampleRate);
      for (const s of scheduled) if (!s.node.done && !s.node.stopped && s.start + s.frames <= frame) { s.node.done = true; s.node.onended?.(); }
    },
    /** Frames the scheduled buffers hold that haven't played yet. */
    unplayed() { const frame = Math.round(ctx.currentTime * sampleRate); return scheduled.filter(s => !s.node.done && !s.node.stopped).reduce((n, s) => n + Math.min(s.frames, s.start + s.frames - Math.max(frame, s.start)), 0); },
  };
}

/** A synthetic PCM source: `frames` of a sine, generated as read (no memory of its own). */
export function sineSource(frames: number, sampleRate: number, channels = 2) {
  let at = 0, closed = false;
  return {
    sampleRate, channels, get closed() { return closed; },
    async read(max: number) {
      if (at >= frames) return undefined;
      const n = Math.min(max, frames - at), c = Array.from({length: channels}, () => new Float32Array(n));
      for (let i = 0; i < n; i++) { const v = 0.5 * Math.sin(2 * Math.PI * 441 * (at + i) / sampleRate); for (const ch of c) ch[i] = v; }
      at += n;
      return {channels: c, frames: n};
    },
    close() { closed = true; },
  };
}


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
    const asked = new Headers(init.headers).get('Range');
    if (!asked) return new Response(new Uint8Array(files[url]!), {status: 200, headers: {'Content-Type': 'audio/flac', 'X-Audio-Trim-Start': '0', 'X-Audio-Trim-End': '0'}});
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

test('current-song leveling facts follow the actual deck gain rather than the saved switch', bounded, async ctx => {
  const {DecodeAudioRender} = await import('../src/platform/playback/DecodeAudioRender.ts');
  const t = setup(), render = new DecodeAudioRender(t.service, (path: string) => path, {addEventListener() {}, removeEventListener() {}, volume: 1, muted: false} as any);
  ctx.after(() => render.dispose());
  const settings = {...t.get().audioEffects.settings, normalization: 'track'};
  render.apply(t.set({audioEffects: {settings}})); await settle();
  assert.equal(render.audioFacts()!.levelingApplied, false, 'unknown gain is unchanged audio even with leveling selected');
  render.apply(t.set({session: {id: 's2', generation: 2, audio: {...t.a, gain: {trackDb: 0, trackPeak: 0.5}}}}));
  assert.equal(render.audioFacts(), undefined, 'previous-song facts cannot describe a new deck still decoding');
  await settle(); assert.equal(render.audioFacts()!.levelingApplied, false, 'a measured zero dB gain is unchanged');
  render.apply(t.set({session: {id: 's3', generation: 3, audio: {...t.a, gain: {trackDb: -6, trackPeak: 0.5}}}})); await settle();
  assert.equal(render.audioFacts()!.levelingApplied, true);
  assert.ok(Math.abs(render.audioFacts()!.levelingGainDb! + 6) < .001);
  assert.deepEqual(t.calls.filter(call => call[0] === 'audioProcessingApplied').at(-1), ['audioProcessingApplied', {intentId: 1, sessionId: 's3', generation: 3}, {eqApplied: false, levelingApplied: true}]);
  render.apply(t.set({audioEffects: {settings: {...settings, normalization: 'off'}}}));
  assert.equal(render.audioFacts()!.levelingApplied, false);
  render.apply(t.set({phase: 'ended'}));
  assert.equal(render.audioFacts(), undefined);
  assert.deepEqual(t.calls.filter(call => call[0] === 'audioProcessingApplied').at(-1)!.at(-1), {eqApplied: false, levelingApplied: false});
});

test('Never boost and already baked normalization report only the actual applied node gain', bounded, async ctx => {
  const {DecodeAudioRender} = await import('../src/platform/playback/DecodeAudioRender.ts');
  const t = setup(), render = new DecodeAudioRender(t.service, (path: string) => path, {addEventListener() {}, removeEventListener() {}, volume: 1, muted: false} as any);
  ctx.after(() => render.dispose());
  const settings = {...t.get().audioEffects.settings, normalization: 'track', neverBoost: true};
  render.apply(t.set({audioEffects: {settings}, session: {id: 'positive', generation: 2, audio: {...t.a, gain: {trackDb: 6, trackPeak: .25}}}})); await settle();
  assert.equal(render.audioFacts()!.levelingApplied, false);
  assert.equal(render.audioFacts()!.levelingGainDb, 0);
  render.apply(t.set({session: {id: 'baked', generation: 3, audio: {...t.a, mode: 'converted', loudnessAppliedDb: -8}}})); await settle();
  assert.equal(render.audioFacts()!.levelingApplied, false, 'server baked gain is never applied again by the client');
  assert.equal(render.audioFacts()!.levelingGainDb, 0);
});

test('autoplay refusal is fenced to the current renderer intent and a replacement gets its own resume attempt', bounded, async ctx => {
  const {DecodeAudioRender} = await import('../src/platform/playback/DecodeAudioRender.ts');
  const t = setup(), pending: ((error: unknown) => void)[] = [];
  t.f.ctx.state = 'suspended'; t.f.ctx.resume = () => new Promise<void>((_, reject) => pending.push(reject));
  const render = new DecodeAudioRender(t.service, (path: string) => path, {addEventListener() {}, removeEventListener() {}, volume: 1, muted: false} as any);
  ctx.after(() => render.dispose());
  render.apply(t.get()); await settle(); assert.equal(pending.length, 1);
  render.apply(t.set({intentId: 2, session: {id: 'new-song', generation: 2, audio: t.b}})); await settle();
  assert.equal(pending.length, 2, 'a replaced blocked song cannot suppress the new song resume attempt');
  pending[0]!(new DOMException('controlled old refusal', 'NotAllowedError')); await settle();
  assert.equal(t.calls.some(call => call[0] === 'pause' || call[0] === 'listeningNotice'), false);
  pending[1]!(new DOMException('controlled current refusal', 'NotAllowedError')); await settle();
  assert.equal(t.calls.filter(call => call[0] === 'pause').length, 1);
  assert.deepEqual(t.calls.filter(call => call[0] === 'listeningNotice'), [['listeningNotice', 'audio_autoplay_blocked']]);
});

test('(e) pause, seek or a rate change drops a prepared edge: nothing is committed, the next track never plays', bounded, async ctx => {
  const {DecodeAudioRender} = await import('../src/platform/playback/DecodeAudioRender.ts');
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
  const {DecodeAudioRender} = await import('../src/platform/playback/DecodeAudioRender.ts');
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
  const {DecodeAudioRender} = await import('../src/platform/playback/DecodeAudioRender.ts');
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

test('an unavailable effects plan retains the ordinary playback path', async () => {
  const {renderable} = await import('../../packages/client-core/src/index.ts');
  const s = {audio: {version: 2 as const, mode: 'unavailable' as const, id: 'm', reason: 'The owner doesn’t allow converting audio, and this device can’t decode ALAC.'}};
  assert.equal(renderable(s), false, 'no effects engine: the ordinary player plays presentation.url');
});

test('the position is what is heard: the output path’s latency is taken off the sample clock (A-17.26)', bounded, async ctx => {
  const {DecodeAudioRender} = await import('../src/platform/playback/DecodeAudioRender.ts');
  const t = setup();
  t.f.ctx.outputLatency = 0.15; t.f.ctx.baseLatency = 0.05;
  const render = new DecodeAudioRender(t.service, (p: string) => p, {addEventListener() {}, removeEventListener() {}, volume: 1, muted: false} as any);
  ctx.after(() => render.dispose());
  render.apply(t.get()); await settle();
  t.f.advance(2); (render as any).tick();
  const start = (render as any).primary.deck.startFrame as number;
  const facts = t.calls.filter(c => c[0] === 'fact' && c[3] === 'playing');
  const heard = facts.at(-1)![2] as number;
  assert.equal(heard, (Math.floor((t.f.ctx.currentTime - 0.2) * 44100) - Math.round(0.08 * 44100) - start) / 44100, 'scheduled frames the speakers have not played yet are not counted');
});

test('an MP3 started part-way asks for the server’s seek index once and never for a conversion', bounded, async ctx => {
  const {DecodeAudioRender} = await import('../src/platform/playback/DecodeAudioRender.ts');
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

/** Controlled PCM at the existing reader port; real Deck, graph and renderer remain in use. */
function stalledPcm(rate = 44100, frames = rate * 8, initial = rate * 2) {
  let at = 0, available = initial, closed = false, reads = 0;
  let wake: (() => void) | undefined;
  const source = {sampleRate: rate, channels: 2,
    async read(max: number) {
      reads++;
      while (!closed && at >= available && at < frames) await new Promise<void>(resolve => { wake = resolve; });
      if (closed || at >= frames) return;
      const n = Math.min(max, available - at, frames - at), from = at;
      at += n;
      return {frames: n, channels: Array.from({length: 2}, () => Float32Array.from({length: n}, (_, i) => (from + i) / frames))};
    },
    close() {closed = true; wake?.();},
  };
  return {source, get reads() {return reads;}, get closed() {return closed;}, release(until = frames) {available = until; const old = wake; wake = undefined; old?.();}};
}
function usePcm(render: unknown, source: ReturnType<typeof stalledPcm>['source']) {
  (render as any).reader = () => ({unlock() {}, close: () => source.close(), prefetchAll: async () => {}, open: async () => source});
}

test('renderer stalls four seconds past its frontier, keeps one pump, and resumes each unheard frame in order', bounded, async ctx => {
  const {DecodeAudioRender} = await import('../src/platform/playback/DecodeAudioRender.ts');
  const t = setup(), pcm = stalledPcm(), render = new DecodeAudioRender(t.service, p => p, {addEventListener() {}, removeEventListener() {}, volume: 1, muted: false} as any);
  ctx.after(() => render.dispose()); usePcm(render, pcm.source);
  render.apply(t.set({session: {id: 'slow', generation: 1, audio: {...t.a, durationFrames: 44100 * 8}}})); await settle();
  const primary = (render as any).primary, graph = (render as any).graph;
  assert.equal(graph.clock.frontier, 44100 * 2);
  let suspend!: () => void;
  t.f.ctx.suspend = () => new Promise<void>(resolve => {suspend = () => {t.f.ctx.state = 'suspended'; resolve();};});
  t.f.advance(6.08); (render as any).tick();
  assert.equal(render.transportPosition(), 2, 'silent elapsed context time is never source progress');
  const pump = primary.deck.pump(30), reads = pcm.reads;
  for (let i = 0; i < 3000; i++) assert.strictEqual(primary.deck.pump(30), pump);
  assert.equal(pcm.reads, reads);
  const oldStarts = t.f.scheduled.length;
  pcm.release(); await settle();
  assert.equal(t.f.scheduled.length, oldStarts, 'late PCM cannot start before actual suspension');
  assert.equal(t.calls.some(c => c[0] === 'fact' && c[3] === 'ended'), false);
  suspend(); await settle(); (render as any).tick();
  assert.equal((render as any).hold, undefined);
  const rebuilt = t.f.scheduled.slice(oldStarts);
  const anchor = Math.floor(6.08 * 44100) + Math.round(0.08 * 44100);
  assert(rebuilt.every(s => s.start >= anchor));
  assert.deepEqual(rebuilt.map(s => s.start), Array.from({length: 6}, (_, i) => anchor + i * 44100));
  assert.equal(rebuilt.reduce((n, s) => n + s.frames, 0), 44100 * 6);
  assert.equal(render.transportPosition(), 2);
});

test('pause during a pending buffering suspension cannot be undone by refill or transport; explicit play recovers', bounded, async ctx => {
  const {DecodeAudioRender} = await import('../src/platform/playback/DecodeAudioRender.ts');
  const t = setup(), pcm = stalledPcm(), render = new DecodeAudioRender(t.service, p => p, {addEventListener() {}, removeEventListener() {}, volume: 1, muted: false} as any);
  ctx.after(() => render.dispose()); usePcm(render, pcm.source);
  render.apply(t.set({session: {id: 'slow', generation: 1, audio: {...t.a, durationFrames: 44100 * 8}}})); await settle();
  let suspend!: () => void, resumes = 0;
  t.f.ctx.suspend = () => new Promise<void>(resolve => {suspend = () => {t.f.ctx.state = 'suspended'; resolve();};});
  t.f.ctx.resume = async () => {resumes++; t.f.ctx.state = 'running';};
  t.f.advance(2.08); (render as any).tick();
  render.apply(t.set({intent: 'paused'})); pcm.release(); await settle(); suspend(); await settle();
  assert.equal(resumes, 0); assert((render as any).hold);
  render.apply(t.set({intent: 'playing'})); await settle();
  assert.equal(resumes, 1); assert.equal((render as any).hold, undefined);
});

test('seek, source replacement and disposal retire a pending buffering ticket before it can rebuild or resume', bounded, async ctx => {
  const {DecodeAudioRender} = await import('../src/platform/playback/DecodeAudioRender.ts');
  for (const retire of ['seek', 'replace', 'dispose']) {
    const t = setup(), pcm = stalledPcm(), render = new DecodeAudioRender(t.service, p => p, {addEventListener() {}, removeEventListener() {}, volume: 1, muted: false} as any);
    ctx.after(() => render.dispose()); usePcm(render, pcm.source);
    render.apply(t.set({session: {id: 'slow', generation: 1, audio: {...t.a, durationFrames: 44100 * 8}}})); await settle();
    let suspend!: () => void, resumes = 0;
    t.f.ctx.suspend = () => new Promise<void>(resolve => {suspend = () => {t.f.ctx.state = 'suspended'; resolve();};});
    t.f.ctx.resume = async () => {resumes++; t.f.ctx.state = 'running';};
    t.f.advance(2.08); (render as any).tick(); const original = (render as any).hold;
    if (retire === 'dispose') render.dispose();
    else if (retire === 'seek') render.apply(t.set({intent: 'paused', pendingSeek: {revision: 2, positionSeconds: 4}}));
    else render.apply(t.set({intent: 'paused', session: {id: 'replacement', generation: 2, audio: t.b}}));
    suspend(); await settle();
    assert.notEqual((render as any).hold, original);
    assert.throws(() => original.graph.preview(44100), /retired/);
    assert.equal(pcm.closed, true);
    assert.equal(resumes, 0, 'old held operation never resumes a replacement');
    render.dispose();
  }
});

test('a recovery resume resolving after pause stays suspended and never publishes completion', bounded, async ctx => {
  const {DecodeAudioRender} = await import('../src/platform/playback/DecodeAudioRender.ts');
  const t = setup(), pcm = stalledPcm(), render = new DecodeAudioRender(t.service, p => p, {addEventListener() {}, removeEventListener() {}, volume: 1, muted: false} as any);
  ctx.after(() => render.dispose()); usePcm(render, pcm.source);
  render.apply(t.set({session: {id: 'slow', generation: 1, audio: {...t.a, durationFrames: 44100 * 8}}})); await settle();
  let resumed!: () => void, attempts = 0;
  t.f.ctx.resume = () => {attempts++; return new Promise<void>(resolve => {resumed = () => {t.f.ctx.state = 'running'; resolve();};});};
  t.f.advance(2.08); (render as any).tick(); await settle(); pcm.release(); await settle(); (render as any).tick();
  assert.equal(attempts, 1);
  render.apply(t.set({intent: 'paused'})); await settle(); resumed(); await settle();
  // The old attempt does not acquire the current paused intent. A browser statechange supplies
  // the same fresh pause guard in production; exercise it explicitly with this controlled clock.
  (render as any).contextChanged(); await settle();
  assert.equal(t.f.ctx.state, 'suspended'); assert((render as any).hold);
  assert.equal(t.calls.some(c => c[0] === 'fact' && c[3] === 'ended'), false);
});

test('a full twelve-second fade uses one frontier and publishes a crossed boundary once after recovery', bounded, async ctx => {
  const {DecodeAudioRender} = await import('../src/platform/playback/DecodeAudioRender.ts');
  const rate = 44100, t = setup(), outgoing = stalledPcm(rate, rate * 40, rate * 30), incoming = stalledPcm(rate, rate * 40, rate * 12);
  const render = new DecodeAudioRender(t.service, p => p, {addEventListener() {}, removeEventListener() {}, volume: 1, muted: false} as any);
  ctx.after(() => render.dispose());
  let opens = 0;
  (render as any).reader = () => {const pcm = opens++ === 0 ? outgoing : incoming; return {unlock() {}, close: () => pcm.source.close(), prefetchAll: async () => {}, open: async () => pcm.source};};
  const settings = {...t.get().audioEffects.settings, gapless: false, crossfadeSeconds: 12};
  const a = {...t.a, durationFrames: rate * 40}, b = {...t.b, durationFrames: rate * 40};
  render.apply(t.set({session: {id: 'outgoing', generation: 1, audio: a}, audioEffects: {settings}})); await settle();
  render.apply(t.set({audioEffects: {prepared: {token: 'fade', itemId: 'second', audio: b}}})); await settle();
  assert.equal((render as any).candidate.deck.decoded, rate * 12, 'readiness includes the full accepted overlap');
  render.apply(t.set({audioEffects: {committed: {token: 'fade', itemId: 'second', session: {id: 'incoming', generation: 1, audio: b}}}}));
  assert.equal((render as any).incoming.edge, rate * 28);
  assert.equal((render as any).incoming.fade, rate * 12);
  let suspend!: () => void;
  t.f.ctx.suspend = () => new Promise<void>(resolve => {suspend = () => {t.f.ctx.state = 'suspended'; resolve();};});
  t.f.advance(31.08); (render as any).tick();
  assert((render as any).hold);
  assert.equal(t.calls.filter(c => c[0] === 'audioBoundary').length, 0, 'a late tick holds the crossed boundary until recovery');
  assert(t.f.scheduled.every(s => s.start + s.frames <= Math.round(30.08 * rate)), 'neither linked voice renders beyond available outgoing PCM');
  outgoing.release(); incoming.release(); await settle(); suspend(); await settle();
  assert.equal(t.calls.filter(c => c[0] === 'audioBoundary' && c[1] === 'fade').length, 1);
  assert.equal((render as any).primary.plan.id, b.id);
  assert((render as any).retiring, 'outgoing fade suffix is retained, not stopped at the boundary');
});

test('a fresh gesture retries an autoplay-pending resume without replaying old refusal', bounded, async ctx => {
  const {DecodeAudioRender} = await import('../src/platform/playback/DecodeAudioRender.ts');
  const t = setup(), pending: ((error: unknown) => void)[] = [];
  t.f.ctx.state = 'suspended'; t.f.ctx.resume = () => new Promise<void>((_, reject) => pending.push(reject));
  const render = new DecodeAudioRender(t.service, p => p, {addEventListener() {}, removeEventListener() {}, volume: 1, muted: false} as any);
  ctx.after(() => render.dispose()); render.apply(t.get()); await settle();
  assert.equal(pending.length, 1);
  (render as any).gesture(); await settle(); assert.equal(pending.length, 2);
  pending[0]!(new DOMException('old blocked attempt', 'NotAllowedError')); await settle();
  assert.equal(t.calls.some(c => c[0] === 'pause'), false);
  pending[1]!(new DOMException('current refusal', 'NotAllowedError')); await settle();
  assert.equal(t.calls.filter(c => c[0] === 'pause').length, 1);
});
