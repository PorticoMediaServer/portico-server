import {useEffect, useMemo} from 'react';
import {createWindowedCollection, type PageResult, type WindowedCollection} from '@core/collections/index.ts';
import {validateContentProjection, type ContentEntry, type ContentScope, type LibraryContentApi} from '@core/library-content.ts';

/**
 * A season's episodes as a windowed collection (Spec — Title Pages §2: the
 * list is never loaded whole). Today's `/show-workspace` pages by cursor, not
 * by offset, so page k needs page k−1's cursor: the source remembers every
 * cursor it has seen and walks forward from the nearest one when a page far
 * ahead is asked for. When lane B's offset/`range` episode pages land, only
 * `fetchPage` changes (deferred-to-contract).
 *
 * The workspace's own first page seeds page 0, so opening a show costs no
 * extra request.
 */
export type EpisodeScope = {libraryId: string; showId: string; seasonId: string | null; group?: string};
export type EpisodeSeed = {entries: readonly ContentEntry[]; nextCursor: string | null; total: number};

export const EPISODE_PAGE = 40;

export function episodeSource(api: LibraryContentApi, scope: ContentScope, where: EpisodeScope, seed?: EpisodeSeed) {
  const cursors = new Map<number, string | null>([[0, null]]);
  let total = seed?.total;
  if (seed) cursors.set(1, seed.nextCursor);
  const request = async (page: number, signal: AbortSignal) => {
    const cursor = cursors.get(page);
    const params = new URLSearchParams({showId: where.showId, limit: String(EPISODE_PAGE)});
    if (where.seasonId) params.set('seasonId', where.seasonId);
    if (where.group) params.set('group', where.group);
    if (cursor) params.set('cursor', cursor);
    const raw = await api.request<{episodes?: unknown}>(`/v1/libraries/${encodeURIComponent(where.libraryId)}/show-workspace?${params}`, 'GET', undefined, signal);
    const projection = validateContentProjection(raw?.episodes, {libraryId: where.libraryId, view: where.seasonId ? 'season' : 'show', entityId: where.seasonId ?? where.showId, sort: 'episode', direction: 'asc', category: '', q: ''}, scope, EPISODE_PAGE);
    const section = projection.sections[0];
    cursors.set(page + 1, section?.nextCursor || null);
    if (section) total = section.totalCount;
    return section?.entries ?? [];
  };
  return async (start: number, count: number, signal: AbortSignal): Promise<PageResult<ContentEntry>> => {
    const page = Math.floor(start / count);
    if (page === 0 && seed) return {items: seed.entries, total: seed.total};
    // Walk forward from the nearest known cursor (only when jumping ahead).
    let known = page;
    while (!cursors.has(known)) known--;
    for (let k = known; k < page; k++) {
      await request(k, signal);
      if (cursors.get(k + 1) === null) return {items: [], total: total ?? start};
    }
    if (page > 0 && cursors.get(page) === null) return {items: [], total: total ?? start};
    const items = await request(page, signal);
    return {items, total: total ?? items.length};
  };
}

/** One collection per show + season; disposed when either changes. */
export function useEpisodes(api: LibraryContentApi, scope: ContentScope, where: EpisodeScope | undefined, seed: EpisodeSeed | undefined): WindowedCollection<ContentEntry> | undefined {
  const key = where ? JSON.stringify(where) : '';
  // The seed belongs to the scope it came with; a new scope starts from its own seed.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const collection = useMemo(() => (where && seed ? createWindowedCollection<ContentEntry>({fetchPage: episodeSource(api, scope, where, seed), pageSize: EPISODE_PAGE, maxResidentPages: 6, keyOf: e => e.id}) : undefined), [api, scope, key, !!seed]);
  useEffect(() => () => collection?.dispose(), [collection]);
  return collection;
}
