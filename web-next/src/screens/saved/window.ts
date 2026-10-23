import type {WindowedCollection, CollectionSnapshot} from '@core/collections/windowed.ts';
import type {SavedEntry, SavedService, SavedSnapshot} from '@core/saved.ts';

/** Presentation of the service's already bounded pages, with one non-entity slot
 * for its real continuation. No offset is invented for an opaque server cursor. */
export function savedSectionWindow(service: SavedService, sectionId: string): WindowedCollection<SavedEntry> {
  let prior: SavedSnapshot | undefined, version = 0, snapshot: CollectionSnapshot;
  let disposed = false;
  const subscriptions = new Set<() => void>();
  const section = () => service.getSnapshot().projection?.sections.find(value => value.id === sectionId);
  const getSnapshot = (): CollectionSnapshot => {
    const current = service.getSnapshot();
    if (current !== prior) {
      prior = current; version++;
      const shown = section();
      snapshot = Object.freeze({generation: current.generation, total: shown ? shown.entries.length + Number(!!shown.nextCursor) : current.phase === 'ready' ? 0 : undefined,
        totalIsLowerBound: !!shown?.nextCursor, status: current.phase === 'error' || current.phase === 'refresh-required' ? 'error' : current.phase === 'idle' ? 'idle' : shown ? 'ready' : 'loading',
        error: current.error, positionIndex: undefined, refreshing: current.phase === 'loading' && !!shown});
    }
    return snapshot;
  };
  const subscribe = (listener: () => void) => {
    if (disposed) return () => {};
    const off = service.subscribe(listener); subscriptions.add(off);
    return () => { off(); subscriptions.delete(off); };
  };
  return {
    pageSize: 40, getSnapshot, subscribe,
    subscribePage: (_page, listener) => subscribe(listener),
    getPageVersion: () => { getSnapshot(); return String(version); },
    pageOf: index => Math.floor(index / 40),
    itemAt: index => section()?.entries[index],
    keyAt: index => section()?.entries[index]?.id ?? `placeholder:${index}`,
    slotAt: index => {
      const item = section()?.entries[index];
      return item ? {kind: 'item', index, key: item.id, item} : {kind: 'placeholder', index, key: `placeholder:${index}`};
    },
    ensureRange: (_first, last) => {
      if (disposed) return;
      const shown = section();
      if (shown?.nextCursor && service.getSnapshot().phase === 'ready' && last >= Math.max(0, shown.entries.length - 10)) void service.next(sectionId);
    },
    indexOfLetter: () => undefined,
    invalidate: () => { if (!disposed) void service.refresh(); },
    revalidate: options => disposed ? Promise.resolve() : service.revalidate({settle: options?.hold}),
    retry: () => { if (!disposed) void service.retry(); },
    resident: () => ({pages: Math.ceil((section()?.entries.length ?? 0) / 40), items: section()?.entries.length ?? 0}),
    residentItems: () => section()?.entries ?? [],
    dispose: () => { disposed = true; for (const off of subscriptions) off(); subscriptions.clear(); },
  };
}
