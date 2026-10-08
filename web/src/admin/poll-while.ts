import {useEffect, useRef} from 'react';

/** 2, 4, 8, 16, then 30 s between checks. */
export const POLL_BACKOFF_MS = [2000, 4000, 8000, 16000, 30000] as const;
export const pollDelay = (attempt: number) => POLL_BACKOFF_MS[Math.min(attempt, POLL_BACKOFF_MS.length - 1)]!;

type Env = {setTimer: (fn: () => void, ms: number) => unknown; clearTimer: (t: unknown) => void; hidden: () => boolean; onVisibility: (fn: () => void) => () => void};
const browserEnv: Env = {
  setTimer: (fn, ms) => setTimeout(fn, ms),
  clearTimer: t => clearTimeout(t as ReturnType<typeof setTimeout>),
  hidden: () => typeof document !== 'undefined' && document.visibilityState === 'hidden',
  onVisibility: fn => { document.addEventListener('visibilitychange', fn); return () => document.removeEventListener('visibilitychange', fn); },
};

/**
 * Re-reads while something is in progress on the server (a source refresh), backing off
 * 2 → 30 s. Stops as soon as `active` is false; pauses while the page is hidden and checks again
 * when it's shown. Returns a stop function (for tests); the hook form ties it to a component.
 */
export function pollWhile(active: () => boolean, reload: () => void, env: Env = browserEnv): () => void {
  let attempt = 0, timer: unknown, stopped = false;
  const schedule = () => {
    env.clearTimer(timer); timer = undefined;
    if (stopped || !active() || env.hidden()) return;
    timer = env.setTimer(() => { timer = undefined; if (stopped || !active() || env.hidden()) return; reload(); attempt++; schedule(); }, pollDelay(attempt));
  };
  const offVisibility = env.onVisibility(() => { if (!env.hidden() && active()) { attempt = 0; reload(); } schedule(); });
  schedule();
  return () => { stopped = true; env.clearTimer(timer); offVisibility(); };
}

/** Poll `reload` with backoff while `active` is true (see `pollWhile`). */
export function usePollWhile(active: boolean, reload: () => void) {
  const reloadRef = useRef(reload);
  reloadRef.current = reload;
  const activeRef = useRef(active);
  activeRef.current = active;
  useEffect(() => {
    if (!active) return;
    return pollWhile(() => activeRef.current, () => reloadRef.current());
  }, [active]);
}
