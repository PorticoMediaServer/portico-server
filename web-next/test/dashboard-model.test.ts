import {test} from 'node:test';
import assert from 'node:assert/strict';
import {chartPaths, orderedSessions, quietestHours, sessionTracks} from '../src/screens/dashboard/model.ts';
import type {DashboardSession} from '../src/screens/dashboard/bindings.ts';
const base: DashboardSession = {id: 'session', user: 'account', profile: 'Kids', device: 'TV', title: 'Actual title', kind: 'vod', state: 'playing', positionMs: 1, location: 'local', startedAt: '2026-10-23T01:00:00Z', decision: {}};
test('source/delivery actions use committed modes and unknown hardware remains absent', () => {
  const session = {...base, media: {video: {id: 'v', codec: 'hevc', height: 2160}}, delivery: {streams: [{kind: 'video', inputCodec: 'hevc', outputCodec: 'h264', action: 'convert'}]}};
  assert.deepEqual(sessionTracks(session), [{reportedAction: 'convert', route: 'converted', kind: 'video', source: '4K HEVC', action: 'transcode', output: 'H.264', hardware: false}]);
  assert.equal(sessionTracks({...session, media: {...session.media, videoHardware: true}})[0].hardware, true);
  assert.equal(sessionTracks({...session, delivery: {streams: [{kind: 'video', inputCodec: 'hevc', outputCodec: 'hevc', action: 'original'}]}})[0].action, 'direct');
});
test('problems outrank conversion and newest start, without changing input', () => {
  const old = {...base, id: 'old'}, converted = {...base, id: 'convert', decision: {audio: {action: 'transcode', reasons: []}}}, problem = {...base, id: 'problem', problem: 'buffering' as const};
  const input = [old, converted, problem]; assert.deepEqual(orderedSessions(input).map(s => s.id), ['problem', 'convert', 'old']); assert.equal(input[0].id, 'old');
});
test('telemetry gaps do not become zeroes or connected history', () => {
  const points = [{t: 0, v: 1}, {t: 2, v: 2}, {t: 4, v: 3}, {t: 30, v: 4}]; assert.equal(chartPaths(points, 0, 30, 5).length, 2); assert.match(chartPaths(points.slice(0, 2), 0, 30, 5, true)[0], /H.*V/); assert.deepEqual(chartPaths([], 0, 30, 5), []);
});
test('quiet window uses actual Monday-first grid and empty has no suggestion', () => {
  const grid = Array.from({length: 7}, () => Array(24).fill(10)); for (const day of grid) day.splice(4, 3, 0, 0, 0); assert.equal(quietestHours(grid), 4); assert.equal(quietestHours(grid.map(day => day.map(() => 0))), undefined);
});
