import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {fakeContext} from './helpers/fake-audio.ts';

/**
 * DecodeAudioRender against the playback service contract, in node: a fake AudioContext whose
 * clock the test moves, the §18.8 FLAC fixture served with Range, and a recording service.
 */
const dir = new URL('../../fixtures/audio-gapless/', import.meta.url);
const file = (name: string) => new Uint8Array(readFileSync(new URL(name, dir)));
const sidecar = (name: string) => JSON.parse(readFileSync(new URL(name, dir), 'utf8'));

function setup() {
  const f = fakeContext(44100);
  const requests: string[] = [];
  const files: Record<string, Uint8Array> = {'/a': file('album-flac-01.flac'), '/b': file('album-flac-02.flac')};
  (globalThis as any).AudioContext = class { constructor() { Object.assign(this, f.ctx); (this as any).suspend = async () => {}; (this as any).resume = async () => {}; (this as any).close = async () => {}; return f.ctx; } };
  f.ctx.suspend = async () => { f.ctx.state = 'suspended'; }; f.ctx.resume = async () => { f.ctx.state = 'running'; }; f.ctx.close = async () => {};
  (globalThis as any).document = {addEventListener() {}, removeEventListener() {}};
  (globalThis as any).fetch = async (url: string, init: RequestInit) => {
    requests.push(url);
    const bytes = files[url]!, [a, b] = String((init.headers as Record<string, string>).Range).replace('bytes=', '').split('-').map(Number);
    return new Response(bytes.slice(a!, Math.min(bytes.length, b! + 1)), {status: 206});
  };
  const plan = (id: string, s: any) => ({version: 2, mode: 'direct', id, url: '/' + id, container: 'flac', codec: 'flac', sampleRate: 44100, channels: 2, bytes: files['/' + id]!.length, prefetchBytes: 1 << 20, durationFrames: s.durationFrames, trim: s.trim});
  const a = plan('a', sidecar('album-flac-01.json')), b = plan('b', sidecar('album-flac-02.json'));
  const calls: unknown[][] = [];
  let snapshot: any = {session: {id: 's1', generation: 1, audio: a}, itemId: 'i1', phase: 'ready', intent: 'playing', intentId: 1, positionSeconds: 0, audioEffects: {rendering: true, rate: 1, settings: {gapless: true, crossfadeSeconds: 0, normalization: 'off'}, notice: ''}};
  const service: any = new Proxy({getSnapshot: () => snapshot}, {get: (t, k) => k in t ? (t as any)[k] : (...args: unknown[]) => { calls.push([k, ...args]); }});
  return {f, requests, a, b, calls, service, set: (patch: any) => { snapshot = {...snapshot, ...patch, audioEffects: {...snapshot.audioEffects, ...(patch.audioEffects ?? {})}}; return snapshot; }, get: () => snapshot};
}
const settle = async () => { for (let i = 0; i < 20; i++) await new Promise(r => setTimeout(r, 0)); };

test('(e) pause, seek or a rate change drops a prepared edge: nothing is committed, the next track never plays', async () => {
  const {DecodeAudioRender} = await import('../src/bridge/DecodeAudioRender.ts');
  const t = setup();
  const render = new DecodeAudioRender(t.service, (p: string) => p, {addEventListener() {}, removeEventListener() {}, volume: 1, muted: false} as any);
  render.apply(t.get()); await settle();
  assert.ok(t.calls.some(c => c[0] === 'ready'), 'the first track started');
  render.apply(t.set({audioEffects: {prepared: {token: 'pn_1', expiresAtMs: Date.now() + 60000, audio: t.b}}})); await settle();
  assert.ok(t.calls.some(c => c[0] === 'audioPrepared' && c[1] === 'pn_1'));
  // The service clears the preparation on pause (and on seek or rate).
  render.apply(t.set({intent: 'paused', audioEffects: {prepared: undefined}})); await settle();
  t.f.advance(3);
  (render as any).tick();
  assert.equal(t.calls.filter(c => c[0] === 'audioCommit').length, 0, 'no commit after the edge was dropped');
  assert.equal((render as any).candidate, undefined);
  render.dispose();
});

