import test from 'node:test';
import assert from 'node:assert/strict';
import {
  AdminSessionsClient, CapabilityPublisher, CommandRouter, DeviceCommandsClient, EventsClient, NowPlayingStore, PlaybackApiError,
  PlaybackOptionsClient, PlaybackSessionController, QUEUE_WINDOW_MAX, QueueClient, QueueView, SessionsClient, ShufflePermutation,
  commandHandler, defaultSelection, markerAt, parseCommand, seekNeedsServer, summarizePlan,
  type MediaPlayer, type PlayerEvent, type PlayerObservation, type PlayerSource, type ServerEvent,
} from '../src/playback-v1/index.ts';
import {FakePlaybackServer, fakeHttp} from '../src/playback-v1/testing/fake-server.ts';

const settle = async () => { for (let i = 0; i < 12; i++) await new Promise(setImmediate); };

function clockwork() {
  let now = 1_000_000;
  const timers: {fn: () => void; at: number; id: number}[] = [];
  let next = 0;
  return {
    now: () => now,
    setTimer: (fn: () => void, ms: number) => { const id = ++next; timers.push({fn, at: now + ms, id}); return id; },
    clearTimer: (id: unknown) => { const i = timers.findIndex(t => t.id === id); if (i >= 0) timers.splice(i, 1); },
    async advance(ms: number) {
      const end = now + ms;
      for (;;) {
        timers.sort((a, b) => a.at - b.at);
        const t = timers[0];
        if (!t || t.at > end) break;
        timers.shift(); now = t.at; t.fn(); await settle();
      }
      now = end;
      await settle();
    },
  };
}

class FakePlayer implements MediaPlayer {
  loads: PlayerSource[] = [];
  seeks: number[] = [];
  sidecar: (string | null)[] = [];
  rate = 1;
  state: PlayerObservation['state'] = 'idle';
  generation = 0;
  positionMs = 0;
  private listeners = new Set<(e: PlayerEvent) => void>();
  load(s: PlayerSource) { this.loads.push(s); this.generation = s.generation; this.positionMs = s.startPositionMs; this.state = s.autoplay ? 'playing' : 'paused'; }
  play() { this.state = 'playing'; this.emit('state'); }
  pause() { this.state = 'paused'; this.emit('state'); }
  seek(ms: number) { this.seeks.push(ms); this.positionMs = ms; this.emit('seeked'); }
  setRate(r: number) { this.rate = r; }
  subtitle: string | null | undefined;
  selectSidecarSubtitle(id: string | null) { this.sidecar.push(id); this.subtitle = id; }
  observe(): PlayerObservation { return {generation: this.generation, state: this.state, positionMs: this.positionMs, rate: this.rate, ...(this.subtitle !== undefined ? {subtitleTrackId: this.subtitle} : {})}; }
  onEvent(l: (e: PlayerEvent) => void) { this.listeners.add(l); return () => { this.listeners.delete(l); }; }
  emit(type: 'state' | 'time' | 'seeked') { for (const l of [...this.listeners]) l({type, observation: this.observe()} as PlayerEvent); }
  dispose() { this.listeners.clear(); }
}

let keys = 0;
const key = () => 'key_' + String(++keys).padStart(16, '0');

