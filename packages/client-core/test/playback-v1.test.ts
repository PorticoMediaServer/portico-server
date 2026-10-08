import test from 'node:test';
import assert from 'node:assert/strict';
import {GenerationGate, ServerClock, TimelineReporter, automaticRung, driftCorrection, qualityDecision, qualityLadder, qualityLane, qualityPreferences, sameQuality, timelinePosition, type PlayerObservation} from '../src/playback-v1/index.ts';
import {FakePlaybackServer, fakeResponseError} from '../src/playback-v1/testing/fake-server.ts';

const settle = async () => { for (let i = 0; i < 10; i++) await new Promise(setImmediate); };

/** A controllable clock and timer wheel. */
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
    pending: () => timers.length,
  };
}

async function started(server: FakePlaybackServer, body: Record<string, unknown> = {itemId: 'movie-1', startPositionMs: 0, state: 'playing'}, key = 'k1') {
  const r = await server.handle({method: 'POST', path: '/v1/playback/sessions', headers: {'Idempotency-Key': key}, body});
  assert.equal(r.status, 201);
  return r.body as {id: string; revision: string; presentation: {generation: number}};
}

function reporterFor(server: FakePlaybackServer, sessionId: string, c: ReturnType<typeof clockwork>, extra: Partial<ConstructorParameters<typeof TimelineReporter>[0]> = {}) {
  const sent: {seq: number; generation: number; state: string; positionMs: number; skipped?: unknown; error?: unknown}[] = [];
  const reporter = new TimelineReporter({
    now: c.now, setTimer: c.setTimer, clearTimer: c.clearTimer,
    send: async report => {
      sent.push(report as never);
      const r = await server.handle({method: 'POST', path: `/v1/playback/sessions/${sessionId}/timeline`, body: report});
      if (r.status >= 300) throw fakeResponseError(r);
      return {reportEveryMs: Number(r.headers['Report-Every-Ms'])};
    },
    ...extra,
  });
  return {reporter, sent};
}

const obs = (generation: number, state: PlayerObservation['state'], positionMs: number): PlayerObservation => ({generation, state, positionMs, rate: 1});

test('fake server: idempotent start, If-Match, generations, idempotent stop and lease expiry (spec §16.1–4)', async () => {
  const c = clockwork();
  const server = new FakePlaybackServer({now: c.now});
  const a = await started(server);
  const replay = await started(server);
  assert.equal(replay.id, a.id, 'same key, same body → same session');
  const reused = await server.handle({method: 'POST', path: '/v1/playback/sessions', headers: {'Idempotency-Key': 'k1'}, body: {itemId: 'other'}});
  assert.equal(reused.status, 422, 'idempotency_key_reused (spec §17.1)');
  const stale = await server.handle({method: 'PATCH', path: `/v1/playback/sessions/${a.id}`, headers: {'If-Match': '0'}, body: {state: 'paused'}});
  assert.equal(stale.status, 412);
  assert.ok((stale.body as {error: {current: unknown}}).error.current, '412 carries the current session');
  const paused = await server.handle({method: 'PATCH', path: `/v1/playback/sessions/${a.id}`, headers: {'If-Match': '1'}, body: {state: 'paused'}});
  assert.equal((paused.body as {presentation: {generation: number}}).presentation.generation, 1, 'pausing changes no bytes');
  const track = await server.handle({method: 'PATCH', path: `/v1/playback/sessions/${a.id}`, headers: {'If-Match': '2'}, body: {audio: {trackId: 'a2'}}});
  assert.equal((track.body as {presentation: {generation: number}}).presentation.generation, 2, 'a track change is a new generation');
  assert.equal((await server.handle({method: 'DELETE', path: `/v1/playback/sessions/${a.id}`})).status, 204);
  assert.equal((await server.handle({method: 'DELETE', path: `/v1/playback/sessions/${a.id}`})).status, 204, 'stop is idempotent');
  const b = await started(server, {itemId: 'movie-2'}, 'k2');
  await c.advance(121_000);
  assert.equal((await server.handle({method: 'POST', path: `/v1/playback/sessions/${b.id}/timeline`, body: {seq: 1, generation: 1, state: 'playing', positionMs: 0}})).status, 410, 'the lease lapsed');
});

