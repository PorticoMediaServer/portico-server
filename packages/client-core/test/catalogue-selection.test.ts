import test from 'node:test';
import assert from 'node:assert/strict';
import {catalogueSelectionSegments, createCataloguePersonalSelection, playCatalogueSelection} from '../src/catalogue-selection.ts';
import {selectionCapabilities, type SelectionItem, type SelectionKind} from '../src/selection-model/index.ts';
import {mediaSelectionKey} from '../src/media-selection.ts';
import type {ContentEntry} from '../src/library-content.ts';
import type {DetailApi} from '../src/detail.ts';
import type {QueueHeader} from '../src/playback-v1/types.ts';

const scope = {serverId: 'server', viewerId: 'viewer'};
const entry: ContentEntry = {id: 'movie', libraryId: 'movies', kind: 'movie', title: 'Movie'};
const state = (revision = 0) => ({watchlisted: false, favorite: revision > 0, rating: null, revision});
const detail = () => ({scope: {...scope, libraryId: 'movies', itemId: 'movie', viewerFence: 'detail-only'}, revision: {catalog: 3, viewer: 0}, item: {...entry, duration: 10, progressSeconds: 0, available: true}, personal: state(), actions: [{id: 'favorite', labelKey: 'favorite', enabled: true}], metadata: {status: 'unavailable', ratings: [], genres: [], credits: []}});
function selected(kind: Exclude<SelectionKind, 'person' | 'channel'>, id = kind): SelectionItem {
  return {kind, id, libraryId: 'library', capabilities: selectionCapabilities(kind, {play: true, queue: true})};
}
const queue: QueueHeader = {id: 'queue', revision: '1', total: 3, repeat: 'off', segments: []};

test('explicit selected identities keep supplied order and aggregate server selectors', () => {
  assert.deepEqual(catalogueSelectionSegments([selected('movie', 'one'), selected('episode', 'two'), selected('show', 'show'), selected('album', 'album'), selected('song', 'three'), selected('collection', 'collection'), selected('playlist', 'playlist')]), [
    {source: {items: {ids: ['one', 'two']}}}, {source: {container: {kind: 'show', id: 'show'}}}, {source: {container: {kind: 'album', id: 'album'}}}, {source: {items: {ids: ['three']}}}, {source: {container: {kind: 'collection', id: 'collection'}}}, {source: {container: {kind: 'playlist', id: 'playlist'}}},
  ]);
  const many = Array.from({length: 501}, (_, i) => selected('movie', String(i)));
  assert.deepEqual(catalogueSelectionSegments(many).map(segment => 'items' in segment.source ? segment.source.items.ids.length : 0), [500, 1]);
});

test('capabilities and unsupported semantic kinds reject before any queue replacement', async () => {
  const unsupported: SelectionItem[] = [selected('author'), selected('disc'), selected('chapter'), {kind: 'person', id: 'person', capabilities: selectionCapabilities('person', {play: true})}, {kind: 'channel', sourceId: 'actual-source', channelId: 'actual-channel', capabilities: selectionCapabilities('channel', {play: true})}, {...selected('movie'), capabilities: selectionCapabilities('movie', {})}];
  let called = 0;
  for (const item of unsupported) await assert.rejects(playCatalogueSelection({playSegments: async () => {called++; return queue;}}, [item], {signal: new AbortController().signal, isCurrent: () => true}), {code: 'catalogue-selection-unavailable'});
  assert.equal(called, 0);
  assert.throws(() => catalogueSelectionSegments([selected('movie'), selected('movie')]), {code: 'catalogue-selection-unavailable'});
  assert.throws(() => catalogueSelectionSegments([{...selected('movie'), capabilities: selectionCapabilities('movie', {play: true})}], 'queue'), {code: 'catalogue-selection-unavailable'});
});

