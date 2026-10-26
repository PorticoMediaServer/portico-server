import assert from 'node:assert/strict';
import test from 'node:test';
import {setImmediate as settle} from 'node:timers/promises';
import type {HttpLocalApi, LocalSession} from '../../packages/client-core/src/index.ts';
import {V1ChannelControl, type OriginalChannelStart} from '../../packages/client-core/src/playback-v1/channel-control.ts';
import {parseStartPreparation, type Timers} from '../../packages/client-core/src/playback-v1/sessions.ts';
import type {V1Request} from '../../packages/client-core/src/playback-v1/http.ts';
import {createWebPreparingStarts} from '../src/platform/playback/start-owner.ts';

// Authored source vectors, UNRUN. Only the original fake API and canonical SDK
// are used; no network, browser, tuner, actual timer or server preparation job.
const real = (id = 'original_channel') => ({id, revision: '1', kind: 'live', role: 'local', state: 'playing', channelId: 'live:source:station',
  lease: {expiresAt: '2099-01-01T00:00:00Z', reportEveryMs: 10_000},
  presentation: {generation: 1, mode: 'stream', engine: 'native', url: '', startPositionMs: 0, quality: {mode: 'original'}, subtitles: [], decision: {}}});
const response = (body: unknown, status = 202) => ({body, status, headers: {}});
function deferred<T>() {let resolve!: (value: T) => void;const promise = new Promise<T>(yes => {resolve = yes;});return {promise, resolve};}
function fixture() {
  let principal = true, view = true;
  const session: LocalSession = {accessToken: 'synthetic', expiresAt: '2099-01-01T00:00:00Z', authorizationHorizon: '2099-01-02T00:00:00Z',
    tokenGeneration: '1', sessionFamilyId: 'family', deviceId: 'device', installationId: 'installation',
    viewer: {serverId: 'server', authority: 'local', accountId: 'account', profileId: 'profile', role: 'owner'}};
  const route = {pin: {serverId: 'server', publicKey: 'pin'}, getSnapshot: () => ({phase: 'ready'})};
  const calls: {method: string; path: string; signal?: AbortSignal}[] = [], reads: Promise<unknown>[] = [], stops: Promise<unknown>[] = [];
  const api = {baseUrl: 'https://original.invalid', getRouteConnection: () => route,
    requestRaw: async (path: string, method: string, options: {signal?: AbortSignal}) => {
      calls.push({path, method, signal: options.signal});
      return response(method === 'GET' ? await (reads.shift() ?? Promise.resolve(real())) : await (stops.shift() ?? Promise.resolve(undefined)), method === 'GET' ? 200 : 204);
    }} as unknown as HttpLocalApi;
  const owner = createWebPreparingStarts(api, session, () => session, () => principal)!, reg = owner.attach();
  const original = (key = 'channel-key'): OriginalChannelStart => Object.freeze({body: Object.freeze({channelId: 'live:source:station', channelGeneration: 'original-guide', state: 'playing' as const}),
    key, signal: new AbortController().signal, isCurrent: () => view});
  return {owner, reg, original, calls, reads, stops, route, retireView: () => {view = false;}, retirePrincipal: () => {principal = false;}, current: () => view};
}

test('genuine channel 202 owns only the original channel read and once-only bounded stop', async () => {
  const f = fixture(), start = f.reg.beginChannel(f.original(), f.current);
  try {
    const cleanup = start.capturePreparing(parseStartPreparation({...real(), retryAfterMs: 500}).session!)!;
    f.retireView();assert.equal(start.isCurrent(), false);await cleanup();await cleanup();start.finish();
    assert.deepEqual(f.calls.map(call => [call.method, call.path]), [['GET', '/v1/playback/sessions/original_channel'], ['DELETE', '/v1/playback/sessions/original_channel']]);
    assert.equal(f.calls[0]!.signal, f.calls[1]!.signal);
  } finally {f.reg.dispose();f.owner.dispose();}
});

test('late real success facts are read under original channel custody, never converted to a Session', async () => {
  const f = fixture(), original = f.original(), start = f.reg.beginChannel(original, f.current);
  try {
    f.retireView();await start.retireUninstalled(Object.freeze({id: 'original_channel', generation: 1}));start.finish();
    assert.deepEqual(f.calls.map(call => call.method), ['GET', 'DELETE']);
    assert.ok(Object.isFrozen(original.body));assert.equal(original.body.channelGeneration, 'original-guide');
  } finally {f.reg.dispose();f.owner.dispose();}
});

test('a changed actual generation, channel, kind or ownership refuses cleanup', async () => {
  for (const changed of [
    {...real(), presentation: {...real().presentation, generation: 2}}, {...real(), channelId: 'live:other:station'},
    {...real(), role: 'receiver'}, {...real(), kind: 'vod'}, {...real(), state: 'ended'},
    {...real(), queue: {queueId: 'queue', entryId: 'entry'}}, {...real(), itemId: 'movie'},
  ]) {
    const f = fixture(), start = f.reg.beginChannel(f.original(), f.current);f.reads.push(Promise.resolve(changed));
    try {await start.retireUninstalled({id: 'original_channel', generation: 1});start.finish();assert.deepEqual(f.calls.map(call => call.method), ['GET']);}
    finally {f.reg.dispose();f.owner.dispose();}
  }
});

