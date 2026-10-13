/**
 * The web's one playback path, driven the way a browser drives it: the real playback service and
 * queue player, the real session controller and media element port, a fake server, a fake
 * `<video>` and a fake hls.js. These are the behaviours the older adapter got wrong or had no
 * test for.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {PlaybackService} from '@core/index.ts';
import {FakePlaybackServer, fakeHttp} from '@core/playback-v1/testing/fake-server.ts';
import {V1QueuePlayer} from '@core/playback-v1/v1-queue-player.ts';
import {queueItemInput} from '@core/queue-controller.ts';
import type {V1Http} from '@core/playback-v1/http.ts';
import {WebPlayback, failure, type WebPlaybackNotice} from '../src/playback-v1/web-playback.ts';
import {playbackFailure} from '../src/player/failure.ts';
import {FakeVideo, fakeHls, manualClock, settle} from './helpers/fake-media.ts';

const viewer = {serverId: 'server', authority: 'local' as const, accountId: 'account', profileId: 'profile'};
type Logged = {method: string; path: string; body?: unknown};

function setup(ctx: {after(fn: () => void): void}, options: {escalates?: boolean; mount?: boolean; hold?: boolean} = {}) {
  const clock = manualClock();
  const server = new FakePlaybackServer({now: clock.now});
  const inner = fakeHttp(server);
  const log: Logged[] = [], farewells: Logged[] = [];
  const http: V1Http = {send: async r => {
    log.push({method: r.method, path: r.path, body: r.body});
    if (r.path === '/v1/playback/route-failures') return {status: 200, headers: {}, body: {route: 'direct', escalates: options.escalates ?? true}};
    return inner.send(r as never) as never;
  }};
  let serial = 0;
  const {Hls, instances} = fakeHls();
  const web = new WebPlayback({
    http, mediaUrl: p => new URL(p, 'https://portico.test').href, loadHls: async () => Hls, key: () => `key-${++serial}-0000000000`,
    timers: {now: clock.now, setTimer: clock.setTimer, clearTimer: clock.clearTimer}, playerTimers: clock,
    farewell: (path, method, body) => { farewells.push({path, method, body}); },
  });
  const playback = new PlaybackService(web.transport, () => `r${++serial}`, {queueRequired: true, startupTimeoutMs: 60_000, seekTimeoutMs: 60_000});
  const player = new V1QueuePlayer({http, api: web.transport, playback, viewer, key: () => `q${++serial}-key-000000`, pendingPollMs: 10});
  if (options.hold !== false) player.setCompletionPolicy(() => 'hold');
  const unbind = web.bind(playback, player);
  const stop = player.connect();
  const video = new FakeVideo();
  const unmount = options.mount === false ? () => {} : web.mount(video);
  const notices: WebPlaybackNotice[] = [];
  web.onNotice(n => notices.push(n));
  // Whatever a test asserts, its timers and listeners go with it.
  ctx.after(() => { playback.leave(); stop(); unmount(); unbind(); web.dispose(); });
  /** The element has the media the newest engine asked for, at the position it was asked for. */
  const arrive = async () => {
    await settle();
    const hls = instances.at(-1)!;
    const at = Number(hls.config.startPosition);
    video.ready(at >= 0 ? at : 0);
    await settle();
    return hls;
  };
  const sessions = () => log.filter(r => r.method === 'POST' && r.path === '/v1/playback/sessions');
  const queues = () => log.filter(r => r.method === 'POST' && r.path === '/v1/queues');
  return {clock, server, log, farewells, web, playback, player, video, instances, notices, arrive, sessions, queues};
}

