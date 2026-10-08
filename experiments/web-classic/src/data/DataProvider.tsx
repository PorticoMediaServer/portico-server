import React, {createContext, useCallback, useContext, useEffect, useRef, useState} from 'react';
import type {PorticoClient, Topic} from './services';
import type {AdminApi} from './adminApi';

type Ctx = {client: PorticoClient; admin: AdminApi};
const DataContext = createContext<Ctx | null>(null);

/** Builds the service layer. This is the one place that knows about fixtures. */
async function createServices(): Promise<Ctx> {
  const [{createFixtureClient}, {createAdminFixtures}] = await Promise.all([import('./fixtures/client'), import('./fixtures/admin')]);
  const client = createFixtureClient();
  return {client, admin: createAdminFixtures(client)};
}

/**
 * Supplies the service layer. Swapping fixtures for real Portico services
 * means changing `createServices` above; screens don't change. The fixture
 * catalogue is its own chunk, loaded behind the start-up screen.
 */
export function DataProvider({children, fallback}: {children: React.ReactNode; fallback: React.ReactNode}) {
  const [value, setValue] = useState<Ctx>();
  useEffect(() => { void createServices().then(setValue); }, []);
  if (!value) return <>{fallback}</>;
  return <DataContext.Provider value={value}>{children}</DataContext.Provider>;
}

export function useClient(): PorticoClient {
  return useContext(DataContext)!.client;
}
export function useAdmin(): AdminApi {
  return useContext(DataContext)!.admin;
}

/** Re-renders when any of the topics change. */
export function useTopic(topics: Topic[]): number {
  const client = useClient();
  const [n, setN] = useState(0);
  const key = topics.join(',');
  useEffect(() => {
    const offs = key.split(',').map(t => client.subscribe(t as Topic, () => setN(x => x + 1)));
    return () => offs.forEach(off => off());
  }, [client, key]);
  return n;
}

export type Query<T> = {data?: T; error?: string; loading: boolean; refresh: () => void};

/**
 * Loads data and refetches when `topics` change. Keeps showing the previous
 * data while refetching, so pages don't flash back to skeletons.
 */
export function useQuery<T>(fn: (client: PorticoClient) => Promise<T>, deps: readonly unknown[], topics: Topic[] = []): Query<T> {
  const client = useClient();
  const [state, setState] = useState<{data?: T; error?: string; loading: boolean}>({loading: true});
  const seq = useRef(0);
  const fnRef = useRef(fn);
  fnRef.current = fn;
  const load = useCallback(() => {
    const n = ++seq.current;
    setState(s => ({...s, loading: true}));
    fnRef.current(client).then(
      data => n === seq.current && setState({data, loading: false}),
      e => n === seq.current && setState(s => ({...s, loading: false, error: e instanceof Error ? e.message : String(e)})),
    );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [client, ...deps]);
  useEffect(() => { setState(s => (s.data === undefined ? s : {loading: true})); load(); }, [load]);
  const topicKey = topics.join(',');
  useEffect(() => {
    if (!topicKey) return;
    const offs = topicKey.split(',').map(t => client.subscribe(t as Topic, load));
    return () => offs.forEach(off => off());
  }, [client, topicKey, load]);
  return {...state, refresh: load};
}

/** Wraps a mutation with busy and error state. */
export function useAction() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const run = useCallback(async <R,>(fn: () => Promise<R>): Promise<R | undefined> => {
    setBusy(true);
    setError(undefined);
    try {
      return await fn();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      return undefined;
    } finally {
      setBusy(false);
    }
  }, []);
  return {busy, error, run, clearError: () => setError(undefined)};
}
