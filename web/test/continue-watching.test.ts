import test from 'node:test';
import assert from 'node:assert/strict';
import {removeFromContinueWatching} from '../src/app/continue-watching.ts';

test('X-12: removal is the server’s dismissal, which keeps the resume point; Undo restores it', async () => {
  const calls: {path: string; method?: string; body: any}[] = [];
  let revision = 7;
  const api = {request: async <T,>(path: string, method?: string, body?: unknown) => { calls.push({path, method, body}); revision++; return {personal: {revision}} as T; }};
  const done = await removeFromContinueWatching(api, 'itm 1', 7);
  assert.equal(calls[0]!.path, '/v1/items/itm%201/personal-state');
  assert.equal(calls[0]!.method, 'PUT');
  assert.deepEqual({...calls[0]!.body, operationId: 'x'}, {operationId: 'x', expectedRevision: 7, continueDismissed: true});
  assert.equal('watched' in calls[0]!.body || 'progressSeconds' in calls[0]!.body, false, 'never marks watched or touches the resume point');
  await done.undo();
  assert.deepEqual({...calls[1]!.body, operationId: 'x'}, {operationId: 'x', expectedRevision: 8, continueDismissed: false}, 'undo is based on the removal’s revision');
  assert.notEqual(calls[0]!.body.operationId, calls[1]!.body.operationId);
});

test('a refusal rejects, so the card comes back', async () => {
  const api = {request: async () => { throw Object.assign(new Error('nope'), {status: 403, code: 'forbidden'}); }};
  await assert.rejects(removeFromContinueWatching(api as never, 'itm', 1));
});
