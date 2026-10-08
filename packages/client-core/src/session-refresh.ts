/**
 * Keeps a device-bound server sign-in alive (lane C, 939a746: 15-minute access tokens with a
 * rotating refresh credential, `POST /v1/auth/refresh`).
 *
 * - Proactive: refreshes `leadMs` before the access token expires, and again whenever a request
 *   finds it about to expire (timers sleep with the app; `wake()` on resume too).
 * - Reactive: a 401 on the current token triggers one refresh; the request is retried once.
 * - Single flight: one refresh at a time in this runtime; concurrent callers wait for it. Across
 *   tabs, `refreshStoredSession` runs under the credential lock and adopts another tab's rotation.
 * - Atomic: the next access and refresh credential are stored in one change of the protected
 *   connection store (web: encrypted IndexedDB; Apple: Keychain), with the request id persisted
 *   before sending so an interrupted refresh replays instead of burning the credential.
 * - Refusal (reuse, revocation, device unapproved, expired sign-in) ends the sign-in: the store
 *   is cleared and `onSignedOut` runs. Transient failures keep the credential and retry.
 */
import type {AuthRecovery, HttpLocalApi, LocalSession} from './index.ts';
import {REFRESH_MIN_VALID_MS, SessionRefreshError, refreshStoredSession, type ConnectionEnvironment, type RefreshStoredOptions} from './server-connections.ts';

export type RefresherSnapshot = Readonly<{
  phase: 'active' | 'refreshing' | 'retrying' | 'ended' | 'stopped';
  expiresAt: string;
  failures: number;
  error?: SessionRefreshError;
}>;

export type LocalSessionRefresherOptions = Readonly<{
  env: ConnectionEnvironment;
  api: Pick<HttpLocalApi, 'request' | 'setAccessToken' | 'setAuthRecovery'>;
  /** The selected session (the viewer's copy is enough: the store supplies the refresh credential). */
  session: LocalSession;
  /** A renewed access token is in use: update the viewer snapshot (`ViewerService.rotate`) and the viewer record. */
  onRotated?: (session: LocalSession) => void;
  /** The server refused the credential and the stored sign-in was removed. Show signed out; don't clear storage again. */
  onSignedOut?: (error: SessionRefreshError) => void;
  /** Refresh this long before expiry (default 2 minutes). */
  leadMs?: number;
  /** Backoff after transient failures. */
  retryMs?: readonly number[];
  now?: () => number;
  setTimer?: (fn: () => void, ms: number) => unknown;
  clearTimer?: (timer: unknown) => void;
  /** Test seam; defaults to `refreshStoredSession`. */
  refresh?: (options: RefreshStoredOptions) => Promise<LocalSession>;
}>;

export class LocalSessionRefresher implements AuthRecovery {
  private o: LocalSessionRefresherOptions;
  private session: LocalSession;
  private flight?: Promise<LocalSession>;
  private timer?: unknown;
  private failures = 0;
  private state: RefresherSnapshot;
  private listeners = new Set<() => void>();
  private now: () => number;
  private setTimer: (fn: () => void, ms: number) => unknown;
  private clearTimer: (timer: unknown) => void;

  constructor(options: LocalSessionRefresherOptions) {
    this.o = options;
    this.session = options.session;
    this.now = options.now ?? Date.now;
    this.setTimer = options.setTimer ?? ((fn, ms) => setTimeout(fn, ms));
    this.clearTimer = options.clearTimer ?? (t => clearTimeout(t as ReturnType<typeof setTimeout>));
    this.state = Object.freeze({phase: 'active', expiresAt: this.session.expiresAt, failures: 0});
    options.api.setAuthRecovery(this);
    this.schedule();
  }

  getSnapshot = (): RefresherSnapshot => this.state;
  subscribe = (fn: () => void) => { this.listeners.add(fn); return () => { this.listeners.delete(fn); }; };
  private publish(p: Partial<RefresherSnapshot>) { this.state = Object.freeze({...this.state, ...p}); for (const l of [...this.listeners]) l(); }

  /** The session in use (without its refresh credential, which only the store holds). */
  current(): LocalSession { return this.session; }
  private get done() { return this.state.phase === 'ended' || this.state.phase === 'stopped'; }
  private get lead() { return this.o.leadMs ?? 120_000; }
  private expiresIn() { return Date.parse(this.session.expiresAt) - this.now(); }

