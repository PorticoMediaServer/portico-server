import test from 'node:test';
import assert from 'node:assert/strict';
import {HttpLocalApi, ReadingError, type Viewer} from '../src/index.ts';
import {routePath} from '../../contracts/src/index.ts';

const viewer: Viewer = {accountId: 'account', profileId: 'profile', serverId: 'server', authority: 'local', role: 'member'};
const item = {id: 'book', libraryId: 'library', title: 'Stored title', sourceId: 'asset', format: 'pdf', inventoryAvailable: false};
const response = (value: unknown) => new Response(JSON.stringify(value), {headers: {'Content-Type': 'application/json'}});
const coded = (code: string) => (error: unknown) => error instanceof ReadingError && error.code === code;

test('Discover uses captured authority and the generated route, retaining unavailable items and exact titles', async () => {
  const calls: Array<{url: URL; headers: Headers}> = [];
  const api = new HttpLocalApi('https://server.example', 'original-token', async (input, init) => {
    calls.push({url: new URL(String(input)), headers: new Headers(init?.headers)});
    return response({rows: [{key: 'recent', items: [item]}]});
  });
  const client = api.reading({viewer, isCurrent: () => true});
  const out = await client.discover({libraryId: 'library', limit: 2});
  assert.equal(calls[0].url.pathname, routePath('get_reading_discover'));
  assert.equal(calls[0].url.search, '?libraryId=library&limit=2');
  assert.equal(calls[0].headers.get('Authorization'), 'Bearer original-token');
  assert.deepEqual(out.rows[0].items, [item]);
  assert.ok(Object.isFrozen(out.rows[0].items));
  api.setAccessToken('successor-token');
  await assert.rejects(client.discover(), coded('cancelled'));
  assert.equal(calls.length, 1);
  client.dispose();
});

test('Discover rejects invented rows, duplicate or out-of-scope items and oversized pages', async () => {
  for (const rows of [
    [{key: 'series', items: [item]}],
    [{key: 'recent', items: [item, item]}],
    [{key: 'recent', items: [{...item, libraryId: 'other'}]}],
    [{key: 'recent', items: [item, {...item, id: 'second'}]}],
  ]) {
    const api = new HttpLocalApi('https://server.example', 'token', async () => response({rows}));
    const client = api.reading({viewer, isCurrent: () => true});
    await assert.rejects(client.discover({libraryId: 'library', limit: 1}), coded('invalid_document'));
    client.dispose();
  }
});

test('Discover refuses invalid limits before IO and cancels an in-flight retired owner', async () => {
  let calls = 0, release!: (value: Response) => void;
  const api = new HttpLocalApi('https://server.example', 'token', async () => {
    calls++;
    return new Promise<Response>(resolve => { release = resolve; });
  });
  const client = api.reading({viewer, isCurrent: () => true});
  for (const limit of [0, 101, 1.5, NaN]) await assert.rejects(client.discover({limit}), coded('invalid_request'));
  assert.equal(calls, 0);
  const pending = client.discover();
  client.dispose();
  await assert.rejects(pending, coded('cancelled'));
  release(response({rows: [{key: 'recent', items: [item]}]}));
});