test('a start loads where the server says to resume, plays, and reports a full timeline', async ctx => {
  const t = setup(ctx);
  await t.player.playItem('movie', 100);
  const hls = await t.arrive();
  assert.equal(hls.config.startPosition, 100, 'loading begins at the start position');
  assert.equal(t.video.seeks.length, 0);
  const s = t.playback.getSnapshot();
  assert.equal(s.phase, 'ready');
  assert.equal(s.pendingSeek, undefined, 'the start is acknowledged once the picture is there');
  assert.equal(s.positionSeconds, 100);
  t.video.advance(104);
  await settle();
  const reports = t.server.session(s.session!.id)!.reports;
  assert.ok(reports.length >= 1);
  const last = reports.at(-1)!;
  assert.equal(last.generation, s.session!.generation);
  assert.equal(typeof last.rate, 'number');
  assert.ok('volume' in last && 'muted' in last, 'the report carries what the owner’s Now Playing shows');
  assert.ok(reports.every(r => (r.positionMs as number) >= 100_000), 'no position before the resume point is ever reported');
  assert.equal(t.log.some(r => r.path.endsWith('/progress')), false, 'only the protocol’s timeline is spoken');
});

test('a session adopted before the player screen exists loads when the element is mounted', async ctx => {
  const t = setup(ctx, {mount: false});
  await t.player.playItem('movie', 120);
  await settle();
  assert.equal(t.instances.length, 0);
  assert.equal(t.playback.getSnapshot().phase, 'starting');
  const unmount = t.web.mount(t.video);
  await t.arrive();
  assert.equal(t.instances.at(-1)!.config.startPosition, 120);
  assert.equal(t.playback.getSnapshot().phase, 'ready');
  unmount();
});

test('changing quality re-plans the playing session where it is and keeps the queue', async ctx => {
  const t = setup(ctx);
  await t.player.playEntries(['e1', 'e2', 'e3'].map(queueItemInput));
  await t.arrive();
  t.video.advance(132);
  await settle();
  const before = t.playback.getSnapshot(), queue = t.player.getSnapshot().view!.queue;
  await t.playback.changePlayback({quality: {mode: 'limit', maxHeight: 720, maxVideoBitrateKbps: 4000}});
  const patch = t.log.find(r => r.method === 'PATCH' && r.path === `/v1/playback/sessions/${before.session!.id}`);
  assert.deepEqual((patch!.body as {quality?: unknown}).quality, {mode: 'limit', maxHeight: 720, maxVideoBitrateKbps: 4000}, 'the choice reaches the server');
  assert.equal(t.queues().length, 1, 'no new queue was made');
  const hls = await t.arrive();
  const after = t.playback.getSnapshot();
  assert.equal(after.session!.id, before.session!.id, 'the same session');
  assert.equal(after.session!.generation, before.session!.generation + 1, 'a new presentation');
  assert.match(hls.source!, new RegExp(`/${after.session!.generation}/master\\.m3u8$`));
  assert.equal(hls.config.startPosition, 132, 'the new stream starts where the picture was');
  assert.equal(after.phase, 'ready');
  assert.equal(after.pendingSeek, undefined);
  const view = t.player.getSnapshot().view!;
  assert.equal(view.queue.id, queue.id);
  assert.equal(view.queue.entries.length, 3);
  assert.equal(view.next.available, true, 'Up next still has the next episode');
  assert.equal(t.web.session(after.session!.id)!.presentation.decision.video!.action, 'transcode');
});

test('an audio track choice re-plans the session too, and the menu ticks what was committed', async ctx => {
  const t = setup(ctx);
  await t.player.playEntries(['e1', 'e2'].map(queueItemInput));
  await t.arrive();
  const s = t.playback.getSnapshot();
  t.playback.installAudioTracks(s.intentId, [{streamIndex: 1, label: 'English', codec: 'ac3'}, {streamIndex: 2, label: 'French', codec: 'aac'}], 1);
  const picked = t.playback.selectAudioTrack(2);
  assert.equal(t.playback.getSnapshot().audioTracks!.pending, 2);
  await picked;
  const patch = t.log.find(r => r.method === 'PATCH');
  assert.deepEqual((patch!.body as {audio?: unknown}).audio, {trackId: 'a2'});
  assert.equal(t.queues().length, 1);
  assert.equal(t.playback.getSnapshot().audioTracks?.selected, 2);
  assert.equal(t.playback.getSnapshot().audioTracks?.pending, undefined);
});