// ── Options (§4) ───────────────────────────────────────────────────
test('options: tolerant parse, plan summary, default selection, markers and preview queries', async () => {
  const server = new FakePlaybackServer();
  const client = new PlaybackOptionsClient(fakeHttp(server));
  const o = await client.options('movie-1');
  assert.equal(o.versions.length, 1);
  assert.equal(o.markers.length, 2, 'an unknown marker type is kept with a fallback, not dropped or fatal');
  const sel = defaultSelection(o);
  assert.equal(sel.version?.id, 'v1');
  assert.equal(sel.audio?.id, 'a1');
  assert.equal(markerAt(o, 30_000)?.type, 'intro');
  const original = summarizePlan(o.plan!, sel.version);
  assert.equal(original.convertsVideo, false);
  assert.equal(original.convertsAudio, true, 'TrueHD converts on this device');
  const limited = await client.options('movie-1', {quality: {mode: 'limit', maxHeight: 1080}, audioId: 'a2'});
  const s = summarizePlan(limited.plan!, sel.version);
  assert.equal(s.kind, 'convert');
  assert.equal(s.convertsVideo, true);
  assert.equal(s.convertsAudio, false);
  // Capabilities go up only when they change.
  const publisher = new CapabilityPublisher(fakeHttp(server));
  const profile = {containers: ['mp4'], video: [], audio: [], subtitles: [], hdr: [], maxResolution: {width: 1920, height: 1080}} as never;
  assert.equal(await publisher.publish(profile), true);
  assert.equal(await publisher.publish(profile), false);
  assert.equal(server.capabilities.size, 1);
});

// ── Sessions (§5) ──────────────────────────────────────────────────
test('sessions: idempotent start replays, a reused key with a new body is 422, 202 preparation is polled', async () => {
  const c = clockwork();
  const server = new FakePlaybackServer({now: c.now, prepareRounds: body => (body.itemId === 'big-remux' ? 2 : 0)});
  const client = new SessionsClient(fakeHttp(server), c);
  const a = await client.start({itemId: 'movie-1'}, {startFrom: 'resume'}, 'key_same_000000001');
  const b = await client.start({itemId: 'movie-1'}, {startFrom: 'resume'}, 'key_same_000000001');
  assert.equal(a.id, b.id);
  assert.equal(a.presentation.startPositionMs, 120_000);
  await assert.rejects(client.start({itemId: 'movie-2'}, {}, 'key_same_000000001'), (e: unknown) => e instanceof PlaybackApiError && e.status === 422 && e.code === 'idempotency_key_reused');
  const preparing: number[] = [];
  const started = client.start({itemId: 'big-remux'}, {}, 'key_prepare_0000001', {onPreparing: ms => preparing.push(ms)});
  await settle();
  assert.equal(preparing.length, 1);
  await c.advance(500);
  await c.advance(500);
  const s = await started;
  assert.equal(s.itemId, 'big-remux');
  assert.deepEqual(preparing, [500, 500]);
});

test('controller: start loads generation 1, reports, and replaces its own previous session', async () => {
  const c = clockwork();
  const server = new FakePlaybackServer({now: c.now});
  const player = new FakePlayer();
  const ctl = new PlaybackSessionController({http: fakeHttp(server), player, timers: c, key});
  const first = (await ctl.start({itemId: 'movie-1'}))!;
  assert.equal(player.loads.length, 1);
  assert.equal(player.loads[0]!.generation, 1);
  assert.equal(ctl.getSnapshot().phase, 'active');
  const second = (await ctl.start({itemId: 'movie-2'}))!;
  assert.notEqual(second.id, first.id);
  assert.equal(server.session(first.id)!.ended, true, 'replacesSessionId ended the old session');
  ctl.dispose();
});

test('controller: track and quality changes reload a new generation; stale player events are ignored', async () => {
  const c = clockwork();
  const server = new FakePlaybackServer({now: c.now});
  const player = new FakePlayer();
  const ctl = new PlaybackSessionController({http: fakeHttp(server), player, timers: c, key});
  await ctl.start({itemId: 'movie-1'});
  await ctl.setAudio('a2');
  assert.equal(player.loads.length, 2);
  assert.equal(player.loads[1]!.generation, 2);
  await ctl.setQuality({mode: 'limit', maxHeight: 720});
  assert.equal(player.loads[2]!.generation, 3);
  // A late event from generation 1 must not move the observation.
  const before = ctl.getSnapshot().observation;
  player.generation = 1; player.emit('time'); player.generation = 3;
  assert.equal(ctl.getSnapshot().observation, before);
  ctl.dispose();
});

