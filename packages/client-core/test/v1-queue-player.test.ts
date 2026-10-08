import test from 'node:test';
import assert from 'node:assert/strict';
import {PlaybackService} from '../src/index.ts';
import {FakePlaybackServer, fakeHttp} from '../src/playback-v1/testing/fake-server.ts';
import {V1PlaybackApi, toPlaybackSession} from '../src/playback-v1/legacy-bridge.ts';
import {V1QueuePlayer, movedEntry, playbackV1ForKind, serverSupportsPlaybackV1, serverSupportsQueueTransitions} from '../src/playback-v1/v1-queue-player.ts';
import {queueItemInput} from '../src/queue-controller.ts';
import {QueueClient} from '../src/playback-v1/queue.ts';
import {playbackUserMessage, sessionEndMessage} from '../src/playback-messages.ts';
import type {V1Http} from '../src/playback-v1/http.ts';

const viewer = {serverId: 'server', authority: 'local' as const, accountId: 'account', profileId: 'profile'};

function setup(options: {log?: {method: string; path: string; body?: unknown}[]; server?: ConstructorParameters<typeof FakePlaybackServer>[0]} = {}) {
  const server = new FakePlaybackServer(options.server);
  const inner = fakeHttp(server);
  const log = options.log ?? [];
  const http: V1Http = {send: async r => { log.push({method: r.method, path: r.path, body: r.body}); return inner.send(r as never) as never; }};
  const timers: {fn: () => void; ms: number}[] = [];
  let serial = 0;
  const api = new V1PlaybackApi(http, {timers: {setTimer: (fn, ms) => { timers.push({fn, ms}); return timers.length; }, clearTimer: () => {}}, key: () => `k${++serial}`});
  const playback = new PlaybackService(api, () => `r${++serial}`, {});
  const player = new V1QueuePlayer({http, api, playback, viewer, key: () => `q${++serial}-key-000000`, pendingPollMs: 10});
  const stop = player.connect();
  return {server, api, playback, player, log, timers, stop};
}

/** The engine plays the adopted session to its end. */
function playToEnd(playback: PlaybackService) {
  const s = playback.getSnapshot();
  playback.ready(s.intentId);
  if (s.pendingSeek) playback.seekApplied(s.intentId, s.pendingSeek.revision, s.pendingSeek.positionSeconds, 'playing');
  const now = playback.getSnapshot();
  playback.fact(now.intentId, now.duration, 'ended');
}

const settle = () => new Promise(r => setTimeout(r, 20));

test('Play is one request: the queue is created with its first session, which the service plays', async () => {
  const {player, playback, log, stop} = setup();
  await player.playEntries(['a', 'b', 'c'].map(queueItemInput));
  const creates = log.filter(r => r.method === 'POST' && r.path === '/v1/queues');
  assert.equal(creates.length, 1);
  assert.deepEqual((creates[0]!.body as {startPlayback?: unknown}).startPlayback, {state: 'playing'});
  assert.equal(log.filter(r => r.method === 'POST' && r.path === '/v1/playback/sessions').length, 0, 'no separate start');
  const view = player.getSnapshot().view!;
  assert.equal(view.queue.entries.length, 3);
  assert.equal(view.next.available, true);
  assert.equal(playback.getSnapshot().session?.id, view.queue.currentPlaybackId);
  assert.equal(playback.getSnapshot().itemId, 'a');
  stop();
});

test('an explicit start position starts the entry separately, at that position', async () => {
  const {player, playback, log, stop} = setup();
  await player.playItem('movie', 42);
  const create = log.find(r => r.method === 'POST' && r.path === '/v1/queues')!;
  assert.equal((create.body as {startPlayback?: unknown}).startPlayback, undefined);
  const start = log.find(r => r.method === 'POST' && r.path === '/v1/playback/sessions')!;
  assert.equal((start.body as {startPositionMs?: number}).startPositionMs, 42_000);
  assert.ok((start.body as {queue?: unknown}).queue);
  assert.equal(playback.getSnapshot().session?.resumeSeconds, 42);
  stop();
});

