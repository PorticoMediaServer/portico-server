import type {JobArgs} from '@core/bulk-jobs.ts';
import {fetchBulkMetadataPreview, metadataPreviewTargets, type BulkMetadataPreview, type BulkMetadataPreviewRequest} from '@core/home-selection/index.ts';
import {runSelectionJobs, selectionJobTasks, type HomeJobTask} from '@core/home-selection/execution.ts';
import type {HomeApi} from '@core/home.ts';
import type {EventsClient} from '@core/playback-v1/events.ts';
import type {BrowseSelectionContext, SelectionOperation, SelectionTarget} from '@core/selection-model/index.ts';
import {selectionQuerySelector} from '@core/selection-model/index.ts';
import type {BulkEditorDraft} from '../../kit/selection';

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
/** selectionQuerySelector ignores the context; the wire target is identical. */
const querySelector = (target: BrowseTarget) => selectionQuerySelector(target as never);

export class SelectionEditorError extends Error {
  constructor(readonly code: string, readonly retryable = false) {
    super(code);
    this.name = 'SelectionEditorError';
  }
}

/** A whole view is edited through the server's query selector; explicit picks go
 * as their exact displayed identities. A refined query is refused: the job
 * selector cannot carry the kind/exclusion refinement. */
export function editorPreviewRequest(target: BrowseTarget): BulkMetadataPreviewRequest {
  if (target.mode === 'explicit') {
    return {targets: metadataPreviewTargets(target.displayItems.map(pick => pick.item))};
  }
  const checked = querySelector(target);
  if (!checked.ok) throw new SelectionEditorError(checked.reason);
  return {selector: checked.selector, expected: checked.expected};
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
    /** One metadata-edit job over the same target, carrying only changed values. */
    async save(operation: AnyOperation, draft: BulkEditorDraft, caller?: AbortSignal): Promise<Readonly<{updated: number; failed: number; jobs: readonly string[]}>> {
      current(caller);
      const target = operation.target;
      const args: JobArgs = {fields: {...draft.fields}, lists: Object.fromEntries(Object.entries(draft.lists).map(([field, value]) => [field, value])), lockEdited: draft.lockEdited};
      let tasks: HomeJobTask[];
      if (target.mode === 'query') {
        // A whole view is fenced to the revision the page was read at, so an edit
        // cannot land on a different set of titles than the one the person saw.
        const checked = querySelector(target);
        if (!checked.ok) throw new SelectionEditorError(checked.reason);
        tasks = [{request: {operationId: (await requestId()).replaceAll('-', ''), command: 'metadata-edit', selector: checked.selector, args, expected: {catalogRevision: checked.expected.catalogRevision}}, failures: [], observed: false}];
      } else {
        // The planner reads only the selector and identities; the context type differs.
        tasks = await selectionJobTasks(target as never, 'metadata-edit', args, requestId);
      }
      if (!tasks.length) throw new SelectionEditorError('empty-selection');
      await runSelectionJobs(api, tasks, AbortSignal.any([routeSignal, ...(caller ? [caller] : [])]), isCurrent, () => {}, undefined, events);
      return Object.freeze({updated: tasks.reduce((sum, task) => sum + (task.job?.done ?? 0), 0),
        failed: tasks.reduce((sum, task) => sum + (task.job?.failed ?? 0), 0), jobs: Object.freeze(tasks.flatMap(task => task.job ? [task.job.jobId] : []))});
    },
  };
}

export type SelectionEditor = ReturnType<typeof createSelectionEditor>;

/** A whole-view query target's selector is what the preview expects; exported for
 * the adapter's gate so it can refuse a refined view before opening an editor. */
export function editorTargetLawful(target: BrowseTarget | null | undefined): boolean {
  if (!target) return false;
  if (target.mode === 'explicit') return target.displayItems.length > 0;
  return querySelector(target).ok;
}

