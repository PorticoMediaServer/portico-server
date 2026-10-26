import test from 'node:test';
import assert from 'node:assert/strict';
import {registerHooks} from 'node:module';
import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {transformTsx} from './transform-tsx.mjs';

registerHooks({load(url, context, next) {
  if (url.endsWith('.module.css')) return {format: 'module', source: 'export default new Proxy({}, {get: (_, key) => String(key)});', shortCircuit: true};
  if (url.endsWith('.tsx')) return {format: 'module', source: transformTsx(url), shortCircuit: true};
  return next(url, context);
}});
const {defaultI18n} = await import('../../packages/i18n/src/index.ts');
const {retryDelayMs, retrySeconds, retryProgress, launchFailure, displayableTransportCode} = await import('../src/shell/connection-retry.ts');
const {ShellBootstrap} = await import('../src/shell/bootstrap.ts');
const {LaunchUnreachableScreen} = await import('../src/shell/LaunchUnreachable.tsx');

const routeError = (code: string) => Object.assign(new Error(code), {code});
const facts = {serverName: 'Portico demo', address: '192.168.1.20:32500', owner: true, category: 'unreachable' as const, attempt: 4,
  firstFailureAt: Date.parse('2026-10-24T16:02:00Z'), startedAt: Date.parse('2026-10-24T16:04:00Z'), nextRetryAt: Date.parse('2026-10-24T16:04:09Z'), retrying: false, code: 'route_unavailable'};

test('states.html#3: the launch wait follows the drawn 2, 4, 8 second cadence and stops at a minute', () => {
  const steady = () => 0.5;
  assert.deepEqual([0, 1, 2, 3, 4, 5, 6, 20].map(attempt => retryDelayMs(attempt, steady)), [2000, 4000, 8000, 16000, 32000, 60000, 60000, 60000]);
  // Jitter stays inside ten percent and never falls under a second.
  assert.equal(retryDelayMs(0, () => 0), 1800);
  assert.equal(retryDelayMs(0, () => 1), 2200);
  assert.equal(retryDelayMs(5, () => 0), 54000);
  assert.equal(retrySeconds(Date.parse('2026-10-24T16:04:09Z'), Date.parse('2026-10-24T16:04:00Z')), 9);
  assert.equal(retrySeconds(Date.parse('2026-10-24T16:04:09Z'), Date.parse('2026-10-24T16:04:12Z')), 0);
  assert.equal(retryProgress(Date.parse('2026-10-24T16:04:00Z'), Date.parse('2026-10-24T16:04:09Z'), Date.parse('2026-10-24T16:04:03Z')), 2 / 3);
  assert.equal(retryProgress(0, 0, 0), null);
});

test('only genuine transport failures become the drawn wait; guards keep the error path', () => {
  for (const code of ['route_unavailable', 'request_failed', 'server_unavailable', 'network']) assert.equal(launchFailure(routeError(code), true), 'unreachable', code);
  assert.equal(launchFailure(new TypeError('Failed to fetch'), true), 'unreachable');
  // A device with no link is drawn as offline, not as a server that will not answer.
  assert.equal(launchFailure(routeError('route_unavailable'), false), 'offline');
  // Identity, access, version and contract failures must never become "retrying".
  for (const code of ['identity_mismatch', 'access_refused', 'unauthorized', 'forbidden', 'owner_required', 'unsupported_version', 'invalid_response', 'cancelled']) assert.equal(launchFailure(routeError(code), true), 'other', code);
});

test('a fresh load that cannot reach the server never draws cached library names', () => {
  // A record written by an older build may still carry names; the drawn page has no bar for them.
  const legacy = {...facts, libraries: ['Movies', 'Anime', 'Music']} as typeof facts;
  const markup = renderToStaticMarkup(createElement(LaunchUnreachableScreen, {facts: legacy, now: facts.startedAt, onRetry: () => {}, onSwitchServer: () => {}}));
  assert.doesNotMatch(markup, /Movies|Anime|Music/);
  assert.doesNotMatch(markup, /data-last-known-bar/);
});

