import {RouteError, routeOrigin} from '@core/route-identity.ts';
import type {WorkspaceServers} from './workspace-api';
import {contactKnownServerForOwnerRecovery} from '@core/owner-recovery-contact.ts';
import {accountDirectoryFailure, directoryFailurePhase} from './directory-errors';

export type DirectoryItem = WorkspaceServers['items'][number];
export type DirectorySnapshot = Readonly<{items: readonly DirectoryItem[]; phase: 'idle'|'reading'|'opening'|'recovering'|'leaving'; target?: DirectoryItem; showTarget?: boolean; status?: ReturnType<typeof directoryFailurePhase>; checkedAt: number; pendingId?: string; error?: unknown; waitingId?: string; seconds: number}>;
type Opened = Readonly<{origin: string; dispose: () => void}>;
type Ports = Readonly<{current: () => boolean; read: (signal: AbortSignal) => Promise<WorkspaceServers>; connect: (server: DirectoryItem, signal: AbortSignal) => Promise<Opened>; contact?: typeof contactKnownServerForOwnerRecovery; navigate: (origin: string, purpose?: 'owner-recovery') => void; localRecovery?: () => void; empty: () => void; now?: () => number}>;

/** One committed Hosted directory. No profile, credential or assertion is sent
 * to another origin; the server's own entry performs its ordinary admission. */
