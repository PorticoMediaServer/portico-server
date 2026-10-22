import test, {type TestContext} from 'node:test';
import assert from 'node:assert/strict';
import {HttpLocalApi} from '@core/index.ts';
import {FakePlaybackServer, fakeHttp} from '@core/playback-v1/testing/fake-server.ts';
import type {V1Request, V1Response} from '@core/playback-v1/http.ts';
import type {ServerEvent} from '@core/playback-v1/types.ts';
import {WebShellPlaybackRuntime} from '../src/platform/playback/runtime.ts';
import {ByteCache} from '../src/platform/playback/audio/bytes.ts';
import type {WebPlaybackSettings, WebPlaybackSource} from '../src/platform/playback/types.ts';
import type {SelectionContext} from '@core/selection-model/types.ts';
import type {HomeSelectionPlaybackItem} from '@core/home-selection/types.ts';
import type {HomeChannel} from '@core/home.ts';

/**
 * A media element and an hls.js that do only what a test tells them to, for the web media
 * player port and the playback path above it. Time is the test's own clock.
 */
import type {HlsErrorData, HlsFragmentLike, HlsLike, HlsModule, VideoLike} from '../src/platform/playback/web-media-player.ts';

export class FakeVideo implements VideoLike {
  src = ''; playbackRate = 1; volume = 1; muted = false;
  paused = true; ended = false; seeking = false; readyState = 0; duration = NaN;
  error: {code: number; message?: string} | null = null;
  bufferedRanges: [number, number][] = [];
  seekableRanges: [number, number][] = [];
  loads = 0;
  plays = 0;
  /** `play()` is refused, as a browser does without a gesture. */
  refusePlay = false;
  /** Every position the element was asked to move to. */
  seeks: number[] = [];
  private time = 0;
  private handlers = new Map<string, Set<() => void>>();

  get currentTime() { return this.time; }
  set currentTime(value: number) { this.time = value; this.seeking = true; this.seeks.push(value); }
  get buffered() { const r = this.bufferedRanges; return {length: r.length, start: (i: number) => r[i]![0], end: (i: number) => r[i]![1]}; }
  get seekable() { const r = this.seekableRanges; return {length: r.length, start: (i: number) => r[i]![0], end: (i: number) => r[i]![1]}; }

  play() {
    this.plays++;
    if (this.refusePlay) return Promise.reject(Object.assign(new Error('refused'), {name: 'NotAllowedError'}));
    if (this.paused) { this.paused = false; this.fire('play'); if (this.readyState >= 3) this.fire('playing'); }
    return Promise.resolve();
  }
  pause() { if (this.paused) return; this.paused = true; this.fire('pause'); }
  load() { this.loads++; this.readyState = 0; this.time = 0; this.ended = false; this.seeking = false; this.paused = true; this.error = null; this.bufferedRanges = []; this.seekableRanges = []; }
  removeAttribute(name: string) { if (name === 'src') this.src = ''; }
  canPlayType(type: string) { return type === 'application/vnd.apple.mpegurl' ? this.nativeHls : ''; }
  nativeHls = '';
  addEventListener(type: string, fn: () => void) { let s = this.handlers.get(type); if (!s) this.handlers.set(type, s = new Set()); s.add(fn); }
  removeEventListener(type: string, fn: () => void) { this.handlers.get(type)?.delete(fn); }
  getVideoPlaybackQuality() { return {droppedVideoFrames: 3}; }
  fire(type: string) { for (const fn of [...(this.handlers.get(type) ?? [])]) fn(); }
  listenerCount() { let n = 0; for (const s of this.handlers.values()) n += s.size; return n; }

  // ── What a browser would do ──────────────────────────────────────────
  /** Media is there around `at` (seconds): metadata, a buffered range, a playable frame. */
  ready(at = this.time, ahead = 12, duration = 7200) {
    this.duration = duration;
    this.seekableRanges = [[0, duration]];
    this.bufferedRanges = [[Math.max(0, at - 1), Math.min(duration, at + ahead)]];
    this.time = at;
    this.seeking = false;
    this.readyState = 4;
    this.fire('loadedmetadata');
    this.fire('canplay');
  }
  /** The seek in hand finishes where it was asked to go (or at `at`). */
  land(at = this.time, ahead = 12) {
    this.time = at;
    this.seeking = false;
    this.readyState = 4;
    this.bufferedRanges = [[Math.max(0, at - 1), at + ahead]];
    this.fire('seeked');
  }
  /** Playback moves on to `at`. */
  advance(at: number, ahead = 12) {
    this.time = at;
    this.bufferedRanges = [[Math.max(0, at - 1), at + ahead]];
    this.fire('timeupdate');
  }
  /** The buffer runs dry. */
  starve() { this.bufferedRanges = []; this.readyState = 2; this.fire('waiting'); }
  /** The title plays out: browsers publish a final pause, then `ended`. */
  finish() { this.time = this.duration; this.paused = true; this.fire('pause'); this.ended = true; this.fire('ended'); }
  fail(code: number, message = '') { this.error = {code, message}; this.fire('error'); }
}

