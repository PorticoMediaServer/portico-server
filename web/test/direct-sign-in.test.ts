import test from 'node:test';
import assert from 'node:assert/strict';
import {signInHintFor} from '../src/app/sign-in-hint.ts';
import {foregroundRetry, jittered} from '../src/app/foreground-retry.ts';
import {ViewerSelectionCommit, selectedViewerRecord} from '../../packages/client-core/src/viewer-selection.ts';
import * as signInErrors from '../src/app/sign-in-errors.ts';
import * as accountSwitch from '../src/app/account-switch.ts';
import {CodedError} from '../../packages/client-core/src/server-messages.ts';
import {componentModule, hooks} from './helpers/component-harness.mjs';

/** C45 on the web: the sign-in session is never ended as an "account session" when it is the viewer,
 * an account-scoped one opens the chooser, and a challenge or a required password change is a step. */
const tick = () => new Promise(r => setTimeout(r, 0));
// The documented direct sign-in envelope.
const session = (token: string, role: string) => ({accessToken: token, refreshToken: 'r'.repeat(43), deviceId: 'dev', installationId: 'inst', expiresAt: new Date(Date.now() + 3600000).toISOString(), viewer: {accountId: 'account', profileId: 'p1', serverId: 'server', authority: 'local' as const, role}, sessionFamilyId: 'family-' + token, tokenGeneration: '1', authorizationHorizon: new Date(Date.now() + 7200000).toISOString()});
const snapshot = (n: number) => ({authority: 'local', serverId: 'server', account: {id: 'account', username: 'owner', primaryProfileId: 'p1', role: 'owner', revision: 1, disabled: false, allowedLibraries: []}, profiles: Array.from({length: n}, (_, i) => ({id: 'p' + (i + 1), name: 'P' + (i + 1), primary: i === 0, position: i, art: 'blue', revision: 1, pinRevision: 1, pinRequired: false, allowedLibraries: []})), canManage: true});
const ended = (f: {requests: [string, string, string][]}, token: string) => f.requests.some(([path, method, t]) => (path === '/v1/sessions/current' || path === '/v1/direct/account-session') && method === 'DELETE' && t === token);

