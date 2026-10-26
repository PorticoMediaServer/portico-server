import assert from 'node:assert/strict';
import test from 'node:test';
import {setImmediate as settle} from 'node:timers/promises';
import type {ProfileAPI} from '../../packages/client-core/src/profile-management.ts';
import {MemberResetCodeSource, type MemberResetCodeOpening} from '../src/screens/settings/member-reset-code.ts';

// Authored UNRUN against the real opening and canonical Make/replace helpers.
// Fixture strings are synthetic; no owner session, clipboard or server is used.
const serverId = 'srv_' + 'A'.repeat(43), codeA = 'A'.repeat(43), codeB = 'B'.repeat(43);
const expiresAt = '2030-01-01T01:00:00Z';
const original = {code: codeA, expiresAt, serverId};
const replacement = {code: codeB, expiresAt, serverId};
const target = Object.freeze({accountId: 'member-1', username: 'Member', role: 'member' as const, revision: 7});
function deferred<T>() {let resolve!: (value: T) => void, reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => {resolve = yes;reject = no;});return {promise, resolve, reject};}
function fixture() {
  let live = true, now = Date.parse('2030-01-01T00:00:00Z');
  const caller = new AbortController(), replies: Promise<unknown>[] = [], copies: string[] = [];
  const calls: {path: string; method?: string; body: unknown; signal?: AbortSignal}[] = [];
  const api: ProfileAPI = {request: async <T,>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T> => {
    calls.push({path, method, body, signal});return await (replies.shift() ?? Promise.resolve(original)) as T;
  }};
  const source = new MemberResetCodeSource(api, {serverId, accountId: 'owner-1', isCurrent: () => live,
    copy: async code => {copies.push(code);}, now: () => now});
  return {source, caller, calls, copies, replies, retire: () => {live = false;}, expire: () => {now = Date.parse(expiresAt);},
    open: () => source.open(target, {signal: caller.signal, isCurrent: () => live})!};
}
function ready(opening: MemberResetCodeOpening) {const state = opening.getSnapshot();assert.equal(state.kind, 'ready');if (state.kind !== 'ready') throw new Error('fixture not ready');return state;}

test('an explicit Make uses only the original target/revision and publishes the canonical response', async () => {
  const f = fixture(), opening = f.open();await settle();
  assert.deepEqual(f.calls.map(call => [call.path, call.method, call.body]), [
    ['/v1/direct/members/member-1/reset-code', 'POST', {expectedRevision: 7, turnOffTwoStep: false}],
  ]);
  assert.deepEqual(ready(opening).result, original);
});

test('replacement withdraws old code/copy before IO, claims once and forwards the exact original proof', async () => {
  const f = fixture(), opening = f.open();await settle();const shown = ready(opening), held = deferred<unknown>();f.replies.push(held.promise);
  const change = opening.change(shown, true);
  assert.deepEqual(opening.getSnapshot(), {kind: 'pending', turnOffTwoStep: true});
  await opening.copy(shown);await opening.change(shown, false);assert.deepEqual(f.copies, []);assert.equal(f.calls.length, 2);
  assert.deepEqual(f.calls[1].body, {expectedRevision: 7, turnOffTwoStep: true, originalResetCode: codeA});
  held.resolve(replacement);await change;assert.deepEqual(ready(opening).result, replacement);
  assert.equal(ready(opening).turnOffTwoStep, true);
});

test('a lost replacement remains unresolved with the original error, never restoring or resending', async () => {
  const f = fixture(), opening = f.open();await settle();const shown = ready(opening), error = new Error('network uncertain');
  f.replies.push(Promise.reject(error));await opening.change(shown, true);
  const state = opening.getSnapshot();assert.equal(state.kind, 'unresolved');assert.equal('result' in state, false);
  if (state.kind === 'unresolved') assert.equal(state.error, error);
  await opening.change(shown, false);await opening.copy(shown);await opening.issue();assert.equal(f.calls.length, 2);assert.deepEqual(f.copies, []);
});

test('a changed deadline, repeated code or foreign server is never published as a replacement', async () => {
  for (const response of [{...replacement, expiresAt: '2030-01-01T02:00:00Z'}, original, {...replacement, serverId: 'srv_' + 'B'.repeat(42) + 'A'}]) {
    const f = fixture(), opening = f.open();await settle();f.replies.push(Promise.resolve(response));
    await opening.change(ready(opening), true);assert.equal(opening.getSnapshot().kind, 'unresolved');assert.equal(f.calls.length, 2);
  }
});

test('self, Owner, contradictory linked facts, invalid ID/revision and retired parent make zero requests', () => {
  const f = fixture();
  for (const person of [{...target, accountId: 'owner-1'}, {...target, role: 'owner' as const}, {...target, hostedAccountId: 'linked'},
    {...target, accountId: 'member\n'}, {...target, accountId: '../member'}, {...target, revision: 0}]) {
    assert.equal(f.source.open(person, {signal: f.caller.signal, isCurrent: () => true}), undefined);
  }
  f.retire();assert.equal(f.open(), undefined);assert.equal(f.calls.length, 0);
});

test('caller retirement during Make clears the port and cannot borrow or block a later explicit opening after settlement', async () => {
  const f = fixture(), held = deferred<unknown>();f.replies.push(held.promise);const opening = f.open();
  f.caller.abort();assert.equal(opening.getSnapshot().kind, 'retired');
  const nextCaller = new AbortController();assert.equal(f.source.open(target, {signal: nextCaller.signal, isCurrent: () => true}), undefined);
  held.resolve(original);await settle();assert.equal(opening.getSnapshot().kind, 'retired');
  const successor = f.source.open(target, {signal: nextCaller.signal, isCurrent: () => true})!;await settle();
  assert.notEqual(successor, opening);assert.equal(ready(successor).result.code, codeA);assert.equal(f.calls.length, 2);
});

test('original authority retirement during replacement suppresses the reply and every retained action', async () => {
  const f = fixture(), opening = f.open();await settle();const shown = ready(opening), held = deferred<unknown>();f.replies.push(held.promise);
  const change = opening.change(shown, true);f.retire();held.resolve(replacement);await change;
  assert.equal(opening.current(), false);assert.equal(opening.getSnapshot().kind, 'pending');
  await opening.copy(shown);await opening.change(shown, false);assert.equal(f.calls.length, 2);assert.deepEqual(f.copies, []);
});

test('expired code cannot be copied or used as a replacement proof; no timer extends it', async () => {
  const f = fixture(), opening = f.open();await settle();const shown = ready(opening);f.expire();
  await opening.copy(shown);assert.equal(opening.getSnapshot().kind, 'unresolved');await opening.change(shown, true);
  assert.equal(f.calls.length, 1);assert.deepEqual(f.copies, []);
});

test('Copy uses only the exact currently published code and old callbacks cannot copy a replacement', async () => {
  const f = fixture(), opening = f.open();await settle();const shown = ready(opening);await opening.copy(shown);
  f.replies.push(Promise.resolve(replacement));await opening.change(shown, true);await opening.copy(shown);await opening.copy(ready(opening));
  assert.deepEqual(f.copies, [codeA, codeB]);
});

test('an unstated choice and retired signal make no replacement request', async () => {
  const f = fixture(), opening = f.open();await settle();const shown = ready(opening);
  await opening.change(shown, undefined as never);f.caller.abort();await opening.change(shown, true);assert.equal(f.calls.length, 1);
});
