import test from 'node:test';
import assert from 'node:assert/strict';
import {routePath} from '../../packages/contracts/src/routes.ts';
import {SettingsApiKeysSource} from '../src/screens/settings/server/integrations/api-keys-source.ts';
const key = {id: 'key-1', kind: 'key', name: 'Automation', scopes: ['read'], hint: '1234', createdAt: '2026-10-22T19:00:00Z', state: 'active', member: {accountId: 'owner', username: 'Owner', profileId: 'profile', profile: 'Owner'}};
const page = {items: [key], limit: 100};
const options = {scopes: [{id: 'read'}, {id: 'admin'}], maximumKeys: 20, maximumTokens: 20, tokensAllowed: true, requestsPerMinute: 600, queryParameter: 'api_key'};
const read = (path: string) => path === routePath('get_api_access_options') ? options : page;

test('late pages cannot replace the latest cursor read', async () => {
  const releases: ((value: unknown) => void)[] = [];
  const source = new SettingsApiKeysSource({request: async <T>(path: string) => path === routePath('get_api_access_options') ? options as T : new Promise<T>(resolve => releases.push(value => resolve(value as T)))}, () => true, () => {});
  const old = source.load(), latest = source.load('next');releases[1]({...page, items: [{...key, id: 'key-2'}]});await latest;releases[0](page);await old;
  assert.equal(source.getSnapshot().page?.items[0].id, 'key-2');source.dispose();
});

test('uncertain key creation freezes the original intent and never stores a returned secret', async () => {
  const writes: Record<string, unknown>[] = [];let failed = false;
  const source = new SettingsApiKeysSource({request: async <T>(path: string, method?: string, body?: unknown) => {
    if (method === 'POST') {writes.push(body as Record<string, unknown>);if (!failed) {failed = true;throw new Error('Response lost');}return {credential: key} as T;}
    return read(path) as T;
  }}, () => true, () => {});
  await source.load();const scopes: ('read'|'admin')[] = ['read'];
  await source.create({name: 'Original', scopes, expiresAt: '2026-11-21T19:00:00Z'});scopes.push('admin');
  await source.create({name: 'Replacement', scopes: ['admin']});assert.equal(writes.length, 1);
  const issued = await source.retryCreate();assert.equal(issued?.secret, undefined);assert.deepEqual(writes[1], writes[0]);assert.equal(writes[1].name, 'Original');assert.deepEqual(writes[1].scopes, ['read']);assert.equal(source.getSnapshot().pendingCreate, false);source.dispose();
});

test('one-time secret is returned to the current caller without entering observable state', async () => {
  const source = new SettingsApiKeysSource({request: async <T>(path: string, method?: string) => (method === 'POST' ? {credential: key, secret: 'pk.secret-once'} : read(path)) as T}, () => true, () => {});
  await source.load();const issued = await source.create({name: 'Automation', scopes: ['read']});assert.equal(issued?.secret, 'pk.secret-once');assert.equal(JSON.stringify(source.getSnapshot()).includes('pk.secret-once'), false);source.dispose();
});

test('a retired authority drops a late issued secret and makes no later request', async () => {
  let current = true, release!: (value: unknown) => void, calls = 0;
  const source = new SettingsApiKeysSource({request: async <T>(path: string, method?: string) => {calls++;return method === 'POST' ? new Promise<T>(resolve => {release = value => resolve(value as T);}) : read(path) as T;}}, () => current, () => {});
  await source.load();const pending = source.create({name: 'Automation', scopes: ['read']});current = false;release({credential: key, secret: 'pk.retired'});assert.equal(await pending, undefined);
  const count = calls;await source.load();await source.retryCreate();assert.equal(calls, count);assert.equal(source.getSnapshot().phase, 'idle');source.dispose();
});

test('a token in the owner key list is refused whole and cannot be revoked here', async () => {
  let deletes = 0;const errors: unknown[] = [];
  const source = new SettingsApiKeysSource({request: async <T>(path: string, method?: string) => {if (method === 'DELETE') deletes++;return (path === routePath('get_api_access_options') ? options : {...page, items: [{...key, kind: 'token'}]}) as T;}}, () => true, error => errors.push(error));
  await source.load();assert.equal(source.getSnapshot().page, undefined);assert.equal(errors.length, 1);await source.revoke(key as never);assert.equal(deletes, 0);source.dispose();
});


test('synchronous retirement during busy publication cannot dispatch a credential command', async () => {
  let writes = 0;
  const source = new SettingsApiKeysSource({request: async <T>(path: string, method?: string) => {if (method === 'POST' || method === 'DELETE') writes++;return read(path) as T;}}, () => true, () => {});
  await source.load();source.subscribe(() => {if (source.getSnapshot().busy) source.dispose();});
  await source.create({name: 'Automation', scopes: ['read']});assert.equal(writes, 0);
});
