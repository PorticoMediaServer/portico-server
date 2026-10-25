import type {BulkEdit, BulkReceipt} from '@core/metadata-bulk.ts';
import {validBulkEdit, validateBulkReceipt} from '@core/metadata-bulk.ts';
import {fetchBulkMetadataPreview, metadataPreviewTargets, type BulkMetadataPreview, type BulkMetadataPreviewRequest} from '@core/home-selection/index.ts';
import type {HomeApi} from '@core/home.ts';
import type {EventsClient} from '@core/playback-v1/events.ts';
import type {BrowseSelectionContext, SelectionOperation, SelectionTarget} from '@core/selection-model/index.ts';
import type {BulkEditorDraft} from '../../kit/selection';
import {editorCoreDraft} from '../../shell/selection-presentation';
import {routePath} from '../../../../packages/contracts/src/routes.ts';

/** The library selection's bulk editor data path. It reuses the same preview
 * parser and job planner the Home editor uses, so a library edit and a Home edit
 * cannot disagree about fields, counts, revisions or receipts. */

export type SelectionEditorOptions = Readonly<{
  api: HomeApi;
  events: Pick<EventsClient, 'on'>;
  requestId: () => Promise<string>;
  isCurrent: () => boolean;
  serverId: string;
  signal?: AbortSignal;
}>;

type AnyOperation = SelectionOperation<BrowseSelectionContext>;
type BrowseTarget = SelectionTarget<BrowseSelectionContext>;

export class SelectionEditorError extends Error {
  constructor(readonly code: string, readonly retryable = false) {
    super(code);
    this.name = 'SelectionEditorError';
  }
}

/** Only explicit picks are edited: the server's whole-view edit path does not
 * exist yet, so a whole view is refused rather than approximated with loaded ids. */
export function editorPreviewRequest(target: BrowseTarget): BulkMetadataPreviewRequest {
  if (target.mode !== 'explicit' || !target.displayItems.length) throw new SelectionEditorError('whole-view-editor-unavailable');
  return {targets: metadataPreviewTargets(target.displayItems.map(pick => pick.item))};
}

export function createSelectionEditor({api, events, requestId, isCurrent, serverId, signal: lifetime}: SelectionEditorOptions) {
  const routeSignal = lifetime ?? new AbortController().signal;
  const current = (caller?: AbortSignal) => {
    if (!isCurrent() || routeSignal.aborted || caller?.aborted) throw new SelectionEditorError('cancelled');
  };
  return {
    async preview(operation: AnyOperation): Promise<BulkMetadataPreview> {
      current();
      const request = editorPreviewRequest(operation.target);
      return fetchBulkMetadataPreview(api, request, serverId, routeSignal);
    },
    /** The same direct bulk endpoint the Home editor uses, so each title gets its
     * own outcome and receipt instead of a job's aggregate counts. */
    async save(operation: AnyOperation, preview: BulkMetadataPreview, draft: BulkEditorDraft, caller?: AbortSignal): Promise<readonly BulkReceipt[]> {
      current(caller);
      const core = editorCoreDraft(draft), receipts: BulkReceipt[] = [];
      const targets = preview.targets;
      for (let start = 0; start < targets.length; start += 200) {
        const body: BulkEdit = {operationId: await requestId(),
          targets: targets.slice(start, start + 200).map(({kind, id, expectedRevision}) => ({kind, id, expectedRevision})), ...core};
        if (!validBulkEdit(body, preview.schema)) throw new SelectionEditorError('invalid-bulk-edit');
        current(caller);
        const raw = await api.request(routePath('post_metadata_bulk'), 'POST', body, routeSignal);
        current(caller);
        receipts.push(validateBulkReceipt(raw, body));
      }
      return Object.freeze(receipts);
    },
  };
}

export type SelectionEditor = ReturnType<typeof createSelectionEditor>;

/** A whole-view query target's selector is what the preview expects; exported for
 * the adapter's gate so it can refuse a refined view before opening an editor. */
export function editorTargetLawful(target: BrowseTarget | null | undefined): boolean {
  return !!target && target.mode === 'explicit' && target.displayItems.length > 0;
}

