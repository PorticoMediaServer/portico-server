import {BrowseSelectionModel, browseSelectionKey, readCatalogueSelectionItem, playCatalogueSelection, selectionCapabilities,
  type BrowseSelectionScope, type BrowseSelectionContext, type SelectionItem, type SelectionOperation, type SelectionSnapshot} from '@core/selection-model/index.ts';
import {selectionItem} from '@core/media-selection.ts';
import type {ContentEntry} from '@core/library-content.ts';
import type {DetailApi, DetailScope} from '@core/detail.ts';
import type {V1QueuePlayer} from '@core/playback-v1/v1-queue-player.ts';
import type {BrowseController} from './controller';

export type LibraryPlayRow = Readonly<{entry: ContentEntry; position: number; key: string; capabilities: SelectionItem['capabilities']}>;
export type LibraryPlaySelectionSnapshot = Readonly<{
  phase: 'waiting' | 'ready' | 'retired' | 'error';
  rows: readonly LibraryPlayRow[];
  selection?: SelectionSnapshot<BrowseSelectionContext>;
  resolving: boolean;
  playing: boolean;
  canPlay: boolean;
  error?: unknown;
}>;
export type LibraryPlaySelectionOptions = Readonly<{
  source: BrowseController;
  api: DetailApi;
  scope: DetailScope;
  principal: Omit<BrowseSelectionScope, 'viewId'>;
  requestId: () => Promise<string>;
  /** The original route/principal lease. isCurrent includes family/role retirement. */
  signal: AbortSignal;
  isCurrent: () => boolean;
  playback?: Pick<V1QueuePlayer, 'playSegments'>;
}>;
type Grant = {entry: ContentEntry; pending: boolean; controller: AbortController; promise: Promise<SelectionItem>; resolve: (item: SelectionItem) => void; reject: (error: unknown) => void};
type Window = {first: number; last: number; entries: readonly Readonly<{entry: ContentEntry; position: number}>[]; controller: AbortController; promise?: Promise<readonly LibraryPlayRow[]>; registrations: (() => void)[]};
const cancelled = () => new DOMException('Library selection retired', 'AbortError');
const unavailable = () => Object.assign(new Error('library-play-selection-unavailable'), {code: 'library-play-selection-unavailable'});
const terminal = (error: unknown) => typeof error === 'object' && error !== null
  && ('status' in error && (error.status === 401 || error.status === 403)
    || 'code' in error && typeof error.code === 'string' && ['unauthorized', 'forbidden', 'detail_scope_changed'].includes(error.code));

/** Headless, captured-parent adapter. No import/constructor reads, Home context,
 * aggregate expansion or role-derived grants. Only mounted resident windows
 * are registered; explicit model picks survive viewport eviction. */
