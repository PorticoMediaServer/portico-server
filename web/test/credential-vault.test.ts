import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';
import {parseSelectedViewerRecord, selectedViewerRecord} from '../../packages/client-core/src/viewer-selection.ts';
import {parseTrustedProfile, trustScopeKey} from '../../packages/client-core/src/profile-management.ts';

/** SEC-12: sign-in tokens are never script-readable plaintext. The records live in the encrypted
 * vault (`BrowserProtectedStore`, faked here as a map per database) and old plaintext entries are
 * moved once and deleted. */
function fakeLocalStorage(initial: Record<string, string> = {}) {
  const m = new Map(Object.entries(initial));
  return {m, api: {get length() { return m.size; }, key: (i: number) => [...m.keys()][i] ?? null, getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => { m.set(k, v); }, removeItem: (k: string) => { m.delete(k); }}};
}
function fakeVault() {
  const dbs = new Map<string, unknown>();
  class BrowserProtectedStore<T> {
    name: string;
    constructor(name: string) { this.name = name; }
    async read() { return structuredClone(dbs.get(this.name)) as T | undefined; }
    async change(update: (c: T | undefined) => T | undefined) { const next = update(structuredClone(dbs.get(this.name)) as T | undefined); if (next === undefined) dbs.delete(this.name); else dbs.set(this.name, structuredClone(next)); }
  }
  return {dbs, BrowserProtectedStore};
}
const until = new Date(Date.now() + 3600000).toISOString();
const session = {accessToken: 'secret-access-token', expiresAt: until, sessionFamilyId: 'fam', tokenGeneration: '1', authorizationHorizon: until, viewer: {accountId: 'a', profileId: 'p', serverId: 's', authority: 'local', role: 'member'}};

test('the selected viewer (with its access token) moves out of plaintext localStorage into the vault, once', async () => {
  const legacy = JSON.stringify(selectedViewerRecord('https://home.example', session));
  const ls = fakeLocalStorage({'portico.selected-viewer.v1': legacy});
  (globalThis as any).localStorage = ls.api;
  const vault = fakeVault();
  const {browserViewerStorage} = await componentModule(new URL('../src/bridge/viewer-selection-storage.ts', import.meta.url), {react: {default: {}}, '@core/viewer-selection': {parseSelectedViewerRecord}, './server-connections': {BrowserProtectedStore: vault.BrowserProtectedStore}}) as any;
  const read = await browserViewerStorage.read();
  assert.equal(read.session.accessToken, 'secret-access-token');
  assert.equal(ls.m.has('portico.selected-viewer.v1'), false, 'the plaintext copy is deleted');
  assert.ok(vault.dbs.has('portico.selected-viewer.v2'));
  assert.equal([...ls.m.values()].some(v => v.includes('secret-access-token')), false, 'no token left in localStorage');
  await browserViewerStorage.write(null);
  assert.equal(await browserViewerStorage.read(), null);
  assert.equal(vault.dbs.has('portico.selected-viewer.v2'), false);
});

test('an old quarantine flag means no viewer, and both old keys are removed', async () => {
  const ls = fakeLocalStorage({'portico.selected-viewer.v1': 'x', 'portico.selected-viewer.quarantined.v1': 'true'});
  (globalThis as any).localStorage = ls.api;
  const vault = fakeVault();
  const {browserViewerStorage} = await componentModule(new URL('../src/bridge/viewer-selection-storage.ts', import.meta.url), {react: {default: {}}, '@core/viewer-selection': {parseSelectedViewerRecord}, './server-connections': {BrowserProtectedStore: vault.BrowserProtectedStore}}) as any;
  assert.equal(await browserViewerStorage.read(), null);
  assert.equal(ls.m.size, 0);
});

test('remembered-profile trust handles move into the vault and are read from memory', async () => {
  const scope = {authority: 'local' as const, accountId: 'a', serverId: 's', profileId: 'p', installationId: 'inst'};
  const proof = {...scope, token: 't'.repeat(43), pinRevision: 1, profileRevision: 1, membershipRevision: 1, expiresAt: new Date(Date.now() + 86400000).toISOString().replace(/\.\d{3}Z$/, 'Z')};
  const ls = fakeLocalStorage({[trustScopeKey(scope)]: JSON.stringify(proof), 'portico.profile-installation.v1': 'inst'});
  (globalThis as any).localStorage = ls.api;
  const vault = fakeVault();
  const trust = await componentModule(new URL('../src/bridge/profile-trust.ts', import.meta.url), {react: {default: {}}, '@core/profile-management': {parseTrustedProfile, trustScopeKey}, './server-connections': {BrowserProtectedStore: vault.BrowserProtectedStore}}) as any;
  assert.equal(trust.readBrowserTrust(scope), undefined, 'before loading: ask for the PIN');
  await trust.browserTrustLoaded();
  assert.equal(ls.m.has(trustScopeKey(scope)), false, 'plaintext handle deleted');
  assert.equal(ls.m.get('portico.profile-installation.v1'), 'inst', 'the non-secret installation id stays');
  const read = trust.readBrowserTrust(scope);
  assert.equal(read?.token, proof.token, 'read from the vault copy in memory');
  assert.ok(JSON.stringify(vault.dbs.get('portico.profile-trust.v2')).includes(proof.token), 'kept in the (encrypted) vault');
  trust.forgetBrowserTrust(scope);
  assert.equal(trust.readBrowserTrust(scope), undefined);
});
