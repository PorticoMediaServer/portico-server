import test from 'node:test';
import assert from 'node:assert/strict';
import {PlaybackService} from '../src/index.ts';
import {V1ChannelControl, channelEndMessage, v1ChannelId} from '../src/playback-v1/channel-control.ts';
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
      this.s = {id: 'ps_1', revision: '1', kind: 'channel', state: 'playing', presentation: {generation: 0, url: '', subtitles: [], decision: {},
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