test('a first actual 202 can never be replaced by late success facts for another id or generation', async () => {
  const f = fixture(), start = f.reg.beginChannel(f.original(), f.current);
  try {
    const cleanup = start.capturePreparing(parseStartPreparation({...real(), retryAfterMs: 500}).session!)!;
    await start.retireUninstalled({id: 'other', generation: 1});await start.retireUninstalled({id: 'original_channel', generation: 2});
    assert.equal(f.calls.length, 0);await cleanup();start.finish();assert.equal(f.calls.length, 2);
  } finally {f.reg.dispose();f.owner.dispose();}
});

test('an installed then forgotten channel permanently denies its older original receipt', async () => {
  const f = fixture(), next = f.owner.attach(), start = f.reg.beginChannel(f.original(), f.current);
  try {
    const cleanup = start.capturePreparing(parseStartPreparation({...real(), retryAfterMs: 500}).session!)!;
    next.remember('original_channel');next.forget('original_channel');await cleanup();start.finish();assert.equal(f.calls.length, 0);
  } finally {next.dispose();f.reg.dispose();f.owner.dispose();}
});

test('installation while the original channel GET is held aborts that cleanup before stop', async () => {
  const f = fixture(), read = deferred<unknown>(), next = f.owner.attach();f.reads.push(read.promise);
  const start = f.reg.beginChannel(f.original(), f.current);
  try {
    const pending = Promise.resolve(start.retireUninstalled({id: 'original_channel', generation: 1}));
    const rejected = assert.rejects(pending, {name: 'AbortError'});await settle();next.remember('original_channel');read.resolve(real());await rejected;start.finish();
    assert.deepEqual(f.calls.map(call => call.method), ['GET']);assert.equal(f.calls[0]!.signal?.aborted, true);
  } finally {next.dispose();f.reg.dispose();f.owner.dispose();}
});

test('original principal or pin retirement refuses even a real late channel resource', async () => {
  for (const retire of [(f: ReturnType<typeof fixture>) => f.retirePrincipal(), (f: ReturnType<typeof fixture>) => {f.route.pin.publicKey = 'replacement';}]) {
    const f = fixture(), start = f.reg.beginChannel(f.original(), f.current);
    try {retire(f);await start.retireUninstalled({id: 'original_channel', generation: 1});start.finish();assert.equal(f.calls.length, 0);}
    finally {f.reg.dispose();f.owner.dispose();}
  }
});

function controlFixture(realPreparation: boolean) {
  const f = fixture(), caller = new AbortController(), calls: V1Request[] = [], jobs = new Map<unknown, () => void>();
  const timers: Timers = {setTimer(fn) {const id = {};jobs.set(id, fn);return id;}, clearTimer(id) {jobs.delete(id);}};
  const replies: Promise<ReturnType<typeof response>>[] = [];
  const control = new V1ChannelControl({key: () => 'actual-channel-key', timers, http: {send: async request => {
    calls.push(request);return await (replies.shift() ?? Promise.resolve(response(realPreparation ? {...real(), retryAfterMs: 500} : {retryAfterMs: 500})));
  }}});
  const start = () => control.open({kind: 'live-source', sourceId: 'source', channelId: 'station', generation: 'original-guide'},
    {state: () => {assert.fail('no late install');}, error: () => {assert.fail('no successor error');}},
    {signal: caller.signal, isCurrent: f.current, captureStart: original => f.reg.beginChannel(original, f.current)});
  return {...f, cleanupCalls: f.calls, control, caller, calls, jobs, replies, start};
}

test('actual channel SDK current real202 then wait-abort spends only its original host receipt', async () => {
  const f = controlFixture(true);
  try {const pending = f.start();await settle();assert.equal(f.jobs.size, 1);f.caller.abort();await pending;await settle();
    assert.deepEqual(f.calls.map(call => call.method), ['POST']);assert.equal(f.jobs.size, 0);
    assert.deepEqual(f.cleanupCalls.map(call => call.method), ['GET', 'DELETE']);
    assert.deepEqual(f.calls[0]!.body, {channelId: 'live:source:station', channelGeneration: 'original-guide', state: 'playing'});
    assert.deepEqual(f.calls[0]!.headers, {'Idempotency-Key': 'actual-channel-key'});
  } finally {f.control.close();f.reg.dispose();f.owner.dispose();}
});

test('actual channel SDK retry-only abort has no resource or cleanup', async () => {
  const f = controlFixture(false);
  try {const pending = f.start();await settle();f.caller.abort();await pending;await settle();assert.equal(f.jobs.size, 0);assert.equal(f.calls.length, 1);assert.equal(f.cleanupCalls.length, 0);}
  finally {f.control.close();f.reg.dispose();f.owner.dispose();}
});

test('actual channel SDK late201 uses its original host facts path, with no bare DELETE', async () => {
  const f = controlFixture(false), held = deferred<ReturnType<typeof response>>();f.replies.push(held.promise);
  try {const pending = f.start();await settle();f.caller.abort();held.resolve(response(real(), 201));await pending;await settle();
    assert.deepEqual(f.calls.map(call => call.method), ['POST']);assert.equal(f.jobs.size, 0);
    assert.deepEqual(f.cleanupCalls.map(call => call.method), ['GET', 'DELETE']);
  } finally {f.control.close();f.reg.dispose();f.owner.dispose();}
});