test('controller: seek is local for direct play and goes through the server once something converts', async () => {
  const c = clockwork();
  const server = new FakePlaybackServer({now: c.now});
  const player = new FakePlayer();
  const ctl = new PlaybackSessionController({http: fakeHttp(server), player, timers: c, key});
  const s = (await ctl.start({itemId: 'movie-1'}))!;
  assert.equal(seekNeedsServer(s), false);
  await ctl.seek(60_000);
  assert.deepEqual(player.seeks, [60_000]);
  await ctl.setQuality({mode: 'limit', maxHeight: 720});
  assert.equal(seekNeedsServer(ctl.getSnapshot().session!), true);
  const loads = player.loads.length;
  await ctl.seek(90_000);
  assert.equal(player.seeks.length, 1, 'no local seek');
  assert.equal(player.loads.length, loads + 1, 'the server re-produced from the new position');
  assert.equal(player.loads.at(-1)!.startPositionMs, 90_000);
  // Sidecar subtitles switch locally; others need the server.
  const sent = server.session(ctl.getSnapshot().session!.id)!.reports.length;
  ctl.setSubtitles('s4');
  await settle();
  assert.deepEqual(player.sidecar, ['s4']);
  const reports = server.session(ctl.getSnapshot().session!.id)!.reports;
  assert.equal(reports.length, sent + 1, 'the local switch is reported at once');
  assert.equal(reports.at(-1)!.subtitleTrackId, 's4');
  await ctl.setSubtitles('s3');
  assert.equal(player.loads.length, loads + 2);
  ctl.dispose();
});

test('controller: a 412 adopts the current session and re-applies only the newest intent', async () => {
  const c = clockwork();
  const server = new FakePlaybackServer({now: c.now});
  const player = new FakePlayer();
  const ctl = new PlaybackSessionController({http: fakeHttp(server), player, timers: c, key});
  const s = (await ctl.start({itemId: 'movie-1'}))!;
  server.bump(s.id, {audio: {trackId: 'a2'}}); // another controller changed it
  const updated = await ctl.setVersion('v1b');
  assert.ok(updated);
  const live = server.session(s.id)!;
  assert.equal(live.versionId, 'v1b');
  assert.deepEqual(live.audio, {trackId: 'a2'}, 'the other change is kept');
  assert.equal(ctl.getSnapshot().session!.revision, String(live.revision));
  // ETag is the quoted revision; If-Match accepts either form.
  const got = await server.handle({method: 'GET', path: `/v1/playback/sessions/${s.id}`});
  assert.equal(got.headers.ETag, `"${live.revision}"`);
  const quoted = await server.handle({method: 'PATCH', path: `/v1/playback/sessions/${s.id}`, headers: {'If-Match': got.headers.ETag!}, body: {state: 'paused'}});
  assert.equal(quoted.status, 200);
  await ctl.sessionUpdated(s.id);
  // Two changes queued behind a conflict: the older one is not re-sent.
  server.bump(s.id, {state: 'paused'});
  const older = ctl.setAudio('a1');
  const newer = ctl.setAudio('a2');
  await Promise.all([older, newer]);
  assert.deepEqual(server.session(s.id)!.audio, {trackId: 'a2'});
  ctl.dispose();
});

test('controller: the server ending the session (410, stop elsewhere) pauses and reports once', async () => {
  const c = clockwork();
  const server = new FakePlaybackServer({now: c.now});
  const player = new FakePlayer();
  const ended: string[] = [];
  const ctl = new PlaybackSessionController({http: fakeHttp(server), player, timers: c, key, onServerEnded: s => ended.push(s.id)});
  const s = (await ctl.start({itemId: 'movie-1'}))!;
  await server.handle({method: 'DELETE', path: `/v1/playback/sessions/${s.id}`, body: {}});
  assert.equal(await ctl.setAudio('a2'), undefined);
  await settle();
  assert.equal(ctl.getSnapshot().phase, 'ended');
  assert.equal(player.state, 'paused');
  assert.deepEqual(ended, [s.id]);
  ctl.dispose();
});