test('Next advances on the server, which starts the next session and ends the old one', async () => {
  const {player, playback, server, stop} = setup();
  await player.playEntries(['a', 'b'].map(queueItemInput));
  const first = playback.getSnapshot().session!.id;
  const q = player.getSnapshot().view!.queue;
  await player.next(q.id, q.revision);
  const second = playback.getSnapshot().session!;
  assert.notEqual(second.id, first);
  assert.equal(playback.getSnapshot().itemId, 'b');
  assert.equal((server as unknown as {sessions: Map<string, {ended?: unknown}>}).sessions.get(first)?.ended !== undefined, true, 'the replaced session ended');
  assert.equal(player.getSnapshot().view!.next.available, false);
  stop();
});

test('completion advances by itself, or holds for a post-play decision when the policy says so', async () => {
  const {player, playback, stop} = setup();
  await player.playEntries(['a', 'b', 'c'].map(queueItemInput));
  playToEnd(playback);
  await settle(); await settle();
  assert.equal(playback.getSnapshot().itemId, 'b', 'advanced to the next entry');
  player.setCompletionPolicy(() => 'hold');
  playToEnd(playback);
  await settle(); await settle();
  const held = player.getSnapshot().completion;
  assert.ok(held && held.nextAvailable);
  assert.equal(playback.getSnapshot().itemId, 'b');
  await player.continueCompletion('manual');
  assert.equal(playback.getSnapshot().itemId, 'c');
  stop();
});

// P1 (web, 24 Sep): the engine reports the end itself, then the completion check reports it
// again. The server ended the session on the first one, so the second answered 410 and the queue
// stalled on track 1. The end is reported once, and an end the server already has is confirmed.
test('an end already reported still advances to the next entry', async () => {
  const {player, playback, api, log, stop} = setup();
  await player.playEntries(['a', 'b', 'c'].map(queueItemInput));
  const first = playback.getSnapshot().session!;
  playToEnd(playback);
  // The engine's own end report, before the queue's completion check gets to report it.
  await api.progressPlayback(first.id, {generation: first.generation, sequence: 1000, positionSeconds: playback.getSnapshot().duration, state: 'ended'});
  await settle(); await settle();
  assert.equal(playback.getSnapshot().itemId, 'b', 'advanced to the next entry');
  const ends = log.filter(r => r.method === 'POST' && r.path.endsWith(`/${first.id}/timeline`) && (r.body as {state?: string} | undefined)?.state === 'ended');
  assert.equal(ends.length, 1, 'the end is reported once');
  stop();
});

// NEW-44 (web, 24 Sep): the real server's queue window names a song "track" (v1 vocabulary), so
// a song queue fell into the video post-play hold, whose countdown the music player never shows:
// the album stopped after track 1. An audio session goes on at once, without the viewer.
test('a song queue advances by itself at the end of a track, whatever the window calls the entry', async () => {
  const log: {method: string; path: string; body?: unknown}[] = [];
  const {player, playback, api, stop} = setup({log, server: {itemKind: id => (id.startsWith('song') ? 'track' : 'movie')}});
  player.setCompletionPolicy(() => 'hold'); // the web player holds video for its post-play countdown
  await player.playEntries(['song-1', 'song-2', 'song-3'].map(queueItemInput));
  const first = playback.getSnapshot().session!;
  playToEnd(playback);
  await settle(); await settle();
  assert.equal(player.getSnapshot().completion, null, 'no post-play hold for audio');
  assert.equal(log.filter(r => r.method === 'POST' && r.path.endsWith(':advance')).length, 1, 'the queue advanced without the viewer');
  assert.equal(playback.getSnapshot().itemId, 'song-2');
  // The replaced session is retired: a late report from the engine for it is dropped, not sent.
  const before = log.length;
  await api.progressPlayback(first.id, {generation: first.generation, sequence: 2000, positionSeconds: 1, state: 'paused'});
  assert.equal(log.slice(before).filter(r => r.path.includes(first.id)).length, 0, 'no report for the replaced session');
  playToEnd(playback);
  await settle(); await settle();
  assert.equal(playback.getSnapshot().itemId, 'song-3', 'and on again');
  stop();
});

