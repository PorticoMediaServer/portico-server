import {randomId} from '@core/random-id.ts';
import {decodePostAdminStorageUsageCategoryCleanupResponse} from '../../../../../packages/contracts/src/decoders.ts';
import {encodedRoutePath} from '../../../../../packages/contracts/src/routes.ts';
import type {StorageCategory, StorageReport, StorageTransport} from './storage-source';
type Intent = Readonly<{category: StorageCategory; days: number; report: StorageReport; body: Readonly<{confirmation: string; olderThanDays: number; operationId: string}>}>;
export type CleanupSnapshot = Readonly<{busy: boolean; pending?: Readonly<{category: StorageCategory; days: number}>; error?: boolean; done?: boolean}>;
const empty: CleanupSnapshot = Object.freeze({busy:false});
const terminal = (error: unknown) => {const e = (error && typeof error === 'object' ? error : {}) as {status?:number;code?:string};return e.status === 401 || e.status === 403 || ['unauthorized','forbidden','not_permitted'].includes(e.code ?? '');};

/** A typed category cleanup is one immutable, deduplicated operation. An
 * uncertain response may retry only that original body and operation ID. */
export class SettingsCleanupSource {
  private state = empty;
  private readonly listeners = new Set<() => void>();
  private readonly lifetime = new AbortController();
  private flight?: AbortController;
  private intent?: Intent;
  private usedReport?: StorageReport;
  constructor(private readonly api: StorageTransport, private readonly serverId: string, private readonly isCurrent: () => boolean, private readonly report: () => StorageReport | undefined, private readonly refresh: () => Promise<void>, private readonly reportError: (error: unknown) => void) {}
  private current() {return !this.lifetime.signal.aborted && this.isCurrent();}
  private owns(flight: AbortController) {return this.current() && this.flight === flight && !flight.signal.aborted;}
  getSnapshot = (): CleanupSnapshot => this.current() ? this.state : empty;
  subscribe = (listener: () => void) => {this.listeners.add(listener);return () => {this.listeners.delete(listener);};};
  private publish(patch: Partial<CleanupSnapshot>) {if (this.current()) {this.state = Object.freeze({...this.state,...patch});for (const listener of this.listeners) listener();}}
  cleanup = async (category: StorageCategory, days: number, typed: string): Promise<boolean> => {
    const report = this.report();
    if (!this.current() || this.flight || this.intent || !report || report === this.usedReport || !report.categories.includes(category) || !category.present || !category.cleanable || category.bytes <= 0 || typed !== category.name || !Number.isSafeInteger(days) || days < 0 || days > 3650) return false;
    let intent: Intent;
    try {intent = Object.freeze({category, days, report, body:Object.freeze({confirmation:category.id,olderThanDays:days,operationId:randomId()})});}
    catch {this.publish({error:true});return false;}
    if (!this.current() || this.flight || this.intent || this.report() !== report) return false;
    this.intent = intent;return this.run(intent);
  };
  retry = async (): Promise<boolean> => {const intent = this.intent;return intent ? this.run(intent) : false;};
  private async run(intent: Intent): Promise<boolean> {
    if (!this.current() || this.flight || this.intent !== intent) return false;
    const flight = new AbortController();this.flight = flight;
    this.publish({busy:true,pending:Object.freeze({category:intent.category,days:intent.days}),error:undefined,done:undefined});
    let completed = false;
    try {
      if (!this.owns(flight)) return false;
      const response = decodePostAdminStorageUsageCategoryCleanupResponse(await this.api.request(encodedRoutePath('post_admin_storage_usage_category_cleanup',{category:encodeURIComponent(intent.category.id)}),'POST',intent.body,flight.signal));
      if (!this.owns(flight)) return false;
      if (response.serverId !== this.serverId || response.result.category !== intent.category.id || response.result.olderThanDays !== intent.days) throw new Error('invalid_cleanup_receipt');
      this.usedReport = intent.report;this.intent = undefined;completed = true;this.publish({pending:undefined,done:true});
    } catch (error) {
      if (this.owns(flight)) {
        if (terminal(error)) {this.lifetime.abort();flight.abort();this.intent = undefined;this.state = empty;for (const listener of this.listeners) listener();this.reportError(error);}
        else this.publish({error:true});
      }
    } finally {if (this.flight === flight) {this.flight = undefined;this.publish({busy:false});}}
    if (completed && this.current()) await this.refresh();
    return completed && this.current();
  }
  dispose = () => {this.flight?.abort();this.flight = undefined;this.intent = undefined;this.lifetime.abort();this.state = empty;this.listeners.clear();};
}
