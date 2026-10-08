import test from 'node:test';
import assert from 'node:assert/strict';
import {ApiError, HttpLocalApi, ViewerService, type LocalSession} from '../src/index.ts';
import {parseLocalSession} from '../src/local-session.ts';
import {selectedViewerRecord} from '../src/viewer-selection.ts';
import {installationHeaders, setCredentialLock, setInstallationIdentity} from '../src/installation.ts';
import {parseRememberedServer, refreshStoredSession, type ConnectionEnvironment, type RememberedServer} from '../src/server-connections.ts';
import {LocalSessionRefresher} from '../src/session-refresh.ts';

/**
 * Lane C's device-bound sessions (939a746): 15-minute access tokens, rotating refresh
 * credentials, a five-minute exact-retry receipt, and plain 401 for reuse or revocation.
 */
const ORIGIN = 'https://portico.test';
const INSTALLATION = 'install_0123456789abcdef0123456789';
const MIN = 60_000;
const viewer = {accountId: 'acct', profileId: 'prof', serverId: 'srv', authority: 'local', role: 'owner'};
let clock = Date.UTC(2026, 8, 22, 12);
const iso = (ms: number) => new Date(ms).toISOString().replace(/\.\d+Z$/, 'Z');

class FakeAuthServer {
  generation = 1;
  access = new Map<string, {generation: number; expires: number}>();
  refresh = 'refresh_000000000001';
  receipts = new Map<string, {requestId: string; response: unknown; until: number}>();
  revoked = false;
  migrated = false;
  refreshCalls = 0;
  rotations = 0;
  meCalls = 0;
  /** Commit the rotation, then lose the response (the client sees a network error). */
  dropAfterCommit = 0;
  failNext: number[] = [];
  headers: Record<string, string>[] = [];
  constructor() { this.access.set('access_1', {generation: 1, expires: clock + 15 * MIN}); }
  envelope(access: string, refresh: string, generation: number) {
    return {accessToken: access, refreshToken: refresh, deviceId: 'dev_1', expiresAt: iso(this.access.get(access)!.expires), viewer, sessionFamilyId: 'family_1', tokenGeneration: String(generation), authorizationHorizon: iso(clock + 90 * 24 * 60 * MIN)};
  }
  initial(): LocalSession { return parseLocalSession(this.envelope('access_1', this.refresh, 1)); }
  expireAccess() { for (const v of this.access.values()) v.expires = clock - 1; }
  fetch: typeof fetch = async (input, init = {}) => {
    const url = new URL(String(input));
    const headers: Record<string, string> = {};
    new Headers(init.headers).forEach((v, k) => { headers[k] = v; });
    this.headers.push(headers);
    const json = (status: number, body?: unknown) => new Response(body === undefined ? null : JSON.stringify(body), {status, headers: {'Content-Type': 'application/json'}});
    const fail = this.failNext.shift();
    if (fail) return json(fail, {error: {code: 'unavailable'}});
    if (url.pathname === '/v1/auth/refresh') {
      this.refreshCalls++;
      const q = JSON.parse(String(init.body)) as {refreshToken: string; installationId: string; requestId: string};
      if (this.migrated) return json(401, {error: {code: 'session_migrated'}});
      if (this.revoked || q.installationId !== INSTALLATION || !/^[A-Za-z0-9_-]{1,128}$/.test(q.requestId)) return json(401, {error: {code: 'unauthorized'}});
      if (q.refreshToken === this.refresh) {
        for (const v of this.access.values()) v.expires = Math.min(v.expires, clock - 1); // predecessor access retired
        const g = ++this.generation, access = 'access_' + g, next = 'refresh_' + String(g).padStart(12, '0');
        this.access.set(access, {generation: g, expires: clock + 15 * MIN});
        this.refresh = next;
        this.rotations++;
        const response = this.envelope(access, next, g);
        this.receipts.set(q.refreshToken, {requestId: q.requestId, response, until: clock + 5 * MIN});
        if (this.dropAfterCommit > 0) { this.dropAfterCommit--; throw new TypeError('network connection lost'); }
        return json(200, response);
      }
      const receipt = this.receipts.get(q.refreshToken);
      if (receipt && receipt.requestId === q.requestId) return receipt.until > clock ? json(200, receipt.response) : json(400, {error: {code: 'invalid_request'}});
      return json(401, {error: {code: 'unauthorized'}}); // reuse of a retired credential
    }
    if (url.pathname === '/v1/me') {
      this.meCalls++;
      const token = headers.authorization?.replace(/^Bearer /, '') ?? '';
      const a = this.access.get(token);
      if (this.revoked || !a || a.expires <= clock) return json(401, {error: {code: 'unauthorized'}});
      return json(200, {viewer});
    }
    if (url.pathname === '/v1/direct/sign-in') return json(401, {error: {code: 'unauthorized'}});
    return json(404, {error: {code: 'not_found'}});
  };
}

