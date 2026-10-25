import {randomId} from '@core/random-id.ts';
import {decodeRevokeProfileDownloadsResponse} from '../../../../../packages/contracts/src/decoders.ts';
import {routePath, routeBody} from '../../../../../packages/contracts/src/routes.ts';
import type {StorageTransport} from '../server/storage-source';
import type {DownloadsUsageRow, DownloadsUsageSource, UsagePage} from './downloads-usage-source';

type Continuation = Readonly<{row: DownloadsUsageRow; after: UsagePage}>;
type Intent = Readonly<{continuation?: Continuation; row: DownloadsUsageRow; page: UsagePage; body: Readonly<{profileKey: string; operationId: string}>}>;
export type DownloadRemovalSnapshot = Readonly<{busy: boolean; pending?: DownloadsUsageRow; continuation?: DownloadsUsageRow; error?: boolean; done?: boolean}>;
const empty: DownloadRemovalSnapshot = Object.freeze({busy: false});

/** One bounded withdrawal of the selected person's claims and offline permissions.
 * Unknown outcomes retry only the original receipt ID/body. A confirmed `more`
 * permits a separate explicit confirmation after a checked usage refresh. */
export class DownloadRemovalSource {
  private state = empty;
  private readonly lifetime = new AbortController();
  private readonly listeners = new Set<() => void>();
  private flight?: AbortController;
  private intent?: Intent;
  private usedPage?: UsagePage;
  private continuation?: Continuation;
  constructor(private readonly api: StorageTransport, private readonly usage: DownloadsUsageSource, private readonly isCurrent: () => boolean, private readonly reportError: (error: unknown) => void) {}
  private current() {return !this.lifetime.signal.aborted && this.isCurrent();}
  private owns(request: AbortController) {return this.current() && this.flight === request && !request.signal.aborted;}
  getSnapshot = (): DownloadRemovalSnapshot => this.current() ? this.state : empty;
  subscribe = (listener: () => void) => {this.listeners.add(listener);return () => {this.listeners.delete(listener);};};
  private publish(patch: Partial<DownloadRemovalSnapshot>) {if (this.current()) {this.state = Object.freeze({...this.state, ...patch});for (const listener of this.listeners) listener();}}
  remove = async (row: DownloadsUsageRow, confirmed: () => boolean): Promise<boolean> => {
    const read = this.usage.getSnapshot(), page = read.page;
    const continued = this.continuation?.row === row && this.continuation.after !== page;
    if (!this.current() || !confirmed() || this.flight || this.intent || this.continuation && this.continuation.row.profileKey !== row.profileKey || read.phase !== 'ready' || !page || page === this.usedPage || !continued && !page.rows.includes(row) || !row.profileKey || row.profileKey.length > 1024 || /[\x00-\x1f\x7f]/.test(row.profileKey) || !Number.isSafeInteger(row.count) || row.count < 1) return false;
    const body = Object.freeze({profileKey: row.profileKey, operationId: randomId()});
    this.intent = Object.freeze({row, page, body, continuation: this.continuation});this.continuation = undefined;
    return this.perform(confirmed);
  };
  retry = (confirmed: () => boolean): Promise<boolean> => this.intent ? this.perform(confirmed) : Promise.resolve(false);
  private async perform(confirmed: () => boolean): Promise<boolean> {
    const intent = this.intent;
    if (!intent || !this.current() || !confirmed() || this.flight) return false;
    const request = new AbortController();this.flight = request;
    const signal = AbortSignal.any([request.signal, this.lifetime.signal]);
    let submitted = false;
    this.publish({busy: true, pending: intent.row, continuation: undefined, error: undefined, done: false});
    try {
      if (!this.owns(request) || !confirmed()) return false;
      submitted = true;
      const raw = await this.api.request<unknown>(routePath('revoke_profile_downloads'), 'POST', routeBody('revoke_profile_downloads', intent.body), signal);
      if (!this.owns(request)) return false;
      const receipt = decodeRevokeProfileDownloadsResponse(raw);
      if (!Number.isSafeInteger(receipt.claims) || receipt.claims < 0 || receipt.claims > 100 || !Number.isSafeInteger(receipt.receipts) || receipt.receipts < 0 || receipt.receipts > 100 || receipt.more && receipt.claims < 100 && receipt.receipts < 100) throw new Error('invalid_download_removal_receipt');
      this.intent = undefined;this.usedPage = intent.page;
      this.continuation = receipt.more ? Object.freeze({row: intent.row, after: intent.page}) : undefined;
      await this.usage.refresh();
      if (!this.owns(request)) return false;
      const refreshed = this.usage.getSnapshot();
      if (refreshed.phase !== 'ready' || !refreshed.page || refreshed.page === intent.page) {
        this.publish({pending: undefined, continuation: this.continuation?.row, error: true});return false;
      }
      // A confirmed more=true receipt witnesses this same target even when the
      // remaining offline permissions no longer have a live preparation row.
      this.publish({pending: undefined, continuation: receipt.more ? intent.row : undefined, done: !receipt.more});
      return true;
    } catch (error) {
      if (!this.owns(request)) return false;
      const failure = (error && typeof error === 'object' ? error : {}) as {status?: number; code?: string};
      if (failure.status === 401 || failure.status === 403 || ['unauthorized','forbidden','not_permitted'].includes(failure.code ?? '')) {
        this.dispose();this.reportError(new Error('download_removal_denied'));
      } else {
        if (!submitted || failure.status === 400 || ['invalid_request','idempotency_key_reused'].includes(failure.code ?? '')) {this.intent = undefined;this.usedPage = intent.page;}
        this.publish({pending: this.intent?.row, error: true});
      }
      return false;
    } finally {
      if (this.flight === request) {
        this.flight = undefined;
        if (!submitted && this.intent === intent) {this.intent = undefined;this.continuation = intent.continuation;}
        this.publish({busy: false, pending: this.intent?.row, continuation: this.continuation?.row});
      }
    }
  }
  dispose = () => {this.lifetime.abort();this.flight?.abort();this.flight = undefined;this.intent = undefined;this.continuation = undefined;this.usedPage = undefined;this.state = empty;this.listeners.clear();};
}
