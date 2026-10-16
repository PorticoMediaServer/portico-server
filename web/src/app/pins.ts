import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {dependsOn} from '@core/freshness-hub.ts';
import {movedPin, readLibraryNavigation, readPinOrder, saveLibraryNavigation, savePinOrder, setResourcePin, withLibraryPinned, withPinRetry, type PinnedResource, type PinsApi} from '@core/library-pins.ts';
import type {Library} from '@core/index.ts';
import {useFreshness} from './freshness';

/**
 * Pins in navigation: the libraries a viewer pinned lead the rail in their order, and pinned
 * collections, saved views and playlists get a place of their own in it (on a phone, at the top of
 * Saved). The arrangement is the server's; this reads it, sends a whole new arrangement with the
 * revision it saw, and reads it again. Another device's change met on the way is re-read and the
 * same change applied once more, so a person never sees that conflict.
 */
export const PINS_CHANGED_EVENT = 'portico:pins-changed';
/** Said by any screen that pins or unpins something, so the rail follows at once. */
export function announcePinsChanged(): void {
  window.dispatchEvent(new Event(PINS_CHANGED_EVENT));
}

export type PinnedResourcesState = {order: readonly PinnedResource[]; revision: number; ready: boolean; reload: () => void};

/** The viewer's pinned collections, saved views and playlists, kept current while shown. */
export function usePinnedResources(api: PinsApi, serverId: string, sessionKey: string | undefined): PinnedResourcesState {
  const [state, setState] = useState<{order: readonly PinnedResource[]; revision: number; ready: boolean; key?: string}>({order: [], revision: 0, ready: false});
  const [tick, setTick] = useState(0);
  const reload = useCallback(() => setTick(n => n + 1), []);
  useEffect(() => {
    if (!sessionKey || !serverId) { setState({order: [], revision: 0, ready: false}); return; }
    const controller = new AbortController();
    readPinOrder(api, serverId, controller.signal)
      .then(read => setState({order: read.order, revision: read.revision, ready: true, key: sessionKey}))
      // A failed read keeps what is shown; the rail never empties for a moment's trouble.
      .catch(() => setState(current => (current.key === sessionKey ? current : {order: [], revision: 0, ready: true, key: sessionKey})));
    return () => controller.abort();
  }, [api, serverId, sessionKey, tick]);
  useEffect(() => {
    window.addEventListener(PINS_CHANGED_EVENT, reload);
    return () => window.removeEventListener(PINS_CHANGED_EVENT, reload);
  }, [reload]);
  // A pin made on another device arrives with this profile's other personal changes.
  const {hub} = useFreshness();
  useEffect(() => hub.watch([dependsOn.personal()], reload), [hub, reload]);
  return useMemo(() => ({order: state.order, revision: state.revision, ready: state.ready, reload}), [state, reload]);
}

const operationId = () => crypto.randomUUID();

/** The changes the rail and the pin editor make. Each resolves once the server has it. */
export function usePinActions(api: PinsApi, serverId: string) {
  const latest = useRef({api, serverId});
  latest.current = {api, serverId};
  return useMemo(() => ({
    /** Pins or unpins one library; a newly pinned one goes last among the pinned. */
    pinLibrary: async (libraryId: string, pin: boolean) => {
      const {api} = latest.current;
      await withPinRetry(() => readLibraryNavigation(api), navigation => saveLibraryNavigation(api, navigation.revision, withLibraryPinned(navigation.pinnedLibraryIds, libraryId, pin)));
      announcePinsChanged();
    },
    /** Moves a pinned library up or down among the pinned. */
    moveLibrary: async (libraryId: string, by: number) => {
      const {api} = latest.current;
      await withPinRetry(() => readLibraryNavigation(api), navigation => saveLibraryNavigation(api, navigation.revision, movedPin(navigation.pinnedLibraryIds, navigation.pinnedLibraryIds.indexOf(libraryId), by)));
      announcePinsChanged();
    },
    /** Moves a pinned collection, saved view or playlist up or down. */
    moveResource: async (entry: PinnedResource, by: number) => {
      const {api, serverId} = latest.current;
      await withPinRetry(() => readPinOrder(api, serverId), read => {
        const at = read.order.findIndex(other => other.kind === entry.kind && other.id === entry.id);
        return savePinOrder(api, serverId, read.revision, movedPin(read.order, at, by));
      });
      announcePinsChanged();
    },
    /** Unpins a collection, saved view or playlist from wherever its pin is shown. */
    unpinResource: async (entry: PinnedResource) => {
      const {api, serverId} = latest.current;
      await withPinRetry(() => readPinOrder(api, serverId).then(read => read.order.find(other => other.kind === entry.kind && other.id === entry.id) ?? entry), fresh => setResourcePin(api, fresh.kind, fresh.id, false, fresh.pinRevision, operationId()), entry);
      announcePinsChanged();
    },
  }), []);
}

/** The pinned libraries of a listing (the server lists them first, in the viewer's order). */
export function pinnedLibraryIds(libraries: readonly Library[]): readonly string[] {
  return libraries.filter(library => library.pinned === true).map(library => library.id);
}
