import {useCallback, useEffect, useMemo, useRef, useSyncExternalStore} from 'react';
import type {BrowseSortSelection} from '@core/browse.ts';
import {BROWSE_PAGE, decodePredicates, encodePredicates, useBrowseCapabilities, useBrowseCollection, useShuffled, type Predicate} from './browse';
import {groupOffered, type GroupSpec} from './group-spec.ts';
import {groupMemoryId, groupMemoryTab, groupOrderSort, offersGroupOrder} from '@core/presentation/browse-groups.ts';
import {useGroupMemory} from './library-memory';
export {acrossGroup, entityGroups, itemPivot, titlePivot, type GroupOrder, type GroupSpec} from './group-spec.ts';

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
  // A group reopens with the sort and filters it was left with, unless the address names its own.
  const memory = useGroupMemory();
  const memoryId = spec ? groupMemoryId(spec.libraryId, spec.pivot, spec.base) : undefined;
  const restored = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (!memoryId || restored.current === memoryId) return;
    restored.current = memoryId;
    if (choice.sort || choice.filters) return;
    const left = memory.recall(memoryId)?.tabs[groupMemoryTab];
    if (left?.sort || left?.filters?.length) onChoice({...(left.sort ? {sort: left.sort.field, direction: left.sort.direction} : {}), filters: encodePredicates((left.filters ?? []) as readonly Predicate[])});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [memoryId, memory]);
  const read = useBrowseCapabilities(spec?.libraryId, spec?.pivot);
  // A library whose vocabulary cannot express the group offers no toolbar for it.
  const capabilities = read.capabilities && spec && groupOffered(spec, read.capabilities) ? read.capabilities : undefined;
  const error = read.error ?? (read.capabilities && !capabilities ? 'unavailable' : undefined);
  // A filter in the address the library does not offer is not sent (an old or hand-made link).
  const predicates = useMemo<readonly Predicate[]>(() => {
    const asked = decodePredicates(choice.filters);
    return capabilities ? asked.filter(p => capabilities.fields.some(f => f.id === p.field && f.operators.includes(p.operator))) : [];
  }, [choice.filters, capabilities]);
  const offered = choice.sort ? capabilities?.sorts.find(x => x.id === choice.sort) : undefined;
  const opening = useMemo<BrowseSortSelection>(() => {
    const own = spec?.opening && capabilities?.sorts.some(x => x.id === spec.opening!.field) ? spec.opening : undefined;
    // A group with an order of its own keeps it under a filter: the server sorts by it (groupOrder).
    const server = spec?.own && offersGroupOrder(capabilities?.sorts) ? {field: groupOrderSort, direction: 'asc' as const} : undefined;
    return own ?? server ?? capabilities?.resolvedPivot?.defaultSort[0] ?? titleOrder;
  }, [spec?.opening, spec?.own, capabilities]);
  const chosen = useMemo<BrowseSortSelection | undefined>(() => (offered ? {field: offered.id, direction: choice.direction && offered.directions.includes(choice.direction) ? choice.direction : offered.defaultDirection} : undefined), [offered, choice.direction]);
  const sort = useShuffled(chosen ?? opening);
  const filtered = predicates.length > 0;
  // The page's own list stands until something is chosen that it cannot show.
  const sameAsOpening = !!chosen && chosen.field === opening.field && chosen.direction === opening.direction;
  const active = !!spec && !!capabilities && (!!spec.always || filtered || (!!chosen && (!!spec.own || !sameAsOpening)));
  // Under a filter the own order is still the one in use when the server keeps it (groupOrder).
  const ownSelected = !!spec?.own && !spec.always && !chosen && (!filtered || opening.field === groupOrderSort);
  // One library's part of a wider list has nothing to show until that library's vocabulary has been read.
  const pending = !!spec?.always && !capabilities && !error;
  const browse = useBrowseCollection({libraryId: spec?.libraryId, pivot: spec?.pivot, base: spec?.base, predicates, sort, enabled: active, pageSize: capabilities ? Math.min(BROWSE_PAGE, capabilities.queryLimits.maximumLimit) : undefined});
  const total = useSyncExternalStore(browse.collection?.subscribe ?? noSubscribe, () => browse.collection?.getSnapshot().total);
  const totalIsLowerBound = useSyncExternalStore(browse.collection?.subscribe ?? noSubscribe, () => browse.collection?.getSnapshot().totalIsLowerBound ?? false);
  const setPredicates = useCallback((next: readonly Predicate[]) => { if (memoryId) memory.rememberFilters(memoryId, groupMemoryTab, next); onChoice({...choice, filters: encodePredicates(next)}); }, [choice, onChoice, memory, memoryId]);
  const setSort = useCallback((next: BrowseSortSelection) => { if (memoryId) memory.rememberSort(memoryId, groupMemoryTab, {field: next.field, direction: next.direction}); onChoice({...choice, sort: next.field, direction: next.direction}); }, [choice, onChoice, memory, memoryId]);
  /** Back to the page's own order (filters stay as they are). */
  const ownOrder = useCallback(() => { if (memoryId) memory.rememberSort(memoryId, groupMemoryTab, undefined); onChoice({filters: choice.filters}); }, [choice.filters, onChoice, memory, memoryId]);
  const clearFilters = useCallback(() => { if (memoryId) memory.rememberFilters(memoryId, groupMemoryTab, []); onChoice({...choice, filters: undefined}); }, [choice, onChoice, memory, memoryId]);
  return {spec, capabilities, error, predicates, sort, opening, filtered, active, pending, ownSelected, collection: active ? browse.collection : undefined, refresh: browse.refresh, total: active ? total : undefined, totalIsLowerBound: active && totalIsLowerBound, setPredicates, setSort, ownOrder, clearFilters};
}
export type GroupBrowse = ReturnType<typeof useGroupBrowse>;