export type FakeHlsInstance = {
  config: Record<string, unknown>; source?: string; destroyed: boolean; startLoads: number[]; recoveries: number; swaps: number;
  handlers: Map<string, (event: string, data: never) => void>; engine: HlsLike & {loadLevel?: number; levels: {bitrate?: number; details?: {fragments: never[]}}[]; playingDate: Date | null};
  emit(event: string, data?: unknown): void;
  error(data: HlsErrorData): void;
  /** A fragment starts downloading; returns it so the test can add bytes. */
  fragment(start?: number): Required<HlsFragmentLike>;
  loaded(frag: HlsFragmentLike, ms?: number): void;
};

export function fakeHls(supported = true) {
  const instances: FakeHlsInstance[] = [];
  const Events = {ERROR: 'hlsError', FRAG_LOADING: 'hlsFragLoading', FRAG_LOADED: 'hlsFragLoaded', FRAG_CHANGED: 'hlsFragChanged', MANIFEST_PARSED: 'hlsManifestParsed', SUBTITLE_TRACKS_UPDATED: 'hlsSubtitleTracksUpdated'};
  const Hls = class {
    static isSupported() { return supported; }
    static Events = Events;
    static ErrorTypes = {NETWORK_ERROR: 'networkError', MEDIA_ERROR: 'mediaError'};
    bandwidthEstimate = 5_000_000;
    playingDate: Date | null = null;
    levels: {bitrate?: number; details?: {fragments: never[]}}[] = [];
    currentLevel = 0;
    loadLevel = 0;
    subtitleDisplay = true;
    subtitleTrack = 0;
    i: FakeHlsInstance;
    constructor(config: Record<string, unknown>) {
      const handlers = new Map<string, (event: string, data: never) => void>();
      this.i = {
        config, destroyed: false, startLoads: [], recoveries: 0, swaps: 0, handlers, engine: this as never,
        emit: (event, data) => handlers.get(event)?.(event, data as never),
        error: data => handlers.get(Events.ERROR)?.(Events.ERROR, data as never),
        fragment: (start = 0) => { const frag = {start, startPTS: start, stats: {loaded: 0, loading: {first: 0, end: 0}}}; handlers.get(Events.FRAG_LOADING)?.(Events.FRAG_LOADING, {frag} as never); return frag; },
        loaded: (frag, ms = 1000) => { if (frag.stats?.loading) { frag.stats.loading.first = 0; frag.stats.loading.end = ms; } handlers.get(Events.FRAG_LOADED)?.(Events.FRAG_LOADED, {frag} as never); },
      };
      instances.push(this.i);
    }
    attachMedia() {}
    loadSource(url: string) { this.i.source = url; }
    startLoad(position = -1) { this.i.startLoads.push(position); }
    recoverMediaError() { this.i.recoveries++; }
    swapAudioCodec() { this.i.swaps++; }
    destroy() { this.i.destroyed = true; }
    on(event: string, fn: (event: string, data: never) => void) { this.i.handlers.set(event, fn); }
  } as unknown as HlsModule;
  return {Hls, instances, Events};
}

/** A clock and timers a test moves by hand. */
export function manualClock(start = 1_000_000) {
  let now = start, serial = 0;
  const timers = new Map<number, {fn: () => void; at: number}>();
  return {
    now: () => now,
    setTimer: (fn: () => void, ms: number) => { timers.set(++serial, {fn, at: now + ms}); return serial; },
    clearTimer: (t: unknown) => { timers.delete(t as number); },
    setTicker: () => 0,
    clearTicker: () => {},
    /** Move time on, running every timer that comes due, in order. */
    advance(ms: number) {
      const until = now + ms;
      for (;;) {
        const due = [...timers.entries()].filter(([, t]) => t.at <= until).sort((a, b) => a[1].at - b[1].at)[0];
        if (!due) break;
        timers.delete(due[0]);
        now = Math.max(now, due[1].at);
        due[1].fn();
      }
      now = until;
    },
    /** The machine sleeps: time passes and no timer runs until it wakes (`advance`). */
    sleep(ms: number) { now += ms; },
    pending: () => timers.size,
  };
}

export const settle = async () => { for (let i = 0; i < 12; i++) await new Promise(setImmediate); };


