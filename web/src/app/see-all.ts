import type {BrowseNode, BrowseSortSelection} from '@core/browse.ts';
import type {HomeRow} from '@core/home.ts';
import {libraryTabForPivot} from '@core/presentation/library-tabs.ts';
import type {Predicate} from './browse-page-source.ts';

/**
 * A Discover personal section's See all as the browse screen's own filters: the pivot's query as
 * its flat list of conditions (a lone condition, or an `all` of conditions), or undefined when the
 * query is richer than the filter bar can hold.
 */
export function seeAllPredicates(query: BrowseNode | undefined): Predicate[] | undefined {
  if (!query) return [];
  if ('field' in query) return [query as Predicate];
  if ('all' in query && query.all.every(n => 'field' in n)) return query.all.map(n => n as Predicate);
  return undefined;
}

/** Where a Home row's See all leads. */
export type HomeSeeAllDestination =
  | Readonly<{to: 'browse'; libraryId: string; tab: string; sort?: BrowseSortSelection; filters: readonly Predicate[]}>
  | Readonly<{to: 'row'; rowId: string}>
  | Readonly<{to: 'library'; libraryId: string}>;

/**
 * Where See all on a Home row goes. The server says: a row that names its own browse (`seeAll`:
 * the pivot, conditions and sort) opens the library on exactly that. Until every row does, the
 * one kind of library row this app knows the meaning of, the server's `recent` kind, opens the
 * library newest first. Any other row opens as its own page, which lists the row and nothing
 * else. Nothing is read out of a row's id. Only a row that offers none of these falls back to
 * its library.
 */
export function homeRowSeeAll(row: Pick<HomeRow, 'id' | 'kind' | 'libraryId' | 'seeAll' | 'cursorCapable'>, libraryKind: string | undefined): HomeSeeAllDestination | undefined {
  if (row.seeAll && row.libraryId && libraryKind) {
    const tab = libraryTabForPivot(libraryKind, row.seeAll.pivot);
    const filters = seeAllPredicates(row.seeAll.query);
    if (tab && filters) return {to: 'browse', libraryId: row.libraryId, tab: tab.id, ...(row.seeAll.sort[0] ? {sort: row.seeAll.sort[0]} : {}), filters};
  }
  if (row.kind === 'recent' && row.libraryId && !row.seeAll) return {to: 'browse', libraryId: row.libraryId, tab: 'browse', sort: {field: 'added', direction: 'desc'}, filters: []};
  if (row.cursorCapable) return {to: 'row', rowId: row.id};
  return row.libraryId ? {to: 'library', libraryId: row.libraryId} : undefined;
}