function memoryStore(record?: RememberedServer) {
  let value = record ? structuredClone(record) : undefined;
  return {
    read: async () => structuredClone(value),
    change: async (update: (v: RememberedServer | undefined) => RememberedServer | undefined) => { value = structuredClone(update(structuredClone(value))); },
    peek: () => value,
  };
}
const remembered = (session: LocalSession): RememberedServer => ({version: '1', selectionId: 'S'.repeat(32), logicalOrigin: ORIGIN, session, pin: {serverId: 'srv', publicKey: 'k'.repeat(43), fingerprint: 'f'.repeat(43)}, routes: {paired: [ORIGIN]}});

function fakeTimers() {
  const timers = new Map<number, {at: number; fn: () => void}>();
  let id = 0;
  return {
    setTimer: (fn: () => void, ms: number) => { timers.set(++id, {at: clock + ms, fn}); return id; },
    clearTimer: (t: unknown) => { timers.delete(t as number); },
    /** Advance the clock, firing due timers in order. */
    advance(ms: number) {
      const end = clock + ms;
      for (;;) {
        const due = [...timers.entries()].filter(([, t]) => t.at <= end).sort((a, b) => a[1].at - b[1].at)[0];
        if (!due) break;
        timers.delete(due[0]); clock = Math.max(clock, due[1].at); due[1].fn();
      }
      clock = end;
    },
    pending: () => timers.size,
  };
}
const settle = async () => { for (let i = 0; i < 20; i++) await new Promise(r => setImmediate(r)); };

function setup(t: import('node:test').TestContext, options: {session?: LocalSession} = {}) {
  t.mock.method(Date, 'now', () => clock);
  setInstallationIdentity({installationId: INSTALLATION, name: 'Test’s Mac', platform: 'macos', app: 'portico-web', appVersion: '1.0.0'});
  setCredentialLock(undefined);
  const server = new FakeAuthServer();
  const session = options.session ?? server.initial();
  const store = memoryStore(remembered(session));
  const env: ConnectionEnvironment = {storage: store};
  const timers = fakeTimers();
  const events = {rotated: [] as LocalSession[], signedOut: [] as string[]};
  const tab = () => {
    const api = new HttpLocalApi(ORIGIN, session.accessToken, server.fetch);
    const {refreshToken: _r, ...visible} = session;
    const refresher = new LocalSessionRefresher({env, api, session: visible, onRotated: s => events.rotated.push(s), onSignedOut: e => events.signedOut.push(e.code), setTimer: timers.setTimer, clearTimer: timers.clearTimer});
    return {api, refresher};
  };
  t.after(() => setInstallationIdentity(undefined));
  return {server, store, env, timers, events, tab};
}

test('refreshes proactively two minutes before expiry and stores the rotation atomically', async t => {
  const {server, store, timers, events, tab} = setup(t);
  const {api, refresher} = tab();
  timers.advance(12 * MIN);
  await settle();
  assert.equal(server.refreshCalls, 0);
  timers.advance(1 * MIN + 1);
  await settle();
  assert.equal(server.rotations, 1);
  const saved = store.peek()!;
  assert.equal(saved.session.refreshToken, server.refresh, 'next refresh credential stored');
  assert.equal(saved.session.accessToken, 'access_2');
  assert.equal(saved.pendingRefresh, undefined, 'request id cleared once stored');
  assert.equal(refresher.current().accessToken, 'access_2');
  assert.equal('refreshToken' in refresher.current(), false, 'the refresh credential never leaves the store');
  assert.equal(events.rotated.length, 1);
  assert.deepEqual(await api.request('/v1/me'), {viewer});
  // …and keeps going: the next one is scheduled for the new expiry.
  timers.advance(13 * MIN + 1);
  await settle();
  assert.equal(server.rotations, 2);
  refresher.stop();
});

