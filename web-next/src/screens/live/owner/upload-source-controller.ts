import type {LibraryContentApi} from '@core/library-content.ts';
import {LiveSourceDraft, type LiveSourceInput} from '@core/live-source-draft.ts';
export type UploadPreview = Readonly<{channels: number; programmes: number; names?: readonly string[]}>;
export type UploadSnapshot = Readonly<{draft: LiveSourceInput; reading: boolean; busy: boolean; preview?: UploadPreview; error?: unknown}>;
/** A file read and its preview share the same single-viewer lifetime. The
 * existing draft enforces byte limits and discards superseded file answers. */
export class UploadSourceController {
  private draft = new LiveSourceDraft();
  private snapshot: UploadSnapshot = {draft: this.draft.getSnapshot(), reading: false, busy: false};
  private listeners = new Set<() => void>();
  private pending?: AbortController;
  private disposed = false;
  private saving = false;
  private generation = 0;
  private off: () => void;
  constructor(private api: LibraryContentApi, private serverId: string, private isCurrent: () => boolean, private reportError: (error: unknown) => void) {
    this.off = this.draft.subscribe(() => {this.publish({...this.snapshot, draft: this.draft.getSnapshot(), reading: this.draft.isReading, preview: undefined});});
  }
  getSnapshot = () => this.snapshot;
  subscribe = (listener: () => void) => {this.listeners.add(listener);return () => {this.listeners.delete(listener);};};
  private current() {return !this.disposed && this.isCurrent();}
  private publish(value: UploadSnapshot) {if (!this.current()) return;this.snapshot = value;for (const listener of this.listeners) listener();}
  private failure(error: unknown) {
    if (!this.current()) return;
    if ([401, 403].includes((error as {status?: number})?.status ?? 0)) {this.reportError(error);this.dispose();return;}
    this.publish({...this.snapshot, busy: false, error});
  }
  edit(patch: Pick<Partial<LiveSourceInput>, 'name' | 'tunerCount'>) {
    if (!this.current() || this.saving) return;
    ++this.generation;this.pending?.abort();this.draft.update(patch);this.publish({...this.snapshot, busy: false, error: undefined});
  }
  async read(field: 'playlist' | 'guide', read: () => Promise<string>) {
    if (!this.current() || this.saving) return;
    ++this.generation;this.pending?.abort();this.publish({...this.snapshot, busy: false, preview: undefined, error: undefined});
    try {await this.draft.readFile(field, read);} catch (error) {this.failure(error);}
  }
  async check() {
    if (!this.current() || this.snapshot.busy || this.draft.isReading || !this.snapshot.draft.name.trim() || !this.snapshot.draft.playlist) return;
    const generation = ++this.generation, pending = this.pending = new AbortController();
    this.publish({...this.snapshot, busy: true, error: undefined, preview: undefined});
    try {
      const raw = await this.api.request<{serverId: string; preview: UploadPreview}>('/v1/admin/live-sources/preview', 'POST', this.snapshot.draft, pending.signal);
      if (raw.serverId !== this.serverId || !raw.preview || !Number.isSafeInteger(raw.preview.channels) || raw.preview.channels < 0 || !Number.isSafeInteger(raw.preview.programmes) || raw.preview.programmes < 0) throw new Error('live_upload_preview_invalid');
      if (!pending.signal.aborted && generation === this.generation) this.publish({...this.snapshot, busy: false, preview: raw.preview});
    } catch (error) {if (!pending.signal.aborted && generation === this.generation) this.failure(error);}
  }
  async save() {
    if (!this.current() || this.snapshot.busy || this.draft.isReading || !this.snapshot.preview) return false;
    const pending = this.pending = new AbortController();this.saving = true;this.publish({...this.snapshot, busy: true, error: undefined});
    try {
      await this.api.request('/v1/admin/live-sources', 'POST', this.snapshot.draft, pending.signal);
      if (!this.current() || pending.signal.aborted) return false;
      this.publish({...this.snapshot, busy: false, preview: undefined});return true;
    } catch (error) {if (!pending.signal.aborted) this.failure(error);return false;}
    finally {this.saving = false;}
  }
  dispose() {this.disposed = true;++this.generation;this.pending?.abort();this.off();this.draft.dispose();this.listeners.clear();}
}
