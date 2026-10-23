import {test} from 'node:test';
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import vm from 'node:vm';
import {fmtDur, fmtHrs, fmtMbps, fmtPct, formatAudioSource, formatDeliveryTarget, formatStreamRate, formatSubtitleSource, formatTrackRoute, formatVideoSource, loadScale, niceScale} from '../src/screens/dashboard/format.ts';
import {trackRoute, sessionTracks, clock, mediaKinds} from '../src/screens/dashboard/model.ts';
import type {DashboardSession} from '../src/screens/dashboard/bindings.ts';
const referenceRepo = process.env.PORTICO_DASHBOARD_DESIGN_REPO;
const reference = referenceRepo && execFileSync('git', ['-C', referenceRepo, 'show', '812dacacc70e296ac870ec36d1a37a485eaeb8ed:design/ui-v1/settings/dashboard.js'], {encoding: 'utf8'});
const session: DashboardSession = {id: 'source', user: 'person', profile: 'person', device: 'device', title: 'title', kind: 'vod', state: 'playing', positionMs: 0, location: 'local', decision: {}};
test('drawn source and delivery strings use actual selected and delivered facts', () => {
  assert.equal(formatVideoSource({id: 'v', height: 2160, codec: 'hevc', profile: 'Main 10', hdr: 'hdr10'}), '4K HEVC HDR10');
  assert.equal(formatVideoSource({id: 'v', height: 1080, codec: 'hevc', profile: 'Main 10'}), '1080p HEVC Main 10');
  assert.equal(formatVideoSource({id: 'v', height: 1080, codec: 'h264'}), '1080p H.264');
  const audio = {id: 'a', codec: 'truehd', language: 'en', layout: '7.1', channels: 8, atmos: false, commentary: false, default: true, visualImpaired: false};
  assert.equal(formatAudioSource(audio, false), 'English TrueHD 7.1');
  assert.equal(formatAudioSource({...audio, codec: 'aac', language: 'ja', layout: 'stereo', channels: 2}, false), 'Japanese AAC 2.0');
  assert.equal(formatAudioSource({...audio, codec: 'flac', bitDepth: 24, sampleRate: 96000}, true), 'FLAC 24-bit 96 kHz');
  assert.equal(formatSubtitleSource({id: 's', format: 'srt', language: 'en'}), 'English SRT');
  assert.equal(formatDeliveryTarget({...session, delivery: {streams: [], videoHeight: 1080}}, 'video', 'h264'), '1080p');
  assert.equal(formatDeliveryTarget({...session, delivery: {streams: [], audioChannels: 2}}, 'audio', 'aac'), 'AAC stereo');
});
test('absent source facts never become guessed language, quality, or multichannel layout', () => {
  assert.equal(formatVideoSource({id: 'v', codec: 'hevc'}), 'HEVC');
  assert.equal(formatSubtitleSource({id: 's', format: 'srt'}), 'SRT');
  assert.equal(formatAudioSource({id: 'a', codec: 'truehd', channels: 8, atmos: false, commentary: false, default: true, visualImpaired: false}, false), 'TrueHD');
  assert.equal(formatDeliveryTarget(session, 'video'), undefined);
  assert.equal(formatDeliveryTarget(session, 'video', 'h264'), 'H.264');
});
test('niceScale uses the drawing steps instead of fractional quarters of the data max', () => {
  assert.deepEqual(niceScale(20 * 1.06), {top: 30, ticks: [0, 10, 20, 30]});
  assert.deepEqual(niceScale(8), {top: 8, ticks: [0, 2, 4, 6, 8]});
  assert.deepEqual(niceScale(9), {top: 10, ticks: [0, 2.5, 5, 7.5, 10]});
  assert.deepEqual(loadScale('conversions', 3.2), {top: 4, ticks: [0, 1, 2, 3, 4]});
});
test('frame numeric formatter boundaries and cardinality match', () => {
  assert.equal(fmtMbps(8), '8.0 Mbps'); assert.equal(fmtMbps(100.4), '100 Mbps'); assert.equal(fmtPct(7), '7.0%');
  assert.equal(fmtDur(2 + 5 / 60), '2h 05m'); assert.equal(fmtDur(24 / 60), '24m'); assert.equal(fmtDur(100.4), '100 h');
  assert.equal(fmtHrs(10.4), '10 h'); assert.equal(fmtHrs(9.44), '9.4 h');
  assert.equal(formatStreamRate(8000), '8 Mbps'); assert.equal(formatStreamRate(9400), '9.4 Mbps'); assert.equal(formatStreamRate(128), '128 kbps');
  assert.equal(clock(184000), '3:04'); assert.equal(clock(7500000), '2:05:00');
});
test('VS-10 maps only known per-track decisions and preserves protocol identifiers', () => {
  const raw = {...session, media: {video: {id: 'v', codec: 'hevc', height: 2160}}, delivery: {videoHeight: 1080, streams: [{kind: 'video', action: 'convert', inputCodec: 'hevc', outputCodec: 'h264'}]}};
  const track = sessionTracks(raw)[0]; assert.equal(track.reportedAction, 'convert'); assert.equal(track.route, 'converted'); assert.equal(raw.delivery.streams[0].action, 'convert');
  for (const [protocol, display] of [['direct', 'original'], ['original', 'original'], ['copy', 'repackaged'], ['convert', 'converted'], ['transcode', 'converted'], ['burn', 'converted'], ['burn_in', 'converted']]) assert.equal(trackRoute(protocol), display);
  for (const action of [undefined, 'drop', 'unknown', 'future', 'sidecar', 'device']) assert.equal(trackRoute(action), undefined);
  assert.equal(formatTrackRoute(track), 'Converted to 1080p');
  assert.equal(formatTrackRoute({...track, route: 'original'}), 'Original');
  assert.equal(formatTrackRoute({...track, route: 'repackaged'}), 'Repackaged');
  assert.equal(formatTrackRoute({...track, kind: 'audio', output: 'AAC stereo'}), 'Converted to AAC stereo');
  assert.equal(formatTrackRoute({...track, kind: 'subtitles', action: 'burn'}), 'Burned in');
  assert.equal(formatTrackRoute({...track, route: undefined, reportedAction: 'drop', action: 'off'}), undefined);
  assert.equal(formatTrackRoute({...track, output: undefined}), 'Converted');
  assert.equal('route' in raw, false);
});
test('Top people retains the five drawn kinds in their drawn order', () => {
  assert.deepEqual(mediaKinds, ['movies', 'tv', 'music', 'audiobooks', 'live']);
});
test('formatter functions agree with the checked drawing over boundary values', {skip: !reference}, () => {
  const scaleSource = reference!.slice(reference!.indexOf('  function niceScale('), reference!.indexOf('  const f1 ='));
  const referenceScale = vm.runInNewContext(scaleSource + '; niceScale');
  for (const max of [0, .001, .2, 1, 2, 2.5, 5, 9.9, 10, 20, 21.2, 22, 99, 100, 1234]) assert.equal(JSON.stringify(niceScale(max)), JSON.stringify(referenceScale(max)));
  const a = reference!.indexOf('  const fmtMbps ='), b = reference!.indexOf('  const clamp =');
  const formatters = vm.runInNewContext(reference!.slice(a, b) + '; ({fmtMbps,fmtPct,fmtDur,fmtHrs})');
  for (const value of [0, .01, .1, .5, 1, 2.08333333333333, 9.44, 10.4, 99.9, 100, 1234]) for (const [name, format] of Object.entries({fmtMbps, fmtPct, fmtDur, fmtHrs})) assert.equal(format(value), formatters[name](value), `${name}(${value})`);
});
