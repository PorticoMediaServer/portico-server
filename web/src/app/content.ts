import {cardCaption, continueCaption} from '@core/presentation/index.ts';
import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {LibraryContentService, type ContentRoute, type LibraryContentSnapshot} from '@core/library-content.ts';
import {compatContentApi} from '@core/presentation/index.ts';
import {currentI18n} from './i18n';
import {useSession} from './session';
import {useViewerScope} from './viewer-scope';
import {RECOMMENDATIONS_RESET, recommendationViewer} from './not-interested';
import {browseCountLabel as countLabel, isQuickFilterVisible as quickVisible, kindAwareQuickKey as quickKey, kindCaptionFor as kindCaption, libraryKindSingular as kindSingular, nounPivotFor as nounPivot, quickFilterLabelForKind as quickLabel, seeAllTarget as seeAll, type T as WordsT} from './library-words';

/** The catalogue `t` accepts message ids; the word helpers take plain strings. */
const wordsT = () => currentI18n().t as unknown as WordsT;

/** Binds a core service with subscribe/getSnapshot to React and disposes it with the owner. */
export function useService<S extends {subscribe: (l: () => void) => () => void; getSnapshot: () => T; dispose?: () => void}, T>(factory: () => S, deps: readonly unknown[]): {service: S; snapshot: T} {
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const service = useMemo(factory, deps);
  const [snapshot, setSnapshot] = useState<T>(() => service.getSnapshot());
  const pendingDispose = useRef<{timer: ReturnType<typeof setTimeout>; service: S}>(undefined);
  useEffect(() => {
    // Only a replay of the same service (StrictMode, Fast Refresh) cancels its disposal; a
    // replaced service is still disposed, so its in-flight work ends at the ownership change.
    if (pendingDispose.current?.service === service) clearTimeout(pendingDispose.current.timer);
    pendingDispose.current = undefined;
    setSnapshot(service.getSnapshot());
    const unsubscribe = service.subscribe(() => setSnapshot(service.getSnapshot()));
    return () => {
      unsubscribe();
      // Deferred so a synchronous effect replay (StrictMode, Fast Refresh) reuses the live service.
      pendingDispose.current = {timer: setTimeout(() => service.dispose?.(), 0), service};
    };
  }, [service]);
  return {service, snapshot};
}

/**
 * One server-authored content projection (Home, a library pivot, an entity
 * page). The service owns fetching, paging, fences and stale detection; this
 * hook only binds it to React and the current viewer.
 */
export function useContent(route: ContentRoute | null) {
  const {api} = useSession();
  const scope = useViewerScope();
  const {service, snapshot} = useService<LibraryContentService, LibraryContentSnapshot>(() => new LibraryContentService({api: compatContentApi(api) as typeof api, scope}), [api, scope]);
  const key = route ? JSON.stringify(route) : '';
  const last = useRef<{key: string; service: unknown}>({key: '', service: null});
  useEffect(() => {
    if (!route) return;
    if (last.current.key === key && last.current.service === service) return;
    last.current = {key, service};
    try { void service.select(route).catch(e => { if (import.meta.env.DEV) console.error('[content] select failed', e); }); } catch (e) { if (import.meta.env.DEV) console.error('[content] select rejected', e); }
  }, [service, key, route]);
  const refresh = useCallback(() => void service.refresh().catch(() => {}), [service]);
  useEffect(() => {
    if (route?.view !== 'discover') return;
    const viewer = recommendationViewer(scope);
    const onReset = (event: Event) => { if ((event as CustomEvent<{viewer: string}>).detail.viewer === viewer) refresh(); };
    window.addEventListener(RECOMMENDATIONS_RESET, onReset);
    return () => window.removeEventListener(RECOMMENDATIONS_RESET, onReset);
  }, [route?.view, scope, refresh]);
  const retry = useCallback(() => void service.retry().catch(() => {}), [service]);
  const next = useCallback((sectionId: string) => void service.next(sectionId).catch(() => {}), [service]);
  return {snapshot, refresh, retry, next, service};
}

/**
 * WEB-LIB-03: nouns by library kind — runtime wrappers over `./library-words`
 * (the pure module the unit tests exercise). The summary noun follows the
 * pivot ("10 artists", "80 movies", "12 books"); captions are kind-aware,
 * never "0 items". US English throughout ("Favorites").
 */

export type LibraryKind = 'movie' | 'tv' | 'anime' | 'music' | 'audiobook' | string;

export const nounPivotFor = nounPivot;
export const libraryKindSingular = kindSingular;
export const kindAwareQuickKey = quickKey;
export const isQuickFilterVisible = quickVisible;
export const seeAllTarget = seeAll;

/** "10 artists", "80 movies", "12 books" — via the shared `lib.count` catalogue. */
export function browseCountLabel(count: number, pivot: string | undefined, libraryKind?: string): string {
  return countLabel(wordsT(), count, pivot, libraryKind);
}

/** Consumer label for a server quick filter in this library kind (US English). */
export function quickFilterLabelForKind(labelKey: string, libraryKind?: string): string {
  return quickLabel(wordsT(), labelKey, libraryKind);
}

type CaptionEntry = {kind: string; subtitle?: string; count?: number; duration?: number};

/** Kind-aware secondary caption, never "0 items" (see `./library-words`). */
export function kindCaptionFor(entry: CaptionEntry): string | undefined {
  return kindCaption(wordsT(), entry);
}

/** The line under a card's title: the shared rule (client-core `card-caption.ts`) in the viewer's language. */
/** A Continue Watching card's caption ("S1 E3 · 24 min left"). */
export function continueCaptionOf(entry: Parameters<typeof continueCaption>[0]): string | undefined {
  return continueCaption(entry, currentI18n().t);
}

export function cardCaptionOf(entry: Parameters<typeof cardCaption>[0]): string | undefined {
  const i18n = currentI18n();
  return cardCaption(entry, i18n.t, {locale: i18n.locale});
}
