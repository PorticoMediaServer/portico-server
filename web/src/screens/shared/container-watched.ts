import {useCallback, useEffect, useState} from 'react';
import {containerCarries, getContainerState, setContainerFlag, setContainerState, type ContainerFlag, type ContainerKind, type ContainerState} from '@core/container-state.ts';

type WatchedApi = {request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T>};

/** Kinds with title-level personal state: a watched default (show, season, album, book) or a Favorite (those and artists). */
export function containerKindFor(kind: string): ContainerKind | undefined {
  return kind === 'show' || kind === 'season' || kind === 'album' || kind === 'book' || kind === 'artist' ? kind : undefined;
}

export type ContainerWatched = Readonly<{
  /** The current default, when the read has answered. */
  state: ContainerState | null;
  loading: boolean;
  /** The write in flight, if any. */
  saving: boolean;
  error: {code: string} | null;
  refresh: () => void;
  /** Writes the default; rereads first on a 409 revision_mismatch. */
  setWatched: (watched: boolean) => Promise<boolean>;
  /** Whether this kind carries the flag (Watchlist: shows; Favorite: shows, albums, books, artists). */
  carries: (flag: ContainerFlag | 'watched') => boolean;
  /** Puts the title on, or takes it off, the Watchlist or Favorites; shown at once, put back if the write fails. */
  setFlag: (flag: ContainerFlag, value: boolean) => Promise<boolean>;
}>;

/**
 * One title's own personal state: the watched default (with the viewer's
 * visible-member counts), written as fenced intents, and its Watchlist and
 * Favorite flags, so a show's action row is a movie's.
 */
export function useContainerWatched(api: WatchedApi | null, kind: ContainerKind | undefined, containerId: string | undefined): ContainerWatched {
  const [state, setState] = useState<ContainerState | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<{code: string} | null>(null);
  const [ticket, setTicket] = useState(0);
  const ready = !!api && !!kind && !!containerId;
  useEffect(() => {
    if (!ready) return;
    let live = true;
    const controller = new AbortController();
    setLoading(true);
    setError(null);
    getContainerState(api!, kind!, containerId!, controller.signal).then(
      next => { if (live) { setState(next); setLoading(false); } },
      e => { if (live && !controller.signal.aborted) { setError({code: (e as {code?: string})?.code ?? 'request_failed'}); setLoading(false); } },
    );
    return () => { live = false; controller.abort(); };
    // The ticket retriggers the read after a write or an explicit refresh.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api, kind, containerId, ticket]);
  const refresh = useCallback(() => setTicket(t => t + 1), []);
  const setWatched = useCallback(async (watched: boolean) => {
    if (!ready) return false;
    setSaving(true);
    setError(null);
    try {
      // The cached revision first (one round trip); a 409 rereads before
      // authoring the intent again, exactly once.
      const base = state ?? await getContainerState(api!, kind!, containerId!);
      let next: ContainerState;
      try {
        next = await setContainerState(api!, kind!, containerId!, {expectedRevision: base.revision, watched});
      } catch (e) {
        if ((e as {code?: string})?.code !== 'revision_mismatch') throw e;
        const current = await getContainerState(api!, kind!, containerId!);
        next = await setContainerState(api!, kind!, containerId!, {expectedRevision: current.revision, watched});
      }
      setState(next);
      setSaving(false);
      return true;
    } catch (e) {
      setError({code: (e as {code?: string})?.code ?? 'request_failed'});
      setSaving(false);
      return false;
    }
  }, [api, kind, containerId, ready, state]);
  const carries = useCallback((flag: ContainerFlag | 'watched') => containerCarries(kind, flag), [kind]);
  const setFlag = useCallback(async (flag: ContainerFlag, value: boolean) => {
    if (!ready || !containerCarries(kind, flag)) return false;
    const before = state;
    if (before) setState({...before, [flag]: value});
    setError(null);
    try {
      const next = await setContainerFlag(api!, kind!, containerId!, flag, value);
      // The write's answer has no counts; keep the ones already read.
      setState(current => ({...next, ...(current?.watchedCount !== undefined ? {watchedCount: current.watchedCount, unwatchedCount: current.unwatchedCount} : {})}));
      return true;
    } catch (e) {
      setState(before);
      setError({code: (e as {code?: string})?.code ?? 'request_failed'});
      return false;
    }
  }, [api, kind, containerId, ready, state]);
  return {state, loading, saving, error, refresh, setWatched, carries, setFlag};
}
