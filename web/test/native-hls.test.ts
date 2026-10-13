import test from 'node:test';
import assert from 'node:assert/strict';
import {WebMediaPlayer} from '../src/playback-v1/web-media-player.ts';
import {nativeHlsPolicy} from '../src/playback-v1/native-hls.ts';
import type {PlayerEvent} from '@core/playback-v1/index.ts';
import {FakeVideo, fakeHls, manualClock, settle} from './helpers/fake-media.ts';

const resolveUrl = (u: string) => new URL(u, 'https://portico.test').href;
const stream = 'https://portico.test/v1/media/grant/1/master.m3u8';
const source = (over: Partial<Parameters<WebMediaPlayer['load']>[0]> = {}) => ({url: '/v1/media/grant/1/master.m3u8', generation: 1, mode: 'stream' as const, startPositionMs: 90_000, subtitles: [], autoplay: true, ...over});

function setup(nativeHls: 'first' | 'fallback' | 'never' | undefined, plays = 'maybe') {
  const video = new FakeVideo();
  video.nativeHls = plays;
  const {Hls, instances} = fakeHls(true);
  let loads = 0;
  const offsets: (number | null | 'reset')[] = [];
  const player = new WebMediaPlayer(video, {loadHls: async () => { loads++; return Hls; }, resolveUrl, timers: manualClock(), nativeHls, subtitles: {reset: () => offsets.push('reset'), timeline: o => offsets.push(o)}});
  const events: PlayerEvent[] = [];
  player.onEvent(e => events.push(e));
  return {video, instances, player, events, offsets, loads: () => loads};
}
const errors = (events: PlayerEvent[]) => events.filter(e => e.type === 'error');

test('which browsers use their own HLS player, and when', () => {
  const cases: {name: string; userAgent: string; mediaSource: boolean; want: string}[] = [
    {name: 'Safari on iPhone (ManagedMediaSource only)', userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Mobile/15E148 Safari/604.1', mediaSource: false, want: 'first'},
    {name: 'Chrome on iPhone is the same engine', userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/140.0.0.0 Mobile/15E148 Safari/604.1', mediaSource: false, want: 'first'},
    {name: 'Safari on a Mac', userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Safari/605.1.15', mediaSource: true, want: 'fallback'},
    {name: 'Safari on iPad', userAgent: 'Mozilla/5.0 (iPad; CPU OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Mobile/15E148 Safari/604.1', mediaSource: true, want: 'fallback'},
    {name: 'Chrome on a Mac', userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36', mediaSource: true, want: 'never'},
    {name: 'Edge', userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36 Edg/141.0.0.0', mediaSource: true, want: 'never'},
    {name: 'Chrome on Android, which now plays HLS itself', userAgent: 'Mozilla/5.0 (Linux; Android 15; Pixel 9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Mobile Safari/537.36', mediaSource: true, want: 'never'},
    {name: 'Firefox', userAgent: 'Mozilla/5.0 (X11; Linux x86_64; rv:143.0) Gecko/20100101 Firefox/143.0', mediaSource: true, want: 'never'},
  ];
  for (const c of cases) assert.equal(nativeHlsPolicy({userAgent: c.userAgent, mediaSource: c.mediaSource}), c.want, c.name);
});

test('iPhone: the system’s HLS player is used from the start and hls.js is never fetched', async () => {
  const {video, instances, player, offsets, loads} = setup('first');
  player.load(source());
  await settle();
  assert.equal(video.src, stream);
  assert.equal(loads(), 0);
  assert.equal(instances.length, 0);
  assert.equal(offsets.at(-1), 0, 'subtitle times are the element’s own');
  // A file played as it is has nothing to do with HLS either way.
  player.load(source({url: '/v1/media/grant/1', mode: 'direct', generation: 2}));
  await settle();
  assert.equal(video.src, 'https://portico.test/v1/media/grant/1');
  player.dispose();
});

test('Safari: hls.js first; a stream whose codecs MediaSource refuses goes to Safari’s player at the same position, unreported', async () => {
  const {video, instances, player, events} = setup('fallback');
  player.load(source());
  await settle();
  assert.equal(instances.length, 1, 'hls.js is the engine');
  assert.equal(video.src, '');
  instances[0]!.error({fatal: true, type: 'mediaError', details: 'manifestIncompatibleCodecsError'});
  await settle();
  assert.equal(instances[0]!.destroyed, true);
  assert.equal(instances[0]!.recoveries, 0, 'no pointless recovery attempts');
  assert.equal(video.src, stream);
  assert.equal(errors(events).length, 0, 'nothing failed as far as the server is concerned');
  assert.equal(player.observe().positionMs, 90_000);
  assert.equal(instances.length, 1, 'and hls.js is not tried again for this load');
  player.dispose();
});

test('Safari: a decoding error survives two recoveries, then Safari’s player; a new title starts with hls.js again', async () => {
  const {video, instances, player, events} = setup('fallback');
  player.load(source());
  await settle();
  const hls = instances[0]!;
  hls.error({fatal: true, type: 'mediaError', details: 'bufferAppendError'});
  hls.error({fatal: true, type: 'mediaError', details: 'bufferAppendError'});
  assert.equal(hls.recoveries, 2);
  assert.equal(video.src, '');
  hls.error({fatal: true, type: 'mediaError', details: 'bufferAppendError'});
  await settle();
  assert.equal(video.src, stream);
  assert.equal(errors(events).length, 0);
  player.load(source({generation: 2}));
  await settle();
  assert.equal(instances.length, 2, 'the next load is hls.js first again');
  player.dispose();
});

test('everywhere else nothing changes: no native player, and the failure is reported as before', async () => {
  for (const policy of ['never', undefined] as const) {
    const {video, instances, player, events} = setup(policy);
    player.load(source());
    await settle();
    instances[0]!.error({fatal: true, type: 'mediaError', details: 'manifestIncompatibleCodecsError'});
    instances[0]!.error({fatal: true, type: 'mediaError', details: 'manifestIncompatibleCodecsError'});
    instances[0]!.error({fatal: true, type: 'mediaError', details: 'manifestIncompatibleCodecsError'});
    await settle();
    assert.equal(video.src, '');
    assert.equal(errors(events).length, 1);
    player.dispose();
  }
  // A browser the policy names that does not in fact play HLS: hls.js, and its failures are failures.
  const {video, instances, player, events} = setup('fallback', '');
  player.load(source());
  await settle();
  for (let i = 0; i < 3; i++) instances[0]!.error({fatal: true, type: 'mediaError', details: 'bufferAppendError'});
  await settle();
  assert.equal(video.src, '');
  assert.equal(errors(events).length, 1);
  player.dispose();
});
