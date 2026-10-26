import test from 'node:test';
import assert from 'node:assert/strict';
import type {GetItemsIdResponse} from '../../packages/contracts/src/types.ts';
import {castCoverPath, castCoverTarget} from '../src/screens/watch/cast-cover.ts';

const item = (fields: Partial<GetItemsIdResponse>): GetItemsIdResponse => ({
  addedAt: null, available: true, durationMs: 180_000, id: 'song-1', kind: 'song',
  libraryId: 'music', missing: false, progressMs: 0, title: 'DNA.', ...fields,
} as GetItemsIdResponse);

const song = (albumId: string): NonNullable<GetItemsIdResponse['song']> => ({
  albumArtist: 'Kendrick Lamar', albumId, albumTitle: 'DAMN.', artist: 'Kendrick Lamar',
  providerMatchStatus: 'matched', providerReleaseStatus: 'matched',
});

const bookFile = (bookId: string): NonNullable<GetItemsIdResponse['bookFile']> => ({
  bookId, bookTitle: 'Project Hail Mary', sourceBoundary: 'file',
});

const relatedAlbum = (fields: Partial<GetItemsIdResponse> = {}): GetItemsIdResponse => item({
  id: 'album-1', kind: 'album', title: 'DAMN.', posterUrl: '/v1/items/album-1/art/poster', ...fields,
});

const albumTarget = Object.freeze({id: 'album-1', kind: 'album' as const});

test('a song with no image of its own targets its album', () => {
  assert.deepEqual(castCoverTarget(item({song: song('album-1')})), {id: 'album-1', kind: 'album'});
});

test('an audiobook file targets its book container, not the item kind', () => {
  assert.deepEqual(castCoverTarget(item({id: 'chapter-1', kind: 'audiobook_file', libraryId: 'books', bookFile: bookFile('book-1')})), {id: 'book-1', kind: 'book'});
});

test('an item with its own poster or backdrop is not re-read', () => {
  assert.equal(castCoverTarget(item({song: song('album-1'), posterUrl: '/v1/items/song-1/art/poster'})), undefined);
  assert.equal(castCoverTarget(item({song: song('album-1'), backdropUrl: '/v1/items/song-1/art/backdrop'})), undefined);
});

test('a movie or an item without a related id has no target', () => {
  assert.equal(castCoverTarget(item({id: 'movie-1', kind: 'movie', libraryId: 'movies'})), undefined);
  assert.equal(castCoverTarget(item({song: song('')})), undefined);
  assert.equal(castCoverTarget(item({song: song('album 1')})), undefined);
});

test('the related id is bounded and cannot traverse the catalogue route', () => {
  assert(castCoverTarget(item({song: song('a'.repeat(128))})));
  assert.equal(castCoverTarget(item({song: song('a'.repeat(129))})), undefined);
  for (const id of ['.', '..', '/album-1', 'album-1/art', 'album\\1']) {
    assert.equal(castCoverTarget(item({song: song(id)})), undefined, id);
  }
});

test('a related cover is selected only for the exact expected entity', () => {
  assert.equal(castCoverPath(albumTarget, relatedAlbum()), '/v1/items/album-1/art/poster');
  assert.equal(castCoverPath(albumTarget, relatedAlbum({id: 'album-2'})), undefined);
  assert.equal(castCoverPath(albumTarget, relatedAlbum({missing: true})), undefined);
  assert.equal(castCoverPath(albumTarget, relatedAlbum({kind: 'audiobook_file'})), undefined);
  assert.equal(castCoverPath(albumTarget, relatedAlbum({kind: 'audiobook'})), undefined);
  assert.equal(castCoverPath({id: 'book-1', kind: 'book'}, item({id: 'book-1', kind: 'book', posterUrl: '/v1/items/book-1/art/poster'})), '/v1/items/book-1/art/poster');
});

test('a related cover never falls back to its backdrop or to another image field', () => {
  assert.equal(castCoverPath(albumTarget, relatedAlbum({posterUrl: undefined, backdropUrl: '/v1/items/album-1/art/backdrop'})), undefined);
  assert.equal(castCoverPath(albumTarget, relatedAlbum({posterUrl: undefined, stillUrl: '/v1/items/album-1/art/still'})), undefined);
});

test('only a same-origin catalogue path is accepted', () => {
  for (const poster of [
    'https://server.example/v1/items/album-1/art/poster',
    '//server.example/v1/items/album-1/art/poster',
    'v1/items/album-1/art/poster',
    '/v1/items/album 1/art/poster',
    '/v1/items/album\\1/art/poster',
    '/v1/items/album-1/art/poster\n',
    '',
    '/' + 'a'.repeat(501),
  ]) assert.equal(castCoverPath(albumTarget, relatedAlbum({posterUrl: poster})), undefined, poster);
  assert.equal(castCoverPath(albumTarget, relatedAlbum({posterUrl: '/v1/items/album-1/art/poster?v=2'})), '/v1/items/album-1/art/poster?v=2');
});

test('no target or no related read selects no path', () => {
  assert.equal(castCoverPath(undefined, relatedAlbum()), undefined);
  assert.equal(castCoverPath(albumTarget, undefined), undefined);
});
