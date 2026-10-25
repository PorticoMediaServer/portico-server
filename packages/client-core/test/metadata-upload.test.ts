import assert from 'node:assert/strict';
import test from 'node:test';
import {HttpLocalApi} from '../src/index.ts';
import {MetadataRepairService} from '../src/metadata-repair.ts';

const scope = {serverId: 'server', viewerId: 'owner'}, target = {kind: 'item' as const, id: 'movie', libraryId: 'library'};
function state(revision = 'b'.repeat(64)) {
  return {serverId: 'server', viewerFence: 'a'.repeat(64), target: {kind: 'item', id: 'movie'}, libraryId: 'library', revision,
    schema: [{field: 'title', label: 'Title', group: 'general', type: 'text', maxLength: 300, bulk: false}], artworkRoles: ['poster'],
    snapshot: {fields: {title: {value: 'Movie', automaticValue: 'Movie', source: 'automatic', locked: false}}, relationshipLocks: {}, relationships: [], artwork: []},
    candidates: [], history: [], artwork: {candidates: [], jobs: []}, cascades: []};
}
test('artwork upload carries the last revision and accepts only a target-bound reply', async () => {
  let sent: FormData | undefined, path = '';
  const service = new MetadataRepairService({scope, api: {async request<T>() {return state() as T;}, async requestMetadataUpload<T>(value: string, body: FormData) {
    path = value; sent = body; return state('c'.repeat(64)) as T;
  }}});
  await service.load(target);
  assert.equal(await service.uploadArtwork('poster', new Blob(['image'], {type: 'image/png'})), true);
  assert.equal(path, '/v1/metadata/item/movie/art/poster/upload');
  assert.equal(sent?.get('expectedRevision'), 'b'.repeat(64));
  assert.equal(sent?.get('subject'), '');
  assert.ok(sent?.get('file') instanceof Blob);
  assert.equal(service.getSnapshot().data?.revision, 'c'.repeat(64));
  service.dispose();
});
test('lost artwork response stays uncertain and cannot be replayed', async () => {
  let writes = 0;
  const service = new MetadataRepairService({scope, api: {async request<T>() {return state() as T;}, async requestMetadataUpload() {writes++; throw Error('lost');}}});
  await service.load(target);
  const file = new Blob(['image'], {type: 'image/jpeg'});
  assert.equal(await service.uploadArtwork('poster', file), false);
  assert.equal(service.getSnapshot().phase, 'uncertain');
  await assert.rejects(service.uploadArtwork('poster', file));
  assert.equal(writes, 1);
  await service.load(target);
  assert.equal(service.getSnapshot().phase, 'ready');
  service.dispose();
});
test('multipart transport keeps auth scoped and refuses a stale session response', async () => {
  let received: RequestInit | undefined;
  let finish!: (response: Response) => void;
  const api = new HttpLocalApi('https://server.example', 'first', async (_url, init) => {received = init; return new Promise(resolve => {finish = resolve;});});
  const body = new FormData(); body.append('file', new Blob(['image'], {type: 'image/png'}), 'artwork');
  const pending = api.requestMetadataUpload('/v1/metadata/item/movie/art/poster/upload', body);
  api.setAccessToken('second');
  finish(new Response('{}', {status: 200, headers: {'Content-Type': 'application/json'}}));
  await assert.rejects(pending, /old session/);
  const headers = new Headers(received?.headers);
  assert.equal(headers.get('Authorization'), 'Bearer first');
  assert.equal(headers.has('Content-Type'), false);
  assert.equal(received?.redirect, 'error');
  assert.equal(received?.credentials, 'omit');
  await assert.rejects(api.requestMetadataUpload('https://other.example/upload', body), /Invalid metadata upload path/);
});

test('saving subscribers cannot send an upload after retiring its attempt', async () => {
  for (const retire of ['cancel', 'dispose', 'successor'] as const) {
    let writes = 0;
    const service = new MetadataRepairService({scope, api: {async request<T>() {return state() as T;}, async requestMetadataUpload<T>() {writes++; return state() as T;}}});
    await service.load(target);
    let retired = false;
    const off = service.subscribe(() => {
      if (retired || service.getSnapshot().phase !== 'saving') return;
      retired = true;
      if (retire === 'successor') void service.load({...target, id: 'successor'});
      else service[retire]();
    });
    assert.equal(await service.uploadArtwork('poster', new Blob(['image'], {type: 'image/png'})), false);
    assert.equal(writes, 0);
    off(); service.dispose();
  }
});
