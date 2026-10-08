/**
 * PERF-S09 whole-set select-all: with the screen's query/container target
 * registered and nothing deselected, one bulk action is exactly one fenced
 * job; a deselected entry, a missing target or a moved set (`selection_changed`,
 * including an unresolvable revision) falls back to the chunked items path.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {decideBulkTarget, runBulkSelection, runWholeSetJob} from '../src/screens/shared/bulk-job.ts';
import type {SelectionTarget} from '../src/app/selection.tsx';

type Script = {polls: string[]; done?: number; failed?: number; errorCode?: string; failures?: {itemId: string; code: string}[]};

function scriptApi(scripts: Script[]): {request<T>(path: string, method?: string, body?: unknown): Promise<T>; calls: {path: string; method?: string; body?: unknown}[]} {
  const calls: {path: string; method?: string; body?: unknown}[] = [];
  const jobs: Record<string, {script: Script; polls: number}> = {};
  let n = 0;
  return {
    calls,
    async request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
      calls.push({path, method, body});
      if (method === 'POST' && path === '/v1/jobs') {
        const id = 'job-' + (++n);
        jobs[id] = {script: scripts[n - 1] ?? {polls: ['running', 'complete']}, polls: 0};
        return {jobId: id, command: (body as {command: string}).command, state: 'running', total: 0, totalKnown: false, done: 0, failed: 0} as T;
      }
      const poll = path.match(/^\/v1\/jobs\/([^/]+)$/);
      if (poll && method === 'GET') {
        const job = jobs[poll[1]]!;
        const state = job.script.polls[Math.min(job.polls++, job.script.polls.length - 1)];
        const terminal = state === 'complete' || state === 'partial' || state === 'failed';
        return {jobId: poll[1], command: 'personal-state', state, total: terminal ? 1240 : 0, totalKnown: terminal, done: terminal ? (job.script.done ?? 1240) : 0, failed: terminal ? (job.script.failed ?? 0) : 0, ...(terminal && job.script.errorCode ? {errorCode: job.script.errorCode} : {})} as T;
      }
      const failures = path.match(/^\/v1\/jobs\/([^/]+)\/failures/);
      if (failures && method === 'GET') return {items: jobs[failures[1]]!.script.failures ?? [], nextCursor: ''} as T;
      throw new Error('unexpected ' + method + ' ' + path);
    },
  };
}

const queryTarget = (over: Partial<SelectionTarget> = {}): SelectionTarget => ({
  selector: {query: {libraryId: 'lib-1', pivot: 'movies', filter: {all: [{field: 'year', operator: 'at-least', value: 2000}]}, sort: [{field: 'title', direction: 'asc'}]}},
  total: 1240,
  catalogRevision: 'rev-1',
  ...over,
});

test('the registered target decides whole-set only when nothing is deselected', () => {
  const target = queryTarget();
  assert.deepEqual(decideBulkTarget(target, ['a', 'b'], new Set(['a', 'b']), true), {kind: 'whole-set', target});
  // Every visible item chosen by hand (or Cmd/Ctrl+A) is exactly those items, never the whole set.
  assert.deepEqual(decideBulkTarget(target, ['a', 'b'], new Set(['a', 'b'])), {kind: 'items'});
  assert.deepEqual(decideBulkTarget(target, ['a', 'b'], new Set(['a'])), {kind: 'items'});
  assert.deepEqual(decideBulkTarget(target, ['a', 'b'], new Set(['a', 'b', 'c'])), {kind: 'items'});
  assert.deepEqual(decideBulkTarget(target, [], new Set()), {kind: 'items'});
  assert.deepEqual(decideBulkTarget(null, ['a'], new Set(['a'])), {kind: 'items'});
  assert.deepEqual(decideBulkTarget(undefined, ['a'], new Set(['a'])), {kind: 'items'});
});

test('select-all with the target submits exactly one fenced job', async () => {
  const api = scriptApi([{polls: ['running', 'complete']}]);
  const seen: string[] = [];
  const target = queryTarget();
  const outcome = await runBulkSelection(api, 'personal-state', {watched: true}, {visibleIds: ['a', 'b'], selectedIds: new Set(['a', 'b']), target, wholeSet: true}, () => 'op-whole', p => seen.push(p.phase + ':' + String(p.totalKnown)));
  assert.deepEqual(outcome.ok, 1240);
  assert.deepEqual(outcome.jobs, 1);
  const posts = api.calls.filter(c => c.method === 'POST');
  assert.equal(posts.length, 1);
  assert.deepEqual(posts[0]!.body, {operationId: 'op-whole', command: 'personal-state', selector: target.selector, args: {watched: true}, expected: {catalogRevision: 'rev-1'}});
  assert.ok(seen.includes('preparing:false'), 'indeterminate while capturing: ' + seen.join(','));
  assert.ok(seen.includes('working:true'), 'determinate once totalKnown: ' + seen.join(','));
});

test('a known revision never calls the lazy resolver; otherwise it resolves once', async () => {
  let resolved = 0;
  const known = queryTarget({resolveRevision: async () => { resolved++; return 'rev-x'; }});
  await runBulkSelection(scriptApi([{polls: ['complete']}]), 'personal-state', {watched: true}, {visibleIds: ['a'], selectedIds: new Set(['a']), target: known, wholeSet: true}, () => 'op-1');
  assert.equal(resolved, 0);
  const lazy = queryTarget({catalogRevision: undefined, resolveRevision: async () => { resolved++; return 'rev-9'; }});
  const api = scriptApi([{polls: ['complete']}]);
  await runBulkSelection(api, 'personal-state', {watched: true}, {visibleIds: ['a'], selectedIds: new Set(['a']), target: lazy, wholeSet: true}, () => 'op-2');
  assert.equal(resolved, 1);
  assert.deepEqual((api.calls.find(c => c.method === 'POST')!.body as {expected: unknown}).expected, {catalogRevision: 'rev-9'});
});

test('selection_changed falls back to the chunked items with fresh operation ids', async () => {
  const api = scriptApi([
    {polls: ['running', 'failed'], errorCode: 'selection_changed'},
    {polls: ['running', 'complete'], done: 2},
  ]);
  let n = 0;
  const target = queryTarget();
  const outcome = await runBulkSelection(api, 'personal-state', {watched: true}, {visibleIds: ['a', 'b'], selectedIds: new Set(['a', 'b']), target, wholeSet: true}, () => 'op-' + (++n));
  assert.deepEqual(outcome.ok, 2);
  assert.deepEqual(outcome.jobs, 1);
  const posts = api.calls.filter(c => c.method === 'POST');
  assert.equal(posts.length, 2);
  assert.deepEqual((posts[0]!.body as {selector: unknown}).selector, target.selector);
  assert.deepEqual((posts[1]!.body as {selector: unknown}).selector, {items: {ids: ['a', 'b']}});
  assert.notEqual((posts[0]!.body as {operationId: string}).operationId, (posts[1]!.body as {operationId: string}).operationId);
});

test('an unresolvable revision falls back without ever posting the whole set', async () => {
  const api = scriptApi([{polls: ['complete'], done: 1}]);
  const target = queryTarget({catalogRevision: undefined, resolveRevision: async () => undefined});
  const outcome = await runBulkSelection(api, 'trash', {}, {visibleIds: ['a'], selectedIds: new Set(['a']), target, wholeSet: true}, () => 'op-1');
  assert.deepEqual(outcome.ok, 1);
  const posts = api.calls.filter(c => c.method === 'POST');
  assert.equal(posts.length, 1);
  assert.deepEqual((posts[0]!.body as {selector: unknown}).selector, {items: {ids: ['a']}});
});

test('without a target the explicit items path is unchanged', async () => {
  const api = scriptApi([{polls: ['complete'], done: 2}]);
  const outcome = await runBulkSelection(api, 'personal-state', {favorite: true}, {visibleIds: ['a', 'b'], selectedIds: new Set(['a', 'b']), target: null}, () => 'op-1');
  assert.deepEqual(outcome.ok, 2);
  const posts = api.calls.filter(c => c.method === 'POST');
  assert.equal(posts.length, 1);
  assert.deepEqual((posts[0]!.body as {selector: unknown}).selector, {items: {ids: ['a', 'b']}});
});

test('a non-selection_changed whole-set failure still throws', async () => {
  const api = scriptApi([{polls: ['failed'], errorCode: 'authority_revoked'}]);
  await assert.rejects(
    runWholeSetJob(api, 'personal-state', {watched: true}, queryTarget(), 'op-1'),
    (e: unknown) => (e as {code?: string}).code === 'authority_revoked',
  );
  assert.equal(api.calls.filter(c => c.method === 'POST').length, 1);
});