test('Replay starts the same queue entry again; the rest of the queue stays', async ctx => {
  const t = setup(ctx);
  await t.player.playEntries(['e1', 'e2', 'e3'].map(queueItemInput));
  await t.arrive();
  const first = t.playback.getSnapshot().session!.id, queue = t.player.getSnapshot().view!.queue.id;
  t.video.finish();
  await settle();
  assert.equal(t.playback.getSnapshot().phase, 'ended');
  assert.ok(t.player.getSnapshot().completion, 'the end is held for the post-play card');
  t.playback.resume();
  await settle();
  const again = t.playback.getSnapshot();
  assert.notEqual(again.session!.id, first);
  assert.equal(again.itemId, 'e1');
  assert.equal(t.queues().length, 1, 'no one-title queue replaced the season');
  const start = t.sessions().at(-1)!.body as {queue?: {queueId: string}; startPositionMs?: number};
  assert.equal(start.queue!.queueId, queue);
  assert.equal(start.startPositionMs, 0);
  assert.equal(t.player.getSnapshot().view!.next.available, true);
});

test('the end of a title reaches the server before the queue moves on', async ctx => {
  const t = setup(ctx, {hold: false});
  await t.player.playEntries(['e1', 'e2'].map(queueItemInput));
  await t.arrive();
  const first = t.playback.getSnapshot().session!.id;
  t.video.finish();
  await settle();
  await settle();
  assert.equal(t.server.session(first)!.reports.at(-1)!.state, 'ended');
  const now = t.playback.getSnapshot();
  assert.equal(now.itemId, 'e2', 'the queue advanced by itself');
  assert.notEqual(now.session!.id, first);
  assert.equal(now.phase, 'starting');
  await t.arrive();
  assert.equal(t.playback.getSnapshot().phase, 'ready');
});

test('a pause from a media key or the Picture in Picture window stays paused', async ctx => {
  const t = setup(ctx);
  await t.player.playItem('movie', 0);
  await t.arrive();
  assert.equal(t.playback.getSnapshot().intent, 'playing');
  t.video.pause(); // not through Portico's own button
  assert.equal(t.playback.getSnapshot().intent, 'paused', 'the viewer’s pause is their intent');
  // Later ticks and snapshot updates do not start it again.
  t.video.fire('timeupdate');
  t.playback.audioEffectsNotice('anything');
  await settle();
  assert.equal(t.video.paused, true);
  // Portico's own Play resumes.
  t.playback.resume();
  await settle();
  assert.equal(t.video.paused, false);
  // A play from outside is intent as well.
  t.playback.pause();
  await settle();
  assert.equal(t.video.paused, true);
  void t.video.play();
  assert.equal(t.playback.getSnapshot().intent, 'playing');
});

test('a browser that refuses to start without a gesture leaves a paused player and says so', async ctx => {
  const t = setup(ctx);
  t.video.refusePlay = true;
  await t.player.playItem('movie', 0);
  await t.arrive();
  assert.deepEqual(t.notices, ['autoplayBlocked']);
  assert.equal(t.playback.getSnapshot().intent, 'paused');
  assert.equal(t.playback.getSnapshot().phase, 'ready');
});

