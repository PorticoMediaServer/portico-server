import test from 'node:test';
import assert from 'node:assert/strict';
import {PlaybackService, tunerHandover} from '../src/index.ts';
import {V1ChannelControl, channelEndMessage, parseChannelSession, v1ChannelId} from '../src/playback-v1/channel-control.ts';
import type {V1Http, V1Request} from '../src/playback-v1/http.ts';
import type {ChannelReference} from '../src/playback/linear-protocol.ts';

const settle = async () => { for (let i = 0; i < 40; i++) await new Promise(r => setImmediate(r)); };
const channel: ChannelReference = {kind: 'library-channel', sourceId: 'news', channelId: 'news', generation: 'g1'};

/** A v1 channel session as spec §18.6 serves it: the stream appears after a read; a live seek
 * resolves to the live edge; a report within 1 s of the pending seek acknowledges it. */
class ChannelServer implements V1Http {
  log: V1Request[] = [];
  refuse?: string;
  private reads = 0;
  private s: any;
  async send(r: V1Request) {
    this.log.push(r);
    const ok = (body: unknown, status = 200) => ({status, headers: {}, body});
    if (r.method === 'POST' && r.path === '/v1/playback/sessions') {
      if (this.refuse) return {status: 409, headers: {}, body: {error: {code: this.refuse, retry: 'after_refresh'}}};
      this.s = {id: 'ps_1', revision: '1', kind: 'channel', state: (r.body as {state:string}).state, presentation: {generation: 0, url: '', subtitles: [], decision: {},
        linear: {name: 'News', programme: {id: 'p1', title: 'Morning', startMs: 0, endMs: 3_600_000}, originMs: 1000, windowStartMs: 0, windowEndMs: 0, liveEdgeMs: 0, state: 'preparing'}}};
      return ok(this.s, 201);
    }
    const s = this.s;
    if (r.method === 'GET') {
      if (++this.reads >= 1 && !s.presentation.url) {
        s.presentation.generation = 1; s.presentation.url = '/v2/media/linear/ps_1/1/tok/index.m3u8';
        Object.assign(s.presentation.linear, {windowStartMs: 0, windowEndMs: 60_000, liveEdgeMs: 60_000, state: 'active'});
      }
      return ok(s);
    }
    if (r.method === 'PATCH') {
      if (r.headers?.['If-Match'] !== `"${s.revision}"`) return {status: 412, headers: {}, body: {error: {code: 'revision_mismatch', current: s}}};
      const b = r.body as any;
      if (b.state) s.state = b.state;
      if (b.seek) s.presentation.linear.seek = {id: b.seek.id, positionMs: b.seek.live ? s.presentation.linear.liveEdgeMs : b.seek.positionMs, acknowledged: false};
      s.revision = String(Number(s.revision) + 1);
      return ok(s);
    }
    if (r.method === 'POST' && r.path.endsWith('/timeline')) {
      const b = r.body as any, seek = s.presentation.linear.seek;
      if (seek && Math.abs(b.positionMs - seek.positionMs) <= 1000) seek.acknowledged = true;
      s.presentation.linear.confirmedPositionMs = b.positionMs;
      return {status: 204, headers: {}};
    }
    if (r.method === 'DELETE') { s.state = 'ended'; return {status: 204, headers: {}}; }
    return {status: 404, headers: {}, body: {error: {code: 'not_found'}}};
  }
}

function setup() {
  const server = new ChannelServer();
  let serial = 0;
  const control = new V1ChannelControl({http: server, key: () => `key-${String(++serial).padStart(16, '0')}`, pollMs: 5});
  const playback = new PlaybackService({createPlayback: async () => { throw new Error('no VOD'); }, stopPlayback: async () => {}, progressPlayback: async () => {}}, () => `r${++serial}`, {});
  playback.useChannelPlayback(control);
  return {server, control, playback};
}

