import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {defaultI18n, enUS} from '@i18n';
import {ApiError} from '@core/index.ts';
import {CodedError} from '@core/server-messages.ts';
import {accountCodes, accountFailure, serverCodes, signInFailure, superseded, type SignInStep} from '../src/app/sign-in-errors.ts';
import {componentModule, hooks} from './helpers/component-harness.mjs';

const src = (p: string) => readFileSync(new URL('../src/' + p, import.meta.url), 'utf8');
const say = (e: unknown, step: SignInStep = 'portico', server = 'Den', online?: boolean) => signInFailure(e, {step, server, online}, defaultI18n);
const steps: SignInStep[] = ['password', 'portico', 'invitation', 'code', 'new-password', 'profile', 'custody', 'account', 'account-code', 'account-create', 'account-recovery'];

// ── Gate 2: a Portico Account never gets a page of its own ─────────────────────────────────

async function gate(session: object, pathname = '/') {
  const h = hooks();
  // NEW-12: Gate reads the pathname and lazily renders the account workspace.
  const react = {...h.react, Suspense: 'Suspense', lazy: () => 'AccountWorkspace'};
  const m = await componentModule(new URL('../src/screens/auth/Gate.tsx', import.meta.url), {
    react, '@tanstack/react-router': {useLocation: () => ({pathname})}, '../../app/session': {useSession: () => session}, '../../app/device': {DeviceGate: 'DeviceGate'},
    './Restore': {RestoreScreen: 'RestoreScreen'}, './SignIn': {SignInScreen: 'SignInScreen'}, '../../shell/ServerlessShell': {ServerlessShell: 'ServerlessShell'},
  }) as any;
  return h.render(() => m.Gate({children: 'app'}));
}

test('gate 2: routing: a Portico Account lands in the shell; only an ended direct sign-in opens the direct form', async () => {
  const hosted = {account: {id: 'a', username: 'alice'}};
  assert.equal((await gate({phase: 'restoring'})).type, 'RestoreScreen');
  assert.equal((await gate({phase: 'signedOut', hosted})).type, 'ServerlessShell', 'signed in to a Portico Account, no server open');
  assert.equal((await gate({phase: 'signedOut', hosted, signInHint: {authority: 'hosted'}})).type, 'ServerlessShell', 'a member’s server sign-in ended: back in the shell');
  assert.equal((await gate({phase: 'signedOut', hosted, signInHint: {authority: 'local'}})).type, 'SignInScreen', 'a direct sign-in ended: its own form');
  assert.equal((await gate({phase: 'signedOut'})).type, 'SignInScreen');
  const ready = await gate({phase: 'ready'});
  assert.equal(ready.type, 'DeviceGate');
  const blocked = ready.props.blocked({state: 'pending', checking: false, recheck() {}});
  assert.equal(blocked.type, 'ServerlessShell', 'waiting for approval: inside the shell, not a page of its own');
  assert.equal(blocked.props.device.state, 'pending');
});

// ── NEW-12: `/account` inside the product shell when a server session is open ──

test('NEW-12: /account renders its light shell with no server session, inside the product shell with one', async () => {
  const hosted = {account: {id: 'a', username: 'alice'}};
  // No server session: the light account shell, never the ServerlessShell loop or the sign-in screen
  // (the workspace itself signs in right there when fully signed out).
  for (const session of [{phase: 'signedOut', hosted}, {phase: 'signedOut', hosted, signInHint: {authority: 'hosted'}}, {phase: 'signedOut'}]) {
    const view = await gate(session, '/account');
    assert.equal(view.type, 'Suspense', `no server session: ${JSON.stringify(session)}`);
    assert.equal(view.props.children[0].type, 'AccountWorkspace');
  }
  // A server session open: Gate renders its children, so the account page lands inside the product shell.
  const ready = await gate({phase: 'ready', session: {viewer: {authority: 'hosted'}}}, '/account');
  assert.equal(ready.type, 'DeviceGate');
  assert.deepEqual(ready.props.children, ['app']);
  // Other routes are untouched by the exemption.
  assert.equal((await gate({phase: 'signedOut', hosted}, '/')).type, 'ServerlessShell');
  assert.equal((await gate({phase: 'restoring'}, '/account')).type, 'RestoreScreen');
});

