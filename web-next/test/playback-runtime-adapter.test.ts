import test, {type TestContext} from 'node:test';
import assert from 'node:assert/strict';
import {registerHooks} from 'node:module';
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
import type {PlayerOffersService} from '@core/player-offers.ts';
import {transformTsx} from './transform-tsx.mjs';
import {playbackPresentation} from '../src/shell/playback-model.ts';
import {dockView} from '../src/kit/dock/model.ts';
import type {DockAction} from '../src/kit/dock/types.ts';

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
const homeViewerFence = 'home_viewer_fence', detailViewerFence = 'detail_viewer_fence';
const selectionContext: SelectionContext = {scope: {...viewer, viewId: 'home', privacyFence: homeViewerFence}, revision: {catalogRevision: '1', viewerRevision: '1'}};
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
  let current = true, serial = 0, viewerFence = detailViewerFence;
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
    api, authority: {...viewer, epoch: 1, viewerFence: homeViewerFence}, contentScope: {serverId: 'server', viewerId: 'local_account_profile'}, isCurrent: () => current,
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
  // Exercise the real owned service's refresh/denial boundary without adding a public
  // renderer command or replacing its validated responses with snapshot doubles.
  const offers = () => (runtime as unknown as {stack: {offers: PlayerOffersService}}).stack.offers;
  return {runtime, source, api, server, log, video, hls, clock, arrive, arriveLive, offers, expireLiveBookmark: () => { channels.windowStartMs = 50000; }, invalidate: () => { current = false; }, callbacks: () => [...callbacks.values()].reduce((n, v) => n + v.size, 0), intercept: (fn: typeof intercept) => { intercept = fn; }, changeViewerFence: () => { viewerFence = 'other_detail_viewer'; }};
}
function deferred() { let resolve!: () => void, enter!: () => void; const promise = new Promise<void>(r => { resolve = r; }), started = new Promise<void>(r => { enter = r; }); return {started, resolve, hold: async () => { enter(); await promise; }}; }