test('a request that finds the token about to expire waits for one refresh first', async t => {
  const {server, tab} = setup(t);
  const {api, refresher} = tab();
  clock += 14.5 * MIN; // timers slept (app in background): no proactive refresh happened
  const results = await Promise.all(Array.from({length: 5}, () => api.request('/v1/me')));
  assert.equal(results.length, 5);
  assert.equal(server.rotations, 1, 'single flight');
  assert.equal(server.meCalls, 5, 'no request was sent with the stale token');
  refresher.stop();
});

test('concurrent 401s share one refresh and each request is retried once', async t => {
  const {server, tab} = setup(t);
  const {api, refresher} = tab();
  server.expireAccess(); // e.g. server clock ahead, or the token retired elsewhere
  const results = await Promise.all(Array.from({length: 8}, () => api.request<{viewer: unknown}>('/v1/me')));
  assert.ok(results.every(r => r.viewer));
  assert.equal(server.refreshCalls, 1);
  assert.equal(server.meCalls, 16, 'each request: one 401, one retry');
  refresher.stop();
});

test('revocation ends the sign-in: store cleared, requests fail with 401, no refresh loop', async t => {
  const {server, store, events, tab} = setup(t);
  const {api, refresher} = tab();
  server.revoked = true;
  await assert.rejects(api.request('/v1/me'), (e: unknown) => e instanceof ApiError && e.status === 401);
  assert.deepEqual(events.signedOut, ['refresh_refused']);
  assert.equal(store.peek(), undefined, 'stored sign-in removed');
  assert.equal(refresher.getSnapshot().phase, 'ended');
  const calls = server.refreshCalls;
  await assert.rejects(api.request('/v1/me'));
  assert.equal(server.refreshCalls, calls, 'an ended sign-in never refreshes again');
});

test('a session migration 0120 ended keeps its code (session_migrated), so apps re-admit silently', async t => {
  const {server, store, events, tab} = setup(t);
  const {api, refresher} = tab();
  server.revoked = true; // the old family no longer answers
  server.migrated = true;
  await assert.rejects(api.request('/v1/me'), (e: unknown) => e instanceof ApiError && e.status === 401);
  assert.deepEqual(events.signedOut, ['session_migrated']);
  assert.equal(store.peek(), undefined, 'the migrated credential is gone either way');
  assert.equal(refresher.getSnapshot().phase, 'ended');
});

test('reusing a retired refresh credential is refused and signs out', async t => {
  const {server, store, events, tab} = setup(t);
  const stale = store.peek()!;
  const {refresher} = tab();
  await refresher.recover('access_1'); // rotates to generation 2
  assert.equal(server.rotations, 1);
  // A backup restored the old record (predecessor credential, a new request id).
  await store.change(() => stale);
  await assert.rejects(refreshStoredSession({env: {storage: store}, api: new HttpLocalApi(ORIGIN, '', server.fetch), family: {sessionFamilyId: 'family_1', serverId: 'srv'}, rejected: 'access_1'}), (e: unknown) => (e as {signOut?: boolean}).signOut === true);
  assert.equal(store.peek(), undefined);
  assert.deepEqual(events.signedOut, []);
  refresher.stop();
});

test('a refresh whose answer was lost is replayed with the same request id', async t => {
  const {server, store, events, timers, tab} = setup(t);
  const {api, refresher} = tab();
  server.dropAfterCommit = 1;
  server.expireAccess();
  await assert.rejects(api.request('/v1/me'), (e: unknown) => e instanceof ApiError && e.status === 401);
  const pending = store.peek()!.pendingRefresh;
  assert.ok(pending, 'request id persisted before sending');
  assert.equal(store.peek()!.session.refreshToken, 'refresh_000000000001', 'credential kept on a transient failure');
  assert.equal(refresher.getSnapshot().phase, 'retrying');
  assert.deepEqual(events.signedOut, []);
  timers.advance(5_000);
  await settle();
  assert.equal(server.rotations, 1, 'the retry replayed the receipt; no second rotation');
  assert.equal(store.peek()!.session.accessToken, 'access_2');
  assert.equal(store.peek()!.pendingRefresh, undefined);
  assert.deepEqual(await api.request('/v1/me'), {viewer});
  refresher.stop();
});

