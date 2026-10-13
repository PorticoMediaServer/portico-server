import test from 'node:test';
import assert from 'node:assert/strict';
import {WebMediaPlayer, bufferTargets, type WebPlayerNotice} from '../src/playback-v1/web-media-player.ts';
import type {PlayerEvent} from '@core/playback-v1/index.ts';
import {FakeVideo, fakeHls, manualClock, settle} from './helpers/fake-media.ts';

const resolveUrl = (u: string) => new URL(u, 'https://portico.test').href;
const source = (over: Partial<Parameters<WebMediaPlayer['load']>[0]> = {}) => ({url: '/v1/media/grant/1/master.m3u8', generation: 1, mode: 'stream' as const, startPositionMs: 90_000, subtitles: [], autoplay: true, ...over});

function setup(options: {supported?: boolean; subtitles?: {reset(): void; timeline(o: number | null): void}; silentMs?: number; giveUpMs?: number; hls?: boolean} = {}) {
  const video = new FakeVideo();
  const clock = manualClock();
  const {Hls, instances, Events} = fakeHls(options.supported ?? true);
  const player = new WebMediaPlayer(video, {...(options.hls === false ? {} : {loadHls: async () => Hls}), resolveUrl, timers: clock, subtitles: options.subtitles, silentMs: options.silentMs, giveUpMs: options.giveUpMs});
  const events: PlayerEvent[] = [], notices: WebPlayerNotice[] = [];
  player.onEvent(e => events.push(e));
  player.onNotice(n => notices.push(n));
  return {video, clock, Hls, instances, Events, player, events, notices};
}
const types = (list: WebPlayerNotice[]) => list.map(n => n.type);

test('a resume loads from the resume point, never from the start of the title', async () => {
  const {video, instances, player, events} = setup();
  player.load(source());
  await settle();
  const hls = instances[0]!;
  assert.equal(hls.source, 'https://portico.test/v1/media/grant/1/master.m3u8');
  assert.equal(hls.config.startPosition, 90, 'hls.js is told where playback begins');
  assert.equal(video.seeks.length, 0, 'no seek after loading the beginning');
  // Until the picture is there, the reading is the position asked for, not the element's zero.
  assert.equal(player.observe().state, 'loading');
  assert.equal(player.observe().positionMs, 90_000);
  video.ready(90);
  assert.equal(player.settling, false);
  assert.equal(events.filter(e => e.type === 'seeked').length, 1, 'the landing is announced once');
  assert.equal(player.observe().state, 'playing');
  assert.equal(player.observe().positionMs, 90_000);
  player.dispose();
  assert.equal(video.listenerCount(), 0);
});

test('capability URLs keep their origin and carry no cookies; the engine never switches quality by itself', async () => {
  const {instances, player} = setup();
  player.load(source({startPositionMs: 0}));
  await settle();
  const config = instances[0]!.config;
  const xhr = {withCredentials: true};
  (config.xhrSetup as (x: typeof xhr, u: string) => void)(xhr, 'https://portico.test/v1/media/grant/1/seg1.ts');
  assert.equal(xhr.withCredentials, false);
  assert.throws(() => (config.xhrSetup as (x: typeof xhr, u: string) => void)(xhr, 'https://elsewhere.test/x.ts'));
  assert.equal(config.capLevelToPlayerSize, false);
  // A manifest with two renditions is pinned to its best; no automatic switching.
  instances[0]!.engine.levels = [{bitrate: 2_000_000}, {bitrate: 8_000_000}];
  instances[0]!.emit('hlsManifestParsed');
  assert.equal(instances[0]!.engine.loadLevel, 1);
  player.dispose();
});