test('runtime Home play reaches the actual queue/service/element and publishes the real source fence', async t => {
  const f = setup(t); await f.runtime.play('movie', 12.5); const engine = await f.arrive();
  const snapshot = f.runtime.getSnapshot(), fence = snapshot.fence;
  assert.equal(snapshot.playback.phase, 'ready');
  assert.equal(snapshot.playback.positionSeconds, 12.5);
  assert.ok(fence);
  if (fence.kind !== 'queue') assert.fail('queue fence required');
  assert.equal(fence.sessionId, snapshot.playback.session!.id);
  assert.equal(fence.sourceId, 'source_movie');
  assert.notEqual(fence.sourceId, snapshot.session!.versionId);
  assert.equal(fence.entryId, snapshot.session!.queue!.entryId);
  assert.equal(snapshot.subject!.kind, 'video');
  assert.equal(engine.config.startPosition, 12.5);
  assert.ok(f.log.some(r => r.method === 'POST' && r.path === '/v1/queues'));
  f.video.advance(18.5); await settle();
  assert.equal(f.runtime.getSnapshot().presentation!.positionSeconds, 18.5);
  const session = f.server.session(fence.sessionId);
  assert.ok(session);
  // A media reading updates the presentation immediately; ordinary timeline progress uses
  // the server's 10-second cadence on the injected clock, not the media element's clock.
  assert.equal(session.reports.some(report => report.positionMs === 18500), false);
  f.clock.advance(9999); await settle();
  assert.equal(session.reports.some(report => report.positionMs === 18500), false);
  f.clock.advance(1); await settle();
  assert.ok(session.reports.some(report => report.positionMs === 18500 && report.generation === fence.generation && report.state === 'playing'));
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

test('dock completion waits for the actual end receipt and Replay restarts the fenced queue occurrence', async t => {
  const f = setup(t); await f.runtime.play('movie', 20); await f.arrive();
  const fence = f.runtime.getSnapshot().fence!, queue = f.runtime.getSnapshot().session!.queue!, held = deferred();
  f.intercept(async (request, response) => {
    if (request.path.endsWith('/timeline') && (request.body as {state?: string})?.state === 'ended') await held.hold();
    return response;
  });
  f.video.finish(); await held.started;
  assert.equal(f.runtime.getSnapshot().playback.phase, 'ended');
  const pending = f.runtime.getSnapshot().presentation!;
  held.resolve(); await settle();
  assert.equal(pending.finished, false, 'a raw media end is provisional while the queue confirms it');
  assert.equal(pending.controls.includes('replay'), false);
  assert.equal(f.runtime.getSnapshot().presentation!.finished, true);
  assert.ok(f.runtime.getSnapshot().presentation!.controls.includes('replay'));
  assert.ok(f.server.session(fence.sessionId)!.reports.some(report => report.state === 'ended'));
  const before = f.log.length;
  await f.runtime.onAction({kind: 'replay', fence}); await f.arrive();
  const replayed = f.runtime.getSnapshot();
  assert.notEqual(replayed.fence!.sessionId, fence.sessionId);
  assert.deepEqual(replayed.session!.queue, queue);
  assert.equal(replayed.playback.positionSeconds, 0);
  assert.equal(replayed.presentation!.finished, false);
  assert.equal(f.hls.instances.at(-1)!.config.startPosition, 0);
  assert.equal(f.log.slice(before).filter(request => request.method === 'POST' && request.path === '/v1/queues').length, 0);
  await assert.rejects(f.runtime.onAction({kind: 'replay', fence}), /stale_playback_command/);
});

test('dock completion never offers final Replay during the actual next-episode countdown or advance', async t => {
  const f = setup(t), port = f.runtime.selectionPlayback(() => true);
  await port.play({context: selectionContext, items: [selectionLeaf('episode-first'), selectionLeaf('episode-next')], signal: new AbortController().signal});
  await f.arrive();
  const first = f.runtime.getSnapshot().fence!, seen: {finished: boolean; replay: boolean}[] = [], held = deferred();
  const off = f.runtime.subscribe(() => {
    const current = f.runtime.getSnapshot().presentation;
    if (current?.fence.sessionId === first.sessionId) seen.push({finished: current.finished, replay: current.controls.includes('replay')});
  });
  t.after(off);
  f.intercept(async (request, response) => { if (request.path.endsWith(':advance')) await held.hold(); return response; });
  f.video.finish(); await settle();
  assert.equal(f.runtime.getSnapshot().playback.phase, 'ended');
  assert.equal(f.runtime.getSnapshot().queueView!.next.available, true);
  f.clock.advance(10_250); await settle();
  assert.ok(f.log.some(request => request.path.endsWith(':advance')), 'the actual core queue must advance automatically');
  await held.started;
  const during = f.runtime.getSnapshot().presentation!;
  held.resolve(); await f.arrive();
  assert.equal(during.finished, false);
  assert.ok(seen.length > 0); assert.ok(seen.every(value => !value.finished && !value.replay));
  assert.equal(f.runtime.getSnapshot().playback.itemId, 'episode-next');
  assert.notEqual(f.runtime.getSnapshot().fence!.sessionId, first.sessionId);
  assert.equal(f.runtime.getSnapshot().presentation!.finished, false);
  assert.equal(f.video.paused, false);
});

test('dock completion keeps real listening auto-advance and marks only the confirmed final item', async t => {
  const f = setup(t), port = f.runtime.selectionPlayback(() => true);
  await port.play({context: selectionContext, items: [selectionLeaf('song-first'), selectionLeaf('song-last')], signal: new AbortController().signal});
  await f.arrive();
  const first = f.runtime.getSnapshot().fence!, finishedSessions: string[] = [];
  const off = f.runtime.subscribe(() => { const p = f.runtime.getSnapshot().presentation; if (p?.finished) finishedSessions.push(p.fence.sessionId); });
  t.after(off);
  f.video.finish(); await f.arrive();
  assert.equal(f.runtime.getSnapshot().playback.itemId, 'song-last');
  assert.ok(!finishedSessions.includes(first.sessionId), 'an advancing track must never advertise final Replay');
  const last = f.runtime.getSnapshot().fence!;
  assert.equal(f.runtime.getSnapshot().presentation!.finished, false, 'being the last queue entry is not completion');
  f.video.finish(); await settle();
  assert.equal(f.runtime.getSnapshot().presentation!.finished, true);
  assert.ok(finishedSessions.includes(last.sessionId));
});

test('dock completion with zero countdown advances only after the real controller publishes its held completion', async t => {
  const f = setup(t);
  f.intercept(async (request, response) => request.method === 'GET' && /^\/v1\/queues\/[^/]+$/.test(request.path)
    ? {...response, body: {...response.body as object, postPlay: {autoplay: true, countdownMs: 0}}} : response);
  await f.runtime.selectionPlayback(() => true).play({context: selectionContext, items: [selectionLeaf('first'), selectionLeaf('next')], signal: new AbortController().signal});
  await f.arrive(); const first = f.runtime.getSnapshot().fence!;
  f.video.finish(); await f.arrive();
  assert.equal(f.runtime.getSnapshot().playback.itemId, 'next');
  assert.equal(f.runtime.getSnapshot().presentation!.finished, false);
  assert.equal(f.log.filter(request => request.path.endsWith(':advance')).length, 1);
  await assert.rejects(f.runtime.onAction({kind: 'replay', fence: first}), /stale_playback_command/);
});

test('dock completion with autoplay disabled offers Replay after confirmation and does not advance', async t => {
  const f = setup(t);
  f.intercept(async (request, response) => request.method === 'GET' && /^\/v1\/queues\/[^/]+$/.test(request.path)
    ? {...response, body: {...response.body as object, postPlay: {autoplay: false, countdownMs: 0}}} : response);
  await f.runtime.selectionPlayback(() => true).play({context: selectionContext, items: [selectionLeaf('first'), selectionLeaf('next')], signal: new AbortController().signal});
  await f.arrive(); f.video.finish(); await settle(); f.clock.advance(15_000); await settle();
  assert.equal(f.runtime.getSnapshot().playback.itemId, 'first');
  assert.equal(f.runtime.getSnapshot().presentation!.finished, true);
  assert.ok(f.runtime.getSnapshot().presentation!.controls.includes('replay'));
  assert.equal(f.log.filter(request => request.path.endsWith(':advance')).length, 0);
});

test('dock completion restores the actual parked listening queue paused only after confirmed final video end', async t => {
  const f = setup(t); await f.runtime.play('song', 35); await f.arrive();
  const originalQueue = f.runtime.getSnapshot().session!.queue!.queueId;
  await f.runtime.play('movie', 20); await f.arrive();
  const held = deferred();
  f.intercept(async (request, response) => {
    if (request.path.endsWith('/timeline') && (request.body as {state?: string})?.state === 'ended') await held.hold();
    return response;
  });
  f.video.finish(); await held.started;
  const provisional = f.runtime.getSnapshot();
  held.resolve(); await f.arrive();
  assert.equal(provisional.playback.itemId, 'movie');
  assert.equal(provisional.presentation!.finished, false);
  const restored = f.runtime.getSnapshot();
  assert.equal(restored.playback.itemId, 'song');
  assert.equal(restored.session!.queue!.queueId, originalQueue);
  assert.equal(restored.playback.positionSeconds, 35);
  assert.equal(restored.playback.intent, 'paused'); assert.equal(f.video.paused, true);
  assert.equal(restored.presentation!.finished, false);
});

test('dock completion does not infer an end from duration, pause, failure, or a stale replacement event', async t => {
  const f = setup(t); await f.runtime.play('movie', 20); await f.arrive();
  const fence = f.runtime.getSnapshot().fence!;
  f.video.advance(f.video.duration); await f.runtime.pause(fence); await settle();
  assert.equal(f.runtime.getSnapshot().presentation!.finished, false);
  assert.equal(f.runtime.getSnapshot().presentation!.controls.includes('replay'), false);
  await f.runtime.play('replacement', 12); await f.arrive();
  const replacement = f.runtime.getSnapshot().fence!;
  f.video.fire('ended'); await settle();
  assert.equal(f.runtime.getSnapshot().presentation!.finished, false);
  assert.equal(f.runtime.getSnapshot().fence!.sessionId, replacement.sessionId);
  f.video.fail(3); await settle();
  assert.notEqual(f.runtime.getSnapshot().presentation?.finished, true);
  await assert.rejects(f.runtime.onAction({kind: 'replay', fence}), /stale_playback_command/);
});

test('dock completion ignores an old confirmed end after replacement and authority retirement', async t => {
  const f = setup(t); await f.runtime.play('movie', 20); await f.arrive();
  const held = deferred();
  f.intercept(async (request, response) => {
    if (request.path.endsWith('/timeline') && (request.body as {state?: string})?.state === 'ended') await held.hold();
    return response;
  });
  f.video.finish(); await held.started;
  await f.runtime.play('replacement', 12); await f.arrive();
  const replacement = f.runtime.getSnapshot().fence!;
  held.resolve(); await settle();
  assert.equal(f.runtime.getSnapshot().fence!.sessionId, replacement.sessionId);
  assert.equal(f.runtime.getSnapshot().presentation!.finished, false);
  f.invalidate(); f.video.fire('volumechange');
  assert.equal(f.runtime.getSnapshot().disposed, true);
  assert.equal(f.runtime.getSnapshot().presentation, undefined);
  assert.deepEqual(f.runtime.getSnapshot().presentations, []);
});

test('dock completion never reinterprets a live-source end as a queue Replay', async t => {
  const f = setup(t);
  await f.runtime.selectionPlayback(() => true).playChannel(homeChannel, selectionContext, new AbortController().signal);
  await f.arriveLive(); const fence = f.runtime.getSnapshot().fence!;
  f.video.finish(); await settle();
  assert.notEqual(f.runtime.getSnapshot().presentation?.finished, true);
  assert.equal(f.runtime.getSnapshot().presentation?.controls.includes('replay'), false);
  await assert.rejects(f.runtime.onAction({kind: 'replay', fence}), /playback_command_unavailable/);
});

test('dock completion actual renderer Replay dispatches through the shell presentation to the fenced runtime API', async t => {
  // Execute the unchanged real Transport/Control callbacks. Leaf icons/artwork are not part
  // of this controlled event test; it makes no React DOM, browser or pixel claim.
  const loader = registerHooks({load(url, context, next) {
    const leaf = url.endsWith('/kit/Icon.tsx') ? 'Icon' : url.endsWith('/kit/artwork/index.ts') ? 'Artwork' : url.endsWith('/kit/controls/Slider.tsx') ? 'Slider' : url.endsWith('/kit/controls/Button.tsx') ? 'Button' : null;
    if (leaf) return {format: 'module', source: `export const ${leaf} = '${leaf}';`, shortCircuit: true};
    if (url.endsWith('.module.css')) return {format: 'module', source: 'export default new Proxy({}, {get: (_, key) => String(key)});', shortCircuit: true};
    if (url.endsWith('.tsx')) return {format: 'module', source: transformTsx(url), shortCircuit: true};
    return next(url, context);
  }});
  t.after(() => loader.deregister());
  const {Transport} = await import('../src/kit/dock/parts.tsx');
  const f = setup(t); await f.runtime.play('movie', 20); await f.arrive();
  f.runtime.collapse(f.runtime.getSnapshot().fence!); f.video.finish(); await settle();
  const state = f.runtime.getSnapshot();
  const presentation = playbackPresentation(state, {t: id => id}, {foreground: () => null, ambient: () => null} as never, {back: 10, forward: 30})!;
  const view = dockView(state.dock, [presentation])!, actions: DockAction[] = [];
  let result: Promise<void> | undefined;
  const tree = Transport({view, binding: {getSnapshot: f.runtime.dock.getSnapshot, onAction: action => { actions.push(action); result = f.runtime.onAction(action); }}});
  const replay = (tree.props.children as any[]).find(child => child?.props?.primary);
  assert.equal(replay.props.label, 'Replay'); assert.equal(replay.props.disabled, false);
  replay.props.onClick(); assert.ok(result); await result; await f.arrive();
  assert.deepEqual(actions, [{kind: 'replay', fence: state.fence}]);
  assert.notEqual(f.runtime.getSnapshot().fence!.sessionId, state.fence!.sessionId);
  assert.equal(f.runtime.getSnapshot().playback.positionSeconds, 0);
  replay.props.onClick(); assert.equal(actions.length, 1, 'the old rendered callback is rejected after actual replacement');
});

test('dock completion cannot publish Finished when the actual end report fails', async t => {
  const f = setup(t); await f.runtime.play('movie', 20); await f.arrive();
  f.intercept(async (request, response) => request.path.endsWith('/timeline') && (request.body as {state?: string})?.state === 'ended'
    ? {status: 503, headers: {}, body: {error: {code: 'temporarily_unavailable'}}} : response);
  f.video.finish(); await settle();
  assert.equal(f.runtime.getSnapshot().playback.phase, 'ended');
  assert.equal(f.runtime.getSnapshot().presentation!.finished, false);
  assert.equal(f.runtime.getSnapshot().presentation!.controls.includes('replay'), false);
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

test('runtime restores the original parked queue after live playback without creating a replacement queue', async t => {
  const f = setup(t); await f.runtime.play('song_one', 10); await f.arrive();
  f.video.advance(35); await settle();
  const audio = f.runtime.getSnapshot(), bookmark = audio.fence, queue = audio.session?.queue;
  assert.ok(bookmark); assert.ok(queue);
  if (bookmark.kind !== 'queue') assert.fail('queue fence required');
  await f.runtime.selectionPlayback(() => true).playChannel(homeChannel, selectionContext, new AbortController().signal);
  const liveEngine = await f.arriveLive(), live = f.runtime.getSnapshot().fence;
  assert.ok(live); assert.equal(live.kind, 'channel');
  assert.equal(f.runtime.getSnapshot().dock.parkedAudio?.fence.sessionId, bookmark.sessionId);
  // A channel does not create a device queue. This exercises the actual adapter's restore
  // path independently of the server lifecycle of a queue replaced by another queue Play.
  assert.ok(f.server.queue(queue.queueId));
  const before = f.log.length, plays = f.video.plays;
  await f.runtime.stop(live); await f.arrive();
  const restored = f.runtime.getSnapshot(), accepted = restored.fence;
  assert.ok(accepted);
  if (accepted.kind !== 'queue') assert.fail('queue fence required');
  assert.notEqual(accepted.sessionId, bookmark.sessionId);
  assert.equal(accepted.entryId, bookmark.entryId); assert.equal(accepted.itemId, bookmark.itemId);
  assert.equal(restored.session?.queue?.queueId, queue.queueId);
  assert.equal(restored.playback.positionSeconds, 35);
  assert.equal(restored.playback.intent, 'paused'); assert.equal(f.video.paused, true);
  assert.equal(f.video.plays, plays); assert.equal(liveEngine.destroyed, true);
  const calls = f.log.slice(before);
  assert.equal(calls.some(request => request.path === '/v1/queues' && request.method === 'POST'), false);
  const starts = calls.filter(request => request.path === '/v1/playback/sessions' && request.method === 'POST');
  assert.equal(starts.length, 1);
  assert.deepEqual(starts[0]?.body, {queue: {queueId: queue.queueId, entryId: bookmark.entryId}, state: 'paused', startPositionMs: 35000});
});

test('runtime refuses a deleted parked queue without inventing another queue or starting media', async t => {
  const f = setup(t); await f.runtime.play('song_one', 10); await f.arrive();
  const audio = f.runtime.getSnapshot(), bookmark = audio.fence, queue = audio.session?.queue;
  assert.ok(bookmark); assert.ok(queue);
  await f.runtime.selectionPlayback(() => true).playChannel(homeChannel, selectionContext, new AbortController().signal);
  await f.arriveLive();
  await f.api.request(`/v1/queues/${queue.queueId}`, 'DELETE');
  assert.equal(f.server.queue(queue.queueId), undefined);
  const before = f.log.length, plays = f.video.plays, engines = f.hls.instances.length;
  await assert.rejects(f.runtime.restoreBookmark(bookmark, true), /queue_not_found/);
  assert.equal(f.video.paused, true); assert.equal(f.video.plays, plays);
  assert.equal(f.hls.instances.length, engines);
  assert.equal(f.log.slice(before).some(request => request.method === 'POST'), false);
  assert.equal(f.runtime.getSnapshot().fence, undefined);
});

test('runtime retains all three actual dock queue candidates and retires superseded extras on the next Play', async t => {
  const f = setup(t); await f.runtime.play('movie_a', 5); await f.arrive();
  const first = f.runtime.getSnapshot(), videoA = first.fence, queueA = first.session?.queue;
  assert.ok(videoA); assert.ok(queueA);
  await f.runtime.play('song_b', 10); await f.arrive();
  const queueB = f.runtime.getSnapshot().session?.queue;
  assert.ok(queueB);
  await f.runtime.play('movie_c', 15); await f.arrive();
  const queueC = f.runtime.getSnapshot().session?.queue;
  assert.ok(queueC);
  assert.equal(f.runtime.getSnapshot().dock.resumeVideo?.fence.sessionId, videoA.sessionId);
  assert.equal(f.runtime.getSnapshot().dock.parkedAudio?.kind, 'music');
  await f.runtime.play('movie_d', 20); await f.arrive();
  const queueD = f.runtime.getSnapshot().session?.queue, createD = f.log.filter(request => request.path === '/v1/queues' && request.method === 'POST').at(-1);
  assert.ok(queueD); assert.ok(createD);
  assert.deepEqual(createD.body, {owner: 'device', segments: [{source: {items: {ids: ['movie_d']}}}], retainQueueIds: [queueC.queueId, queueB.queueId, queueA.queueId]});
  for (const queue of [queueA, queueB, queueC, queueD]) assert.ok(f.server.queue(queue.queueId));
  await f.runtime.play('movie_e', 25); await f.arrive();
  const createE = f.log.filter(request => request.path === '/v1/queues' && request.method === 'POST').at(-1);
  assert.ok(createE);
  assert.deepEqual(createE.body, {owner: 'device', segments: [{source: {items: {ids: ['movie_e']}}}], retainQueueIds: [queueD.queueId, queueB.queueId, queueA.queueId]});
  assert.equal(f.server.queue(queueC.queueId), undefined, 'an old current queue is not retained once no actual dock bookmark names it');
  const plays = f.video.plays, restored = await f.runtime.restoreBookmark(videoA, false);
  await f.arrive();
  assert.equal(f.runtime.getSnapshot().session?.queue?.queueId, queueA.queueId);
  assert.equal(f.runtime.getSnapshot().playback.positionSeconds, 5);
  assert.equal(f.runtime.getSnapshot().playback.intent, 'paused'); assert.equal(f.video.plays, plays);
  await f.runtime.stop(restored); await f.arrive();
  assert.equal(f.runtime.getSnapshot().session?.queue?.queueId, queueB.queueId);
  assert.equal(f.runtime.getSnapshot().playback.positionSeconds, 10);
  assert.equal(f.runtime.getSnapshot().playback.intent, 'paused'); assert.equal(f.video.plays, plays);
});

test('runtime keeps an offered parked bookmark through more Plays than the private bookmark cache bound', async t => {
  const f = setup(t); await f.runtime.play('song_one', 10); await f.arrive();
  f.video.advance(35); await settle();
  const audio = f.runtime.getSnapshot(), bookmark = audio.fence, queue = audio.session?.queue;
  assert.ok(bookmark); assert.ok(queue);
  for (let index = 0; index < 9; index++) { await f.runtime.play(`movie_${index}`, 5); await f.arrive(); }
  const current = f.runtime.getSnapshot().fence, plays = f.video.plays;
  assert.ok(current);
  assert.equal(f.runtime.getSnapshot().dock.parkedAudio?.fence.sessionId, bookmark.sessionId);
  await f.runtime.stop(current); await f.arrive();
  assert.equal(f.runtime.getSnapshot().session?.queue?.queueId, queue.queueId);
  assert.equal(f.runtime.getSnapshot().playback.positionSeconds, 35);
  assert.equal(f.runtime.getSnapshot().playback.intent, 'paused'); assert.equal(f.video.plays, plays);
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

test('runtime requires the bound detail viewer fence before exposing source commands', async t => {
  const f = setup(t); await f.runtime.play('movie', 5); const engine = await f.arrive();
  const fence = f.runtime.getSnapshot().fence;
  assert.ok(fence);
  assert.equal(f.runtime.getSnapshot().offers?.scope.viewerFence, detailViewerFence);
  f.changeViewerFence(); await f.offers().refresh({quiet: true}); await settle();
  assert.equal(f.runtime.getSnapshot().fence, undefined);
  assert.equal(f.runtime.getSnapshot().presentation, undefined);
  assert.equal(f.offers().getSnapshot().error?.code, 'permission_changed');
  assert.equal(f.offers().getSnapshot().data, null);
  assert.equal(f.runtime.getSnapshot().offers, null);
  assert.equal(f.runtime.getSnapshot().disposed, true);
  assert.deepEqual(f.runtime.getSnapshot().presentations, []);
  assert.equal(f.runtime.getSnapshot().dock.local, null);
  assert.equal(engine.destroyed, true); assert.equal(f.video.listenerCount(), 0);
  await assert.rejects(f.runtime.resume(fence), /stale_playback_command/);
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
  const original = f.runtime.getSnapshot().fence!, port = f.runtime.selectionPlayback(() => true), before = f.log.length, plays = f.video.plays;
  const receipt = await port.enqueue({context: selectionContext, items: [selectionLeaf('picked-second'), selectionAlbum, selectionLeaf('picked-first')], placement: 'next', signal: new AbortController().signal});
  const state = f.runtime.getSnapshot();
  assert.equal(receipt.queue.id, state.session!.queue!.queueId);
  assert.equal(state.fence!.sessionId, original.sessionId); assert.equal(engine.destroyed, false);
  assert.deepEqual(state.fence, original); assert.equal(f.video.plays, plays);
  assert.equal(state.playback.positionSeconds, 10);
  assert.deepEqual(state.queueView!.queue.entries.map(e => e.hidden ? null : e.itemId), ['original', 'picked-second', ...Array.from({length: 12}, (_, index) => `song_release-${index}`), 'picked-first']);
  const next = state.queueView?.next;
  assert.ok(next?.entryId);
  assert.equal(state.queueView?.postPlay.nextEntryId, next.entryId);
  assert.ok(state.queueView?.queue.entries.some(entry => entry.id === next.entryId));
  assert.equal(f.log.slice(before).some(request => request.path === '/v1/playback/sessions' && request.method === 'POST'), false);
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

test('runtime idle enqueue retains an offered video queue without starting it or the enqueued media', async t => {
  const f = setup(t); await f.runtime.play('movie', 20); await f.arrive();
  const video = f.runtime.getSnapshot(), bookmark = video.fence, queue = video.session?.queue;
  assert.ok(bookmark); assert.ok(queue);
  await f.runtime.play('song_one', 5); await f.arrive();
  const audio = f.runtime.getSnapshot().fence;
  assert.ok(audio);
  await f.runtime.stop(audio);
  assert.equal(f.runtime.getSnapshot().playback.session, undefined);
  assert.equal(f.runtime.getSnapshot().dock.resumeVideo?.fence.sessionId, bookmark.sessionId);
  const before = f.log.length, plays = f.video.plays;
  const receipt = await f.runtime.selectionPlayback(() => true).enqueue({context: selectionContext, items: [selectionLeaf('queued')], placement: 'end', signal: new AbortController().signal});
  const calls = f.log.slice(before), create = calls.find(request => request.path === '/v1/queues' && request.method === 'POST');
  assert.ok(create);
  assert.deepEqual(create.body, {owner: 'device', segments: [{source: {items: {ids: ['queued']}}}], retainQueueIds: [queue.queueId]});
  assert.ok(f.server.queue(receipt.queue.id)); assert.ok(f.server.queue(queue.queueId));
  assert.equal(calls.some(request => request.path === '/v1/playback/sessions' && request.method === 'POST'), false);
  assert.equal(f.runtime.getSnapshot().playback.session, undefined); assert.equal(f.video.plays, plays);
  await f.runtime.restoreBookmark(bookmark, false); await f.arrive();
  assert.equal(f.runtime.getSnapshot().session?.queue?.queueId, queue.queueId);
  assert.equal(f.runtime.getSnapshot().playback.positionSeconds, 20);
  assert.equal(f.runtime.getSnapshot().playback.intent, 'paused'); assert.equal(f.video.plays, plays);
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

test('runtime rejects missing or invalid action values before changing the mounted media', async t => {
  const f = setup(t); await f.runtime.play('movie', 20); const engine = await f.arrive();
  const fence = f.runtime.getSnapshot().fence;
  assert.ok(fence);
  const before = f.log.length, seeks = [...f.video.seeks], plays = f.video.plays;
  await assert.rejects(f.runtime.onAction({kind: 'seek', fence}), /invalid_playback_position/);
  for (const seconds of [NaN, Infinity, -1, 1e9 + 1]) await assert.rejects(f.runtime.onAction({kind: 'seek', fence, seconds}), /invalid_playback_position/);
  await assert.rejects(f.runtime.onAction({kind: 'volume', fence}), /invalid_playback_volume/);
  for (const volume of [NaN, Infinity, -0.1, 1.1]) await assert.rejects(f.runtime.onAction({kind: 'volume', fence, volume}), /invalid_playback_volume/);
  assert.deepEqual(f.video.seeks, seeks);
  assert.equal(f.video.volume, 1); assert.equal(f.video.plays, plays); assert.equal(engine.destroyed, false);
  assert.equal(f.log.slice(before).some(request => request.method === 'POST' || request.method === 'PATCH' || request.method === 'DELETE'), false);
  await f.runtime.onAction({kind: 'volume', fence, volume: 0});
  assert.equal(f.video.volume, 0); assert.equal(f.runtime.getSnapshot().presentation?.volume, 0);
  await f.runtime.onAction({kind: 'mute', fence}); assert.equal(f.video.muted, true);
  await f.runtime.onAction({kind: 'mute', fence}); assert.equal(f.video.muted, false);
});

test('runtime queue-only commands cannot reinterpret a real live fence as a catalogue item', async t => {
  const f = setup(t); await f.runtime.selectionPlayback(() => true).playChannel(homeChannel, selectionContext, new AbortController().signal);
  const engine = await f.arriveLive(), fence = f.runtime.getSnapshot().fence;
  assert.ok(fence); assert.equal(fence.kind, 'channel');
  const before = f.log.length, plays = f.video.plays;
  await assert.rejects(f.runtime.replay(fence), /playback_command_unavailable/);
  await assert.rejects(f.runtime.previous(fence), /playback_command_unavailable/);
  await assert.rejects(f.runtime.next(fence), /playback_command_unavailable/);
  await assert.rejects(f.runtime.setQuality(fence, 'original'), /playback_quality_unavailable/);
  await assert.rejects(f.runtime.selectAudio(fence, 'a1'), /playback_audio_unavailable/);
  await assert.rejects(f.runtime.selectSubtitle(fence, null), /playback_subtitle_unavailable/);
  await assert.rejects(f.runtime.restoreBookmark(fence, true), /stale_playback_command/);
  assert.deepEqual(f.runtime.getSnapshot().fence, fence);
  assert.equal(engine.destroyed, false); assert.equal(f.video.plays, plays);
  assert.equal(f.log.slice(before).some(request => request.method !== 'GET'), false);
  assert.equal(f.log.some(request => request.path.startsWith('/v1/items/') || request.path === '/v1/queues'), false);
});

test('runtime refuses restoring the current queue or a different expected bookmark without toggling playback', async t => {
  const f = setup(t); await f.runtime.play('song_one', 10); await f.arrive();
  const audio = f.runtime.getSnapshot().fence;
  assert.ok(audio);
  if (audio.kind !== 'queue') assert.fail('queue fence required');
  await f.runtime.play('movie', 20); const engine = await f.arrive(), current = f.runtime.getSnapshot().fence;
  assert.ok(current);
  const before = f.log.length, plays = f.video.plays;
  await assert.rejects(f.runtime.restoreBookmark(current, true), /stale_playback_command/);
  await assert.rejects(f.runtime.restoreBookmark({...audio, sourceId: 'different_source'}, true), /stale_playback_command/);
  await assert.rejects(f.runtime.restoreBookmark({...audio, entryId: 'different_entry'}, true), /stale_playback_command/);
  await assert.rejects(f.runtime.restoreBookmark({...audio, generation: audio.generation + 1}, true), /stale_playback_command/);
  assert.deepEqual(f.runtime.getSnapshot().fence, current);
  assert.equal(f.runtime.getSnapshot().dock.parkedAudio?.fence.sessionId, audio.sessionId);
  assert.equal(f.video.paused, false); assert.equal(f.video.plays, plays); assert.equal(engine.destroyed, false);
  assert.equal(f.log.slice(before).some(request => request.method !== 'GET'), false);
});

test('runtime explicit Resume waits for the restored queue source and starts only its fresh paused session', async t => {
  const f = setup(t); await f.runtime.play('song_one', 10); await f.arrive();
  f.video.advance(35); await settle();
  const audio = f.runtime.getSnapshot(), bookmark = audio.fence;
  assert.ok(bookmark);
  if (bookmark.kind !== 'queue') assert.fail('queue fence required');
  assert.ok(audio.session?.queue);
  await f.runtime.play('movie', 5); await f.arrive();
  const gate = deferred(), before = f.log.length, plays = f.video.plays;
  f.intercept(async (request, response) => { if (request.path.startsWith('/v1/items/song_one/playback-offers')) await gate.hold(); return response; });
  const pending = f.runtime.restoreBookmark(bookmark, true);
  await gate.started; await settle();
  assert.equal(f.runtime.getSnapshot().playback.intent, 'paused');
  assert.equal(f.runtime.getSnapshot().fence, undefined);
  assert.equal(f.video.paused, true); assert.equal(f.video.plays, plays);
  assert.equal(f.log.slice(before).some(request => request.path === '/v1/queues' && request.method === 'POST'), false);
  const starts = f.log.slice(before).filter(request => request.path === '/v1/playback/sessions' && request.method === 'POST');
  assert.equal(starts.length, 1);
  assert.deepEqual(starts[0]?.body, {queue: {queueId: audio.session.queue.queueId, entryId: bookmark.entryId}, state: 'paused', startPositionMs: 35000});
  gate.resolve();
  const accepted = await pending; await f.arrive();
  assert.equal(accepted.kind, 'queue');
  if (accepted.kind !== 'queue') assert.fail('queue fence required');
  assert.notEqual(accepted.sessionId, bookmark.sessionId);
  assert.equal(accepted.itemId, bookmark.itemId); assert.equal(accepted.entryId, bookmark.entryId);
  assert.equal(f.runtime.getSnapshot().session?.queue?.queueId, audio.session.queue.queueId);
  assert.equal(f.runtime.getSnapshot().playback.intent, 'playing'); assert.equal(f.video.paused, false);
  assert.equal(f.runtime.getSnapshot().playback.positionSeconds, 35);
});

test('runtime late explicit Resume cannot resume or replace a newer Play', async t => {
  const f = setup(t); await f.runtime.play('song_one', 10); await f.arrive();
  const bookmark = f.runtime.getSnapshot().fence;
  assert.ok(bookmark);
  await f.runtime.play('movie', 5); await f.arrive();
  const gate = deferred();
  f.intercept(async (request, response) => { if (request.path.startsWith('/v1/items/song_one/playback-offers')) await gate.hold(); return response; });
  const rejected = assert.rejects(f.runtime.restoreBookmark(bookmark, true), /stale_playback_command/);
  await gate.started;
  await f.runtime.play('replacement', 20); const engine = await f.arrive();
  const current = f.runtime.getSnapshot().fence, plays = f.video.plays;
  assert.ok(current);
  await f.runtime.pause(current); assert.equal(f.video.paused, true);
  gate.resolve(); await rejected; await settle();
  assert.deepEqual(f.runtime.getSnapshot().fence, current);
  assert.equal(f.runtime.getSnapshot().playback.itemId, 'replacement');
  assert.equal(f.runtime.getSnapshot().playback.positionSeconds, 20);
  assert.equal(f.runtime.getSnapshot().playback.intent, 'paused'); assert.equal(f.video.paused, true);
  assert.equal(f.video.plays, plays); assert.equal(engine.destroyed, false);
});

test('runtime rejects a late parked queue header after a competing Play', async t => {
  const f = setup(t); await f.runtime.play('song_one', 10); await f.arrive();
  const audio = f.runtime.getSnapshot(), bookmark = audio.fence, queue = audio.session?.queue;
  assert.ok(bookmark); assert.ok(queue);
  await f.runtime.selectionPlayback(() => true).playChannel(homeChannel, selectionContext, new AbortController().signal);
  await f.arriveLive();
  const gate = deferred(), restoring: {signal?: AbortSignal} = {};
  f.intercept(async (request, response) => {
    if (request.method === 'GET' && request.path === `/v1/queues/${queue.queueId}`) { restoring.signal = request.signal; await gate.hold(); }
    return response;
  });
  const rejected = assert.rejects(f.runtime.restoreBookmark(bookmark, true), /stale_playback_command/);
  await gate.started;
  await f.runtime.play('replacement', 20); const engine = await f.arrive();
  const current = f.runtime.getSnapshot().fence;
  assert.ok(current); assert.ok(restoring.signal); assert.equal(restoring.signal.aborted, true);
  await f.runtime.pause(current);
  const before = f.log.length, plays = f.video.plays;
  gate.resolve(); await rejected; await settle();
  assert.deepEqual(f.runtime.getSnapshot().fence, current);
  assert.equal(f.runtime.getSnapshot().playback.itemId, 'replacement');
  assert.equal(f.runtime.getSnapshot().playback.positionSeconds, 20);
  assert.equal(f.runtime.getSnapshot().playback.intent, 'paused'); assert.equal(f.video.paused, true);
  assert.equal(f.video.plays, plays); assert.equal(engine.destroyed, false);
  assert.equal(f.log.slice(before).some(request => request.method === 'POST' && request.path === '/v1/playback/sessions'), false);
});

test('runtime quality changes require current options and an enabled rung on the actual source', async t => {
  const f = setup(t), gate = deferred();
  f.intercept(async (request, response) => { if (request.path === '/v1/items/movie/playback-options?versionId=v1') await gate.hold(); return response; });
  await f.runtime.play('movie', 20); await gate.started; await f.arrive();
  const fence = f.runtime.getSnapshot().fence;
  assert.ok(fence);
  assert.equal(f.runtime.getSnapshot().options, null);
  const before = f.log.length;
  await assert.rejects(f.runtime.setQuality(fence, '720'), /playback_quality_unavailable/);
  assert.equal(f.log.slice(before).some(request => request.method === 'PATCH'), false);
  gate.resolve(); await settle();
  assert.ok(f.runtime.getSnapshot().options);
  // The older options endpoint offers 1080p, but this source's current offer does not.
  await assert.rejects(f.runtime.setQuality(fence, '1080'), /playback_quality_unavailable/);
  assert.equal(f.log.slice(before).some(request => request.method === 'PATCH'), false);
  await f.runtime.setQuality(fence, '720'); await settle();
  assert.ok(f.log.slice(before).some(request => request.path === `/v1/playback/sessions/${fence.sessionId}` && request.method === 'PATCH'));
  assert.equal(f.runtime.getSnapshot().playback.session?.id, fence.sessionId);
  assert.ok((f.runtime.getSnapshot().playback.session?.generation ?? 0) > fence.generation);
  await assert.rejects(f.runtime.setQuality(fence, '720'), /stale_playback_command/);
});

test('runtime Home selection binds distinct Home and detail fences to the actual queue and persistent media', async t => {
  const f = setup(t), picked = selectionLeaf('movie');
  // The canonical fake's default queue leaf is three minutes, while this fixture's
  // actual movie media/options already declare two hours. Publish that same duration
  // in this test's queue receipt so 1040 seconds is a valid authoritative position.
  f.intercept(async (request, response) => {
    if (request.method === 'POST' && request.path === '/v1/queues' && response.status === 201) {
      const body = response.body as {window: {itemId: string; durationMs: number}[]};
      assert.ok(body.window.length > 0); assert.ok(body.window.every(entry => entry.itemId === 'movie' && entry.durationMs === 180000));
      return {...response, body: {...body, window: body.window.map(entry => ({...entry, durationMs: 7200000}))}};
    }
    return response;
  });
  assert.ok('target' in picked);
  const receipt = await f.runtime.selectionPlayback(context => context === selectionContext).play({context: selectionContext, items: [{...picked, target: {...picked.target, startSeconds: 1040}}], signal: new AbortController().signal});
  const engine = await f.arrive(), state = f.runtime.getSnapshot(), fence = state.fence;
  assert.ok(fence); assert.equal(fence.kind, 'queue');
  assert.equal(state.authority.viewerFence, homeViewerFence);
  assert.equal(state.offers?.scope.viewerFence, detailViewerFence);
  assert.notEqual(state.offers?.scope.viewerFence, state.authority.viewerFence);
  assert.equal(state.offers, f.offers().getSnapshot().data, 'the actual detail response is preserved, including its own fence');
  assert.equal(state.session?.queue?.queueId, receipt.queue.id);
  assert.equal(fence.sessionId, state.session?.id); assert.equal(fence.generation, state.playback.session?.generation);
  assert.equal(state.dock.local?.fence.sessionId, fence.sessionId);
  assert.ok(state.presentation); assert.equal(state.presentation.expanded, true); assert.equal(state.presentation.positionSeconds, 1040);
  const loads = f.video.loads, listeners = f.video.listenerCount();
  f.runtime.collapse(fence); f.video.advance(1042); await settle();
  assert.equal(f.runtime.getSnapshot().presentation?.expanded, false);
  assert.equal(f.runtime.getSnapshot().dock.local?.positionSeconds, 1042);
  f.runtime.open(fence);
  assert.equal(f.runtime.getSnapshot().presentation?.expanded, true);
  assert.equal(f.runtime.getSnapshot().presentation?.positionSeconds, 1042);
  assert.equal(f.video.loads, loads); assert.equal(f.video.listenerCount(), listeners);
  assert.equal(engine.destroyed, false); assert.equal(f.hls.instances.length, 1);
});

test('runtime accepts the same actual detail fence after refresh and a new media generation', async t => {
  const f = setup(t); await f.runtime.play('movie', 20); await f.arrive();
  const first = f.runtime.getSnapshot(), fence = first.fence;
  assert.ok(fence); assert.notEqual(first.offers?.scope.viewerFence, first.authority.viewerFence);
  await f.offers().refresh({quiet: true}); await settle();
  assert.deepEqual(f.runtime.getSnapshot().fence, fence);
  assert.equal(f.runtime.getSnapshot().offers?.scope.viewerFence, detailViewerFence);
  await f.runtime.setQuality(fence, '720'); await f.arrive();
  const changed = f.runtime.getSnapshot(), next = changed.fence;
  assert.ok(next); assert.equal(next.sessionId, fence.sessionId); assert.ok(next.generation > fence.generation);
  assert.equal(changed.offers?.current?.generation, next.generation);
  assert.equal(changed.offers?.scope.viewerFence, detailViewerFence);
  assert.equal(changed.presentation?.fence.generation, next.generation);
  assert.equal(changed.dock.local?.fence.generation, next.generation);
  await assert.rejects(f.runtime.pause(fence), /stale_playback_command/);
  await f.runtime.pause(next); assert.equal(f.video.paused, true);
});

test('runtime cannot rebind a changed detail fence when the media generation changes', async t => {
  const f = setup(t); await f.runtime.play('movie', 20); await f.arrive();
  const first = f.runtime.getSnapshot().fence;
  assert.ok(first);
  f.changeViewerFence(); await f.runtime.setQuality(first, '720'); await settle();
  assert.equal(f.offers().getSnapshot().error?.code, 'permission_changed');
  assert.equal(f.offers().getSnapshot().data, null);
  assert.equal(f.runtime.getSnapshot().offers, null);
  assert.equal(f.runtime.getSnapshot().fence, undefined);
  assert.equal(f.runtime.getSnapshot().presentation, undefined);
  assert.deepEqual(f.runtime.getSnapshot().presentations, []);
  assert.equal(f.runtime.getSnapshot().disposed, true);
  assert.equal(f.runtime.getSnapshot().dock.local, null);
  await assert.rejects(f.runtime.resume(first), /stale_playback_command/);
});

test('runtime rejects cross-server, library, item, session and media-generation offer responses', async t => {
  type OfferWire = {scope: {serverId: string; libraryId: string; itemId: string}; current: {sessionId: string; generation: number}};
  const changes: readonly ((wire: OfferWire) => void)[] = [
    wire => { wire.scope.serverId = 'foreign_server'; },
    wire => { wire.scope.libraryId = 'foreign_library'; },
    wire => { wire.scope.itemId = 'foreign_item'; },
    wire => { wire.current.sessionId = 'foreign_session'; },
    wire => { wire.current.generation += 1; },
  ];
  for (const [index, change] of changes.entries()) {
    const f = setup(t);
    f.intercept(async (request, response) => { if (request.path.includes('/playback-offers')) change(response.body as OfferWire); return response; });
    await f.runtime.play('movie', 20); await f.arrive();
    assert.equal(f.runtime.getSnapshot().offers, null);
    assert.equal(f.runtime.getSnapshot().fence, undefined);
    assert.equal(f.runtime.getSnapshot().presentation, undefined);
    assert.equal(f.runtime.getSnapshot().dock.local, null);
    if (index === changes.length - 1) assert.equal(f.offers().getSnapshot().phase, 'ready', 'future media generation is rejected by the runtime after response parsing');
    f.runtime.dispose();
  }
});

test('runtime rejects an older media-generation offer even when the real offers parser accepts it', async t => {
  const f = setup(t); await f.runtime.play('movie', 20); await f.arrive();
  const first = f.runtime.getSnapshot().fence;
  assert.ok(first);
  f.intercept(async (request, response) => {
    if (request.path.includes('/playback-offers')) (response.body as {current: {generation: number}}).current.generation = first.generation;
    return response;
  });
  await f.runtime.setQuality(first, '720'); await f.arrive();
  assert.ok(f.runtime.getSnapshot().playback.session!.generation > first.generation);
  assert.equal(f.offers().getSnapshot().phase, 'ready');
  assert.equal(f.offers().getSnapshot().data?.current?.generation, first.generation);
  assert.equal(f.runtime.getSnapshot().offers, null);
  assert.equal(f.runtime.getSnapshot().fence, undefined);
  assert.equal(f.runtime.getSnapshot().presentation, undefined);
  await assert.rejects(f.runtime.resume(first), /stale_playback_command/);
});

test('runtime drops late same-item offers from a replaced session even when transport ignores abort', async t => {
  const f = setup(t), gate = deferred(); let once = true;
  f.intercept(async (request, response) => {
    if (once && request.path.includes('/movie/playback-offers')) { once = false; await gate.hold(); }
    return response;
  });
  await f.runtime.play('movie', 5); await gate.started;
  const first = f.runtime.getSnapshot().playback.session;
  assert.ok(first);
  await f.runtime.play('movie', 30); const engine = await f.arrive();
  const second = f.runtime.getSnapshot(), fence = second.fence;
  assert.ok(fence); assert.notEqual(fence.sessionId, first.id);
  gate.resolve(); await settle();
  assert.deepEqual(f.runtime.getSnapshot().fence, fence);
  assert.equal(f.runtime.getSnapshot().offers?.current?.sessionId, fence.sessionId);
  assert.equal(f.runtime.getSnapshot().presentation?.positionSeconds, 30);
  assert.equal(f.server.session(first.id)?.ended, true); assert.equal(engine.destroyed, false);
});

test('runtime privacy denials purge current and parked metadata, media and command authority', async t => {
  for (const status of [401, 403]) {
    const f = setup(t); await f.runtime.play('song_one', 10); await f.arrive();
    await f.runtime.play('movie', 20); const engine = await f.arrive();
    const first = f.runtime.getSnapshot(), fence = first.fence;
    assert.ok(fence); assert.ok(first.dock.parkedAudio); assert.ok(first.presentations.length > 1);
    const offers = f.offers();
    f.intercept(async (request, response) => request.path.includes('/playback-offers') ? {status, headers: {}, body: {message: 'private denial payload'}} : response);
    await offers.refresh({quiet: true}); await settle();
    const denied = f.runtime.getSnapshot();
    assert.equal(offers.getSnapshot().error?.code, status === 401 ? 'unauthorized' : 'forbidden');
    assert.equal(offers.getSnapshot().data, null);
    assert.equal(denied.disposed, true); assert.equal(denied.fence, undefined);
    assert.equal(denied.subject, undefined); assert.equal(denied.presentation, undefined);
    assert.equal(denied.offers, null); assert.equal(denied.options, null); assert.equal(denied.queueView, null);
    assert.deepEqual(denied.presentations, []);
    assert.equal(denied.dock.local, null); assert.equal(denied.dock.parkedAudio, null); assert.equal(denied.dock.resumeVideo, null);
    assert.equal(engine.destroyed, true); assert.equal(f.video.listenerCount(), 0); assert.equal(f.callbacks(), 0);
    assert.equal(f.server.session(fence.sessionId)?.ended, true);
    assert.equal(JSON.stringify([offers.getSnapshot(), denied]).includes('private denial payload'), false);
    await assert.rejects(f.runtime.resume(fence), /stale_playback_command/);
    const before = f.log.length;
    await assert.rejects(f.runtime.play('other'), /playback_authority_retired/);
    assert.equal(f.log.length, before);
  }
});

test('runtime Home authority retirement prevents an outstanding detail response from publishing', async t => {
  const f = setup(t), gate = deferred();
  f.intercept(async (request, response) => { if (request.path.includes('/playback-offers')) await gate.hold(); return response; });
  await f.runtime.play('movie', 20); await gate.started;
  assert.equal(f.runtime.getSnapshot().fence, undefined);
  // bootstrap restarts assembly/runtime when Home's library-set fence changes. Its epoch
  // closure retires this source before the new Home authority can become current.
  f.invalidate(); f.video.advance(21); await settle();
  gate.resolve(); await settle();
  const retired = f.runtime.getSnapshot();
  assert.equal(retired.disposed, true); assert.equal(retired.offers, null);
  assert.equal(retired.fence, undefined); assert.equal(retired.subject, undefined); assert.equal(retired.presentation, undefined);
  assert.deepEqual(retired.presentations, []); assert.equal(retired.dock.local, null);
  assert.equal(f.video.listenerCount(), 0); assert.equal(f.callbacks(), 0);
  assert.equal(f.log.some(request => request.path.includes('/playback-options')), false);
  await assert.rejects(f.runtime.play('other'), /playback_authority_retired/);
  const video = new FakeVideo(), hls = fakeHls(), clock = manualClock(); let serial = 0;
  const next = new WebShellPlaybackRuntime({...f.source, authority: {...f.source.authority, epoch: 2, viewerFence: 'home_after_library_change'}, isCurrent: () => true}, {timers: clock, playerTimers: clock, loadHls: async () => hls.Hls, nativeHls: 'never', key: () => `next-home-key-${++serial}`});
  next.mount(video); t.after(() => next.dispose());
  await next.play('movie', 30); await settle();
  video.ready(Number(hls.instances.at(-1)!.config.startPosition)); await settle();
  assert.ok(next.getSnapshot().fence);
  assert.equal(next.getSnapshot().authority.viewerFence, 'home_after_library_change');
  assert.equal(next.getSnapshot().authority.epoch, 2);
  assert.equal(next.getSnapshot().offers?.scope.viewerFence, detailViewerFence);
  assert.equal(next.getSnapshot().presentation?.positionSeconds, 30);
  assert.equal(f.runtime.getSnapshot().disposed, true);
});

test('runtime selection still requires the Home authority fence rather than the bound detail fence', async t => {
  const f = setup(t), wrong = {...selectionContext, scope: {...selectionContext.scope, privacyFence: detailViewerFence}};
  await settle();
  assert.deepEqual(f.log.map(request => [request.method, request.path]), [['GET', '/v1/listening/preferences']]);
  const before = f.log.length;
  await assert.rejects(f.runtime.selectionPlayback(() => true).play({context: wrong, items: [selectionLeaf('movie')], signal: new AbortController().signal}), /stale_selection_context/);
  assert.equal(f.log.length, before); assert.deepEqual(f.log.slice(before), []); assert.equal(f.hls.instances.length, 0);
  assert.equal(f.runtime.getSnapshot().fence, undefined); assert.equal(f.runtime.getSnapshot().offers, null);
});