test('controller: stop flushes the position and ends the session (idempotent)', async () => {
  const c = clockwork();
  const server = new FakePlaybackServer({now: c.now});
  const player = new FakePlayer();
  const ctl = new PlaybackSessionController({http: fakeHttp(server), player, timers: c, key});
  const s = (await ctl.start({itemId: 'movie-1'}))!;
  player.positionMs = 42_000; player.emit('time');
  await ctl.stop();
  await ctl.stop();
  const live = server.session(s.id)!;
  assert.equal(live.ended, true);
  assert.equal(live.positionMs, 42_000);
  ctl.dispose();
});

// ── Queues (§8) ────────────────────────────────────────────────────
test('shuffle permutation is a bijection with a working inverse, sampled up to 2^20', () => {
  for (const n of [1, 2, 3, 17, 1000, 65_537]) {
    const p = new ShufflePermutation(n, 12345);
    const seen = new Set<number>();
    for (let i = 0; i < n; i++) { const v = p.at(i); assert.ok(v >= 0 && v < n); seen.add(v); assert.equal(p.indexOf(v), i); }
    assert.equal(seen.size, n);
  }
  const big = new ShufflePermutation(1 << 20, 7), seen = new Set<number>();
  for (let i = 0; i < 5000; i++) { const at = (i * 2654435761) % (1 << 20); const v = big.at(at); assert.equal(big.indexOf(v), at); assert.ok(!seen.has(v)); seen.add(v); }
  assert.notDeepEqual([0, 1, 2, 3, 4].map(i => new ShufflePermutation(1000, 1).at(i)), [0, 1, 2, 3, 4].map(i => new ShufflePermutation(1000, 2).at(i)));
});

test('queue: shuffled Play is stable for a seed, windows stay ≤ 200, and a 10M queue costs only what is read', async () => {
  const server = new FakePlaybackServer({containerSize: (_k, id) => (id === 'everything' ? 10_000_000 : 50)});
  const client = new QueueClient(fakeHttp(server), key);
  const a = await client.create([{source: {container: {kind: 'library', id: 'music'}}, order: {mode: 'shuffle', seed: '99'}}], {startPlayback: {}});
  assert.ok(a.session, 'Play and the first session are one request');
  assert.equal(a.session!.queue?.entryId, a.queue.current?.entryId);
  const order1 = (await client.entries(a.queue.id, 0, 50)).items.map(e => e.itemId);
  const b = await client.create([{source: {container: {kind: 'library', id: 'music'}}, order: {mode: 'shuffle', seed: '99'}}]);
  const order2 = (await client.entries(b.queue.id, 0, 50)).items.map(e => e.itemId);
  assert.deepEqual(order1, order2, 'same seed, same order');
  assert.equal(new Set(order1).size, 50);
  const big = await client.create([{source: {container: {kind: 'library', id: 'everything'}}, order: {mode: 'shuffle', seed: '5'}}]);
  assert.equal(big.queue.total, 10_000_000);
  const page = await client.entries(big.queue.id, 9_999_900, 500);
  assert.equal(page.items.length, 100);
  const w = await client.entries(big.queue.id, 0, 1000);
  assert.equal(w.items.length, QUEUE_WINDOW_MAX, 'the client never asks for more than 200');
  // The view is O(visible): a scroll deep into the queue holds a few pages, not the queue.
  const view = new QueueView(client, big.queue, {pageSize: 100, maxResidentPages: 4});
  view.entries.ensureRange(5_000_000, 5_000_030);
  await settle();
  assert.ok(view.entries.itemAt(5_000_010));
  for (const at of [100, 2_000_000, 7_000_000, 9_000_000, 9_999_000]) { view.entries.ensureRange(at, at + 20); await settle(); }
  assert.ok(view.entries.resident().items <= 4 * 100);
  view.dispose();
});

