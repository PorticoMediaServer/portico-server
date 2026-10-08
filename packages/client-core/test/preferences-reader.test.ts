import assert from 'node:assert/strict';
import test from 'node:test';
import {PreferencesReader,type PreferencesApi} from '../src/preferences-reader.ts';

const fence = 'a'.repeat(64);
const envelope = (data: unknown, serverId = 'server-one', viewerFence = fence) => ({scope: {serverId, viewerFence}, data});
const digest = 'a'.repeat(64);
const fields = [
  {key: 'search.rememberHistory', type: 'boolean', default: true, scopes: ['profile-server'], group: 'search', labelKey: 'preferences.search.rememberHistory'},
  {key: 'appearance.reduceMotion', type: 'boolean', default: false, scopes: ['profile-device-class'], group: 'appearance', labelKey: 'preferences.appearance.reduceMotion'},
];
const snapshot = (revision = 1) => ({
  deviceClass: 'web', registryRevision: 'p39.A1', registry: {revision: 'p39.A1', fields},
  documents: [
    {scope: 'profile-server', revision, digest, activeRevision: revision, values: {'search.rememberHistory': false}},
    {scope: 'profile-device-class', revision: 1, digest, activeRevision: 1, values: {}},
  ],
  effective: {'search.rememberHistory': false, 'appearance.reduceMotion': false},
  effectiveSource: {'search.rememberHistory': 'profile-server', 'appearance.reduceMotion': 'default'},
  clampedFields: [],
});

function reader(handler: (path: string, method: string, body: unknown, signal: AbortSignal) => Promise<unknown>, viaConsole = true) {
  const api: PreferencesApi = viaConsole
    ? {async request<T>(): Promise<T> { throw new Error('unbounded fallback must not be used'); }, async requestConsole<T>(path: string, method: string, body: unknown, signal: AbortSignal) { return await handler(path, method, body, signal) as T; }}
    : {async request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal) { return await handler(path, method ?? 'GET', body, signal ?? AbortSignal.timeout(1000)) as T; }};
  return new PreferencesReader(api, {serverId: 'server-one', viewerId: 'local:one:profile'});
}

test('preferences read parses the published snapshot without the console client', async () => {
  const seen: {path: string; method: string}[] = [];
  const r = reader(async (path, method) => { seen.push({path, method}); return envelope(snapshot()); });
  const parsed = await r.preferences('web');
  assert.equal(parsed.effective['search.rememberHistory'], false);
  assert.deepEqual(seen, [{path: '/v1/preferences?deviceClass=web', method: 'GET'}]);
  r.dispose();
});

test('the plain request transport works when requestConsole is absent', async () => {
  const r = reader(async () => envelope(snapshot()), false);
  assert.equal((await r.preferences('web')).deviceClass, 'web');
  r.dispose();
});

test('a changed server or fence ends the scope instead of mixing viewers', async () => {
  let foreign = false;
  const r = reader(async () => envelope(snapshot(), foreign ? 'server-two' : 'server-one'));
  await r.preferences('web');
  foreign = true;
  await assert.rejects(r.preferences('web'), /selected viewer changed/);
  r.dispose();
  let changed = false;
  const fenced = reader(async () => envelope(snapshot(), 'server-one', changed ? 'b'.repeat(64) : fence));
  await fenced.preferences('web');
  changed = true;
  await assert.rejects(fenced.preferences('web'), /selected viewer changed/);
  fenced.dispose();
});

test('a patch carries scope, revision, values and an idempotency key', async () => {
  let sent: Record<string, unknown> = {};
  const r = reader(async (path, method, body) => {
    assert.equal(path, '/v1/preferences');
    assert.equal(method, 'PATCH');
    sent = body as Record<string, unknown>;
    return envelope(snapshot(2));
  });
  const next = await r.applyPreferences('profile-server', 'web', 1, {'search.rememberHistory': true});
  assert.equal(sent.scope, 'profile-server');
  assert.equal(sent.deviceClass, 'web');
  assert.equal(sent.expectedRevision, 1);
  assert.deepEqual(sent.values, {'search.rememberHistory': true});
  assert.ok(typeof sent.idempotencyKey === 'string' && sent.idempotencyKey.length > 0);
  assert.equal(next.documents[0]!.revision, 2);
  r.dispose();
});

test('disposal ends in-flight work and refuses new reads', async () => {
  let resolve!: (v: unknown) => void;
  const r = reader(() => new Promise(res => { resolve = res; }));
  const pending = r.preferences('web');
  r.dispose();
  resolve(envelope(snapshot()));
  await assert.rejects(pending);
  await assert.rejects(r.preferences('web'), /closed/);
});

test('a selected viewer is required', () => {
  assert.throws(() => new PreferencesReader({request: async () => { throw new Error('unused'); }}, {serverId: '', viewerId: 'v'}));
});
