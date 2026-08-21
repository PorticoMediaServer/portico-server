import { useCallback, useEffect, useRef, useState } from 'react';
import type { SettingsDataSource, QueryState } from './settingsTypes';
import { useOptionalViewerRuntime } from '../../data/DataProvider';

const SETTINGS_QUERY_DEADLINE_MS = 15_000;

export function useSettingsQuery<T>(
  load: (source: SettingsDataSource, signal: AbortSignal) => Promise<T>,
  source: SettingsDataSource,
  revision: number,
): QueryState<T> {
  const runtime = useOptionalViewerRuntime();
  const [state, setState] = useState<QueryState<T>>({ status: 'loading' });

  useEffect(() => {
    const controller = new AbortController();
    const deadline = AbortSignal.timeout(SETTINGS_QUERY_DEADLINE_MS);
    const querySignal = AbortSignal.any([controller.signal, deadline]);
    setState({ status: 'loading' });
    const request = runtime
      ? runtime.run('settings.query', [revision, querySignal], (runtimeSignal) => load(source, AbortSignal.any([querySignal, runtimeSignal])))
      : load(source, querySignal);
    request.then(
      (data) => {
        if (!controller.signal.aborted) setState({ status: 'success', data });
      },
      (reason: unknown) => {
        if (controller.signal.aborted) return;
        setState({
          status: 'error',
          error: reason instanceof Error ? reason : new Error('Portico could not load this settings section.'),
        });
      },
    );
    return () => controller.abort();
  }, [load, revision, runtime, source]);

  return state;
}

export function useAbortableMutation() {
  const runtime = useOptionalViewerRuntime();
  const active = useRef<AbortController | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => () => active.current?.abort(), []);

  const run = useCallback(async <T,>(mutation: (signal: AbortSignal) => Promise<T>): Promise<T> => {
    active.current?.abort();
    const controller = new AbortController();
    active.current = controller;
    setBusy(true);
    try {
      return await (runtime
        ? runtime.run('settings.mutation', [controller.signal], (runtimeSignal) => mutation(AbortSignal.any([controller.signal, runtimeSignal])))
        : mutation(controller.signal));
    } finally {
      if (active.current === controller) {
        active.current = null;
        setBusy(false);
      }
    }
  }, [runtime]);

  return { busy, run };
}