const viewer = {serverId: 'server', authority: 'local' as const, accountId: 'account', profileId: 'profile'};
const selectionContext: SelectionContext = {scope: {...viewer, viewId: 'home', privacyFence: 'viewer_fence'}, revision: {catalogRevision: '1', viewerRevision: '1'}};
const selectionLeaf = (id: string): HomeSelectionPlaybackItem => ({identity: {id, kind: 'movie', libraryId: 'library'}, entry: {id, kind: 'movie', title: id, libraryId: 'library', playback: {itemId: id}}, target: {itemId: id}});
const selectionAlbum: HomeSelectionPlaybackItem = {identity: {id: 'song_release', kind: 'album', libraryId: 'library'}, entry: {id: 'song_release', kind: 'album', title: 'song_release', libraryId: 'library', playback: {itemId: 'song_release-0'}}, target: {itemId: 'song_release-0'}};
const homeChannel: HomeChannel = {source: {id: 'source', name: 'source', provenance: 'live-source', generation: 'g1', publishedAt: '', availableStart: '', availableEnd: '', refreshState: ''}, channel: {id: 'channel', sourceId: 'source', provenance: 'live-source', generation: 'g1', name: 'channel', number: '', group: '', programmes: [], tuneAvailable: true, recordAvailable: false, tuneUnavailableReason: '', recordUnavailableReason: 'not-recordable', favorite: false, hidden: false, preferenceRevision: 1, mark: {text: 'channel', seed: 'channel'}}};

/** The same channel wire seam used by client-core/v1-channel-control.test.ts: a real v1
 * session/join/seek contract, without a browser clock or a catalogue item standing for live. */