test('segments detach selected identities and cannot be edited after capture', () => {
  const input = {kind: 'movie' as const, id: 'original', libraryId: 'movies', capabilities: selectionCapabilities('movie', {play: true})};
  const result = catalogueSelectionSegments([input]);
  input.id = 'next-viewer';
  assert.deepEqual(result, [{source: {items: {ids: ['original']}}}]);
  assert(Object.isFrozen(result));
  assert(Object.isFrozen(result[0].source));
  assert('items' in result[0].source && Object.isFrozen(result[0].source.items.ids));
});

test('real player receives original lifetime; source retirement rejects late receipt', async () => {
  let current = true;
  const controller = new AbortController();
  await assert.rejects(playCatalogueSelection({playSegments: async (segments, lifetime) => {
    assert.equal(lifetime?.signal, controller.signal);
    assert.equal(lifetime?.isCurrent?.(), true);
    assert.deepEqual(segments, [{source: {container: {kind: 'book', id: 'book'}}}]);
    current = false;
    return queue;
  }}, [selected('book')], {signal: controller.signal, isCurrent: () => current}), {name: 'AbortError'});
});

test('personal binding is inert, retains captured transport and preserves retry operation identity', async () => {
  let requests = 0, fail = true;
  const operations: string[] = [];
  const api: DetailApi = {request: async<T>(_path: string, method = 'GET', body?: unknown): Promise<T> => {
    requests++;
    if (method === 'GET') return detail() as T;
    const operation = body as {operationId: string};
    operations.push(operation.operationId);
    if (fail) {fail = false; throw new TypeError('Connection lost');}
    return {operationId: operation.operationId, serverId: 'server', libraryId: 'movies', itemId: 'movie', viewerFence: 'detail-only', personal: state(1)} as T;
  }};
  const options = {api, scope, requestId: async () => crypto.randomUUID(), signal: new AbortController().signal, isCurrent: () => true};
  const binding = createCataloguePersonalSelection(options);
  assert.equal(requests, 0);
  options.api = {request: async () => {throw new Error('Next viewer transport must not be used');}};
  binding.service.setEntries([entry]);
  binding.service.toggle(mediaSelectionKey(entry));
  await binding.service.apply('favorite', true);
  assert.equal(binding.service.getSnapshot().results[0].status, 'failed');
  await binding.service.retry();
  assert.equal(binding.service.getSnapshot().results[0].status, 'saved');
  assert.equal(operations.length, 2);
  assert.equal(operations[0], operations[1]);
  binding.dispose();
});

test('source retirement or original abort during Detail read prevents personal write', async () => {
  for (const abort of [false, true]) {
    let current = true, writes = 0;
    const controller = new AbortController();
    const api: DetailApi = {request: async<T>(_path: string, method = 'GET'): Promise<T> => {
      if (method !== 'GET') {writes++; throw new Error('Retired write');}
      if (abort) controller.abort(); else current = false;
      return detail() as T;
    }};
    const binding = createCataloguePersonalSelection({api, scope, requestId: async () => crypto.randomUUID(), signal: controller.signal, isCurrent: () => current});
    binding.service.setEntries([entry]);binding.service.selectLoaded();
    await binding.service.apply('favorite', true);
    assert.equal(writes, 0);
    binding.dispose();
  }
});

test('aggregate parent personal intent stays unavailable rather than mutating its first child', async () => {
  let requests = 0;
  const binding = createCataloguePersonalSelection({api: {request: async () => {requests++; throw new Error('unused');}}, scope, requestId: async () => crypto.randomUUID(), signal: new AbortController().signal, isCurrent: () => true});
  binding.service.setEntries([{id: 'show', kind: 'show', libraryId: 'tv', title: 'Show', playback: {itemId: 'first-episode'}}]);binding.service.selectLoaded();
  assert.equal(binding.service.getSnapshot().canApply, false);
  await binding.service.apply('favorite', true);
  assert.equal(requests, 0);
  binding.dispose();
});
