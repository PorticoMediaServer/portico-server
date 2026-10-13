import test from 'node:test';
import assert from 'node:assert/strict';
import {createDownloadRequest, getDownloadRequest, isDownloadRequestTerminal, parseDownloadRequest, parseDownloadRequestList, pollDownloadRequest} from '../src/download-requests.ts';

const sha = 'b'.repeat(64);
function preparation(id: string): any {
  return {
    id: 'prep-' + id, itemId: id, libraryId: 'lib', profileId: 'profile', quality: 'original', origin: 'container',
    title: {kind: 'episode', title: 'Episode ' + id, onServer: true},
    batchId: 'batch', state: 'ready', reason: '',
    artifact: {kind: 'prepared', ref: 'version', sha256: sha, bytes: 4096, estimated: false, container: 'mp4', contentType: 'video/mp4', fileName: id + '.mp4'},
    progress: {bytesDone: 4096, bytesTotal: 4096, bytesTotalEstimated: false, percent: 100, etaSeconds: null},
    actions: ['remove'], revision: 1,
    createdAt: '2026-09-16T10:00:00Z', updatedAt: '2026-09-16T10:05:00Z', readyAt: '2026-09-16T10:05:00Z', expiresAt: '2026-10-16T10:05:00Z',
  };
}

function request(over: Record<string, unknown> = {}): any {
  return {
    requestId: 'req-1', total: 0, totalKnown: false, state: 'capturing',
    ready: 0, preparing: 0, queued: 0, failed: 0, paused: 0, items: [], nextCursor: '', ...over,
  };
}

function fakeApi(routes: Record<string, (body?: unknown) => unknown>): {request<T>(path: string, method?: string, body?: unknown): Promise<T>; calls: {path: string; method?: string; body?: unknown}[]} {
  const calls: {path: string; method?: string; body?: unknown}[] = [];
  return {
    calls,
    async request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
      calls.push({path, method, body});
      const route = routes[method + ' ' + path.split('?')[0]];
      if (!route) throw Object.assign(new Error('not found'), {status: 404, code: 'not_found'});
      return route(body) as T;
    },
  };
}

test('a download request starts capturing without a known total', () => {
  const parsed = parseDownloadRequest(request());
  assert.equal(parsed.totalKnown, false);
  assert.equal(isDownloadRequestTerminal(parsed.state), false);
  assert.equal(isDownloadRequestTerminal(parseDownloadRequest(request({state: 'complete', total: 3, totalKnown: true, ready: 3})).state), true);
});

test('requests validate the target, device, quality and policy', async () => {
  const api = fakeApi({'POST /v1/downloads/requests': () => request()});
  const created = await createDownloadRequest(api, {operationId: 'op-1', target: {kind: 'season', id: 's-1'}, deviceId: 'dev-1', quality: 'original', policy: {episodes: 'next', keepNext: 3}});
  assert.equal(created.requestId, 'req-1');
  assert.deepEqual(api.calls[0].body, {operationId: 'op-1', target: {kind: 'season', id: 's-1'}, deviceId: 'dev-1', quality: 'original', policy: {episodes: 'next', keepNext: 3}});
  await assert.rejects(createDownloadRequest(api, {operationId: 'op-1', target: {kind: 'show', id: 's'}, deviceId: 'dev-1', quality: 'original', policy: {episodes: 'all', keepNext: 2}}));
  await assert.rejects(createDownloadRequest(api, {operationId: 'bad id!', target: {kind: 'show', id: 's'}, deviceId: 'dev-1', quality: 'original', policy: {episodes: 'all'}}));
  await assert.rejects(createDownloadRequest(api, {operationId: 'op-1', target: {kind: 'episode' as any, id: 'e'}, deviceId: 'dev-1', quality: 'original', policy: {episodes: 'all'}}));
});

test('a reused operationId with a different body fails as idempotency_key_reused', async () => {
  const api = fakeApi({'POST /v1/downloads/requests': () => { throw Object.assign(new Error('reused'), {status: 422, code: 'idempotency_key_reused'}); }});
  await assert.rejects(
    createDownloadRequest(api, {operationId: 'op-1', target: {kind: 'show', id: 's'}, deviceId: 'dev-1', quality: '720p', policy: {episodes: 'unwatched'}}),
    (e: any) => e.code === 'idempotency_key_reused',
  );
});

test('reads page members and refuse another request id', async () => {
  const api = fakeApi({'GET /v1/downloads/requests/req-1': () => request({state: 'preparing', total: 2, totalKnown: true, ready: 1, queued: 1, items: [{itemId: 'e1', preparation: preparation('e1')}, {itemId: 'e2', reason: 'storage_full'}]})});
  const view = await getDownloadRequest(api, 'req-1', {limit: 100});
  assert.equal(view.items.length, 2);
  assert.equal(view.items[0].preparation?.itemId, 'e1');
  assert.equal(view.items[1].reason, 'storage_full');
  const wrong = fakeApi({'GET /v1/downloads/requests/req-1': () => request({requestId: 'req-2', state: 'preparing', total: 0, totalKnown: true})});
  await assert.rejects(getDownloadRequest(wrong, 'req-1'));
});

test('polling ends complete and reports a failed capture without effects', async () => {
  let polls = 0;
  const api = fakeApi({'GET /v1/downloads/requests/req-1': () => (++polls < 2 ? request({state: 'capturing'}) : request({state: 'complete', total: 1, totalKnown: true, ready: 1, items: [{itemId: 'e1', preparation: preparation('e1')}]}))});
  const done = await pollDownloadRequest(api, 'req-1', {intervalMs: 500, timeoutMs: 10000});
  assert.equal(done.state, 'complete');
  const failed = fakeApi({'GET /v1/downloads/requests/req-9': () => request({requestId: 'req-9', state: 'failed', total: 0, totalKnown: true, errorCode: 'authority_revoked'})});
  const gone = await pollDownloadRequest(failed, 'req-9', {intervalMs: 500});
  assert.equal(gone.errorCode, 'authority_revoked');
});

test('a device lists only its own requests, and duplicate rows are a contract break', () => {
  const summary = {requestId: 'req-1', state: 'preparing', target: {kind: 'season', id: 's-1'}, title: 'Season 1', quality: 'original', createdAt: '2026-09-16T10:00:00Z'};
  const listed = parseDownloadRequestList({requests: [summary]});
  assert.equal(listed[0].requestId, 'req-1');
  assert.equal(listed[0].target.kind, 'season');
  assert.equal(listed[0].title, 'Season 1');
  // The same request cannot appear twice in the list a device reconciles against.
  assert.throws(() => parseDownloadRequestList({requests: [summary, {...summary}]}));
  assert.throws(() => parseDownloadRequestList({requests: [{...summary, state: 'lost'}]}));
  assert.throws(() => parseDownloadRequestList({requests: [{...summary, target: {kind: 'episode', id: 'e-1'}}]}));
});

test('duplicate members and stray error codes are contract breaks', () => {
  assert.throws(() => parseDownloadRequest(request({items: [{itemId: 'a'}, {itemId: 'a'}]})));
  // An error code only rides a failed capture, and only the published ones exist.
  assert.throws(() => parseDownloadRequest(request({state: 'preparing', errorCode: 'authority_revoked'})));
  assert.throws(() => parseDownloadRequest(request({state: 'failed', total: 0, totalKnown: true, errorCode: 'selection_changed'})));
  assert.throws(() => parseDownloadRequest(request({state: 'failed', total: 0, totalKnown: true, errorCode: 'nope'})));
});