class ChannelPort {
  private serial = 0;
  windowStartMs = 0;
  private sessions = new Map<string, {id: string; revision: string; state: string; presentation: {generation: number; url: string; linear: {name: string; originMs: number; windowStartMs: number; windowEndMs: number; liveEdgeMs: number; state: string; seek?: {id: string; positionMs: number; acknowledged: boolean}; confirmedPositionMs?: number}}}>();
  send(request: V1Request): V1Response | undefined {
    const body = request.body as {channelId?: string; state?: string; seek?: {id: string; live?: boolean; positionMs?: number}; positionMs?: number} | undefined;
    const response = (value: unknown, status = 200): V1Response => ({status, headers: {}, ...(value === undefined ? {} : {body: value})});
    if (request.method === 'POST' && request.path === '/v1/playback/sessions' && body?.channelId) {
      const id = `live_session_${++this.serial}`, session = {id, revision: '1', state: body.state ?? 'playing', presentation: {generation: 1, url: `/v2/media/linear/${id}/1/tok/index.m3u8`, linear: {name: 'channel', originMs: 1000, windowStartMs: this.windowStartMs, windowEndMs: 120000, liveEdgeMs: 60000, state: 'active'}}};
      this.sessions.set(id, session); return response(session, 201);
    }
    const match = /^\/v1\/playback\/sessions\/(live_session_\d+)(?:\/timeline)?$/.exec(request.path), session = match ? this.sessions.get(match[1]!) : undefined;
    if (!session) return;
    if (request.method === 'GET') return response(session);
    if (request.method === 'PATCH') {
      if (body?.state) session.state = body.state;
      if (body?.seek) session.presentation.linear.seek = {id: body.seek.id, positionMs: body.seek.live ? session.presentation.linear.liveEdgeMs : body.seek.positionMs!, acknowledged: false};
      session.revision = String(Number(session.revision) + 1); return response(session);
    }
    if (request.method === 'DELETE') { session.state = 'ended'; return response(undefined, 204); }
    if (request.method === 'POST' && request.path.endsWith('/timeline')) {
      const linear = session.presentation.linear;
      if (linear.seek && Math.abs(body!.positionMs! - linear.seek.positionMs) <= 1000) linear.seek.acknowledged = true;
      linear.confirmedPositionMs = body!.positionMs; return response(undefined, 204);
    }
    return;
  }
}
function setup(t: TestContext, preferences: Partial<WebPlaybackSettings> = {}) {
  const clock = manualClock(), server = new FakePlaybackServer({now: clock.now, audioItem: id => id.startsWith('song') || id.startsWith('book'), itemKind: id => id.startsWith('book') ? 'audiobook_file' : undefined}), inner = fakeHttp(server);
  const log: V1Request[] = [], callbacks = new Map<string, Set<(event: ServerEvent) => void>>();
  const hls = fakeHls(), video = new FakeVideo();
  const channels = new ChannelPort();
  let current = true, serial = 0, viewerFence = 'viewer_fence';
  let intercept = async (_request: V1Request, response: V1Response): Promise<V1Response> => response;
  const fetcher = async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = new URL(String(input));
    const request: V1Request = {path: url.pathname + url.search, method: (init.method ?? 'GET') as V1Request['method'], headers: Object.fromEntries(new Headers(init.headers)), ...(typeof init.body === 'string' ? {body: JSON.parse(init.body)} : {}), signal: init.signal ?? undefined};
    log.push(request);
    const itemMatch = /^\/v1\/items\/([^/]+)$/.exec(url.pathname);
    const offerMatch = /^\/v1\/items\/([^/]+)\/playback-offers$/.exec(url.pathname);
    let response: V1Response;
    if (itemMatch) {
      const itemId = decodeURIComponent(itemMatch[1]!);
      response = {status: 200, headers: {}, body: {id: itemId, kind: itemId.startsWith('song') ? 'song' : itemId.startsWith('book') ? 'audiobook_file' : 'movie', title: itemId, libraryId: 'library', available: true, missing: false, durationMs: 7200000, progressMs: 0, sources: [{id: `source_${itemId}`, durationMs: 7200000, container: 'mp4', videoCodec: 'h264', audioCodec: 'aac', width: 1920, height: 1080}]}};
    } else if (offerMatch) {
      const itemId = decodeURIComponent(offerMatch[1]!), sessionId = url.searchParams.get('sessionId')!, session = server.session(sessionId)!;
      response = {status: 200, headers: {}, body: {
        scope: {serverId: 'server', libraryId: 'library', itemId, viewerFence}, revision: `offer_${session.generation}`,
        offersRevision: 'a'.repeat(64), transcodingEnabled: true,
        sources: [{id: `source_${itemId}`, available: true, container: 'mp4', videoCodec: 'h264', audioCodec: 'aac', width: 1920, height: 1080, durationMs: 7200000, factsRevision: 1, factsStatus: 'known', relation: 'unknown', boundary: 'whole_source', enabled: false, reason: 'source_selection_unavailable', streams: [{index: 1, type: 'audio', codec: 'aac', channels: 2, default: true, forced: false, enabled: true, reason: ''}], qualities: [{id: 'auto', kind: 'automatic', label: 'Automatic', enabled: true}, {id: 'original', kind: 'original', label: 'Original', enabled: true}, {id: '720p', kind: 'fixed', label: '720p', enabled: true, maxVideoBitrateBps: 4000000, maxAudioBitrateBps: 128000, targetDisplayHeight: 720}]}],
        current: {sessionId, generation: session.generation, state: session.state, sourceId: `source_${itemId}`, delivery: 'hls', audioSelection: 'server_first_audio', subtitleSelection: 'disabled', qualityId: 'original'},
        controls: [{id: 'source', enabled: false, reason: 'source_selection_unavailable'}, {id: 'audio', enabled: false, reason: 'track_selection_unavailable'}, {id: 'subtitles', enabled: false, reason: 'subtitle_delivery_unavailable'}, {id: 'quality', enabled: true, reason: ''}],
      }};
    } else response = channels.send(request) ?? await inner.send(request as never) as V1Response;
    response = await intercept(request, response);
    return new Response(response.body === undefined ? null : JSON.stringify(response.body), {status: response.status, headers: {'Content-Type': 'application/json', ...response.headers}});
  };
  const api = new HttpLocalApi('https://portico.test', '', fetcher as typeof fetch);
  const source: WebPlaybackSource = {
    api, authority: {...viewer, epoch: 1, viewerFence: 'viewer_fence'}, contentScope: {serverId: 'server', viewerId: 'local_account_profile'}, isCurrent: () => current,
    events: {on: (type, callback) => { let list = callbacks.get(type); if (!list) callbacks.set(type, list = new Set()); list.add(callback); return () => { list!.delete(callback); }; }},
    settings: {videoSpeed: () => 1, audiobookSpeed: () => 1.25, skipModes: () => ({intro: 'off', recap: 'off', credits: 'off'}), audioEffects: () => ({gapless: false, crossfadeSeconds: 0, normalization: 'off'}), webVolume: () => true, ...preferences},
  };
  const runtime = new WebShellPlaybackRuntime(source, {timers: clock, playerTimers: clock, loadHls: async () => hls.Hls, nativeHls: 'never', key: () => `runtime-key-${++serial}`});
  runtime.mount(video);
  t.after(() => runtime.dispose());
  const arrive = async () => { await settle(); const engine = hls.instances.at(-1)!; video.ready(Number(engine.config.startPosition)); await settle(); return engine; };
  const arriveLive = async () => {
    await settle(); const instance = hls.instances.at(-1)!, seconds = runtime.getSnapshot().playback.pendingSeek?.positionSeconds ?? 60;
    instance.engine.levels = [{details: {fragments: [{start: seconds - 2, duration: 8, programDateTime: 1000 + (seconds - 2) * 1000} as never]}}];
    instance.engine.playingDate = new Date(1000 + seconds * 1000); video.ready(seconds, 12, 120); await settle(); video.land(seconds); await settle(); return instance;
  };
  return {runtime, source, api, server, log, video, hls, clock, arrive, arriveLive, expireLiveBookmark: () => { channels.windowStartMs = 50000; }, invalidate: () => { current = false; }, callbacks: () => [...callbacks.values()].reduce((n, v) => n + v.size, 0), intercept: (fn: typeof intercept) => { intercept = fn; }, changeViewerFence: () => { viewerFence = 'other_viewer'; }};
}
function deferred() { let resolve!: () => void, enter!: () => void; const promise = new Promise<void>(r => { resolve = r; }), started = new Promise<void>(r => { enter = r; }); return {started, resolve, hold: async () => { enter(); await promise; }}; }

