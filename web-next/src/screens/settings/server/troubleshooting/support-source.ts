import {ConsoleClient, type SupportExport} from '@core/console.ts';
import {isStatePermissionsAlert} from '@core/administration.ts';
import {fixStatePermissions} from '@core/backups.ts';
import {parseCapabilities, type ServerCapability} from '@core/server-admin/capabilities.ts';
import {skippedParts, type SkippedPart} from '@core/tolerant.ts';
import {randomId} from '@core/random-id.ts';
import {routePath} from '../../../../../../packages/contracts/src/routes.ts';
import type {ReportTransport} from '@core/client-reports.ts';

export type SupportSnapshot = Readonly<{
  phase: 'idle' | 'loading' | 'ready' | 'error';
  capabilities: readonly ServerCapability[];
  /** The live permissions warning the server reports, if any. */
  permissions: boolean;
  /** What this app left out since it started, counted by document and part. */
  skipped: readonly SkippedPart[];
  preparing: boolean;
  export?: SupportExport;
  fixed: boolean;
}>;

const empty: SupportSnapshot = Object.freeze({phase: 'idle', capabilities: Object.freeze([]), permissions: false, skipped: Object.freeze([]), preparing: false, fixed: false});
/** The four parts the drawn export offers; the server decides what each one holds. */
export const SUPPORT_COMPONENTS = Object.freeze(['runtime', 'client', 'audit', 'snapshot'] as const);
export type SupportComponent = typeof SUPPORT_COMPONENTS[number];

/**
 * Owner-scoped Support: what this server can do, the permissions warning, the redacted
 * support export, and the parts this app could not read. Reads go through the same console
 * client and captured viewer fence as the rest of Settings.
 */
export class SettingsSupportSource {
  private state: SupportSnapshot = empty;
  private readonly listeners = new Set<() => void>();
  private readonly lifetime = new AbortController();
  private reading?: Promise<void>;
  private readonly console: ConsoleClient;
  constructor(private readonly transport: ReportTransport, private readonly serverId: string, viewerId: string, private readonly isCurrent: () => boolean) {
    this.console = new ConsoleClient(transport, {serverId, viewerId});
  }
  private current() {return !this.lifetime.signal.aborted && this.isCurrent();}
  getSnapshot = (): SupportSnapshot => this.current() ? this.state : empty;
  subscribe = (listener: () => void): (() => void) => {this.listeners.add(listener);return () => {this.listeners.delete(listener);};};
  private publish(state: SupportSnapshot) {if (this.current()) {this.state = Object.freeze(state);for (const listener of [...this.listeners]) listener();}}
  load = (): Promise<void> => {
    if (!this.current() || this.reading) return this.reading ?? Promise.resolve();
    const task = Promise.resolve().then(async () => {
      this.publish({...this.state, phase: 'loading'});
      try {
        const [capabilities, alerts] = await Promise.all([
          this.transport.request<unknown>(routePath('get_capability_report'), 'GET', undefined, this.lifetime.signal).then(parseCapabilities),
          this.console.alerts().catch(() => []),
        ]);
        if (!this.current()) return;
        this.publish({...this.state, phase: 'ready', capabilities, permissions: alerts.some(alert => isStatePermissionsAlert(alert)), skipped: skippedParts()});
      } catch {if (this.current() && !this.lifetime.signal.aborted) this.publish({...this.state, phase: 'error'});}
      finally {this.reading = undefined;}
    });
    this.reading = task;return task;
  };
  /** Prepare the redacted bundle the drawn page offers, over the window and parts chosen. */
  async prepare(windowMs: number, components: readonly SupportComponent[]): Promise<void> {
    if (!this.current() || this.state.preparing || !components.length) return;
    this.publish({...this.state, preparing: true});
    try {
      const to = Date.now(), from = to - windowMs;
      const ready = await this.console.createExport(from, to, [...components]);
      if (!this.current()) return;
      this.publish({...this.state, preparing: false, export: ready});
    } catch (error) {if (this.current()) this.publish({...this.state, preparing: false});throw error;}
  }
  /** The prepared bundle itself, for the download the owner asked for. */
  async download(): Promise<SupportExport> {
    const ready = this.state.export;
    if (!this.current() || !ready) throw new Error('No prepared export.');
    return this.console.export(ready.id);
  }
  async fixPermissions(): Promise<boolean> {
    if (!this.current()) return false;
    await fixStatePermissions(this.transport, this.serverId, randomId(), this.lifetime.signal);
    if (!this.current()) return false;
    this.publish({...this.state, permissions: false, fixed: true});
    return true;
  }
  retry = (): Promise<void> => {this.state = {...empty};return this.load();};
  dispose(): void {if (!this.lifetime.signal.aborted) {this.lifetime.abort();this.listeners.clear();this.console.dispose();this.state = empty;}}
}
