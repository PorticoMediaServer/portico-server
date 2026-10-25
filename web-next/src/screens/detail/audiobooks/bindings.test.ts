import test from 'node:test';
import assert from 'node:assert/strict';
import {createAudiobookBindings} from './bindings.ts';
import type {ScreenContext} from '../context.ts';
const target = {libraryId: 'audio', view: 'book' as const, entityId: 'book'};
function harness(kind = 'audiobook') {
  let active = true, calls = 0;
  const starts: unknown[] = [];
  const raw = {scope: {serverId: 'server', libraryId: 'audio', libraryKind: kind, view: 'book', entityId: 'book', viewerFence: 'fence'},
    revision: {catalog: 1, viewer: 1}, query: {sort: '', direction: '', category: '', q: '', limit: 40},
    entity: {id: 'book', libraryId: 'audio', kind: 'book', title: 'Book'}, heading: {key: 'entity.title', fallback: 'Book'},
    listening: {target: {libraryId: 'audio', kind: 'book', id: 'book'}, actions: ['play'], resumeUnavailable: false, resume: {itemId: 'part', positionMs: 12000}},
    sections: [{id: 'files', type: 'list', heading: {key: 'files', fallback: 'Parts'}, entries: [{id: 'part', libraryId: 'audio', kind: 'audiobook_file', title: 'Part', available: true, playback: {itemId: 'part', startMs: 0}}], totalCount: 1, nextCursor: ''}]};
  const context = {api: {request: async () => {calls++;return raw;}}, scope: {serverId: 'server', viewerId: 'viewer'}, isCurrent: () => active,
    effects: {play: async (value: unknown) => {starts.push(value);}, openEntry: () => {}}} as unknown as ScreenContext;
  const binding = createAudiobookBindings(context, target);
  return {binding, raw, starts, calls: () => calls, retire: () => {active = false;}};
}
test('constructor is inert and a non-audiobook library cannot authorize the page', async () => {
  const h = harness('book');assert.equal(h.calls(), 0);
  await assert.rejects(h.binding.load(), {name: 'AbortError'});assert.deepEqual(h.starts, []);h.binding.dispose();
});
test('resume uses the producer part-relative time, not the whole book position', async () => {
  const h = harness();await h.binding.load();const page = h.binding.service.getSnapshot().projection!;
  await h.binding.primary(page);assert.deepEqual(h.starts, [{libraryId: 'audio', itemId: 'part', startSeconds: 12}]);h.binding.dispose();
});
test('a forged row or retired authority cannot start playback', async () => {
  const h = harness();await h.binding.load();const page = h.binding.service.getSnapshot().projection!, row = h.binding.service.getSnapshot().appended[0].entries[0];
  await assert.rejects(h.binding.play(page, {...row}), {name: 'AbortError'});
  h.retire();await assert.rejects(h.binding.primary(page), {name: 'AbortError'});assert.deepEqual(h.starts, []);h.binding.dispose();
});
test('a superseded projection cannot authorize a command', async () => {
  const h = harness();await h.binding.load();const page = h.binding.service.getSnapshot().projection!;
  await h.binding.load();await assert.rejects(h.binding.primary(page), {name: 'AbortError'});h.binding.dispose();
});