test('v1 channels: a tune starts a channel session, joins at the live edge, and the reached seek is acknowledged', async () => {
  const {server, playback} = setup();
  try {
    await playback.playChannel(channel);
    await new Promise(r => setTimeout(r, 40));
    await settle();
    const start = server.log.find(r => r.method === 'POST' && r.path === '/v1/playback/sessions')!;
    assert.deepEqual(start.body, {channelId: v1ChannelId(channel), channelGeneration: 'g1', state: 'playing'});
    const join = server.log.find(r => r.method === 'PATCH')!;
    assert.equal((join.body as any).seek.live, true);
    let s = playback.getSnapshot();
    assert.equal(s.session?.streamUrl, '/v2/media/linear/ps_1/1/tok/index.m3u8');
    assert.equal(s.linear?.name, 'News');
    assert.equal(s.pendingSeek?.positionSeconds, 60, 'the engine attaches at the resolved live edge');
    // The engine reached it: the report acknowledges the seek and the pending seek clears.
    playback.ready(s.intentId);
    playback.seekApplied(s.intentId, s.pendingSeek!.revision, 60, 'playing');
    await new Promise(r => setTimeout(r, 40));
    await settle();
    s = playback.getSnapshot();
    assert.equal(s.pendingSeek, undefined);
    assert.ok(server.log.some(r => r.path.endsWith('/timeline') && (r.body as any).positionMs === 60_000));
  } finally { playback.leave(); }
  await settle();
  assert.ok(server.log.some(r => r.method === 'DELETE'), 'leaving stops the session');
});

test('v1 channels: pause and a seek in the window are PATCHes on the current revision', async () => {
  const {server, playback} = setup();
  try {
    await playback.playChannel(channel);
    await new Promise(r => setTimeout(r, 40));
    playback.pause();
    await settle();
    playback.seek(12);
    await new Promise(r => setTimeout(r, 30));
    const patches = server.log.filter(r => r.method === 'PATCH').map(r => r.body as any);
    assert.ok(patches.some(b => b.state === 'paused'));
    assert.ok(patches.some(b => b.seek?.positionMs === 12_000));
    assert.equal(playback.getSnapshot().pendingSeek?.positionSeconds, 12);
  } finally { playback.leave(); }
});

test('v1 channels: a restore POST and its resolved join stay paused until an explicit resume',async()=>{
 const {server,playback}=setup();
 try{
  const intents:string[]=[];const unsubscribe=playback.subscribe(()=>intents.push(playback.getSnapshot().intent));
  try{
   await playback.playChannel(channel,{state:'paused'});
   for(let i=0;i<100&&!playback.getSnapshot().session;i++)await new Promise(r=>setTimeout(r,2));
   const start=server.log.find(r=>r.method==='POST'&&r.path==='/v1/playback/sessions')!;
   assert.deepEqual(start.body,{channelId:v1ChannelId(channel),channelGeneration:'g1',state:'paused'});
   let snapshot=playback.getSnapshot();assert.ok(snapshot.session);assert.equal(snapshot.intent,'paused');assert.equal(snapshot.playbackRun,1);
   const join=server.log.find(r=>r.method==='PATCH')!;assert.equal((join.body as any).state,'paused');assert.equal((join.body as any).seek.live,true);
   playback.ready(snapshot.intentId);playback.seekApplied(snapshot.intentId,snapshot.pendingSeek!.revision,60,'paused');await settle();
   // Restore only inside the real retained window, while the actual session is paused.
   snapshot=playback.getSnapshot();assert.equal(snapshot.intent,'paused');assert.equal(snapshot.linear?.desired.state,'paused');
   assert.ok(Number(snapshot.linear!.media.windowStartUs)/1e6<=12&&Number(snapshot.linear!.media.windowEndUs)/1e6>=12);
   playback.seek(12);await settle();
   const seek=server.log.filter(r=>r.method==='PATCH').at(-1)!;assert.equal((seek.body as any).state,'paused');assert.equal((seek.body as any).seek.positionMs,12000);
   assert.ok(intents.every(intent=>intent==='paused'));assert.ok(server.log.filter(r=>r.method==='POST'||r.method==='PATCH').every(r=>(r.body as any).state===undefined||(r.body as any).state==='paused'));
   playback.resume();await settle();assert.equal(playback.getSnapshot().intent,'playing');assert.ok(server.log.some(r=>r.method==='PATCH'&&(r.body as any).state==='playing'));
  }finally{unsubscribe();}
 }finally{playback.leave();}
});