async function fixture(results: unknown[], options: {portico?: boolean; refuse?: boolean; hostedRefuse?: boolean; notProven?: boolean} = {}) {
  const h = hooks();
  const requests: [string, string, string][] = [];
  const refreshers: any[] = [];
  let saved: unknown;
  class Viewer { state: any = {phase: 'signedOut'}; subscribe = () => () => {}; getSnapshot = () => this.state; clear = () => { this.state = {phase: 'signedOut'}; }; select = (serverUrl: string, s: any) => { this.state = {phase: 'ready', serverUrl, session: s}; }; }
  class Api {
    baseUrl: string; token: string;
    constructor(url: string, token = '') { this.baseUrl = url; this.token = token; }
    setAccessToken(t: string) { this.token = t; }
    async request(path: string, method = 'GET') { requests.push([path, method, this.token]); if (path === '/v1/me') return {viewer: session(this.token, 'owner').viewer}; return undefined; }
    async system() { return {setupRequired: false}; }
    async logout() { requests.push(['logout', 'POST', this.token]); }
    subscribeRoute = () => () => {}; getRouteSnapshot = () => null;
  }
  const env = {storage: {read: async () => undefined, change: async () => {}}};
  const storage = {read: async () => saved, write: async (v: unknown) => { saved = v; }};
  const live = {account: {id: 'acc_portico', username: 'justin'}, familyId: 'fam', accessToken: 'hosted-token'};
  const account = {api: {origin: 'https://hosted.example', send: async () => ({})}, service: {subscribe: () => () => {}, getSnapshot: () => (options.portico ? {phase: 'ready', session: live} : {phase: 'signedOut'}), accessSession: async () => live, updateContext: async () => {}}};
  const portico = {
    ServerListWatcher: class { adopt() {} getRecord() { return null; } foreground() { return Promise.resolve('not-due'); } refresh() { requests.push(['server-list', 'CHECK', '']); return Promise.resolve('unchanged'); } },
    porticoSignIn: async (_h: unknown, origin: string, api: any, serverId: string, o: any = {}) => { requests.push(['portico-sign-in', o.kind ?? 'interactive', serverId + '@' + origin + '>' + api.baseUrl]); if (options.refuse) throw Object.assign(new Error('refused'), {status: 403, code: 'access_refused'}); if (options.notProven) throw Object.assign(new Error('NOT PROVEN'), {code: 'challenge_not_proven'}); return queue.shift(); },
    acceptPorticoInvitation: async (_h: unknown, _o: string, api: any, serverId: string, code: string) => { requests.push(['portico-accept', code, serverId + '>' + api.baseUrl]); return queue.shift(); },
    challengeNotProven: 'NOT PROVEN', acceptPorticoCustody: async (_h: unknown, _o: string, api: any, serverId: string) => { requests.push(['custody', serverId, api.token]); },
    isAccessRefused: (e: any) => e?.status === 403 && e?.code === 'access_refused', isSessionMigrated: (e: any) => e?.status === 401 && e?.code === 'session_migrated', isClockSkew: () => false,
  };
  const queue = [...results];
  const modules: any = {
    react: h.react, '@core/index.ts': {HttpLocalApi: Api, ViewerService: Viewer, parseServerPresence: () => undefined}, './known-servers': {knownServers: {remember: async () => {}, setProfile: async () => {}}}, './sign-in-hint': {signInHintFor}, './foreground-retry': {foregroundRetry, jittered}, '@core/viewer-selection.ts': {ViewerSelectionCommit, selectedViewerRecord},
    '@core/server-connections.ts': {inspectDirectServer: async () => ({fingerprint: 'pin', serverId: 'server'}), connectPairedServer: async (origin: string) => ({api: new Api(origin), connection: {dispose: () => requests.push(['dispose', '', ''])}}), connectHostedServer: async (server: any) => { if (options.hostedRefuse) throw Object.assign(new Error('This operation is not permitted.'), {status: 403, code: 'permission_denied'}); return ({api: new Api(server.baseUrl), connection: {dispose: () => requests.push(['dispose', '', ''])}, hosted: {origin: 'https://hosted.example', accountId: 'acc_portico', key: {}}}); }, rememberNativeSession: async (_api: unknown, s: any, _env: unknown, hosted: any) => { requests.push(['remember', s.accessToken, hosted?.accountId ?? '']); }},
    '@core/profile-management.ts': {parseDirectSnapshot: (v: any) => v, directSignIn: async () => queue.shift(), selectDirectProfile: async (_api: unknown, _scope: unknown, profileId: string, input: any = {}) => { requests.push(['select', profileId, input.pin ?? '']); return {session: {...session('chosen', 'owner'), viewer: {...session('chosen', 'owner').viewer, profileId}}}; }, completeDirectSignInChallenge: async () => queue.shift(), changeRequiredPassword: async (api: any, current: string, next: string) => { requests.push(['/v1/direct/password', 'POST', api.token + ':' + current + '>' + next]); }},
    '@core/session-selection.ts': {}, '@core/portico-servers.ts': portico, '@core/hosted-gate.ts': {hostedGate: () => ({run: (_k: string, f: () => unknown) => f(), blockedFor: () => 0})}, './server-list-cache': {lastServer: () => undefined, rememberLastServer: () => {}, cachedServerList: () => undefined, serverListStorage: () => ({load: () => null, save: () => {}}), storeServerList: () => {}}, './i18n': {currentI18n: () => ({t: (k: string) => k})}, '../bridge/account': {browserAccount: () => account}, '../bridge/api': {hosted: async (_o: string, path: string) => { requests.push([path, 'HOSTED', '']); return {items: [{id: 'server', name: 'Home', baseUrl: 'https://home.example', publicKey: 'k', policyRevision: 1}], version: '7', watch: 'w'}; }},
    '../bridge/server-connections': {browserConnectionEnvironment: () => env, trackBrowserConnection: () => {}, forgetBrowserConnection: async () => {}},
    '../bridge/viewer-selection-storage': {browserViewerStorage: storage}, '../bridge/restore-policy': {retryableServerRestore: () => false},
    '../bridge/profile-trust': {browserInstallation: () => 'installation', browserTrustLoaded: async () => {}, readBrowserTrust: () => undefined, saveBrowserTrust: () => {}},
    '@core/installation.ts': {setCredentialLock: () => {}, setInstallationIdentity: () => {}, validInstallationId: () => false}, '@core/session-refresh.ts': {LocalSessionRefresher: class { constructor(o: any) { refreshers.push(o); } stop() {} wake() {} }}, './sign-in-errors':signInErrors,'../bridge/audio/bytes':{audioByteCache:{clear(){}}},'./account-switch':accountSwitch,'@core/server-messages.ts':{CodedError},'@core/presentation/index.ts':{friendlyHost:()=>undefined},'./device': {describeBrowser: () => 'Browser'}, './errors': {errorText: (e: any) => String(e?.message ?? '')},
  };
  const app = await componentModule(new URL('../src/app/session.tsx', import.meta.url), modules);
  const render = () => h.render(() => app.SessionProvider({children: null})).props.value;
  render(); render().direct.setOrigin('https://server.example'); render();
  return {render, requests, refreshers, viewer: app.viewer};
}