test('Up Next edits go to the server: remove, reorder (one moved entry), repeat, add', async () => {
  const {player, stop} = setup();
  await player.playEntries(['a', 'b', 'c', 'd'].map(queueItemInput));
  const service = player.workspace.service;
  const ids = () => service.getSnapshot().queue!.entries.map(e => (e as {itemId?: string}).itemId);
  await service.mutate({action: 'remove', entryId: service.getSnapshot().queue!.entries[3]!.id} as never);
  assert.deepEqual(ids(), ['a', 'b', 'c']);
  const e = service.getSnapshot().queue!.entries.map(x => x.id);
  await service.mutate({action: 'reorder', entryIds: [e[0], e[2], e[1]]} as never);
  assert.deepEqual(ids(), ['a', 'c', 'b']);
  await service.mutate({action: 'repeat', repeat: 'all'} as never);
  assert.equal(service.getSnapshot().queue!.repeat, 'all');
  await player.enqueue(queueItemInput('z'));
  assert.deepEqual(ids(), ['a', 'c', 'b', 'z']);
  stop();
});

test('movedEntry finds the one entry a drag moved', () => {
  assert.deepEqual(movedEntry(['a', 'b', 'c', 'd'], ['a', 'c', 'd', 'b']), {entryId: 'b', after: 'd'});
  assert.deepEqual(movedEntry(['a', 'b', 'c'], ['c', 'a', 'b']), {entryId: 'c', before: 'a'});
  assert.equal(movedEntry(['a', 'b'], ['a', 'b']), undefined);
});

test('the bridge reports with its own rising seq, keeps a paused session alive, and stops at the last position', async () => {
  const {api, player, playback, log, timers, stop} = setup();
  await player.playItem('movie');
  const id = playback.getSnapshot().session!.id;
  await api.progressPlayback(id, {generation: 1, sequence: 1, positionSeconds: 12, state: 'paused'});
  const keepalive = timers.at(-1)!;
  keepalive.fn();
  await settle();
  const reports = log.filter(r => r.path.endsWith('/timeline')).map(r => r.body as {seq: number; positionMs: number; state: string});
  assert.deepEqual(reports.map(r => r.seq), [1, 2]);
  assert.equal(reports[1]!.positionMs, 12_000);
  await api.stopPlayback(id);
  const del = log.find(r => r.method === 'DELETE' && r.path === `/v1/playback/sessions/${id}`)!;
  assert.deepEqual(del.body, {positionMs: 12_000});
  stop();
});

test('the switch: v1 for video, audiobooks and music on a v1 server; channels switch on their own capability', () => {
  assert.equal(serverSupportsPlaybackV1({features: {playback_v1: 'enabled'}}), true);
  assert.equal(serverSupportsPlaybackV1({features: {}}), false);
  assert.equal(playbackV1ForKind(true, 'movie'), true);
  assert.equal(playbackV1ForKind(true, 'episode'), true);
  assert.equal(playbackV1ForKind(true, 'audiobook_file'), true);
  assert.equal(playbackV1ForKind(true, 'song'), true);
  assert.equal(playbackV1ForKind(true, 'channel'), false);
  assert.equal(playbackV1ForKind(false, 'movie'), false);
});

test('a v1 session becomes the service session: stream is HLS, direct stays direct', () => {
  const s = toPlaybackSession({id: 's1', revision: '1', kind: 'vod', role: 'local', state: 'playing', lease: {reportEveryMs: 10000}, presentation: {generation: 2, mode: 'stream', url: '/v1/media/x/index.m3u8', startPositionMs: 5000, subtitles: [], decision: {}}} as never, 100);
  assert.deepEqual(s, {id: 's1', generation: 2, streamUrl: '/v1/media/x/index.m3u8', mode: 'hls', duration: 100, resumeSeconds: 5, protocol: 'v1'});
});

// ── Slice 2: events, queue_building, a deferred start ──────────────────────────

type Hook = (r: {method: string; path: string; body?: unknown}, response: {status: number; headers: Record<string, string>; body?: unknown}) => {status: number; headers: Record<string, string>; body?: unknown};

