import {createContext, useCallback, useContext, useEffect, useMemo, useRef, useState} from 'react';
import {errorText} from './errors';
import {currentI18n, type I18n} from './i18n';
import {onServerChanged} from './server-changes';
import {useFreshness} from './freshness';
import {dependsOn} from '@core/freshness-hub.ts';
import {readRailCache, writeRailCache} from './rail-cache';
import type {HttpLocalApi, Library} from '@core/index.ts';

export type LibrariesState = {items: readonly Library[]; loading: boolean; error?: string; reload: () => void};

/** Authorized libraries for the selected viewer. Refetched on session change; the rail shows the
 * viewer's last list from this browser until the read lands, so it never assembles in steps. */
export function useLibraries(api: HttpLocalApi, sessionKey: string | undefined): LibrariesState {
  const [items, setItems] = useState<readonly Library[]>(() => readRailCache(sessionKey)?.libraries ?? []);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>();
  const [tick, setTick] = useState(0);
  const shownFor = useRef<string | undefined>(undefined);
  const readFor = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (!sessionKey) {
      shownFor.current = undefined; readFor.current = undefined;
      setItems([]);
      return;
    }
    let active = true;
    // The first read for this viewer starts from what this browser last saw; a later read keeps what is on screen.
    if (shownFor.current !== sessionKey) { shownFor.current = sessionKey; setItems(readRailCache(sessionKey)?.libraries ?? []); setLoading(true); }
    setError(undefined);
    api
      .libraries()
      .then(result => { if (active) { readFor.current = sessionKey; setItems(result.items); writeRailCache(sessionKey, {libraries: result.items}); } })
      // Once the list has been read, a later read that fails leaves it as it is and says nothing.
      .catch(e => active && readFor.current !== sessionKey && setError(errorText(e, 'library', 'load')))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, [api, sessionKey, tick]);
  const reload = useCallback(() => setTick(t => t + 1), []);
  useEffect(() => onServerChanged(reload), [reload]);
  // A library added, removed or renamed anywhere reaches the rail by itself: the list is read
  // again behind the one on screen, which stays until the answer is in.
  const {hub} = useFreshness();
  useEffect(() => hub.watch([dependsOn.libraries], reload), [hub, reload]);
  // PERF-28: a stable value, so the context re-renders its readers only when something changed.
  return useMemo(() => ({items, loading, error, reload}), [items, loading, error, reload]);
}

export const LibrariesContext = createContext<LibrariesState>({items: [], loading: false, reload: () => {}});
export const useLibrariesContext = () => useContext(LibrariesContext);

const LIBRARY_KINDS = new Set(['movie', 'tv', 'anime', 'music', 'audiobook', 'recordings']);
/**
 * The noun for a kind of library ("Movies", "TV Shows"), in the viewer's language. A component
 * passes the `t` it reads from `useI18n()`; without one the active catalogue is used.
 */
export function libraryKindLabel(kind: string, t: I18n['t'] = currentI18n().t): string {
  return t('web.libraryKind.plural', {kind: LIBRARY_KINDS.has(kind) ? kind : 'other'});
}
