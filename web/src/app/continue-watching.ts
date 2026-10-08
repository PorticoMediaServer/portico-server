/**
 * X-12: Remove from Continue Watching, on the server's own dismissal (server 0c1d30d2,
 * api/home.md): client-core `setContinueDismissed` sends `{continueDismissed: true}`, which takes
 * the title off this profile's row until it's next played. It isn't marked watched and its resume
 * point stays. Undo sends `false`.
 *
 * The removal is optimistic: the card menu announces it on a window event, Home hides the card at
 * once and offers Undo; a refused request puts it back.
 */
import {setContinueDismissed} from '@core/continue-watching.ts';

type Api = {request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T>};

export const CONTINUE_REMOVED = 'portico:continue-removed';
export type ContinueRemoval = {itemId: string; title: string; phase: 'pending' | 'done' | 'failed'; undo: () => Promise<void>};

const operationId = () => crypto.randomUUID();

/** Dismisses the title from Continue Watching; the returned undo restores it. */
export async function removeFromContinueWatching(api: Api, itemId: string, revision?: number): Promise<{undo: () => Promise<void>}> {
  const done = await setContinueDismissed(api, {itemId, dismissed: true, revision, operationId});
  return {undo: async () => { await setContinueDismissed(api, {itemId, dismissed: false, revision: done.revision, operationId}); }};
}

export function announceRemoval(removal: ContinueRemoval) {
  if (typeof window !== 'undefined') window.dispatchEvent(new CustomEvent<ContinueRemoval>(CONTINUE_REMOVED, {detail: removal}));
}