function setupWith(hook: Hook, before?: (r: {method: string; path: string}) => {status: number; headers: Record<string, string>; body?: unknown} | undefined) {
  const server = new FakePlaybackServer();
  const inner = fakeHttp(server);
  const http: V1Http = {send: async r => (before?.(r as never) ?? hook(r as never, await inner.send(r as never) as never)) as never};
  const listeners: ((e: {id: string; type: string; resource?: {kind: string; id: string}; revision?: string}) => void)[] = [];
  const events = {on: (_type: string, fn: (e: never) => void) => { listeners.push(fn as never); return () => {}; }};
  let serial = 0;
  const api = new V1PlaybackApi(http, {timers: {setTimer: () => 0, clearTimer: () => {}}, key: () => `k${++serial}`});
  const playback = new PlaybackService(api, () => `r${++serial}`, {});
  const player = new V1QueuePlayer({http, api, playback, viewer, key: () => `q${++serial}-key-000000`, pendingPollMs: 60_000, events, buildingRetryMs: 5000});
  const stop = player.connect();
  const emit = (queueId: string, revision?: string) => { for (const fn of listeners) fn({id: String(Date.now()), type: 'queue.updated', resource: {kind: 'queue', id: queueId}, revision}); };
  return {server, http: inner, player, playback, emit, stop};
}

test('queue.updated from another device refreshes Up Next', async () => {
  const {player, http, emit, stop} = setupWith((_r, res) => res);
  await player.playEntries(['a', 'b', 'c'].map(queueItemInput));
  const q = player.getSnapshot().view!.queue;
  // Another device (same profile) removes the last entry.
  await http.send({method: 'DELETE', path: `/v1/queues/${q.id}/entries/${encodeURIComponent(q.entries[2]!.id)}`, headers: {'If-Match': q.revision}} as never);
  emit(q.id, String(Number(q.revision) + 1));
  await new Promise(r => setTimeout(r, 250));
  assert.equal(player.getSnapshot().view!.queue.entries.length, 2);
  stop();
});

test('an entry still being snapshotted is retried until it can play (503 queue_building)', async () => {
  let refusals = 1;
  const {player, playback, stop} = setupWith((_r, res) => res, r => {
    if (r.method === 'POST' && r.path.endsWith(':advance') && refusals > 0) { refusals--; return {status: 503, headers: {'Retry-After': '0'}, body: {error: {code: 'queue_building', retry: 'same_request'}}}; }
    return undefined;
  });
  await player.playEntries(['a', 'b'].map(queueItemInput));
  const q = player.getSnapshot().view!.queue;
  await player.next(q.id, q.revision);
  assert.equal(refusals, 0);
  assert.equal(playback.getSnapshot().itemId, 'b');
  stop();
});

test('a start the server chooses later (a very large shuffle) waits for the build, woken by the event', async () => {
  let built = false;
  const building = (b: Record<string, unknown>) => ({...b, current: undefined, segments: (b.segments as Record<string, unknown>[]).map(s => ({...s, state: 'building'}))});
  const {player, playback, emit, stop} = setupWith((r, res) => {
    if (built || res.status >= 300) return res;
    if (r.method === 'POST' && r.path === '/v1/queues') {
      const b = res.body as {queue: Record<string, unknown>; window: unknown};
      return {...res, body: {queue: building(b.queue), window: b.window}};
    }
    if (r.method === 'GET' && /^\/v1\/queues\/[^/]+$/.test(r.path)) return {...res, body: building(res.body as Record<string, unknown>)};
    return res;
  });
  const started = player.playEntries(['a', 'b', 'c'].map(queueItemInput), {shuffle: true});
  await new Promise(r => setTimeout(r, 50));
  assert.equal(playback.getSnapshot().session, undefined, 'nothing plays before the server chooses');
  built = true;
  emit(player.getSnapshot().view!.queue.id);
  await started;
  assert.ok(playback.getSnapshot().session, 'the chosen entry plays');
  assert.ok(player.getSnapshot().view!.queue.currentEntryId);
  stop();
});

