import { useCallback, useEffect, useRef, useState } from 'react';
import type { SettingsDataSource, QueryState } from './settingsTypes';
import { useOptionalViewerRuntime } from '../../data/DataProvider';
import { useHostedAvailabilityRetry } from '../../runtime/hostedAvailability';
import { combineAbortSignals, timeoutSignal } from '../../runtime/abortSignal';

const SETTINGS_QUERY_DEADLINE_MS = 15_000;

export function useSettingsQuery<T>(
  load: (source: SettingsDataSource, signal: AbortSignal) => Promise<T>,
  source: SettingsDataSource,
  revision: number,
  options: { automaticHostedRetry?: boolean } = {},
): QueryState<T> & { hostedAvailability: ReturnType<typeof useHostedAvailabilityRetry> } {
  const runtime = useOptionalViewerRuntime();
  const [state, setState] = useState<QueryState<T>>({ status: 'loading' });
  const [automaticRevision, setAutomaticRevision] = useState(0);
  const retryAutomatically = useCallback(() => setAutomaticRevision((current) => current + 1), []);
  const hostedAvailability = useHostedAvailabilityRetry({
    enabled: options.automaticHostedRetry === true && state.status === 'error',
    reason: state.status === 'error' ? state.error : undefined,
    retry: retryAutomatically,
  });

  useEffect(() => {
    const controller = new AbortController();
    const deadline = timeoutSignal(SETTINGS_QUERY_DEADLINE_MS);
    const querySignal = combineAbortSignals([controller.signal, deadline]);
    setState({ status: 'loading' });
    const request = runtime
      ? runtime.run('settings.query', [revision, querySignal], (runtimeSignal) => load(source, combineAbortSignals([querySignal, runtimeSignal])))
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
  }, [automaticRevision, load, revision, runtime, source]);

  return Object.assign(state, { hostedAvailability });
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
        ? runtime.run('settings.mutation', [controller.signal], (runtimeSignal) => mutation(combineAbortSignals([controller.signal, runtimeSignal])))
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