test('(f) a commit that fails falls back: the track ends and says so, for :advance', async () => {
  const {DecodeAudioRender} = await import('../src/bridge/DecodeAudioRender.ts');
  const t = setup();
  const render = new DecodeAudioRender(t.service, (p: string) => p, {addEventListener() {}, removeEventListener() {}, volume: 1, muted: false} as any);
  render.apply(t.get()); await settle();
  render.apply(t.set({audioEffects: {prepared: {token: 'pn_1', expiresAtMs: Date.now() + 60000, audio: t.b}}})); await settle();
  // Near the end: the engine commits.
  for (let i = 0; i < 30 && !t.calls.some(c => c[0] === 'audioCommit'); i++) { t.f.advance(0.1); (render as any).tick(); await settle(); }
  assert.ok(t.calls.some(c => c[0] === 'audioCommit' && c[1] === 'pn_1'), 'committed within crossfade + 2 s of the edge');
  // The commit fails: the queue discards the preparation.
  render.apply(t.set({audioEffects: {prepared: undefined}})); await settle();
  for (let i = 0; i < 60 && !t.calls.some(c => c[0] === 'fact' && c[3] === 'ended'); i++) { t.f.advance(0.1); (render as any).tick(); await settle(); }
  assert.ok(t.calls.some(c => c[0] === 'fact' && c[3] === 'ended'), 'the track ends, so the queue advances with reason completion');
  render.dispose();
});

test('(b)(i) a committed edge promotes at the boundary; the engine keeps a constant number of decks however many edges pass', async () => {
  const {DecodeAudioRender} = await import('../src/bridge/DecodeAudioRender.ts');
  const t = setup();
  const render = new DecodeAudioRender(t.service, (p: string) => p, {addEventListener() {}, removeEventListener() {}, volume: 1, muted: false} as any);
  render.apply(t.get()); await settle();
  let current = t.a, next = t.b, n = 1;
  for (let edge = 0; edge < 6; edge++, n++) {
    const token = 'pn_' + edge;
    render.apply(t.set({audioEffects: {prepared: {token, expiresAtMs: Date.now() + 60000, audio: next}}})); await settle();
    for (let i = 0; i < 40 && !t.calls.some(c => c[0] === 'audioCommit' && c[1] === token); i++) { t.f.advance(0.1); (render as any).tick(); await settle(); }
    const committedSession = {id: 's' + (n + 1), generation: 1, audio: next};
    render.apply(t.set({audioEffects: {committed: {token, itemId: 'i' + (n + 1), session: committedSession}}})); await settle();
    const edgeFrame = (render as any).incoming.edge;
    assert.equal(edgeFrame, (render as any).primary.deck.endFrame, 'gapless: the next track starts at the frame after the last');
    for (let i = 0; i < 40 && !t.calls.some(c => c[0] === 'audioBoundary' && c[1] === token); i++) { t.f.advance(0.1); (render as any).tick(); await settle(); }
    assert.ok(t.calls.some(c => c[0] === 'audioBoundary' && c[1] === token));
    render.apply(t.set({session: committedSession, itemId: 'i' + (n + 1), audioEffects: {prepared: undefined, committed: undefined}})); await settle();
    const open = ['primary', 'candidate', 'incoming', 'retiring'].filter(k => (render as any)[k]).length;
    assert.ok(open <= 2, `edge ${edge}: ${open} decks open`);
    [current, next] = [next, current];
  }
  assert.equal(t.calls.filter(c => c[0] === 'ready').length, 1, 'no restart at any edge');
  render.dispose();
});

test('(h) an unavailable plan plays ordinarily and says why, quietly', async () => {
  const {renderable, renderNotice} = await import('../../packages/client-core/src/index.ts');
  const s = {audio: {version: 2 as const, mode: 'unavailable' as const, id: 'm', reason: 'The owner doesn’t allow converting audio, and this device can’t decode ALAC.'}};
  assert.equal(renderable(s), false, 'no effects engine: the ordinary player plays presentation.url');
  assert.equal(renderNotice(s), s.audio.reason);
});