test('v1 channels: every tuner in use is said as such, and the start is over', async () => {
  const {server, playback} = setup();
  server.refuse = 'no_tuner_available';
  await playback.playChannel(channel);
  await settle();
  const s = playback.getSnapshot();
  assert.equal(s.phase, 'error');
  assert.match(s.error ?? '', /tuners/);
  playback.leave();
});

test('v1 channels: an end names why: the administrator’s message, Live TV off for the profile, else the generic end', () => {
  assert.equal(channelEndMessage({reason: 'terminated', message: 'Server maintenance'}), 'Server maintenance');
  assert.equal(channelEndMessage({reason: 'feature_restricted'}), 'Live TV is turned off for this profile.');
  assert.match(channelEndMessage({reason: 'something_new'}), /has ended/);
  assert.match(channelEndMessage(undefined), /has ended/);
});

// B-11.27: with the owner's "Stop watching" setting a recording takes a watched tuner. The viewer
// is counted down, then the channel stops by itself and says why; it is not retried.
test('v1 channels: a tuner deadline counts down, then the channel stops once with the reason', async () => {
  const {server, playback} = setup();
  try {
    await playback.playChannel(channel);
    await new Promise(r => setTimeout(r, 40));
    await settle();
    const deadline = Date.now() + 90_000;
    (server as any).s.presentation.linear.reservationRequiredAtMs = deadline;
    await new Promise(r => setTimeout(r, 20));
    await settle();
    let s = playback.getSnapshot();
    assert.equal(s.linear?.media.reservationRequiredAtMs, deadline);
    assert.deepEqual(tunerHandover(deadline, Date.now()), {kind: 'later', atMs: deadline});
    assert.deepEqual(tunerHandover(deadline, deadline - 12_300), {kind: 'countdown', secondsLeft: 13});
    assert.notEqual(s.phase, 'error');
    // The server gives the tuner to the recording.
    (server as any).s.presentation.linear.errorCode = 'tuner_reservation_changed';
    await new Promise(r => setTimeout(r, 20));
    await settle();
    s = playback.getSnapshot();
    assert.equal(s.phase, 'error');
    assert.equal(s.ended?.reason, 'tuner_reserved');
    assert.equal(s.linear?.name, 'News', 'the stop still names the channel');
    assert.ok(server.log.some(r => r.method === 'DELETE'), 'the channel session is stopped');
    const reads = server.log.length;
    await new Promise(r => setTimeout(r, 30));
    await settle();
    assert.equal(server.log.length, reads, 'nothing polls or retries after the stop');
  } finally { playback.leave(); }
});

// N18: the countdown runs on server time. A device clock five minutes slow still sees the tuner
// go ninety seconds after the server said so.
test('v1 channels: a tuner deadline is moved onto this device clock with the server clock sent beside it', () => {
  const server = 1_800_000_000_000, device = server - 300_000;
  const raw = {id: 's', revision: '1', state: 'playing', presentation: {generation: 1, url: '/x', linear: {name: 'One', originMs: 0, windowStartMs: 0, windowEndMs: 0, liveEdgeMs: 0, state: 'live', reservationRequiredAtMs: server + 90_000, nowMs: server}}};
  const parsed = parseChannelSession(raw, () => device);
  assert.equal(parsed.presentation.linear?.reservationRequiredAtMs, device + 90_000);
  assert.deepEqual(tunerHandover(parsed.presentation.linear?.reservationRequiredAtMs, device + 30_000), {kind: 'countdown', secondsLeft: 60});
  // Without the server clock, the deadline is taken as sent.
  const {nowMs: _n, ...plain} = raw.presentation.linear;
  assert.equal(parseChannelSession({...raw, presentation: {...raw.presentation, linear: plain}}, () => device).presentation.linear?.reservationRequiredAtMs, server + 90_000);
});


