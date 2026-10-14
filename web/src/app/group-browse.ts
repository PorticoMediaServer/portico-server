import {useCallback, useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {parseBrowseResult, type BrowseNode, type BrowseSortSelection} from '@core/browse.ts';
import {BROWSE_PAGE, decodePredicates, encodePredicates, useBrowseCapabilities, useBrowseCollection, useShuffled, type Predicate} from './browse';
import {groupOffered, type GroupLibrary, type GroupSpec} from './group-spec.ts';
import {useLibrariesContext} from './libraries';
import {useSession} from './session';
export {entityGroups, itemPivot, spanningGroup, titlePivot, type GroupLibrary, type GroupOrder, type GroupSpec} from './group-spec.ts';

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
    return own ?? capabilities?.resolvedPivot?.defaultSort[0] ?? titleOrder;
  }, [spec?.opening, capabilities]);
  const chosen = useMemo<BrowseSortSelection | undefined>(() => (offered ? {field: offered.id, direction: choice.direction && offered.directions.includes(choice.direction) ? choice.direction : offered.defaultDirection} : undefined), [offered, choice.direction]);
  const sort = useShuffled(chosen ?? opening);
  const filtered = predicates.length > 0;
  // The page's own list stands until something is chosen that it cannot show.
  const sameAsOpening = !!chosen && chosen.field === opening.field && chosen.direction === opening.direction;
  const active = !!spec && !!capabilities && (!!spec.always || filtered || (!!chosen && (!!spec.own || !sameAsOpening)));
  const ownSelected = !!spec?.own && !spec.always && !chosen && !filtered;
  // One library's part of a wider list has nothing to show until that library's vocabulary has been read.
  const pending = !!spec?.always && !capabilities && !error;
  const browse = useBrowseCollection({libraryId: spec?.libraryId, pivot: spec?.pivot, base: spec?.base, predicates, sort, enabled: active, pageSize: capabilities ? Math.min(BROWSE_PAGE, capabilities.queryLimits.maximumLimit) : undefined});
  const total = useSyncExternalStore(browse.collection?.subscribe ?? noSubscribe, () => browse.collection?.getSnapshot().total);
  const totalIsLowerBound = useSyncExternalStore(browse.collection?.subscribe ?? noSubscribe, () => browse.collection?.getSnapshot().totalIsLowerBound ?? false);
  const setPredicates = useCallback((next: readonly Predicate[]) => onChoice({...choice, filters: encodePredicates(next)}), [choice, onChoice]);
  const setSort = useCallback((next: BrowseSortSelection) => onChoice({...choice, sort: next.field, direction: next.direction}), [choice, onChoice]);
  /** Back to the page's own order (filters stay as they are). */
  const ownOrder = useCallback(() => onChoice({filters: choice.filters}), [choice.filters, onChoice]);
  const clearFilters = useCallback(() => onChoice({...choice, filters: undefined}), [choice, onChoice]);
  return {spec, capabilities, error, predicates, sort, opening, filtered, active, pending, ownSelected, collection: active ? browse.collection : undefined, refresh: browse.refresh, total: active ? total : undefined, totalIsLowerBound: active && totalIsLowerBound, setPredicates, setSort, ownOrder, clearFilters};
}
export type GroupBrowse = ReturnType<typeof useGroupBrowse>;

/**
 * Which libraries hold part of a list that spans them (a person's titles, a playlist, My List),
 * and how much: one small read per library the viewer has, asked once when the page opens. A
 * library whose vocabulary cannot express the list (a person in a music library) is left out.
 */
export function useGroupLibraries(base: readonly BrowseNode[] | undefined, pivotOf: (libraryKind: string) => string | undefined): {libraries: readonly GroupLibrary[]; ready: boolean} {
  const {api} = useSession();
  const {items} = useLibrariesContext();
  const key = base ? JSON.stringify([base, items.map(library => library.id)]) : '';
  const [state, setState] = useState<{key: string; libraries: readonly GroupLibrary[]}>({key: '', libraries: []});
  useEffect(() => {
    if (!base || !key) return;
    let live = true;
    const candidates = items.flatMap(library => { const pivot = pivotOf(library.kind); return pivot ? [{id: library.id, name: library.name, kind: library.kind, pivot}] : []; });
    void Promise.all(candidates.map(library => api.request<unknown>(`/v1/libraries/${encodeURIComponent(library.id)}/browse`, 'POST', {pivot: library.pivot, query: {all: base}, limit: 1}).then(
      raw => { try { return {...library, total: parseBrowseResult(raw).pageInfo.total}; } catch { return {...library, total: 0}; } },
      () => ({...library, total: 0}),
    ))).then(found => { if (live) setState({key, libraries: found.filter(library => library.total > 0)}); });
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api, key]);
  return state.key === key && key ? {libraries: state.libraries, ready: true} : {libraries: [], ready: false};
}
