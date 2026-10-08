import test from 'node:test';
import assert from 'node:assert/strict';
import {NOW_PLAYING_POLL_MS, deliveryId, driveNowPlaying, locationId, rateMbps, stateId, stopMessage, windowRange} from '../src/admin/now-playing.ts';
import {ownerStopOf} from '../src/player/owner-stop.ts';
import {V1PlaybackApi} from '../../packages/client-core/src/playback-v1/legacy-bridge.ts';
import {AdminSessionsClient, NowPlayingStore} from '../../packages/client-core/src/playback-v1/admin-sessions.ts';

function fakeEnv() {
  const timers: {fn: () => void; ms: number}[] = [];
  let hidden = false; const listeners = new Set<() => void>();
  return {
    timers, setHidden(h: boolean) { hidden = h; for (const fn of [...listeners]) fn(); },
    fire() { const t = timers.shift(); t?.fn(); return t?.ms; },
    env: {setTimer: (fn: () => void, ms: number) => { const t = {fn, ms}; timers.push(t); return t; }, clearTimer: (t: unknown) => { const i = timers.indexOf(t as never); if (i >= 0) timers.splice(i, 1); }, hidden: () => hidden, onVisibility: (fn: () => void) => { listeners.add(fn); return () => listeners.delete(fn); }},
  };
}

function fakeEvents(connected: boolean) {
  const on = new Map<string, Set<() => void>>(); const status = new Set<() => void>(); const resync = new Set<() => void>();
  let state = {connected};
  return {
    emit(type: string) { for (const fn of on.get(type) ?? []) fn(); },
    resync() { for (const fn of resync) fn(); },
    setConnected(c: boolean) { state = {connected: c}; for (const fn of status) fn(); },
    cursorMoved() { for (const fn of status) fn(); },
    counts: () => ({on: [...on.values()].reduce((n, s) => n + s.size, 0), status: status.size}),
    client: {
      on(type: string, fn: () => void) { if (!on.has(type)) on.set(type, new Set()); on.get(type)!.add(fn); return () => { on.get(type)!.delete(fn); }; },
      onResync(fn: () => void) { resync.add(fn); return () => { resync.delete(fn); }; },
      getStatus: () => state,
      subscribeStatus(fn: () => void) { status.add(fn); return () => { status.delete(fn); }; },
    },
  };
}

test('without an event feed the list reads now, then every 10 s while the tab is visible', () => {
  const f = fakeEnv(); let reads = 0;
  const stop = driveNowPlaying(() => { reads++; }, undefined, f.env);
  assert.equal(reads, 1);
  assert.equal(f.fire(), NOW_PLAYING_POLL_MS);
  assert.equal(f.fire(), NOW_PLAYING_POLL_MS);
  assert.equal(reads, 3);
  f.setHidden(true);
  assert.equal(f.timers.length, 0, 'no timer while hidden');
  f.setHidden(false);
  assert.equal(reads, 4, 'shown again: one read straight away');
  assert.equal(f.timers.length, 1);
  stop();
  assert.equal(f.timers.length, 0);
});

test('with a connected feed it reads on admin.sessions, session.updated and resync, and never polls', () => {
  const f = fakeEnv(), e = fakeEvents(true); let reads = 0;
  const stop = driveNowPlaying(() => { reads++; }, e.client, f.env);
  assert.equal(reads, 1);
  assert.equal(f.timers.length, 0);
  e.emit('admin.sessions'); e.emit('session.updated'); e.resync(); e.emit('queue.updated');
  assert.equal(reads, 4);
  e.cursorMoved();
  assert.equal(f.timers.length, 0, 'a cursor move is not a connection change');
  f.setHidden(true); f.setHidden(false);
  assert.equal(reads, 4, 'events keep it current; showing the tab does not re-read');
  stop();
  assert.deepEqual(e.counts(), {on: 0, status: 0});
});

test('when the feed drops it falls back to 10 s polling, and stops polling when it reconnects', () => {
  const f = fakeEnv(), e = fakeEvents(true); let reads = 0;
  driveNowPlaying(() => { reads++; }, e.client, f.env);
  e.setConnected(false);
  assert.equal(f.timers.length, 1);
  assert.equal(f.fire(), NOW_PLAYING_POLL_MS);
  assert.equal(reads, 2);
  e.setConnected(true);
  assert.equal(f.timers.length, 0);
});

