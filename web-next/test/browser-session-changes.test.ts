import test from 'node:test';
import assert from 'node:assert/strict';
import {BrowserProtectedStore, browserAppSessions, observeBrowserSessionChanges} from '../src/platform/server-connections.ts';

test('actual app-session writer emits a noncredential read hint only after commit, in this tab and other tabs', async t => {
  const page = new EventTarget(), channels: Channel[] = [], sent: unknown[] = [];
  class Channel extends EventTarget {
    constructor(readonly name: string) {super(); channels.push(this);}
    postMessage(data: unknown) {sent.push(data); for (const other of channels) if (other !== this && other.name === this.name) other.dispatchEvent(new MessageEvent('message', {data}));}
    close() {channels.splice(channels.indexOf(this), 1);}
  }
  for (const [name, value] of [['window', page], ['BroadcastChannel', Channel]] as const) {
    const old = Object.getOwnPropertyDescriptor(globalThis, name);
    Object.defineProperty(globalThis, name, {configurable: true, value});
    t.after(() => {if (old) Object.defineProperty(globalThis, name, old); else delete (globalThis as Record<string, unknown>)[name];});
  }
  const connection = (id: string) => ({signIn: 'local', connection: {version: '1', selectionId: id.padEnd(32, '_'), logicalOrigin: 'https://' + id + '.test', pin: {serverId: id, publicKey: 'p', fingerprint: 'f'}, routes: {paired: ['https://' + id + '.test']}, session: {sessionFamilyId: 'family-' + id, tokenGeneration: '1', accessToken: 'private-fixture-credential-' + id, expiresAt: '2099-01-01T00:00:00Z', authorizationHorizon: '2099-02-01T00:00:00Z', viewer: {serverId: id, accountId: 'account', profileId: 'main', authority: 'local', role: 'member'}}}});
  let raw: unknown = {version: 1, revision: 1, selectedServerId: 'first', servers: [connection('first'), connection('second')]};
  let release: (() => void) | undefined, commit: Promise<void> | undefined, failing = false, committed = false;
  t.mock.method(BrowserProtectedStore.prototype, 'read', async () => raw);
  t.mock.method(BrowserProtectedStore.prototype, 'change', async (update: (old: unknown) => unknown) => {
    const next = update(raw);
    if (failing) throw new Error('Durable write refused.');
    if (commit) await commit;
    raw = next; committed = true;
  });
  let heard = 0;
  const stop = observeBrowserSessionChanges(() => {assert.equal(committed, true); heard++;});
  t.after(stop); t.after(() => {for (const channel of [...channels]) channel.close();});
  const store = browserAppSessions();
  commit = new Promise<void>(resolve => {release = resolve;});
  const selecting = store.select('second'); await new Promise(resolve => setImmediate(resolve));
  assert.equal(heard, 0); assert.deepEqual(sent, []);
  release!(); await selecting; commit = undefined;
  assert.equal((await store.read()).selectedServerId, 'second'); assert.equal(heard, 1); assert.deepEqual(sent, ['changed']);
  await store.signOutServer('absent', 'absent-family');
  assert.equal(heard, 1, 'a no-op is not a durable change');
  failing = true; await assert.rejects(store.select('first')); failing = false;
  assert.equal(heard, 1, 'a failed write publishes nothing'); assert.deepEqual(sent, ['changed']);
  const otherTab = new Channel('portico.app-session-change.v1');
  otherTab.postMessage('changed'); assert.equal(heard, 2);
  otherTab.postMessage({changed: true}); assert.equal(heard, 2, 'unknown channel payload is not an authority event');
  page.dispatchEvent(new Event('focus')); page.dispatchEvent(new Event('pageshow')); assert.equal(heard, 4);
  const broken = observeBrowserSessionChanges(() => {throw new Error('Consumer failed.');});
  await store.select('first'); assert.equal(heard, 5, 'a failed observer cannot fail the committed write or another observer'); broken();
  stop(); otherTab.postMessage('changed'); page.dispatchEvent(new Event('focus')); assert.equal(heard, 5);
});
