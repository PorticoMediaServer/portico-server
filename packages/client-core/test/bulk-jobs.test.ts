import test from 'node:test';
import assert from 'node:assert/strict';
import {
  bulkFailureCounts, createJob, getAllJobFailures, getJob, getJobFailures, groupMetadataTargets, isJobTerminal, itemsJobRequests, itemsSelector,
  jobRequestBody, parseJob, parseJobFailures, parseJobUpdatedEvent, parseJobUpdatedFrame, pollJob, runBulkJobs, runBulkRequests,
  IDEMPOTENCY_REUSED, REVISION_MISMATCH,
} from '../src/bulk-jobs.ts';

function job(over: Record<string, unknown> = {}): any {
  return {
    jobId: 'job-1', command: 'personal-state', state: 'running',
    total: 0, totalKnown: false, done: 0, failed: 0, ...over,
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

test('a bulk receipt starts indeterminate and parses terminal states', () => {
  const created = parseJob(job());
  assert.equal(created.totalKnown, false);
  assert.equal(isJobTerminal(created.state), false);
  for (const state of ['complete', 'partial', 'failed']) assert.equal(isJobTerminal(parseJob(job({state, total: 10, totalKnown: true, done: 9, failed: 1})).state), true);
  assert.equal(parseJob(job({state: 'failed', total: 4, totalKnown: true, done: 1, failed: 1, errorCode: 'selection_changed'})).errorCode, 'selection_changed');
});

test('a job rejects unknown states, mismatched counts and stray error codes', () => {
  for (const change of [
    (v: any) => { v.state = 'done'; },
    (v: any) => { v.totalKnown = true; v.total = 2; v.done = 3; },
    (v: any) => { v.errorCode = 'selection_changed'; },
    (v: any) => { v.errorCode = 'made_up'; v.state = 'failed'; v.totalKnown = true; v.total = 1; },
    (v: any) => { v.command = 'reindex'; },
  ]) {
    const raw = job({total: 0, totalKnown: false});
    change(raw);
    assert.throws(() => parseJob(raw));
  }
});

test('requests validate selectors, args and fences', () => {
  const base = {operationId: 'op-1', command: 'personal-state' as const, selector: itemsSelector(['a', 'b']), args: {watched: true}};
  assert.deepEqual(jobRequestBody(base).selector, {items: {ids: ['a', 'b']}});
  assert.throws(() => jobRequestBody({...base, operationId: 'has spaces!'}));
  assert.throws(() => jobRequestBody({...base, selector: {items: {ids: []}} as any}));
  assert.throws(() => jobRequestBody({...base, selector: {container: {kind: 'show', id: 's'}, items: {ids: ['a']}} as any}));
  assert.throws(() => jobRequestBody({...base, args: {}}));
  assert.throws(() => jobRequestBody({...base, args: {watched: true, playlistId: 'p'}}));
  assert.throws(() => jobRequestBody({...base, command: 'trash' as const, args: {watched: true}}));
  const query = jobRequestBody({
    operationId: 'op-2', command: 'personal-state', args: {favorite: true},
    selector: {query: {libraryId: 'lib', pivot: 'movies', filter: {all: []}, sort: [{field: 'title', direction: 'asc'}]}},
    expected: {catalogRevision: 'rev-9'},
  });
  assert.equal(query.expected?.catalogRevision, 'rev-9');
  const container = jobRequestBody({operationId: 'op-3', command: 'personal-state', args: {watched: false}, selector: {container: {kind: 'season', id: 's1'}}});
  assert.deepEqual(container.selector, {container: {kind: 'season', id: 's1'}});
  // playlist-add: placement next is kept verbatim (it means prepend); after is explicit.
  for (const placement of ['end', 'next', {after: 'entry-1'}]) {
    const placed = jobRequestBody({operationId: 'op-4', command: 'playlist-add', selector: itemsSelector(['a']), args: {playlistId: 'p', expectedRevision: 3, placement: placement as any}});
    assert.deepEqual(placed.args.placement, placement);
  }
  assert.throws(() => jobRequestBody({operationId: 'op-4', command: 'playlist-add', selector: itemsSelector(['a']), args: {playlistId: 'p', expectedRevision: 0, placement: 'end'}}));
  assert.throws(() => jobRequestBody({...base, args: {rating: 2.3}}));
  jobRequestBody({...base, args: {rating: 2.5}});
});

test('explicit selections chunk into one 200-item request each', () => {
  assert.throws(() => itemsSelector([]));
  assert.throws(() => itemsSelector(new Array(201).fill('x').map((_, i) => 'id-' + i)));
  const ids = new Array(450).fill(0).map((_, i) => 'id-' + i);
  const requests = itemsJobRequests(['op-a', 'op-b', 'op-c'], 'personal-state', {watched: true}, ids);
  assert.equal(requests.length, 3);
  assert.equal((requests[0].selector as {items: {ids: string[]}}).items.ids.length, 200);
  assert.equal((requests[2].selector as {items: {ids: string[]}}).items.ids.length, 50);
  assert.throws(() => itemsJobRequests(['only'], 'personal-state', {watched: true}, ids));
});

test('create is idempotent on the same operationId and maps the shared error codes', async () => {
  const api = fakeApi({'POST /v1/jobs': () => job({totalKnown: false})});
  const created = await createJob(api, {operationId: 'op-1', command: 'personal-state', selector: itemsSelector(['a']), args: {watched: true}});
  assert.equal(created.jobId, 'job-1');
  assert.deepEqual(api.calls[0].body, {operationId: 'op-1', command: 'personal-state', selector: {items: {ids: ['a']}}, args: {watched: true}});
  const reused = fakeApi({'POST /v1/jobs': () => { throw Object.assign(new Error('reused'), {status: 422, code: 'idempotency_key_reused'}); }});
  await assert.rejects(createJob(reused, {operationId: 'op-1', command: 'personal-state', selector: itemsSelector(['a']), args: {watched: true}}), (e: any) => e.code === IDEMPOTENCY_REUSED);
  assert.equal(IDEMPOTENCY_REUSED, 'idempotency_key_reused');
  assert.equal(REVISION_MISMATCH, 'revision_mismatch');
  // A bare 409 still means a stale revision, never a reused key.
  const conflict = fakeApi({'POST /v1/jobs': () => { throw Object.assign(new Error('stale'), {status: 409}); }});
  await assert.rejects(createJob(conflict, {operationId: 'op-1', command: 'personal-state', selector: itemsSelector(['a']), args: {watched: true}}), (e: any) => e.code === 'revision_mismatch');
});

test('polling ends at a terminal state and failures page through cursors', async () => {
  let polls = 0;
  const api = fakeApi({
    'GET /v1/jobs/job-1': () => (++polls < 3 ? job({state: 'running', total: 4, totalKnown: true, done: polls}) : job({state: 'partial', total: 4, totalKnown: true, done: 3, failed: 1})),
    'GET /v1/jobs/job-1/failures': () => ({items: [{itemId: 'a', code: 'trash_conflict'}], nextCursor: ''}),
  });
  const finished = await pollJob(api, 'job-1', {intervalMs: 250, timeoutMs: 10000});
  assert.equal(finished.state, 'partial');
  assert.equal((await getJobFailures(api, 'job-1')).items.length, 1);
  assert.throws(() => parseJobFailures({items: [{itemId: 'a', code: ''}]}));
});

test('failure pages collect across cursors and stop on a repeated cursor', async () => {
  const api = fakeApi({
    'GET /v1/jobs/job-1/failures': (body?: unknown) => {
      void body;
      return {items: [{itemId: 'a', code: 'trash_conflict'}], nextCursor: 'stuck'};
    },
  });
  // A cursor that never advances is a contract break, not an infinite loop.
  await assert.rejects(getAllJobFailures(api, 'job-1'));
});

test('job.updated frames parse progress and ignore other events', () => {
  const event = parseJobUpdatedFrame('job.updated', JSON.stringify(job({total: 5, totalKnown: true, done: 2})));
  assert.equal(event?.done, 2);
  assert.equal(parseJobUpdatedFrame('queue.updated', '{}'), null);
  assert.throws(() => parseJobUpdatedFrame('job.updated', 'not json'));
  assert.deepEqual(parseJobUpdatedEvent(job({command: 'trash'})).command, 'trash');
});

test('M25-4: runBulkRequests runs one job per request with preparing/working progress', async () => {
  const calls: {path: string; method?: string; body?: unknown}[] = [];
  const state: Record<string, {polls: number}> = {};
  let n = 0;
  const api = {
    calls,
    async request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
      calls.push({path, method, body});
      if (method === 'POST' && path === '/v1/jobs') {
        const id = 'job-' + (++n);
        state[id] = {polls: 0};
        return {jobId: id, command: (body as {command: string}).command, state: 'running', total: 0, totalKnown: false, done: 0, failed: 0} as T;
      }
      const poll = path.match(/^\/v1\/jobs\/([^/]+)$/);
      if (poll && method === 'GET') {
        const polls = state[poll[1]]!.polls++;
        const terminal = polls > 0;
        return {jobId: poll[1], command: 'metadata-edit', state: terminal ? 'complete' : 'running', total: terminal ? 2 : 0, totalKnown: terminal, done: terminal ? 2 : 0, failed: 0} as T;
      }
      const failures = path.match(/^\/v1\/jobs\/([^/]+)\/failures/);
      if (failures && method === 'GET') return {items: [], nextCursor: ''} as T;
      throw new Error('unexpected ' + method + ' ' + path);
    },
  };
  const seen: string[] = [];
  const outcome = await runBulkRequests(api, 'metadata-edit', {lockEdited: true}, [{selector: {items: {ids: ['a', 'b']}}, operationId: 'op-1', ids: ['a', 'b']}], p => seen.push(p.phase));
  assert.equal(outcome.ok, 2);
  assert.equal(outcome.jobs, 1);
  assert.ok(seen.includes('preparing') && seen.includes('working'), seen.join(','));
});

test('M25-4: runBulkRequests maps the failures page to per-target outcomes; a failed job throws', async () => {
  let n = 0;
  const api = {
    async request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
      if (method === 'POST') {
        n++;
        return {jobId: 'job-' + n, command: 'metadata-edit', state: 'running', total: 0, totalKnown: false, done: 0, failed: 0} as T;
      }
      if (path.endsWith('/failures')) return {items: [{itemId: 'b', code: 'metadata_conflict'}], nextCursor: ''} as T;
      return {jobId: 'job-1', command: 'metadata-edit', state: 'complete', total: 2, totalKnown: true, done: 1, failed: 1} as T;
    },
  };
  const outcome = await runBulkRequests(api, 'metadata-edit', {lockEdited: true}, [{selector: {items: {ids: ['a', 'b']}}, operationId: 'op-1', ids: ['a', 'b']}]);
  assert.equal(outcome.ok, 1);
  assert.deepEqual(outcome.failed, [{itemId: 'b', code: 'metadata_conflict'}]);
  assert.deepEqual(bulkFailureCounts(outcome.failed), {metadata_conflict: 1});
  const failing = {request: async <T>(): Promise<T> => ({jobId: 'job-9', command: 'trash', state: 'failed', total: 0, totalKnown: true, done: 0, failed: 0, errorCode: 'selection_changed'}) as T};
  await assert.rejects(runBulkRequests(failing, 'trash', {}, [{selector: {items: {ids: ['a']}}, operationId: 'op-1', ids: ['a']}]), (e: unknown) => (e as {code?: string}).code === 'selection_changed');
});

