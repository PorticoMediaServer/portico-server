import {useCallback, useEffect, useRef, useState} from 'react';
import {DetailService, parseCreditPage, type DetailCredit, type DetailSnapshot, type DetailTarget, type PersonalIntent} from '@core/detail.ts';
import {fetchItemRecommendations, fetchShowRecommendations, type ItemRecommendations, type ShowRecommendations} from '@core/recommendations.ts';
import {ShowWorkspaceService, type ShowWorkspaceRoute, type ShowWorkspaceSnapshot} from '@core/show-workspace.ts';
import {useService} from './content';
import {compatContentApi} from '@core/presentation/index.ts';
import {useSession} from './session';
import {useViewerScope} from './viewer-scope';
import {RECOMMENDATIONS_RESET, recommendationViewer} from './not-interested';

export const requestId = () => Promise.resolve(crypto.randomUUID());

/** Window event after an accepted Watchlist, Favorites or watched change: `{itemId, action, value}`. */
export const PERSONAL_CHANGE = 'portico:personal-change';

/** One media item's detail projection plus personal-state mutations. */
export function useDetail(target: DetailTarget | null) {
  const {api} = useSession();
  const scope = useViewerScope();
  const {service, snapshot} = useService<DetailService, DetailSnapshot>(() => new DetailService({api, scope, requestId}), [api, scope]);
  const key = target ? target.libraryId + '/' + target.itemId : '';
  const last = useRef<{key: string; service: unknown}>({key: '', service: null});
  useEffect(() => {
    if (!target || (last.current.key === key && last.current.service === service)) return;
    last.current = {key, service};
    try { void service.select(target).catch(e => { if (import.meta.env.DEV) console.error('[detail] select failed', e); }); } catch (e) { if (import.meta.env.DEV) console.error('[detail] select rejected', e); }
  }, [service, key, target]);
  // WEB-SAVED-04: tell the rest of the app (Saved listens) once a Watchlist,
  // Favorites or watched change has been accepted by the server.
  // WEB-DETAIL-02: `mutate` validates synchronously; a throw must still end in a rejected promise.
  const mutate = useCallback((intent: PersonalIntent) => void Promise.resolve().then(() => service.mutate(intent)).then(() => {
    const snap = service.getSnapshot();
    if (snap.mutationError || !('value' in intent) || (intent.action !== 'watchlist' && intent.action !== 'favorite' && intent.action !== 'watched')) return;
    const itemId = snap.data?.item.id ?? target?.itemId;
    if (typeof window !== 'undefined' && itemId) window.dispatchEvent(new CustomEvent(PERSONAL_CHANGE, {detail: {itemId, action: intent.action, value: intent.value}}));
  }).catch(() => {}), [service, target?.itemId]);
  const retry = useCallback(() => void service.retryRead().catch(() => {}), [service]);
  const refresh = useCallback(() => void service.refresh().catch(() => {}), [service]);
  const retryMutation = useCallback(() => void service.retryMutation().catch(() => {}), [service]);
  const dismissMutation = useCallback(() => service.dismissMutation(), [service]);
  return {snapshot, mutate, retry, refresh, retryMutation, dismissMutation};
}

/** A show workspace: show identity, seasons, and the selected season's episodes. */
export function useShowWorkspace(route: ShowWorkspaceRoute | null) {
  const {api} = useSession();
  const scope = useViewerScope();
  const {service, snapshot} = useService<ShowWorkspaceService, ShowWorkspaceSnapshot>(() => new ShowWorkspaceService({api: compatContentApi(api) as typeof api, scope}), [api, scope]);
  const key = route ? JSON.stringify(route) : '';
  const last = useRef<{key: string; service: unknown}>({key: '', service: null});
  useEffect(() => {
    if (!route || (last.current.key === key && last.current.service === service)) return;
    last.current = {key, service};
    try { void service.select(route).catch(e => { if (import.meta.env.DEV) console.error('[content] select failed', e); }); } catch (e) { if (import.meta.env.DEV) console.error('[content] select rejected', e); }
  }, [service, key, route]);
  const selectSeason = useCallback((id: string) => void service.selectSeason(id).catch(() => {}), [service]);
  const selectGroup = useCallback(() => void service.selectGroup('unassigned_absolute').catch(() => {}), [service]);
  // The shared workspace serves one bounded page at a time and keeps its own
  // history, so the screen needs both directions — and the season list pages
  // the same way. Exposing only `next` is what made earlier episodes and later
  // seasons unreachable.
  const next = useCallback(() => void service.nextEpisodes().catch(() => {}), [service]);
  const previous = useCallback(() => void service.previousEpisodes().catch(() => {}), [service]);
  const nextSeasons = useCallback(() => void service.nextSeasons().catch(() => {}), [service]);
  const previousSeasons = useCallback(() => void service.previousSeasons().catch(() => {}), [service]);
  const retry = useCallback(() => void service.retry().catch(() => {}), [service]);
  return {snapshot, selectSeason, selectGroup, next, previous, nextSeasons, previousSeasons, retry};
}

