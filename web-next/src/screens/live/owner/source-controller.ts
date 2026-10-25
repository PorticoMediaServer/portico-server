import type {LibraryContentApi} from '@core/library-content.ts';
import type {RefreshStatus} from '@core/linear-api.ts';
import {channelRequestID} from '@core/live-source-draft.ts';
import {createOperationIds} from '@core/server-admin/server-forms.ts';

export type OwnerLiveSource = Readonly<{
  id: string; name: string; revision: number; state: string; tunerCount: number;
  tunerCountMode: string; generation: string; publishedAt: string; channels: number;
  programmes: number; diagnostics?: unknown;
}>;
export type SourceDocument = Readonly<{sources: readonly OwnerLiveSource[]; refresh: readonly RefreshStatus[]}>;
export type SourceSnapshot = Readonly<{document?: SourceDocument; loading: boolean; busy?: string; error?: unknown; checkedAt?: number}>;
const sourcePath = '/v1/admin/live-sources';

/** The mounted owner page owns one read generation. A retired identity cannot
 * publish a response or issue a command, including a command after a refresh. */
export class LiveSourceController {
  private snapshot: SourceSnapshot = {loading: false};
  private listeners = new Set<() => void>();
  private generation = 0;
  private disposed = false;
  private pending?: AbortController;
  private command?: AbortController;
  private operations: ReturnType<typeof createOperationIds>;
  constructor(private api: LibraryContentApi, private serverId: string, private isCurrent: () => boolean, private reportError: (error: unknown) => void,
    requestId: () => string = channelRequestID) {this.operations = createOperationIds(requestId);}
  getSnapshot = () => this.snapshot;
  subscribe = (listener: () => void) => {this.listeners.add(listener);return () => {this.listeners.delete(listener);};};
  private current() {return !this.disposed && this.isCurrent();}
  private publish(value: SourceSnapshot) {if (!this.current()) return;this.snapshot = value;for (const listener of this.listeners) listener();}
  private failure(error: unknown) {
    if (!this.current()) return;
    const status = (error as {status?: number})?.status;
    if (status === 401 || status === 403) {this.reportError(error);this.dispose();return;}
    this.publish({...this.snapshot, loading: false, busy: undefined, error});
  }
  async load() {
    if (!this.current()) return;
    const generation = ++this.generation;
    this.pending?.abort();const pending = this.pending = new AbortController();
    this.publish({...this.snapshot, loading: true, error: undefined});
    try {
      const raw = await this.api.request<SourceDocument & {serverId: string}>(sourcePath, 'GET', undefined, pending.signal);
      if (raw.serverId !== this.serverId || !Array.isArray(raw.sources) || !Array.isArray(raw.refresh)) throw new Error('live_source_response_invalid');
      const document = raw;
      if (!pending.signal.aborted && generation === this.generation) this.publish({...this.snapshot, document, checkedAt: Date.now(), loading: false});
    } catch (error) {if (!pending.signal.aborted && generation === this.generation) this.failure(error);}
  }
  /** Commands use the displayed revision. A conflict requires a new read and
   * deliberate user action; never silently replay removal against new facts. */
  async act(source: OwnerLiveSource, action: 'refresh' | 'toggle' | 'remove') {
    if (!this.current() || this.snapshot.busy || !this.snapshot.document?.sources.some(row => row.id === source.id && row.revision === source.revision)) return false;
    if (action === 'refresh' && (source.state === 'disabled' || this.snapshot.document.refresh.some(row => row.sourceId === source.id && row.state === 'refreshing'))) return false;
    const command = this.command = new AbortController();
    const body = action === 'refresh' ? {expectedRevision: source.revision} : {
      expectedRevision: source.revision, requestId: this.operations.forPayload(JSON.stringify({id: source.id, revision: source.revision, action, enabled: source.state === 'disabled'})), ...(action === 'toggle' ? {enabled: source.state === 'disabled'} : {}),
    };
    this.publish({...this.snapshot, busy: source.id, error: undefined});
    try {
      await this.api.request(`${sourcePath}/${encodeURIComponent(source.id)}/${action === 'toggle' ? 'enabled' : action}`, 'POST', body, command.signal);
      if (!this.current() || command.signal.aborted) return false;
      this.operations.release();this.publish({...this.snapshot, busy: undefined});await this.load();return this.current();
    } catch (error) {
      if (!command.signal.aborted) {
        this.failure(error);
        if ((error as {status?: number})?.status === 409) {
          await this.load();this.publish({...this.snapshot, error});
        }
      }
      return false;
    }
  }
  dispose() {this.disposed = true;++this.generation;this.pending?.abort();this.command?.abort();this.listeners.clear();}
}