test('a channel start forwards captured cancellation and refuses late adoption after its screen retires', async () => {
  for (const abort of [false, true]) {
    const server = new ChannelServer(), life = new AbortController();let active = true, enter!: () => void, release!: () => void;
    const entered = new Promise<void>(resolve => {enter = resolve;}), held = new Promise<void>(resolve => {release = resolve;});
    const states: unknown[] = [], errors: string[] = []; let sentSignal: AbortSignal | undefined;
    const control = new V1ChannelControl({http: {send: async request => {
      const response = await server.send(request);
      if (request.method === 'POST' && request.path === '/v1/playback/sessions') {sentSignal = request.signal;enter();await held;}
      return response;
    }}});
    const opening = control.open(channel, {state: value => states.push(value), error: value => errors.push(value)}, {signal: life.signal, isCurrent: () => active});
    await entered;
    assert.ok(sentSignal); assert.equal(sentSignal.aborted, false);
    if (abort) life.abort();else active = false;
    release();await opening;await settle();
    assert.equal(sentSignal.aborted, true, 'the channel releases only its own pending request when the opening retires');
    assert.deepEqual(states, []);assert.deepEqual(errors, []);
    assert.ok(server.log.some(request => request.method === 'DELETE' && request.path === '/v1/playback/sessions/ps_1'));
    control.close();
  }
});

function preparationQueue() {
  const jobs = new Map<number, () => void>(); const cleared: unknown[] = []; let serial = 0;
  return {timers: {setTimer: (fn: () => void) => {jobs.set(++serial, fn); return serial;}, clearTimer: (timer: unknown) => {cleared.push(timer); jobs.delete(timer as number);}},
    fire() {const job = jobs.entries().next().value; if (!job) throw new Error('no channel preparation wait'); jobs.delete(job[0]); job[1]();},
    pending: () => jobs.size, cleared};
}

test('a channel retry-only 202 prepares the original start before any channel state', async () => {
  const server = new ChannelServer(), clock = preparationQueue();
  const posts: {key?: string; body: unknown}[] = [];
  const http: V1Http = {send: async request => {
    if (request.method === 'POST' && request.path === '/v1/playback/sessions') {
      posts.push({key: request.headers?.['Idempotency-Key'], body: request.body});
      if (posts.length <= 2) return {status: 202, headers: {}, body: {retryAfterMs: 500}};
    }
    return server.send(request);
  }};
  const control = new V1ChannelControl({http, key: () => 'channel-key-0000001', pollMs: 5, timers: clock.timers});
  const states: any[] = [], errors: string[] = [], preparation: any[] = [];
  try {
    const opening = control.open(channel, {state: value => states.push(value), error: message => errors.push(message), preparation: value => preparation.push(value)});
    await settle();
    assert.deepEqual(preparation, [{messageId: 'player.preparing.slowStart', retryAfterMs: 500}]);
    assert.deepEqual(states, [], 'no channel state exists before a real 201');
    assert.equal(clock.pending(), 1);
    clock.fire(); await settle();
    assert.deepEqual(preparation, [{messageId: 'player.preparing.slowStart', retryAfterMs: 500}, {messageId: 'player.preparing.slowStart', retryAfterMs: 500}]);
    assert.deepEqual(states, []);
    clock.fire(); await settle(); await opening;
    assert.equal(posts.length, 3);
    for (const post of posts) {assert.equal(post.key, 'channel-key-0000001'); assert.deepEqual(post.body, posts[0].body);}
    assert.deepEqual(posts[0].body, {channelId: v1ChannelId(channel), channelGeneration: 'g1', state: 'playing'});
    assert.equal(preparation.at(-1), undefined, 'acceptance clears the notice');
    assert.deepEqual(errors, []);
    assert.equal(states.length, 1);
    assert.equal(states[0].playbackId, 'ps_1');
    assert.equal(states[0].linear.name, 'News');
    await new Promise(resolve => setTimeout(resolve, 40)); await settle();
    assert.ok(server.log.some(request => request.method === 'PATCH' && (request.body as any).seek?.live === true), 'the honest generation-0/empty-URL 201 still joins');
  } finally { control.close(); }
});

