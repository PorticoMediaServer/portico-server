import type {JobArgs, JobCommand} from '@core/bulk-jobs.ts';
import {createJob, getAllJobFailures, getJob, isJobTerminal, runBulkJobs, type BulkJobsApi, type BulkOutcome, type BulkProgress} from '@core/bulk-jobs.ts';
import type {SelectionTarget} from '../../app/selection.tsx';

/**
 * One bulk action as jobs: every bulk personal-state, trash,
 * refresh, playlist-add, collection-add and metadata-edit action sends one
 * `POST /v1/jobs` per 200 explicit items — usually exactly one — then polls
 * `GET /v1/jobs/{id}` for progress and reads the failures page for the
 * currently visible per-item failures. PERF-S09: Select-all with nothing
 * deselected sends the screen's whole-set target (one query or container
 * selector, fenced with `expected.catalogRevision`) as exactly one job, and
 * falls back to the chunked items path when the set moved
 * (`selection_changed`).
 *
 * M25-4: the chunked runner lives in client-core (`bulk-jobs.ts`, shared with
 * Apple); this module re-exports it so existing web callers keep importing
 * `./bulk-job`. The whole-set runner lives here because client-core's
 * `runBulkRequests` doesn't forward `expected`, and because the decision —
 * whole set versus explicit items — is the SelectionBar's own.
 */
export {
  bulkFailureCounts,
  groupMetadataTargets,
  runBulkJobs,
  runBulkRequests,
  type BulkOutcome,
  type BulkProgress,
  type BulkRequest,
} from '@core/bulk-jobs.ts';
export type {JobArgs, JobCommand};

/**
 * Whether a bulk submit may send the whole-set target in one job: the screen
 * named the set behind its grid, every visible entry is selected, and nothing
 * is deselected (the chosen set equals the visible set). Anything else —
 * including a partial page — submits the explicit items. Pure so node tests
 * cover it (`test/bulk-select-all.test.ts`); the SelectionBar calls it on
 * every submit.
 */
export function decideBulkTarget(target: SelectionTarget | null | undefined, visibleIds: readonly string[], selectedIds: ReadonlySet<string>, wholeSet = false): {kind: 'whole-set'; target: SelectionTarget} | {kind: 'items'} {
  // Only the viewer's explicit "Select all N" means the whole set: choosing every visible item
  // by hand (or Cmd/Ctrl+A) acts on exactly those items.
  if (!wholeSet || !target || !visibleIds.length) return {kind: 'items'};
  const visible = new Set(visibleIds);
  if (selectedIds.size !== visible.size) return {kind: 'items'};
  for (const id of visible) if (!selectedIds.has(id)) return {kind: 'items'};
  return {kind: 'whole-set', target};
}

/** Poll cadence for one whole-set job, mirroring client-core's chunked runner. */
const WHOLE_SET_POLL_MS = 1500;
const WHOLE_SET_TIMEOUT_MS = 300000;

const sleep = (ms: number, signal?: AbortSignal) => new Promise<void>((resolve, reject) => {
  const timer = setTimeout(resolve, ms);
  signal?.addEventListener('abort', () => { clearTimeout(timer); reject(Object.assign(new Error('The bulk action was cancelled.'), {code: 'cancelled'})); }, {once: true});
});

/**
 * Submits one whole-set bulk job: the screen's query or container selector
 * with `expected.catalogRevision` (the known revision, else one lazy read
 * through the target's resolver — one tiny browse request, never a library
 * enumeration), then polls it like the chunked runner. A set that moved
 * before capture throws `selection_changed` before any mutation, and the
 * caller falls back to the explicit items; anything else throws as usual.
 */
export async function runWholeSetJob(
  api: BulkJobsApi,
  command: JobCommand,
  args: JobArgs,
  target: SelectionTarget,
  operationId: string,
  onProgress?: (progress: BulkProgress) => void,
  signal?: AbortSignal,
): Promise<BulkOutcome> {
  const revision = target.catalogRevision ?? await target.resolveRevision?.();
  if (!revision) throw Object.assign(new Error('The whole set changed. Select all again to refresh it.'), {code: 'selection_changed'});
  const created = await createJob(api, {operationId, command, selector: target.selector, args, expected: {catalogRevision: revision}}, signal);
  const started = Date.now();
  let current = created;
  for (;;) {
    if (signal?.aborted) throw Object.assign(new Error('The bulk action was cancelled.'), {code: 'cancelled'});
    current = await getJob(api, created.jobId, signal);
    onProgress?.({phase: current.totalKnown ? 'working' : 'preparing', done: current.done, total: current.total, totalKnown: current.totalKnown});
    if (isJobTerminal(current.state)) break;
    if (Date.now() - started > WHOLE_SET_TIMEOUT_MS) throw Object.assign(new Error('The bulk action is still running. Reopen it to check the result.'), {code: 'job_poll_timeout'});
    await sleep(WHOLE_SET_POLL_MS, signal);
  }
  if (current.state === 'failed') throw Object.assign(new Error('The bulk action could not finish.'), {code: current.errorCode ?? 'job_failed', job: current});
  const failed = current.failed > 0 ? [...await getAllJobFailures(api, created.jobId, signal)] : [];
  return {ok: current.done, failed: Object.freeze(failed), jobs: 1};
}

/**
 * One bulk submit for the SelectionBar: the whole-set target in exactly one
 * job when Select-all covers it (`decideBulkTarget`), else the chunked items
 * path. A whole-set job refused with `selection_changed` retries once as
 * explicit items with fresh operation ids — the set moved, so the viewer sees
 * the items they have on screen acted on.
 */
export async function runBulkSelection(
  api: BulkJobsApi,
  command: JobCommand,
  args: JobArgs,
  selection: Readonly<{visibleIds: readonly string[]; selectedIds: ReadonlySet<string>; target?: SelectionTarget | null; wholeSet?: boolean}>,
  newOperationId: () => string,
  onProgress?: (progress: BulkProgress) => void,
  signal?: AbortSignal,
): Promise<BulkOutcome> {
  const whole = decideBulkTarget(selection.target ?? null, selection.visibleIds, selection.selectedIds, selection.wholeSet === true);
  if (whole.kind === 'whole-set') {
    try {
      return await runWholeSetJob(api, command, args, whole.target, newOperationId(), onProgress, signal);
    } catch (e) {
      if ((e as {code?: unknown} | null)?.code !== 'selection_changed') throw e;
    }
  }
  const ids = [...selection.selectedIds];
  const jobs = Math.max(1, Math.ceil(ids.length / 200));
  return runBulkJobs(api, command, args, ids, Array.from({length: jobs}, newOperationId), onProgress, signal);
}