test('a fragment has no wall-clock limit: one that keeps receiving bytes for minutes is never restarted or failed', async () => {
  const {video, clock, instances, player, events, notices} = setup();
  player.load(source({startPositionMs: 0}));
  await settle();
  const hls = instances[0]!;
  const policy = (hls.config.fragLoadPolicy as {default: {maxLoadTimeMs: number}}).default;
  assert.ok(policy.maxLoadTimeMs >= 600_000, 'the total is a far ceiling, not a deadline');
  video.ready(0, 4);
  video.starve();
  const frag = hls.fragment(6);
  // Five minutes of a slow link: a trickle of bytes every tick.
  for (let i = 0; i < 300; i++) { frag.stats.loaded += 20_000; clock.advance(1000); player.tick(); }
  assert.deepEqual(hls.startLoads, [], 'the download in hand was never thrown away');
  assert.equal(events.some(e => e.type === 'error'), false);
  assert.ok(types(notices).includes('alive'), 'the start or seek in hand is reported alive');
  player.dispose();
});

test('a download that receives nothing is restarted; with no sign of life for long enough, playback is given up', async () => {
  const {video, clock, instances, player, events, notices} = setup({silentMs: 20_000, giveUpMs: 90_000});
  player.load(source({startPositionMs: 0}));
  await settle();
  const hls = instances[0]!;
  video.ready(0, 4);
  video.starve();
  hls.fragment(6);
  for (let i = 0; i < 19; i++) { clock.advance(1000); player.tick(); }
  assert.deepEqual(hls.startLoads, [], 'nineteen silent seconds are still patience');
  for (let i = 0; i < 4; i++) { clock.advance(1000); player.tick(); }
  assert.equal(hls.startLoads.length, 1, 'a dead transfer is asked for again');
  assert.ok(notices.some(n => n.type === 'starved' && n.active), 'the viewer is told once the buffer is empty');
  assert.equal(events.some(e => e.type === 'error'), false);
  for (let i = 0; i < 100; i++) { clock.advance(1000); player.tick(); }
  const error = events.find(e => e.type === 'error');
  assert.ok(error && error.type === 'error' && error.code === 'network_error');
  player.dispose();
});

test('recovery never interrupts a download that is receiving bytes, and "still preparing" keeps a slow server alive', async () => {
  const {video, clock, instances, player, events} = setup({giveUpMs: 30_000});
  player.load(source({startPositionMs: 0}));
  await settle();
  const hls = instances[0]!;
  video.ready(0, 4);
  video.starve();
  hls.error({fatal: true, type: 'networkError', details: 'fragLoadError', response: {code: 502}});
  clock.advance(1000);
  assert.equal(hls.startLoads.length, 1, 'a failed load is asked for again');
  const frag = hls.fragment(6);
  for (let i = 0; i < 60; i++) { frag.stats.loaded += 10_000; clock.advance(1000); player.tick(); }
  assert.equal(hls.startLoads.length, 1, 'while bytes arrive nothing is restarted');
  assert.equal(events.some(e => e.type === 'error'), false, 'slow is not failed, however long it takes');
  // The server says it is still preparing: that is the server working.
  hls.loaded(frag);
  for (let i = 0; i < 60; i++) { hls.error({fatal: false, type: 'networkError', response: {code: 503, data: JSON.stringify({error: {code: 'segment_preparing'}})}}); clock.advance(1000); player.tick(); }
  assert.equal(events.some(e => e.type === 'error'), false);
  // Media arrives: recovery ends.
  video.ready(0, 12);
  video.fire('playing');
  clock.advance(12_000);
  assert.equal(clock.pending(), 0, 'nothing keeps retrying once media flows');
  player.dispose();
});

test('errors: a refused stream fails at once; a media error recovers twice, then is a decode error', async () => {
  const a = setup();
  a.player.load(source());
  await settle();
  a.instances[0]!.error({fatal: true, type: 'networkError', response: {code: 410}});
  const gone = a.events.find(e => e.type === 'error');
  assert.ok(gone && gone.type === 'error' && gone.code === 'stream_gone');
  assert.equal(a.instances[0]!.destroyed, true);
  a.player.dispose();

  const b = setup();
  b.player.load(source());
  await settle();
  const hls = b.instances[0]!;
  hls.error({fatal: true, type: 'mediaError', details: 'bufferAppendError'});
  hls.error({fatal: true, type: 'mediaError', details: 'bufferAppendError'});
  assert.equal(hls.recoveries, 2);
  assert.equal(hls.swaps, 1, 'the second attempt swaps the audio codec hint');
  hls.error({fatal: true, type: 'mediaError', details: 'bufferAppendError'});
  const decode = b.events.find(e => e.type === 'error');
  assert.ok(decode && decode.type === 'error' && decode.code === 'decode_error');
  hls.error({fatal: false, type: 'mediaError'});
  b.player.dispose();
});

