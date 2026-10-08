import type {SavedService} from '@core/saved.ts';
import {createJob, getAllJobFailures, pollJob, type JobFailure} from '@core/bulk-jobs.ts';

export type PlaylistAddResult = {ok: number; failed: readonly string[]};

type AddService = Pick<SavedService, 'mutate'>;
type JobsApi = {request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T>};

/**
 * PERF-S08/Q7: playlists have no batch add endpoint, so adding N items is one
 * request per item. Every item's outcome comes from its mutate: success
 * resolves (after its refresh, so the next add sees a ready service) and a
 * failed add rejects, leaving the queue so the remaining items still send.
 */
export async function addItemsToPlaylist(service: AddService, _playlistId: string, ids: readonly string[]): Promise<PlaylistAddResult> {
  void _playlistId;
  let ok = 0;
  const failed: string[] = [];
  for (const id of ids) {
    try {
      await service.mutate({action: 'add', itemId: id});
      ok++;
    } catch {
      failed.push(id);
    }
  }
  return {ok, failed};
}

export type PlaylistJobPlacement = 'end' | 'next' | {after: string};

/**
 * Large additions go through one `playlist-add` job per 200
 * items instead of one request per item. Placement `next` prepends (Justin's
 * decision); `{after: entryId}` is explicit placement; `end` appends.
 */
export async function addItemsToPlaylistViaJob(
  api: JobsApi,
  input: Readonly<{playlistId: string; expectedRevision: number; placement: PlaylistJobPlacement; ids: readonly string[]; operationIds: readonly string[]}>,
): Promise<PlaylistAddResult> {
  const distinct = [...new Set(input.ids)];
  if (!distinct.length) return {ok: 0, failed: []};
  if (!Number.isInteger(input.expectedRevision) || input.expectedRevision < 1) throw new Error('A playlist addition needs the current revision.');
  let ok = 0;
  let failed: JobFailure[] = [];
  for (let at = 0, job = 0; at < distinct.length; at += 200, job++) {
    const operationId = input.operationIds[job];
    if (!operationId) throw new Error('Each playlist job needs its own operationId.');
    const created = await createJob(api, {
      operationId, command: 'playlist-add', selector: {items: {ids: distinct.slice(at, at + 200)}},
      args: {playlistId: input.playlistId, expectedRevision: input.expectedRevision, placement: input.placement},
    });
    const finished = await pollJob(api, created.jobId, {intervalMs: 1500});
    if (finished.state === 'failed') throw Object.assign(new Error('The playlist addition could not finish.'), {code: finished.errorCode ?? 'job_failed'});
    ok += finished.done;
    if (finished.failed > 0) failed = [...failed, ...(await getAllJobFailures(api, created.jobId))];
  }
  return {ok, failed: failed.map(f => f.itemId)};
}
