import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

const load = () => componentModule(new URL('../src/screens/server/Storage.tsx', import.meta.url), {
  react: {default: {}, useEffect: () => {}, useMemo: (f: () => unknown) => f(), useState: (v: unknown) => [v, () => {}]},
  '@core/storage-management.ts': {}, '@core/remote-sources.ts': {}, '../../admin/console': {},
  '../../app/content': {}, '@core/presentation/index.ts': {}, '../../app/session': {},
  '../../app/viewer-scope': {}, '../../app/i18n': {}, '../../ui': {}, './Server': {},
}) as Promise<any>;

// The shared operation-ID shape both client services and the server enforce:
// 13-digit epoch milliseconds, a UUID, and a 10-minute freshness window.
const VALID = /^\d{13}-[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;

test('CD-09: operation IDs carry a 13-digit millisecond prefix and a UUID', async () => {
  const {storageOperationId} = await load();
  const before = Date.now();
  const id: string = storageOperationId();
  assert.match(id, VALID);
  const ms = Number(id.slice(0, 13));
  assert.ok(ms >= before && ms <= Date.now(), 'freshness window opens now');
});

test('CD-09: retrying one operation reuses its ID; a new operation takes a new one', async () => {
  const {stableStorageOperationId, storageOperationId} = await load();
  const op = stableStorageOperationId();
  const first: string = await op.next();
  assert.match(first, VALID);
  assert.equal(await op.next(), first, 'same-operation retry reuses the ID');
  op.release();
  const second: string = await op.next();
  assert.match(second, VALID);
  assert.notEqual(second, first, 'a settled operation releases its ID');
  assert.notEqual(storageOperationId(), storageOperationId(), 'fresh IDs differ');
});

test('CD-09: WebDAV create sends fresh credentials with both keep flags false', async () => {
  const {davConfiguration} = await load();
  const body = davConfiguration(null, {name: 'Cloud', root: 'https://cloud.example.com/dav', username: 'me', password: 'secret', insecureLocal: false});
  assert.deepEqual(body, {name: 'Cloud', root: 'https://cloud.example.com/dav', username: 'me', password: 'secret', keepPassword: false, keepConnection: false, insecureLocal: false});
  assert.ok(!('expectedGeneration' in body), 'create carries no generation fence');
});

test('CD-09: WebDAV edit with an untouched address keeps the connection, and the password only when unreplaced', async () => {
  const {davConfiguration} = await load();
  const kept = davConfiguration({generation: 4}, {name: 'Cloud', root: '', username: '', password: '', insecureLocal: false});
  assert.deepEqual(kept, {name: 'Cloud', root: '', username: '', password: '', keepPassword: true, keepConnection: true, insecureLocal: false, expectedGeneration: 4});
  const replaced = davConfiguration({generation: 4}, {name: 'Cloud', root: '', username: '', password: 'new-secret', insecureLocal: false});
  assert.equal(replaced.keepPassword, false);
  assert.equal(replaced.password, 'new-secret');
  assert.equal(replaced.keepConnection, true);
  assert.equal(replaced.expectedGeneration, 4);
});

test('CD-09: WebDAV edit with a new address never reuses the stored connection or password', async () => {
  const {davConfiguration} = await load();
  const changed = davConfiguration({generation: 4}, {name: 'Cloud', root: 'https://other.example.com/dav', username: 'me', password: '', insecureLocal: false});
  assert.deepEqual(changed, {name: 'Cloud', root: 'https://other.example.com/dav', username: 'me', password: '', keepPassword: false, keepConnection: false, insecureLocal: false, expectedGeneration: 4});
  const explicit = davConfiguration({generation: 4}, {name: 'Cloud', root: 'https://other.example.com/dav', username: 'you', password: 'typed', insecureLocal: false});
  assert.equal(explicit.username, 'you');
  assert.equal(explicit.password, 'typed');
  assert.equal(explicit.keepPassword, false);
  assert.equal(explicit.keepConnection, false);
});

test('CD-09: only an accepted mount receipt counts as success — a refusal never shows Added', async () => {
  const {mountCommandSucceeded, davCommandSucceeded} = await load();
  assert.equal(mountCommandSucceeded({phase: 'accepted'}), true);
  for (const phase of ['idle', 'pending', 'error', 'conflict', 'ambiguous']) assert.equal(mountCommandSucceeded({phase}), false);
  assert.equal(davCommandSucceeded({ambiguous: false, receipt: {accepted: true}}), true);
  assert.equal(davCommandSucceeded({ambiguous: true, receipt: {accepted: true}}), false);
  assert.equal(davCommandSucceeded({ambiguous: false, receipt: null}), false);
});
