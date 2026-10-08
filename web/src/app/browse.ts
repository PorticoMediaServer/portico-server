import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {currentI18n} from './i18n';
import {createWindowedCollection} from '@core/collections/index.ts';
import type {ContentEntry} from '@core/library-content.ts';
import {parseBrowseCapabilities, parseBrowseFacets, type BrowseCapabilities, type BrowseFacetValue, type BrowseSortSelection} from '@core/browse.ts';
import {appliedFilterLabel} from '@core/presentation/index.ts';
import type {MessageId} from '@i18n';
import {sessionIdentity, useSession} from './session';
import {errorText} from './errors';
import {browsePageSource, type Predicate} from './browse-page-source.ts';
export {browsePageSource} from './browse-page-source.ts';
export type {Predicate} from './browse-page-source.ts';

/**
 * Library browsing on the server's expression engine. The server publishes the
 * vocabulary (fields, operators, sorts, quick filters) per library and pivot;
 * this hook only assembles requests from it and pages results. Filters live in
 * the URL as a compact JSON list of predicates so a view is linkable.
 */

const capabilityCache = new Map<string, Promise<BrowseCapabilities>>();
export function useBrowseCapabilities(libraryId: string | undefined, pivot: string | undefined) {
  const {api, session} = useSession();
  const [state, setState] = useState<{capabilities?: BrowseCapabilities; error?: string}>({});
  const who = sessionIdentity(session);
  useEffect(() => {
    if (!libraryId || !pivot) { setState({}); return; }
    const key = `${who}:${libraryId}:${pivot}`;
    let promise = capabilityCache.get(key);
    if (!promise) {
      promise = api.request<unknown>(`/v1/libraries/${encodeURIComponent(libraryId)}/browse-capabilities?pivot=${encodeURIComponent(pivot)}`, 'GET').then(parseBrowseCapabilities);
      capabilityCache.set(key, promise);
      promise.catch(() => capabilityCache.delete(key));
    }
    let cancelled = false;
    promise.then(c => { if (!cancelled) setState({capabilities: c}); }, e => { if (!cancelled) setState({error: errorText(e, 'library', 'load')}); });
    return () => { cancelled = true; };
  }, [api, who, libraryId, pivot]);
  return state;
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

export function useBrowseCollection(input: {libraryId?: string; pivot?: string; predicates: readonly Predicate[]; sort?: BrowseSortSelection; enabled: boolean; pageSize?: number}) {
  const {api} = useSession();
  const {libraryId, pivot, predicates, sort, enabled} = input;
  const pageSize = Math.max(1, Math.min(BROWSE_PAGE, input.pageSize ?? BROWSE_PAGE));
  const query = JSON.stringify([predicates, sort]);
  // One collection per library and pivot; a new query is a new generation of it.
  const collection = useMemo(() => (enabled && libraryId && pivot ? createWindowedCollection<ContentEntry>({fetchPage: browsePageSource(api, {libraryId, pivot, predicates, sort}), pageSize, maxResidentPages: 10, keyOf: e => e.id}) : undefined),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [api, libraryId, pivot, enabled, pageSize]);
  const shown = useRef({collection, query});
  useEffect(() => {
    if (!collection) return;
    if (shown.current.collection === collection && shown.current.query !== query) collection.invalidate(browsePageSource(api, {libraryId: libraryId!, pivot: pivot!, predicates, sort}));
    shown.current = {collection, query};
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [collection, query]);
  useEffect(() => () => collection?.dispose(), [collection]);
  /** Reload from the top (a new revision), keeping what's on screen until it arrives. */
  const refresh = useCallback(() => {
    if (collection && libraryId && pivot) collection.invalidate(browsePageSource(api, {libraryId, pivot, predicates, sort}));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [collection, api, libraryId, pivot, query]);
  return {collection, refresh};
}

export function useFacets(libraryId: string | undefined) {
  const {api} = useSession();
  return useCallback(async (field: string, q = ''): Promise<readonly BrowseFacetValue[]> => {
    if (!libraryId) return [];
    const params = new URLSearchParams({field, limit: '100'});
    if (q) params.set('q', q);
    const raw = await api.request<unknown>(`/v1/libraries/${encodeURIComponent(libraryId)}/facets?${params}`, 'GET');
    return parseBrowseFacets(raw, field).values;
  }, [api, libraryId]);
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
function catalogueLabel(prefix: 'filter.field.' | 'filter.quick.', ...keys: string[]): string | undefined {
  const i18n = currentI18n();
  for (const key of keys) { const id = prefix + key; if (i18n.has(id)) return i18n.t(id); }
  return undefined;
}
const sentence = (key: string) => key.replace(/([A-Z])/g, ' $1').toLowerCase().replace(/^./, c => c.toUpperCase());
/** Sorts that are not fields name themselves (`sort.forYou`: the profile's taste ranks the list). */
const OWN_SORT_LABELS: ReadonlySet<string> = new Set(['sort.forYou']);
export function fieldLabel(id: string, labelKey?: string): string {
  if (labelKey && OWN_SORT_LABELS.has(labelKey) && currentI18n().has(labelKey)) return currentI18n().t(labelKey as MessageId);
  const key = (labelKey ?? '').replace(/^browse\.field\./, '').replace(/^sort\./, '') || id;
  return catalogueLabel('filter.field.', key, id) ?? sentence(key);
}

export {seeAllPredicates} from './see-all.ts';
export function quickFilterLabel(labelKey: string): string {
  const key = labelKey.replace(/^filter\./, '');
  return catalogueLabel('filter.quick.', key) ?? sentence(key);
}
export function predicateLabel(p: Predicate, capabilities?: BrowseCapabilities): string {
  return appliedFilterLabel(p, id => fieldLabel(id, capabilities?.fields.find(f => f.id === id)?.labelKey), currentI18n().t);
}
