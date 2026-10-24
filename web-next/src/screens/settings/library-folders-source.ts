import type {ContentScope, LibraryContentApi} from '@core/library-content.ts';
import {readLibraryInventoryConfig, type InventoryConfig} from '@core/library-inventory.ts';

export type SettingsLibraryFoldersSnapshot = Readonly<{libraryId?: string; config?: InventoryConfig; loading: boolean; error?: unknown}>;
const empty: SettingsLibraryFoldersSnapshot = Object.freeze({loading: false});

/** One explicitly selected library's read-only folder configuration. It is the producer's inventory revision, never a Settings revision. */
export class SettingsLibraryFoldersSource {
  private readonly bound: ContentScope;
  private state = empty;
  private request?: AbortController;
  private pending?: Promise<void>;
  private closed = false;
  constructor(private readonly api: LibraryContentApi, scope: ContentScope, private readonly isCurrent: () => boolean, private readonly notify: () => void, private readonly reportError: (error: unknown) => void) {
    // The caller's server/viewer scope is the fence every answer is checked against.
    this.bound = Object.freeze({serverId: scope.serverId, viewerId: scope.viewerId});
  }
  private current = () => !this.closed && this.isCurrent();
  getSnapshot = (): SettingsLibraryFoldersSnapshot => this.current() ? this.state : empty;
  /** Folders for exactly this selection; another library never borrows its answer. */
  config = (libraryId?: string): InventoryConfig | undefined => libraryId && this.current() && this.state.libraryId === libraryId ? this.state.config : undefined;
  private publish(state: SettingsLibraryFoldersSnapshot) {if (this.current()) {this.state = Object.freeze(state);this.notify();}}
  private owns(request: AbortController, libraryId: string) {return this.current() && !request.signal.aborted && this.request === request && this.state.libraryId === libraryId;}
  select = (libraryId?: string): Promise<void> => {
    if (!this.current()) return Promise.resolve();
    if (!libraryId) {this.clear();return Promise.resolve();}
    if (this.state.libraryId === libraryId) {
      if (this.pending) return this.pending;
      if (this.state.config) return Promise.resolve();
    }
    return this.read(libraryId);
  };
  /** Explicit recovery: rereads the selected library, keeping its genuine folders visible while the read runs. */
  refresh = (): Promise<void> => {
    const libraryId = this.state.libraryId;
    if (!this.current() || !libraryId) return Promise.resolve();
    return this.pending ?? this.read(libraryId);
  };
  private read(libraryId: string): Promise<void> {
    this.request?.abort();
    const request = new AbortController();this.request = request;
    const retained = this.state.libraryId === libraryId ? this.state.config : undefined;
    const pending = readLibraryInventoryConfig(this.api, this.bound, libraryId, request.signal).then(config => {
      if (this.owns(request, libraryId)) this.publish({libraryId, config, loading: false});
    }).catch(error => {
      // An error means this selection has no current configuration; a genuine reread is the only recovery.
      if (this.owns(request, libraryId)) {this.publish({libraryId, loading: false, error});this.reportError(error);}
    }).finally(() => {if (this.request === request) {this.request = undefined;this.pending = undefined;}});
    this.pending = pending;
    this.publish({libraryId, ...(retained ? {config: retained} : {}), loading: true});
    return pending;
  }
  clear = () => {
    this.request?.abort();this.request = undefined;this.pending = undefined;
    if (this.state !== empty) {this.state = empty;if (this.current()) this.notify();}
  };
  dispose() {this.closed = true;this.request?.abort();this.request = undefined;this.pending = undefined;this.state = empty;}
}
