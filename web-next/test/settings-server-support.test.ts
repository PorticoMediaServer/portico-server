import test from 'node:test';
import assert from 'node:assert/strict';
import {SettingsSupportSource} from '../src/screens/settings/server/troubleshooting/support-source.ts';

const serverId = 'captured-server', fence = 'a'.repeat(64);
const envelope = (data: unknown) => ({scope: {serverId, viewerFence: fence}, data});
const capabilities = {items: [
  {capability: 'live_tv', available: true, checkedAt: '2026-10-22T18:00:00Z'},
  {capability: 'hardware_encoding', available: false, code: 'hardware_unavailable', detail: 'No usable encoder.', checkedAt: '2026-10-22T18:00:00Z'},
]};
const alert = {id: 'permissions', code: 'state-permissions', status: 'open', severity: 'warning', firstAt: '2026-10-22T17:00:00Z', lastAt: '2026-10-22T18:00:00Z', occurrences: 3, revision: 1};
const ready = {id: 'export-1', expiresAt: '2026-10-25T18:00:00Z', manifest: {version: '1', from: '2026-10-21T18:00:00Z', to: '2026-10-22T18:00:00Z',
  included: ['runtime', 'client'], excluded: ['media'], redaction: 'on', maxBytes: 1048576}};

function fixture() {
  const calls: {path: string; method: string; body: unknown}[] = [];
  let permissions = true;
  const transport = {request: async <T>(path: string, method = 'GET', body?: unknown) => {
    calls.push({path, method, body});
    if (path.includes('/diagnostics/capabilities')) return capabilities as T;
    if (path.includes('/state-permissions:fix')) {permissions = false;return undefined as T;}
    if (path.includes('/console/alerts')) return envelope(permissions ? [alert] : []) as T;
    if (path.includes('/console/exports/')) return envelope({...ready, snapshot: {anything: true}}) as T;
    if (path.includes('/console/exports')) {
      const sent = body as {from: string; to: string; components: string[]};
      assert.deepEqual(sent.components, ['runtime', 'client', 'audit', 'snapshot']);
      return envelope(ready) as T;
    }
    return envelope({items: [], nextCursor: ''}) as T;
  }};
  return {source: new SettingsSupportSource(transport, serverId, 'captured-viewer', () => true), calls};
}

test('Support reads what the server can do and the live permissions warning, in one read', async () => {
  const f = fixture();
  try {
    await f.source.load();
    const state = f.source.getSnapshot();
    assert.equal(state.phase, 'ready');
    assert.deepEqual(state.capabilities.map(row => row.capability), ['live_tv', 'hardware_encoding']);
    assert.equal(state.capabilities[1].code, 'hardware_unavailable');
    assert.equal(state.permissions, true);
    assert.ok(Array.isArray(state.skipped));
  } finally {f.source.dispose();}
});

test('Prepare export sends the four drawn parts over the last day and keeps the server receipt', async () => {
  const f = fixture();
  try {
    await f.source.load();
    await f.source.prepare(24 * 60 * 60 * 1000, ['runtime', 'client', 'audit', 'snapshot']);
    const created = f.calls.find(call => call.method === 'POST' && call.path.endsWith('/console/exports'));
    const sent = created?.body as {from: string; to: string};
    assert.equal(Date.parse(sent.to) - Date.parse(sent.from), 24 * 60 * 60 * 1000);
    assert.equal(f.source.getSnapshot().export?.id, 'export-1');
    const bundle = await f.source.download();
    assert.deepEqual(bundle.snapshot, {anything: true});
  } finally {f.source.dispose();}
});

test('Fixing permissions clears the warning only after the server accepts the fix', async () => {
  const f = fixture();
  try {
    await f.source.load();
    assert.equal(await f.source.fixPermissions(), true);
    assert.equal(f.source.getSnapshot().permissions, false);
    assert.equal(f.source.getSnapshot().fixed, true);
  } finally {f.source.dispose();}
});

test('a retired scope reads, exports and fixes nothing more', async () => {
  const f = fixture();
  try {
    await f.source.load();
    f.source.dispose();
    const count = f.calls.length;
    await assert.rejects(f.source.download());
    assert.equal(await f.source.fixPermissions(), false);
    await f.source.load();
    assert.equal(f.calls.length, count);
  } finally {f.source.dispose();}
});
