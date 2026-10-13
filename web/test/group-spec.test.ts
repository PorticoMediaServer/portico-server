import test from 'node:test';
import assert from 'node:assert/strict';
import {entityGroups, itemPivot, titlePivot} from '../src/app/group-spec.ts';
import {browsePageSource} from '../src/app/browse-page-source.ts';

test('a collection lists the library\'s titles, kept to the collection, in its own order first', () => {
  assert.deepEqual(entityGroups('lib', 'movie', {kind: 'collection', id: 'c1'}), {titles: {libraryId: 'lib', libraryKind: 'movie', pivot: 'movies', base: [{field: 'collection', operator: 'equals', value: 'c1'}], own: 'collection'}});
  assert.equal(entityGroups('lib', 'tv', {kind: 'collection', id: 'c1'}).titles?.pivot, 'shows');
  assert.equal(entityGroups('lib', 'anime', {kind: 'collection', id: 'c1'}).titles?.pivot, 'shows');
  assert.equal(entityGroups('lib', 'audiobook', {kind: 'collection', id: 'c1'}).titles?.pivot, 'books');
});

test('an artist lists its albums and its songs, each as a group of its own', () => {
  const groups = entityGroups('lib', 'music', {kind: 'artist', id: 'a1'});
  assert.deepEqual(groups.titles, {libraryId: 'lib', libraryKind: 'music', pivot: 'albums', base: [{field: 'memberOf', operator: 'equals', value: 'a1'}], own: 'default'});
  assert.deepEqual(groups.songs, {libraryId: 'lib', libraryKind: 'music', pivot: 'songs', base: [{field: 'memberOf', operator: 'equals', value: 'a1'}], own: 'default'});
});

test('an album opens in track order, which the server can sort by, so it needs no order of its own', () => {
  const groups = entityGroups('lib', 'music', {kind: 'album', id: 'al'});
  assert.equal(groups.titles, undefined);
  assert.deepEqual(groups.songs, {libraryId: 'lib', libraryKind: 'music', pivot: 'songs', base: [{field: 'memberOf', operator: 'equals', value: 'al'}], opening: {field: 'trackNumber', direction: 'asc'}});
});

test('an author lists books; a kind with no pivot of its own has no toolbar', () => {
  assert.equal(entityGroups('lib', 'audiobook', {kind: 'author', id: 'w'}).titles?.pivot, 'books');
  assert.deepEqual(entityGroups('lib', 'audiobook', {kind: 'book', id: 'b'}), {});
  assert.deepEqual(entityGroups('lib', 'music', {kind: 'disc', id: 'd'}), {});
  assert.deepEqual(entityGroups('lib', 'unknown', {kind: 'collection', id: 'c'}), {titles: undefined});
  assert.deepEqual(entityGroups('', 'movie', {kind: 'collection', id: 'c'}), {});
  assert.deepEqual(entityGroups('lib', 'movie', undefined), {});
});

test('titles and playable items have a pivot in every kind of library', () => {
  assert.deepEqual(['movie', 'tv', 'anime', 'music', 'audiobook', 'other'].map(titlePivot), ['movies', 'shows', 'shows', 'albums', 'books', undefined]);
  assert.deepEqual(['movie', 'tv', 'anime', 'music', 'audiobook', 'other'].map(itemPivot), ['movies', 'episodes', 'episodes', 'songs', 'books', undefined]);
});

const page = {pivot: 'songs', applied: {query: null, sort: []}, entries: [], pageInfo: {start: 0, total: 0, revision: 'r1', hasMore: false, nextCursor: ''}, positionIndex: []};

test('a group\'s condition is always sent, ahead of the viewer\'s filters', async () => {
  const sent: unknown[] = [];
  const api = {request: async <T,>(_path: string, _method?: string, body?: unknown) => { sent.push(body); return page as T; }};
  const base = [{field: 'memberOf', operator: 'equals' as const, value: 'album-1'}];
  await browsePageSource(api, {libraryId: 'lib', pivot: 'songs', base, predicates: [{field: 'favorite', operator: 'equals', value: true}], sort: {field: 'title', direction: 'asc'}})(0, 60, new AbortController().signal);
  // Clearing the viewer's filters leaves the group's own condition in place: the list never becomes the whole library.
  await browsePageSource(api, {libraryId: 'lib', pivot: 'songs', base, predicates: []})(0, 60, new AbortController().signal);
  assert.deepEqual((sent[0] as {query: unknown}).query, {all: [{field: 'memberOf', operator: 'equals', value: 'album-1'}, {field: 'favorite', operator: 'equals', value: true}]});
  assert.deepEqual((sent[1] as {query: unknown}).query, {all: [{field: 'memberOf', operator: 'equals', value: 'album-1'}]});
});

test('a library tab with no filters still sends no query', async () => {
  const sent: unknown[] = [];
  const api = {request: async <T,>(_path: string, _method?: string, body?: unknown) => { sent.push(body); return page as T; }};
  await browsePageSource(api, {libraryId: 'lib', pivot: 'songs', predicates: []})(0, 60, new AbortController().signal);
  assert.equal('query' in (sent[0] as object), false);
});
