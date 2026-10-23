import test from 'node:test';
import assert from 'node:assert/strict';
import type {AccountEntryState} from '../src/platform/account-entry.ts';
import {initializeAccountSignIn} from '../src/screens/accounts/sign-in-start.ts';

const captured = Object.freeze({href: 'https://server.test/auth/portico-return', fragment: '#opaque-captured-value'});
const deferred = <T,>() => {let resolve!: (value: T) => void; const promise = new Promise<T>(done => {resolve = done;}); return {promise, resolve};};
function setup() {
  const calls: unknown[] = [];
  let state: AccountEntryState = {phase: 'welcome'};
  const entry = {
    getSnapshot: () => state,
    finishPorticoReturn: async (value: typeof captured) => {calls.push(['finish', value]);},
    loadProfiles: async () => {calls.push(['profiles']);},
    inspect: async (origin: string) => {calls.push(['inspect', origin]); return undefined;},
  };
  const ports = {pageOrigin: 'https://actual-page.test', isCurrent: () => true,
    selectedOrigin: async (): Promise<string | undefined> => {calls.push(['selected']); return undefined;}, username: (value: string) => {calls.push(['username', value]);}};
  return {entry, ports, calls, set: (value: AccountEntryState) => {state = value;}};
}
test('the one-use original return goes straight to its canonical controller without another consent or store read', async () => {
  const f = setup(); let takes = 0;
  await initializeAccountSignIn(f.entry, {...f.ports, takeReturn: () => {takes++; return captured;}});
  assert.equal(takes, 1); assert.deepEqual(f.calls, [['finish', captured]]);
  assert.equal((f.calls[0] as unknown[])[1], captured);
});
test('a fresh server page proves its actual origin before showing credentials', async () => {
  const f = setup(); await initializeAccountSignIn(f.entry, f.ports);
  assert.deepEqual(f.calls, [['selected'], ['inspect', f.ports.pageOrigin]]);
});
test('restored restricted accounts re-enter existing sign-in using only actual name and origin facts', async () => {
  const f = setup(); f.set({phase: 'two-step-setup', origin: 'https://proved-server.test', snapshot: {account: {username: 'genuine-user'}} as NonNullable<AccountEntryState['snapshot']>});
  await initializeAccountSignIn(f.entry, {...f.ports, selectedOrigin: async () => 'https://saved-server.test'});
  assert.deepEqual(f.calls, [['profiles'], ['username', 'genuine-user'], ['inspect', 'https://proved-server.test']]);
});
test('a replaced page cannot use a selected-origin answer that arrives after disposal', async () => {
  const f = setup(), pending = deferred<string | undefined>(); let current = true;
  const work = initializeAccountSignIn(f.entry, {...f.ports, selectedOrigin: () => pending.promise, isCurrent: () => current});
  current = false; pending.resolve('https://old.test'); await work;
  assert.deepEqual(f.calls, []);
});
