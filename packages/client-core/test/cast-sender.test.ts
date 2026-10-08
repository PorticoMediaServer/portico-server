import test from 'node:test';
import assert from 'node:assert/strict';
import {CastController, parseCastEvent, type CastCommand, type CastTransport} from '../src/cast/index.ts';

const settle = async () => { for (let i = 0; i < 10; i++) await new Promise(setImmediate); };

/** A fake receiver: records what the sender sends and lets the test answer as the TV would. */
function receiver(deviceName = 'Living Room TV') {
  const sent: CastCommand[] = [];
  const onMsg = new Set<(raw: unknown) => void>();
  const onEnd = new Set<() => void>();
  let ended: boolean | undefined;
  const transport: CastTransport = {
    deviceName,
    send: async m => { sent.push(m); },
    onMessage: fn => { onMsg.add(fn); return () => onMsg.delete(fn); },
    onSessionEnd: fn => { onEnd.add(fn); return () => onEnd.delete(fn); },
    end: stop => { ended = stop; },
  };
  return {
    transport, sent,
    reply: (m: unknown) => { for (const fn of [...onMsg]) fn(typeof m === 'string' ? m : JSON.stringify(m)); },
    endSession: () => { for (const fn of [...onEnd]) fn(); },
    get ended() { return ended; },
    get listeners() { return onMsg.size; },
  };
}

function harness(code: string | null = 'ABC123') {
  const requests: {path: string; method?: string; body?: unknown}[] = [];
  const timers: {fn: () => void; ms: number; id: number}[] = [];
  let next = 0;
  const controller = new CastController({
    api: {request: async <T,>(path: string, method?: string, body?: unknown) => { requests.push({path, method, body}); return {bootstrap: code ? {code} : {}} as T; }},
    origin: 'https://demo.getportico.tv/some/path',
    displayName: 'Chrome on Mac',
    setTimer: (fn, ms) => { const id = ++next; timers.push({fn, ms, id}); return id; },
    clearTimer: id => { const i = timers.findIndex(t => t.id === id); if (i >= 0) timers.splice(i, 1); },
  });
  return {controller, requests, fire: (ms: number) => { for (const t of timers.filter(x => x.ms === ms)) { timers.splice(timers.indexOf(t), 1); t.fn(); } }};
}

const meta = {title: 'Pilot', seriesTitle: 'The Show', season: 1, episode: 1, kind: 'episode', artworkPath: '/v1/artwork/x'};

test('pairs with a server-issued code, then loads the title with its display fields', async () => {
  const h = harness(), r = receiver();
  const started = h.controller.start(r.transport, 'item-1', meta, 42);
  await settle();
  assert.deepEqual(h.requests[0], {path: '/v1/cast/bootstrap', method: 'POST', body: {protocolVersion: '1.0', displayName: 'Living Room TV'}});
  assert.deepEqual(r.sent[0], {type: 'pair', code: 'ABC123', origin: 'https://demo.getportico.tv', displayName: 'Chrome on Mac'});
  assert.equal(h.controller.getSnapshot().phase, 'pairing');
  r.reply({type: 'pair-pending'});
  assert.equal(h.controller.getSnapshot().phase, 'waiting', 'the TV is asking the person watching');
  r.reply({type: 'paired'});
  assert.equal(await started, true);
  assert.deepEqual(r.sent[1], {type: 'load', itemId: 'item-1', startSeconds: 42, title: 'Pilot', seriesTitle: 'The Show', season: 1, episode: 1, kind: 'episode', artworkPath: '/v1/artwork/x'});
  const s = h.controller.getSnapshot();
  assert.equal(s.phase, 'casting');
  assert.equal(s.deviceName, 'Living Room TV');
  assert.equal(s.positionSeconds, 42);
});

