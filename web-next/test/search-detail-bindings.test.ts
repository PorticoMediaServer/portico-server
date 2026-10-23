import test from 'node:test';
import assert from 'node:assert/strict';
import type {ScreenContext} from '../src/screens/detail/context.ts';
import {createDetailBindings} from '../src/screens/detail/bindings.ts';
import {createShowBindings} from '../src/screens/detail/show-bindings.ts';
import {createPersonBindings} from '../src/screens/detail/person-bindings.ts';
import {createSearchBindings} from '../src/screens/search/bindings.ts';

function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(done => { resolve = done; }); return {promise, resolve}; }
function setup(handler: (path: string, method?: string, body?: unknown, signal?: AbortSignal) => Promise<unknown>) {
  let current = true, requests = 0;
  const context = {
    api: {request: <T>(path: string, method?: string, body?: unknown, signal?: AbortSignal) => { requests++; return handler(path, method, body, signal) as Promise<T>; }},
    scope: {serverId: 'server', viewerId: JSON.stringify(['local', 'account', 'profile'])}, isCurrent: () => current,
    preferences: {getSnapshot: () => ({}), subscribe: () => () => {}},
    effects: {openEntry: () => {}, openSettings: () => {}, play: async () => {}, reportError: () => {}},
  } as unknown as ScreenContext;
  return {context, requests: () => requests, retire: () => { current = false; }};
}
const personPage = (id: string) => ({serverId: 'server', viewerFence: 'fence', revision: {catalog: 1, viewer: 1}, person: {id, name: id, revision: 1}, credits: [], pageInfo: {total: 0, nextCursor: ''}});
const detailPage = (id: string) => ({scope: {serverId: 'server', libraryId: 'library', itemId: id, viewerFence: 'fence'}, revision: {catalog: 1, viewer: 1},
  item: {id, libraryId: 'library', kind: 'movie', title: id, durationMs: 1000, progressMs: 0, available: true},
  personal: {watchlisted: false, favorite: false, rating: null, revision: 1, watched: false, progressMs: 0, lastPlayedAt: '', status: 'current', conflicts: []},
  actions: [], metadata: {status: 'unavailable', ratings: [], genres: [], credits: []}});

test('domain factory creation is inert and disposal erases all private snapshots', () => {
  const owner = setup(async () => { throw new Error('No constructor request is authorized'); });
  const detail = createDetailBindings(owner.context), show = createShowBindings(owner.context), person = createPersonBindings(owner.context), search = createSearchBindings(owner.context);
  assert.equal(owner.requests(), 0);
  detail.dispose(); show.dispose(); person.dispose(); search.dispose();
  assert.equal(detail.service.getSnapshot().data, null);
  assert.equal(show.service.getSnapshot().data, null);
  assert.equal(show.personal.getSnapshot(), undefined);
  assert.equal(person.getSnapshot().page, undefined);
  assert.equal(search.service.getSnapshot().data, null);
  assert.deepEqual(search.session(), {text: '', group: undefined});
});

test('a bare item-library resolution aborts at disposal and never starts detail with a late reply', async () => {
  const response = deferred<unknown>(); let signal: AbortSignal | undefined;
  const owner = setup(async (_path, _method, _body, input) => { signal = input; return response.promise; });
  const binding = createDetailBindings(owner.context), read = binding.select({itemId: 'one'});
  assert.equal(owner.requests(), 1);
  binding.dispose(); assert.equal(signal?.aborted, true);
  response.resolve({id: 'one', libraryId: 'library', kind: 'movie', title: 'one', available: true, durationMs: 1000, progressMs: 0});
  await assert.rejects(read, {name: 'AbortError'});
  assert.equal(owner.requests(), 1);
  assert.equal(binding.service.getSnapshot().data, null);
});

test('a family or profile handover refuses even a newly invoked old service request', async () => {
  const owner = setup(async () => ({})), binding = createSearchBindings(owner.context);
  owner.retire();
  await assert.rejects(binding.select('one'), {name: 'AbortError'});
  await assert.rejects(binding.recent(), {name: 'AbortError'});
  assert.equal(owner.requests(), 0);
  binding.dispose();
});

test('switching people aborts the old read and ignores a transport that still returns it', async () => {
  const old = deferred<unknown>(); let oldSignal: AbortSignal | undefined;
  const owner = setup(async (path, _method, _body, signal) => {
    if (path.includes('/first?')) { oldSignal = signal; return old.promise; }
    return personPage('second');
  });
  const binding = createPersonBindings(owner.context), first = binding.select('first');
  await binding.select('second');
  assert.equal(oldSignal?.aborted, true);
  old.resolve(personPage('first')); await first;
  assert.equal(binding.getSnapshot().page?.person.id, 'second');
  binding.dispose();
  assert.equal(binding.getSnapshot().page, undefined);
});

test('person page identity must still be current before starting a credit collection', async () => {
  const owner = setup(async path => personPage(path.includes('/first?') ? 'first' : 'second'));
  const binding = createPersonBindings(owner.context);
  await binding.select('first'); const first = binding.getSnapshot().page!;
  await binding.select('second');
  assert.throws(() => binding.credits(first), /Person required/);
  assert.equal(owner.requests(), 2);
  binding.dispose();
});

test('aborting the previous caller cannot cancel a newly selected title in the same turn', async () => {
  const first = deferred<unknown>(), second = deferred<unknown>(); let secondSignal: AbortSignal | undefined;
  const owner = setup(async (path, _method, _body, signal) => {
    if (path.includes('/first/')) return first.promise;
    secondSignal = signal; return second.promise;
  });
  const binding = createDetailBindings(owner.context), oldCaller = new AbortController();
  const before = assert.rejects(binding.select({libraryId: 'library', itemId: 'first'}, oldCaller.signal), {name: 'AbortError'});
  const after = binding.select({libraryId: 'library', itemId: 'second'});
  oldCaller.abort();
  assert.equal(secondSignal?.aborted, false);
  second.resolve(detailPage('second')); await after;
  first.resolve(detailPage('first')); await before;
  assert.equal(binding.service.getSnapshot().data?.item.id, 'second');
  binding.dispose();
});

test('an owner panel permission is re-read before even preparing a delete command', async () => {
  let fileReads = 0; const writes: string[] = [];
  const owner = setup(async (path, method) => {
    if (method !== 'GET') { writes.push(path); throw new Error('Revoked permission must not reach a command'); }
    if (path.includes('/file-info')) return {scope: detailPage('one').scope, revision: {catalog: 1, viewer: 1}, kind: 'movie', files: [], manage: {delete: ++fileReads === 1, split: false, merge: false}};
    return detailPage('one');
  });
  const binding = createDetailBindings(owner.context); await binding.select({libraryId: 'library', itemId: 'one'});
  const data = binding.service.getSnapshot().data!;
  assert.ok(data);
  const panel = await binding.readFileInfo(data);
  await assert.rejects(binding.previewDelete(data, panel, 'asset'), {name: 'AbortError'});
  assert.equal(fileReads, 2); assert.deepEqual(writes, []);
  binding.dispose();
});
