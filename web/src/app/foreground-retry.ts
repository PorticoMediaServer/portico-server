/**
 * Automatic retries that only run in a visible tab (Justin's rule: Hosted contact stays minimal,
 * every automatic call backs off with random delays; Apple reconnects in the foreground only).
 *
 * - A retry is armed with a delay; while the tab is hidden no timer runs at all. A retry that
 *   came due, or was armed, while hidden runs once, a moment after the tab is shown again.
 * - `jittered` spreads delays so tabs and devices told the same thing don't come back together.
 * - `hostedWait` lets a retry that may reach the Portico Account service honour client-core's
 *   shared circuit (`hostedGate(origin).blockedFor()`): it never goes before the circuit allows.
 */
type Env = {
  setTimer: (fn: () => void, ms: number) => unknown;
  clearTimer: (t: unknown) => void;
  hidden: () => boolean;
  onVisibility: (fn: () => void) => () => void;
  random: () => number;
  now: () => number;
};
const browserEnv: Env = {
  setTimer: (fn, ms) => setTimeout(fn, ms),
  clearTimer: t => clearTimeout(t as ReturnType<typeof setTimeout>),
  hidden: () => typeof document !== 'undefined' && document.visibilityState === 'hidden',
  onVisibility: fn => { document.addEventListener('visibilitychange', fn); return () => document.removeEventListener('visibilitychange', fn); },
  random: () => Math.random(),
  now: () => Date.now(),
};

/** `ms` plus up to half again, at random. */
export const jittered = (ms: number, random: () => number = Math.random) => Math.round(ms + random() * ms / 2);

/** Exponential backoff from `baseMs`, capped at `maxMs`, with jitter. */
export const backoffDelay = (attempt: number, baseMs: number, maxMs: number, random: () => number = Math.random) => jittered(Math.min(maxMs, baseMs * 2 ** Math.max(0, attempt)), random);

/** A retry that came due while hidden runs 1 to 1.5 s after the tab is shown again. */
const RESUME_MS = 1000;

export type ForegroundRetry = {
  /** Arm one retry after `delayMs` (never before `hostedWait()`); replaces an armed one. */
  arm(delayMs: number): void;
  /** Disarm without stopping (the work succeeded or was cancelled). */
  disarm(): void;
  /** Whether a retry is armed or waiting for the tab to be shown. */
  pending(): boolean;
  stop(): void;
};

export function foregroundRetry(attempt: () => void, options: {hostedWait?: () => number} = {}, env: Env = browserEnv): ForegroundRetry {
  let timer: unknown, due = 0, armed = false, stopped = false;
  const wait = () => Math.max(0, options.hostedWait?.() ?? 0);
  const clear = () => { if (timer !== undefined) env.clearTimer(timer); timer = undefined; };
  const start = (ms: number) => {
    clear();
    if (stopped || !armed || env.hidden()) return;
    timer = env.setTimer(fire, Math.max(ms, wait()));
  };
  const fire = () => {
    timer = undefined;
    if (stopped || !armed) return;
    if (env.hidden()) return; // Shown again later: runs then.
    const floor = wait();
    if (floor > 0) { timer = env.setTimer(fire, jittered(floor, env.random)); return; }
    armed = false;
    attempt();
  };
  const offVisibility = env.onVisibility(() => {
    if (stopped || !armed) return;
    if (env.hidden()) { clear(); return; }
    // Shown again: a retry already due runs shortly; one not yet due keeps its remaining time.
    const remaining = Math.max(0, due - env.now());
    start(remaining > 0 ? remaining : jittered(RESUME_MS, env.random));
  });
  return {
    arm(delayMs) { if (stopped) return; armed = true; due = env.now() + delayMs; start(delayMs); },
    disarm() { armed = false; clear(); },
    pending: () => armed,
    stop() { stopped = true; armed = false; clear(); offVisibility(); },
  };
}