test('a pause from outside the page is the viewer’s; the port’s own pause and the end of a title are not', async () => {
  const {video, player, notices} = setup();
  player.load(source({startPositionMs: 0}));
  await settle();
  video.ready(0);
  assert.equal(player.observe().state, 'playing');
  // A media key, the Picture in Picture window, a headset: the element pauses by itself.
  video.pause();
  assert.deepEqual(notices.filter(n => n.type === 'unsolicited'), [{type: 'unsolicited', state: 'paused'}]);
  assert.equal(player.observe().state, 'paused');
  // …and starts again from outside.
  void video.play();
  assert.deepEqual(notices.filter(n => n.type === 'unsolicited').at(-1), {type: 'unsolicited', state: 'playing'});
  // The port's own pause is not news.
  const before = notices.length;
  player.pause();
  assert.equal(notices.length, before);
  player.play();
  await settle();
  // The browser's final pause just before `ended` is completion.
  video.finish();
  assert.equal(notices.filter(n => n.type === 'unsolicited').length, 2);
  assert.equal(player.observe().state, 'ended');
  player.dispose();
});

test('a refused autoplay is reported, not retried', async () => {
  const {video, player, notices} = setup();
  video.refusePlay = true;
  player.load(source({startPositionMs: 0}));
  await settle();
  video.ready(0);
  await settle();
  assert.ok(types(notices).includes('autoplay-blocked'));
  assert.equal(player.observe().state, 'paused');
  player.dispose();
});

test('direct play: the source is the file, the start position is set once metadata is there', async () => {
  const {video, instances, player, events} = setup();
  player.load(source({mode: 'direct', url: '/v1/media/grant/1/file.mp4', startPositionMs: 30_000}));
  await settle();
  assert.equal(instances.length, 0);
  assert.equal(video.src, 'https://portico.test/v1/media/grant/1/file.mp4');
  video.readyState = 1;
  video.duration = 600;
  video.fire('loadedmetadata');
  assert.deepEqual(video.seeks, [30]);
  video.land(30);
  video.fire('canplay');
  assert.equal(events.filter(e => e.type === 'seeked').length, 1);
  assert.equal(player.observe().positionMs, 30_000);
  // A network error after it has played is asked for again, quietly, from where it was.
  video.advance(45);
  video.fail(2);
  assert.equal(events.some(e => e.type === 'error'), false);
  // A file the browser cannot decode is the engine refusing this route.
  const fresh = setup();
  fresh.player.load(source({mode: 'direct', url: '/v1/media/grant/2/file.mkv', startPositionMs: 0}));
  fresh.video.fail(4);
  const refused = fresh.events.find(e => e.type === 'error');
  assert.ok(refused && refused.type === 'error' && refused.code === 'source_not_supported');
  player.dispose();
  fresh.player.dispose();
});

test('a seek is acknowledged where it lands; one the element cannot reach is reported', async () => {
  const {video, player, events, notices} = setup();
  player.load(source({startPositionMs: 0}));
  await settle();
  video.ready(0);
  player.seek(600_000);
  assert.deepEqual(video.seeks, [600]);
  assert.equal(player.settling, true);
  assert.equal(player.observe().positionMs, 0, 'the position is not reported before the picture is there');
  video.land(600);
  assert.equal(player.settling, false);
  assert.equal(events.filter(e => e.type === 'seeked').at(-1)!.observation.positionMs, 600_000);
  player.seek(900_000);
  video.land(30); // the element went somewhere else
  assert.deepEqual(notices.find(n => n.type === 'seek-failed'), {type: 'seek-failed', positionMs: 900_000});
  player.dispose();
});