// ── Slice 3: the migration plan's queue scenarios ──────────────────────────────

test('a 1M-entry shuffled container plays in one request and the player holds only a window', async () => {
  const server = new FakePlaybackServer({containerSize: () => 1_000_000});
  const inner = fakeHttp(server);
  const log: string[] = [];
  const http: V1Http = {send: async r => { log.push(`${r.method} ${r.path}`); return inner.send(r as never) as never; }};
  let serial = 0;
  const api = new V1PlaybackApi(http, {timers: {setTimer: () => 0, clearTimer: () => {}}, key: () => `k${++serial}`});
  const playback = new PlaybackService(api, () => `r${++serial}`, {});
  const player = new V1QueuePlayer({http, api, playback, viewer, key: () => `q${++serial}-key-000000`});
  const stop = player.connect();
  await player.playSelector({container: {kind: 'collection', id: 'everything'}}, {shuffle: true});
  assert.equal(log.filter(l => l === 'POST /v1/queues').length, 1);
  assert.ok(playback.getSnapshot().session);
  for (let i = 0; i < 5; i++) {
    const q = player.getSnapshot().view!.queue;
    assert.ok(q.entries.length <= 71, `window ${q.entries.length}`);
    await player.next(q.id, q.revision);
  }
  const header = await new QueueClient(http).header(player.getSnapshot().view!.queue.id);
  assert.equal(header.total, 1_000_000);
  assert.ok(player.getSnapshot().view!.queue.entries.length <= 71);
  stop();
});

test('a start at a position, then choosing another entry, replaces the playing session', async () => {
  const log: {method: string; path: string; body?: unknown}[] = [];
  const {player, playback, stop} = setup({log});
  await player.playEntries(['a', 'b', 'c'].map(queueItemInput), {startSeconds: 30});
  const first = playback.getSnapshot().session!.id;
  const q = player.getSnapshot().view!.queue;
  await player.playEntry(q.id, q.entries[2]!.id, q.revision);
  assert.equal(playback.getSnapshot().itemId, 'c');
  assert.notEqual(playback.getSnapshot().session!.id, first);
  const start = log.find(r => r.method === 'POST' && r.path === '/v1/playback/sessions')!;
  assert.equal((start.body as {startPositionMs?: number}).startPositionMs, 30_000);
  stop();
});

test('a relaunch shows the device queue without playing it', async () => {
  const first = setup();
  await first.player.playEntries(['a', 'b'].map(queueItemInput));
  const id = first.player.getSnapshot().view!.queue.id;
  const again = new V1QueuePlayer({http: fakeHttp(first.server) as never, api: first.api, playback: new PlaybackService(first.api, () => 'x', {}), viewer});
  await again.open(id);
  assert.equal(again.getSnapshot().view!.queue.entries.length, 2);
  assert.equal(again.getSnapshot().view!.queue.id, id);
  first.stop();
  again.dispose();
});

// ── The server ends a playback (spec §14: an administrator terminates it) ──
test('an administrator’s terminate stops the service with their message, from the event, and nothing advances', async () => {
  const {server, player, playback, api, log, stop} = setup();
  await player.playEntries(['a', 'b'].map(queueItemInput));
  const sid = playback.getSnapshot().session!.id;
  playback.ready(playback.getSnapshot().intentId);
  const terminated = await fakeHttp(server).send({method: 'POST', path: `/v1/admin/sessions/${sid}:terminate`, body: {message: 'Maintenance tonight.'}} as never);
  assert.equal((terminated as {status: number}).status, 204);
  api.sessionUpdated(sid, {state: 'ended', reason: 'terminated', message: 'Maintenance tonight.'});
  const s = playback.getSnapshot();
  assert.equal(s.phase, 'error');
  assert.equal(s.intent, 'paused');
  assert.equal(s.error, 'Maintenance tonight.');
  assert.deepEqual(s.ended, {reason: 'terminated', message: 'Maintenance tonight.'});
  await settle();
  assert.equal(log.filter(r => r.path.endsWith(':advance')).length, 0, 'a terminated playback does not advance');
  // The next play clears it.
  await player.playEntries(['c'].map(queueItemInput));
  assert.equal(playback.getSnapshot().ended, undefined);
  stop();
});

