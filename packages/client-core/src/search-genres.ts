import {decodeGetSearchGenresResponse} from '../../contracts/src/decoders.ts';
import {routePath, type RouteResponse} from '../../contracts/src/route-paths.ts';
import type {BrowseRequestBody} from './browse.ts';
import type {LibraryContentApi} from './library-content.ts';
import {unreadableServerResponse} from './server-messages.ts';

export type SearchGenreArt = Readonly<{kind: 'item' | 'show'; id: string; path: string}>;
export type SearchGenre = Readonly<{
  value: string; label: string; count: number;
  backdrop?: SearchGenreArt; covers: readonly SearchGenreArt[];
}>;
export type SearchGenrePage = Readonly<{
  revision: string; genres: readonly SearchGenre[]; nextCursor?: string;
}>;
export type SearchGenreRequest = Readonly<{limit?: number; cursor?: string}>;

const object = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null && !Array.isArray(value);
const text = (value: unknown, max: number): value is string => typeof value === 'string' && value.length > 0 && value.length <= max && !/[\x00-\x1f\x7f]/.test(value);
function invalid(): never { throw new Error(unreadableServerResponse); }

function art(value: {kind: string; id: string; path: string}, role: 'backdrop' | 'poster'): SearchGenreArt {
  if ((value.kind !== 'item' && value.kind !== 'show') || !/^[a-z0-9_-]{1,256}$/i.test(value.id) || !text(value.path, 2048)) invalid();
  const expected = routePath('get_metadata_kind_id_art_role', {kind: value.kind, id: value.id, role});
  if (!value.path.startsWith(`${expected}?`)) invalid();
  // Only the selected-art route for this exact title and slot can be returned.
  // A URL, provider locator, different subject, fragment or extra query is not
  // an art reference. The query width requests a variant; it is not geometry.
  const parsed = new URL(value.path, 'https://metadata.invalid');
  if (parsed.origin !== 'https://metadata.invalid' || parsed.pathname !== expected || parsed.hash || parsed.searchParams.size !== 2 || !/^[a-f0-9]{64}$/.test(parsed.searchParams.get('v') ?? '') || parsed.searchParams.get('w') !== (role === 'backdrop' ? '1920' : '400')) invalid();
  return Object.freeze({kind: value.kind, id: value.id, path: value.path});
}

/** The server owns counts, ordering and representatives. This parser preserves
 * them, including counted genres with missing art for the All genres list.
 * Each surface applies only its drawn art requirement and prefix length. */
export function parseSearchGenres(input: unknown): SearchGenrePage {
  // Optional nonnull slots are absence or a real value, never null. Retain
  // this distinction before a forward-compatible generated decoder copies it.
  if (!object(input) || input.nextCursor === null || !Array.isArray(input.genres) || input.genres.length > 50 || input.genres.some(genre => !object(genre) || genre.backdrop === null)) invalid();
  let raw: RouteResponse<'get_search_genres'>;
  try { raw = decodeGetSearchGenresResponse(input); } catch { invalid(); }
  if (!text(raw.revision, 1024) || raw.nextCursor !== undefined && !text(raw.nextCursor, 4096)) invalid();
  const seen = new Set<string>();
  const genres = raw.genres.map(genre => {
    if (!text(genre.value, 1024) || !text(genre.label, 1024) || seen.has(genre.value)) invalid();
    seen.add(genre.value);
    const covers = genre.covers.map(value => art(value, 'poster'));
    if (new Set(covers.map(value => `${value.kind}:${value.id}`)).size !== covers.length) invalid();
    return Object.freeze({value: genre.value, label: genre.label, count: genre.count,
      ...(genre.backdrop !== undefined ? {backdrop: art(genre.backdrop, 'backdrop')} : {}),
      covers: Object.freeze(covers)});
  });
  return Object.freeze({revision: raw.revision, genres: Object.freeze(genres), ...(raw.nextCursor !== undefined ? {nextCursor: raw.nextCursor} : {})});
}

export function searchGenresPath(request: SearchGenreRequest = {}): string {
  const limit = request.limit ?? 50;
  if (!Number.isSafeInteger(limit) || limit < 1 || limit > 50 || request.cursor !== undefined && request.cursor !== '' && !text(request.cursor, 4096)) throw new Error('Invalid genre read window.');
  const query = new URLSearchParams({limit: String(limit)});
  if (request.cursor) query.set('cursor', request.cursor);
  return `${routePath('get_search_genres')}?${query}`;
}

function current(signal?: AbortSignal): void {
  if (signal?.aborted) throw signal.reason instanceof Error ? signal.reason : Object.assign(new Error('Genre read cancelled.'), {name: 'AbortError'});
}

/** One canonical authorized read for Search and All genres. The parent's API
 * and cancellation lifetime stay in force through response parsing. */
export async function readSearchGenres(api: LibraryContentApi, request: SearchGenreRequest = {}, signal?: AbortSignal): Promise<SearchGenrePage> {
  current(signal);
  const raw = await api.request<RouteResponse<'get_search_genres'>>(searchGenresPath(request), 'GET', undefined, signal);
  current(signal);
  return parseSearchGenres(raw);
}

/** A tile opens the exact movie/show domain its count describes through the
 * existing across-library browse read. Use this pivot for its capabilities
 * and context facets as well; the parent retains viewer/cursor authority. */
export const searchGenreBrowsePivot = 'video-titles';
export function searchGenreBrowseRequest(value: string): BrowseRequestBody {
  if (!text(value, 1024)) throw new Error('Invalid genre value.');
  return Object.freeze({pivot: searchGenreBrowsePivot,
    query: Object.freeze({field: 'genre', operator: 'equals', value}),
    sort: Object.freeze([Object.freeze({field: 'title', direction: 'asc' as const})])});
}
