import type {LibraryContentApi} from '@core/library-content.ts';
import {lanConfirmationRoots, previewRemoteSource, remoteSourceDraft, withConfirmedLanRoots, type RemoteSourceDraft, type RemoteSourcePreview} from '@core/linear-api.ts';
import {channelRequestID} from '@core/live-source-draft.ts';
import {normalizeHdHomerunLocator} from '@core/server-admin/panel-logic.ts';
import {createOperationIds} from '@core/server-admin/server-forms.ts';

export type RemoteSourceSnapshot = Readonly<{draft: RemoteSourceDraft; busy: boolean; preview?: RemoteSourcePreview; confirm?: readonly string[]; error?: unknown}>;
/** A preview belongs to one exact draft and identity. Editing any field
 * cancels its read and retires both the preview and pending LAN confirmation. */
export class RemoteSourceController {
  private snapshot: RemoteSourceSnapshot;
  private listeners = new Set<() => void>();
  private pending?: AbortController;
  private generation = 0;
  private disposed = false;
  private saving = false;
  private operations: ReturnType<typeof createOperationIds>;
  constructor(private api: LibraryContentApi, private serverId: string, private isCurrent: () => boolean,
    private reportError: (error: unknown) => void, makeId = channelRequestID, tuner?: string) {
    this.operations = createOperationIds(makeId);
    this.snapshot = {busy: false, draft: {...remoteSourceDraft(makeId()), viewerAccess: 'server-members',
      ...(tuner ? {kind: 'hdhomerun', locator: tuner, useDiscoveredCapacity: true} : {})}};
  }
  getSnapshot = () => this.snapshot;
  subscribe = (listener: () => void) => {this.listeners.add(listener);return () => {this.listeners.delete(listener);};};
  private current() {return !this.disposed && this.isCurrent();}
  private publish(value: RemoteSourceSnapshot) {if (!this.current()) return;this.snapshot = value;for (const listener of this.listeners) listener();}
  private failure(error: unknown) {
    if (!this.current()) return;
    if ([401, 403].includes((error as {status?: number})?.status ?? 0)) {this.reportError(error);this.dispose();return;}
    this.publish({...this.snapshot, busy: false, error});
  }
  edit(patch: Partial<RemoteSourceDraft>) {
    if (!this.current() || this.saving) return;
    ++this.generation;this.pending?.abort();
    const draft = {...this.snapshot.draft, ...patch};
    // A different endpoint must earn its own local-network confirmation.
    if (['kind', 'locator', 'guideUrl', 'username', 'password'].some(key => key in patch)) draft.confirmedLanRoots = [];
    this.publish({draft, busy: false});
  }
  async check(confirm = false) {
    if (!this.current() || this.snapshot.busy) return;
    const generation = ++this.generation;
    const pending = this.pending = new AbortController();
    let draft = this.snapshot.draft;
    if (!draft.name.trim() || !draft.locator.trim()) return;
    if (confirm) {
      if (!this.snapshot.confirm?.length) return;
      draft = withConfirmedLanRoots(draft, this.snapshot.confirm);
    }
    try {
      if (draft.kind === 'hdhomerun') draft = {...draft, locator: normalizeHdHomerunLocator(draft.locator)};
      this.publish({draft, busy: true});
      const preview = await previewRemoteSource(this.api, this.serverId, draft, pending.signal);
      if (!pending.signal.aborted && generation === this.generation) this.publish({draft, busy: false, preview});
    } catch (error) {
      if (pending.signal.aborted || generation !== this.generation || !this.current()) return;
      const roots = lanConfirmationRoots(error);
      if (roots) this.publish({draft, busy: false, confirm: roots});else this.failure(error);
    }
  }
  async save() {
    const preview = this.snapshot.preview;
    if (!this.current() || this.snapshot.busy || !preview) return false;
    if (Date.parse(preview.expiresAt) <= Date.now()) {this.publish({...this.snapshot, preview: undefined});return false;}
    const pending = this.pending = new AbortController();this.saving = true;
    this.publish({...this.snapshot, busy: true, error: undefined});
    try {
      await this.api.request('/v1/admin/live-sources/remote/save', 'POST', {previewId: preview.previewId,
        requestId: this.operations.forPayload(preview.previewId)}, pending.signal);
      if (!this.current() || pending.signal.aborted) return false;
      this.operations.release();this.publish({draft: remoteSourceDraft(''), busy: false});return true;
    } catch (error) {if (!pending.signal.aborted) this.failure(error);return false;}
    finally {this.saving = false;}
  }
  dispose() {this.disposed = true;++this.generation;this.pending?.abort();this.listeners.clear();this.snapshot = {draft: remoteSourceDraft(''), busy: false};}
}