test('NEW-12: /account is parented under the app route (path unchanged)', () => {
  const router = src('app/router.tsx');
  assert.match(router, /const account = createRoute\(\{getParentRoute: \(\) => app, path: '\/account'/);
  assert.match(router, /app\.addChildren\(\[[^\]]*account/);
  assert.doesNotMatch(router, /getParentRoute: \(\) => root, path: '\/account'/);
});

async function signInScreen(session: object) {
  (globalThis as any).location ??= {hostname: 'web.getportico.tv', origin: 'https://web.getportico.tv'};
  const h = hooks();
  const m = await componentModule(new URL('../src/screens/auth/SignIn.tsx', import.meta.url), {
    react: h.react, '@core/index.ts': {IdentityClient: class {}, HttpLocalApi: class {}}, '../../bridge/profile-trust': {}, '../../app/session': {useSession: () => session, serverAddress: (v: string) => v}, '@core/profile-management.ts': {devE2EUsername: async () => undefined},
    '../../app/i18n': {useI18n: () => ({t: (id: string) => id})}, '@core/presentation/index.ts': {friendlyHost: () => 'den.local'}, '../../ui': {}, '@core/password-strength.ts': {},
    './AccountForm': {}, './AuthFrame': {AuthFrame: 'AuthFrame'}, './DirectProfiles': {}, './ServerSetup': {}, './Auth.module.css': {default: {}},
  }) as any;
  return h.render(() => m.SignInScreen());
}

test('gate 2: an ended direct sign-in with a Portico Account still signed in shows the direct form, never "Welcome"', async () => {
  let dismissed = 0;
  const session = {hosted: {account: {id: 'a', username: 'alice', displayName: 'Alice'}}, signInHint: {authority: 'local', serverUrl: 'http://den.local:32500', username: 'justin'}, direct: {origin: 'http://den.local:32500'}, dismissSignInHint: () => dismissed++, servers: []};
  const view = await signInScreen(session);
  assert.equal(view.type.name, 'DirectSignIn');
  view.props.onAccount();
  assert.equal(dismissed, 1, '“Use a Portico Account” returns to the shell rather than a second account page');
  const code = src('screens/auth/SignIn.tsx');
  assert.doesNotMatch(code, /Welcome, \$\{|HostedChooser/, 'the full-page chooser branch is gone');
  assert.doesNotMatch(code, /if \(session\.hosted\) setMode\('account'\)/, 'a Portico Account no longer pre-empts the direct form');
});

// ── Gate 3: every sign-in failure says what happened ──────────────────────────────────────

test('gate 3: clock skew, unreachable, device pending and denied each say what happened', () => {
  assert.equal(say(new ApiError(401, 'clock_skew', 'x')).text, enUS['web.portico.clockSkew'].replace('{server}', 'Den'));
  const down = say(new TypeError('Failed to fetch'));
  assert.equal(down.messageId, 'web.signIn.error.unreachable');
  assert.equal(down.wait, 'server', 'the server is expected back: Portico retries it');
  assert.equal(say(new TypeError('Failed to fetch'), 'password', 'Den', false).messageId, 'web.signIn.error.offline');
  assert.equal(say(Object.assign(new Error('route'), {code: 'route_unavailable'})).wait, 'server');
  assert.equal(say(new ApiError(403, 'device_approval_pending', 'x'), 'password').text, enUS['device.approval.body']);
  const denied = say(new ApiError(403, 'device_denied', 'x'), 'password');
  assert.match(denied.text, /The owner of Den has turned this browser away/);
  assert.equal(denied.wait, undefined, 'a refusal is not retried');
  assert.equal(say(new ApiError(403, 'access_refused', 'x')).refused, true);
});

test('gate 3: a Portico Account service outage is said as that, never as “Waiting for {server}”', () => {
  for (const e of [new ApiError(503, 'unavailable', 'x'), new ApiError(502, 'request_failed', 'x'), new ApiError(503, 'account_service_recovering', 'x'), new TypeError('Failed to fetch')]) {
    const f = say(accountFailure(e));
    assert.equal(f.wait, 'account', String((e as any).code ?? e.name));
    assert.doesNotMatch(f.text, /Waiting for|Den/);
  }
  assert.equal(say(accountFailure(new ApiError(503, 'unavailable', 'x'))).messageId, 'web.signIn.error.accountUnavailable');
  assert.equal(say(new ApiError(503, 'authentication_busy', 'x')).wait, 'server', 'the server busy is the server’s wait');
  const chooser = src('screens/auth/HostedChooser.tsx');
  assert.match(chooser, /outcome === 'account-unavailable' && target\) return <StateView icon="warning" title=\{t\('web\.chooser\.accountWaitingTitle'\)\}/);
  const session = src('app/session.tsx');
  assert.match(session, /failure\.wait === 'server' \? 'unreachable' : failure\.wait === 'account' \? 'account-unavailable' : 'failed'/);
  assert.doesNotMatch(session.slice(session.indexOf('const porticoConnect'), session.indexOf('const acceptPortico')), /retryableServerRestore/);
});

test('gate 3: the codes the INT review named get their own sentence', () => {
  assert.equal(say(accountFailure(new ApiError(401, 'invalid_credentials', 'x')), 'account').messageId, 'web.signIn.error.accountCredentials');
  assert.equal(say(new ApiError(401, 'unauthorized', 'x'), 'password').messageId, 'web.signIn.error.credentials');
  assert.equal(say(new ApiError(400, 'invalid_request', 'x'), 'password').messageId, 'web.signIn.error.details');
  assert.equal(say(new ApiError(422, 'invalid_request', 'x'), 'new-password').messageId, 'web.signIn.error.newPassword');
  assert.equal(say(new CodedError('invalid_response', 'unreadable')).messageId, 'web.signIn.error.unreadable');
  assert.equal(say(new CodedError('server_mismatch', 'The server answered as a different server.')).messageId, 'web.signIn.error.identity');
  assert.equal(say(new CodedError('challenge_not_proven', 'x')).messageId, 'web.portico.notProven');
  assert.equal(say(new DOMException('The operation timed out.', 'TimeoutError')).messageId, 'web.signIn.error.timeout');
  assert.equal(say(new ApiError(429, 'rate_limited', 'x', true, 30), 'password').text, 'Too many sign-in attempts. Try again in 30 seconds.');
  assert.equal(say(superseded()).silent, true, 'a sign-in the person moved on from says nothing');
  assert.equal(say(new DOMException('aborted', 'AbortError')).silent, true);
  assert.equal(say(new Error('something internal'), 'password').messageId, 'web.signIn.error.bug', 'a Portico fault is said as one, naming the server');
});

test('gate 3: exhaustive, and no generic copy is reachable from sign-in', () => {
  const ids = new Set<string>();
  for (const table of [serverCodes, accountCodes]) for (const answer of Object.values(table)) for (const step of steps) ids.add(typeof answer === 'function' ? answer(step) : answer);
  for (const id of ids) assert.ok(id in enUS, `${id} is in the catalogue`);
  const generic = Object.entries(enUS).filter(([id]) => id.startsWith('error.')).map(([, text]) => text);
  const forbidden = [/Something went wrong/, /Some details weren/, /didn’t confirm the change/, /Waiting for/, /That didn’t work\. Check what you entered/];
  const samples: unknown[] = [new Error('plain'), new TypeError('Failed to fetch'), new DOMException('t', 'TimeoutError'), 'weird_code', null, undefined, {}, new CodedError('invalid_saved_connection', 'x')];
  for (const code of [...Object.keys(serverCodes), ...Object.keys(accountCodes), 'never_heard_of_it', 'invalid_whatever'])
    for (const status of [undefined, 400, 401, 403, 404, 405, 409, 410, 412, 422, 426, 429, 500, 502, 503, 504, 507]) samples.push(Object.assign(new Error('x'), {code, status}));
  for (const status of [400, 401, 403, 404, 409, 422, 429, 500, 503]) samples.push({status});
  let checked = 0;
  for (const sample of samples) for (const step of steps) for (const account of [false, true]) {
    const e = account && sample && typeof sample === 'object' ? accountFailure(Object.create(sample as object, {code: {value: (sample as any).code}, status: {value: (sample as any).status}, name: {value: (sample as any).name}})) : sample;
    const f = signInFailure(e, {step, server: 'Den'}, defaultI18n);
    if (f.silent) continue;
    checked++;
    assert.ok(f.text && !generic.includes(f.text), `${step} ${JSON.stringify(sample)} → ${f.messageId}`);
    for (const pattern of forbidden) assert.doesNotMatch(f.text, pattern, `${step} ${JSON.stringify(sample)}`);
    assert.doesNotMatch(f.text, /\{server\}|\{seconds/, 'every sentence is filled in');
  }
  assert.ok(checked > 5000);
});

test('gate 3: sign-in paths use the sign-in presenter, not the generic one', () => {
  const session = src('app/session.tsx');
  const uses = [...session.matchAll(/\bmessage\(e,/g)].length;
  assert.equal(uses, 1, 'only sign-out still uses the generic presenter');
  assert.match(session, /setError\(message\(e, 'Sign-out could not be saved\.'\)\)/);
  assert.doesNotMatch(session, /throw new Error\('The selected server changed\.'\)/, 'a superseded sign-in is silent');
  assert.doesNotMatch(src('screens/auth/AccountForm.tsx'), /errorText\(/);
  assert.doesNotMatch(src('screens/auth/DirectJoin.tsx'), /errorText\(/);
});

async function deviceModule(announce: () => Promise<string>, session: object) {
  const h = hooks();
  const effects: (() => unknown)[] = [];
  h.react.useEffect = (fn: () => unknown) => { effects.push(fn); };
  (h.react as any).default.useEffect = h.react.useEffect;
  const m = await componentModule(new URL('../src/app/device.tsx', import.meta.url), {
    react: h.react, './i18n': {useI18n: () => ({t: (id: string, v?: Record<string, string>) => (v ? `${id}:${JSON.stringify(v)}` : id)})},
    '@core/index.ts': {IdentityClient: class { rememberAccount() { return Promise.resolve(); } }, announceDevice: announce},
    '../bridge/client-profile': {keepBrowserProfilePublished: () => undefined}, '../bridge/profile-trust': {browserInstallation: () => 'i'.repeat(36)},
    '../ui': {StateView: 'StateView'}, '@core/presentation/index.ts': {friendlyHost: () => 'den.local'}, './session': {useSession: () => session},
  }) as any;
  return {m, h, effects};
}

test('gate 3: a device the owner denied or has yet to approve is held inside the shell with Check again and Sign out', async () => {
  let signedOut = 0;
  const session = {api: {}, session: {sessionFamilyId: 'fam', viewer: {authority: 'local'}}, signOut: () => signedOut++, system: {name: 'Den'}};
  for (const outcome of ['pending', 'denied'] as const) {
    let calls = 0;
    const {m, h, effects} = await deviceModule(async () => { calls++; return outcome; }, session);
    const render = () => h.render(() => m.DeviceGate({children: 'app', blocked: (access: any) => ({type: 'Shell', access})}));
    assert.deepEqual(render().props.children, ['app'], 'the app shows while the device is announced');
    await effects[0]();
    const held = render().props.children[0];
    assert.equal(held.type, 'Shell');
    const access = held.access;
    assert.equal(access.state, outcome);
    const block = h.render(() => m.DeviceBlock({access}));
    assert.equal(block.type, 'StateView');
    assert.equal(block.props.title, outcome === 'denied' ? 'web.device.deniedTitle' : 'device.approval.title');
    if (outcome === 'denied') assert.match(block.props.body, /"server":"Den"/);
    assert.equal(block.props.action.label, 'device.approval.checkAgain');
    block.props.action.onClick();
    assert.equal(calls, 2, 'Check again asks the server again');
    block.props.secondaryAction.onClick();
  }
  assert.equal(signedOut, 2);
  const {m, h, effects} = await deviceModule(async () => 'skipped', session);
  h.render(() => m.DeviceGate({children: 'app', blocked: () => 'held'}));
  await effects[0]();
  assert.deepEqual(h.render(() => m.DeviceGate({children: 'app', blocked: () => 'held'})).props.children, ['app'], 'an incomplete device list never costs the sign-in');
});

test('gate 3: device pending and a direct step show inside the shell, not as a page', () => {
  const shell = src('shell/ServerlessShell.tsx');
  assert.match(shell, /device \? <DeviceBlock access=\{device\} \/> : session\.direct\.step \? <DirectStep inShell \/>/);
  assert.match(shell, /<SignInEnded \/>/, 'an ended member sign-in is explained in the shell');
  assert.doesNotMatch(src('app/device.tsx'), /minHeight: '100dvh'/, 'no full-page approval wait');
  assert.match(src('screens/auth/SignIn.tsx'), /function StepFrame\(\{inShell/);
});