test('an app restarted mid-refresh finishes it from the stored request id', async t => {
  const {server, store, env} = setup(t);
  await store.change(r => ({...r!, pendingRefresh: 'req_before_crash'}));
  // The first process sent it and died before storing the answer.
  server.receipts.set(server.refresh, {requestId: 'req_before_crash', response: undefined, until: 0});
  server.dropAfterCommit = 1;
  await assert.rejects(refreshStoredSession({env, api: new HttpLocalApi(ORIGIN, '', server.fetch), family: {sessionFamilyId: 'family_1', serverId: 'srv'}}));
  assert.equal(store.peek()!.pendingRefresh, 'req_before_crash');
  const next = await refreshStoredSession({env, api: new HttpLocalApi(ORIGIN, '', server.fetch), family: {sessionFamilyId: 'family_1', serverId: 'srv'}});
  assert.equal(next.accessToken, 'access_2');
  assert.equal(server.rotations, 1);
  assert.equal(parseRememberedServer(await store.read())!.session.refreshToken, server.refresh);
});

test('a second tab adopts the first tab’s rotation instead of refreshing (or signing out)', async t => {
  const {server, events, tab} = setup(t);
  const a = tab(), b = tab();
  await a.api.request('/v1/me');
  server.expireAccess();
  await a.api.request('/v1/me'); // tab A rotates; access_1 is retired
  assert.equal(server.rotations, 1);
  assert.deepEqual(await b.api.request('/v1/me'), {viewer}, 'tab B recovers from the store');
  assert.equal(server.refreshCalls, 1, 'tab B made no refresh request');
  assert.equal(b.refresher.current().accessToken, 'access_2');
  assert.deepEqual(events.signedOut, []);
  a.refresher.stop(); b.refresher.stop();
});

test('without a refresh credential (plain HTTP) the sign-in ends when the access token does', async t => {
  t.mock.method(Date, 'now', () => clock);
  const server0 = new FakeAuthServer();
  const {refreshToken: _r, ...plain} = server0.initial();
  const {server, events, tab} = setup(t, {session: plain as LocalSession});
  const {api} = tab();
  server.expireAccess();
  clock += 15 * MIN;
  await assert.rejects(api.request('/v1/me'), (e: unknown) => e instanceof ApiError && e.status === 401);
  assert.equal(server.refreshCalls, 0);
  assert.deepEqual(events.signedOut, ['refresh_unavailable']);
});

test('credential requests carry the installation claim; other requests never do', async t => {
  const {server} = setup(t);
  const api = new HttpLocalApi(ORIGIN, '', server.fetch);
  await assert.rejects(api.request('/v1/direct/sign-in', 'POST', {username: 'u', password: 'p'}));
  await api.request('/v1/me').catch(() => {});
  const [signIn, me] = server.headers;
  assert.equal(signIn!['x-portico-installation-id'], INSTALLATION);
  assert.equal(signIn!['x-portico-device-name'], "Test's Mac", 'folded to ASCII for the header');
  assert.equal(me!['x-portico-installation-id'], undefined);
  assert.throws(() => setInstallationIdentity({installationId: 'short', name: '', platform: '', app: '', appVersion: ''}));
  setInstallationIdentity(undefined);
  assert.deepEqual(installationHeaders(), {});
});

test('device-bound envelopes parse; the refresh credential stays out of viewer records and snapshots', async t => {
  const {server} = setup(t);
  const session = server.initial();
  assert.equal(session.deviceId, 'dev_1');
  assert.equal(session.refreshToken, 'refresh_000000000001');
  assert.throws(() => parseLocalSession({...server.envelope('access_1', 'bad token!', 1)}));
  assert.equal('refreshToken' in selectedViewerRecord(ORIGIN, session).session, false);
  const viewers = new ViewerService();
  viewers.select(ORIGIN, session);
  const generation = viewers.getSnapshot().generation;
  assert.equal('refreshToken' in viewers.getSnapshot().session!, false);
  const renewed = parseLocalSession(server.envelope('access_1', 'refresh_000000000009', 2));
  assert.equal(viewers.rotate(renewed), true);
  assert.equal(viewers.getSnapshot().generation, generation, 'rotation is not a new viewer');
  assert.equal(viewers.rotate({...renewed, sessionFamilyId: 'other'}), false);
  assert.equal(viewers.rotate({...renewed, tokenGeneration: '1'}), false, 'never rolls back');
});