test('one unprotected profile: the sign-in session is the viewer and is never ended', async () => {
  const f = await fixture([{kind: 'signed-in', snapshot: snapshot(1), session: session('viewer', 'owner'), accountScoped: false, passwordChangeRequired: false}]);
  await f.render().direct.signIn('sam', 'pw'); await tick();
  assert.equal(ended(f, 'viewer'), false);
  assert.equal(f.viewer.getSnapshot().session?.accessToken, 'viewer');
});

test('several profiles: the account-scoped session opens the chooser', async () => {
  const f = await fixture([{kind: 'signed-in', snapshot: snapshot(2), session: session('acct', 'account'), accountScoped: true, passwordChangeRequired: false}]);
  await f.render().direct.signIn('sam', 'pw'); await tick();
  const view = f.render();
  assert.equal(view.direct.pending?.accountGrant, true);
  assert.equal(view.direct.pending?.accountSession?.accessToken, 'acct');
  assert.equal(ended(f, 'acct'), false);
  // Cancelling the chooser retires the account-scoped family with DELETE /v1/sessions/current (C45).
  view.direct.cancelPending(); await tick();
  assert.equal(f.requests.some(([path, method, t]) => path === '/v1/sessions/current' && method === 'DELETE' && t === 'acct'), true);
  assert.equal(f.requests.some(([path]) => path === '/v1/direct/account-session'), false);
});

test('two-step sign-in asks for a code, then continues', async () => {
  const f = await fixture([{kind: 'challenge', token: 'c'.repeat(43), expiresAt: '2099-01-01T00:00:00Z'}, {kind: 'signed-in', snapshot: snapshot(2), session: session('acct', 'account'), accountScoped: true, passwordChangeRequired: false}]);
  await f.render().direct.signIn('sam', 'pw'); await tick();
  assert.equal(f.render().direct.step?.kind, 'code');
  await f.render().direct.continueSignIn('123456'); await tick();
  const view = f.render();
  assert.equal(view.direct.step, undefined);
  assert.equal(view.direct.pending?.accountSession?.accessToken, 'acct');
});

test('a required password change uses the sign-in session and the typed password, then signs in again', async () => {
  const f = await fixture([{kind: 'signed-in', snapshot: snapshot(1), session: session('forced', 'account'), accountScoped: true, passwordChangeRequired: true}, {kind: 'signed-in', snapshot: snapshot(1), session: session('viewer', 'owner'), accountScoped: false, passwordChangeRequired: false}]);
  await f.render().direct.signIn('sam', 'old-Pass1'); await tick();
  assert.equal(f.render().direct.step?.kind, 'new-password');
  await f.render().direct.continueSignIn('new-Pass2'); await tick();
  assert.ok(f.requests.some(([path, , detail]) => path === '/v1/direct/password' && detail === 'forced:old-Pass1>new-Pass2'));
  assert.equal(f.viewer.getSnapshot().session?.accessToken, 'viewer');
});

/** A Portico Account member of the server: its account carries `hostedAccountId`. */
const member = (v: ReturnType<typeof snapshot>) => ({...v, account: {...v.account, hostedAccountId: 'acc_portico'}});
const server = {id: 'server', name: 'Home', baseUrl: 'https://home.example', publicKey: 'k', policyRevision: 1};

test('Portico Account: choosing a server signs in with an identity assertion, then the server’s own profile chooser', async () => {
  const f = await fixture([{kind: 'signed-in', snapshot: snapshot(2), session: session('acct', 'account'), accountScoped: true, passwordChangeRequired: false}], {portico: true});
  assert.equal(await f.render().portico.connect(server), 'done'); await tick();
  assert.ok(f.requests.some(([path, kind, detail]) => path === 'portico-sign-in' && kind === 'interactive' && detail === 'server@https://hosted.example>https://home.example'));
  const view = f.render();
  assert.equal(view.direct.pending?.accountSession?.accessToken, 'acct', 'the direct chooser, as for a password sign-in');
  assert.equal(view.direct.pending?.hosted?.accountId, 'acc_portico', 'routes keep coming from the Portico Account');
  assert.equal(f.requests.some(([path]) => path.includes('/attachments') || path.includes('/v1/hosted/attach')), false, 'no ticket attach');
});

