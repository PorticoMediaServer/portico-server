import test from 'node:test';
import assert from 'node:assert/strict';
import {runBulkJobs, runBulkRequests, bulkFailureCounts} from '../src/screens/shared/bulk-job.ts';

function fakeApi(jobs: Record<string, {states: string[]; done?: number; failed?: number; failures?: {itemId: string; code: string}[]; errorCode?: string}>): {request<T>(path: string, method?: string, body?: unknown): Promise<T>; calls: {path: string; method?: string; body?: unknown}[]} {
  const calls: {path: string; method?: string; body?: unknown}[] = [];
  let n = 0;
  return {
    calls,
    async request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
      calls.push({path, method, body});
      if (method === 'POST' && path === '/v1/jobs') {
        const id = 'job-' + (++n);
        const key = (body as {operationId: string}).operationId;
        (jobs as Record<string, {key?: string; states: string[]; polls?: number}>)[id] = {key, states: ['running', 'complete'], polls: 0};
        return {jobId: id, command: (body as {command: string}).command, state: 'running', total: 0, totalKnown: false, done: 0, failed: 0} as T;
      }
      const poll = path.match(/^\/v1\/jobs\/([^/]+)$/);
      if (poll && method === 'GET') {
        const job = (jobs as Record<string, {states: string[]; polls: number; done?: number; failed?: number; failures?: {itemId: string; code: string}[]; errorCode?: string}>)[poll[1]];
        const state = job.states[Math.min(job.polls++, job.states.length - 1)];
        const terminal = state === 'complete' || state === 'partial' || state === 'failed';
        return {jobId: poll[1], command: 'personal-state', state, total: terminal ? 2 : 0, totalKnown: terminal, done: terminal ? (job.done ?? 2) : 0, failed: terminal ? (job.failed ?? 0) : 0, ...(terminal && job.errorCode ? {errorCode: job.errorCode} : {})} as T;
      }
      const failures = path.match(/^\/v1\/jobs\/([^/]+)\/failures/);
      if (failures && method === 'GET') {
        const job = (jobs as Record<string, {failures?: {itemId: string; code: string}[]}>)[failures[1]];
        return {items: job.failures ?? [], nextCursor: ''} as T;
      }
      throw new Error('unexpected ' + method + ' ' + path);
    },
  };
}

test('one bulk action is one job with progress from preparing to done', async () => {
  const api = fakeApi({});
  const seen: string[] = [];
  const outcome = await runBulkJobs(api, 'personal-state', {watched: true}, ['a', 'b'], ['op-1'], p => seen.push(p.phase));
  assert.equal(outcome.ok, 2);
  assert.equal(outcome.jobs, 1);
  assert.deepEqual(api.calls.filter(c => c.method === 'POST').length, 1);
  assert.ok(seen.includes('preparing'), 'indeterminate while capturing: ' + seen.join(','));
  assert.ok(seen.includes('working'), 'determinate once totalKnown: ' + seen.join(','));
  assert.deepEqual(api.calls[0].body, {operationId: 'op-1', command: 'personal-state', selector: {items: {ids: ['a', 'b']}}, args: {watched: true}});
});

test('selections over 200 items become one job each, and failures aggregate', async () => {
  const api = fakeApi({});
  const ids = Array.from({length: 250}, (_, i) => 'id-' + i);
  const outcome = await runBulkJobs(api, 'personal-state', {favorite: true}, ids, ['op-a', 'op-b']);
  assert.equal(outcome.jobs, 2);
  assert.equal(outcome.ok, 4);
});

test('a failed job throws its error code instead of counting silently', async () => {
  const failing = fakeApi({});
  (failing as {request: unknown}).request = async () => ({jobId: 'job-9', command: 'trash', state: 'failed', total: 0, totalKnown: true, done: 0, failed: 0, errorCode: 'selection_changed'});
  await assert.rejects(runBulkJobs(failing as never, 'trash', {}, ['a'], ['op-1']), (e: unknown) => (e as {code?: string}).code === 'selection_changed');
});

test('explicit container selectors run through the same progress contract', async () => {
  const api = fakeApi({});
  const outcome = await runBulkRequests(api, 'metadata-edit', {lockEdited: true}, [
    {selector: {container: {kind: 'show', id: 'show-1'}}, operationId: 'op-c', ids: ['show-1']},
  ]);
  assert.equal(outcome.jobs, 1);
  assert.equal(outcome.ok, 2);
  const posted = api.calls.find(c => c.method === 'POST');
  assert.deepEqual(posted?.body, {operationId: 'op-c', command: 'metadata-edit', selector: {container: {kind: 'show', id: 'show-1'}}, args: {lockEdited: true}});
});

test('failure codes count by code', () => {
  assert.deepEqual(bulkFailureCounts([{itemId: 'a', code: 'trash_conflict'}, {itemId: 'b', code: 'trash_conflict'}, {itemId: 'c', code: 'trash_not_allowed'}]), {trash_conflict: 2, trash_not_allowed: 1});
});
