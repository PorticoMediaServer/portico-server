export {alertLabel, alertLabels, alertTone, duration, factLabel, factLabels, formatFact, jobLabel, jobLabels, jobStateLabel, jobTone, sentence} from '@core/server-admin/console-words.ts';
import {useFreshness} from '../app/freshness';
import type {FreshnessDependency} from '@core/freshness-hub.ts';
import type {I18n} from '@i18n';
import {useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {QuietRead} from '@core/server-admin/quiet-read.ts';
export {QUIET_READ_EVERY_MS} from '@core/server-admin/quiet-read.ts';
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

export type ReadOptions = {
  auto?: boolean;
  /** Look again this often while the page is in front, for something the server sends no hint about (`QUIET_READ_EVERY_MS` is the usual pace). */
  every?: number;
  /** Read again when any of these moves on the event feed (a scan's progress, a task's state, the server's settings). */
  on?: readonly FreshnessDependency[];
};

/**
 * A console panel's read, kept current quietly (client-core `QuietRead`): what is shown stays
 * while it is read again; a read the owner did not ask for never spins, never blanks and never
 * shows its own failure; `reload` is the read the owner asks for (Try again, Refresh, after their
 * own action), and that one answers at once. `loading` is true only for a read the panel is
 * waiting on.
 */
export function useRead<T>(fn: () => Promise<T>, deps: readonly unknown[], options: ReadOptions = {}): Read<T> {
  const {hub} = useFreshness();
  const store = useMemo(() => new QuietRead<T>(hub), [hub]);
  useEffect(() => () => store.dispose(), [store]);
  const latest = useRef(fn);
  latest.current = fn;
  const on = options.on?.join('\n') ?? '';
  const every = options.every ?? 0;
  useEffect(() => { store.configure(() => latest.current(), on ? on.split('\n') : [], every); }, [store, on, every]);
  const auto = options.auto !== false;
  useEffect(() => {
    if (!auto) return;
    store.setOpen(true);
    store.reload(false);
    return () => store.setOpen(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [store, auto, ...deps]);
  const state = useSyncExternalStore(store.subscribe, store.getSnapshot);
  const reload = useCallback(() => store.reload(), [store]);
  const error = useMemo(() => (state.error === undefined ? undefined : problem(state.error) || undefined), [state.error]);
  // Before the first read has started there is nothing yet: that is loading, not empty.
  return {data: state.data, error, loading: state.loading || (auto && state.data === undefined && state.error === undefined), at: state.at, reload};
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

/**
 * A moment as the console shows it: the time alone for today, the day and time before that, in
 * the owner's locale and 12/24-hour clock.
 */
export function when(i18n: I18n, ms: number): string {
  if (!ms) return '—';
  const d = new Date(ms);
  return d.toDateString() === new Date().toDateString() ? i18n.time(d) : i18n.formatDate(d, {month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit'});
}
