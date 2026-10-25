import {useSyncExternalStore} from 'react';
import type {ContentEntry} from '@core/library-content.ts';
import type {SearchService} from '@core/search.ts';
import type {BrowseSelectionScope} from '@core/selection-model/index.ts';
import {randomId} from '@core/random-id.ts';
import type {WebShellPlaybackRuntime} from '../../platform/playback/runtime';
import type {ScreenContext} from '../detail/context';
import {screenLifetime} from '../detail/context';
import {createLibraryPlaySelection, type ExplicitCatalogueSelectionSource} from '../browse/play-selection';
import {createLibrarySelection, type LibrarySelection} from '../browse/selection';

/** Search remains explicit. No result count or Search fence becomes a Browse
 * query descriptor. Only mounted genuine catalogue entries enter the adapter. */
export function createSearchSelection(context: ScreenContext, service: SearchService,
  principal: Omit<BrowseSelectionScope, 'viewId'>, metadataFence: () => string | undefined, runtime?: WebShellPlaybackRuntime) {
  const life = screenLifetime(context), listeners = new Set<() => void>();
  const mounted = new Map<ContentEntry, {order: number; count: number}>();
  let admitted = new WeakSet<ContentEntry>();
  let version = 0, disposed = false, started = false, pending = false, serial = 0;
  let active: {key: string; play: ReturnType<typeof createLibraryPlaySelection>; selection: LibrarySelection; off(): void} | undefined;
  const emit = () => {version++; for (const listener of listeners) listener();};
  const sourceListeners = new Set<() => void>();
  const notifySource = () => {for (const listener of sourceListeners) listener();};
  const sourceKey = () => {
    const snapshot = service.getSnapshot(), data = snapshot.data, query = snapshot.query;
    if (disposed || !life.current() || !data || !query || !['ready', 'loading'].includes(snapshot.phase)
      || query.q !== data.query.q || (query.group ?? '') !== data.query.group) return undefined;
    return JSON.stringify([data.scope.serverId, data.scope.viewerFence, data.revision, query.q, query.group, query.groups, query.libraryIds, query.sort, query.direction]);
  };
  const rows = () => [...mounted].sort((a, b) => a[1].order - b[1].order).map(([entry]) => entry).slice(0, 60);
  const register = () => {
    if (pending || disposed) return;
    pending = true;
    queueMicrotask(() => {
      pending = false; const owned = active;
      if (!owned || sourceKey() !== owned.key) return;
      notifySource();
      const count = rows().length;
      if (count) void owned.play.registerWindow(0, count - 1).catch(error => {if (active === owned && life.current() && sourceKey() === owned.key) context.effects.reportError(error);});
      emit();
    });
  };
  const retire = () => {const old = active; active = undefined; old?.off(); old?.play.dispose(); mounted.clear(); admitted = new WeakSet(); notifySource();};
  const sync = () => {
    const key = sourceKey();
    if (active?.key === key) {for (const group of service.getSnapshot().groups) for (const entry of group.items) if ('libraryId' in entry) admitted.add(entry); return;}
    retire();
    if (!key) {emit(); return;}
    const data = service.getSnapshot().data!;
    for (const group of service.getSnapshot().groups) for (const entry of group.items) if ('libraryId' in entry) admitted.add(entry);
    const revision = JSON.stringify([data.scope.viewerFence, data.revision]), queryRevision = ++serial, viewId = `search-selection:${randomId()}`;
    const current = () => !disposed && life.current() && sourceKey() === key;
    const subscribe = (listener: () => void) => {sourceListeners.add(listener); return () => {sourceListeners.delete(listener);};};
    const source: ExplicitCatalogueSelectionSource = {
      viewIdentity: () => viewId,
      getSnapshot: () => ({phase: current() ? 'ready' : 'retired', revision, queryRevision}),
      subscribe, contains: entry => current() && admitted.has(entry) && mounted.has(entry),
      collection: {subscribe, itemAt: index => {const entry = rows()[index]; return entry ? {entry} : undefined;}},
    };
    const owner = context.identity?.viewer.role === 'owner' && context.identity.viewer.authority === 'local';
    const play = createLibraryPlaySelection({source, api: life.api, scope: context.scope, principal, requestId: async () => randomId(),
      signal: life.signal(), isCurrent: current, editor: owner, deletion: owner, destinations: true, personal: {myList: true, watched: true, favorite: true},
      playback: runtime?.cataloguePlayback(), queue: runtime?.catalogueQueue()});
    const selection = createLibrarySelection({playSelection: play, owner,
      ...(runtime ? {enqueue: play.enqueue} : {}),
      pickers: {api: life.api, serverId: context.scope.serverId, viewerId: context.scope.viewerId, requestId: async () => randomId(), signal: life.signal(), isCurrent: current,
        canCreateCollection: context.identity?.viewer.role === 'owner', pickerIdentity: {...principal, events: context.events}},
      ...(owner ? {editor: {api: life.api, events: context.events, requestId: async () => randomId(), serverId: context.scope.serverId, viewerId: context.scope.viewerId,
        viewerFence: metadataFence, signal: life.signal(), isCurrent: current},
        deletion: {api: life.api, serverId: context.scope.serverId, viewerId: context.scope.viewerId, requestId: async () => randomId(), signal: life.signal(), isCurrent: current}} : {}),
      actions: {api: life.api, events: context.events, requestId: async () => randomId(), signal: life.signal(), isCurrent: current, reportError: context.effects.reportError},
      reportError: context.effects.reportError,
    });
    active = {key, play, selection, off: play.subscribe(emit)};
    play.start(); emit();
  };
  let off = () => {};
  const start = () => {if (started || disposed) return; started = true; off = service.subscribe(sync); sync();};
  const subscribe = (listener: () => void) => {listeners.add(listener); return () => {listeners.delete(listener);};};
  const getSnapshot = () => version;
  function Toolbar() {useSyncExternalStore(subscribe, getSnapshot, getSnapshot); return active?.selection.toolbar ?? null;}
  return {
    start, subscribe, getSnapshot, epoch: () => serial, isActive: () => active?.play.getSnapshot().selection?.active === true, toolbar: <Toolbar/>,
    register(entry: ContentEntry, order: number) {
      const owned = active;
      if (!owned || sourceKey() !== owned.key || !admitted.has(entry)) return () => {};
      const previous = mounted.get(entry); mounted.set(entry, {order, count: (previous?.count ?? 0) + 1}); register();
      return () => {if (active !== owned) return; const value = mounted.get(entry); if (value && value.count > 1) mounted.set(entry, {...value, count: value.count - 1}); else mounted.delete(entry); register();};
    },
    card(entry: ContentEntry) {return active?.selection.entry({entry}, rows().indexOf(entry));},
    dispose() {if (disposed) return; disposed = true; off(); retire(); life.dispose(); emit(); listeners.clear(); sourceListeners.clear();},
  };
}
export type SearchSelection = ReturnType<typeof createSearchSelection>;