export function createLibraryPlaySelection(input: LibraryPlaySelectionOptions) {
  const {source, api: originalApi, requestId, signal, isCurrent} = input;
  const principal = Object.freeze({serverId: input.principal.serverId, authority: input.principal.authority,
    accountId: input.principal.accountId, profileId: input.principal.profileId});
  const scope = Object.freeze({serverId: input.scope.serverId, viewerId: input.scope.viewerId});
  if (scope.serverId !== principal.serverId) throw unavailable();
  const playback = input.playback && Object.freeze({playSegments: input.playback.playSegments.bind(input.playback)});
  const life = new AbortController(), listeners = new Set<() => void>(), grants = new Map<ContentEntry, Grant>(), queued: Grant[] = [];
  let active = 0, closed = false, started = false, model: BrowseSelectionModel | undefined, modelOff: (() => void) | undefined;
  let sourceOff = () => {}, collectionOff = () => {}, expected: Readonly<{queryRevision: number; revision: string}> | undefined;
  let window: Window | undefined, handles = new WeakMap<LibraryPlayRow, Readonly<{window: Window; run: string}>>();
  let playing: Readonly<{operation: SelectionOperation<BrowseSelectionContext>; controller: AbortController}> | undefined;
  let error: unknown;
  let state: LibraryPlaySelectionSnapshot = Object.freeze({phase: 'waiting', rows: Object.freeze([]), resolving: false, playing: false, canPlay: false});
  const notify = () => {for (const listener of [...listeners]) listener();};
  const publish = (patch: Partial<LibraryPlaySelectionSnapshot> = {}) => {
    if (closed) return;
    const selection = model?.getSnapshot();
    state = Object.freeze({...state, phase: model ? 'ready' : 'waiting', ...patch,
      ...(selection ? {selection} : {}), playing: !!playing, canPlay: !!playback && !!selection?.summary.every.play,
      ...(error === undefined ? {} : {error})});notify();
  };
  const clearWindow = () => {
    const previous = window;window = undefined;
    previous?.controller.abort();for (const unregister of previous?.registrations ?? []) unregister();
    handles = new WeakMap();
  };
  const retire = (reason?: unknown, silent = false) => {
    if (closed) return;
    closed = true;sourceOff();collectionOff();modelOff?.();signal.removeEventListener('abort', dispose);
    life.abort();playing?.controller.abort();playing = undefined;clearWindow();model?.dispose();
    for (const grant of grants.values()) {grant.controller.abort();grant.reject(cancelled());}
    grants.clear();queued.length = 0;
    state = Object.freeze({phase: reason === undefined ? 'retired' : 'error', rows: Object.freeze([]), resolving: false, playing: false, canPlay: false,
      ...(reason === undefined ? {} : {error: reason})});
    try {if (!silent) notify();} finally {listeners.clear();}
  };
  const current = () => {
    if (closed) return false;
    if (signal.aborted || !isCurrent()) {retire();return false;}
    const snapshot = source.getSnapshot();
    if (expected && (snapshot.queryRevision !== expected.queryRevision || snapshot.revision !== expected.revision)) {retire();return false;}
    return true;
  };
  const api: DetailApi = {request: async<T>(path: string, method?: string, body?: unknown, requestSignal?: AbortSignal): Promise<T> => {
    if (!current()) throw cancelled();
    try {const value = await originalApi.request<T>(path, method, body, requestSignal);if (!current()) throw cancelled();return value;}
    catch (failure) {if (terminal(failure)) retire(failure);throw failure;}
  }};
  const pump = () => {
    while (!closed && active < 4 && queued.length) {
      const grant = queued.shift()!;
      if (grant.controller.signal.aborted || grants.get(grant.entry) !== grant) continue;
      active++;
      const lifetime = AbortSignal.any([signal, life.signal, grant.controller.signal]);
      void readCatalogueSelectionItem({api, scope, requestId, signal: lifetime,
        isCurrent: () => current() && source.contains(grant.entry)}, grant.entry, {play: true}).then(item => {grant.pending = false;grant.resolve(item);}, failure => {
          if (grants.get(grant.entry) === grant) grants.delete(grant.entry);
          if (terminal(failure)) retire(failure);grant.reject(failure);
        }).finally(() => {active--;pump();});
    }
  };
  const read = (entry: ContentEntry): Promise<SelectionItem> => {
    if (!entry.libraryId) return Promise.reject(unavailable());
    const leaf = selectionItem(entry);
    if (!playback || !leaf || leaf.itemId !== entry.id) return Promise.resolve(Object.freeze({kind: entry.kind, id: entry.id,
      libraryId: entry.libraryId, capabilities: selectionCapabilities(entry.kind, {})}));
    const cached = grants.get(entry);if (cached) return cached.promise;
    let resolve!: Grant['resolve'], reject!: Grant['reject'];
    const promise = new Promise<SelectionItem>((done, failed) => {resolve = done;reject = failed;});
    const grant: Grant = {entry, pending: true, controller: new AbortController(), promise, resolve, reject};
    grants.set(entry, grant);queued.push(grant);pump();return promise;
  };
  const prune = () => {
    if (!current()) return;
    for (const [entry, grant] of grants) if (!source.contains(entry)) {grants.delete(entry);grant.controller.abort();grant.reject(cancelled());}
    if (window?.entries.some(({entry}) => !source.contains(entry))) {clearWindow();publish({rows: Object.freeze([]), resolving: false});}
  };
  const sync = () => {
    if (!current()) return;
    const snapshot = source.getSnapshot();
    if (!model && snapshot.phase === 'ready' && snapshot.revision) {
      try {
        const query = source.query();
        const viewId = JSON.stringify({libraryId: source.source.libraryId, pivot: source.source.pivot, query: query.query ?? null, sort: query.sort ?? []});
        model = new BrowseSelectionModel({scope: {...principal, viewId}, revision: {pageRevision: snapshot.revision}});
        expected = Object.freeze({queryRevision: snapshot.queryRevision, revision: snapshot.revision});
        modelOff = model.subscribe(() => {
          if (closed) return;
          if (playing && !model!.isOperationCurrent(playing.operation)) playing.controller.abort();
          publish();
        });publish();
      } catch (failure) {retire(failure);}
    }
    prune();
  };
  function dispose() {retire();}
  const start = () => {
    if (closed || started) return;
    started = true;
    if (!current()) return;
    signal.addEventListener('abort', dispose, {once: true});
    sourceOff = source.subscribe(sync);collectionOff = source.collection.subscribe(prune);sync();
  };
  return {
    start,
    getSnapshot(): LibraryPlaySelectionSnapshot {
      if (!closed && (signal.aborted || !isCurrent())) retire(undefined, true);
      return state;
    },
    subscribe(listener: () => void) {if (closed) return () => {};listeners.add(listener);return () => {listeners.delete(listener);};},
    registerWindow(first: number, last: number): Promise<readonly LibraryPlayRow[]> {
      start();
      if (!current()) return Promise.reject(cancelled());
      if (!model || !expected) return Promise.reject(unavailable());
      if (!Number.isSafeInteger(first) || !Number.isSafeInteger(last) || first < 0 || last < first || last - first >= 60) return Promise.reject(unavailable());
      prune();
      const entries = Array.from({length: last - first + 1}, (_, offset) => ({position: first + offset, entry: source.collection.itemAt(first + offset)?.entry}))
        .flatMap(({entry, position}) => entry?.libraryId && source.contains(entry) ? [Object.freeze({entry, position})] : []);
      if (window?.first === first && window.last === last && window.entries.length === entries.length && window.entries.every((value, index) => value.entry === entries[index].entry)) return window.promise!;
      clearWindow();error = undefined;
      const visible = new Set(entries.map(value => value.entry));
      for (const [entry, grant] of grants) if (grant.pending && !visible.has(entry)) {
        grants.delete(entry);grant.controller.abort();grant.reject(cancelled());
      }
      const captured: Window = {first, last, entries, controller: new AbortController(), registrations: []};window = captured;
      captured.promise = Promise.resolve().then(async () => {
        if (!current() || window !== captured) throw cancelled();
        const stop = AbortSignal.any([signal, life.signal, captured.controller.signal]);
        let onAbort!: () => void;
        const aborted = new Promise<never>((_, reject) => {onAbort = () => reject(cancelled());if (stop.aborted) onAbort();else stop.addEventListener('abort', onAbort, {once: true});});
        try {
          const items = await Promise.race([Promise.all(entries.map(({entry}) => read(entry))), aborted]);
          if (!current() || window !== captured || stop.aborted || entries.some(({entry}) => !source.contains(entry))) throw cancelled();
          const ticket = model!.captureContext(), rows: LibraryPlayRow[] = [];
          for (let begin = 0; begin < entries.length;) {
            let end = begin + 1;while (end < entries.length && entries[end].position === entries[end - 1].position + 1) end++;
            const run = 'visible:' + entries[begin].position;
            const registration = model!.registerWindow(ticket, {id: run, sectionId: 'library', sectionOrder: 0, start: entries[begin].position, items: items.slice(begin, end)});
            if (!registration.ok) throw cancelled();captured.registrations.push(registration.unregister);
            for (let index = begin; index < end; index++) {
              const row = Object.freeze({...entries[index], key: browseSelectionKey(model!.getSnapshot().context.scope, items[index]), capabilities: items[index].capabilities});
              rows.push(row);handles.set(row, {window: captured, run});
            }
            begin = end;
          }
          const result = Object.freeze(rows);publish({rows: result, resolving: false});return result;
        } catch (failure) {
          if (!closed && window === captured) {clearWindow();error = failure;publish({rows: Object.freeze([]), resolving: false});}
          throw failure;
        } finally {stop.removeEventListener('abort', onAbort);}
      });publish({rows: Object.freeze([]), resolving: true, error: undefined});return captured.promise;
    },
    toggle(row: LibraryPlayRow, range = false) {
      const handle = handles.get(row);
      if (!current() || !model || !handle || handle.window !== window || !source.contains(row.entry)) throw cancelled();
      return model.toggle(handle.run, row.key, range);
    },
    captureOperation() {return current() ? model?.captureOperation() ?? null : null;},
    isOperationCurrent(operation: SelectionOperation<BrowseSelectionContext>) {return current() && !!model?.isOperationCurrent(operation);},
    selectLoaded() {if (!current() || !model) throw cancelled();return model.selectLoaded('library');},
    clear() {if (!current() || !model) throw cancelled();model.clear();},
    async play(callerSignal: AbortSignal = signal) {
      if (!current()) throw cancelled();
      if (!model || !playback || !model.getSnapshot().summary.every.play) throw unavailable();
      callerSignal.throwIfAborted();
      const operation = model.captureOperation();if (!operation || operation.target.mode !== 'explicit') throw unavailable();
      playing?.controller.abort();
      const captured = Object.freeze({operation, controller: new AbortController()});playing = captured;publish();
      const commandSignal = AbortSignal.any([signal, life.signal, callerSignal, captured.controller.signal]);
      try {return await playCatalogueSelection(playback, operation.target.displayItems.map(value => value.item),
        {signal: commandSignal, isCurrent: () => current() && playing === captured && model!.isOperationCurrent(operation)});}
      finally {if (playing === captured) {playing = undefined;publish();}}
    },
    dispose,
  };
}
export type LibraryPlaySelection = ReturnType<typeof createLibraryPlaySelection>;