export class ServicesDirectoryFlow {
  private state: DirectorySnapshot;
  private listeners = new Set<() => void>();
  private active?: {controller: AbortController; automatic: boolean};
  private disposed = false;
  private next = 0;
  private delay = 20;
  private initialAttempted = false;
  private readonly startedAt: number;
  constructor(initial: WorkspaceServers, private readonly ports: Ports) {
    this.startedAt = this.now();
    this.state = Object.freeze({items: initial.items, phase: 'idle', checkedAt: this.now(), seconds: 20});
  }
  private now() {return this.ports.now?.() ?? Date.now();}
  getSnapshot = () => this.state;
  subscribe = (listener: () => void) => {this.listeners.add(listener); return () => {this.listeners.delete(listener);};};
  private current() {return !this.disposed && this.ports.current();}
  private publish(state: DirectorySnapshot) {if (this.current()) {this.state = Object.freeze(state); this.listeners.forEach(listener => listener());}}
  automaticInitial() {
    if (!this.current() || this.initialAttempted) return;
    this.initialAttempted = true;
    if (this.state.items.length === 1 && this.state.items[0].status === 'active') {this.delay = 10; void this.open(this.state.items[0], this.state, true);}
  }
  retryDelay() {return this.delay;}
  inConnectingGrace(now = this.now()) {return now - this.startedAt < 40_000;}
  reportError(error: unknown, expected: DirectorySnapshot) {if(this.current()&&this.state===expected)this.publish({...this.state,error});}
  countdown(seconds: number) {if (this.current() && !this.active && this.state.seconds !== seconds) this.publish({...this.state, seconds});}
  automatic() {
    if (!this.current() || this.active || !this.state.items.length) return;
    const allOffline=this.state.items.every(item=>item.status!=='active');
    const target = ((this.state.showTarget || !allOffline) && this.state.target) || this.state.items[this.next++ % this.state.items.length];
    void this.open(target, this.state, true);
  }
  cancelAutomatic() {
    const active = this.active;
    if (!active?.automatic) return;
    this.active = undefined; active.controller.abort();
    this.publish({...this.state, phase: 'idle', pendingId: undefined});
  }
  cancelOpening(expected: DirectorySnapshot) {
    if (!this.current() || this.state !== expected) return;
    const active = this.active; this.active = undefined; active?.controller.abort();
    this.publish({...this.state, phase: 'idle', pendingId: undefined, waitingId: undefined, target: undefined, status: undefined, showTarget: undefined, error: undefined});
  }
  async refresh(expected: DirectorySnapshot) {
    if (!this.current() || this.state !== expected || this.active) return;
    const active = {controller: new AbortController(), automatic: false}; this.active = active;
    const current = () => this.current() && this.active === active && !active.controller.signal.aborted;
    this.publish({...expected, phase: 'reading', error: undefined});
    try {
      const result = await this.ports.read(active.controller.signal); if (!current()) return;
      this.delay = 20; this.initialAttempted = false;
      this.publish({items: result.items, phase: 'idle', checkedAt: this.now(), seconds: 20});
      if (current() && !result.items.length) this.ports.empty();
    } catch (error) {if (current()) this.publish({...this.state, phase: 'idle', error});}
    finally {if (this.active === active) this.active = undefined;}
  }
  async open(server: DirectoryItem, expected: DirectorySnapshot, automatic = false) {
    if (!this.current() || this.state !== expected || this.active || !expected.items.includes(server) && expected.target !== server) return;
    const captured = Object.freeze({...server}), active = {controller: new AbortController(), automatic}; this.active = active;
    const current = () => this.current() && this.active === active && !active.controller.signal.aborted;
    this.publish({...expected, phase: 'opening', pendingId: captured.id, target: captured, status: undefined, showTarget: !automatic || expected.showTarget || expected.items.length === 1 && captured.status === 'active', error: undefined});
    let opened: Opened | undefined, readingAccount = true;
    try {
      const result = await this.ports.read(active.controller.signal); if (!current()) return;
      readingAccount = false;
      const matches = result.items.filter(item => item.id === captured.id), fresh = matches[0];
      if (!result.items.length) {this.publish({...this.state, items: result.items, phase: 'idle', pendingId: undefined, checkedAt: this.now()}); if (current()) this.ports.empty(); return;}
      if (matches.length !== 1 || fresh.publicKey !== captured.publicKey || fresh.policyRevision < captured.policyRevision) throw new RouteError('identity_mismatch', 'The selected server identity or membership changed.');
      this.publish({...this.state, items: result.items, checkedAt: this.now()});
      opened = await this.ports.connect(Object.freeze({...fresh}), active.controller.signal); if (!current()) return;
      const origin = routeOrigin(opened.origin); if (origin !== opened.origin) throw new RouteError('identity_mismatch', 'The server did not prove this address.');
      this.publish({...this.state, phase: 'leaving', waitingId: undefined});
      if (current()) this.ports.navigate(origin);
    } catch (error) {
      if (current()) {
        const failure = readingAccount ? accountDirectoryFailure(error) : error;
        const retry = Number((failure as {retryAfterSeconds?: unknown})?.retryAfterSeconds);
        const floor = Number.isFinite(retry) && retry > 0 ? Math.min(86400, Math.ceil(retry)) : 0;
        this.delay = Math.max(floor, Math.min(300, automatic ? this.delay * 2 : 20));
        this.publish({...this.state, phase: 'idle', pendingId: undefined, waitingId: captured.id, error: failure, status: directoryFailurePhase(failure, automatic), seconds: this.delay});
      }
    } finally {opened?.dispose(); if (this.active === active) this.active = undefined;}
  }
  /** The only emergency contact is a tap on an actual Hosted-unavailable
   * opening. Never reread Hosted or adopt its newer directory record here. */
  async recoverOwner(expected: DirectorySnapshot) {
    const captured = expected.target;
    if (!this.current() || this.state !== expected || this.active || expected.status !== 'account-waiting' || !captured) return;
    if (!captured.baseUrl) {this.ports.localRecovery?.(); return;}
    const active = {controller: new AbortController(), automatic: false}; this.active = active;
    const current = () => this.current() && this.active === active && !active.controller.signal.aborted && this.state.target === captured;
    this.publish({...expected, phase: 'recovering', pendingId: captured.id, error: undefined});
    try {
      const result = await (this.ports.contact ?? contactKnownServerForOwnerRecovery)({baseUrl: captured.baseUrl, serverId: captured.id, publicKey: captured.publicKey}, {signal: active.controller.signal, isCurrent: current});
      if (!current()) return;
      const origin = routeOrigin(captured.baseUrl);
      if (result.purpose !== 'owner-recovery' || result.serverId !== captured.id || result.origin !== origin || result.signInURL !== origin + '/sign-in') throw new RouteError('identity_mismatch', 'The recovery destination changed.');
      this.publish({...this.state, phase: 'leaving'});
      if (current()) this.ports.navigate(origin, 'owner-recovery');
    } catch (error) {
      if (current()) this.publish({...this.state, phase: 'idle', pendingId: undefined, error, status: directoryFailurePhase(error, false) ?? 'account-waiting'});
    } finally {if (this.active === active) this.active = undefined;}
  }
  dispose() {if (this.disposed) return; this.disposed = true; const active = this.active; this.active = undefined; active?.controller.abort(); this.listeners.clear();}
}