test('queue: play next, move and remove carry If-Match; a 412 adopts the current header', async () => {
  const server = new FakePlaybackServer();
  const client = new QueueClient(fakeHttp(server), key);
  const created = await client.create([{source: {items: {ids: ['t1', 't2', 't3', 't4']}}}]);
  const view = new QueueView(client, created.queue);
  const ids = async () => (await client.entries(view.header.id, 0, 50)).items.map(e => e.itemId);
  await view.run((id, rev) => client.addSegment(id, rev, 'next', {source: {items: {ids: ['x1']}}}));
  assert.deepEqual(await ids(), ['t1', 't2', 't3', 't4', 'x1'], 'the segment after the current one (one segment here)');
  const entries = (await client.entries(view.header.id, 0, 50)).items;
  await view.run((id, rev) => client.move(id, rev, entries[3]!.entryId, {before: entries[0]!.entryId}));
  assert.deepEqual(await ids(), ['t4', 't1', 't2', 't3', 'x1']);
  const now = (await client.entries(view.header.id, 0, 50)).items;
  await view.run((id, rev) => client.remove(id, rev, now[2]!.entryId));
  assert.deepEqual(await ids(), ['t4', 't1', 't3', 'x1']);
  // Another device changes the queue; our stale revision gets 412 and the view adopts the new header.
  const stale = view.header.revision;
  await client.update(view.header.id, view.header.revision, {repeat: 'all'});
  await assert.rejects(view.run((id) => client.update(id, stale, {repeat: 'one'})), (e: unknown) => e instanceof PlaybackApiError && e.status === 412);
  assert.equal(view.header.repeat, 'all');
  view.dispose();
});

test('queue: advance moves the current entry and starts the next session; repeat all wraps', async () => {
  const server = new FakePlaybackServer();
  const client = new QueueClient(fakeHttp(server), key);
  const created = await client.create([{source: {items: {ids: ['t1', 't2']}}}], {startPlayback: {}});
  let q = created.queue;
  const next = await client.advance(q.id, q.revision, 'completion');
  assert.equal(next.queue.current?.position, 1);
  assert.equal(next.session?.itemId, 't2');
  assert.equal(server.session(created.session!.id)!.ended, true);
  q = next.queue;
  await assert.rejects(client.advance(q.id, q.revision, 'completion'), (e: unknown) => e instanceof PlaybackApiError && e.code === 'queue_ended');
  q = (await client.update(q.id, q.revision, {repeat: 'all'})).queue;
  const wrapped = await client.advance(q.id, q.revision, 'completion');
  assert.equal(wrapped.queue.current?.position, 0);
});

// ── Commands and transfers (§9) ────────────────────────────────────
test('commands: sent through the server, delivered once to the target even when redelivered', async () => {
  const server = new FakePlaybackServer({devices: [{id: 'phone', name: 'Phone', form: 'phone'}, {id: 'tv', name: 'Living room', form: 'tv'}], deviceId: 'phone'});
  const phone = new DeviceCommandsClient(fakeHttp(server), key);
  const devices = await phone.controllable();
  assert.deepEqual(devices.map(d => d.id), ['tv']);
  const got: string[] = [];
  const router = new CommandRouter({onCommand: c => { got.push(c.type); }});
  const id = await phone.send('tv', {type: 'seek', positionMs: 5000});
  assert.ok(id);
  const event = server.eventLog().at(-1) as ServerEvent;
  assert.equal(router.handle(event), true);
  assert.equal(router.handle(event), false, 'a redelivered command is ignored');
  await router.settled();
  assert.deepEqual(got, ['seek']);
  assert.equal(parseCommand({commandId: 'c', type: 'setVolume', level: 3}), undefined, 'out-of-range values are refused');
  assert.equal(parseCommand({commandId: 'c', type: 'teleport'}), undefined, 'unknown types are ignored');
});

