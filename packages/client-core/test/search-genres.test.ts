import assert from 'node:assert/strict';
import test from 'node:test';
import {parseSearchGenres, readSearchGenres, searchGenresPath, searchGenreBrowsePivot, searchGenreBrowseRequest} from '../src/search-genres.ts';
import type {LibraryContentApi} from '../src/library-content.ts';

const artwork = (id: string, role: 'poster' | 'backdrop', kind = 'item') => ({kind, id, path: `/v1/metadata/${kind}/${id}/art/${role}?v=${'a'.repeat(64)}&w=${role === 'backdrop' ? 1920 : 400}`});
const answer = () => ({revision: 'opaque-authorized-revision', genres: [
  {value: 'Sci-Fi & Fantasy', label: 'Sci-Fi & Fantasy', count: 17, backdrop: artwork('film-a', 'backdrop'), covers: [artwork('film-b', 'poster'), artwork('show-c', 'poster', 'show')]},
  {value: 'Crime', label: 'Crime', count: 9, covers: []},
], nextCursor: 'opaque.cursor'});

test('preserves exact server order, metadata labels, counts and missing art immutably', () => {
  const raw = answer(), page = parseSearchGenres(raw);
  assert.deepEqual(page.genres.map(genre => [genre.value, genre.label, genre.count]), [['Sci-Fi & Fantasy', 'Sci-Fi & Fantasy', 17], ['Crime', 'Crime', 9]]);
  assert.equal(page.genres[1].backdrop, undefined);
  assert.deepEqual(page.genres[1].covers, []);
  assert.equal(page.nextCursor, 'opaque.cursor');
  assert.ok(Object.isFrozen(page) && Object.isFrozen(page.genres) && Object.isFrozen(page.genres[0]) && Object.isFrozen(page.genres[0].covers) && Object.isFrozen(page.genres[0].covers[0]) && Object.isFrozen(page.genres[0].backdrop));
  raw.genres.reverse();
  raw.genres[1].label = 'changed later';
  assert.equal(page.genres[0].label, 'Sci-Fi & Fantasy');
});

test('refuses unsupported/null art, invalid counts, duplicate genres and unbounded pages', () => {
  for (const mutate of [
    (raw: any) => raw.genres[0].backdrop = null,
    (raw: any) => raw.genres[0].covers = null,
    (raw: any) => raw.genres[0].covers.push(artwork('film-d', 'poster')),
    (raw: any) => raw.genres[0].covers[1] = raw.genres[0].covers[0],
    (raw: any) => raw.genres[0].count = 0,
    (raw: any) => raw.genres[0].count = 1.2,
    (raw: any) => raw.genres[1].value = raw.genres[0].value,
    (raw: any) => raw.genres[0].label = '',
    (raw: any) => raw.nextCursor = null,
    (raw: any) => raw.nextCursor = 'x'.repeat(4097),
    (raw: any) => raw.genres = Array(51).fill(raw.genres[0]),
  ]) {const raw = answer(); mutate(raw); assert.throws(() => parseSearchGenres(raw));}
});

test('accepts only the exact selected-art kind, subject, role and bounded relative route', () => {
  for (const path of [
    'https://provider.invalid/art?token=private', '//provider.invalid/art', '/private/file.jpg',
    `https://metadata.invalid${artwork('film-a', 'backdrop').path}`, `//metadata.invalid${artwork('film-a', 'backdrop').path}`,
    artwork('another-title', 'backdrop').path,
    artwork('film-a', 'poster').path,
    artwork('film-a', 'backdrop', 'show').path,
    `${artwork('film-a', 'backdrop').path}&token=private`,
    `${artwork('film-a', 'backdrop').path}#fragment`,
    artwork('film-a', 'backdrop').path.replace('v=', 'subject=someone&v='),
    artwork('film-a', 'backdrop').path.replace('a'.repeat(64), 'A'.repeat(64)),
    artwork('film-a', 'backdrop').path.replace('w=1920', 'w=400'),
    `${artwork('film-a', 'backdrop').path}&v=${'b'.repeat(64)}`,
  ]) {const raw = answer(); raw.genres[0].backdrop!.path = path; assert.throws(() => parseSearchGenres(raw), path);}
  const raw: any = answer();
  raw.token = 'private'; raw.genres[0].providerURL = 'https://private.invalid'; raw.genres[0].backdrop.token = 'secret';
  const page = parseSearchGenres(raw);
  assert.equal(JSON.stringify(page).includes('private'), false);
  assert.equal(JSON.stringify(page).includes('secret'), false);
});

