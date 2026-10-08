import {createContext, useCallback, useContext, useEffect, useMemo, useState} from 'react';
import {errorText} from './errors';
import {onServerChanged} from './server-changes';
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
  useEffect(() => {
    if (!sessionKey) {
      setItems([]);
      return;
    }
    let active = true;
    setItems(readRailCache(sessionKey)?.libraries ?? []);
    setLoading(true);
    setError(undefined);
    api
      .libraries()
      .then(result => { if (active) { setItems(result.items); writeRailCache(sessionKey, {libraries: result.items}); } })
      .catch(e => active && setError(errorText(e, 'library', 'load')))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, [api, sessionKey, tick]);
  const reload = useCallback(() => setTick(t => t + 1), []);
  useEffect(() => onServerChanged(reload), [reload]);
  // PERF-28: a stable value, so the context re-renders its readers only when something changed.
  return useMemo(() => ({items, loading, error, reload}), [items, loading, error, reload]);
}

export const LibrariesContext = createContext<LibrariesState>({items: [], loading: false, reload: () => {}});
export const useLibrariesContext = () => useContext(LibrariesContext);

export function libraryKindLabel(kind: string): string {
  switch (kind) {
    case 'movie': return 'Movies';
    case 'tv': return 'TV Shows';
    case 'anime': return 'Anime';
    case 'music': return 'Music';
    case 'audiobook': return 'Audiobooks';
    case 'recordings': return 'Recordings';
    default: return 'Library';
  }
}
