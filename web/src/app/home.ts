import {useCallback, useEffect, useMemo} from 'react';
import {type HomeDocument, type HomeLayout, type HomeLayoutView, type HomeRow} from '@core/home.ts';
import {HomeService, type HomeSnapshot} from '@core/home-service.ts';
import {dependsOn, titlesWithPersonalState} from '@core/freshness-hub.ts';
import {serviceDocument} from '@core/freshness-pool.ts';
import {useSession} from './session';
import {useViewerScope} from './viewer-scope';
import {usePooled, useRecovering} from './freshness';
import {RECOMMENDATIONS_RESET, recommendationViewer} from './not-interested';
import {currentI18n} from './i18n';
import type {MessageId} from '@i18n';
import {serverTextLabel} from '@core/presentation/index.ts';
import type {ServerText} from '@core/home.ts';
import type {HomeRowPager} from '@core/home-row-page.ts';

/**
 * Home on the rows API: one document with every row the server composed for
 * this viewer (order and hidden set applied server-side), per-row paging by
 * cursor, and a layout the viewer can reorder or hide with a revision fence.
 *
 * The document is client-core's `HomeService`, kept for the signed-in viewer: coming back to
 * Home shows it at once, and it is read again in the background only when a library, the list of
 * libraries, or this profile's watched state or positions moved. What changed is merged into
 * the rows on screen; nothing is blanked and no row loses its place.
 */
export type HomeState = {phase: 'loading' | 'ready' | 'error'; document?: HomeDocument; error?: unknown; paging: ReadonlySet<string>};

/** Everything Home is built from: every library's titles and order, the list of libraries, and this profile's state. */
// Trending and Popular move with the server, not this profile: their own hint.
const homeDependencies = Object.freeze([dependsOn.libraries, dependsOn.list('recommendations'), ...titlesWithPersonalState()]);
const operationKey = () => crypto.randomUUID();

export function useHome() {
  const {api} = useSession();
  const viewer = recommendationViewer(useViewerScope());
  const {service, snapshot, document, memo} = usePooled<HomeService, HomeSnapshot>('home', () => serviceDocument(new HomeService(api), homeDependencies));
  useEffect(() => {
    if (memo.selected) return;
    memo.selected = true;
    document.markCurrent();
    void service.load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [service]);
  /** An explicit refresh. `clear` first drops what is shown (this profile's taste was reset). */
  const load = useCallback((clear = false, _invalidate = false) => {
    document.markCurrent();
    return service.load({clear: clear === true});
  }, [service, document]);
  useEffect(() => {
    const onReset = (event: Event) => { if ((event as CustomEvent<{viewer: string}>).detail.viewer === viewer) void load(true); };
    window.addEventListener(RECOMMENDATIONS_RESET, onReset);
    return () => window.removeEventListener(RECOMMENDATIONS_RESET, onReset);
  }, [viewer, load]);
  const more = useCallback((rowId: string) => service.more(rowId), [service]);
  const saveLayout = useCallback((rowOrder: readonly string[], hiddenRowIds: readonly string[], expectedRevision?: number): Promise<HomeLayout> => service.saveLayout(rowOrder, hiddenRowIds, operationKey, expectedRevision), [service]);
  const resetLayout = useCallback(() => service.resetLayout(operationKey), [service]);
  const loadLayout = useCallback((signal?: AbortSignal): Promise<HomeLayoutView> => service.loadLayout(signal), [service]);
  // A first load that failed is tried again by itself; for its first moments Home goes on showing that it is loading.
  const recovering = useRecovering(document);
  const state = useMemo<HomeState>(() => {
    const quiet = recovering && snapshot.phase === 'error' && !snapshot.document;
    return {phase: snapshot.phase === 'idle' || quiet ? 'loading' : snapshot.phase, document: snapshot.document, error: quiet ? undefined : snapshot.error, paging: snapshot.paging};
  }, [snapshot, recovering]);
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

/** A failed first read has no cursor to continue; only a later page can read more. */
export const retryHomeRow = (pager: Pick<HomeRowPager, 'load' | 'more'>, nextCursor: string): Promise<void> => nextCursor ? pager.more() : pager.load();
