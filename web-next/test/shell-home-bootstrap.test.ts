import test from 'node:test';
import assert from 'node:assert/strict';
import {ShellBootstrap, sameShellAuthority, type ShellAssembler} from '../src/shell/bootstrap.ts';
import type {HttpLocalApi, LocalSession} from '@core/index.ts';
import type {SelectedViewerRecord} from '@core/viewer-selection.ts';

const unavailableAssembly: ShellAssembler = async () => { throw new Error('Unexpected assembly.'); };
const selected = {session: {sessionFamilyId: 'family-a'}} as SelectedViewerRecord;
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(done => { resolve = done; }); return {promise, resolve}; }

test('an absent protected selection reaches the registered signed-out flow without reading a server', async () => {
  let reads = 0;
  const bootstrap = new ShellBootstrap(unavailableAssembly, {selected: {read: async () => null}, restore: async () => { reads++; return undefined; }});
  await bootstrap.start();
  assert.equal(bootstrap.getSnapshot().phase, 'signed-out');
  assert.equal(reads, 0);
  bootstrap.dispose();
});

test('retiring while the vault is opening prevents a later selection from restoring authority', async () => {
  const vault = deferred<SelectedViewerRecord | null>(); let reads = 0;
  const bootstrap = new ShellBootstrap(unavailableAssembly, {selected: {read: () => vault.promise}, restore: async () => { reads++; return undefined; }});
  const work = bootstrap.start(); bootstrap.dispose(); vault.resolve(selected); await work;
  assert.equal(bootstrap.getSnapshot().phase, 'signed-out'); assert.equal(reads, 0);
});

test('a route restored after retirement is disposed and never assembled', async () => {
  const route = deferred<{api: HttpLocalApi; session: LocalSession} | undefined>(); let disposed = 0, assembling = 0;
  const bootstrap = new ShellBootstrap(async context => { assembling++; return unavailableAssembly(context); }, {
    selected: {read: async () => selected}, restore: () => route.promise,
  });
  const work = bootstrap.start(); await Promise.resolve(); bootstrap.dispose();
  route.resolve({api: {getRouteConnection: () => ({dispose: () => { disposed++; }})} as unknown as HttpLocalApi, session: selected.session}); await work;
  assert.equal(disposed, 1); assert.equal(assembling, 0); assert.equal(bootstrap.getSnapshot().phase, 'signed-out');
});

test('a failed protected read exposes retry state and a later explicit retry can restore signed-out state', async () => {
  let attempt = 0, observed = 0;
  const bootstrap = new ShellBootstrap(unavailableAssembly, {selected: {read: async () => { if (attempt++ === 0) throw new Error('vault_busy'); return null; }}, restore: async () => undefined});
  const off = bootstrap.subscribe(() => { observed++; });
  await bootstrap.start(); assert.equal(bootstrap.getSnapshot().phase, 'error');
  await bootstrap.start(); assert.equal(bootstrap.getSnapshot().phase, 'signed-out'); assert.ok(observed >= 4);
  off(); const before = observed; bootstrap.dispose(); assert.equal(observed, before);
});

test('token rotation retains a scope but every authority or family replacement retires it', () => {
  const session = {sessionFamilyId: 'family-a', accessToken: 'test-token-a', viewer: {serverId: 'server-a', accountId: 'account-a', profileId: 'profile-a', authority: 'local', role: 'owner'}} as LocalSession;
  assert.equal(sameShellAuthority(session, {...session, accessToken: 'test-token-b'}), true);
  assert.equal(sameShellAuthority(session, {...session, sessionFamilyId: 'family-b'}), false);
  for (const key of ['serverId', 'accountId', 'profileId', 'authority', 'role'] as const) {
    assert.equal(sameShellAuthority(session, {...session, viewer: {...session.viewer, [key]: 'replacement'}} as LocalSession), false, key);
  }
});