test('commands: the default handler drives a session controller', async () => {
  const calls: string[] = [];
  const target = {
    play: () => calls.push('play'), pause: () => calls.push('pause'), stop: async () => { calls.push('stop'); },
    seek: (ms: number) => calls.push('seek:' + ms), setAudio: (id: string) => calls.push('audio:' + id), setSubtitles: (id: string | null) => calls.push('subs:' + id),
    setQuality: () => calls.push('quality'), start: async (t: {itemId: string} | {transferId: string}) => { calls.push('start:' + ('itemId' in t ? t.itemId : t.transferId)); },
  };
  const next: string[] = [];
  const handle = commandHandler(target, {next: () => next.push('next')});
  for (const c of [{type: 'pause'}, {type: 'resume'}, {type: 'seek', positionMs: 10}, {type: 'setTracks', audio: 'a2', subtitles: null}, {type: 'play', target: {itemId: 'm'}}, {type: 'next'}] as const)
    await handle({...c, commandId: 'x'} as never);
  assert.deepEqual(calls, ['pause', 'play', 'seek:10', 'audio:a2', 'subs:null', 'start:m']);
  assert.deepEqual(next, ['next']);
});

test('transfer: the source keeps playing until the target reports playing, then the server ends it', async () => {
  const c = clockwork();
  const server = new FakePlaybackServer({now: c.now, devices: [{id: 'phone', name: 'Phone', form: 'phone'}, {id: 'tv', name: 'TV', form: 'tv'}], deviceId: 'phone'});
  const phonePlayer = new FakePlayer();
  const ended: string[] = [];
  const phone = new PlaybackSessionController({http: fakeHttp(server), player: phonePlayer, timers: c, key, onServerEnded: s => ended.push(s.id)});
  const source = (await phone.start({itemId: 'movie-1'}))!;
  phonePlayer.positionMs = 300_000; phonePlayer.emit('time');
  await server.handle({method: 'POST', path: `/v1/playback/sessions/${source.id}/timeline`, body: {seq: 1, generation: 1, state: 'playing', positionMs: 300_000}});
  const transferId = await new DeviceCommandsClient(fakeHttp(server), key).offerTransfer(source.id, 'tv');
  // The TV hears the offer and starts from the transfer.
  const offers: string[] = [];
  const router = new CommandRouter({onTransferOffered: o => offers.push(o.transferId)});
  for (const e of server.eventLog()) router.handle(e as ServerEvent);
  assert.deepEqual(offers, [transferId]);
  const tvServer = server.asDevice('tv');
  const tvPlayer = new FakePlayer();
  const tv = new PlaybackSessionController({http: fakeHttp(tvServer), player: tvPlayer, timers: c, key});
  const target = (await tv.start({transferId}))!;
  assert.equal(target.presentation.startPositionMs, 300_000);
  assert.equal(server.session(source.id)!.ended, false, 'not committed until the target plays');
  await tvServer.handle({method: 'POST', path: `/v1/playback/sessions/${target.id}/timeline`, body: {seq: 1, generation: 1, state: 'playing', positionMs: 301_000}});
  assert.equal(server.session(source.id)!.ended, true);
  await phone.sessionUpdated(source.id);
  assert.deepEqual(ended, [source.id]);
  phone.dispose(); tv.dispose();
});

// ── Events (§15) ───────────────────────────────────────────────────
test('events: one stream, prefix subscriptions, reconnect from the cursor after stream.closed, no duplicates', async () => {
  const c = clockwork();
  const server = new FakePlaybackServer({now: c.now, setTimer: c.setTimer, clearTimer: c.clearTimer});
  const events = new EventsClient({http: fakeHttp(server), openStream: server.openStream, setTimer: c.setTimer, clearTimer: c.clearTimer});
  const seen: string[] = [];
  events.on('queue.', e => seen.push(e.type + '#' + e.id));
  events.on('session.updated', e => seen.push(e.type + '#' + e.id));
  events.start();
  await settle();
  assert.equal(events.getStatus().mode, 'stream');
  server.publish('queue.updated', {kind: 'queue', id: 'q1'}, '2');
  server.publish('session.updated', {kind: 'session', id: 's1'}, '3');
  await settle();
  assert.deepEqual(seen, ['queue.updated#1', 'session.updated#2']);
  server.closeStreams(1000);
  await settle();
  server.publish('queue.updated', {kind: 'queue', id: 'q1'}, '4'); // while disconnected
  await c.advance(1000);
  assert.deepEqual(seen, ['queue.updated#1', 'session.updated#2', 'queue.updated#4'], 'resumed from the cursor; the closed marker is not an event');
  events.deliver({id: '4', type: 'queue.updated'});
  assert.equal(seen.length, 3, 'deduplicated by id');
  events.stop();
});

