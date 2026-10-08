import test from 'node:test';
import assert from 'node:assert/strict';

// A fake Cast framework and receiver: records what the sender sends and lets the test reply.
function fakeCast(resumed = false) {
  const sent: any[] = [];
  // Like the SDK: no current session until one is requested, unless Chrome rejoined one on load.
  let current = resumed;
  let listener: ((ns: string, raw: string) => void) | undefined;
  const session = {
    getCastDevice: () => ({friendlyName: 'Living Room TV'}),
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
  assert.equal(await started, true);
  assert.equal(tv.sent[1].type, 'load');
  assert.equal(tv.sent[1].itemId, 'item-1');
  assert.equal(tv.sent[1].title, 'The Fix');
  assert.equal(tv.sent[1].artworkPath, '/v1/metadata/item-1/art?kind=poster');
  assert.equal(sender.getSnapshot().phase, 'casting');

  // Someone else asks; the controller answers.
  tv.reply({type: 'takeover-requested', requester: 'Sam’s Pixel'});
  assert.deepEqual(sender.getSnapshot().takeover, {requester: 'Sam’s Pixel'});
  sender.answerTakeover(false);
  assert.deepEqual(tv.sent.at(-1), {type: 'takeover', allow: false});
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

test('CAST-05: after a reload the sender adopts the session Chrome rejoined, with the title', async () => {
  const tv = fakeCast(true);
  const {CastSender} = await import('../src/bridge/cast.ts');
  const titled = {...api, request: async (path: string) => (path === '/v1/items/item-9' ? {title: 'Jurassic Park'} : api.request(path))};
  const sender = new CastSender(titled, 'Chrome on Mac');
  await sender.prepare();
  await tick();
  assert.deepEqual(tv.sent, [{type: 'status'}]);
  tv.reply({type: 'status', itemId: 'item-9', positionSeconds: 1834, durationSeconds: 7620, paused: false, audioTracks: [], textTracks: [{id: 3, language: 'en', name: 'English'}]});
  await tick(); await tick();
  const s = sender.getSnapshot();
  assert.equal(s.phase, 'casting');
  assert.equal(s.title, 'Jurassic Park');
  assert.equal(s.positionSeconds, 1834);
  sender.setSubtitleTrack(3);
  assert.deepEqual(tv.sent.at(-1), {type: 'subtitles', trackIds: [3]});
  sender.dispose();
});
