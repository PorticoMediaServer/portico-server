import test from 'node:test';
import assert from 'node:assert/strict';
import {CastController, castChoiceFields, parseCastEvent, type CastCommand, type CastTransport} from '../src/cast/index.ts';

const settle = async () => { for (let i = 0; i < 10; i++) await new Promise(setImmediate); };

/** A fake receiver: records what the sender sends and lets the test answer as the TV would. */
function receiver(deviceName = 'Living Room TV', deviceId?: string) {
  const sent: CastCommand[] = [];
  const onMsg = new Set<(raw: unknown) => void>();
  const onEnd = new Set<() => void>();
  let ended: boolean | undefined;
  let autoAck = true;
  const transport: CastTransport = {
    deviceName,
    ...(deviceId ? {deviceId} : {}),
    send: async m => { sent.push(m); if (m.type === 'load' && autoAck) queueMicrotask(() => { for (const fn of onMsg) fn({type:'loaded',requestId:m.requestId,itemId:m.itemId}); }); },
    onMessage: fn => { onMsg.add(fn); return () => onMsg.delete(fn); },
    onSessionEnd: fn => { onEnd.add(fn); return () => onEnd.delete(fn); },
    end: stop => { ended = stop; },
  };
  return {
    transport, sent,
    set autoAck(value: boolean) { autoAck = value; },
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

test('the subtitle chosen on the sender travels with load: a track id, or off; nothing when unknown', async () => {
  assert.deepEqual(castChoiceFields({subtitleTrackId: 's3'}), {subtitleTrackId: 's3'});
  assert.deepEqual(castChoiceFields({subtitleTrackId: null}), {subtitleTrackId: 'off'});
  assert.deepEqual(castChoiceFields({}), {});
  assert.deepEqual(castChoiceFields(undefined), {});
  assert.deepEqual(castChoiceFields({subtitleTrackId: 'a b'}), {});
  const h = harness(), r = receiver();
  const started = h.controller.start(r.transport, 'item-1', {title: 'Pilot'}, 0, {subtitleTrackId: 's3'});
  await settle();
  r.reply({type: 'paired'});
  assert.equal(await started, true);
  assert.deepEqual(r.sent[1], {type: 'load', requestId: 'load-1', itemId: 'item-1', startSeconds: 0, title: 'Pilot', subtitleTrackId: 's3'});
});

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
  assert.deepEqual(r.sent[1], {type: 'load', requestId: 'load-1', itemId: 'item-1', startSeconds: 42, title: 'Pilot', seriesTitle: 'The Show', season: 1, episode: 1, kind: 'episode', artworkPath: '/v1/artwork/x'});
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
  assert.equal(r.ended, false, 'a refused takeover leaves the current film playing');
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

test('a stored controller token resumes the existing TV session without pairing or loading again',async()=>{
 const saved=new Map<string,string>();let codes=0;
 const options={api:{request:async<T,>()=>{codes++;return {bootstrap:{code:'ABC123'}} as T;}},origin:'https://server.test',displayName:'Phone',controllerStorage:{get:(k:string)=>saved.get(k),set:(k:string,v:string)=>{saved.set(k,v);}}};
 const first=new CastController(options),initial=receiver();const started=first.start(initial.transport,'movie',{title:'Film'},12);
 await settle();initial.reply({type:'paired',controlToken:'a'.repeat(48)});assert.equal(await started,true);first.dispose();
 const returning=new CastController(options),transport=receiver();const adopting=returning.adopt(transport.transport);
 assert.deepEqual(transport.sent[0],{type:'status',controlToken:'a'.repeat(48)});
 transport.reply({type:'status',itemId:'movie',positionSeconds:80,durationSeconds:600,paused:false,audioTracks:[],textTracks:[]});assert.equal(await adopting,true);
 assert.equal(codes,1);assert.equal(transport.sent.filter(x=>x.type==='load').length,0);
 returning.pause();assert.equal(transport.sent.at(-1)?.type,'pause');returning.dispose();
});

test('two televisions with one name keep their own pairing proofs when the platform names each device', async () => {
  const saved = new Map<string, string>();
  const options = {api: {request: async <T,>() => ({bootstrap: {code: 'ABC123'}}) as T}, origin: 'https://server.test', displayName: 'Phone', controllerStorage: {get: (k: string) => saved.get(k), set: (k: string, v: string) => { saved.set(k, v); }}};
  const first = new CastController(options), lounge = receiver('TV', 'device-a');
  const started = first.start(lounge.transport, 'movie', {title: 'Film'}, 0);
  await settle(); lounge.reply({type: 'paired', controlToken: 'a'.repeat(48)}); assert.equal(await started, true); first.dispose();
  // The other "TV" never paired with this phone: it is asked without the first one's proof.
  const returning = new CastController(options), bedroom = receiver('TV', 'device-b');
  void returning.adopt(bedroom.transport);
  assert.deepEqual(bedroom.sent[0], {type: 'status'});
  returning.dispose();
  // The first one, by its own id, gets its proof back.
  const again = new CastController(options), loungeAgain = receiver('TV', 'device-a');
  void again.adopt(loungeAgain.transport);
  assert.deepEqual(loungeAgain.sent[0], {type: 'status', controlToken: 'a'.repeat(48)});
  again.dispose();
});

test('load acknowledgement is correlated and a missing/rejected load never claims playback', async () => {
  const h = harness(), r = receiver(); r.autoAck = false;
  const started = h.controller.start(r.transport, 'item-1', meta, 0);
  await settle(); r.reply({type:'paired'}); await settle();
  assert.equal(h.controller.getSnapshot().phase, 'pairing');
  r.reply({type:'loaded',requestId:'load-other',itemId:'item-1'});
  assert.equal(h.controller.getSnapshot().phase, 'pairing');
  r.reply({type:'load-failed',requestId:'load-1'});
  assert.equal(await started, false);
  assert.equal(h.controller.getSnapshot().phase, 'error');
  const second=h.controller.start(r.transport, 'item-1', meta, 0);
  await settle();r.reply({type:'paired'});await settle();h.fire(150_000);
  assert.equal(await second,false);
});
test('a queue load carries only bounded server queue and entry identities', async () => {
  assert.deepEqual(castChoiceFields({queue:{queueId:'q-1',entryId:'entry-2'}}),{queue:{queueId:'q-1',entryId:'entry-2'}});
  assert.deepEqual(castChoiceFields({queue:{queueId:'https://evil.test',entryId:'entry-2'}}),{});
  const h=harness(),r=receiver();const started=h.controller.start(r.transport,'song',meta,0,{queue:{queueId:'q-1',entryId:'entry-2'}});
  await settle();r.reply({type:'paired'});assert.equal(await started,true);
  const load=r.sent.find(m=>m.type==='load');assert.deepEqual(load?.queue,{queueId:'q-1',entryId:'entry-2'});
});