test('events: a refused stream (409 stream_exists) falls back to long-poll; an old cursor resyncs', async () => {
  const c = clockwork();
  const server = new FakePlaybackServer({now: c.now, setTimer: c.setTimer, clearTimer: c.clearTimer, ring: 5});
  server.refuseStreams(409);
  const events = new EventsClient({http: fakeHttp(server), openStream: server.openStream, waitSeconds: 25, setTimer: c.setTimer, clearTimer: c.clearTimer, cursor: '0'});
  const seen: string[] = [];
  let resyncs = 0;
  events.on('admin.sessions', e => seen.push(e.id));
  events.onResync(() => resyncs++);
  for (let i = 0; i < 8; i++) server.publish('admin.sessions'); // the ring keeps only 5
  events.start();
  await settle();
  assert.equal(events.getStatus().mode, 'poll');
  assert.equal(resyncs, 1, 'the cursor fell out of the ring: refetch');
  server.publish('admin.sessions');
  await settle();
  assert.deepEqual(seen, ['9'], 'a parked long-poll answers as soon as an event arrives');
  await c.advance(25_000);
  events.stop();
});

// ── Admin Now Playing (§14) ────────────────────────────────────────
test('admin: Now Playing refreshes on events (coalesced) and terminate ends the viewer’s session', async () => {
  const server = new FakePlaybackServer();
  const http = fakeHttp(server);
  const sessions = new SessionsClient(http);
  const a = await sessions.start({itemId: 'movie-1'}, {}, key());
  await sessions.start({itemId: 'movie-2'}, {}, key());
  const client = new AdminSessionsClient(http, key);
  const store = new NowPlayingStore(client);
  await Promise.all([store.refresh(), store.refresh(), store.refresh()]);
  assert.equal(store.getSnapshot().sessions.length, 2);
  assert.equal(store.getSnapshot().sessions[0]!.decision.video?.action, 'copy');
  await store.terminate(a.id, 'Maintenance tonight');
  await store.refresh();
  assert.equal(store.getSnapshot().sessions.length, 1);
  assert.equal(server.session(a.id)!.ended, true);
  assert.equal(server.session(a.id)!.terminatedMessage, 'Maintenance tonight');
});

test('controller: an administrator’s terminate ends it with the reason and message, from the event or a re-read', async () => {
  for (const withPayload of [true, false]) {
    const c = clockwork();
    const server = new FakePlaybackServer({now: c.now});
    const player = new FakePlayer();
    const ctl = new PlaybackSessionController({http: fakeHttp(server), player, timers: c, key});
    const s = (await ctl.start({itemId: 'movie-1'}))!;
    await fakeHttp(server).send({method: 'POST', path: `/v1/admin/sessions/${s.id}:terminate`, body: {message: 'Maintenance tonight'}} as never);
    await ctl.sessionUpdated(s.id, undefined, withPayload ? {state: 'ended', reason: 'terminated', message: 'Maintenance tonight'} : undefined);
    await c.advance(0);
    assert.equal(ctl.getSnapshot().phase, 'ended');
    assert.deepEqual(ctl.getSnapshot().ended, {reason: 'terminated', message: 'Maintenance tonight'});
    ctl.dispose();
  }
});