test('runtime Home play reaches the actual queue/service/element and publishes the real source fence', async t => {
  const f = setup(t); await f.runtime.play('movie', 12.5); const engine = await f.arrive();
  const snapshot = f.runtime.getSnapshot();
  assert.equal(snapshot.playback.phase, 'ready');
  assert.equal(snapshot.playback.positionSeconds, 12.5);
  if (snapshot.fence!.kind !== 'queue') assert.fail('queue fence required');
  assert.equal(snapshot.fence!.sessionId, snapshot.playback.session!.id);
  assert.equal(snapshot.fence!.sourceId, 'source_movie');
  assert.notEqual(snapshot.fence!.sourceId, snapshot.session!.versionId);
  assert.equal(snapshot.fence!.entryId, snapshot.session!.queue!.entryId);
  assert.equal(snapshot.subject!.kind, 'video');
  assert.equal(engine.config.startPosition, 12.5);
  assert.ok(f.log.some(r => r.method === 'POST' && r.path === '/v1/queues'));
  f.video.advance(18.5); await settle();
  assert.equal(f.runtime.getSnapshot().presentation!.positionSeconds, 18.5);
  assert.ok(f.server.session(snapshot.fence!.sessionId)!.reports.some(report => report.positionMs === 18500));
});

test('runtime collapse and expansion preserve the same element, network and current position', async t => {
  const f = setup(t); await f.runtime.play('movie', 20); const engine = await f.arrive();
  const fence = f.runtime.getSnapshot().fence!, loads = f.video.loads, listeners = f.video.listenerCount();
  f.runtime.collapse(fence); f.video.advance(24); await settle(); f.runtime.open(fence);
  assert.equal(f.video.loads, loads); assert.equal(f.video.listenerCount(), listeners);
  assert.equal(engine.destroyed, false); assert.equal(f.hls.instances.length, 1);
  assert.equal(f.runtime.getSnapshot().playback.positionSeconds, 24);
});

test('runtime commands pause and resume the real element; seek waits for the actual landing', async t => {
  const f = setup(t); await f.runtime.play('movie', 20); await f.arrive();
  const fence = f.runtime.getSnapshot().fence!;
  await f.runtime.pause(fence); assert.equal(f.video.paused, true);
  await f.runtime.resume(fence); assert.equal(f.video.paused, false);
  await f.runtime.seek(fence, 60); assert.equal(f.runtime.getSnapshot().playback.pendingSeek!.positionSeconds, 60);
  assert.notEqual(f.runtime.getSnapshot().playback.positionSeconds, 60);
  f.video.land(60); await settle();
  assert.equal(f.runtime.getSnapshot().playback.pendingSeek, undefined);
  assert.equal(f.runtime.getSnapshot().playback.positionSeconds, 60);
});

test('runtime rejects an old source callback after replacement and releases the previous network', async t => {
  const f = setup(t); await f.runtime.play('first', 10); const old = await f.arrive(), fence = f.runtime.getSnapshot().fence!, count = f.video.listenerCount();
  await f.runtime.play('second', 30); await f.arrive();
  assert.equal(old.destroyed, true); assert.equal(f.video.listenerCount(), count);
  await assert.rejects(f.runtime.seek(fence, 90), /stale_playback_command/);
  assert.equal(f.runtime.getSnapshot().playback.itemId, 'second');
  assert.equal(f.runtime.getSnapshot().playback.positionSeconds, 30);
});

test('runtime parks music with its queue and restores it through an actual fresh paused session', async t => {
  const f = setup(t); await f.runtime.play('song_one', 10); await f.arrive();
  f.video.advance(35); await settle();
  const audio = f.runtime.getSnapshot(), queueId = audio.session!.queue!.queueId;
  await f.runtime.play('movie', 5); await f.arrive();
  const video = f.runtime.getSnapshot();
  assert.equal(video.dock.parkedAudio!.positionSeconds, 35);
  assert.equal(video.dock.parkedAudio!.queue, null, 'v1 occurrences are not fabricated into the strict player-core queue snapshot');
  assert.ok(video.presentations.some(p => p.subject.kind === 'music' && p.positionSeconds === 35 && p.playing === false));
  const plays = f.video.plays;
  await f.runtime.stop(video.fence!); await f.arrive();
  const restored = f.runtime.getSnapshot();
  assert.equal(restored.subject!.kind, 'music');
  assert.equal(restored.playback.intent, 'paused'); assert.equal(f.video.paused, true);
  assert.equal(f.video.plays, plays, 'the actual web adapter never calls media play during paused restoration');
  assert.equal(restored.playback.positionSeconds, 35);
  assert.equal(restored.session!.queue!.queueId, queueId);
  assert.notEqual(restored.session!.id, audio.session!.id);
  const starts = f.log.filter(r => r.method === 'POST' && r.path === '/v1/playback/sessions');
  assert.equal((starts.at(-1)!.body as {state: string}).state, 'paused');
});

