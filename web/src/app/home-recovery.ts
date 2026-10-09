import {foregroundRetry, jittered} from './foreground-retry.ts';

export type HomeRecoveryEnv = {
  setTimer(fn: () => void, ms: number): unknown;
  clearTimer(timer: unknown): void;
  hidden(): boolean;
  onVisibility(fn: () => void): () => void;
  random(): number;
  now(): number;
};
const browserEnv: HomeRecoveryEnv = {
  setTimer: (fn, ms) => setTimeout(fn, ms),
  clearTimer: timer => clearTimeout(timer as ReturnType<typeof setTimeout>),
  hidden: () => typeof document !== 'undefined' && document.visibilityState === 'hidden',
  onVisibility: fn => { document.addEventListener('visibilitychange', fn); return () => document.removeEventListener('visibilitychange', fn); },
  random: () => Math.random(), now: () => Date.now(),
};

function retryable(error: unknown): boolean {
  const e = error as {status?: number; retryable?: boolean} | null;
  if (e?.status) return e.status === 429 || e.status >= 500;
  return e?.retryable !== false;
}
function minimumDelay(error: unknown): number {
  const e = error as {retryAfterSeconds?: number; retryAfterMs?: number} | null;
  const ms = e?.retryAfterMs ?? (e?.retryAfterSeconds === undefined ? 0 : e.retryAfterSeconds * 1000);
  return Number.isFinite(ms) && ms > 0 ? ms : 0;
}

/** Recovery for the small Home document read only. Never wraps long polls or playback. */
export function homeRecovery<T>(read: (signal: AbortSignal) => Promise<T>, handlers: {
  loading(): void; success(value: T): void; error(error: unknown): void;
}, env: HomeRecoveryEnv = browserEnv) {
  type Flight = {controller: AbortController; epoch: number; promise: Promise<void>};
  let active: Flight | undefined, epoch = 0, stopped = false, failures = 0, due = 0;
  // Browser timers overflow above ~24 days. Long server cooldowns are checked in
  // bounded chunks so even an unusually large Retry-After cannot turn into a hot loop.
  const retryEnv = {...env, setTimer: (fn: () => void, ms: number) => env.setTimer(fn, Math.min(ms, 2_147_483_647))};
  const retry = foregroundRetry(() => { void refresh(); }, {}, retryEnv);
  const schedule = (delay: number) => { due = env.now() + delay; retry.arm(delay); };
  const offVisibility = env.onVisibility(() => {
    if (stopped || !env.hidden() || !active) return;
    active.controller.abort();
    // Preserve an existing cooldown and spread foreground resumes; no hidden timer/read.
    schedule(Math.max(due - env.now(), jittered(1000, env.random)));
  });

  async function attempt(flight: Flight) {
    const {controller} = flight;
    let timedOut = false;
    const timer = env.setTimer(() => { timedOut = true; controller.abort(); }, 15_000);
    let abort = () => {};
    const cancelled = new Promise<never>((_, reject) => {
      abort = () => reject(Object.assign(new Error('home_read_timeout'), {code: 'request_timeout', retryable: true}));
      controller.signal.addEventListener('abort', abort, {once: true});
    });
    try {
      const value = await Promise.race([read(controller.signal), cancelled]);
      if (stopped || flight.epoch !== epoch || controller.signal.aborted) return;
      failures = 0; due = 0; retry.disarm(); handlers.success(value);
    } catch (error) {
      if (stopped || flight.epoch !== epoch || (controller.signal.aborted && !timedOut)) return;
      handlers.error(error);
      if (retryable(error)) {
        // Positive jitter follows the server floor; a 60-second Retry-After stays >= 60s.
        const delay = jittered(Math.max(minimumDelay(error), Math.min(30_000, 1000 * 2 ** Math.min(failures++, 5))), env.random);
        schedule(delay);
      }
    } finally {
      env.clearTimer(timer); controller.signal.removeEventListener('abort', abort);
      if (active === flight) active = undefined;
    }
  }

  /** Concurrent/manual refreshes share one read and cannot bypass overload cooldown. */
  function refresh(invalidate = false): Promise<void> {
    if (stopped) return Promise.resolve();
    if (invalidate) { epoch++; active?.controller.abort(); active = undefined; }
    if (active) return active.promise;
    if (env.hidden() || due > env.now()) {
      retry.arm(Math.max(1000, due - env.now()));
      return Promise.resolve();
    }
    retry.disarm();
    const flight: Flight = {controller: new AbortController(), epoch, promise: Promise.resolve()};
    active = flight;
    handlers.loading();
    flight.promise = attempt(flight);
    return flight.promise;
  }
  return {refresh, stop() { stopped = true; epoch++; active?.controller.abort(); retry.stop(); offVisibility(); }};
}
