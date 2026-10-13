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
  | Readonly<{to: 'library'; libraryId: string}>
  | Readonly<{to: 'saved_view'; resourceId: string}>;

/** Follow the destination published with the composed Home row. */
export function homeRowSeeAll(row: Pick<HomeRow, 'id' | 'kind' | 'libraryId' | 'seeAll' | 'cursorCapable'>, libraryKind: string | undefined): HomeSeeAllDestination | undefined {
  const target = row.seeAll;
  if (!target) {
    if (row.kind === 'recent' && row.libraryId) return {to: 'browse', libraryId: row.libraryId, tab: 'browse', sort: {field: 'added', direction: 'desc'}, filters: []};
    if (row.cursorCapable) return {to: 'row', rowId: row.id};
    return row.libraryId ? {to: 'library', libraryId: row.libraryId} : undefined;
  }
  if (target.type === 'row') return {to: 'row', rowId: target.rowId};
  if (target.type === 'saved_view') return {to: 'saved_view', resourceId: target.resourceId};
  const tab = libraryKind ? libraryTabForPivot(libraryKind, target.pivot) : undefined;
  const filters = seeAllPredicates(target.query);
  if (tab && filters) return {to: 'browse', libraryId: target.libraryId, tab: tab.id, ...(target.sort[0] ? {sort: target.sort[0]} : {}), filters};
  // The row page retains the server's membership when its query cannot be represented here.
  return {to: 'row', rowId: row.id};
}
