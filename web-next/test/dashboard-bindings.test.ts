import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createDashboardBindings, type DashboardBindingOptions} from '../src/screens/dashboard/bindings.ts';
const page = () => ({scope: {serverId: 'server', viewerFence: 'a'.repeat(64)}, observedAt: '2026-10-23T00:00:00Z', items: [], page: {limit: 10, start: 0, total: 0, revision: '1'}, totals: {streams: 0, atHome: 0, away: 0, paused: 0, converting: 0, people: 0, bandwidthKbps: 0}});
function options(overrides: Partial<DashboardBindingOptions> = {}): DashboardBindingOptions {
  return {api: {request: async () => ({entries: [], total: 0, nextCursor: ''}) as never, requestRaw: async () => ({status: 200, headers: {}, body: page()})}, scope: {serverId: 'server', viewerId: 'viewer'}, role: 'owner', isCurrent: () => true, isMainProfile: () => true, artwork: () => undefined, effects: {openHistory() {}, openSettings() {}, reportError() {}}, ...overrides};
}
test('administrator reads sessions only; secondary profile and member dispatch nothing', async () => {
  for (const change of [{role: 'member' as const}, {isMainProfile: () => false}]) {let calls = 0; const b = createDashboardBindings(options({...change, api: {request: async () => {calls++; return {} as never;}, requestRaw: async () => {calls++; return {} as never;}}})); await assert.rejects(async () => b.read.sessions()); await assert.rejects(async () => b.read.status()); assert.equal(calls, 0); b.dispose();}
  const b = createDashboardBindings(options({role: 'admin'})); assert.equal((await b.read.sessions()).totals.streams, 0);
  for (const read of [() => b.read.status(), () => b.read.people({period: 'all'}), () => b.read.history({period: 'all'}, 'music'), () => b.read.telemetry('live'), () => b.read.plays({period: 'all'}), () => b.effects.stop('id')]) await assert.rejects(async () => read()); b.dispose();
});
test('late private response after authority replacement is rejected and disposed source cannot read', async () => {
  let current = true, release!: (value: never) => void; const b = createDashboardBindings(options({isCurrent: () => current, api: {...options().api, request: async () => new Promise(resolve => {release = resolve;})}}));
  const pending = b.read.plays({period: 'all'}); current = false; release({entries: [], nextCursor: ''} as never); await assert.rejects(pending); b.dispose(); await assert.rejects(async () => b.read.sessions());
});
test('session scope mismatch rejects; history passes exact server query and opaque cursor', async () => {
  const b = createDashboardBindings(options({api: {...options().api, requestRaw: async () => ({status: 200, headers: {}, body: {...page(), scope: {...page().scope, serverId: 'wrong'}}})}})); await assert.rejects(b.read.sessions()); b.dispose();
  let path = ''; const c = createDashboardBindings(options({api: {...options().api, request: async request => {path = request; return {entries: [], nextCursor: ''} as never;}}})); await c.read.plays({period: '30d', media: 'audiobooks', accountId: 'account', sort: 'watched', direction: 'asc'}, 'opaque');
  const q = new URL(path, 'https://server.invalid').searchParams; assert.equal(q.get('limit'), '100'); assert.equal(q.get('sort'), 'watched'); assert.equal(q.get('cursor'), 'opaque'); assert.equal(q.get('accountId'), 'account'); assert.equal(q.get('media'), 'audiobooks'); c.dispose();
});
test('terminal private-read refusal synchronously retires every mounted consumer', async () => {
  let changed = 0;
  const b = createDashboardBindings(options({api: {...options().api, request: async () => {throw {status: 403};}}}));
  const off = b.subscribeAuthority!(() => changed++); await assert.rejects(b.read.plays({period: 'all'})); assert.equal(b.isCurrent(), false); assert.equal(changed, 1); off(); b.dispose();
});
test('shell authority notification clears current bindings before another request', () => {
  let notify!: () => void, current = true;
  const b = createDashboardBindings(options({isCurrent: () => current, subscribeAuthority: listener => {notify = listener; return () => {};}}));
  let changed = 0; b.subscribeAuthority!(() => changed++); current = false; notify(); assert.equal(changed, 1); assert.equal(b.isCurrent(), false); b.dispose();
});
