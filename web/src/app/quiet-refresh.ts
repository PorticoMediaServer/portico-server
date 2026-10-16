import {useEffect, useRef, useState} from 'react';
import {quietRefreshStep} from '@core/quiet-refresh.ts';
import {foregroundRetry} from './foreground-retry';

/**
 * The web's quiet re-read of a list the server says changed under the page (the shared rule,
 * `quietRefreshStep`): tries run only while the tab is visible, and the notice shows only once
 * they have not brought the list back. True once that is so: then, and only then, show the notice.
 */
export function useRefreshRequiredRetry(required: boolean, refresh: () => unknown): boolean {
  const [stuck, setStuck] = useState(false);
  const latest = useRef(refresh);
  latest.current = refresh;
  const still = useRef(required);
  still.current = required;
  const state = useRef({tries: 0, lastAt: 0});
  const retry = useRef<ReturnType<typeof foregroundRetry> | null>(null);
  const flying = useRef(false);
  const next = useRef(() => {});
  next.current = () => {
    if (!still.current) return;
    const step = quietRefreshStep(state.current, Date.now());
    state.current = step.state;
    if (step.stuck) setStuck(true); else retry.current?.arm(step.delayMs ?? 0);
  };
  useEffect(() => {
    // After each try: a list still refused is tried again, until the tries run out.
    const r = foregroundRetry(() => {
      let attempt: Promise<unknown>;
      flying.current = true;
      try { attempt = Promise.resolve(latest.current()); } catch (error) { attempt = Promise.reject(error); }
      void attempt.catch(() => {}).finally(() => setTimeout(() => { flying.current = false; next.current(); }, 0));
    });
    retry.current = r;
    return () => { r.stop(); retry.current = null; };
  }, []);
  useEffect(() => {
    if (!required) { retry.current?.disarm(); setStuck(false); return; }
    // A try under way says what comes next when it ends.
    if (!flying.current && !retry.current?.pending()) next.current();
  }, [required]);
  return required && stuck;
}