test('a lease that lapsed while the laptop slept starts the title again where it was, with no error', async ctx => {
  const t = setup(ctx);
  await t.player.playEntries(['e1', 'e2'].map(queueItemInput));
  await t.arrive();
  t.video.advance(150);
  await settle();
  const asleep = t.playback.getSnapshot().session!.id;
  // Five minutes pass with nothing sent; the server ends the session for its lease.
  t.clock.sleep(300_000);
  t.clock.advance(1); // awake: the overdue report is sent, and finds the session gone
  await settle();
  await settle();
  const now = t.playback.getSnapshot();
  assert.notEqual(now.phase, 'error', 'no error card');
  assert.notEqual(now.session!.id, asleep, 'a new session');
  assert.equal(now.itemId, 'e1');
  const start = t.sessions().at(-1)!.body as {queue?: unknown; startPositionMs?: number};
  assert.ok(start.queue, 'the same queue entry');
  assert.equal(start.startPositionMs, 150_000, 'at the last position');
  assert.equal(t.queues().length, 1);
  const hls = await t.arrive();
  assert.equal(hls.config.startPosition, 150);
  assert.equal(t.playback.getSnapshot().phase, 'ready');
  assert.equal(t.playback.getSnapshot().recovering, undefined);
});

test('an owner’s stop is final and carries their message; nothing is restarted', async ctx => {
  const t = setup(ctx);
  await t.player.playItem('movie', 0);
  await t.arrive();
  const id = t.playback.getSnapshot().session!.id;
  t.web.transport.sessionUpdated(id, '99', {state: 'ended', reason: 'terminated', message: 'Maintenance tonight.'});
  await settle();
  const s = t.playback.getSnapshot();
  assert.equal(s.phase, 'error');
  assert.deepEqual(s.ended, {reason: 'terminated', message: 'Maintenance tonight.'});
  assert.equal(t.sessions().length, 1, 'no new session');
  // This device's own queue move is not news to it, even if the notice overtakes the answer.
  const u = setup(ctx);
  await u.player.playItem('movie', 0);
  await u.arrive();
  u.web.transport.sessionUpdated(u.playback.getSnapshot().session!.id, '99', {state: 'ended', reason: 'replaced'});
  await settle();
  assert.equal(u.playback.getSnapshot().phase, 'ready');
});

test('an engine failure is reported to the server and the title comes back on another route, in the same queue', async ctx => {
  const t = setup(ctx, {escalates: true});
  await t.player.playEntries(['e1', 'e2'].map(queueItemInput));
  const hls = await t.arrive();
  t.video.advance(60);
  await settle();
  const failed = t.playback.getSnapshot().session!.id;
  for (let i = 0; i < 3; i++) hls.error({fatal: true, type: 'mediaError', details: 'bufferAppendError'});
  await settle();
  const report = t.log.find(r => r.path === '/v1/playback/route-failures');
  assert.ok(report, 'the server is told which route failed');
  assert.deepEqual({sessionId: (report!.body as {sessionId: string}).sessionId, code: (report!.body as {code: string}).code}, {sessionId: failed, code: 'decode_error'});
  const now = t.playback.getSnapshot();
  assert.notEqual(now.phase, 'error', 'the viewer sees recovery, not an error');
  assert.notEqual(now.session!.id, failed);
  assert.equal(t.queues().length, 1, 'the queue was not replaced');
  assert.equal((t.sessions().at(-1)!.body as {startPositionMs?: number}).startPositionMs, 60_000);

  // With no route above it, the viewer is told plainly that it cannot be played here.
  const u = setup(ctx, {escalates: false});
  await u.player.playItem('movie', 0);
  const engine = await u.arrive();
  for (let i = 0; i < 3; i++) engine.error({fatal: true, type: 'mediaError', details: 'bufferAppendError'});
  await settle();
  const s = u.playback.getSnapshot();
  assert.equal(s.phase, 'error');
  assert.equal(s.error, failure.cannotPlay);
  const card = playbackFailure(s.error);
  assert.equal(card.permanent, true);
  assert.equal(card.title, 'web.player.error.cannotPlayTitle');
  assert.equal(card.body, 'web.player.error.cannotPlay');
});