test('reporter: first report at once, immediate on state change and seek, then 10 s playing / 30 s paused', async () => {
  const c = clockwork();
  const server = new FakePlaybackServer({now: c.now, reportEveryMs: 10_000});
  const s = await started(server);
  const {reporter, sent} = reporterFor(server, s.id, c);
  reporter.setGeneration(1);
  reporter.observe(obs(1, 'playing', 0), 'state');
  await settle();
  assert.equal(sent.length, 1);
  for (let t = 1; t <= 25; t++) reporter.observe(obs(1, 'playing', t * 1000));
  await c.advance(25_000);
  assert.equal(sent.length, 3, 'time updates alone report every 10 s');
  reporter.observe(obs(1, 'paused', 25_000), 'state');
  await settle();
  assert.equal(sent.at(-1)!.state, 'paused', 'a state change reports immediately');
  const before = sent.length;
  await c.advance(29_000);
  assert.equal(sent.length, before, 'paused waits 30 s');
  await c.advance(1000);
  assert.equal(sent.length, before + 1);
  reporter.observe(obs(1, 'playing', 60_000), 'seeked');
  await settle();
  assert.equal(sent.at(-1)!.positionMs, 60_000, 'a completed seek reports immediately');
  assert.deepEqual(sent.map(r => r.seq), sent.map((_, i) => i + 1), 'seq rises by one per report');
  assert.equal(server.session(s.id)!.positionMs, 60_000);
});

test('reporter: a generation switch drops the old presentation’s observations; stale reports are never sent (invariant 3)', async () => {
  const c = clockwork();
  const server = new FakePlaybackServer({now: c.now});
  const s = await started(server);
  const {reporter, sent} = reporterFor(server, s.id, c);
  reporter.setGeneration(1);
  reporter.observe(obs(1, 'playing', 1000), 'state');
  await settle();
  await server.handle({method: 'PATCH', path: `/v1/playback/sessions/${s.id}`, headers: {'If-Match': '1'}, body: {audio: {trackId: 'a2'}}});
  reporter.setGeneration(2);
  reporter.observe(obs(1, 'playing', 5000), 'state');
  reporter.failed(1, 'decoder_error');
  await c.advance(30_000);
  assert.ok(sent.every(r => r.generation === 1 ? r.positionMs === 1000 : true));
  assert.equal(sent.filter(r => r.generation === 1).length, 1, 'nothing more from generation 1');
  reporter.observe(obs(2, 'playing', 6000), 'state');
  await settle();
  assert.equal(sent.at(-1)!.generation, 2);
  reporter.setGeneration(1);
  assert.equal(reporter.snapshot().generation, 2, 'generations only move forward');
});

test('reporter: failures back off and coalesce to the newest state; offline long enough warns about the lease', async () => {
  const c = clockwork();
  const server = new FakePlaybackServer({now: c.now});
  const s = await started(server);
  let risk = 0;
  const {reporter, sent} = reporterFor(server, s.id, c, {onLeaseRisk: () => risk++});
  reporter.setGeneration(1);
  server.failNext(500, 500, 'offline');
  reporter.observe(obs(1, 'playing', 0), 'state');
  await settle();
  for (let t = 1; t <= 5; t++) reporter.observe(obs(1, 'playing', t * 1000));
  await c.advance(1000);
  await c.advance(2000);
  assert.equal(reporter.snapshot().offline, true);
  assert.equal(sent.length, 3, 'one attempt per backoff step, not one per observation');
  await c.advance(5000);
  assert.equal(reporter.snapshot().failures, 0);
  assert.equal(sent.at(-1)!.positionMs, 5000, 'the retry carries the newest position');
  const offlineAll = Array.from({length: 20}, () => 'offline' as const);
  server.failNext(...offlineAll);
  reporter.observe(obs(1, 'paused', 6000), 'state');
  await c.advance(100_000);
  assert.equal(risk, 1, 'one warning when the lease is at risk');
});

test('reporter: skips and errors ride the next report; a session the server ended stops reporting', async () => {
  const c = clockwork();
  const server = new FakePlaybackServer({now: c.now});
  const s = await started(server);
  let ended = 0;
  const {reporter, sent} = reporterFor(server, s.id, c, {onEnded: () => ended++});
  reporter.setGeneration(1);
  reporter.observe(obs(1, 'playing', 0), 'state');
  await settle();
  reporter.skipped({markerId: 'intro-1', mode: 'automatic', positionMs: 10_000});
  await settle();
  assert.deepEqual(sent.at(-1)!.skipped, {markerId: 'intro-1', mode: 'automatic', positionMs: 10_000});
  reporter.observe(obs(1, 'playing', 80_000), 'seeked');
  await settle();
  assert.equal(sent.at(-1)!.skipped, undefined, 'a skip is reported once');
  await server.handle({method: 'DELETE', path: `/v1/playback/sessions/${s.id}`});
  reporter.observe(obs(1, 'paused', 81_000), 'state');
  await settle();
  assert.equal(ended, 1);
  assert.equal(reporter.snapshot().stopped, true);
  assert.equal(c.pending(), 0, 'no timers left behind');
  await reporter.flush();
});

