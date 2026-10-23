import type {WindowedCollection, CollectionSnapshot} from '@core/collections/windowed.ts';
import type {SavedEntry, SavedService, SavedSnapshot} from '@core/saved.ts';
import type {PersonalResourceEntry, PersonalSavedService, PersonalSavedSnapshot} from '@core/personal-saved.ts';

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

/** Personal resources keep their real cursor and resident entries. The one
 * extra slot invites that continuation; it never claims a directory total. */
export function personalEntriesWindow(service: PersonalSavedService): WindowedCollection<PersonalResourceEntry> {
  let prior: PersonalSavedSnapshot | undefined, version = 0, generation = 0, snapshot: CollectionSnapshot;
  let disposed = false;
  const subscriptions = new Set<() => void>();
  const getSnapshot = (): CollectionSnapshot => {
    const current = service.getSnapshot();
    if (current !== prior) {
      if (prior?.route !== current.route) generation++;
      prior = current; version++;
      snapshot = Object.freeze({generation, total: current.loading && !current.entries.length ? undefined : current.entries.length + Number(!!current.nextCursor),
        totalIsLowerBound: !!current.nextCursor, status: current.error ? 'error' : current.loading && !current.entries.length ? 'loading' : 'ready',
        error: current.error, positionIndex: undefined, refreshing: current.loading && !!current.entries.length});
    }
    return snapshot;
  };
  const subscribe = (listener: () => void) => {
    if (disposed) return () => {};
    const off = service.subscribe(listener); subscriptions.add(off);
    return () => { off(); subscriptions.delete(off); };
  };
  return {
    pageSize: 40, getSnapshot, subscribe, subscribePage: (_page, listener) => subscribe(listener),
    getPageVersion: () => { getSnapshot(); return String(version); }, pageOf: index => Math.floor(index / 40),
    itemAt: index => service.getSnapshot().entries[index], keyAt: index => service.getSnapshot().entries[index]?.id ?? `placeholder:${index}`,
    slotAt: index => { const item = service.getSnapshot().entries[index]; return item ? {kind: 'item', index, key: item.id, item} : {kind: 'placeholder', index, key: `placeholder:${index}`}; },
    ensureRange: (_first, last) => {
      const current = service.getSnapshot();
      if (!disposed && !current.loading && !current.error && current.nextCursor && last >= Math.max(0, current.entries.length - 10)) void service.next();
    },
    indexOfLetter: () => undefined,
    invalidate: () => { if (!disposed) void service.refresh(); },
    revalidate: options => disposed ? Promise.resolve() : service.revalidate({settle: options?.hold}),
    retry: () => { if (!disposed) void service.retryRead(); },
    resident: () => ({pages: Math.ceil(service.getSnapshot().entries.length / 40), items: service.getSnapshot().entries.length}),
    residentItems: () => service.getSnapshot().entries,
    dispose: () => { disposed = true; for (const off of subscriptions) off(); subscriptions.clear(); },
  };
}