  // ── AuthRecovery (HttpLocalApi calls these) ─────────────────────
  token = async (current: string): Promise<string> => {
    if (this.done) return current;
    if (current !== this.session.accessToken) return this.session.accessToken;
    if (this.expiresIn() > REFRESH_MIN_VALID_MS) return current;
    try { return (await this.run()).accessToken; } catch { return this.session.accessToken; }
  };

  recover = async (rejected: string): Promise<string | undefined> => {
    if (this.done) return undefined;
    if (rejected !== this.session.accessToken) return this.session.accessToken;
    try { return (await this.run(rejected)).accessToken; } catch { return undefined; }
  };

  /** Check now (app returned to the foreground, network came back). */
  wake(): void {
    if (this.done) return;
    if (this.expiresIn() <= this.lead) void this.run().catch(() => {});
    else this.schedule();
  }

  /** Stop keeping this sign-in alive (sign-out, switching viewer). Does not touch storage. */
  stop(): void {
    if (this.timer !== undefined) this.clearTimer(this.timer);
    this.timer = undefined;
    if (!this.done) this.publish({phase: 'stopped'});
    this.o.api.setAuthRecovery(undefined);
  }

  /** One refresh at a time; concurrent callers share it. */
  private run(rejected?: string): Promise<LocalSession> {
    if (this.flight) return this.flight;
    if (this.done) return Promise.reject(this.state.error ?? new SessionRefreshError('stopped', 'Sign-in renewal stopped.', false));
    this.publish({phase: 'refreshing'});
    const refresh = this.o.refresh ?? refreshStoredSession;
    const flight = refresh({
      env: this.o.env,
      api: this.o.api,
      family: {sessionFamilyId: this.session.sessionFamilyId, serverId: this.session.viewer.serverId},
      rejected,
      now: this.now,
      // A proactive run renews inside the lead window; the store answers without a request when another context already did.
      minValidMs: Math.max(REFRESH_MIN_VALID_MS, this.lead),
    }).then(next => { this.adopt(next); return next; }, (e: unknown) => { this.failed(e); throw e; });
    this.flight = flight;
    void flight.finally(() => { if (this.flight === flight) this.flight = undefined; }).catch(() => {});
    return flight;
  }

  private adopt(next: LocalSession) {
    if (this.done) return;
    this.failures = 0;
    const {refreshToken: _secret, ...visible} = next;
    const changed = visible.accessToken !== this.session.accessToken;
    this.session = Object.freeze(visible);
    if (changed) { this.o.api.setAccessToken(visible.accessToken); this.o.onRotated?.(this.session); }
    this.publish({phase: 'active', expiresAt: visible.expiresAt, failures: 0, error: undefined});
    this.schedule();
  }

  private failed(e: unknown) {
    if (this.done) return;
    const error = e instanceof SessionRefreshError ? e : new SessionRefreshError('refresh_unavailable_now', 'The server couldn’t renew your sign-in right now.', false);
    // Nothing more to try: refused, or no refresh credential and the access token is spent.
    if (error.signOut || (error.code === 'refresh_unavailable' && this.expiresIn() <= 0)) {
      if (this.timer !== undefined) this.clearTimer(this.timer);
      this.timer = undefined;
      this.publish({phase: 'ended', error});
      this.o.api.setAuthRecovery(undefined);
      this.o.onSignedOut?.(error);
      return;
    }
    this.failures++;
    this.publish({phase: 'retrying', failures: this.failures, error});
    this.schedule();
  }

  private schedule() {
    if (this.timer !== undefined) this.clearTimer(this.timer);
    this.timer = undefined;
    if (this.done) return;
    let delay: number;
    if (this.failures > 0) {
      const backoff = this.o.retryMs ?? [5_000, 15_000, 30_000, 60_000, 120_000];
      delay = backoff[Math.min(this.failures - 1, backoff.length - 1)] ?? 120_000;
      // An old server without the endpoint never expires tokens this way; check rarely.
      if (this.state.error?.code === 'refresh_unsupported') delay = Math.max(delay, 15 * 60_000);
      if (this.state.error?.code === 'refresh_unavailable') { const left = this.expiresIn(); if (left <= 0) return; delay = left + 1_000; }
    } else delay = Math.max(0, this.expiresIn() - this.lead);
    this.timer = this.setTimer(() => { this.timer = undefined; void this.run().catch(() => {}); }, Math.min(delay, 2_147_000_000));
  }
}
