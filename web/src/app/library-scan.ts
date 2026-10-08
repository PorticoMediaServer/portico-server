import {useCallback, useEffect, useMemo, useRef, useSyncExternalStore} from 'react';
import {fetchInventoryStatus, scanProgress} from '@core/library-management.ts';
import {LibraryScanTracker, parseLibraryScanEvent} from '@core/library-inventory.ts';
import {compatContentApi} from '@core/presentation/index.ts';
import type {HttpLocalApi} from '@core/index.ts';
import type {EventsClient} from '@core/playback-v1/events.ts';
import {useDeviceEvents} from './device-events';

/**
 * MU2: library scan progress from the device's one event feed, not polling.
 * One module-level `LibraryScanTracker` per `api` + `serverId` holds the
 * state; hooks register the ids they show (the tracker watches the union, so
 * the library screen unmounting never un watches the sidebar's ids) and read
 * through it. Requests are one `inventory-status` read per library when it
 * first comes on screen, one per watched library on resync/reconnect, and one
 * content refresh per `finished` for the library on screen. While the tab is
 * hidden nothing runs; the feed catches up on return.
 */

type TrackerEntry = {api: HttpLocalApi; serverId: string; tracker: LibraryScanTracker};
let active: TrackerEntry | undefined;
const registrations = new Map<number, readonly string[]>();
let nextRegistration = 1;
let wiring: {events: EventsClient; tracker: LibraryScanTracker; off: () => void} | undefined;
let wiredHooks = 0;

function union(): string[] {
  const ids = new Set<string>();
  for (const list of registrations.values()) for (const id of list) ids.add(id);
  return [...ids];
}

function ensureTracker(api: HttpLocalApi, serverId: string): LibraryScanTracker {
  if (active && active.api === api && active.serverId === serverId) return active.tracker;
  active?.tracker.dispose();
  const reader = compatContentApi(api) as Parameters<typeof fetchInventoryStatus>[0];
  const tracker = new LibraryScanTracker({
    read: (libraryId, signal) => fetchInventoryStatus(reader, serverId, libraryId, signal).then(scanProgress),
  });
  active = {api, serverId, tracker};
  tracker.watch(union());
  return tracker;
}

function wireFeed(events: EventsClient, tracker: LibraryScanTracker): () => void {
  const offEvent = events.on('library.scan.updated', raw => {
    const parsed = parseLibraryScanEvent(raw);
    if (parsed) tracker.apply(parsed);
  });
  const offResync = events.onResync(() => tracker.resync());
  let lastConnected = events.getStatus().connected;
  let everConnected = lastConnected;
  const offStatus = events.subscribeStatus(() => {
    const connected = events.getStatus().connected;
    if (connected && !lastConnected && everConnected) tracker.resync();
    if (connected) everConnected = true;
    lastConnected = connected;
  });
  // Catch up on anything missed before the feed existed.
  tracker.resync();
  return () => {
    offEvent();
    offResync();
    offStatus();
  };
}

/** Registers `ids` on the shared tracker and wires the feed once; every hook runs first. */
function useScanTracker(api: HttpLocalApi | undefined, serverId: string | undefined, ids: readonly string[]): LibraryScanTracker | undefined {
  const events = useDeviceEvents();
  const idRef = useRef(0);
  if (!idRef.current) idRef.current = nextRegistration++;
  const regId = idRef.current;
  const idsKey = ids.join('\n');
  const tracker = useMemo(() => {
    if (!api || !serverId) return undefined;
    return ensureTracker(api, serverId);
  }, [api, serverId]);
  useEffect(() => {
    const list = idsKey ? idsKey.split('\n') : [];
    registrations.set(regId, list);
    tracker?.watch(union());
    return () => {
      if (registrations.get(regId) === list) registrations.delete(regId);
    };
  }, [regId, idsKey, tracker]);
  useEffect(
    () => () => {
      registrations.delete(regId);
      active?.tracker.watch(union());
    },
    [regId],
  );
  useEffect(() => {
    if (!tracker || !events) return;
    if (!wiring || wiring.events !== events || wiring.tracker !== tracker) {
      wiring?.off();
      wiring = {events, tracker, off: wireFeed(events, tracker)};
    }
    wiredHooks++;
    return () => {
      wiredHooks--;
      if (wiredHooks <= 0) {
        wiredHooks = 0;
        wiring?.off();
        wiring = undefined;
      }
    };
  }, [tracker, events]);
  return tracker;
}

/**
 * One library's scan state for the header and empty state, from the
 * viewer-safe inventory-status read plus the event feed. Silent on error.
 * `finished` counts completed runs; screens refresh once when it changes.
 */
export function useLibraryScan(api: HttpLocalApi, serverId: string | undefined, libraryId: string | undefined): {scanning: boolean; found: number; finished: number} {
  const ids = useMemo(() => (libraryId ? [libraryId] : []), [libraryId]);
  const tracker = useScanTracker(api, serverId, ids);
  const subscribe = useCallback((cb: () => void) => tracker?.subscribe(cb) ?? (() => {}), [tracker]);
  const version = useSyncExternalStore(subscribe, () => tracker?.getSnapshot() ?? 0);
  void version;
  if (!tracker || !libraryId) return {scanning: false, found: 0, finished: 0};
  const state = tracker.get(libraryId);
  return {scanning: state.scanning, found: state.found, finished: tracker.finished(libraryId)};
}

/**
 * Scan counts for every sidebar library, keyed by id. One small status read
 * per library when it first appears (libraries are few; items are never
 * listed); a new map only when membership or counts change, so the rail keeps
 * stable props while scans run.
 */
export function useSidebarScans(api: HttpLocalApi, serverId: string | undefined, libraryIds: readonly string[]): ReadonlyMap<string, number> {
  const idsKey = libraryIds.join('\n');
  const ids = useMemo(() => (idsKey ? idsKey.split('\n') : []), [idsKey]);
  const tracker = useScanTracker(api, serverId, ids);
  const subscribe = useCallback((cb: () => void) => tracker?.subscribe(cb) ?? (() => {}), [tracker]);
  const version = useSyncExternalStore(subscribe, () => tracker?.getSnapshot() ?? 0);
  void version;
  const previous = useRef<ReadonlyMap<string, number>>(new Map());
  if (!tracker) {
    if (previous.current.size) previous.current = new Map();
    return previous.current;
  }
  const next = new Map<string, number>();
  for (const id of ids) {
    const state = tracker.get(id);
    if (state.scanning) next.set(id, state.found);
  }
  const before = previous.current;
  const unchanged = before.size === next.size && [...next].every(([id, found]) => before.get(id) === found);
  if (!unchanged) previous.current = next;
  return previous.current;
}
