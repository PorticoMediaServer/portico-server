import type {BrowseNode} from '@core/browse.ts';
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