test('uses the generated route and preserves the exact opaque continuation', async () => {
  assert.equal(searchGenresPath(), '/v1/search/genres?limit=50');
  assert.equal(searchGenresPath({limit: 8, cursor: 'opaque/+='}), '/v1/search/genres?limit=8&cursor=opaque%2F%2B%3D');
  for (const limit of [0, -1, 51, Infinity, 1.2]) assert.throws(() => searchGenresPath({limit}));
  const controller = new AbortController();
  const api: LibraryContentApi = {request: async <T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T> => {
    assert.equal(path, '/v1/search/genres?limit=8'); assert.equal(method, 'GET'); assert.equal(body, undefined); assert.equal(signal, controller.signal);
    return answer() as T;
  }};
  assert.equal((await readSearchGenres(api, {limit: 8}, controller.signal)).genres.length, 2);
});

test('a cancelled caller neither starts another read nor installs a late response', async () => {
  const controller = new AbortController();
  let reads = 0;
  const api: LibraryContentApi = {request: async <T>(): Promise<T> => {reads++; controller.abort(); return answer() as T;}};
  await assert.rejects(readSearchGenres(api, {}, controller.signal), {name: 'AbortError'});
  assert.equal(reads, 1);
  await assert.rejects(readSearchGenres(api, {}, controller.signal), {name: 'AbortError'});
  assert.equal(reads, 1);
});

test('a genre opens the canonical video-only across browse with its exact value', () => {
  const request = searchGenreBrowseRequest('Sci-Fi & Fantasy');
  assert.equal(searchGenreBrowsePivot, 'video-titles');
  assert.deepEqual(request, {pivot: 'video-titles', query: {field: 'genre', operator: 'equals', value: 'Sci-Fi & Fantasy'}, sort: [{field: 'title', direction: 'asc'}]});
  assert.ok(Object.isFrozen(request) && Object.isFrozen(request.query) && Object.isFrozen(request.sort));
  for (const value of ['', 'x'.repeat(1025), 'Crime\u0000private']) assert.throws(() => searchGenreBrowseRequest(value));
});

test('slot previews are independent bounded reads and retain artless-directory presence', async () => {
  assert.equal(searchGenresPath({limit: 6, preview: 'covers'}), '/v1/search/genres?limit=6&preview=covers');
  assert.throws(() => searchGenresPath({preview: 'covers', cursor: 'list-only'}));
  for (const raw of [
    {revision: 'basis', genres: [], hasGenres: true},
    {revision: 'basis', genres: [], hasGenres: false},
  ]) {
    const page = await readSearchGenres({request: async <T>() => raw as T}, {limit: 4, preview: 'backdrop'});
    assert.equal(page.hasGenres, raw.hasGenres);
    assert.deepEqual(page.genres, []);
  }
  for (const raw of [
    {revision: 'basis', genres: []},
    {revision: 'basis', genres: [], hasGenres: true, nextCursor: 'do-not-drain'},
    {revision: 'basis', genres: [answer().genres[1]], hasGenres: true},
    {revision: 'basis', genres: answer().genres, hasGenres: true},
  ]) {
    await assert.rejects(readSearchGenres({request: async <T>() => raw as T}, {limit: 1, preview: 'backdrop'}));
  }
});