test('runtime resets a session speed on a new play and rejects unsupported remote commands', async t => {
  const f = setup(t); await f.runtime.play('first', 5); await f.arrive();
  f.runtime.setSpeed(f.runtime.getSnapshot().fence!, 1.5); assert.equal(f.video.playbackRate, 1.5);
  await f.runtime.play('second', 5); await f.arrive(); assert.equal(f.video.playbackRate, 1);
  await assert.rejects(f.runtime.onAction({kind: 'play', fence: f.runtime.getSnapshot().fence!, remote: {id: 'unbound', sequence: '1'}}), /remote_playback_binding_unavailable/);
});

test('runtime uses the resolved subject preference for both actual engine and service speed', async t => {
  const f = setup(t, {videoSpeed: () => 1.75, audiobookSpeed: () => 1.25});
  for (const [itemId, kind, rate] of [['movie', 'video', 1.75], ['book_file', 'audiobook', 1.25], ['song_one', 'music', 1]] as const) {
    await f.runtime.play(itemId, 5); await f.arrive();
    const snapshot = f.runtime.getSnapshot();
    assert.equal(snapshot.subject!.kind, kind);
    assert.equal(snapshot.presentation!.rate, rate);
    assert.equal(snapshot.playback.audioEffects.rate, rate);
    assert.equal(f.video.playbackRate, rate);
  }
  assert.throws(() => f.runtime.setSpeed(f.runtime.getSnapshot().fence!, 1.5), /playback_speed_unavailable/);
  assert.equal(f.video.playbackRate, 1);
});

test('runtime waits for the actual subject before choosing a speed preference', async t => {
  let videoReads = 0, bookReads = 0;
  const f = setup(t, {videoSpeed: () => { videoReads++; return 1.75; }, audiobookSpeed: () => { bookReads++; return 1.25; }}), delayed = deferred();
  f.intercept(async (request, response) => { if (request.path === '/v1/items/book_file') await delayed.hold(); return response; });
  await f.runtime.play('book_file', 5); await delayed.started;
  assert.equal(f.runtime.getSnapshot().subject, undefined);
  assert.equal(videoReads, 0); assert.equal(bookReads, 0);
  delayed.resolve(); await f.arrive();
  assert.equal(videoReads, 0); assert.ok(bookReads > 0);
  assert.equal(f.video.playbackRate, 1.25);
});

test('runtime leaves omitted quality absent and forwards a current explicit saved preference', async t => {
  let quality: ReturnType<NonNullable<WebPlaybackSettings['defaultQuality']>>;
  const f = setup(t, {defaultQuality: () => quality});
  await f.runtime.play('movie', 5); await f.arrive();
  let request = f.log.filter(r => r.method === 'POST' && r.path === '/v1/playback/sessions').at(-1)!;
  assert.equal(Object.hasOwn(request.body as object, 'quality'), false);
  quality = {mode: 'fixed', id: '720p'};
  await f.runtime.play('second', 5); await f.arrive();
  request = f.log.filter(r => r.method === 'POST' && r.path === '/v1/playback/sessions').at(-1)!;
  assert.deepEqual((request.body as {quality: unknown}).quality, quality);
});

test('runtime ignores late playback offers after another source starts', async t => {
  const f = setup(t), gate = deferred();
  f.intercept(async (request, response) => { if (request.path.includes('/first/playback-offers')) await gate.hold(); return response; });
  await f.runtime.play('first', 5); await gate.started;
  await f.runtime.play('second', 15); await f.arrive();
  gate.resolve(); await settle();
  assert.equal(f.runtime.getSnapshot().playback.itemId, 'second');
  assert.equal(f.runtime.getSnapshot().presentation!.subject.kind, 'video');
  assert.equal(f.runtime.getSnapshot().playback.positionSeconds, 15);
});

test('runtime requires the actual viewer fence before exposing source commands', async t => {
  const f = setup(t); f.changeViewerFence(); await f.runtime.play('movie', 5); await f.arrive();
  assert.equal(f.runtime.getSnapshot().fence, undefined);
  assert.equal(f.runtime.getSnapshot().presentation, undefined);
});