test('no hls.js support: native HLS where the browser has it, else unsupported', async () => {
  const a = setup({supported: false});
  a.video.nativeHls = 'maybe';
  a.player.load(source());
  await settle();
  assert.equal(a.video.src, 'https://portico.test/v1/media/grant/1/master.m3u8');
  a.player.dispose();
  const b = setup({supported: false});
  b.player.load(source());
  await settle();
  const e = b.events.find(x => x.type === 'error');
  assert.ok(e && e.type === 'error' && e.code === 'unsupported');
  b.player.dispose();
});

test('the route to the server changing lets go of the media and comes back at the same position', async () => {
  const {video, instances, player} = setup();
  player.load(source({startPositionMs: 0}));
  await settle();
  video.ready(0);
  video.advance(321);
  player.suspend();
  assert.equal(instances[0]!.destroyed, true);
  assert.equal(player.observe().positionMs, 321_000, 'the position is kept, not the element’s zero');
  player.resume();
  await settle();
  assert.equal(instances[1]!.config.startPosition, 321);
  player.dispose();
});

test('cues are placed by the stream’s offset, and a hair’s difference between fragments does not move them', async () => {
  const offsets: (number | null)[] = [];
  let resets = 0;
  const {instances, player} = setup({subtitles: {reset: () => { resets++; }, timeline: o => { offsets.push(o); }}});
  player.load(source({startPositionMs: 0}));
  await settle();
  assert.deepEqual(offsets, [null], 'nothing is shown until the stream says where it sits');
  const hls = instances[0]!;
  hls.emit('hlsFragChanged', {frag: {start: 0, startPTS: 1.4}});
  hls.emit('hlsFragChanged', {frag: {start: 6, startPTS: 7.4004}});
  hls.emit('hlsFragChanged', {frag: {start: 12, startPTS: 13.409}});
  assert.deepEqual(offsets, [null, 1.4], 'sub-frame wobble is ignored');
  hls.emit('hlsFragChanged', {frag: {start: 18, startPTS: 20}});
  assert.equal(offsets.length, 3, 'a real change is followed');
  assert.ok(resets >= 1);
  player.dispose();
});

test('a live channel is positioned by the stream’s programme dates', async () => {
  const origin = Date.UTC(2026, 0, 1, 20, 0, 0);
  const {video, instances, player} = setup();
  player.load(source({startPositionMs: 600_000, liveOriginMs: origin}));
  await settle();
  const hls = instances[0]!;
  assert.equal(hls.config.startPosition, -1, 'a live stream joins at its edge');
  hls.engine.levels = [{details: {fragments: [{start: 100, duration: 6, programDateTime: origin + 598_000} as never, {start: 106, duration: 6, programDateTime: origin + 604_000} as never]}}];
  video.readyState = 4;
  video.seekableRanges = [[0, 400]];
  player.tick();
  assert.deepEqual(video.seeks, [102], 'ten minutes after the origin is two seconds into the first fragment');
  hls.engine.playingDate = new Date(origin + 600_000);
  video.land(102);
  assert.equal(player.observe().positionMs, 600_000);
  player.dispose();
});

test('a late event from the presentation before cannot speak for the one now loading', async () => {
  const {video, player, events} = setup();
  player.load(source({generation: 1, startPositionMs: 0}));
  await settle();
  video.ready(0);
  player.load(source({generation: 2, startPositionMs: 0}));
  const before = events.length;
  video.fire('ended');
  video.fire('playing');
  video.fire('pause');
  assert.equal(events.slice(before).some(e => e.type === 'state' && e.observation.state !== 'loading'), false);
  assert.equal(player.observe().generation, 2);
  player.dispose();
});

test('buffers are sized by bit rate: a high-rate stream holds fewer seconds than a page can afford', () => {
  assert.equal(bufferTargets(8000).forward, 30);
  assert.ok(bufferTargets(80_000).forward <= 12, 'an 80 Mbps remux is not given thirty seconds');
  assert.ok(bufferTargets(80_000).forward >= 6);
  assert.ok(bufferTargets(80_000).back <= bufferTargets(8000).back);
  assert.equal(bufferTargets(undefined).forward, 30);
  assert.ok(bufferTargets(8000, 2).forward <= 20, 'a small device buffers less');
});
