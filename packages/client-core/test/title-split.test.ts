import assert from 'node:assert/strict';
import test from 'node:test';
import {canMergeTitles, findLibraryFile, folderOfPath, libraryFolders, separateLibraryFile, type TitleIdentityApi} from '../src/title-identity.ts';

const row = (n: number, assetId = `asset-${n}`) => ({id: `object-${n}`, assetId, revision: `rev-${n}`, itemId: 'item-1'});
const api = (answer: (path: string, method: string, body: unknown) => unknown, calls: string[] = []): TitleIdentityApi => ({request: async <T,>(path: string, method = 'GET', body?: unknown) => { calls.push(`${method} ${path}`); return answer(path, method, body) as T; }});

test('two titles are offered for merging when they are different titles of one kind, not known to be in different libraries', () => {
  const a = {id: 'a', kind: 'movie', libraryId: 'films'};
  assert.equal(canMergeTitles(a, {id: 'b', kind: 'movie', libraryId: 'films'}), true);
  assert.equal(canMergeTitles(a, a), false);
  assert.equal(canMergeTitles(a, {id: 'b', kind: 'show', libraryId: 'films'}), false);
  assert.equal(canMergeTitles(a, {id: 'b', kind: 'movie', libraryId: 'more-films'}), false);
  assert.equal(canMergeTitles({id: 'a', kind: 'season', libraryId: 'tv'}, {id: 'b', kind: 'season', libraryId: 'tv'}), false);
  assert.equal(canMergeTitles({id: 'a', kind: 'movie'}, {id: 'b', kind: 'movie'}), true);
  assert.equal(canMergeTitles({id: 'a', kind: 'movie'}, {id: 'b', kind: 'movie', libraryId: 'films'}), true);
});

test('a file\'s folder is the library folder with the longest path that leads to it', () => {
  const folders = [{id: 'all', path: '/media', resolvedPath: '/media'}, {id: 'films', path: '/media/films', resolvedPath: '/mnt/disk1/films'}, {id: 'win', path: 'D:\\Media\\Films', resolvedPath: 'D:\\Media\\Films'}];
  assert.equal(folderOfPath(folders, '/media/films/Ran (1985)/Ran.mkv'), 'films');
  assert.equal(folderOfPath(folders, '/mnt/disk1/films/Ran.mkv'), 'films');
  assert.equal(folderOfPath(folders, '/media/filmsets/x.mkv'), 'all');
  assert.equal(folderOfPath(folders, 'D:\\Media\\Films\\Ran\\Ran.mkv'), 'win');
  assert.equal(folderOfPath(folders, '/elsewhere/x.mkv'), undefined);
});

test('a library\'s folders are read one by one, and an unreadable one is left out alone', async () => {
  const folders = await libraryFolders(api(() => ({sources: [{id: 's1', path: '/a', resolvedPath: '/a'}, {id: '', path: 7}, {id: 's2', path: '/b', resolvedPath: '/b'}]})), 'films');
  assert.deepEqual(folders.map(f => f.id), ['s1', 's2']);
});

test('finding a file asks the library for that one file', async () => {
  const calls: string[] = [];
  const found = await findLibraryFile(api(() => ({items: [row(3), row(4, 'wanted')]}), calls), 'films', 'wanted', {folderId: 'folder-1'});
  assert.deepEqual(found, {objectId: 'object-4', revision: 'rev-4'});
  assert.equal(calls.length, 1);
  assert.match(calls[0]!, /^GET \/v1\/admin\/libraries\/films\/inventory\?limit=40&assetId=wanted&state=available&sourceId=folder-1$/);
});

test('a file the library does not list is not found', async () => {
  assert.equal(await findLibraryFile(api(() => ({items: [row(1)]})), 'films', 'wanted'), undefined);
  assert.equal(await findLibraryFile(api(() => ({items: []})), 'films', 'wanted'), undefined);
});

test('an answer that is not a file list is refused, and a stop is passed on', async () => {
  await assert.rejects(findLibraryFile(api(() => ({items: 'no'})), 'films', 'wanted'));
  await assert.rejects(findLibraryFile(api(() => ({items: [{assetId: 'wanted', id: '', revision: 'r'}]})), 'films', 'wanted'));
  const stop = new AbortController();
  let seen: AbortSignal | undefined;
  await findLibraryFile({request: async <T,>(_path: string, _method?: string, _body?: unknown, signal?: AbortSignal) => { seen = signal; return {items: []} as T; }}, 'films', 'wanted', {signal: stop.signal});
  assert.equal(seen, stop.signal);
});

test('splitting a file off answers with the new title', async () => {
  const calls: string[] = [];
  const bodies: unknown[] = [];
  const id = await separateLibraryFile(api((_path, _method, body) => { bodies.push(body); return {itemId: 'item-9'}; }, calls), 'films', {objectId: 'object 4', revision: 'rev-4'});
  assert.equal(id, 'item-9');
  assert.deepEqual(calls, ['POST /v1/admin/libraries/films/inventory/object%204/separate']);
  assert.deepEqual(bodies, [{revision: 'rev-4'}]);
  await assert.rejects(separateLibraryFile(api(() => ({})), 'films', {objectId: 'o', revision: 'r'}));
});

test('a refresh is asked on the show\'s own route for a show and on the item route for a playable title', async () => {
  const {refreshMetadataPath} = await import('../src/presentation/index.ts');
  assert.equal(refreshMetadataPath('show', 'show 1'), '/v1/shows/show%201/metadata/refresh');
  assert.equal(refreshMetadataPath('movie', 'm1'), '/v1/items/m1/metadata/refresh');
  assert.equal(refreshMetadataPath('episode', 'e1'), '/v1/items/e1/metadata/refresh');
  for (const kind of ['season', 'album', 'artist', 'book', undefined]) assert.equal(refreshMetadataPath(kind, 'x'), undefined, String(kind));
});
