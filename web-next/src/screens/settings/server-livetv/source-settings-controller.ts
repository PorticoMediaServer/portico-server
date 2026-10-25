import type {LibraryContentApi} from '@core/library-content.ts';
import {parseEnvelope, parseLiveSourceDocument, type LiveSourceDocument, type LiveSourceSettings} from '@core/administration.ts';
import {rebaseDraft} from '@core/freshness-actions.ts';
import {channelRequestID} from '@core/live-source-draft.ts';
import {OwnerStore, errorFacts, terminal} from '../../live/owner/lifetime';
export type SourceSettingsSnapshot = Readonly<{loading: boolean; saving: boolean; document?: LiveSourceDocument; draft?: LiveSourceSettings; error?: unknown}>;

/** One mounted source configuration, over the existing administration document.
 * The list remains the sole source directory; this model never invents one. */
export class SourceSettingsController extends OwnerStore<SourceSettingsSnapshot> {
  private operations = new Map<string, string>();
  constructor(private api: LibraryContentApi, private serverId: string, readonly sourceId: string, current: () => boolean,
    reportError: (error: unknown) => void, private makeId = channelRequestID) {
    super(Object.freeze({loading: false, saving: false}), current, reportError);
    if (!/^[a-f0-9]{48}$/.test(sourceId)) throw new Error('live_source_identity_invalid');
  }
  private path() {return `/v1/admin/live-sources/${encodeURIComponent(this.sourceId)}/configuration`;}
  private checked(raw: unknown) {
    const doc = parseEnvelope(raw, this.serverId, parseLiveSourceDocument).result;
    if (doc.sourceId !== this.sourceId) throw new Error('live_source_identity_changed');
    return doc;
  }
  async load() {
    if (!this.isActive() || this.snapshot.saving) return;
    const attempt = this.begin();this.publish({...this.snapshot, loading: true, error: undefined});if (!this.valid(attempt)) return;
    try {
      const raw = await this.api.request(this.path(), 'GET', undefined, attempt.abort.signal);if (!this.valid(attempt)) return;
      const document = this.checked(raw);this.publish({loading: false, saving: false, document, draft: document.settings});
    } catch (error) {if (this.valid(attempt)) this.fail(error, {...this.snapshot, loading: false, error: errorFacts(error)});}
  }
  edit(patch: Partial<LiveSourceSettings>) {
    if (!this.isActive() || this.snapshot.saving || !this.snapshot.document || !this.snapshot.draft) return;
    const draft = parseLiveSourceDocument({...this.snapshot.document, settings: {...this.snapshot.draft, ...patch}}).settings;
    this.publish({...this.snapshot, draft, error: undefined});
  }
  discard() {if (this.isActive() && !this.snapshot.saving && this.snapshot.document) this.publish({...this.snapshot, draft: this.snapshot.document.settings, error: undefined});}
  async save() {
    const {document, draft} = this.snapshot;
    if (!this.isActive() || this.snapshot.loading || this.snapshot.saving || !document || !draft || JSON.stringify(document.settings) === JSON.stringify(draft)) return false;
    const attempt = this.begin();this.publish({...this.snapshot, saving: true, error: undefined});
    const put = async (revision: number, settings: LiveSourceSettings) => {
      if (!this.valid(attempt)) throw {code: 'cancelled'};
      const key = JSON.stringify({revision, settings});let operationId = this.operations.get(key);
      if (!operationId) {if (this.operations.size >= 128) throw {code: 'server_busy'};operationId = this.makeId();this.operations.set(key, operationId);}
      if (!this.valid(attempt)) throw {code: 'cancelled'};
      return this.checked(await this.api.request(this.path(), 'PUT', {expectedRevision: revision, operationId, settings}, attempt.abort.signal));
    };
    try {
      if (!this.valid(attempt)) return false;
      let saved: LiveSourceDocument;
      try {saved = await put(document.revision, draft);}
      catch (error) {
        if (!this.valid(attempt) || terminal(error) || errorFacts(error).status !== 409) throw error;
        const raw = await this.api.request(this.path(), 'GET', undefined, attempt.abort.signal);if (!this.valid(attempt)) return false;
        const fresh = this.checked(raw), rebased = rebaseDraft(document.settings, draft, fresh.settings);
        if (rebased.conflict) throw error;
        saved = await put(fresh.revision, rebased.merged);
      }
      if (!this.valid(attempt)) return false;
      this.publish({loading: false, saving: false, document: saved, draft: saved.settings});return this.valid(attempt);
    } catch (error) {if (this.valid(attempt)) this.fail(error, {...this.snapshot, saving: false, error: errorFacts(error)});return false;}
  }
  override dispose() {this.operations?.clear();super.dispose();}
}