test('a server that stops answering is a failure another try can fix, and is worded that way', async ctx => {
  const t = setup(ctx);
  await t.player.playItem('movie', 0);
  const hls = await t.arrive();
  hls.error({fatal: true, type: 'networkError', response: {code: 410}});
  await settle();
  const s = t.playback.getSnapshot();
  assert.equal(s.phase, 'error');
  assert.equal(s.error, failure.streamGone);
  const card = playbackFailure(s.error);
  assert.equal(card.permanent, false);
  assert.equal(card.title, 'web.player.error.stoppedTitle');
  assert.equal(playbackFailure('This title can’t be played on this device.').permanent, true);
  assert.equal(playbackFailure('The stream could not connect.').permanent, false);
});

test('two quick presses of Next move on twice instead of colliding', async ctx => {
  const t = setup(ctx);
  await t.player.playEntries(['e1', 'e2', 'e3', 'e4'].map(queueItemInput));
  await t.arrive();
  const q = t.player.getSnapshot().view!.queue;
  const a = t.player.next(q.id, q.revision), b = t.player.next(q.id, q.revision);
  await Promise.all([a, b]);
  await settle();
  const s = t.playback.getSnapshot();
  assert.equal(s.itemId, 'e3');
  assert.notEqual(s.phase, 'error');
  assert.equal(t.log.filter(r => r.path.endsWith(':advance')).length, 2);
  // Only the session that is playing is alive on the server.
  const live = (t.server as unknown as {sessions: Map<string, {ended: boolean; id: string}>}).sessions;
  assert.deepEqual([...live.values()].filter(x => !x.ended).map(x => x.id), [s.session!.id]);
});

test('closing the tab ends the session with its position; hiding it only reports', async ctx => {
  const t = setup(ctx);
  await t.player.playItem('movie', 0);
  await t.arrive();
  t.video.advance(77);
  await settle();
  const id = t.playback.getSnapshot().session!.id;
  t.web.pageHidden(false);
  assert.equal(t.farewells.length, 0, 'a hidden tab keeps its session');
  t.web.pageHidden(true);
  assert.deepEqual(t.farewells, [{path: `/v1/playback/sessions/${id}`, method: 'DELETE', body: {positionMs: 77_000}}]);
});

test('leaving stops the session at the last position', async ctx => {
  const t = setup(ctx);
  await t.player.playItem('movie', 0);
  await t.arrive();
  t.video.advance(55);
  await settle();
  const id = t.playback.getSnapshot().session!.id;
  t.playback.leave();
  await settle();
  const stop = t.log.find(r => r.method === 'DELETE' && r.path === `/v1/playback/sessions/${id}`);
  assert.deepEqual(stop!.body, {positionMs: 55_000});
  assert.equal(t.instances.at(-1)!.destroyed, true, 'the media is let go');
});

test('Automatic steps down on a sustained shortfall, says so, and asks the server for the lower quality', async ctx => {
  const t = setup(ctx);
  await t.player.playEntries(['e1', 'e2'].map(queueItemInput));
  const hls = await t.arrive();
  t.web.setAutomatic({steps: [{id: '1080', height: 1080, videoKbps: 8000}, {id: '720', height: 720, videoKbps: 4000}, {id: '480', height: 480, videoKbps: 1500}], originalKbps: 20_000});
  assert.equal(t.web.automaticOn, true);
  assert.equal(t.web.automaticAt, undefined, 'it starts at Original');
  // Twelve seconds of fragments arriving at about 5 Mbps against a 20 Mbps original.
  for (let i = 0; i < 12; i++) {
    const frag = hls.fragment(i * 6);
    frag.stats.loaded = 625_000;
    hls.loaded(frag, 1000);
    t.clock.advance(1000);
    t.video.advance(1 + i);
  }
  await settle();
  assert.equal(t.web.automaticAt?.id, '720', 'the best step the connection carries with room to spare');
  assert.deepEqual(t.notices, ['qualityLowered']);
  const patch = t.log.find(r => r.method === 'PATCH');
  assert.deepEqual((patch!.body as {quality: unknown}).quality, {mode: 'limit', maxHeight: 720, maxVideoBitrateKbps: 4000});
  assert.equal(t.queues().length, 1);
});
