import type {LibraryAdminDocument} from '@core/administration.ts';
import type {LibraryContentApi} from '@core/library-content.ts';
import {patchLibrarySettingsBinding, readLibrarySettingsBinding, patchLibrarySonicSettingsBinding, readLibrarySonicSettingsBinding} from '@core/presentation/settings-bindings.ts';
import {randomId} from '@core/random-id.ts';
import {readLibrarySettings, saveLibrarySettings} from '@core/server-admin/libraries.ts';
import {readLibrarySonic, retryLibrarySonic, saveLibrarySonic, type LibrarySonicDocument} from '@core/server-admin/library-sonic.ts';

export type SettingsLibrarySnapshot = Readonly<{libraryId?: string; document?: LibraryAdminDocument; sonic?: LibrarySonicDocument; loading: boolean; saving: boolean; error?: unknown}>;
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
  private offersSonic = (document: LibraryAdminDocument) => document.eligibleControls.includes('libraries.sonicAnalysis');
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
    const pending = readLibrarySettings(this.api, libraryId, request.signal).then(async document => {
      if (!this.owns(request, libraryId)) return;
      const sonic = this.offersSonic(document) ? await readLibrarySonic(this.api, libraryId, request.signal) : undefined;
      if (this.owns(request, libraryId)) this.publish({libraryId, document, sonic, loading: false, saving: false});
    }).catch(error => {
      if (this.owns(request, libraryId)) {this.publish({libraryId, loading: false, saving: false, error});this.reportError(error);}
    }).finally(() => {if (this.request === request) {this.request = undefined;this.pending = undefined;}});
    this.pending = pending;return pending;
  };
  read = (libraryId: string | undefined, rowId: string) => {
    const document = this.document(libraryId);
    if (rowId === 'libraries.sonicAnalysis') {
      const sonic = this.state.sonic;
      return document && sonic ? readLibrarySonicSettingsBinding(rowId, document, sonic) : undefined;
    }
    return document && readLibrarySettingsBinding(rowId, document);
  };
  /** Raw status is data for the future approved presenter, never generic JSON row copy. */
  sonicStatus = (libraryId: string) => {
    const document = this.document(libraryId), sonic = this.state.sonic;
    return document && sonic ? readLibrarySonicSettingsBinding('libraries.sonicStatus', document, sonic)?.current : undefined;
  };
  async change(libraryId: string, rowId: string, value: unknown): Promise<boolean> {
    if (rowId === 'libraries.sonicAnalysis') {
      if (typeof value !== 'boolean') throw new Error('Invalid Sonic setting value.');
      return this.sonicCommand(libraryId, value);
    }
    const before = this.document(libraryId);
    if (!before || this.state.loading || this.state.saving) return false;
    const settings = patchLibrarySettingsBinding(rowId, value, before);
    const request = new AbortController();this.request = request;
    this.publish({...this.state, libraryId, document: before, loading: false, saving: true});
    try {
      const document = await saveLibrarySettings(this.api, before, settings, randomId(), request.signal);
      if (this.owns(request, libraryId)) {this.publish({libraryId, document, sonic: this.offersSonic(document) ? this.state.sonic : undefined, loading: false, saving: false});return true;}
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
  retrySonic = (libraryId: string) => this.sonicCommand(libraryId);
  private async sonicCommand(libraryId: string, enabled?: boolean): Promise<boolean> {
    const document = this.document(libraryId), sonic = this.state.sonic;
    if (!document || !sonic || this.state.loading || this.state.saving || !readLibrarySonicSettingsBinding('libraries.sonicAnalysis', document, sonic)) return false;
    const patch = enabled === undefined ? undefined : patchLibrarySonicSettingsBinding('libraries.sonicAnalysis', enabled, document, sonic);
    const request = new AbortController();this.request = request;
    this.publish({...this.state, saving: true});
    try {
      const next = patch ? await saveLibrarySonic(this.api, sonic, patch.enabled, request.signal) : await retryLibrarySonic(this.api, libraryId, request.signal);
      if (this.owns(request, libraryId)) {this.publish({libraryId, document, sonic: next, loading: false, saving: false});return true;}
    } catch (error) {
      if (this.owns(request, libraryId)) {this.publish({libraryId, loading: false, saving: false, error});throw error;}
    } finally {if (this.request === request) this.request = undefined;}
    return false;
  }
  dispose() {this.closed = true;this.request?.abort();this.request = undefined;this.pending = undefined;this.state = empty;}
}
