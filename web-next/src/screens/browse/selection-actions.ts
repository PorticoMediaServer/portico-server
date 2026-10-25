import type {JobArgs, JobRequest} from '@core/bulk-jobs.ts';
import type {HomeApi} from '@core/home.ts';
import type {EventsClient} from '@core/playback-v1/events.ts';
import {selectionQuerySelector, type BrowseSelectionContext, type SelectionContext, type SelectionOperation, type SelectionTarget} from '@core/selection-model/index.ts';
import {runSelectionJobs, selectionJobTasks, type HomeJobTask} from '@core/home-selection/execution.ts';

/** The one place a library selection becomes real work. It hands the model's own
 * target to the existing job planner and runner, so explicit picks stay explicit
 * and a whole view stays the server's query - never a locally enumerated set. */

export type SelectionActionsOptions = Readonly<{
  api: HomeApi;
  events: Pick<EventsClient, 'on'>;
  requestId: () => Promise<string>;
  isCurrent: () => boolean;
  /** The route's lifetime; an absent signal is a private, never-aborted one. */
  signal?: AbortSignal;
  reportError: (error: unknown) => void;
}>;

export type SelectionActionResult = Readonly<{updated: number; failed: number; jobs: readonly string[]}>;

export type PersonalSelectionAction = Readonly<{
  watched?: boolean;
  favorite?: boolean;
  watchlist?: boolean;
}>;

const leafKinds: ReadonlySet<string> = new Set(['movie', 'episode', 'song', 'audiobook_file']);
/** Both selection contexts plan jobs the same way; only the selector/items matter. */
type AnyOperation = SelectionOperation<SelectionContext> | SelectionOperation<BrowseSelectionContext>;

/** A container's own favourite/watched state is a direct command in the canonical
 * dispatcher, not a job over its members. Until that executor is bound here, a
 * personal job is offered only for leaf picks and for the server's leaf views. */
export function personalTargetIsLeaf(target: AnyOperation['target']): boolean {
  if (target.mode === 'query') return true;
  return target.items.length > 0 && target.items.every(pick => leafKinds.has(pick.item.kind));
}

export class SelectionActionError extends Error {
  constructor(readonly code: string, readonly retryable = false) {
    super(code);
    this.name = 'SelectionActionError';
  }
}

export function createSelectionActions({api, events, requestId, isCurrent, reportError, signal: lifetime}: SelectionActionsOptions) {
  const routeSignal = lifetime ?? new AbortController().signal;
  const run = async (operation: AnyOperation, args: JobArgs, signal: AbortSignal): Promise<SelectionActionResult> => {
    const target = operation.target as SelectionTarget;
    if (!isCurrent() || signal.aborted) throw new SelectionActionError('cancelled');
    if (!personalTargetIsLeaf(target)) throw new SelectionActionError('container-personal-state-unsupported');
    const grant = args.watched !== undefined ? 'watched' : args.favorite !== undefined ? 'favorite' : args.watchlist !== undefined ? 'myList' : undefined;
    const offered = grant !== undefined && (target.mode === 'query'
      ? target.summary.every[grant] === true
      : target.items.length > 0 && target.items.every(pick => pick.item.capabilities[grant] === true));
    if (!grant || !offered) throw new SelectionActionError('selection-grant-unavailable');
    if (target.mode === 'query') {
      // A whole view dispatches only its unrefined, checked selector with the
      // exact catalog revision. Exclusions or a kind refinement cannot be dropped
      // silently, so they are refused before any operation id or request.
      const checked = selectionQuerySelector(target);
      if (!checked.ok) throw new SelectionActionError(checked.reason);
      const request: JobRequest = Object.freeze({operationId: (await requestId()).replaceAll('-', ''), command: 'personal-state',
        selector: checked.selector, args, expected: checked.expected});
      if (!isCurrent() || signal.aborted) throw new SelectionActionError('cancelled');
      const task: HomeJobTask = {request, failures: [], observed: false};
      await runSelectionJobs(api, [task], signal, isCurrent, () => {}, undefined, events);
      const job = task.job;
      if (!job) throw new SelectionActionError('job-receipt-required');
      return Object.freeze({updated: job.done, failed: job.failed, jobs: Object.freeze([job.jobId])});
    }
    const tasks = await selectionJobTasks(target, 'personal-state', args, requestId);
    if (!tasks.length) throw new SelectionActionError('empty-selection');
    await runSelectionJobs(api, tasks, signal, isCurrent, () => {}, undefined, events);
    const jobs = tasks.flatMap(task => task.job ? [task.job.jobId] : []);
    return Object.freeze({updated: tasks.reduce((sum, task) => sum + (task.job?.done ?? 0), 0),
      failed: tasks.reduce((sum, task) => sum + (task.job?.failed ?? 0), 0), jobs: Object.freeze(jobs)});
  };
  return {
    /** The three personal commands the drawn bar offers on a selection. */
    async personal(operation: AnyOperation, action: PersonalSelectionAction, signal: AbortSignal = routeSignal): Promise<SelectionActionResult> {
      if (action.watched === undefined && action.favorite === undefined && action.watchlist === undefined) throw new SelectionActionError('empty-command');
      if (action.watched !== undefined && action.favorite === undefined && action.watchlist === undefined) return run(operation, {watched: action.watched}, signal);
      if (action.favorite !== undefined && action.watched === undefined && action.watchlist === undefined) return run(operation, {favorite: action.favorite}, signal);
      if (action.watchlist !== undefined && action.watched === undefined && action.favorite === undefined) return run(operation, {watchlist: action.watchlist}, signal);
      // One job command carries one personal fact; a mixed request would need a
      // server command that does not exist.
      throw new SelectionActionError('mixed-personal-command');
    },
    reportError,
  };
}

export type SelectionActions = ReturnType<typeof createSelectionActions>;
