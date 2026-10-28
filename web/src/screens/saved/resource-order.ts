import type {SavedIntent, SavedService} from '@core/saved.ts';

export type ResourceReorder = Extract<SavedIntent, {action: 'reorder'}>;
const changed = (message: string) => Object.assign(new Error(message), {code: 'revision_mismatch'});

export type ResourceOrderPreparation = Readonly<{prepare(): Promise<void>; current(): boolean}>;
/** The final authority check and invocation must share one synchronous turn:
 * preparation may yield to a newer resource revision before its caller resumes. */
export async function commitPreparedResourceOrder(preparation: ResourceOrderPreparation, mutate: () => Promise<void>): Promise<void> {
  await preparation.prepare();
  if (!preparation.current()) throw changed('The resource changed before its order could be saved.');
  await mutate();
}

/** Move one occurrence among known slots without dropping hidden occurrence ids. */
export function resourceOrderMove(ids: readonly string[], entryId: string, targetId: string): ResourceReorder | undefined {
  const from = ids.indexOf(entryId), to = ids.indexOf(targetId);
  if (from < 0 || to < 0 || from === to || new Set(ids).size !== ids.length) return undefined;
  const start = Math.min(from, to), end = Math.max(from, to), moved = ids.slice(start, end + 1);
  if (moved.length > 200) return undefined;
  moved.splice(from - start, 1); moved.splice(to - start, 0, entryId);
  return {action: 'reorder', entryIds: moved};
}

/** Core requires a revision-fenced order witness for a partial playlist page.
 * Read only enough opaque ids to cover the page already reached by the viewer;
 * never scan an entire large playlist just to move one visible occurrence. */
export async function preparePlaylistReorder(service: Pick<SavedService, 'getSnapshot' | 'loadOrder' | 'loadMoreOrder'>,
  intent: ResourceReorder, current: () => boolean): Promise<void> {
  const captured = service.getSnapshot(), resource = captured.playlist;
  if (!resource || !resource.actions.includes('reorder') || !current()) throw changed('Playlist order is no longer available.');
  const needed = [...intent.entryIds, ...(intent.afterEntryId ? [intent.afterEntryId] : [])];
  const check = () => {
    const value = service.getSnapshot(), now = value.playlist;
    if (!current() || !now || now.id !== resource.id || now.revision !== resource.revision || now.viewerFence !== resource.viewerFence
      || !now.actions.includes('reorder')) throw changed('The playlist changed. Reload its order before moving an entry.');
    return value;
  };
  const sections = captured.projection?.sections ?? [];
  if (sections.length === 1 && !sections[0].nextCursor && sections[0].entries.length === resource.entryCount
    && captured.projection?.playlistRevision === resource.revision && needed.every(id => sections[0].entries.some(entry => entry.id === id))) return;
  const verified = () => {
    const value = check(), order = value.order;
    return !!order && order.playlistId === resource.id && order.revision === resource.revision && order.viewerFence === resource.viewerFence
      && needed.every(id => order.entryIds.includes(id));
  };
  if (verified()) return;
  await service.loadOrder();
  if (check().orderError) throw check().orderError;
  const pageBudget = Math.ceil((captured.pagination.history.length * 40 + sections.reduce((sum, section) => sum + section.entries.length, 0)) / 100) + 1;
  const cursors = new Set<string>();
  for (let page = 1; !verified(); page++) {
    const next = check().order?.nextCursor;
    if (!next || cursors.has(next) || page >= pageBudget) throw changed('The current playlist order could not be verified. Reload the playlist before moving an entry.');
    cursors.add(next); await service.loadMoreOrder();
    if (check().orderError) throw check().orderError;
  }
}

/** EOF alone is insufficient on a restored/deep page: every occurrence must be present. */
export function resourceOrderComplete(ids: readonly string[], count: number, fromStart: boolean, cursors: readonly (string | null | undefined)[]): boolean {
  return fromStart && ids.length === count && new Set(ids).size === ids.length && cursors.every(cursor => !cursor);
}

export function resourceOrderEdge(ids: readonly string[], entryId: string, edge: 'top' | 'bottom', complete: boolean): ResourceReorder | undefined {
  if (!ids.includes(entryId) || new Set(ids).size !== ids.length) return;
  if (edge === 'top') return complete && ids[0] === entryId ? undefined : {action: 'reorder', entryIds: [entryId], afterEntryId: ''};
  const tail = ids.at(-1);
  if (!complete || !tail || tail === entryId) return;
  return {action: 'reorder', entryIds: [entryId], afterEntryId: tail};
}

/** Only announce a global position when the captured order proves it. */
export function resourceOrderPosition(intent: ResourceReorder, ids: readonly string[], entryId: string, complete: boolean): number | undefined {
  if (!intent.entryIds.includes(entryId)) return;
  if (intent.afterEntryId === '') return intent.entryIds.indexOf(entryId) + 1;
  if (!complete) return;
  const moved = new Set(intent.entryIds), remaining = ids.filter(id => !moved.has(id));
  if (intent.afterEntryId !== undefined) {
    const anchor = remaining.indexOf(intent.afterEntryId);
    if (anchor < 0) return;
    return anchor + 2 + intent.entryIds.indexOf(entryId);
  }
  let index = 0;
  const result = ids.map(id => moved.has(id) ? intent.entryIds[index++] : id);
  const position = result.indexOf(entryId);
  return position < 0 ? undefined : position + 1;
}