test('a report that finds the session gone re-reads it and says why (an event missed while offline)', async () => {
  const {server, player, playback, api, stop} = setup();
  await player.playEntries(['a'].map(queueItemInput));
  const sid = playback.getSnapshot().session!.id;
  playback.ready(playback.getSnapshot().intentId);
  await fakeHttp(server).send({method: 'POST', path: `/v1/admin/sessions/${sid}:terminate`, body: {}} as never);
  await api.progressPlayback(sid, {generation: 1, sequence: 1, positionSeconds: 5, state: 'playing'}).catch(() => {});
  await settle();
  const s = playback.getSnapshot();
  assert.deepEqual(s.ended, {reason: 'terminated', message: undefined});
  assert.equal(s.error, 'The server owner stopped this playback.');
  stop();
});

test('the terminated failure code and the end reasons read as the owner wrote them', () => {
  assert.equal(playbackUserMessage({code: 'terminated', message: 'Back at\u0007 six.'}), 'Back at six.');
  assert.equal(playbackUserMessage({code: 'terminated', message: ''}), 'The server owner stopped this playback.');
  assert.equal(sessionEndMessage({reason: 'transferred'}), 'Playback continued on another device.');
});

// ── Rendered-audio edges on v1 (spec §18; plan §9) ──────────────────
function transitionsSetup(now: () => number = Date.now) {
  const server = new FakePlaybackServer({now});
  const http = fakeHttp(server) as unknown as V1Http;
  let serial = 0;
  const api = new V1PlaybackApi(http, {timers: {setTimer: () => 0, clearTimer: () => {}}, key: () => `k${++serial}`});
  const playback = new PlaybackService(api, () => `r${++serial}`, {});
  const player = new V1QueuePlayer({http, api, playback, viewer, key: () => `q${++serial}-key-000000`, pendingPollMs: 10, transitions: true});
  const stop = player.connect();
  return {server, api, playback, player, stop};
}
/** The engine is playing the current track, `left` seconds from its end. */
function nearEnd(playback: PlaybackService, left: number) {
  const s = playback.getSnapshot();
  playback.ready(s.intentId);
  if (s.pendingSeek) playback.seekApplied(s.intentId, s.pendingSeek.revision, s.pendingSeek.positionSeconds, 'playing');
  const now = playback.getSnapshot();
  playback.fact(now.intentId, Math.max(0, now.duration - left), 'playing');
}

test('§18: near the end of a rendered track the next one is prepared, committed and adopted at the boundary', async () => {
  const {server, api, playback, player, stop} = transitionsSetup();
  await player.playEntries(['song-1', 'song-2', 'song-3'].map(queueItemInput));
  const first = playback.getSnapshot().session!;
  assert.equal(first.audio?.mode, 'direct', 'the v1 presentation carries its version 2 render plan');
  assert.equal(playback.getSnapshot().audioEffects.rendering, true, 'gapless is on, so the engine renders');
  nearEnd(playback, 10);
  await settle();
  const prepared = playback.getSnapshot().audioEffects.prepared!;
  assert.ok(prepared, 'prepared');
  assert.equal(server.preparations, 1);
  assert.equal(prepared.audio!.durationFrames, 180 * 44100);
  // The engine buffered the next window, then reaches the edge.
  player.audioPrepared(prepared.token);
  await player.commitAudio(prepared.token);
  assert.equal(server.commits, 1);
  const committed = playback.getSnapshot().audioEffects.committed!;
  assert.ok(committed, 'committed');
  assert.equal(server.session(first.id)!.endReason, 'completed', 'the previous session ends as completed');
  assert.equal(server.session(committed.session.id)!.state, 'playing', 'the next session inherits the state');
  // Still the old track until the boundary; then the next. Its last reports go nowhere (the
  // server ended it), and they don't end playback.
  assert.equal(playback.getSnapshot().session!.id, first.id);
  const reports = server.session(first.id)!.reports.length;
  await api.progressPlayback(first.id, {generation: 1, sequence: 99, positionSeconds: 175, state: 'playing'});
  await settle();
  assert.equal(server.session(first.id)!.reports.length, reports);
  assert.equal(playback.getSnapshot().phase, 'ready');
  await player.audioBoundary(prepared.token, 0.05);
  assert.equal(playback.getSnapshot().session!.id, committed.session.id);
  assert.equal(playback.getSnapshot().itemId, 'song-2');
  assert.equal(player.getSnapshot().view?.queue.currentPlaybackId, committed.session.id);
  // One try per track: the new track prepares again only near its own end.
  assert.equal(server.preparations, 1);
  stop();
});

