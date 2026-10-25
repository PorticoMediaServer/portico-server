import type {QueuePlaybackView} from '@core/queue-controller.ts';
import type {QueueIntent, QueueState} from '@core/queues.ts';
import type {QueuePlaybackFence} from '../../../../packages/player-core/src/index.ts';

export type WebQueueListEntry = Readonly<{
  entryId: string; title?: string; hidden: boolean; pending: boolean; playing: boolean;
  canPlay: boolean; canMoveUp: boolean; canMoveDown: boolean; canRemove: boolean;
}>;
export type WebQueueListSnapshot = Readonly<{
  current: boolean; fence?: QueuePlaybackFence; queueId?: string; revision?: string; currentEntryId?: string;
  entries: readonly WebQueueListEntry[]; more: number | null;
  change: 'idle' | 'sending' | 'failed'; error?: 'queue_changed' | 'queue_edit_failed';
}>;
export type WebQueueListPort = Readonly<{
  getSnapshot(): WebQueueListSnapshot; subscribe(listener: () => void): () => void; isCurrent(): boolean;
  play(snapshot: WebQueueListSnapshot, entryId: string): Promise<void>;
  move(snapshot: WebQueueListSnapshot, entryId: string, direction: -1 | 1): Promise<void>;
  remove(snapshot: WebQueueListSnapshot, entryId: string): Promise<void>;
}>;
type QueueListSource = Readonly<{
  fence: QueuePlaybackFence; queueId: string; isCurrent(): boolean;
  read(): Readonly<{view: QueuePlaybackView | null; workspace: QueueState}>;
  subscribe(listener: () => void): () => void;
  play(entryId: string, revision: string): Promise<void>;
  mutate(intent: QueueIntent): Promise<void>;
  refresh(): Promise<void>;
}>;
const empty: WebQueueListSnapshot = Object.freeze({current: false, entries: Object.freeze([]), more: null, change: 'idle'});
const changed = () => Object.assign(new Error('queue_changed'), {code: 'queue_changed'});

/** One captured queue run. A screen must pass the immutable snapshot that
 * rendered its command; neither an entry id nor the latest player is a grant. */
export function createWebQueueListPort(source: QueueListSource): WebQueueListPort {
  let sending = false, error: WebQueueListSnapshot['error'];
  let cache: {view: QueuePlaybackView; workspace: QueueState; sending: boolean; error: WebQueueListSnapshot['error']; snapshot: WebQueueListSnapshot} | undefined;
  const listeners = new Set<() => void>();
  const notify = () => {for (const listener of [...listeners]) listener();};
  const getSnapshot = (): WebQueueListSnapshot => {
    if (!source.isCurrent()) return empty;
    const {view, workspace} = source.read(), queue = view?.queue;
    if (!view || !queue || queue.id !== source.queueId || workspace.queue?.id !== queue.id || workspace.queue.revision !== queue.revision) return empty;
    if (workspace.phase === 'denied' || workspace.phase === 'fenced' || workspace.phase === 'expired') return empty;
    if (cache?.view === view && cache.workspace === workspace && cache.sending === sending && cache.error === error) return cache.snapshot;
    const all = queue.entries.filter(entry => !entry.removed), at = all.findIndex(entry => entry.id === queue.currentEntryId);
    if (at < 0) return empty;
    const shown = all.slice(at, at + 50), items = new Map(view.items.map(item => [item.entryId, item]));
    const busy = sending || workspace.phase === 'sending';
    const entries = Object.freeze(shown.map((entry, index): WebQueueListEntry => {
      const item = items.get(entry.id), readable = !entry.hidden && item !== undefined && item.itemId === entry.itemId;
      const playing = entry.id === queue.currentEntryId;
      const allowed = readable && !playing && !busy && workspace.phase === 'ready';
      return Object.freeze({entryId: entry.id, ...(readable ? {title: item.title} : {}), hidden: !readable,
        pending: entry.hidden && entry.pending === true, playing,
        canPlay: allowed, canMoveUp: allowed && index > 1, canMoveDown: allowed && index < shown.length - 1, canRemove: allowed});
    }));
    const remaining = view.remainingCount;
    const more = remaining !== undefined && Number.isSafeInteger(remaining) && remaining >= 0 ? Math.max(0, remaining - (shown.length - 1)) : null;
    const failure = error ?? (workspace.error ? 'queue_edit_failed' as const : undefined);
    const snapshot: WebQueueListSnapshot = Object.freeze({current: true, fence: source.fence, queueId: queue.id, revision: queue.revision,
      currentEntryId: queue.currentEntryId ?? undefined, entries, more, change: busy ? 'sending' : failure ? 'failed' : 'idle', ...(failure ? {error: failure} : {})});
    cache = {view, workspace, sending, error, snapshot};
    return snapshot;
  };
  const execute = async (snapshot: WebQueueListSnapshot, entryId: string, action: 'play' | 'remove' | -1 | 1) => {
    if (sending || !source.isCurrent() || snapshot !== getSnapshot() || !snapshot.current || !snapshot.revision || snapshot.queueId !== source.queueId) throw changed();
    const entry = snapshot.entries.find(row => row.entryId === entryId);
    if (!entry || (action === 'play' ? !entry.canPlay : action === 'remove' ? !entry.canRemove : action === -1 ? !entry.canMoveUp : !entry.canMoveDown)) throw changed();
    const {view} = source.read();
    if (!view || view.queue.revision !== snapshot.revision) throw changed();
    // Reordering is the existing service's single-entry move, anchored among
    // this real loaded window. It never replaces or drops the unseen remainder.
    const order = view.queue.entries.filter(row => !row.removed).map(row => row.id), index = order.indexOf(entryId);
    if (index < 0) throw changed();
    sending = true;error = undefined;notify();
    try {
      if (!source.isCurrent()) throw changed();
      const accepted = source.read();
      // Subscribers may run synchronously while the sending state publishes.
      // Their newer queue can never replace the command's captured witness.
      if (accepted.view !== view || accepted.workspace.queue?.id !== snapshot.queueId || accepted.workspace.queue.revision !== snapshot.revision) throw changed();
      if (action === 'play') await source.play(entryId, snapshot.revision);
      else if (action === 'remove') await source.mutate({action: 'remove', entryId});
      else {
        const to = index + action;
        if (to < 0 || to >= order.length) throw changed();
        [order[index], order[to]] = [order[to]!, order[index]!];
        await source.mutate({action: 'reorder', entryIds: order});
      }
    } catch (failure) {
      if (source.isCurrent()) {
        error = 'queue_edit_failed';
        // Reconcile the authoritative queue after an uncertain response. Never
        // automatically repeat the mutation or claim a local-only success.
        try {await source.refresh();} catch { /* Keep the failure visible. */ }
      }
      throw failure;
    } finally {sending = false;if (source.isCurrent()) notify();}
  };
  return Object.freeze({getSnapshot, isCurrent: () => getSnapshot().current,
    subscribe(listener: () => void) {listeners.add(listener);const off = source.subscribe(listener);return () => {listeners.delete(listener);off();};},
    play: (snapshot: WebQueueListSnapshot, entryId: string) => execute(snapshot, entryId, 'play'),
    remove: (snapshot: WebQueueListSnapshot, entryId: string) => execute(snapshot, entryId, 'remove'),
    move: (snapshot: WebQueueListSnapshot, entryId: string, direction: -1 | 1) => direction === -1 || direction === 1 ? execute(snapshot, entryId, direction) : Promise.reject(changed()),
  });
}
