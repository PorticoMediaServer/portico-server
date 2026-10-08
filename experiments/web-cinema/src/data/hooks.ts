import React, {createContext, useCallback, useContext, useEffect, useRef, useState, useSyncExternalStore} from 'react';
import type {Services} from './services';

export const ServicesContext = createContext<Services | null>(null);

export function useServices(): Services {
  const s = useContext(ServicesContext);
  if (!s) throw new Error('ServicesContext missing');
  return s;
}

/** Re-renders when any data changes (mutations bump the version). */
export function useDataVersion(): number {
  const s = useServices();
  return useSyncExternalStore(s.subscribe, s.version, s.version);
}

const cache = new Map<string, unknown>();

export type Query<T> = {data: T | undefined; loading: boolean; error: Error | null; reload: () => void};

/**
 * Loads data for a screen. Results are cached by key so returning to a screen is instant, and the
 * query re-runs quietly after any mutation so lists stay in sync (watchlist toggles, renames…).
 */
export function useQuery<T>(key: string | null, fn: () => Promise<T>): Query<T> {
  const version = useDataVersion();
  const [state, setState] = useState<{data: T | undefined; loading: boolean; error: Error | null; key: string | null}>(() => ({data: key ? (cache.get(key) as T | undefined) : undefined, loading: !!key && !cache.has(key), error: null, key}));
  const [nonce, setNonce] = useState(0);
  const fnRef = useRef(fn);
  fnRef.current = fn;
  useEffect(() => {
    if (!key) return;
    let alive = true;
    const cached = cache.get(key) as T | undefined;
    setState(s => ({data: s.key === key ? s.data ?? cached : cached, loading: cached === undefined, error: null, key}));
    fnRef.current().then(
      data => {
        if (!alive) return;
        cache.set(key, data);
        setState({data, loading: false, error: null, key});
      },
      (error: Error) => alive && setState(s => ({...s, loading: false, error})),
    );
    return () => {
      alive = false;
    };
  }, [key, version, nonce]);
  const reload = useCallback(() => {
    if (key) cache.delete(key);
    setNonce(n => n + 1);
  }, [key]);
  return {data: state.key === key ? state.data : (key ? (cache.get(key) as T | undefined) : undefined), loading: state.loading, error: state.error, reload};
}

/** Runs a mutation with busy state. */
export function useAction() {
  const [busy, setBusy] = useState(false);
  const run = useCallback(async <T,>(fn: () => Promise<T>): Promise<T | undefined> => {
    setBusy(true);
    try {
      return await fn();
    } finally {
      setBusy(false);
    }
  }, []);
  return {busy, run};
}

export function ServicesProvider({services, children}: {services: Services; children: React.ReactNode}) {
  return React.createElement(ServicesContext.Provider, {value: services}, children);
}