test('§18: a canceled or expired edge falls back to the ordinary end and advance', async () => {
  let clock = Date.now();
  const {server, playback, player, stop} = transitionsSetup(() => clock);
  await player.playEntries(['song-1', 'song-2'].map(queueItemInput));
  const first = playback.getSnapshot().session!;
  nearEnd(playback, 10);
  await settle();
  const prepared = playback.getSnapshot().audioEffects.prepared!;
  player.audioPrepared(prepared.token);
  // Another command on the queue cancels the preparation on the server.
  await player.workspace.service.mutate({action: 'repeat', repeat: 'all'} as never).catch(() => {});
  await player.commitAudio(prepared.token);
  assert.equal(server.commits, 0);
  assert.equal(playback.getSnapshot().audioEffects.prepared, undefined, 'the edge is dropped');
  assert.match(playback.getSnapshot().audioEffects.notice, /ordinary playback/);
  assert.equal(playback.getSnapshot().session!.id, first.id, 'the current track keeps playing');
  // Expiry is checked before asking: no request, the same fallback.
  const again = transitionsSetup(() => clock);
  await again.player.playEntries(['song-1', 'song-2'].map(queueItemInput));
  nearEnd(again.playback, 10);
  await settle();
  const late = again.playback.getSnapshot().audioEffects.prepared!;
  again.player.audioPrepared(late.token);
  clock += 61_000;
  await again.player.commitAudio(late.token);
  assert.equal(again.server.commits, 0);
  assert.match(again.playback.getSnapshot().audioEffects.notice, /expired/);
  stop(); again.stop();
});

test('§18: without the server’s queue transitions (or for video) nothing is prepared', async () => {
  const {server, playback, player, stop} = setup();
  await player.playEntries(['song-1', 'song-2'].map(queueItemInput));
  nearEnd(playback, 10);
  await settle();
  assert.equal(playback.getSnapshot().audioEffects.prepared, undefined);
  stop();
  const video = transitionsSetup();
  await video.player.playEntries(['a', 'b'].map(queueItemInput));
  nearEnd(video.playback, 10);
  await settle();
  assert.equal(video.server.preparations, 0);
  video.stop();
  void server;
});

test('§18 (B7): music plays on v1 by default, /v2 only when asked; the transitions capability is read', () => {
  assert.equal(playbackV1ForKind(true, 'song'), true, 'music is on v1');
  assert.equal(playbackV1ForKind(true, 'album'), true);
  assert.equal(playbackV1ForKind(true, 'song', {music: false}), false);
  assert.equal(playbackV1ForKind(false, 'song', {music: true}), false);
  assert.equal(playbackV1ForKind(true, 'movie'), true);
  assert.equal(serverSupportsQueueTransitions({features: {queueTransitions: 'enabled'}}), true);
  assert.equal(serverSupportsQueueTransitions({features: {playback_v1: 'enabled'}}), false);
});