test('status updates position, pause and tracks; commands go to the TV', async () => {
  const h = harness(), r = receiver();
  const started = h.controller.start(r.transport, 'item-1', meta, 0);
  await settle(); r.reply({type: 'paired'}); await started;
  r.reply({type: 'status', itemId: 'item-1', positionSeconds: 60, durationSeconds: 1800, paused: true, audioTracks: [{id: 1, language: 'en', name: 'English'}], textTracks: [{id: 3, language: 'fr', name: 'Français'}, {id: 'bad'}]});
  const s = h.controller.getSnapshot();
  assert.deepEqual([s.positionSeconds, s.durationSeconds, s.paused], [60, 1800, true]);
  assert.deepEqual(s.audioTracks, [{id: 1, language: 'en', name: 'English'}]);
  assert.deepEqual(s.textTracks.map(t => t.id), [3], 'malformed tracks are dropped');
  h.controller.toggle(); h.controller.seek(-5); h.controller.setAudioTrack(1); h.controller.setSubtitleTrack(3); h.controller.setSubtitleTrack(null);
  await settle();
  assert.deepEqual(r.sent.slice(2), [{type: 'play'}, {type: 'seek', positionSeconds: 0}, {type: 'audio', trackId: 1}, {type: 'subtitles', trackIds: [3]}, {type: 'subtitles', trackIds: []}]);
  h.controller.stop(); await settle();
  assert.deepEqual(r.sent.at(-1), {type: 'stop'});
  assert.equal(r.ended, true, 'stop closes Portico on the TV');
  assert.equal(h.controller.getSnapshot().phase, 'off');
  assert.equal(r.listeners, 0);
});

test('a busy TV refuses with catalogue copy, never the receiver’s text', async () => {
  const h = harness(), r = receiver();
  const started = h.controller.start(r.transport, 'item-1', meta, 0);
  await settle();
  r.reply({type: 'pair-failed', code: 'tv_busy', message: 'Sam is watching'});
  assert.equal(await started, false);
  const s = h.controller.getSnapshot();
  assert.equal(s.phase, 'error');
  assert.equal(s.errorId, 'cast.error.tvBusy');
  assert.ok(!/Sam/.test(s.error!));
  assert.equal(r.ended, true);
});

test('no answer within the pairing window, and no code from the server, are errors', async () => {
  const h = harness(), r = receiver();
  const started = h.controller.start(r.transport, 'item-1', meta, 0);
  await settle(); h.fire(35_000);
  assert.equal(await started, false);
  assert.equal(h.controller.getSnapshot().errorId, 'cast.error.noAnswer');
  const noCode = harness(null), r2 = receiver();
  assert.equal(await noCode.controller.start(r2.transport, 'item-1', meta, 0), false);
  assert.equal(noCode.controller.getSnapshot().errorId, 'cast.error.noCode');
  assert.equal(r2.sent.length, 0, 'nothing is sent to the TV without a code');
});

test('takeover: the controller is asked, answers, and the prompt expires after 30 s', async () => {
  const h = harness(), r = receiver();
  const started = h.controller.start(r.transport, 'item-1', meta, 0);
  await settle(); r.reply({type: 'paired'}); await started;
  r.reply({type: 'takeover-requested', requester: '  Sam’s iPhone  '});
  assert.deepEqual(h.controller.getSnapshot().takeover, {requester: 'Sam’s iPhone'});
  h.controller.answerTakeover(false); await settle();
  assert.deepEqual(r.sent.at(-1), {type: 'takeover', allow: false});
  assert.equal(h.controller.getSnapshot().takeover, undefined);
  r.reply({type: 'takeover-requested'});
  assert.equal(h.controller.getSnapshot().takeover?.requester, 'Someone');
  h.fire(30_000);
  assert.equal(h.controller.getSnapshot().takeover, undefined);
});