test('Portico Account: one profile is entered at once and remembered with its Portico route source', async () => {
  const f = await fixture([{kind: 'signed-in', snapshot: snapshot(1), session: session('viewer', 'owner'), accountScoped: false, passwordChangeRequired: false}], {portico: true});
  assert.equal(await f.render().portico.connect(server), 'done'); await tick();
  assert.equal(f.viewer.getSnapshot().session?.accessToken, 'viewer');
  assert.ok(f.requests.some(([path, token, hosted]) => path === 'remember' && token === 'viewer' && hosted === 'acc_portico'));
});

test('Portico Account: a server that refuses the account (access_refused) leaves the list and nothing is kept', async () => {
  const f = await fixture([], {portico: true, refuse: true});
  assert.equal(await f.render().portico.connect(server), 'refused'); await tick();
  const view = f.render();
  assert.equal(view.error, 'web.portico.noAccess');
  assert.ok(f.requests.some(([path]) => path === 'dispose'), 'the connection is closed');
  assert.equal(f.viewer.getSnapshot().session, undefined);
});

test('Portico Account: an invitation is accepted with the account and the server is added at once', async () => {
  const f = await fixture([{kind: 'signed-in', snapshot: snapshot(1), session: session('joined', 'owner'), accountScoped: false, passwordChangeRequired: false}], {portico: true});
  assert.equal(await f.render().portico.acceptInvitation('https://home.example', 'c'.repeat(40)), true); await tick();
  assert.ok(f.requests.some(([path, code]) => path === 'portico-accept' && code === 'c'.repeat(40)));
  assert.equal(f.viewer.getSnapshot().session?.accessToken, 'joined');
  assert.ok(f.requests.some(([path]) => path === 'server-list'), 'the server list is checked so other screens see it');
});

test('session_migrated: a Portico member is re-admitted silently with the same profile, no sign-in screen', async () => {
  const f = await fixture([
    {kind: 'signed-in', snapshot: member(snapshot(1)), session: session('old', 'owner'), accountScoped: false, passwordChangeRequired: false},
    {kind: 'signed-in', snapshot: member(snapshot(2)), session: session('acct', 'account'), accountScoped: true, passwordChangeRequired: false},
  ], {portico: true});
  await f.render().portico.connect(server); await tick();
  assert.equal(f.viewer.getSnapshot().session?.accessToken, 'old');
  f.render();
  // The server moved its membership (migration 0120): the renewal answers 401 session_migrated.
  f.refreshers.at(-1).onSignedOut({status: 401, code: 'session_migrated'});
  for (let i = 0; i < 5; i++) { await tick(); f.render(); }
  assert.ok(f.requests.some(([path, kind]) => path === 'portico-sign-in' && kind === 'automatic'), 'an automatic identity sign-in');
  assert.ok(f.requests.some(([path, profile]) => path === 'select' && profile === 'p1'), 'the same profile, without asking');
  assert.equal(f.viewer.getSnapshot().session?.viewer.profileId, 'p1');
  assert.equal(f.render().signInHint, undefined, 'never the sign-in screen');
});

test('Portico Account: Hosted no longer listing the account (routes 403) is a refusal too: the server leaves the list', async () => {
  const f = await fixture([], {portico: true, hostedRefuse: true});
  assert.equal(await f.render().portico.connect(server), 'refused'); await tick();
  assert.equal(f.render().error, 'web.portico.noAccess');
  assert.ok(f.requests.some(([path]) => path === 'server-list'), 'the kept list is checked at once');
});

test('INT M7: a server that can’t prove its identity is named as such, and nothing is kept', async () => {
  const f = await fixture([], {portico: true, notProven: true});
  assert.equal(await f.render().portico.connect(server), 'failed'); await tick();
  assert.equal(f.render().error, 'web.portico.notProven');
  assert.equal(f.viewer.getSnapshot().session, undefined);
});

test('INT M6: an owner whose custody is pending is flagged, and accepting asks the server with the viewer session', async () => {
  const pending = {...member(snapshot(1)), custodyPending: true};
  const f = await fixture([{kind: 'signed-in', snapshot: pending, session: session('viewer', 'owner'), accountScoped: false, passwordChangeRequired: false}], {portico: true});
  await f.render().portico.connect(server); await tick();
  assert.equal(f.render().local?.custodyPending, true);
  void f.render().portico.acceptCustody(); await tick();
  assert.ok(f.requests.some(([path, serverId, token]) => path === 'custody' && serverId === 'server' && token === 'viewer'));
});
