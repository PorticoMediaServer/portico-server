import test from 'node:test';
import assert from 'node:assert/strict';
import {RECOMMENDATIONS_RESET, clearNotInterested, currentNotInterestedNotice, hiddenNotInterested, markNotInterested, startNotInterested, undoNotInterested, withoutHidden} from '../src/app/not-interested.ts';

test('P5: Not interested sends the title’s own id, and Undo takes it back', async () => {
  const bodies: any[] = [];
  const api = {request: async <T,>(_path: string, _method?: string, body?: any) => { bodies.push(body); return {serverId: 's1', viewerFence: 'f', operationId: body.operationId, updated: 1, failed: 0, results: [{itemId: body.items[0].itemId, ok: true, personal: {revision: bodies.length}}]} as T; }};
  const done = await markNotInterested(api, 's1', {id: 'card', kind: 'show', navigation: {entityId: 'show-1'}});
  assert.deepEqual(bodies[0].items, [{itemId: 'show-1', notInterested: true}]);
  await done.undo();
  assert.deepEqual(bodies[1].items, [{itemId: 'show-1', notInterested: false}]);
  assert.notEqual(bodies[0].operationId, bodies[1].operationId);
});

test('a refusal rejects, so the card comes back', async () => {
  const api = {request: async () => { throw Object.assign(new Error('nope'), {code: 'forbidden'}); }};
  await assert.rejects(markNotInterested(api as never, 's1', {id: 'm1', kind: 'movie'}));
});

test('the shared hidden set filters a recommendation row only for its viewer', async () => {
  const api = {request: async <T,>(_path: string, _method?: string, body?: any) => ({serverId: 'server', viewerFence: 'f', operationId: body.operationId, updated: 1, failed: 0, results: [{itemId: 'm1', ok: true, personal: {revision: 1}}]} as T)};
  startNotInterested(api, 'viewer-filter', 'server', {id: 'm1', title: 'M1'});
  const row = {id: 'recommended', entries: [{id: 'm1'}, {id: 'm2'}]};
  assert.deepEqual(withoutHidden(row, hiddenNotInterested('viewer-filter')).entries, [{id: 'm2'}]);
  assert.equal(withoutHidden(row, hiddenNotInterested('other-viewer')), row);
  clearNotInterested('viewer-filter');
});

test('mark, Undo, then late mark completion keeps the latest decision for this viewer and target', async () => {
  let finishMark!: (value: unknown) => void;
  const first = new Promise<unknown>(resolve => { finishMark = resolve; });
  const bodies: any[] = [];
  const api = {request: async <T,>(_path: string, _method?: string, body?: any) => {
    bodies.push(body);
    if (bodies.length === 1) return first as T;
    return {serverId: 'server', viewerFence: 'f', operationId: body.operationId, updated: 1, failed: 0, results: [{itemId: 'target', ok: true, personal: {revision: 2}}]} as T;
  }};
  const viewer = 'viewer-race';
  startNotInterested(api, viewer, 'server', {id: 'card', title: 'Title', kind: 'show', navigation: {entityId: 'target'}});
  assert.equal(hiddenNotInterested(viewer).has('target'), true);
  assert.deepEqual(withoutHidden({entries: [{id: 'another-card', kind: 'show', navigation: {entityId: 'target'}}, {id: 'other'}]}, hiddenNotInterested(viewer)).entries, [{id: 'other'}]);
  const pending = currentNotInterestedNotice(viewer)!;
  const undo = undoNotInterested(viewer, pending);
  assert.equal(hiddenNotInterested(viewer).has('target'), false);
  finishMark({serverId: 'server', viewerFence: 'f', operationId: bodies[0].operationId, updated: 1, failed: 0, results: [{itemId: 'target', ok: true, personal: {revision: 1}}]});
  await undo;
  assert.deepEqual(bodies.map(body => body.items), [[{itemId: 'target', notInterested: true}], [{itemId: 'target', notInterested: false}]]);
  assert.equal(hiddenNotInterested(viewer).has('target'), false);
  assert.equal(currentNotInterestedNotice(viewer), undefined, 'late mark cannot restore the old Undo notice');
  clearNotInterested(viewer);
});

test('reset clears only the selected viewer and late completions cannot hide it again', async () => {
  let finish!: (value: unknown) => void;
  const pending = new Promise<unknown>(resolve => { finish = resolve; });
  let calls = 0;
  let firstOperation = '';
  const api = {request: async <T,>(_path: string, _method?: string, body?: any) => {
    if (++calls === 1) { firstOperation = body.operationId; return pending as T; }
    return {serverId: 'server', viewerFence: 'f', operationId: body.operationId, updated: 1, failed: 0, results: [{itemId: 'card', ok: true, personal: {revision: 1}}]} as T;
  }};
  startNotInterested(api, 'viewer-reset', 'server', {id: 'card', title: 'Title'});
  startNotInterested(api, 'other-viewer', 'server', {id: 'card', title: 'Title'});
  clearNotInterested('viewer-reset');
  finish({serverId: 'server', viewerFence: 'f', operationId: firstOperation, updated: 1, failed: 0, results: [{itemId: 'card', ok: true, personal: {revision: 1}}]});
  await Promise.resolve();
  assert.equal(hiddenNotInterested('viewer-reset').has('card'), false);
  assert.equal(hiddenNotInterested('other-viewer').has('card'), true);
  clearNotInterested('other-viewer');
});

test('a successful local clear broadcasts its viewer for mounted recommendation readers', () => {
  const previous = globalThis.window;
  const events: string[] = [];
  try {
    (globalThis as any).window = new EventTarget();
    window.addEventListener(RECOMMENDATIONS_RESET, event => events.push((event as CustomEvent<{viewer: string}>).detail.viewer));
    clearNotInterested('viewer-one');
    assert.deepEqual(events, ['viewer-one']);
  } finally {
    (globalThis as any).window = previous;
  }
});
