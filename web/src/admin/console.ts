import {formatBytes} from '@core/presentation/content.ts';
export {alertLabel, alertLabels, alertTone, duration, factLabel, factLabels, formatFact, jobLabel, jobLabels, jobStateLabel, jobTone, sentence, when} from '@core/server-admin/console-words.ts';
import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {ConsoleClient} from '@core/console.ts';
import {presentError} from '@core/presentation/index.ts';
import {useSession} from '../app/session';
import {serverChanged} from '../app/server-changes';
import {useViewerScope} from '../app/viewer-scope';

/** One console client per signed-in owner; drafts and unconfirmed intents live on it. */
export function useConsole(): ConsoleClient {
  const {api} = useSession();
  const scope = useViewerScope();
  const client = useMemo(() => new ConsoleClient(api, scope), [api, scope]);
  useEffect(() => () => client.dispose(), [client]);
  return client;
}

export type Read<T> = {data?: T; error?: string; loading: boolean; at?: number; reload: () => void};

/** A bounded read with freshness, retry and stale-result fencing. */
export function useRead<T>(fn: () => Promise<T>, deps: readonly unknown[], options: {auto?: boolean; every?: number} = {}): Read<T> {
  const [state, setState] = useState<{data?: T; error?: string; loading: boolean; at?: number}>({loading: options.auto !== false});
  const seq = useRef(0);
  const reload = useCallback(() => {
    const n = ++seq.current;
    setState(s => ({...s, loading: true, error: undefined}));
    fn().then(data => n === seq.current && setState({data, loading: false, at: Date.now()})).catch(e => n === seq.current && setState(s => ({...s, loading: false, error: problem(e)})));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
  useEffect(() => {
    if (options.auto === false) return;
    reload();
    if (!options.every) return;
    const t = setInterval(() => document.visibilityState === 'visible' && reload(), options.every);
    return () => clearInterval(t);
  }, [reload, options.auto, options.every]);
  return {...state, reload};
}

/** Serialised mutation runner with one error and one notice at a time. */
export function useAction() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const live = useRef(true);
  useEffect(() => {
    live.current = true;
    return () => {
      live.current = false;
    };
  }, []);
  const run = useCallback(async (fn: () => Promise<unknown>, done?: string) => {
    if (busy) return false;
    setBusy(true);
    setError('');
    setNotice('');
    try {
      await fn();
      serverChanged();
      if (live.current && done) setNotice(done);
      return true;
    } catch (e) {
      if (live.current) setError(problem(e, 'action'));
      return false;
    } finally {
      if (live.current) setBusy(false);
    }
  }, [busy]);
  return {busy, error, notice, run, clear: () => { setError(''); setNotice(''); }};
}

/** Console failures in plain words (X-04): copy comes from the shared presenter by code and status, never from `error.message`. */
export function problem(e: unknown, operation: 'load' | 'action' = 'load'): string {
  const p = presentError(e, 'server-console', {operation, deviceOnline: typeof navigator === 'undefined' ? undefined : navigator.onLine});
  return p.silent ? '' : p.body;
}

/** A 404 from a console read means the feature isn't set up on this server (or this version lacks it): an empty state, never an error. */
export const unconfigured = <T,>(read: Promise<T>): Promise<T | null> => read.catch(e => ((e as {status?: number} | null)?.status === 404 ? null : Promise.reject(e)));

/** One byte formatter for every screen: client-core's. */
export const bytes = (n: number): string => formatBytes(n);