test('states.html#3: the drawn page names the server, counts its real tries and offers only real actions', () => {
  const now = facts.startedAt, markup = renderToStaticMarkup(createElement(LaunchUnreachableScreen, {facts, now, onRetry: () => {}, onSwitchServer: () => {}}));
  assert.match(markup, new RegExp(defaultI18n.t('next.states.cantReach', {server: 'Portico demo'}).replace(/'/g, '&#x27;')));
  assert.match(markup, /You&#x27;re still signed in\. Portico keeps trying/);
  assert.match(markup, /Trying again in 9 seconds/);
  assert.match(markup, /4 tries since/);
  assert.match(markup, />Try again now</);
  assert.match(markup, />Switch server</);
  assert.match(markup, />Details</);
  // The opened details carry the real address and the device's own state.
  const withDetails = renderToStaticMarkup(createElement(LaunchUnreachableScreen, {facts: {...facts, owner: false}, now, initialDetailsOpen: true, onRetry: () => {}, onSwitchServer: () => {}}));
  assert.match(withDetails, /192\.168\.1\.20:32500/);
  assert.match(withDetails, /No answer/);
  // Only the owner sees the owner's part, the real code and Copy details.
  assert.doesNotMatch(withDetails, /For you, as the owner/);
  assert.doesNotMatch(withDetails, /route_unavailable/);
  const ownerMarkup = renderToStaticMarkup(createElement(LaunchUnreachableScreen, {facts, now, initialDetailsOpen: true, onRetry: () => {}, onSwitchServer: () => {}}));
  assert.match(ownerMarkup, /For you, as the owner/);
  assert.match(ownerMarkup, /route_unavailable/);
  assert.match(ownerMarkup, /Copy details/);
});

test('a server whose name was never cached keeps the page without inventing one', () => {
  const markup = renderToStaticMarkup(createElement(LaunchUnreachableScreen, {facts: {...facts, serverName: undefined, retrying: true}, now: facts.startedAt, onRetry: () => {}, onSwitchServer: () => {}}));
  assert.match(markup, new RegExp(defaultI18n.t('next.states.cantReachServer').replace(/'/g, '&#x27;')));
  assert.doesNotMatch(markup, /Portico demo/);
  assert.match(markup, /Trying again/);
});

test('desktop-app.html#15: an offline computer says so instead of blaming the server', () => {
  const markup = renderToStaticMarkup(createElement(LaunchUnreachableScreen, {facts: {...facts, category: 'offline' as const}, now: facts.startedAt, onRetry: () => {}, onSwitchServer: () => {}}));
  assert.match(markup, new RegExp(defaultI18n.t('next.states.thisComputerOffline')));
  assert.match(markup, new RegExp(defaultI18n.t('next.states.reconnectsWhenBack').replace(/'/g, '&#x27;')));
  assert.doesNotMatch(markup, /Portico demo/);
});

test('a retired wait stops waking on focus, view or network return', async t => {
  t.mock.timers.enable({apis: ['setTimeout', 'Date']});
  const windowEvents = new EventTarget(), documentEvents = new EventTarget();
  Object.defineProperty(documentEvents, 'visibilityState', {value: 'visible'});
  const supplied = {
    addEventListener: windowEvents.addEventListener.bind(windowEvents),
    removeEventListener: windowEvents.removeEventListener.bind(windowEvents),
    dispatchEvent: windowEvents.dispatchEvent.bind(windowEvents),
    document: documentEvents,
  };
  const originals = Object.keys(supplied).map(key => [key, Object.getOwnPropertyDescriptor(globalThis, key)] as const);
  for (const [key, value] of Object.entries(supplied)) Object.defineProperty(globalThis, key, {configurable: true, value});
  let restores = 0;
  const bootstrap = bootstrapped(async () => { restores++; throw transport(); });
  t.after(() => {
    bootstrap.dispose();
    for (const [key, descriptor] of originals) {
      if (descriptor) Object.defineProperty(globalThis, key, descriptor);
      else Reflect.deleteProperty(globalThis, key);
    }
  });
  await bootstrap.start();
  assert.equal(bootstrap.getSnapshot().phase, 'unreachable');
  let unsubscribe = () => {};
  const retried = new Promise<void>(resolve => {
    unsubscribe = bootstrap.subscribe(() => {
      const snapshot = bootstrap.getSnapshot();
      if (snapshot.phase === 'unreachable' && !snapshot.unreachable?.retrying && restores === 2) resolve();
    });
  });
  globalThis.dispatchEvent(new Event('focus'));
  await retried;
  unsubscribe();
  assert.equal(restores, 2);
  bootstrap.dispose();
  t.mock.timers.tick(120000);
  // A retired wait has no wake path left: timer, focus, network or view return.
  globalThis.dispatchEvent(new Event('focus'));
  globalThis.dispatchEvent(new Event('online'));
  globalThis.document.dispatchEvent(new Event('visibilitychange'));
  for (let flush = 0; flush < 4; flush++) await Promise.resolve();
  assert.equal(restores, 2);
});


/* ---- lifecycle: the wait belongs to exactly one launch attempt ---- */

const savedRecord = {version: 1, serverUrl: 'https://demo.getportico.local:32500', session: {sessionFamilyId: 'family-a',
  viewer: {serverId: 'server', authority: 'local', accountId: 'account', profileId: 'profile', role: 'owner'}}};
const transport = (code?: string) => Object.assign(new TypeError('Failed to fetch'), code ? {code} : {});
const deferred = <T,>() => { let resolve!: (value: T) => void; const promise = new Promise<T>(done => { resolve = done; }); return {promise, resolve}; };
const bootstrapped = (restore: () => Promise<unknown>) => new ShellBootstrap(async () => { throw new Error('unexpected assembly'); }, {
  selected: {read: async () => savedRecord as never}, restore: restore as never, actions: () => ({switchServer: async () => {}}) as never,
});

test('selecting another server drops the old retry instead of letting it start a restore', async t => {
  t.mock.timers.enable({apis: ['setTimeout', 'Date']});
  const calls: number[] = [];
  let call = 0;
  const bootstrap = bootstrapped(async () => { calls.push(++call); if (call === 1) throw transport(); return undefined; });
  await bootstrap.start();
  assert.equal(bootstrap.getSnapshot().phase, 'unreachable');
  await bootstrap.selectServer('another-server');
  assert.equal(bootstrap.getSnapshot().phase, 'signed-out');
  t.mock.timers.tick(10000);
  for (let flush = 0; flush < 6; flush++) await Promise.resolve();
  assert.deepEqual(calls, [1, 2]);
  bootstrap.dispose();
});

test('a retired attempt that settles late changes neither the phase nor a later wait', async t => {
  t.mock.timers.enable({apis: ['setTimeout', 'Date']});
  const first = deferred<undefined>(), settles: string[] = [];
  const bootstrap = bootstrapped(async () => { settles.push('restore'); return first.promise; });
  const retired = bootstrap.start();
  await Promise.resolve(); await Promise.resolve();
  assert.deepEqual(settles, ['restore']);
  bootstrap.suspendForAccountEntry();
  assert.equal(bootstrap.getSnapshot().phase, 'signed-out');
  first.resolve(undefined); await retired; await Promise.resolve(); await Promise.resolve();
  assert.equal(bootstrap.getSnapshot().phase, 'signed-out');
  t.mock.timers.tick(120000);
  for (let flush = 0; flush < 4; flush++) await Promise.resolve();
  assert.deepEqual(settles, ['restore']);
  bootstrap.dispose();
});

test('only allowlisted transport codes can reach the page or the clipboard', async () => {
  assert.equal(displayableTransportCode('route_unavailable'), 'route_unavailable');
  assert.equal(displayableTransportCode('request_failed'), 'request_failed');
  for (const unsafe of ['restore_integrity_failed', 'https://user:pw@server/x', 'auth.some-secret', '', 7, null]) assert.equal(displayableTransportCode(unsafe), undefined, String(unsafe));
  const codes = [transport('route_unavailable'), transport('https://user:pw@server/x')];
  let call = 0;
  const bootstrap = bootstrapped(async () => { throw codes[call++]; });
  await bootstrap.start();
  const first = bootstrap.getSnapshot();
  assert.equal(first.phase, 'unreachable');
  assert.equal(first.phase === 'unreachable' ? first.unreachable.code : 'x', 'route_unavailable');
  await bootstrap.retryNow();
  const second = bootstrap.getSnapshot();
  assert.equal(second.phase, 'unreachable');
  assert.equal(second.phase === 'unreachable' ? second.unreachable.code : 'x', undefined);
  bootstrap.dispose();
});
