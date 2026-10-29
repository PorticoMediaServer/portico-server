import test from 'node:test';
import assert from 'node:assert/strict';
import {FileInfoWindowError, FileInfoWindowSession, fileInfoRows, type FileInfoWindowFacts, type FileInfoWindowSessionOptions} from '../src/file-info-windows.ts';
import type {PreparationResponse} from '../src/read-preparation.ts';

type Read = Readonly<{itemId: string}>;
type Page = Readonly<{rows: readonly string[]; window: FileInfoWindowFacts}>;
const response = (status: number, body?: unknown, headers: Record<string, string> = {}): PreparationResponse => ({status, body, headers});
const page = (rows: string[], window: Partial<FileInfoWindowFacts> = {}) => ({rows, window: {queryId: 'q1', basis: 'b1', limit: 2, returned: rows.length, ...window}});
const preparing = (queryId = 'q1') => ({kind: 'file-preparing', state: 'preparing', queryId, retryAfterMs: 1000});
async function flush() { for (let i = 0; i < 40; i++) await Promise.resolve(); }

function fixture(replies: PreparationResponse[]) {
  const calls: {path: string; method: string}[] = [], timers: (() => void)[] = [];
  let time = 0;
  const options: FileInfoWindowSessionOptions<Read, Page> = {
    api: {requestRaw(path, method) {
      calls.push({path, method});
      if (method === 'DELETE') return Promise.resolve(response(204));
      assert.ok(replies.length, `unexpected request ${path}`);
      return Promise.resolve(replies.shift()!);
    }},
    path: (read, cursor) => `/v1/items/${read.itemId}/groups${cursor ? `?cursor=${cursor}` : ''}`,
    poll: id => `/v1/file-info-preparations/${id}`, cancel: id => `/v1/file-info-preparations/${id}`,
    parse: body => body as Page, window: value => value.window,
    parsePreparing: body => ({queryId: (body as {queryId: string}).queryId, retryAfterMs: (body as {retryAfterMs: number}).retryAfterMs}),
    readyQueryId: body => (body as Page).window.queryId,
    errorFromResponse: raw => new Error(`status ${raw.status}`),
    isCurrent: () => true,
    runtime: {now: () => time, random: () => 0, setTimeout: (callback, ms) => { timers.push(() => { time += ms; callback(); }); return timers.length; }, clearTimeout: () => {}},
  };
  return {session: new FileInfoWindowSession(options), calls, timers};
}

test('a window session reads one window at a time and keeps only the latest lease', async () => {
  const {session, calls} = fixture([response(200, page(['a', 'b'], {next: 'n1'})), response(200, page(['c'], {queryId: 'q2', previous: 'p2'}))]);
  const first = await session.open({itemId: 'i1'});
  assert.deepEqual(first.page.rows, ['a', 'b']);assert.equal(first.reopened, false);
  const second = await session.next();
  assert.deepEqual(second.page.rows, ['c']);
  assert.deepEqual(calls.map(call => `${call.method} ${call.path}`), ['GET /v1/items/i1/groups', 'GET /v1/items/i1/groups?cursor=n1', 'DELETE /v1/file-info-preparations/q1'],
    'the first window is released only after the server consumed its token');
  await assert.rejects(session.next(), (error: unknown) => error instanceof FileInfoWindowError && error.code === 'file_info_continuation_unavailable');
  session.retire();
  assert.equal(calls.at(-1)?.method, 'DELETE');assert.equal(calls.at(-1)?.path, '/v1/file-info-preparations/q2');
});

test('a preparing window polls the handle, then publishes the ready window', async () => {
  const {session, calls, timers} = fixture([response(202, preparing(), {Location: '/v1/file-info-preparations/q1', 'Retry-After': '1'}), response(200, page(['a']))]);
  const reading = session.open({itemId: 'i1'});
  await flush();assert.equal(timers.length, 1);timers[0]!();
  assert.deepEqual((await reading).page.rows, ['a']);
  assert.deepEqual(calls.map(call => call.path), ['/v1/items/i1/groups', '/v1/file-info-preparations/q1']);
});

test('a continuation onto a different opening is stale, and a stale reply reopens the same lane from its start', async () => {
  const changed = fixture([response(200, page(['a'], {next: 'n1'})), response(200, page(['b'], {basis: 'b2'}))]);
  await changed.session.open({itemId: 'i1'});
  await assert.rejects(changed.session.next(), (error: unknown) => error instanceof FileInfoWindowError && error.code === 'stale_continuation');
  const stale = fixture([response(200, page(['a'], {next: 'n1'})), response(409, {code: 'stale_continuation'}), response(200, page(['a2'], {basis: 'b2', queryId: 'q3'}))]);
  await stale.session.open({itemId: 'i1'});
  const again = await stale.session.next();
  assert.equal(again.reopened, true);assert.deepEqual(again.page.rows, ['a2']);
  assert.ok(stale.calls.some(call => call.method === 'GET' && call.path === '/v1/items/i1/groups' && stale.calls.indexOf(call) > 0), 'reopened without the old token');
});

