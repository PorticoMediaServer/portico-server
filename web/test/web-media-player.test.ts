import test from 'node:test';
import assert from 'node:assert/strict';
import {WebMediaPlayer, type HlsErrorData, type HlsModule, type VideoLike} from '../src/playback-v1/web-media-player.ts';
import {PlaybackSessionController, type PlayerEvent} from '@core/playback-v1/index.ts';
import {FakePlaybackServer, fakeHttp} from '@core/playback-v1/testing/fake-server.ts';

const settle = async () => { for (let i = 0; i < 10; i++) await new Promise(setImmediate); };

class FakeVideo implements VideoLike {
  src = ''; currentTime = 0; playbackRate = 1; volume = 1; muted = false; paused = true; ended = false;
  buffered = {length: 1, start: () => 0, end: () => this.currentTime + 12};
  tracks: {id: string; mode: string; kind: string; src: string; srclang: string; label: string; default: boolean; remove(): void}[] = [];
  get textTracks() { return this.tracks; }
  loads = 0;
  private handlers = new Map<string, Set<() => void>>();
  ownerDocument = {createElement: () => { const t = {id: '', mode: 'disabled', kind: '', src: '', srclang: '', label: '', default: false, remove: () => { this.tracks = this.tracks.filter(x => x !== t); }}; return t; }};
  play() { this.paused = false; this.fire('playing'); return Promise.resolve(); }
  pause() { if (this.paused) return; this.paused = true; this.fire('pause'); }
  load() { this.loads++; }
  removeAttribute(name: string) { if (name === 'src') this.src = ''; }
  canPlayType(type: string) { return type === 'application/vnd.apple.mpegurl' ? this.nativeHls : ''; }
  nativeHls = '';
  appendChild(node: unknown) { this.tracks.push(node as FakeVideo['tracks'][number]); return node; }
  querySelectorAll() { return [...this.tracks]; }
  addEventListener(type: string, fn: () => void) { let s = this.handlers.get(type); if (!s) this.handlers.set(type, s = new Set()); s.add(fn); }
  removeEventListener(type: string, fn: () => void) { this.handlers.get(type)?.delete(fn); }
  getVideoPlaybackQuality() { return {droppedVideoFrames: 3}; }
  fire(type: string) { for (const fn of [...(this.handlers.get(type) ?? [])]) fn(); }
  listenerCount() { let n = 0; for (const s of this.handlers.values()) n += s.size; return n; }
}

function fakeHls(supported = true) {
  const instances: {config: Record<string, unknown>; source?: string; destroyed: boolean; startLoads: number; recoveries: number; swaps: number; error?: (e: string, d: HlsErrorData) => void}[] = [];
  const Hls = class {
    static isSupported() { return supported; }
    static Events = {ERROR: 'hlsError'};
    static ErrorTypes = {NETWORK_ERROR: 'networkError', MEDIA_ERROR: 'mediaError'};
    bandwidthEstimate = 5_000_000;
    i: (typeof instances)[number];
    constructor(config: Record<string, unknown>) { this.i = {config, destroyed: false, startLoads: 0, recoveries: 0, swaps: 0}; instances.push(this.i); }
    attachMedia() {}
    loadSource(url: string) { this.i.source = url; }
    startLoad() { this.i.startLoads++; }
    recoverMediaError() { this.i.recoveries++; }
    swapAudioCodec() { this.i.swaps++; }
    destroy() { this.i.destroyed = true; }
    on(_e: string, fn: (e: string, d: HlsErrorData) => void) { this.i.error = fn; }
  } as unknown as HlsModule;
  return {Hls, instances};
}

const source = (over: Partial<Parameters<WebMediaPlayer['load']>[0]> = {}) => ({url: '/v1/media/grant/1/master.m3u8', generation: 1, mode: 'stream' as const, startPositionMs: 90_000, subtitles: [{trackId: 's4', format: 'webvtt', url: '/v1/media/grant/1/subs/s4.vtt'}], autoplay: true, ...over});
const resolveUrl = (u: string) => new URL(u, 'https://portico.test').href;

test('stream: hls.js gets an absolute URL, the start position and an origin guard; autoplay on canplay', async () => {
  const video = new FakeVideo();
  const {Hls, instances} = fakeHls();
  const player = new WebMediaPlayer(video, {loadHls: async () => Hls, resolveUrl, hlsConfig: {maxBufferLength: 30}});
  const events: PlayerEvent[] = [];
  player.onEvent(e => events.push(e));
  player.load(source());
  await settle();
  assert.equal(instances[0]!.source, 'https://portico.test/v1/media/grant/1/master.m3u8');
  assert.equal(instances[0]!.config.startPosition, 90);
  assert.equal(instances[0]!.config.maxBufferLength, 30);
  const xhr = {withCredentials: true};
  (instances[0]!.config.xhrSetup as (x: typeof xhr, u: string) => void)(xhr, 'https://portico.test/v1/media/grant/1/seg1.ts');
  assert.equal(xhr.withCredentials, false);
  assert.throws(() => (instances[0]!.config.xhrSetup as (x: typeof xhr, u: string) => void)(xhr, 'https://elsewhere.test/x.ts'));
  assert.equal(player.observe().state, 'loading');
  video.fire('canplay');
  assert.equal(player.observe().state, 'playing');
  const o = player.observe();
  assert.equal(o.generation, 1);
  assert.equal(o.bufferedMs, 12_000);
  assert.equal(o.bandwidthKbps, 5000);
  assert.equal(o.droppedFrames, 3);
  assert.equal(video.tracks[0]!.src, 'https://portico.test/v1/media/grant/1/subs/s4.vtt');
  player.dispose();
  assert.equal(video.listenerCount(), 0);
  assert.equal(instances[0]!.destroyed, true);
});