test('runtime source disappearance drops media, event listeners and all pending completion effects', async t => {
  const f = setup(t); await f.runtime.play('movie', 5); const engine = await f.arrive(), fence = f.runtime.getSnapshot().fence!;
  assert.ok(f.callbacks() > 0); f.invalidate(); f.runtime.dispose();
  assert.equal(engine.destroyed, true); assert.equal(f.video.listenerCount(), 0); assert.equal(f.callbacks(), 0);
  assert.equal(f.runtime.getSnapshot().disposed, true); assert.equal(f.runtime.getSnapshot().dock.local, null);
  await assert.rejects(f.runtime.resume(fence));
  f.clock.advance(30000); await settle(); assert.equal(f.runtime.getSnapshot().disposed, true);
});

test('runtime disposal clears the actual authority-owned audio byte cache', async t => {
  const original = ByteCache.prototype.clear, cleared: ByteCache[] = [];
  ByteCache.prototype.clear = function () { cleared.push(this); original.call(this); };
  t.after(() => { ByteCache.prototype.clear = original; });
  const first = setup(t), second = setup(t);
  first.runtime.dispose(); second.runtime.dispose();
  assert.equal(cleared.length, 2); assert.notEqual(cleared[0], cleared[1], 'different authorities/runtimes cannot share private cached media');
  assert.ok(cleared.every(cache => cache.size === 0));
});

test('runtime selection Play creates and adopts the real mixed queue in supplied display order', async t => {
  const f = setup(t), port = f.runtime.selectionPlayback(context => context === selectionContext);
  const receipt = await port.play({context: selectionContext, items: [selectionLeaf('first'), selectionAlbum, selectionLeaf('last')], signal: new AbortController().signal});
  await f.arrive();
  const state = f.runtime.getSnapshot();
  assert.equal(receipt.queue.id, state.session!.queue!.queueId);
  assert.equal(state.playback.session!.id, state.session!.id);
  assert.deepEqual(state.queueView!.queue.entries.map(e => e.hidden ? null : e.itemId), ['first', ...Array.from({length: 12}, (_, index) => `song_release-${index}`), 'last']);
  const create = f.log.find(r => r.method === 'POST' && r.path === '/v1/queues')!;
  assert.deepEqual((create.body as {segments: unknown}).segments, [{source: {items: {ids: ['first']}}}, {source: {container: {kind: 'album', id: 'song_release'}}}, {source: {items: {ids: ['last']}}}]);
});

test('runtime selection Play next inserts real segments in pick order and preserves current media', async t => {
  const f = setup(t); await f.runtime.play('original', 10); const engine = await f.arrive();
  const original = f.runtime.getSnapshot().fence!, port = f.runtime.selectionPlayback(() => true);
  const receipt = await port.enqueue({context: selectionContext, items: [selectionLeaf('picked-second'), selectionAlbum, selectionLeaf('picked-first')], placement: 'next', signal: new AbortController().signal});
  const state = f.runtime.getSnapshot();
  assert.equal(receipt.queue.id, state.session!.queue!.queueId);
  assert.equal(state.fence!.sessionId, original.sessionId); assert.equal(engine.destroyed, false);
  assert.equal(state.playback.positionSeconds, 10);
  assert.deepEqual(state.queueView!.queue.entries.map(e => e.hidden ? null : e.itemId), ['original', 'picked-second', ...Array.from({length: 12}, (_, index) => `song_release-${index}`), 'picked-first']);
});

test('runtime enqueue while idle creates the actual queue without starting media', async t => {
  const f = setup(t), receipt = await f.runtime.selectionPlayback(() => true).enqueue({context: selectionContext, items: [selectionLeaf('second'), selectionLeaf('first')], placement: 'end', signal: new AbortController().signal});
  assert.equal(receipt.queue.total, 2);
  assert.equal(f.runtime.getSnapshot().playback.session, undefined);
  assert.equal(f.hls.instances.length, 0);
  assert.equal(f.log.some(r => r.method === 'POST' && r.path === '/v1/playback/sessions'), false);
  const create = f.log.find(r => r.method === 'POST' && r.path === '/v1/queues')!;
  assert.equal(Object.hasOwn(create.body as object, 'startPlayback'), false);
});

test('runtime selection cancels a late queue before media adoption when the Home context changes', async t => {
  const f = setup(t); let current = true, sessionId = '';
  f.intercept(async (request, response) => { if (request.method === 'POST' && request.path === '/v1/queues') { sessionId = (response.body as {session: {id: string}}).session.id; current = false; } return response; });
  await assert.rejects(f.runtime.selectionPlayback(() => current).play({context: selectionContext, items: [selectionLeaf('first')], signal: new AbortController().signal}));
  assert.equal(f.hls.instances.length, 0);
  assert.equal(f.server.session(sessionId)!.ended, true);
  assert.equal(f.runtime.getSnapshot().playback.session, undefined);
});

