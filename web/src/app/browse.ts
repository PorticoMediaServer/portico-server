import {createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {currentI18n} from './i18n';
import {createWindowedCollection, type FetchPage, type WindowedCollection} from '@core/collections/index.ts';
import {titlesWithPersonalState} from '@core/freshness-hub.ts';
import {usePooled} from './freshness';
import type {ContentEntry} from '@core/library-content.ts';
import {acrossLibraries, browseCapabilitiesPath, browseFacetsPath, parseBrowseCapabilities, parseBrowseFacets, type BrowseCapabilities, type BrowseFacetValue, type BrowseNode, type BrowseSortSelection} from '@core/browse.ts';
import {appliedFilterLabel, browseNameId} from '@core/presentation/index.ts';
import type {MessageId} from '@i18n';
import {sessionIdentity, useSession} from './session';
import {errorText} from './errors';
import {browsePageSource, type Predicate} from './browse-page-source.ts';
import {RevalidatingCache} from './revalidating-cache.ts';
export {browsePageSource} from './browse-page-source.ts';
export type {Predicate} from './browse-page-source.ts';

/**
 * Library browsing on the server's expression engine. The server publishes the
 * vocabulary (fields, operators, sorts, quick filters) per library and pivot;
 * this hook only assembles requests from it and pages results. Filters live in
 * the URL as a compact JSON list of predicates so a view is linkable.
 */

/**
 * A library's vocabulary has no revision to key on, so what was read last is shown at once and
 * checked again each time a library is opened: a server upgrade, a changed library setting or
 * facets that appear after the first scan reach an open tab the next time the library is shown.
 * Entries belong to whoever is signed in and are forgotten when that changes.
 */
const capabilityCache = new RevalidatingCache<BrowseCapabilities>({max: 32});
export function useBrowseCapabilities(libraryId: string | undefined, pivot: string | undefined) {
  const {api, session} = useSession();
  const [state, setState] = useState<{capabilities?: BrowseCapabilities; error?: string; key?: string}>({});
  const who = sessionIdentity(session);
  const key = libraryId && pivot ? `${libraryId}:${pivot}` : undefined;
  useEffect(() => {
    if (!libraryId || !pivot || !key) { setState({}); return; }
    return capabilityCache.use(who, key, () => api.request<unknown>(browseCapabilitiesPath(libraryId, pivot), 'GET').then(parseBrowseCapabilities), {
      value: capabilities => setState({capabilities, key}),
      error: e => setState({error: errorText(e, 'library', 'load'), key}),
    });
  }, [api, who, libraryId, pivot, key]);
  // Another library's vocabulary is never shown while this one's is on its way.
  return state.key === key ? {capabilities: state.capabilities, error: state.error} : {};
}

/**
 * Browse results as a windowed collection (scale rule: 5–10M items, client
 * cost O(visible)). Pages are fetched by offset (`range.start`) and pinned to
 * the first page's `revision`, so a long scroll reads one consistent snapshot;
 * `positionIndex` gives letter jumps without asking the server to seek.
 * Pages away from the viewport are evicted (≤ `maxResidentPages` × page size
 * items in memory). A filter or sort change starts a new generation while the
 * old results stay on screen until the new first page arrives.
 */
export const BROWSE_PAGE = 60;

type BrowseStore = {collection?: WindowedCollection<ContentEntry>; /** What a quiet read asks for: the filters and sort on screen now. */ source?: () => FetchPage<ContentEntry>; getSnapshot(): unknown; subscribe(listener: () => void): () => void; dispose(): void};
const idleStore = (): BrowseStore => ({getSnapshot: () => undefined, subscribe: () => () => {}, dispose: () => {}});

export function useBrowseCollection(input: {libraryId?: string; pivot?: string; /** What the list is of (a collection's titles, an album's songs). Always applied; the viewer's filters only narrow it. */ base?: readonly BrowseNode[]; predicates: readonly Predicate[]; sort?: BrowseSortSelection; enabled: boolean; pageSize?: number}) {
  const {api} = useSession();
  const {libraryId, pivot, base, predicates, sort, enabled} = input;
  const pageSize = Math.max(1, Math.min(BROWSE_PAGE, input.pageSize ?? BROWSE_PAGE));
  const query = JSON.stringify([predicates, sort]);
  // Each group is its own list: an album's songs are not the library's Songs tab.
  const baseKey = base?.length ? ':' + JSON.stringify(base) : '';
  const on = enabled && !!libraryId && !!pivot;
  // One collection per library and pivot, kept for the signed-in viewer: Back returns to the
  // same list at the same place without a request. A new query is a new generation of it.
  const latest = useRef<() => FetchPage<ContentEntry>>(undefined);
  latest.current = () => browsePageSource(api, {libraryId: libraryId!, pivot: pivot!, base, predicates, sort});
  const {service: store, document, memo} = usePooled<BrowseStore, unknown>(on ? `browse:${libraryId}:${pivot}:${pageSize}${baseKey}` : 'browse:idle', () => {
    if (!on) return {service: idleStore(), document: {dependencies: [], revalidate: async () => {}}};
    const collection = createWindowedCollection<ContentEntry>({fetchPage: latest.current!(), pageSize, maxResidentPages: 10, keyOf: e => e.id});
    const service: BrowseStore = {collection, getSnapshot: collection.getSnapshot, subscribe: collection.subscribe, dispose: collection.dispose};
    return {
      service,
      document: {
        // The titles, their order, and this profile's marks on them.
        // A list across libraries follows them all.
        dependencies: titlesWithPersonalState(libraryId === acrossLibraries ? undefined : libraryId),
        hasContent: () => collection.getSnapshot().total !== undefined,
        // The same list read again behind the one on screen (a new reading has a new revision), swapped in whole at the same place.
        revalidate: context => collection.revalidate({fetchPage: service.source?.(), hold: context.settle}),
      },
    };
  });
  const collection = on ? store.collection : undefined;
  if (on) store.source = () => latest.current!();
  useEffect(() => {
    if (!collection) return;
    if (memo.query === undefined) { memo.query = query; document.markCurrent(); return; }
    if (memo.query !== query) { memo.query = query; document.markCurrent(); collection.invalidate(latest.current!()); }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [collection, query]);
  /** Reload from the top (a new revision), keeping what's on screen until it arrives. */
  const refresh = useCallback(() => {
    if (!collection) return;
    document.markCurrent();
    collection.invalidate(latest.current!());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [collection, document]);
  return {collection, refresh};
}

/** The group a list across libraries is (its person, playlist, collection, Home row, My List, Favorites or history), so its filter choices count that list's own titles. */
export const FacetGroupContext = createContext<Readonly<{group: string; value: string}> | undefined>(undefined);

export function useFacets(libraryId: string | undefined) {
  const {api} = useSession();
  const facetGroup = useContext(FacetGroupContext);
  const groupField = facetGroup?.group, groupValue = facetGroup?.value;
  return useCallback(async (field: string, q = ''): Promise<readonly BrowseFacetValue[]> => {
    if (!libraryId) return [];
    const params = new URLSearchParams({field, limit: '100'});
    if (q) params.set('q', q);
    if (groupField && libraryId === acrossLibraries) { params.set('group', groupField); params.set('value', groupValue ?? ''); }
    const raw = await api.request<unknown>(`${browseFacetsPath(libraryId)}?${params}`, 'GET');
    return parseBrowseFacets(raw, field).values;
  }, [api, libraryId, groupField, groupValue]);
}

/** URL form: a JSON array of predicates, bounded so a link stays a link. */
export function encodePredicates(p: readonly Predicate[]): string | undefined {
  if (!p.length) return undefined;
  const s = JSON.stringify(p.map(x => [x.field, x.operator, x.value]));
  return s.length <= 2048 ? s : undefined;
}
export function decodePredicates(raw: string | undefined): Predicate[] {
  if (!raw) return [];
  try {
    const rows = JSON.parse(raw) as unknown;
    if (!Array.isArray(rows)) return [];
    return rows.flatMap(r => (Array.isArray(r) && r.length === 3 && typeof r[0] === 'string' && typeof r[1] === 'string' ? [{field: r[0], operator: r[1] as Predicate['operator'], value: r[2] as Predicate['value']}] : []));
  } catch { return []; }
}

/** Field and quick-filter names come from the catalogue (`filter.field.*`, `filter.quick.*`; US English, CON-19). */
function catalogueLabel(prefix: 'filter.quick.', ...keys: string[]): string | undefined {
  const i18n = currentI18n();
  for (const key of keys) { const id = prefix + key; if (i18n.has(id)) return i18n.t(id); }
  return undefined;
}
const sentence = (key: string) => key.replace(/([A-Z])/g, ' $1').toLowerCase().replace(/^./, c => c.toUpperCase());
/** A filter field's or a sort's name, from the catalogue by the shared rule (client-core `browseNameId`); an id the catalogue has no name for reads as words. */
export function fieldLabel(id: string, labelKey?: string): string {
  const i18n = currentI18n();
  const named = browseNameId(id, labelKey, messageId => i18n.has(messageId));
  if (named) return i18n.t(named as MessageId);
  return sentence((labelKey ?? '').replace(/^browse\.field\./, '').replace(/^sort\./, '') || id);
}

/**
 * Random order is drawn once and kept while the app is open: paging, Back and a quiet update all
 * read the same shuffle. Choosing Random again draws another.
 */
let shuffleSeed = Math.floor(Math.random() * 0x100000000) >>> 0;
const shuffleListeners = new Set<() => void>();
export function reshuffle(): void {
  shuffleSeed = Math.floor(Math.random() * 0x100000000) >>> 0;
  for (const listener of [...shuffleListeners]) listener();
}
const subscribeShuffle = (listener: () => void) => { shuffleListeners.add(listener); return () => { shuffleListeners.delete(listener); }; };
/** The sort as the server is asked for it: Random carries the current shuffle. */
export function useShuffled(sort: BrowseSortSelection): BrowseSortSelection {
  const seed = useSyncExternalStore(subscribeShuffle, () => shuffleSeed);
  return useMemo(() => (sort.field === 'random' ? {field: sort.field, direction: sort.direction, seed} : sort), [sort, seed]);
}

export {seeAllPredicates} from './see-all.ts';
export function quickFilterLabel(labelKey: string): string {
  const key = labelKey.replace(/^filter\./, '');
  return catalogueLabel('filter.quick.', key) ?? sentence(key);
}
export function predicateLabel(p: Predicate, capabilities?: BrowseCapabilities): string {
  return appliedFilterLabel(p, id => fieldLabel(id, capabilities?.fields.find(f => f.id === id)?.labelKey), currentI18n().t);
}