/**
 * M32: one bounded "More like this" row for a movie, from
 * `GET /v1/items/{id}/recommendations?limit=12` (12 entries per row, the
 * server's row budget). Movies only; other kinds stay idle, and failures expose a retryable
 * error while the screen keeps its detail.related rows.
 */
/** `limit` on the recommendations route is entries per row, not rows. */
const MORE_LIKE_THIS_ENTRIES = 12;

type RecommendationRead<T> = {phase: 'idle' | 'loading' | 'ready' | 'error'; data: T | null; error?: unknown; retry: () => void};

function useRecommendationRead<T>(key: string, read: (signal: AbortSignal) => Promise<T>): RecommendationRead<T> {
  const viewer = recommendationViewer(useViewerScope());
  const boundKey = key ? `${viewer}/${key}` : '';
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState<{key: string; phase: RecommendationRead<T>['phase']; data: T | null; error?: unknown}>({key: '', phase: 'idle', data: null});
  const retry = useCallback(() => setAttempt(value => value + 1), []);
  useEffect(() => {
    if (!boundKey) { setState({key: boundKey, phase: 'idle', data: null}); return; }
    let live = true;
    const controller = new AbortController();
    setState({key: boundKey, phase: 'loading', data: null});
    read(controller.signal).then(data => { if (live) setState({key: boundKey, phase: 'ready', data}); }, error => { if (live) setState({key: boundKey, phase: 'error', data: null, error}); });
    return () => { live = false; controller.abort(); };
  }, [boundKey, read, attempt]);
  useEffect(() => {
    const onReset = (event: Event) => { if ((event as CustomEvent<{viewer: string}>).detail.viewer === viewer) retry(); };
    window.addEventListener(RECOMMENDATIONS_RESET, onReset);
    return () => window.removeEventListener(RECOMMENDATIONS_RESET, onReset);
  }, [viewer, retry]);
  return state.key === boundKey ? {...state, retry} : {phase: boundKey ? 'loading' : 'idle', data: null, retry};
}

export function useItemRecommendations(target: {libraryId: string; itemId: string; kind?: string} | null): RecommendationRead<ItemRecommendations> {
  const {api} = useSession();
  const itemId = target?.kind === 'movie' ? target.itemId : undefined;
  const libraryId = target?.kind === 'movie' ? target.libraryId : undefined;
  const read = useCallback((signal: AbortSignal) => fetchItemRecommendations(api, itemId!, MORE_LIKE_THIS_ENTRIES, signal), [api, itemId]);
  return useRecommendationRead(libraryId && itemId ? `${libraryId}/${itemId}` : '', read);
}

/**
 * P4/P7: a show page's recommendation rows (`GET /v1/shows/{id}/recommendations`): More like the
 * show, From its creator… Failures remain distinct from a successful empty result.
 */
export function useShowRecommendations(showId: string | undefined): RecommendationRead<ShowRecommendations> {
  const {api} = useSession();
  const read = useCallback((signal: AbortSignal) => fetchShowRecommendations(api, showId!, MORE_LIKE_THIS_ENTRIES, signal), [api, showId]);
  return useRecommendationRead(showId && showId !== 'season' ? showId : '', read);
}

/**
 * A title's cast or crew, complete: it starts from what the detail carried
 * (`initial`) and, once `more()` is asked for, pages the whole group from
 * GET /v1/items/{id}/credits in billing order until `total` is reached. The
 * first fetched page replaces the initial slice (the same rows lead both).
 */
export function useCreditPages(itemId: string | undefined, group: 'cast' | 'crew', initial: readonly DetailCredit[], total: number) {
  const {api} = useSession();
  const [state, setState] = useState<{itemId?: string; credits: readonly DetailCredit[]; cursor?: string; started: boolean}>({credits: [], started: false});
  const busy = useRef(false);
  useEffect(() => { setState({itemId, credits: [], started: false}); busy.current = false; }, [itemId, group]);
  const current = state.itemId === itemId && state.started ? state.credits : initial;
  const done = state.itemId === itemId && state.started ? !state.cursor : initial.length >= total;
  const more = useCallback(() => {
    if (!itemId || busy.current || done) return;
    busy.current = true;
    const cursor = state.itemId === itemId && state.started ? state.cursor : undefined;
    const path = `/v1/items/${encodeURIComponent(itemId)}/credits?group=${group}` + (cursor ? `&cursor=${encodeURIComponent(cursor)}` : '');
    api.request<unknown>(path, 'GET').then(raw => {
      const page = parseCreditPage(raw, itemId, group);
      setState(prev => prev.itemId !== itemId ? prev : {itemId, credits: [...(prev.started ? prev.credits : []), ...page.credits], cursor: page.nextCursor, started: true});
    }, () => {}).finally(() => { busy.current = false; });
  }, [api, itemId, group, done, state]);
  return {credits: current, more, done};
}
