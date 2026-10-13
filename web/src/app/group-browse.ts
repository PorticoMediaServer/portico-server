import {useCallback, useMemo, useSyncExternalStore} from 'react';
import type {BrowseSortSelection} from '@core/browse.ts';
import {BROWSE_PAGE, decodePredicates, encodePredicates, useBrowseCapabilities, useBrowseCollection, useShuffled, type Predicate} from './browse';
import type {GroupSpec} from './group-spec.ts';
export {entityGroups, itemPivot, titlePivot, type GroupOrder, type GroupSpec} from './group-spec.ts';

/**
 * The library's sort and filter controls on a group of titles: a collection, an artist's albums,
 * an album's songs, a season's episodes, a person's titles, a playlist. The group is a library, a
 * pivot and a condition that is always applied (`base`); the viewer's own filters only narrow it,
 * so clearing them never leaves the group.
 *
 * A group page has an order of its own (the collection's, the season's, the playlist's): that is
 * what it shows until the viewer chooses a sort or a filter. From then on the list is the browse
 * engine's, with every sort and filter the server offers for the pivot.
 */
/** What the viewer chose, as the address keeps it. */
export type GroupChoice = Readonly<{sort?: string; direction?: 'asc' | 'desc'; filters?: string}>;

const noSubscribe = () => () => {};
const titleOrder: BrowseSortSelection = {field: 'title', direction: 'asc'};

export function useGroupBrowse(spec: GroupSpec | undefined, choice: GroupChoice, onChoice: (next: GroupChoice) => void) {
  const {capabilities, error} = useBrowseCapabilities(spec?.libraryId, spec?.pivot);
  // A filter in the address the library does not offer is not sent (an old or hand-made link).
  const predicates = useMemo<readonly Predicate[]>(() => {
    const asked = decodePredicates(choice.filters);
    return capabilities ? asked.filter(p => capabilities.fields.some(f => f.id === p.field && f.operators.includes(p.operator))) : [];
  }, [choice.filters, capabilities]);
  const offered = choice.sort ? capabilities?.sorts.find(x => x.id === choice.sort) : undefined;
  const opening = useMemo<BrowseSortSelection>(() => {
    const own = spec?.opening && capabilities?.sorts.some(x => x.id === spec.opening!.field) ? spec.opening : undefined;
    return own ?? capabilities?.resolvedPivot?.defaultSort[0] ?? titleOrder;
  }, [spec?.opening, capabilities]);
  const chosen = useMemo<BrowseSortSelection | undefined>(() => (offered ? {field: offered.id, direction: choice.direction && offered.directions.includes(choice.direction) ? choice.direction : offered.defaultDirection} : undefined), [offered, choice.direction]);
  const sort = useShuffled(chosen ?? opening);
  const filtered = predicates.length > 0;
  // The page's own list stands until something is chosen that it cannot show.
  const sameAsOpening = !!chosen && chosen.field === opening.field && chosen.direction === opening.direction;
  const active = !!spec && !!capabilities && (filtered || (!!chosen && (!!spec.own || !sameAsOpening)));
  const ownSelected = !!spec?.own && !chosen && !filtered;
  const browse = useBrowseCollection({libraryId: spec?.libraryId, pivot: spec?.pivot, base: spec?.base, predicates, sort, enabled: active, pageSize: capabilities ? Math.min(BROWSE_PAGE, capabilities.queryLimits.maximumLimit) : undefined});
  const total = useSyncExternalStore(browse.collection?.subscribe ?? noSubscribe, () => browse.collection?.getSnapshot().total);
  const setPredicates = useCallback((next: readonly Predicate[]) => onChoice({...choice, filters: encodePredicates(next)}), [choice, onChoice]);
  const setSort = useCallback((next: BrowseSortSelection) => onChoice({...choice, sort: next.field, direction: next.direction}), [choice, onChoice]);
  /** Back to the page's own order (filters stay as they are). */
  const ownOrder = useCallback(() => onChoice({filters: choice.filters}), [choice.filters, onChoice]);
  const clearFilters = useCallback(() => onChoice({...choice, filters: undefined}), [choice, onChoice]);
  return {spec, capabilities, error, predicates, sort, opening, filtered, active, ownSelected, collection: active ? browse.collection : undefined, refresh: browse.refresh, total: active ? total : undefined, setPredicates, setSort, ownOrder, clearFilters};
}
export type GroupBrowse = ReturnType<typeof useGroupBrowse>;
