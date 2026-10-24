import type {LibraryAdminDocument} from '@core/administration.ts';
import type {LibraryContentApi} from '@core/library-content.ts';
import {patchLibrarySettingsBinding, readLibrarySettingsBinding} from '@core/presentation/settings-bindings.ts';
import {randomId} from '@core/random-id.ts';
import {readLibrarySettings, saveLibrarySettings} from '@core/server-admin/libraries.ts';

export type SettingsLibrarySnapshot = Readonly<{libraryId?: string; document?: LibraryAdminDocument; loading: boolean; saving: boolean; error?: unknown}>;
const empty: SettingsLibrarySnapshot = Object.freeze({loading: false, saving: false});

/** One explicitly selected library. A settings revision never becomes a folder revision. */
export class SettingsLibrarySource {
  private state = empty;
  private request?: AbortController;
  private pending?: Promise<void>;
  private closed = false;
  constructor(private readonly api: LibraryContentApi, private readonly isCurrent: () => boolean, private readonly notify: () => void, private readonly reportError: (error: unknown) => void) {}
  private current = () => !this.closed && this.isCurrent();
  getSnapshot = () => this.current() ? this.state : empty;
  document = (libraryId?: string) => libraryId && this.current() && this.state.libraryId === libraryId ? this.state.document : undefined;
  private publish(state: SettingsLibrarySnapshot) {if (this.current()) {this.state = Object.freeze(state);this.notify();}}
  private owns(request: AbortController, libraryId: string) {return this.current() && !request.signal.aborted && this.request === request && this.state.libraryId === libraryId;}
  select = (libraryId?: string): Promise<void> => {
    if (!this.current()) return Promise.resolve();
    if (libraryId && this.state.libraryId === libraryId) {
      if (this.pending) return this.pending;
      if (this.state.document) return Promise.resolve();
    }
    this.request?.abort();this.pending = undefined;
    if (!libraryId) {this.publish(empty);return Promise.resolve();}
    const request = new AbortController();this.request = request;
    this.publish({libraryId, loading: true, saving: false});
    const pending = readLibrarySettings(this.api, libraryId, request.signal).then(document => {
      if (this.owns(request, libraryId)) this.publish({libraryId, document, loading: false, saving: false});
    }).catch(error => {
      if (this.owns(request, libraryId)) {this.publish({libraryId, loading: false, saving: false, error});this.reportError(error);}
    }).finally(() => {if (this.request === request) {this.request = undefined;this.pending = undefined;}});
    this.pending = pending;return pending;
  };
  read = (libraryId: string | undefined, rowId: string) => {
    const document = this.document(libraryId);
    return document && readLibrarySettingsBinding(rowId, document);
  };
  async change(libraryId: string, rowId: string, value: unknown): Promise<boolean> {
    const before = this.document(libraryId);
    if (!before || this.state.loading || this.state.saving) return false;
    const settings = patchLibrarySettingsBinding(rowId, value, before);
    const request = new AbortController();this.request = request;
    this.publish({libraryId, document: before, loading: false, saving: true});
    try {
      const document = await saveLibrarySettings(this.api, before, settings, randomId(), request.signal);
      if (this.owns(request, libraryId)) {this.publish({libraryId, document, loading: false, saving: false});return true;}
    } catch (error) {
      // A lost/conflicting response needs a genuine reread before another edit.
      if (this.owns(request, libraryId)) {this.publish({libraryId, loading: false, saving: false, error});throw error;}
    } finally {if (this.request === request) this.request = undefined;}
    return false;
  }
  async reset(libraryId: string, rowId: string): Promise<boolean> {
    const value = this.read(libraryId, rowId)?.default;
    return value !== undefined ? this.change(libraryId, rowId, value) : false;
  }
  dispose() {this.closed = true;this.request?.abort();this.request = undefined;this.pending = undefined;this.state = empty;}
}