test('quality: modes map to requests; Automatic follows the estimate with hysteresis and never exceeds preferences', () => {
  const prefs = (mode: string, extra: Record<string, number> = {}) => qualityPreferences(<T,>(k: string, f: T) => (k.endsWith('.mode') ? mode : k.endsWith('maxVideoHeight') ? (extra.h ?? f) : k.endsWith('maxVideoBitrateMbps') ? (extra.mbps ?? f) : f) as T, 'wifi');
  assert.deepEqual(qualityDecision(prefs('off'), 'remote'), {allowed: false, reason: 'network_off'});
  assert.deepEqual(qualityDecision(prefs('original'), 'local'), {allowed: true, quality: {mode: 'original'}, network: 'local'});
  assert.deepEqual(qualityDecision(prefs('data-saver'), 'cellular'), {allowed: true, quality: {mode: 'limit', maxVideoBitrateKbps: 1500, maxHeight: 480, maxAudioBitrateKbps: 128}, network: 'cellular'});
  assert.deepEqual(qualityDecision(prefs('standard', {h: 720}), 'remote'), {allowed: true, quality: {mode: 'limit', maxVideoBitrateKbps: 8000, maxHeight: 720}, network: 'remote'}, 'preferences cap the mode');
  const auto = qualityDecision(prefs('automatic', {h: 1080}), 'remote', 100_000);
  assert.ok(auto.allowed && auto.quality.mode === 'limit' && auto.quality.maxHeight === 1080, 'a fast link still respects the 1080p preference');
  const at1080 = qualityLadder.findIndex(r => r.height === 1080);
  assert.equal(automaticRung(3000, at1080), qualityLadder.findIndex(r => r.height === 480), 'down-switches are immediate');
  assert.equal(automaticRung(11_000, at1080 + 1), at1080 + 1, 'not enough headroom to climb to 1080p');
  assert.equal(automaticRung(13_000, at1080 + 1), at1080, 'with 25 % headroom it climbs');
  assert.equal(qualityLane('remote', true), 'wifi');
  assert.equal(qualityLane('remote'), 'unknown');
  assert.ok(sameQuality({mode: 'original'}, {mode: 'original'}));
});

test('clock: best-RTT offset, server time, group position and drift correction (spec §10)', () => {
  let now = 10_000;
  const clock = new ServerClock({now: () => now});
  assert.equal(clock.estimate(), undefined);
  assert.equal(clock.add({t0: 1000, t1: 6010, t2: 6020, t3: 1100}), true);
  assert.equal(clock.add({t0: 2000, t1: 7005, t2: 7006, t3: 2020}), true);
  assert.equal(clock.add({t0: 5, t1: 1, t2: 0, t3: 4}), false, 'impossible samples are ignored');
  const e = clock.estimate()!;
  assert.equal(e.rttMs, 19);
  assert.equal(Math.round(e.offsetMs), 4996);
  assert.equal(Math.round(clock.serverNow()), 14_996);
  assert.equal(timelinePosition({state: 'playing', positionMs: 1000, rate: 1, atServerTimeMs: 14_000}, 14_996), 1996);
  assert.equal(timelinePosition({state: 'paused', positionMs: 1000, rate: 1, atServerTimeMs: 0}, 99_999), 1000);
  assert.deepEqual(driftCorrection(1000, 1020), {kind: 'none'});
  const nudge = driftCorrection(1000, 1200);
  assert.ok(nudge.kind === 'rate' && nudge.rate > 1 && nudge.rate <= 1.05);
  assert.deepEqual(driftCorrection(1000, 2000), {kind: 'seek', positionMs: 2000});
  now += 11 * 60_000;
  assert.equal(clock.estimate(), undefined, 'old samples expire');
  const gate = new GenerationGate();
  assert.equal(gate.advance(3), true);
  assert.equal(gate.advance(3), false);
  assert.equal(gate.accepts(2), false);
});
