import assert from 'node:assert/strict';
import test from 'node:test';
import {componentModule} from './helpers/component-harness.mjs';
import {createRenderer, tree} from './helpers/interaction-harness.mjs';

// Mount both actual components and the real flow. A same-family account render
// must not dispose the pending directory read before it can reach the server.
async function fixture() {
  const saved = Object.fromEntries(['location', 'document', 'addEventListener', 'removeEventListener', 'setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'].map(key => [key, globalThis[key]]));
  const destinations = [], timers = new Set();
  globalThis.location = {pathname: '/', search: '', hash: '', assign: origin => destinations.push(origin)};
  globalThis.document = {visibilityState: 'visible', addEventListener() {}, removeEventListener() {}};
  globalThis.addEventListener = globalThis.removeEventListener = () => {};
  globalThis.setTimeout = globalThis.setInterval = run => {const timer = {run}; timers.add(timer); return timer;};
  globalThis.clearTimeout = globalThis.clearInterval = timer => timers.delete(timer);
  const h = createRenderer(), session = {account: {id: 'account', username: 'owner'}, familyId: 'family', accessToken: 'original'};
  const account = {service: {getSnapshot: () => ({session})}};
  const server = {id: 'srv_' + 'a'.repeat(43), name: 'Actual server', status: 'quiet', ownedByMe: true, role: 'owner',
    publicKey: 'p'.repeat(43), policyRevision: 1, baseUrl: 'https://server.example:32500', presence: {lastSeenAt: null}};
  const value = {items: [server], version: '1', watch: '1'};
  let reads = 0, resolveRead, readSignal, connections = 0;
  const api = {servers: signal => {
    if (++reads === 1) return Promise.resolve(value);
    readSignal = signal;
    return new Promise(resolve => {resolveRead = resolve;});
  }};
  const errors = {accountDirectoryFailure: error => error, directoryFailurePhase: () => undefined, DirectoryAccountUnavailable: class extends Error {}};
  const flow = await componentModule(new URL('../src/services/directory-flow.ts', import.meta.url), {'./directory-errors': errors});
  const directory = await componentModule(new URL('../src/services/DirectoryPage.tsx', import.meta.url), {
    react: h.react, './directory-flow': flow, './directory-errors': errors,
    './session': {accountIdentity: value => ({accountId: value.account.id, familyId: value.familyId}),
      sameAccount: (a, b) => a.accountId === b.accountId && a.familyId === b.familyId},
    './directory-connection': {servicesDirectoryConnection: () => async () => {connections++; return {origin: server.baseUrl, dispose() {}};}},
    './i18n': {servicesI18n: () => ({t: key => key})}, './server-directory': {servicesServerDirectoryCopy: () => ({})},
    '../kit/accounts/ServerDirectory': {ServicesServerDirectory: 'Directory'},
    '../kit/accounts/ServicesAuth': {ServicesAuthStage: 'Stage', HostedError: 'Error'},
  });
  const home = await componentModule(new URL('../src/services/HomePage.tsx', import.meta.url), {
    react: h.react, './DirectoryPage': directory, './workspace-api': {workspaceApi: () => api},
    '../shell/DocumentBootReady': {DocumentBootReady: ({children}) => children},
    './session': {accountIdentity: value => ({accountId: value.account.id, familyId: value.familyId})},
    './i18n': {servicesI18n: () => ({t: key => key})},
    '../kit/accounts/ServicesAuth': {ServicesAuthStage: 'Stage'},
    '../kit/accounts/ServicesWorkspace': {ServicesHeader: 'Header'},
  });
  const render = () => tree(h.render(() => home.ServicesHomePage({account, session: {...session}})));
  const settle = async () => {let result; for (let i = 0; i < 8; i++) {result = render(); await Promise.resolve();} return result;};
  await settle();
  return {render, settle, destinations, open: () => render().find(node => node.type === 'Directory').props.onOpen(server.id),
    finishRead: () => resolveRead(value), signal: () => readSignal, connections: () => connections,
    rotate: () => {session.accessToken = 'renewed-same-family';},
    close: () => {h.dispose(); Object.assign(globalThis, saved);}};
}

test('account synchronization rerenders preserve the pending real Home directory connection', async () => {
  const f = await fixture();
  try {
    f.open(); await Promise.resolve();
    const signal = f.signal(); assert.ok(signal); assert.equal(signal.aborted, false);
    f.rotate(); await f.settle();
    assert.equal(signal.aborted, false, 'same-family account rendering must not dispose its pending flow');
    f.finishRead(); await f.settle();
    assert.equal(f.connections(), 1);
    assert.deepEqual(f.destinations, ['https://server.example:32500/']);
  } finally {f.close();}
});

test('unmount still cancels the pending directory read and cannot navigate late', async () => {
  const f = await fixture();
  try {
    f.open(); await Promise.resolve();
    const signal = f.signal(); assert.ok(signal);
    f.close(); assert.equal(signal.aborted, true);
    f.finishRead(); await Promise.resolve(); await Promise.resolve();
    assert.equal(f.connections(), 0); assert.deepEqual(f.destinations, []);
  } finally {f.close();}
});
