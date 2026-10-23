import {useCallback, useEffect, useState, useSyncExternalStore} from 'react';
import type {DashboardBindings} from './bindings';
export type ReadState<T> = Readonly<{value?: T; loading: boolean; failed: boolean}>;
/** One read in flight, cancellation and identity fence on every completion. Events coalesce. */
export function useDashboardRead<T>(bindings: DashboardBindings, load: (signal: AbortSignal) => Promise<T>, enabled: boolean, everyMs = 0) {
  const [version, setVersion] = useState(0), [state, setState] = useState<ReadState<T> & {loader?: typeof load}>({loading: true, failed: false});
  const refresh = useCallback(() => setVersion(v => v + 1), []);
  useEffect(() => {
    if (!enabled || !bindings.isCurrent()) return;
    const controller = new AbortController(); let running = false, queued = false;
    setState({loader: load, loading: true, failed: false});
    const read = async () => {
      if (running) {queued = true; return;}
      if (controller.signal.aborted || !bindings.isCurrent() || document.hidden) return;
      running = true;
      try {const value = await load(controller.signal); if (!controller.signal.aborted && bindings.isCurrent()) setState({loader: load, value, loading: false, failed: false});}
      catch (error) {if (!controller.signal.aborted && bindings.isCurrent()) {setState({loader: load, loading: false, failed: true}); bindings.effects.reportError(error);}}
      finally {running = false; if (queued) {queued = false; void read();}}
    };
    const update = () => {void read();};
    const offs = ['admin.sessions', 'session.updated', 'playback.history', 'library.scan', 'job.updated', 'stream.resync'].map(type => bindings.events?.on(type, update));
    offs.push(bindings.events?.onResync?.(update));
    document.addEventListener('visibilitychange', update);
    const timer = everyMs ? setInterval(update, everyMs) : undefined; update();
    return () => {controller.abort(); if (timer) clearInterval(timer); offs.forEach(off => off?.()); document.removeEventListener('visibilitychange', update);};
  }, [bindings, load, enabled, everyMs, version]);
  return {...(state.loader === load ? state : {loading: true, failed: false}), refresh};
}

/** The filter enumerates actual account IDs, including servers with more than one hundred people. */
export function useDashboardAccounts(bindings: DashboardBindings) {
  const [cursor, setCursor] = useState(''), [page, setPage] = useState<{items: Awaited<ReturnType<DashboardBindings['read']['people']>>['items']; nextCursor: string}>({items: [], nextCursor: ''});
  const load = useCallback((signal: AbortSignal) => bindings.read.people({period: 'all'}, cursor, signal), [bindings, cursor]);
  const state = useDashboardRead(bindings, load, true);
  useEffect(() => {if (state.value) setPage(previous => ({items: cursor ? [...previous.items, ...state.value!.items.filter(item => !previous.items.some(p => p.accountId === item.accountId))] : state.value!.items, nextCursor: state.value!.nextCursor}));}, [state.value, cursor]);
  return {...page, loading: state.loading, loadMore: () => {if (page.nextCursor) setCursor(page.nextCursor);}};
}

const noAuthoritySubscription = () => () => {};
export function useDashboardAuthority(bindings: DashboardBindings): boolean {
  return useSyncExternalStore(bindings.subscribeAuthority ?? noAuthoritySubscription, bindings.isCurrent, () => false);
}
