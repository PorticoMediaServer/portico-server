import test from 'node:test';
import assert from 'node:assert/strict';

// A fake Cast framework and receiver: records what the sender sends and lets the test reply.
function fakeCast(resumed = false) {
  const sent: any[] = [];
  // Like the SDK: no current session until one is requested, unless Chrome rejoined one on load.
  let current = resumed;
  let listener: ((ns: string, raw: string) => void) | undefined;
  const session = {
    getCastDevice: () => ({friendlyName: 'Living Room TV', label: 'actual-sdk-id'}),
    sendMessage: async (_ns: string, body: any) => { sent.push(body); },
    addMessageListener: (_ns: string, fn: any) => { listener = fn; },
    removeMessageListener: () => { listener = undefined; },
    endSession: () => {},
  };
  const context = {setOptions() {}, requestSession: async () => { current = true; }, getCurrentSession: () => (current ? session : null), addEventListener() {}, removeEventListener() {}};
  const g = globalThis as any;
  g.window = g;
  g.chrome = {cast: {AutoJoinPolicy: {ORIGIN_SCOPED: 'origin_scoped'}}};
  g.cast = {framework: {CastContext: {getInstance: () => context}, CastContextEventType: {SESSION_STATE_CHANGED: 'x'}, SessionState: {SESSION_ENDED: 'ended', SESSION_START_FAILED: 'failed'}}};
  return {sent, reply: (m: unknown) => listener?.('ns', JSON.stringify(m))};
}
const api = {
  request: async (path: string) => (path === '/v1/cast/configuration' ? {applicationId: 'APP'} : {bootstrap: {code: 'ABC123'}}),
  mediaUrl: (p: string) => `https://server.example${p}`,
};
const tick = () => new Promise(r => setTimeout(r, 0));

test('Cast pairs with a display name, waits through pair-pending, and loads with display metadata', async () => {
  const tv = fakeCast();
  const {CastSender} = await import('../src/bridge/cast.ts');
  const sender = new CastSender(api, 'Chrome on Mac');
  await sender.prepare();
  assert.equal(sender.getSnapshot().available, true);
  const started = sender.start('item-1', {title: 'The Fix', subtitle: '2026', kind: 'movie', artworkPath: '/v1/metadata/item-1/art?kind=poster'}, 42);
  await tick(); await tick();
  assert.equal(tv.sent[0].type, 'pair');
  assert.equal(tv.sent[0].code, 'ABC123');
  assert.equal(tv.sent[0].origin, 'https://server.example');
  assert.equal(tv.sent[0].displayName, 'Chrome on Mac');
  tv.reply({type: 'pair-pending'});
  assert.equal(sender.getSnapshot().phase, 'waiting');
  tv.reply({type: 'paired'});
  await tick();
  assert.equal(sender.getSnapshot().phase, 'waiting', 'transport send has not acknowledged playback');
  tv.reply({type:'loaded',requestId:tv.sent[1].requestId,itemId:'item-1'});
  assert.equal(await started, true);
  assert.equal(tv.sent[1].type, 'load');
  assert.equal(tv.sent[1].itemId, 'item-1');
  assert.equal(tv.sent[1].title, 'The Fix');
  assert.equal(tv.sent[1].artworkPath, '/v1/metadata/item-1/art?kind=poster');
  assert.equal(sender.getSnapshot().phase, 'casting');

  // Someone else asks; the controller answers the correlation it was given.
  const controller = {bindingId: 'cct_current', controllerGeneration: 1, connectionGeneration: 1};
  tv.reply({type: 'takeover-requested', requester: 'Sam’s Pixel', controller, requestId: 'cbs_OTHER1'});
  assert.deepEqual(sender.getSnapshot().takeover, {requester: 'Sam’s Pixel', controller, requestId: 'cbs_OTHER1'});
  sender.answerTakeover(false);
  assert.deepEqual(tv.sent.at(-1), {type: 'takeover', allow: false, controller, requestId: 'cbs_OTHER1'});
  assert.equal(sender.getSnapshot().takeover, undefined);

  // Losing the TV to another sender ends the bar with its own message.
  tv.reply({type: 'replaced'});
  assert.equal(sender.getSnapshot().phase, 'replaced');
  sender.dismiss();
});