test('resident rows are bounded: appending past the bound drops the earliest and says so', () => {
  const facts = (over: Partial<FileInfoWindowFacts> = {}): FileInfoWindowFacts => ({queryId: 'q', basis: 'b', limit: 2, returned: 2, ...over});
  let rows = fileInfoRows<number>(undefined, [1, 2], facts({next: 'n'}), 'replace', 3);
  assert.deepEqual(rows, {rows: [1, 2], more: true, earlier: false});
  rows = fileInfoRows(rows, [3, 4], facts({next: 'n2', previous: 'p'}), 'append', 3);
  assert.deepEqual(rows, {rows: [2, 3, 4], more: true, earlier: true});
  rows = fileInfoRows(rows, [5], facts({previous: 'p'}), 'append', 3);
  assert.deepEqual(rows, {rows: [3, 4, 5], more: false, earlier: true});
});

import {FileInfoSession, fileInfoTrackKey} from '../src/file-info-session.ts';
import type {FileInfoOpening} from '../src/file-info-pages.ts';

const opening: FileInfoOpening = {scope: {serverId: 's1', viewerId: 'v1'}, target: {libraryId: 'l1', itemId: 'm1'}, revision: {catalog: 4, viewer: 2}};
const envelope = (queryId: string, over: Record<string, unknown> = {}, window: Record<string, unknown> = {}) => ({
  scope: {serverId: 's1', libraryId: 'l1', itemId: 'm1', viewerFence: 'f1'}, detail: {scope: {serverId: 's1', libraryId: 'l1', itemId: 'm1', viewerFence: 'f1'}, revision: {catalog: 4, viewer: 2}},
  basis: 'b1', kind: 'file-info', window: {queryId, limit: 40, returned: 1, ...window}, ...over});
const surface = {fileCount: 2, eligibleFileCount: 2, groupCount: 2, canSplit: false, hasDiscColumns: false};
const groupRow = (ordinal: number) => ({id: `g${ordinal}`, headOccurrence: `m1.a${ordinal}`, ordinal, memberCount: 1, container: 'mkv', sizeBytes: 10, durationMs: 1000, audioCount: 1, subtitleCount: 0});

test('a File info session reads versions window by window and a finished lane gives up its lease', async () => {
  const replies = [
    response(200, envelope('q1', {surface, groups: [groupRow(1)]}, {nextCursor: 'n1'})),
    response(200, envelope('q2', {surface, groups: [groupRow(2)]})),
    response(200, envelope('q3', {lane: 'audio', occurrence: 'm1.a1', total: 1, tracks: [{ordinal: 1, codec: 'aac', language: 'en'}]})),
  ];
  const calls: string[] = [];
  const session = new FileInfoSession({opening, isCurrent: () => true, errorFromResponse: raw => new Error(String(raw.status)),
    api: {requestRaw(path, method) { calls.push(`${method} ${path}`); return Promise.resolve(method === 'DELETE' ? response(204) : replies.shift()!); }}});
  await session.groups();
  assert.equal(session.getSnapshot().groups.status, 'ready');assert.equal(session.getSnapshot().groups.rows?.more, true);
  await session.groups('next');
  assert.deepEqual(session.getSnapshot().groups.rows?.rows.map(row => row.ordinal), [1, 2]);assert.equal(session.getSnapshot().groups.rows?.more, false);
  assert.equal(session.getSnapshot().surface?.groupCount, 2);
  await session.tracks('m1.a1', 'audio');
  assert.equal(session.getSnapshot().tracks.get(fileInfoTrackKey('m1.a1', 'audio'))?.rows?.rows[0]?.language, 'en');
  assert.deepEqual(calls, ['GET /v1/items/m1/groups?limit=40', 'GET /v1/items/m1/groups?limit=40&cursor=n1', 'DELETE /v1/file-info-preparations/q1', 'DELETE /v1/file-info-preparations/q2',
    'GET /v1/items/m1/tracks?lane=audio&occurrence=m1.a1&limit=40', 'DELETE /v1/file-info-preparations/q3']);
  session.retire();
});

test('a window from another Detail revision is refused', async () => {
  const session = new FileInfoSession({opening, isCurrent: () => true, errorFromResponse: raw => new Error(String(raw.status)),
    api: {requestRaw: (_path, method) => Promise.resolve(method === 'DELETE' ? response(204) : response(200, {...envelope('q1', {surface, groups: [groupRow(1)]}), detail: {revision: {catalog: 5, viewer: 2}}}))}});
  await session.groups();
  assert.equal(session.getSnapshot().groups.status, 'failed');
});