test('a new generation replaces the engine and the sidecars; events carry the new generation', async () => {
  const video = new FakeVideo();
  const {Hls, instances} = fakeHls();
  const player = new WebMediaPlayer(video, {loadHls: async () => Hls, resolveUrl});
  const gens: number[] = [];
  player.onEvent(e => gens.push(e.observation.generation));
  player.load(source());
  await settle();
  player.load(source({generation: 2, url: '/v1/media/grant/2/master.m3u8', subtitles: []}));
  await settle();
  assert.equal(instances[0]!.destroyed, true);
  assert.equal(instances.length, 2);
  assert.equal(video.tracks.length, 0, 'old sidecars removed');
  video.fire('timeupdate');
  assert.equal(gens.at(-1), 2);
  player.dispose();
});

test('direct play sets src and seeks to the start on loadedmetadata; seeks report seeked', async () => {
  const video = new FakeVideo();
  const player = new WebMediaPlayer(video, {resolveUrl});
  const types: string[] = [];
  player.onEvent(e => types.push(e.type));
  player.load(source({mode: 'direct', url: '/v1/media/grant/1/file.mp4', autoplay: false}));
  assert.equal(video.src, 'https://portico.test/v1/media/grant/1/file.mp4');
  video.fire('loadedmetadata');
  assert.equal(video.currentTime, 90);
  video.fire('canplay');
  assert.equal(player.observe().state, 'paused', 'not autoplay: stays paused at the start');
  video.fire('seeked');
  assert.ok(!types.includes('seeked'), 'a seek nobody asked for is not reported');
  player.seek(120_000);
  video.fire('seeked');
  assert.equal(types.filter(t => t === 'seeked').length, 1);
  assert.equal(player.observe().positionMs, 120_000);
  player.dispose();
});

test('sidecar subtitles switch locally and show in the observation', async () => {
  const video = new FakeVideo();
  const player = new WebMediaPlayer(video, {resolveUrl});
  player.load(source({mode: 'direct'}));
  player.selectSidecarSubtitle('s4');
  assert.equal(video.tracks[0]!.mode, 'showing');
  assert.equal(player.observe().subtitleTrackId, 's4');
  player.selectSidecarSubtitle(null);
  assert.equal(video.tracks[0]!.mode, 'disabled');
  assert.equal(player.observe().subtitleTrackId, null);
  player.dispose();
});

test('errors: refused streams fail at once, network retries twice, media recovers twice then fails', async () => {
  for (const [data, expect] of [
    [{fatal: true, type: 'networkError', response: {code: 410}}, {code: 'stream_gone', startLoads: 0}],
    [{fatal: true, type: 'networkError', details: 'fragLoadTimeOut'}, {code: 'network_error', startLoads: 2}],
    [{fatal: true, type: 'mediaError', details: 'bufferStalledError'}, {code: 'decode_error', recoveries: 2, swaps: 1}],
  ] as const) {
    const video = new FakeVideo();
    const {Hls, instances} = fakeHls();
    const player = new WebMediaPlayer(video, {loadHls: async () => Hls, resolveUrl});
    const errors: string[] = [];
    player.onEvent(e => { if (e.type === 'error') errors.push(e.code); });
    player.load(source());
    await settle();
    const i = instances[0]!;
    i.error!('hlsError', {fatal: false, type: 'networkError'}); // non-fatal: ignored
    for (let n = 0; n < 3; n++) i.error!('hlsError', data as HlsErrorData);
    assert.deepEqual(errors, [expect.code]);
    if ('startLoads' in expect) assert.equal(i.startLoads, expect.startLoads);
    if ('recoveries' in expect) { assert.equal(i.recoveries, expect.recoveries); assert.equal(i.swaps, expect.swaps); }
    assert.equal(player.observe().state, 'error');
    player.dispose();
  }
});

test('no hls.js support: native HLS where the browser has it, else unsupported', async () => {
  const {Hls} = fakeHls(false);
  const safari = new FakeVideo(); safari.nativeHls = 'maybe';
  const a = new WebMediaPlayer(safari, {loadHls: async () => Hls, resolveUrl});
  a.load(source());
  await settle();
  assert.equal(safari.src, 'https://portico.test/v1/media/grant/1/master.m3u8');
  const other = new FakeVideo();
  const b = new WebMediaPlayer(other, {loadHls: async () => Hls, resolveUrl});
  const errors: string[] = [];
  b.onEvent(e => { if (e.type === 'error') errors.push(e.code); });
  b.load(source());
  await settle();
  assert.deepEqual(errors, ['unsupported']);
  a.dispose(); b.dispose();
});

test('driven by PlaybackSessionController against the fake server: load, report, track change', async () => {
  const server = new FakePlaybackServer();
  const video = new FakeVideo();
  const {Hls, instances} = fakeHls();
  const player = new WebMediaPlayer(video, {loadHls: async () => Hls, resolveUrl});
  let n = 0;
  const ctl = new PlaybackSessionController({http: fakeHttp(server), player, key: () => 'key_web_player_' + String(++n).padStart(4, '0')});
  const s = (await ctl.start({itemId: 'movie-1'}))!;
  await settle();
  assert.equal(instances.length, 1);
  video.fire('canplay');
  await settle();
  assert.ok(server.session(s.id)!.reports.some(r => r.state === 'playing'), 'the first playing state is reported');
  await ctl.setAudio('a2');
  await settle();
  assert.equal(instances.length, 2, 'a new generation loads a new engine');
  assert.equal(instances[0]!.destroyed, true);
  await ctl.stop();
  assert.equal(server.session(s.id)!.ended, true);
  ctl.dispose();
  player.dispose();
});