test('Cast shows the TV’s tv_busy message as is and never raw codes', async () => {
  const tv = fakeCast();
  const {CastSender} = await import('../src/bridge/cast.ts');
  const sender = new CastSender(api, 'Chrome on Mac');
  await sender.prepare();
  const started = sender.start('item-1', {title: 'The Fix'}, 0);
  await tick(); await tick();
  tv.reply({type: 'pair-failed', code: 'tv_busy', message: 'Sam is watching on this TV right now.'});
  assert.equal(await started, false);
  assert.ok(sender.getSnapshot().error);
  assert.doesNotMatch(sender.getSnapshot().error ?? '', /tv_busy/);

  const again = sender.start('item-1', {title: 'The Fix'}, 0);
  await tick(); await tick();
  tv.reply({type: 'pair-failed', code: 'pair_code_expired', message: 'internal: code 0x1f expired'});
  assert.equal(await again, false);
  assert.doesNotMatch(sender.getSnapshot().error ?? '', /0x1f|internal/);
});

test('Cast display name names the browser and platform only', async () => {
  const {castDisplayName} = await import('../src/bridge/cast.ts');
  assert.equal(castDisplayName('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36'), 'Chrome on Mac');
  assert.equal(castDisplayName('Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36 Edg/140.0'), 'Edge on Windows');
});

test('CAST-05: after a reload the sender adopts the session Chrome rejoined, with the title', async t => {
  const tv = fakeCast(true);
  const {CastSender} = await import('../src/bridge/cast.ts');
  const {castProofKey} = await import('@core/cast/index.ts');
  // The proof this browser stored when it paired this exact receiver, keyed by
  // the original session identity and the SDK's receiver id.
  const context = {origin: 'https://server.example', publicKey: 'A'.repeat(43), serverId: 'server', authority: 'local',
    accountId: 'account', profileId: 'profile', senderDeviceId: 'device', senderFamilyId: 'family'};
  const scope = {serverId: 'server', authority: 'local', accountId: 'account', profileId: 'profile', senderDeviceId: 'device', senderFamilyId: 'family'};
  const binding = {id: 'cct_current', bootstrapId: 'cbs_prior', receiverId: 'actual-sdk-id', targetId: 'cdv_target', targetFamilyId: 'target-family', targetGeneration: '1', scope};
  const proof = {version: 2, context, binding, controlToken: 'a'.repeat(48)};
  const store = new Map<string, string>([[castProofKey(context, 'actual-sdk-id'), JSON.stringify(proof)]]);
  (globalThis as any).localStorage = {getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => { store.set(k, v); },
    removeItem: (k: string) => { store.delete(k); }, get length() { return store.size; }, key: (i: number) => [...store.keys()][i] ?? null};
  t.after(() => { delete (globalThis as any).localStorage; });
  const titled = {...api, request: async (path: string, _method?: string, body?: unknown) => {
    if (path === '/v1/items/item-9') return {title: 'Jurassic Park'};
    if (path === '/v1/cast/controllers/validate') return {binding, attemptId: (body as {attemptId?: unknown}).attemptId, ticket: 't'.repeat(43)};
    return api.request(path);
  }};
  const sender = new CastSender(titled, 'Chrome on Mac', context);
  await sender.prepare();
  const settle = async () => { for (let i = 0; i < 12; i++) await tick(); };
  await settle();
  const adopt = tv.sent[0] as {type?: string; bindingId?: string; attemptId?: string; ticket?: string; controlToken?: string};
  assert.equal(adopt.type, 'adopt');
  assert.equal(adopt.bindingId, binding.id);
  assert.equal(adopt.controlToken, proof.controlToken);
  assert.match(String(adopt.attemptId), /^adopt-/);assert.equal(adopt.ticket, 't'.repeat(43));
  // The receiver answers the adopted attempt with its own controller/occurrence
  // witness; only then does the bar show casting, with the described title.
  const controller = {bindingId: 'cct_current', controllerGeneration: 1, connectionGeneration: 1};
  tv.reply({type: 'status', attemptId: adopt.attemptId, controller, occurrence: {sessionId: 'ps_cast', generation: 1, itemId: 'item-9'},
    itemId: 'item-9', positionSeconds: 1834, durationSeconds: 7620, paused: false, audioTracks: [],
    textTracks: [{id: 3, language: 'en', name: 'English'}]});
  await settle();
  const s = sender.getSnapshot();
  assert.equal(s.phase, 'casting');
  assert.equal(s.title, 'Jurassic Park');
  assert.equal(s.positionSeconds, 1834);
  assert.deepEqual(s.remoteIdentity, {controller, occurrence: {sessionId: 'ps_cast', generation: 1, itemId: 'item-9'}});
  assert.deepEqual(s.reported, {itemId: 'item-9', positionSeconds: 1834, durationSeconds: 7620, paused: false});
  sender.setSubtitleTrack(3);
  assert.deepEqual(tv.sent.at(-1), {type: 'subtitles', trackIds: [3], target: s.remoteIdentity});
  sender.dispose();
});
