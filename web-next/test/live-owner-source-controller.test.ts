import test from 'node:test';
import assert from 'node:assert/strict';
import {LiveSourceController, type OwnerLiveSource} from '../src/screens/live/owner/source-controller.ts';
const source: OwnerLiveSource = {id: 'source/a', name: 'Stored source', revision: 7, state: 'ready', tunerCount: 2, tunerCountMode: 'configured', generation: 'g', publishedAt: '', channels: 12, programmes: 20};
const document = {serverId: 'server', sources: [source], refresh: []};
test('owner actions use displayed revision and encoded source, then read back', async () => {
  const calls: unknown[][] = [];
  const controller = new LiveSourceController({async request<T>(...args: unknown[]) {calls.push(args);return document as T;}}, 'server', () => true, () => {}, () => 'operation-id');
  await controller.load();assert.equal(await controller.act(source, 'toggle'), true);
  assert.deepEqual(calls[1]?.slice(0, 3), ['/v1/admin/live-sources/source%2Fa/enabled', 'POST', {expectedRevision: 7, requestId: 'operation-id', enabled: false}]);
  assert.equal(calls.length, 3);controller.dispose();
});
test('retired identity discards a late list and prevents writes', async () => {
  let resolve!: (value: unknown) => void;let current = true;let calls = 0;
  const controller = new LiveSourceController({request<T>() {calls++;return new Promise(done => {resolve = done;}) as Promise<T>;}}, 'server', () => current, () => {});
  const pending = controller.load();current = false;resolve(document);await pending;
  assert.equal(controller.getSnapshot().document, undefined);assert.equal(await controller.act(source, 'remove'), false);assert.equal(calls, 1);controller.dispose();
});
test('stale displayed rows cannot remove a replacement source revision', async () => {
  let calls = 0;
  const controller = new LiveSourceController({async request<T>() {calls++;return {serverId: 'server', sources: [{...source, revision: 8}], refresh: []} as T;}}, 'server', () => true, () => {});
  await controller.load();assert.equal(await controller.act(source, 'remove'), false);assert.equal(calls, 1);controller.dispose();
});
test('conflicting removal refreshes facts without replaying the command', async () => {
  let reads = 0, writes = 0;
  const conflict = {status: 409};
  const controller = new LiveSourceController({async request<T>(_path: string, method?: string) {if (method === 'POST') {writes++;throw conflict;}reads++;return document as T;}}, 'server', () => true, () => {});
  await controller.load();assert.equal(await controller.act(source, 'remove'), false);assert.equal(writes, 1);assert.equal(reads, 2);assert.equal(controller.getSnapshot().error, conflict);controller.dispose();
});
test('authorization failure retires the controller and reports the boundary', async () => {
  const forbidden = {status: 403};let reported: unknown;
  const controller = new LiveSourceController({async request<T>(): Promise<T> {throw forbidden;}}, 'server', () => true, error => {reported = error;});
  await controller.load();assert.equal(reported, forbidden);assert.equal(await controller.act(source, 'refresh'), false);
});

test('transport retry retains the logical write id', async () => {
  const ids: unknown[] = [];let attempt = 0, serial = 0;
  const controller = new LiveSourceController({async request<T>(_path: string, method?: string, body?: unknown) {
    if (method === 'POST') {ids.push((body as {requestId: string}).requestId);if (++attempt === 1) throw {status: 503};}
    return document as T;
  }}, 'server', () => true, () => {}, () => String(++serial));
  await controller.load();await controller.act(source, 'remove');await controller.act(source, 'remove');
  assert.deepEqual(ids, ['1', '1']);controller.dispose();
});
test('a response for another server cannot populate the page', async () => {
  const controller = new LiveSourceController({async request<T>() {return {...document, serverId: 'other'} as T;}}, 'server', () => true, () => {});
  await controller.load();assert.equal(controller.getSnapshot().document, undefined);assert.ok(controller.getSnapshot().error);controller.dispose();
});
