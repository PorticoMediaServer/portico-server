import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {test} from 'node:test';
import {HttpLocalApi, ReadingError, parsePreferenceSnapshot} from '@portico/client-core';
import {routePath} from '../../../packages/contracts/src/index.ts';
import {createReadBindings} from '../../src/screens/read/bindings.ts';

const preferences = parsePreferenceSnapshot(JSON.parse(readFileSync(new URL('../../../server/testdata/contracts/get-bootstrap.json', import.meta.url), 'utf8')).response.preferences);
function fixture() {
  let selected = {sessionFamilyId: 'unit-family', viewer: {serverId: 'unit-server', accountId: 'unit-account', profileId: 'unit-profile', authority: 'local', role: 'member'}};
  const calls: {path: string; method: string}[] = [];
  let sequence = 0;
  const api = new HttpLocalApi('http://127.0.0.1:39117', 'unit-captured-token', async (input, init = {}) => {
    const path = new URL(String(input)).pathname; calls.push({path, method: init.method ?? 'GET'});
    if (path === routePath('post_reading_sessions')) return new Response(JSON.stringify({itemId: 'unit-book', sourceId: 'unit-source', sessionId: `unit-session-${++sequence}`,
      revision: 'a'.repeat(64), format: 'pdf', size: 1024, metadata: {title: 'Stored unit title'}}), {headers: {'Content-Type': 'application/json', ETag: '"' + 'a'.repeat(64) + '"'}});
    if (init.method === 'DELETE') return new Response(null, {status: 204});
    throw new Error('unexpected unit operation');
  });
  const source = {getSnapshot: () => preferences, subscribe: () => () => {}};
  const bound = createReadBindings({api, session: () => selected, isCurrent: () => true, preferences: source, facts: {preferences}});
  return {bound, calls, source, api, switchProfile() {selected = {...selected, viewer: {...selected.viewer, profileId: 'different-profile'}};}};
}

test('captured Read bindings are lazy and explicitly close the previous real facade session before replacement', async () => {
  const f = fixture(); assert.equal(f.calls.length, 0);
  assert.strictEqual(f.bound.preferences.getSnapshot(), preferences);
  const first = await f.bound.open({itemId: 'unit-book', sourceId: 'unit-source'});
  const second = await f.bound.open({itemId: 'unit-book', sourceId: 'unit-source'});
  assert.notEqual(first.manifest.sessionId, second.manifest.sessionId); assert.equal(first.signal.aborted, true);
  assert.deepEqual(f.calls.map(call => call.method), ['POST', 'DELETE', 'POST']);
  await f.bound.dispose(); assert.deepEqual(f.calls.map(call => call.method), ['POST', 'DELETE', 'POST', 'DELETE']);
  assert.equal(f.bound.client.signal.aborted, true); assert.equal(f.bound.authority.isCurrent(), false);
  await f.bound.dispose(); assert.equal(f.calls.length, 4);
});

test('profile replacement retires locally without a cleanup request under another viewer', async () => {
  const f = fixture(), opened = await f.bound.open({itemId: 'unit-book', sourceId: 'unit-source'});
  f.switchProfile(); assert.equal(f.bound.authority.isCurrent(), false);
  await assert.rejects(f.bound.open({itemId: 'unit-book', sourceId: 'unit-source'}), error => error instanceof ReadingError && error.code === 'cancelled');
  await f.bound.dispose(); assert.equal(opened.signal.aborted, true); assert.deepEqual(f.calls.map(call => call.method), ['POST']);
});

test('token retirement uses the captured facade lifetime and notifies the mounted reader', async () => {
  const f = fixture(); let retired = 0;
  const unsubscribe = f.bound.authority.subscribe(() => retired++);
  await f.bound.open({itemId: 'unit-book', sourceId: 'unit-source'});
  f.api.setAccessToken('unit-replacement-token'); assert.equal(f.bound.authority.isCurrent(), false); assert.equal(retired, 1);
  unsubscribe(); await f.bound.dispose(); assert.equal(retired, 1); assert.deepEqual(f.calls.map(call => call.method), ['POST']);
});