// F-title, 23 Sep: on Apple (queue required) every v1 session was refused with "The server did
// not return canonical playback authority". A v1 session is its own authority (its lease is the
// timeline), so it plays.
test('with Apple’s options (queue required), a v1 session plays', async () => {
  const server = new FakePlaybackServer();
  const http = fakeHttp(server);
  let serial = 0;
  const api = new V1PlaybackApi(http, {timers: {setTimer: () => 0, clearTimer: () => {}}, key: () => `k${++serial}`});
  const playback = new PlaybackService(api, () => `r${++serial}`, {queueRequired: true, deferRecovery: true});
  const player = new V1QueuePlayer({http, api, playback, viewer, key: () => `q${++serial}-key-000000`, pendingPollMs: 10});
  const stop = player.connect();
  try {
    await player.playEntries(['movie-1', 'movie-2'].map(queueItemInput));
    const s = playback.getSnapshot();
    assert.equal(s.error, undefined, s.error);
    assert.ok(s.session?.id, 'the v1 session was adopted');
    assert.equal(s.session?.protocol, 'v1');
    assert.equal(s.phase, 'starting');
  } finally { stop(); player.dispose(); }
});

// Web's retry (PlaybackService.retry → V1QueuePlayer.check) restarts the entry where the failed
// start meant to be, not from the server's resume or 0:00.
test('a retry after a start that never played restarts at the requested position', async () => {
  const {server, playback, player, stop} = setup();
  await player.playEntries(['movie-1'].map(queueItemInput), {startSeconds: 6});
  const first = playback.getSnapshot().session!.id;
  playback.fail(playback.getSnapshot().intentId, 'The selected queue item did not become ready.');
  assert.equal(playback.recoveryTarget(), 6);
  await playback.retry();
  const s = playback.getSnapshot();
  assert.notEqual(s.session?.id, first);
  assert.equal(server.session(s.session!.id)!.positionMs, 6000);
  assert.equal(s.pendingSeek?.positionSeconds, 6);
  stop();
});

// B7: a music container played (not shuffled) starts with the viewer's defaults; an
// explicit Shuffle is a shuffle whatever the default; video is untouched.
test('a music queue started with Play takes the viewer\'s shuffle and repeat defaults', async () => {
  const log: {method: string; path: string; body?: unknown}[] = [];
  const server = new FakePlaybackServer();
  const inner = fakeHttp(server);
  const http: V1Http = {send: async r => { log.push({method: r.method, path: r.path, body: r.body}); return inner.send(r as never) as never; }};
  let serial = 0;
  const api = new V1PlaybackApi(http, {timers: {setTimer: () => 0, clearTimer: () => {}}, key: () => `k${++serial}`});
  const playback = new PlaybackService(api, () => `r${++serial}`, {});
  const player = new V1QueuePlayer({http, api, playback, viewer, key: () => `q${++serial}-key-000000`, pendingPollMs: 10, musicDefaults: () => ({shuffle: true, repeat: 'all'})});
  const stop = player.connect();
  try {
    await player.playSelector({container: {kind: 'album', id: 'album-1'}});
    const create = log.find(r => r.method === 'POST' && r.path === '/v1/queues')!;
    assert.deepEqual((create.body as any).segments[0].order, {mode: 'shuffle'});
    const patch = log.find(r => r.method === 'PATCH' && r.path.startsWith('/v1/queues/'));
    assert.deepEqual(patch?.body, {repeat: 'all'});
    assert.equal(player.getSnapshot().view?.queue.repeat, 'all');
    log.length = 0;
    await player.playSelector({container: {kind: 'season', id: 'season-1'}});
    assert.equal((log.find(r => r.path === '/v1/queues')!.body as any).segments[0].order, undefined, 'video keeps its order');
    assert.equal(log.some(r => r.method === 'PATCH'), false);
  } finally { stop(); }
});

test('sameAlbum: the next entry follows the current one on its album', async () => {
  const {player, stop} = setup();
  try {
    await player.playEntries(['song-1', 'song-2', 'song-3'].map(queueItemInput));
    const held = player as unknown as {window: readonly Record<string, unknown>[]};
    const album = [{albumId: 'a', disc: 1, track: 1}, {albumId: 'a', disc: 1, track: 2}, {albumId: 'b', disc: 1, track: 1}];
    held.window = held.window.map((e, i) => ({...e, ...album[i]}));
    assert.equal(player.sameAlbum('song-1', 'song-2'), true);
    assert.equal(player.sameAlbum('song-2', 'song-3'), false);
  } finally { stop(); }
});