test('runtime selection abort cannot stop a newer playback or prevent another Play', async t => {
  const f = setup(t), delayed = deferred(), abort = new AbortController(); let once = true;
  f.intercept(async (request, response) => { if (once && request.method === 'POST' && request.path === '/v1/queues') { once = false; await delayed.hold(); } return response; });
  const pending = f.runtime.selectionPlayback(() => true).play({context: selectionContext, items: [selectionLeaf('first')], signal: abort.signal});
  const rejected = assert.rejects(pending);
  await delayed.started; abort.abort(); await f.runtime.play('replacement', 20); await f.arrive();
  delayed.resolve(); await rejected;
  assert.equal(f.runtime.getSnapshot().playback.itemId, 'replacement');
  assert.equal(f.video.paused, false);
});

test('runtime Home channel Play binds actual channel/session generations without a queue or catalogue reads', async t => {
  const f = setup(t); await f.runtime.selectionPlayback(() => true).playChannel(homeChannel, selectionContext, new AbortController().signal); await f.arriveLive();
  const state = f.runtime.getSnapshot(), fence = state.fence!;
  assert.equal(fence.kind, 'channel'); if (fence.kind !== 'channel') assert.fail('channel fence required');
  assert.equal(fence.channelId, 'channel'); assert.equal(fence.sourceId, 'source'); assert.equal(fence.channelGeneration, 'g1');
  assert.equal(fence.sessionId, state.playback.session!.id); assert.equal(fence.generation, state.playback.session!.generation);
  assert.equal(Object.hasOwn(fence, 'entryId'), false); assert.equal(Object.hasOwn(fence, 'itemId'), false);
  assert.equal(state.dock.local!.kind, 'live'); assert.equal(state.dock.local!.queue, null); assert.equal(state.presentation!.policy, null);
  assert.equal(f.log.some(r => r.path.startsWith('/v1/items/') || r.path === '/v1/queues'), false);
  await f.runtime.pause(fence); await settle(); assert.equal(f.video.paused, true);
  await f.runtime.resume(fence); await settle(); assert.equal(f.video.paused, false);
  await f.runtime.play('movie', 10); await f.arrive();
  await assert.rejects(f.runtime.pause(fence), /stale_playback_command/);
});

test('runtime restores a parked live session paused through the actual paused start and retained-window seek', async t => {
  const f = setup(t), port = f.runtime.selectionPlayback(() => true);
  await port.playChannel(homeChannel, selectionContext, new AbortController().signal); await f.arriveLive();
  await f.runtime.seek(f.runtime.getSnapshot().fence!, 30); await settle(); await f.arriveLive();
  const channel = f.runtime.getSnapshot().fence!;
  await f.runtime.play('song_one', 5); await f.arrive();
  assert.equal(f.runtime.getSnapshot().dock.resumeVideo!.fence.kind, 'channel');
  const before = f.log.length, plays = f.video.plays;
  await f.runtime.restoreBookmark(channel, false); await settle(); await f.arriveLive();
  const state = f.runtime.getSnapshot();
  assert.equal(state.fence!.kind, 'channel'); assert.equal(state.playback.intent, 'paused'); assert.equal(f.video.paused, true);
  assert.notEqual(state.fence!.sessionId, channel.sessionId); assert.equal(f.video.plays, plays);
  const calls = f.log.slice(before), start = calls.find(r => r.method === 'POST' && r.path === '/v1/playback/sessions')!;
  assert.equal((start.body as {state: string}).state, 'paused');
  assert.ok(calls.filter(r => r.method === 'PATCH').every(r => (r.body as {state?: string}).state !== 'playing'));
  assert.ok(calls.some(r => r.method === 'PATCH' && (r.body as {seek?: {positionMs: number}}).seek?.positionMs === 30000));
});

test('runtime reports an expired live bookmark instead of claiming a clamped restoration', async t => {
  const f = setup(t); await f.runtime.selectionPlayback(() => true).playChannel(homeChannel, selectionContext, new AbortController().signal); await f.arriveLive();
  await f.runtime.seek(f.runtime.getSnapshot().fence!, 30); await settle(); await f.arriveLive();
  const bookmark = f.runtime.getSnapshot().fence!;
  await f.runtime.play('song_one', 5); await f.arrive(); f.expireLiveBookmark();
  const before = f.log.length;
  await assert.rejects(f.runtime.restoreBookmark(bookmark, false), /channel_bookmark_expired/);
  assert.equal(f.runtime.getSnapshot().playback.intent, 'paused');
  assert.equal(f.runtime.getSnapshot().problem, 'channel_bookmark_expired');
  assert.equal(f.log.slice(before).some(r => r.method === 'PATCH' && (r.body as {seek?: {positionMs: number}}).seek?.positionMs === 30000), false);
});
