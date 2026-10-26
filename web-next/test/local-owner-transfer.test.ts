import test from 'node:test';
import assert from 'node:assert/strict';
import type {ProfileAPI} from '@core/profile-management.ts';
import {LocalOwnerTransferSource} from '../src/screens/settings/local-owner-transfer.ts';

// Accepting-composition vectors, authored UNRUN. Fixtures carry actual People
// envelopes and revisions; the presenter is not an authority or a role receipt.
const limits = {accessTokens: false, maxProfiles: 2, allowRemote: false, conversion: 'none', maxStreams: 1, maxStreamsMode: 'limit'};
const row = (accountId: string, role = 'member') => ({accountId, username: accountId, role, revision: 3, limitsRevision: 1,
  disabled: false, status: 'active', hasMoveCode: false, allowedLibraries: [], libraryNames: [], maxStreams: 1, maxStreamsMode: 'limit',
  serverMaxStreams: null, effectiveMaxStreams: 1, profileCount: 1, profileNames: ['Main'], liveState: 'idle'});
function fixture() {
  let current = true, revision = 3, link = false, fail: unknown, complete = 0;
  let post: (() => Promise<void>) | undefined;
  const calls: {path: string; method?: string; body?: unknown}[] = [];
  const api: ProfileAPI = {request: async <T>(path: string, method?: string, body?: unknown): Promise<T> => {
    calls.push({path, method, body});
    if (path.includes('/members?')) return {items: [row('member'), row('admin', 'admin')], totalCount: 2, matchedCount: 2, nextCursor: '', limit: 40} as T;
    if (path === '/v1/admin/access/members/owner' || path === '/v1/admin/access/members/member') {
      const owner = path.endsWith('/owner');
      return {...row(owner ? 'owner' : 'member', owner ? 'owner' : 'member'), profileId: owner ? 'owner-main' : 'member-main',
        revision: owner ? 9 : revision, limits, ...(link && !owner ? {hostedAccountId: 'contradictory-link'} : {})} as T;
    }
    if (path === '/v1/direct/ownership') {if (fail) throw fail; await post?.(); return undefined as T;}
    throw new Error('Unexpected transfer request');
  }};
  const source = new LocalOwnerTransferSource(api, {accountId: 'owner', profileId: 'owner-main', isCurrent: () => current,
    onTransferred: () => {complete++; current = false;}});
  return {source, calls, get complete() {return complete;}, retire: () => {current = false;},
    revised: () => {revision++;}, linked: () => {link = true;}, fail: (error: unknown) => {fail = error;}, wait: (value: () => Promise<void>) => {post = value;}};
}
const caller = () => {
  const abort = new AbortController();
  return {signal: abort.signal, isCurrent: () => !abort.signal.aborted, retire: () => abort.abort()};
};
const input = {accountId: 'member', currentPassword: ' original password ', expectedRevision: 9};

test('one bounded original inventory uses the CALLER revision and never fabricates a role receipt', async () => {
  const f = fixture(), life = caller();
  try {
    const port = await f.source.open(life);
    assert.deepEqual(port.opening.members, [{accountId: 'member', username: 'member'}]);
    assert.equal(port.opening.ownerRevision, 9); assert.equal(f.calls.length, 2);
    await assert.rejects(port.accept({...input, expectedRevision: 3}, life));
    assert.equal(f.calls.length, 2);
    await port.accept(input, life);
    assert.deepEqual(f.calls.filter(value => value.path === '/v1/direct/ownership'), [{path: '/v1/direct/ownership', method: 'POST', body: input}]);
    assert.equal(f.complete, 1);
    const count = f.calls.length; await assert.rejects(f.source.open(caller())); assert.equal(f.calls.length, count);
  } finally {f.source.dispose();}
});
test('Cancel A and reopen B never lets the retained A port act or close B', async () => {
  const f = fixture(), a = caller(), b = caller();
  try {
    const first = await f.source.open(a); first.close(); a.retire();
    const second = await f.source.open(b); first.close();
    const count = f.calls.length; await assert.rejects(first.accept(input, a)); assert.equal(f.calls.length, count);
    await second.accept(input, b); assert.equal(f.complete, 1);
  } finally {f.source.dispose();}
});
for (const mode of ['authority', 'caller', 'target-revision', 'contradictory-link'] as const) test('original transfer refuses '+mode+' without an accepting write', async () => {
  const f = fixture(), life = caller();
  try {
    const port = await f.source.open(life);
    if (mode === 'authority') f.retire(); if (mode === 'caller') life.retire();
    if (mode === 'target-revision') f.revised(); if (mode === 'contradictory-link') f.linked();
    await assert.rejects(port.accept(input, life));
    assert.equal(f.calls.some(value => value.path === '/v1/direct/ownership'), false); assert.equal(f.complete, 0);
  } finally {f.source.dispose();}
});
test('an uncertain accepting error keeps its identity and never retries or reconciles a guessed role', async () => {
  const f = fixture(), life = caller(), error = new Error('lost response'); f.fail(error);
  try {
    const port = await f.source.open(life);
    await assert.rejects(port.accept(input, life), value => value === error);
    assert.equal(f.calls.filter(value => value.path === '/v1/direct/ownership').length, 1); assert.equal(f.complete, 0);
  } finally {f.source.dispose();}
});
test('the synchronous opening claim admits only one transfer flight', async () => {
  const f = fixture(), life = caller(); let release!: () => void;
  f.wait(() => new Promise<void>(resolve => {release = resolve;}));
  try {
    const port = await f.source.open(life), first = port.accept(input, life);
    await assert.rejects(port.accept(input, life));
    // Drain only the fixture's microtasks; there is no test runtime or browser job here.
    for (let i = 0; i < 8 && !release; i++) await Promise.resolve();
    assert.equal(typeof release, 'function'); release(); await first;
    assert.equal(f.calls.filter(value => value.path === '/v1/direct/ownership').length, 1); assert.equal(f.complete, 1);
  } finally {f.source.dispose();}
});
