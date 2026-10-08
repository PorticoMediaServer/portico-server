import {useCallback, useEffect, useRef, useState} from 'react';
import {fetchHome, fetchHomeLayout, fetchHomeRow, appendHomeRowPage, resetHomeLayout, saveHomeLayout, type HomeDocument, type HomeLayout, type HomeLayoutView, type HomeRow} from '@core/home.ts';
import {sessionIdentity, useSession} from './session';
import {useViewerScope} from './viewer-scope';
import {RECOMMENDATIONS_RESET, recommendationViewer} from './not-interested';
import {currentI18n} from './i18n';
import type {MessageId} from '@i18n';
import {serverTextLabel} from '@core/presentation/index.ts';
import type {ServerText} from '@core/home.ts';

/**
 * Home on the rows API: one document with every row the server composed for
 * this viewer (order and hidden set applied server-side), per-row paging by
 * cursor, and a layout the viewer can reorder or hide with a revision fence.
 */
export type HomeState = {phase: 'loading' | 'ready' | 'error'; document?: HomeDocument; error?: unknown; paging: ReadonlySet<string>};

export function useHome() {
  const {api, session} = useSession();
  const viewer = recommendationViewer(useViewerScope());
  const [state, setState] = useState<HomeState>({phase: 'loading', paging: new Set()});
  const generation = useRef(0);
  const inflight = useRef<AbortController | undefined>(undefined);
  const pages = useRef(new Set<AbortController>());
  const load = useCallback(async (clear = false) => {
    const mine = ++generation.current;
    inflight.current?.abort();
    for (const page of pages.current) page.abort();
    pages.current.clear();
    const controller = new AbortController();
    inflight.current = controller;
    setState(prev => ({...prev, ...(clear ? {document: undefined} : {}), phase: 'loading', paging: new Set(), error: undefined}));
    try {
      const document = await fetchHome(api, undefined, controller.signal);
      if (mine !== generation.current) return;
      setState({phase: 'ready', document, paging: new Set()});
    } catch (e) {
      if (mine !== generation.current) return;
      // Bind the error before the updater runs: Hermes does not keep a catch
      // binding alive for closures created inside an async function.
      const error: unknown = e;
      setState(prev => ({...prev, phase: 'error', error}));
    }
  }, [api]);
  const who = sessionIdentity(session);
  useEffect(() => { void load(); return () => { generation.current++; inflight.current?.abort(); for (const page of pages.current) page.abort(); pages.current.clear(); }; }, [load, who]);
  useEffect(() => {
    const onReset = (event: Event) => { if ((event as CustomEvent<{viewer: string}>).detail.viewer === viewer) void load(true); };
    window.addEventListener(RECOMMENDATIONS_RESET, onReset);
    return () => window.removeEventListener(RECOMMENDATIONS_RESET, onReset);
  }, [viewer, load]);
  const more = useCallback(async (rowId: string) => {
    const row = state.document?.rows.find(r => r.id === rowId);
    if (!row?.nextCursor || state.paging.has(rowId)) return;
    const mine = generation.current;
    const controller = new AbortController();
    pages.current.add(controller);
    setState(prev => ({...prev, paging: new Set(prev.paging).add(rowId)}));
    try {
      const page = await fetchHomeRow(api, rowId, {cursor: row.nextCursor}, controller.signal);
      if (mine !== generation.current || controller.signal.aborted) return;
      setState(prev => {
        if (mine !== generation.current || !prev.document || prev.document.rows.find(r => r.id === rowId)?.nextCursor !== row.nextCursor) return prev;
        const rows = prev.document.rows.map(r => (r.id === rowId ? appendHomeRowPage(r, page) : r));
        const paging = new Set(prev.paging); paging.delete(rowId);
        return {...prev, phase: 'ready', error: undefined, document: {...prev.document, rows}, paging};
      });
    } catch (error) {
      if (mine !== generation.current || controller.signal.aborted) return;
      if ((error as {code?: string})?.code === 'stale_continuation') { void load(true); return; }
      setState(prev => { if (mine !== generation.current || !prev.document) return prev; const paging = new Set(prev.paging); paging.delete(rowId); return {...prev, phase: 'error', paging, error}; });
    } finally {
      pages.current.delete(controller);
    }
  }, [api, state.document, state.paging, load]);
  const saveLayout = useCallback(async (rowOrder: readonly string[], hiddenRowIds: readonly string[], expectedRevision?: number): Promise<HomeLayout> => {
    const current = state.document?.layout;
    const revision = expectedRevision ?? current?.revision;
    if (revision === undefined) throw new Error('Home is not loaded.');
    const layout = await saveHomeLayout(api, {expectedRevision: revision, rowOrder, hiddenRowIds, idempotencyKey: crypto.randomUUID()});
    await load();
    return layout;
  }, [api, state.document, load]);
  const resetLayout = useCallback(async () => { await resetHomeLayout(api, crypto.randomUUID()); await load(); }, [api, load]);
  const loadLayout = useCallback(async (signal?: AbortSignal): Promise<HomeLayoutView> => fetchHomeLayout(api, signal), [api]);
  return {state, refresh: load, more, saveLayout, resetLayout, loadLayout};
}

export type {HomeRow};

/**
 * CON-19/CON-23 (client side): a Home row's name. The row's `titleText` comes first (the
 * catalogue message for its code, with its parameters: "Drama for you", "More with Ada
 * Lovelace"); a row without one (an older server, a layout row listed only by id) is named from
 * the catalogue by row id, so rows read in sentence case and a hidden row still has its real
 * name. The server's title is the fallback for rows the catalogue doesn't know.
 */
const ROW_TITLES: Record<string, MessageId> = {
  continue: 'home.row.continueWatching',
  continue_listening: 'home.row.continueListening',
  ondeck: 'home.row.upNext',
  recommended: 'home.row.recommended',
  trending_now: 'home.row.trending',
  community_watching: 'home.row.popularOnServer',
};
export function homeRowTitle(id: string, serverTitle: string | undefined, libraryName: (libraryId: string) => string | undefined, titleText?: ServerText): string {
  const i18n = currentI18n();
  if (titleText?.code && i18n.has(titleText.code)) {
    const named = serverTextLabel(i18n, titleText);
    if (named) return named;
  }
  const known = ROW_TITLES[id];
  if (known) return i18n.t(known);
  if (id.startsWith('recent_')) {
    const name = libraryName(id.slice('recent_'.length));
    if (name) return i18n.t('home.row.recentlyAddedIn', {library: name});
    return serverTitle ?? i18n.t('home.row.recentlyAdded');
  }
  if (serverTitle) return serverTitle;
  const words = id.replace(/[_-]+/g, ' ').trim();
  return words.charAt(0).toUpperCase() + words.slice(1);
}