test('the store reads v1 sessions and Stop playback sends the trimmed message', async () => {
  const calls: {method: string; path: string; body?: unknown}[] = [];
  const session = {id: 'ses_1', user: {id: 'a', name: 'justin'}, profile: {id: 'p', name: 'Kids'}, device: {id: 'd', name: 'Living room TV'}, item: {id: 'itm_1', title: 'Arrival'}, kind: 'vod', state: 'playing', positionMs: 1000, decision: {video: {action: 'transcode', reasons: []}, audio: {action: 'copy', reasons: []}}, bitrateKbps: 8000, bandwidthKbps: 12400, location: 'remote', startedAt: '2026-09-23T10:00:00Z'};
  let listed = [session];
  const http = {send: async (r: {method: string; path: string; body?: unknown}) => {
    calls.push({method: r.method, path: r.path, body: r.body});
    if (r.method === 'GET') return {status: 200, headers: {}, body: {items: listed, page: {limit: 50, total: listed.length}}};
    listed = [];
    return {status: 204, headers: {}};
  }};
  const store = new NowPlayingStore(new AdminSessionsClient(http as never, () => 'key-1'));
  await store.refresh();
  const [s] = store.getSnapshot().sessions;
  assert.equal(s!.title, 'Arrival');
  assert.equal(s!.profile, 'Kids');
  assert.equal(deliveryId(s!.decision), 'web.nowPlaying.delivery.convertVideo');
  assert.equal(locationId(s!.location), 'web.nowPlaying.location.remote');
  assert.equal(rateMbps(s!), '12');
  await store.terminate('ses_1', stopMessage('  The server is restarting in five minutes.  '));
  const post = calls.find(c => c.method === 'POST')!;
  assert.equal(post.path, '/v1/admin/sessions/ses_1:terminate');
  assert.deepEqual(post.body, {message: 'The server is restarting in five minutes.'});
  assert.equal(store.getSnapshot().sessions.length, 0);
  await store.terminate('ses_2', stopMessage('   '));
  assert.deepEqual(calls.filter(c => c.method === 'POST')[1]!.body, {}, 'no message when left empty');
});

test('display helpers: delivery, state, rate, message limit, window', () => {
  assert.equal(deliveryId({audio: {action: 'transcode', reasons: []}}), 'web.nowPlaying.delivery.convertAudio');
  assert.equal(deliveryId({video: {action: 'copy', reasons: []}}), 'web.nowPlaying.delivery.directStream');
  assert.equal(deliveryId({video: {action: 'direct', reasons: []}, subtitles: {action: 'burn', reasons: []}}), 'web.nowPlaying.delivery.convertVideo');
  assert.equal(deliveryId({video: {action: 'direct', reasons: []}}), 'web.nowPlaying.delivery.directPlay');
  assert.equal(deliveryId({}), undefined);
  assert.equal(stateId('paused'), 'web.nowPlaying.state.paused');
  assert.equal(stateId('weird'), undefined);
  assert.equal(locationId('other'), undefined);
  assert.equal(rateMbps({bitrateKbps: 3200}), '3.2');
  assert.equal(rateMbps({}), undefined);
  assert.equal([...stopMessage('é'.repeat(600))!].length, 500);
  assert.deepEqual(windowRange(0, 476, 0), {first: 0, last: -1});
  assert.deepEqual(windowRange(0, 476, 3), {first: 0, last: 2});
  // 10,000 sessions: a scrolled window renders about a screenful, not the list.
  const w = windowRange(68 * 5000, 476, 10_000);
  assert.deepEqual(w, {first: 4997, last: 5010});
});

test('the server’s session.updated payload reaches core as an owner’s stop with the message', async () => {
  const http = {send: async () => ({status: 204, headers: {}})};
  const timers = {setTimer: () => 0, clearTimer: () => {}};
  const api = new V1PlaybackApi(http as never, {timers});
  await api.progressPlayback('ses_1', {generation: 1, sequence: 1, positionSeconds: 12, state: 'playing'});
  const ends: unknown[] = [];
  api.onServerEnded((id, end) => ends.push({id, end}));
  api.sessionUpdated('ses_2', {state: 'ended', reason: 'terminated'});
  api.sessionUpdated('ses_1', {state: 'paused', deviceId: 'dev_1', generation: 1});
  assert.equal(ends.length, 0, 'another session, or one still going, is not an end');
  // The shape be/playback c85d526 publishes to the device (sessionEventsTx).
  api.sessionUpdated('ses_1', {state: 'ended', deviceId: 'dev_1', generation: 1, reason: 'terminated', message: 'The server is restarting in five minutes.'});
  assert.deepEqual(ends, [{id: 'ses_1', end: {reason: 'terminated', message: 'The server is restarting in five minutes.'}}]);
});

test('the player shows the owner’s stop from snapshot.ended, with or without a message', () => {
  assert.deepEqual(ownerStopOf({reason: 'terminated', message: '  Restarting soon.  '}), {message: 'Restarting soon.'});
  assert.deepEqual(ownerStopOf({reason: 'terminated'}), {});
  assert.deepEqual(ownerStopOf({reason: 'terminated', message: '   '}), {}, 'a blank message is no message');
  assert.equal(ownerStopOf({reason: 'transferred'}), undefined, 'other ends use the ordinary stopped card');
  assert.equal(ownerStopOf(undefined), undefined);
  assert.equal(ownerStopOf({reason: 'terminated', message: 'x'.repeat(900)})!.message!.length, 500);
  assert.equal(ownerStopOf({reason: 'terminated', message: 'one\u0007two'})!.message, 'one two', 'control characters removed');
});