test('a channel real-session 202 keeps one original ownership receipt and the actual projection', async () => {
  const server = new ChannelServer(), clock = preparationQueue();
  let posts = 0, captures = 0, abandoned = 0;
  const http: V1Http = {send: async request => {
    if (request.method === 'POST' && request.path === '/v1/playback/sessions') {
      posts++;
      if (posts === 1) {
        const real = await server.send(request);
        return {status: 202, headers: {}, body: {...(real.body as Record<string, unknown>), channelId: v1ChannelId(channel), retryAfterMs: 500}};
      }
    }
    return server.send(request);
  }};
  const control = new V1ChannelControl({http, key: () => 'channel-key-0000002', pollMs: 5, timers: clock.timers});
  const states: any[] = [], errors: string[] = [], preparation: any[] = [];
  try {
    const opening = control.open(channel, {state: value => states.push(value), error: message => errors.push(message), preparation: value => preparation.push(value)},
      {capturePreparing: () => {captures++; return () => {abandoned++;};}});
    await settle();
    assert.equal(captures, 1, 'the real 202 session is the original ownership receipt');
    assert.deepEqual(preparation, [{messageId: 'player.preparing.slowStart', retryAfterMs: 500}]);
    assert.deepEqual(states, []);
    clock.fire(); await opening; await new Promise(resolve => setTimeout(resolve, 40)); await settle();
    assert.equal(posts, 2); assert.equal(abandoned, 0);
    assert.ok(states.length >= 1);
    assert.equal(states[0].playbackId, 'ps_1');
    assert.equal(states[0].linear.name, 'News');
    assert.deepEqual(errors, []);
  } finally { control.close(); }
});

test('closing a channel during a valid preparation wait clears its timer and sends nothing more', async () => {
  const server = new ChannelServer(), clock = preparationQueue();
  let posts = 0;
  const http: V1Http = {send: async request => {
    if (request.method === 'POST' && request.path === '/v1/playback/sessions') {posts++; return {status: 202, headers: {}, body: {retryAfterMs: 500}};}
    return server.send(request);
  }};
  const control = new V1ChannelControl({http, key: () => 'channel-key-0000003', pollMs: 5, timers: clock.timers});
  const states: any[] = [], errors: string[] = [], preparation: any[] = [];
  const opening = control.open(channel, {state: value => states.push(value), error: message => errors.push(message), preparation: value => preparation.push(value)});
  await settle();
  assert.equal(posts, 1); assert.equal(clock.pending(), 1); assert.equal(preparation.length, 1);
  control.close(); await opening; await settle();
  assert.equal(clock.pending(), 0); assert.ok(clock.cleared.length >= 1);
  assert.equal(posts, 1); assert.deepEqual(states, []); assert.deepEqual(errors, []);
});

test('a channel notice callback can retire its own opening before a wait is installed', async () => {
  const server = new ChannelServer(), clock = preparationQueue();
  let posts = 0, notices = 0;
  const http: V1Http = {send: async request => {
    if (request.method === 'POST' && request.path === '/v1/playback/sessions') {posts++; return {status: 202, headers: {}, body: {retryAfterMs: 500}};}
    return server.send(request);
  }};
  const control = new V1ChannelControl({http, key: () => 'channel-key-0000004', pollMs: 5, timers: clock.timers});
  const states: any[] = [], errors: string[] = [];
  await control.open(channel, {state: value => states.push(value), error: message => errors.push(message),
    preparation: () => {notices++; control.close();}});
  await settle();
  assert.equal(notices, 1); assert.equal(posts, 1); assert.equal(clock.pending(), 0);
  assert.deepEqual(states, []); assert.deepEqual(errors, []);
});

test('an older channel completion cannot publish over its successor', async () => {
  const server = new ChannelServer();
  let posts = 0, release!: () => void;
  const held = new Promise<void>(resolve => {release = resolve;});
  const http: V1Http = {send: async request => {
    if (request.method === 'POST' && request.path === '/v1/playback/sessions') {
      posts++;
      if (posts === 1) {await held; return server.send(request);}
    }
    return server.send(request);
  }};
  let serial = 0;
  const control = new V1ChannelControl({http, key: () => `channel-key-${String(++serial).padStart(7, '0')}`, pollMs: 5});
  const statesOld: any[] = [], statesNew: any[] = [], errorsNew: string[] = [];
  try {
    const oldOpening = control.open(channel, {state: value => statesOld.push(value), error: () => {}});
    await settle();
    assert.equal(posts, 1);
    await control.open(channel, {state: value => statesNew.push(value), error: message => errorsNew.push(message)});
    await settle();
    assert.ok(statesNew.length >= 1);
    const before = statesNew.length;
    release(); await oldOpening; await settle();
    assert.equal(statesNew.length, before, 'the older completion published nothing over its successor');
    assert.deepEqual(errorsNew, []);
  } finally { control.close(); }
});