test('M25-4: runBulkJobs chunks explicit ids one job per 200', async () => {
  const posts: unknown[] = [];
  const api = {
    async request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
      if (method === 'POST') { posts.push(body); return {jobId: 'job-' + posts.length, command: 'personal-state', state: 'complete', total: 200, totalKnown: true, done: 200, failed: 0} as T; }
      if (path.endsWith('/failures')) return {items: [], nextCursor: ''} as T;
      return {jobId: 'job-1', command: 'personal-state', state: 'complete', total: 0, totalKnown: true, done: 0, failed: 0} as T;
    },
  };
  const ids = Array.from({length: 250}, (_, i) => 'id-' + i);
  const outcome = await runBulkJobs(api, 'personal-state', {watched: true}, ids, ['op-a', 'op-b']);
  assert.equal(outcome.jobs, 2);
  assert.equal(posts.length, 2);
});

test('M25-4: groupMetadataTargets chunks items at 200 with one container selector each', () => {
  let n = 0;
  const makeId = () => 'op-' + (++n);
  const items = Array.from({length: 201}, (_, i) => ({kind: 'item', id: 'id-' + i}));
  const groups = groupMetadataTargets(items, makeId);
  assert.equal(groups.length, 2);
  assert.deepEqual(groups[0]!.ids.length, 200);
  assert.deepEqual(groups[1]!.ids, ['id-200']);
  assert.ok(groups.every(g => g.operationId.startsWith('op-')));
  const mixed = groupMetadataTargets([{kind: 'item', id: 'a'}, {kind: 'show', id: 's'}, {kind: 'album', id: 'b'}], makeId);
  assert.equal(mixed.length, 3);
  assert.deepEqual(mixed[0]!.selector, {items: {ids: ['a']}});
  assert.deepEqual(mixed[1]!.selector, {container: {kind: 'show', id: 's'}});
  assert.deepEqual(mixed[2]!.selector, {container: {kind: 'album', id: 'b'}});
});
