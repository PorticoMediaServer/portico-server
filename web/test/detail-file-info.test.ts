import test from 'node:test';
import assert from 'node:assert/strict';
import type {DetailApi} from '../../packages/client-core/src/detail.ts';
import {readDetailFileInfo} from '../src/screens/detail/file-info.ts';

const scope = {serverId: 'server', viewerId: 'viewer'};
const data = {scope: {serverId: 'server', libraryId: 'library', itemId: 'item', viewerFence: 'detail-fence'}, revision: {catalog: 3, viewer: 1}};
const file = (id: string) => ({id, container: 'mkv', durationMs: 100000, size: 1000, available: true, audio: [{codec: 'aac', language: 'en'}], subtitles: []});
const page = (id: string, nextCursor?: string) => ({scope: {...data.scope, viewerFence: 'file-fence'}, revision: data.revision, kind: 'movie', files: [file(id)], ...(nextCursor ? {nextCursor} : {})});
function api(read: (path: string, signal?: AbortSignal) => unknown | Promise<unknown>): DetailApi {
  return {request: async<T>(path: string, _method?: string, _body?: unknown, signal?: AbortSignal) => await read(path, signal) as T};
}

test('reads every file cursor in order, preserving actual facts and the separate FileInfo fence', async () => {
  const paths: string[] = [];
  const controller = new AbortController();
  const result = await readDetailFileInfo(api((path, signal) => {
    paths.push(path);
    assert.equal(signal, controller.signal);
    const cursor = new URL(path, 'http://test').searchParams.get('cursor');
    return cursor === 'tail' ? page('third') : cursor === 'next' ? page('second', 'tail') : page('first', 'next');
  }), scope, data, controller.signal, () => true);
  assert.equal(paths.length, 3);
  assert.match(paths[0], /limit=100/);
  assert.deepEqual(result.files.map(value => [value.id, value.duration, value.path]), [['first', 100, undefined], ['second', 100, undefined], ['third', 100, undefined]]);
  assert.equal(result.scope.viewerFence, 'file-fence');
  assert.equal(result.nextCursor, undefined);
  assert.equal(result.manage, undefined);
});

test('rejects changed revision or FileInfo authority instead of joining incompatible pages', async () => {
  for (const change of [
    {revision: {catalog: 4, viewer: 1}},
    {revision: {catalog: 3, viewer: 2}},
    {scope: {...data.scope, viewerFence: 'replacement-file-fence'}},
    {manage: {delete: true, split: true, merge: true}},
  ]) {
    let calls = 0;
    await assert.rejects(readDetailFileInfo(api(() => ++calls === 1 ? page('first', 'next') : {...page('second'), ...change}), scope, data, new AbortController().signal, () => true), {code: 'refresh_required'});
    assert.equal(calls, 2);
  }
});

test('a repeated cursor fails instead of looping or duplicating files', async () => {
  let calls = 0;
  await assert.rejects(readDetailFileInfo(api(() => page(String(++calls), 'same')), scope, data, new AbortController().signal, () => true), {code: 'refresh_required'});
  assert.equal(calls, 2);
});

test('retirement or abort during a read cannot publish or request the continuation', async () => {
  for (const abort of [false, true]) {
    let current = true;
    let calls = 0;
    const controller = new AbortController();
    await assert.rejects(readDetailFileInfo(api(() => {
      calls++;
      if (abort) controller.abort(); else current = false;
      return page('first', 'next');
    }), scope, data, controller.signal, () => current), {name: 'AbortError'});
    assert.equal(calls, 1);
  }
  let calls = 0;
  await assert.rejects(readDetailFileInfo(api(() => { calls++; return page('first'); }), scope, data, new AbortController().signal, () => false), {name: 'AbortError'});
  assert.equal(calls, 0);
});

test('canonical parsing rejects private paths without actual management authority', async () => {
  await assert.rejects(readDetailFileInfo(api(() => ({...page('first'), files: [{...file('first'), path: '/private/movie.mkv'}]})), scope, data, new AbortController().signal, () => true), {code: 'invalid_detail'});
  const result = await readDetailFileInfo(api(() => ({...page('first'), files: [{...file('first'), path: '/private/movie.mkv'}], manage: {delete: true, split: false, merge: false}})), scope, data, new AbortController().signal, () => true);
  assert.equal(result.files[0].path, '/private/movie.mkv');
  assert.deepEqual(result.manage, {delete: true, split: false, merge: false});
});

test('a genuine empty response remains distinct from an unread or failed resource', async () => {
  const result = await readDetailFileInfo(api(() => ({...page('unused'), files: []})), scope, data, new AbortController().signal, () => true);
  assert.deepEqual(result.files, []);
  assert.equal(result.scope.viewerFence, 'file-fence');
});