test('replaced by another sender, or the session ending, releases the TV without stopping it', async () => {
  const h = harness(), r = receiver();
  const started = h.controller.start(r.transport, 'item-1', meta, 0);
  await settle(); r.reply({type: 'paired'}); await started;
  r.reply({type: 'replaced'});
  assert.equal(h.controller.getSnapshot().phase, 'replaced');
  assert.equal(r.ended, false, 'the new sender keeps the TV');
  const h2 = harness(), r2 = receiver();
  const s2 = h2.controller.start(r2.transport, 'item-1', meta, 0);
  await settle(); r2.reply({type: 'paired'}); await s2;
  r2.endSession();
  assert.equal(h2.controller.getSnapshot().phase, 'off');
  assert.equal(r2.listeners, 0);
});

test('receiver messages are validated; HTTP origins are refused', () => {
  assert.equal(parseCastEvent('not json'), undefined);
  assert.equal(parseCastEvent({type: 'surprise'}), undefined);
  assert.deepEqual(parseCastEvent({type: 'pair-failed'}), {type: 'pair-failed', code: 'pair_failed'});
  assert.throws(() => new CastController({api: {request: async () => ({}) as never}, origin: 'http://192.168.1.2:32500', displayName: 'x'}));
});

test('CAST-05: a resumed session is adopted when the TV answers a status request', async () => {
  const {controller} = harness();
  const tv = receiver();
  const adopted = controller.adopt(tv.transport, async id => (id === 'item-1' ? 'Jurassic Park' : undefined));
  assert.deepEqual(tv.sent, [{type: 'status'}]);
  tv.reply({type: 'status', itemId: 'item-1', positionSeconds: 1834, durationSeconds: 7620, paused: false, audioTracks: [], textTracks: []});
  assert.equal(await adopted, true);
  await settle();
  const s = controller.getSnapshot();
  assert.equal(s.phase, 'casting');
  assert.equal(s.itemId, 'item-1');
  assert.equal(s.title, 'Jurassic Park');
  assert.equal(s.deviceName, 'Living Room TV');
  assert.equal(s.positionSeconds, 1834);
  // Later status keeps flowing through the normal path.
  tv.reply({type: 'status', itemId: 'item-1', positionSeconds: 1840, durationSeconds: 7620, paused: true, audioTracks: [], textTracks: []});
  assert.equal(controller.getSnapshot().paused, true);
});

test('CAST-05: a resumed session that does not answer, or needs pairing, is left alone', async () => {
  const {controller, fire} = harness();
  const quiet = receiver();
  const first = controller.adopt(quiet.transport);
  fire(5000);
  assert.equal(await first, false);
  assert.equal(quiet.ended, undefined);
  assert.equal(quiet.listeners, 0);
  assert.equal(controller.getSnapshot().phase, 'off');
  const other = receiver();
  const second = controller.adopt(other.transport);
  other.reply({type: 'pair-required'});
  assert.equal(await second, false);
  assert.equal(other.ended, undefined);
  assert.equal(controller.getSnapshot().phase, 'off');
});

test('CAST-05: a resumed session is adopted on the TV status; a silent one is left alone; dispose settles a pending adoption', async () => {
  const {controller} = harness();
  const tv = receiver();
  const adopted = controller.adopt(tv.transport, async id => (id === 'item-1' ? 'Pilot' : undefined));
  assert.deepEqual(tv.sent.map(m => m.type), ['status']);
  tv.reply({type: 'status', itemId: 'item-1', positionSeconds: 12, durationSeconds: 100, paused: false});
  assert.equal(await adopted, true);
  await settle();
  assert.equal(controller.getSnapshot().phase, 'casting');
  assert.equal(controller.getSnapshot().title, 'Pilot');
  assert.equal(tv.ended, undefined);

  const silent = harness();
  const quiet = receiver();
  const pending = silent.controller.adopt(quiet.transport);
  silent.fire(5000);
  assert.equal(await pending, false);
  assert.equal(quiet.ended, undefined, 'a session this page did not start is never ended');

  const disposed = harness();
  const other = receiver();
  const waiting = disposed.controller.adopt(other.transport);
  disposed.controller.dispose();
  assert.equal(await waiting, false);
});
