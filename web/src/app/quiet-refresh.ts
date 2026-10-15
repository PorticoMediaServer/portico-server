import {useEffect, useRef, useState} from 'react';
import {backoffDelay, foregroundRetry} from './foreground-retry';

/**
 * A list the server says changed under the page (`refresh-required`: even its first page was
 * refused as out of date) is read again by itself, quietly, with what is shown kept in place:
 * open screens stay current without anyone pressing Refresh. Only a list that is still refused
 * after a few tries (about half a minute) says so, with the notice's own Refresh.
 *
 * Tries run only while the tab is visible and back off with jitter; a list that has stayed
 * readable for a minute starts counting again from zero.
 */
export const QUIET_REFRESH_TRIES = 3;
const BASE_MS = 1500, MAX_MS = 12000, SETTLED_MS = 60_000;

/** The pure rule: whether to try again now (and after how long), or say the list is stuck. */
export function quietRefreshStep(state: {tries: number; lastAt: number}, now: number, random: () => number = Math.random): {state: {tries: number; lastAt: number}; delayMs?: number; stuck: boolean} {
  const tries = now - state.lastAt > SETTLED_MS ? 0 : state.tries;
  if (tries >= QUIET_REFRESH_TRIES) return {state: {tries, lastAt: state.lastAt}, stuck: true};
  return {state: {tries: tries + 1, lastAt: now}, delayMs: tries === 0 ? 0 : backoffDelay(tries - 1, BASE_MS, MAX_MS, random), stuck: false};
}

/** True once quiet tries have not brought the list back: then, and only then, show the notice. */
export function useQuietRefresh(required: boolean, refresh: () => unknown): boolean {
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
