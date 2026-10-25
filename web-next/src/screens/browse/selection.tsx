import {QuerySelectionEditorSurface} from './QuerySelectionEditorSurface';
import {LibraryPickerSurface, type LibraryPickerOptions} from './LibraryPickerSurface';
import type {EntryActionPorts, EntryDestination} from '../../shell/entry-actions';
import type {SelectionItem} from '@core/selection-model/index.ts';
import {LibraryDeleteSurface, type LibraryDeleteOptions} from './LibraryDeleteSurface';
import {SelectionEditorSurface, type LibraryEditorSession} from './SelectionEditorSurface';
import type {SelectionEditorOptions} from './selection-editor';
import {defaultI18n} from '@i18n';
import type {HomeSelectionProgress} from '@core/home-selection/types.ts';
import {useEffect, useSyncExternalStore, type ReactNode} from 'react';
import type {BrowseWholeSelection} from '@core/browse.ts';
import type {BrowseSelectionContext, SelectionOperation, SelectionQuery, SelectionSnapshot} from '@core/selection-model/index.ts';
import {selectionQuerySelector} from '@core/selection-model/index.ts';
import {SelectionBar} from '../../kit/selection';
import type {CardSelection} from '../../kit/cards';
import type {BrowseMember} from './controller';
import type {LibraryPlayRow, LibraryPlaySelection} from './play-selection';
import {createSelectionActions, personalTargetIsLeaf, type PersonalSelectionAction, type SelectionActionsOptions} from './selection-actions';
import {wholeViewQuery} from './selection-query';

/** The library's selection surface. It rides the existing headless play adapter,
 * which already registers the mounted window, reads each entry's real grants and
 * plays explicit picks in display order; this file only draws the bar and the
 * circles the cards position, and it never offers an action the adapter cannot
 * perform. */
/** The play adapter's own model owns the whole-view registration and the range
 * preview; these become available when the route supplies them. The operation
 * pair lets the bar capture the exact target before an awaited job. */
export type LibrarySelectionHost = Omit<LibraryPlaySelection, 'selectWhole' | 'preview' | 'selected' | 'keepOnly' | 'enqueue'> & Readonly<{
  keepOnly?: (kind: import('@core/selection-model/index.ts').SelectionKind) => unknown;
  selectWhole?: (query: SelectionQuery) => unknown;
  preview?: (row: LibraryPlayRow) => boolean;
  /** The model's own membership: explicit picks and whole-view query members. */
  selected?: (row: LibraryPlayRow) => boolean;
  captureOperation?: () => SelectionOperation<BrowseSelectionContext> | null;
  isOperationCurrent?: (operation: SelectionOperation<BrowseSelectionContext>) => boolean;
}>;

/** A browse-safe queue add. Items are the explicit ids in original pick order. */
export type LibraryEnqueueInput = Readonly<{items: readonly string[]; placement: 'next' | 'end'; signal: AbortSignal; isCurrent: () => boolean}>;
/** The shared shell picker takes the explicit ids and the anchoring button. */
export type LibraryDestinations = Readonly<{
  playlist?: (items: readonly string[], anchor: HTMLElement) => Promise<unknown>;
  collection?: (items: readonly string[], anchor: HTMLElement) => Promise<unknown>;
}>;

export type LibrarySelectionOptions = Readonly<{
  playSelection: LibrarySelectionHost;
  /** Current local owner fact; only explains the hard delete bound, never grants a write. */
  owner?: boolean;
  /** The server's optional descriptor for this exact page, when the controller holds one. */
  descriptor?: () => BrowseWholeSelection | undefined;
  /** The real job ports. Absent means the bar offers no personal action at all. */
  actions?: SelectionActionsOptions;
  editor?: SelectionEditorOptions;
  deletion?: LibraryDeleteOptions;
  pickers?: LibraryPickerOptions;
  enqueue?: (input: LibraryEnqueueInput) => Promise<unknown>;
  destinations?: LibraryDestinations;
  reportError: (error: unknown) => void;
}>;

export type LibrarySelection = Readonly<{
  toolbar: ReactNode;
  entry: (member: Pick<BrowseMember, 'entry'>, index: number) => CardSelection | undefined;
  registerWindow: (first: number, last: number) => void;
  /** The one path the bar's personal buttons use; also the test seam. */
  personal: (action: PersonalSelectionAction) => Promise<void>;
  /** Queue and the destination pickers, offered for leaf explicit picks only. */
  queue: (placement: 'next' | 'end') => Promise<void>;
  addToPlaylist: (anchor: HTMLElement) => Promise<void>;
  addToCollection: (anchor: HTMLElement) => Promise<void>;
  playSelection: LibrarySelectionHost;
}>;

/** Leaf explicit picks are the only target every add path can act on correctly:
 * a container's add would either become a container entry or a different command.
 * Channel and person picks are not catalogue titles and never join a queue or a
 * saved list, so they are not offered either. */
function explicitLeafItems(selection: BrowseSelection | undefined): readonly string[] | undefined {
  const target = selection?.target;
  if (!target || target.mode !== 'explicit' || !target.displayItems.length) return undefined;
  if (!personalTargetIsLeaf(target)) return undefined;
  const ids: string[] = [];
  for (const pick of target.items) {
    const item = pick.item;
    if (item.kind === 'channel' || item.kind === 'person') return undefined;
    ids.push(item.id);
  }
  return ids;
}

type BrowseSelection = SelectionSnapshot<BrowseSelectionContext>;

/** A personal action is offered only when the model says every selected title has
 * it, the target can carry a personal job at all (leaf picks or a leaf view), and
 * a whole-view job is dispatchable without dropping a kind or exclusion refinement. */
function personalOffers(selection: BrowseSelection | undefined, actions: SelectionActionsOptions | undefined, host: LibrarySelectionHost) {
  if (!actions || !host.captureOperation || !host.isOperationCurrent || !selection?.target) return {watched: false, myList: false, favorite: false};
  const usable = personalTargetIsLeaf(selection.target);
  const dispatchable = selection.target.mode !== 'query' || selectionQuerySelector(selection.target).ok;
  return {watched: usable && dispatchable && selection.summary.every.watched, myList: usable && dispatchable && selection.summary.every.myList,
    favorite: usable && dispatchable && selection.summary.every.favorite};
}

type PersonalProgress = Readonly<{operation: SelectionOperation<BrowseSelectionContext>; value: HomeSelectionProgress}>;
type VisibleRange = Readonly<{first: number; last: number}>;
type VisibleStore = Readonly<{subscribe: (listener: () => void) => () => void; getSnapshot: () => VisibleRange | undefined}>;
type ProgressStore = Readonly<{subscribe: (listener: () => void) => () => void; getSnapshot: () => PersonalProgress | undefined}>;

function LibrarySelectionToolbar(props: LibrarySelectionOptions & {visible: VisibleStore; openDelete: () => void; openEditor: (anchor: HTMLElement) => void; progress: ProgressStore; personal: (action: PersonalSelectionAction) => Promise<void>; queue: (placement: 'next' | 'end') => Promise<void>; addToPlaylist: (anchor: HTMLElement) => Promise<void>; addToCollection: (anchor: HTMLElement) => Promise<void>}) {
  const {playSelection, descriptor, actions, enqueue, destinations, personal, queue, addToPlaylist, addToCollection, reportError} = props;
  const visible = useSyncExternalStore(props.visible.subscribe, props.visible.getSnapshot, props.visible.getSnapshot);
  const progress = useSyncExternalStore(props.progress.subscribe, props.progress.getSnapshot, props.progress.getSnapshot);
  const current = useSyncExternalStore(playSelection.subscribe, playSelection.getSnapshot, playSelection.getSnapshot);
  useEffect(() => {
    const key = (event: KeyboardEvent) => {
      const escape = event.key === 'Escape' && !event.ctrlKey && !event.metaKey;
      const selectLoaded = (event.ctrlKey || event.metaKey) && !event.shiftKey && event.key.toLowerCase() === 'a';
      if ((!escape && !selectLoaded) || event.defaultPrevented || event.isComposing || event.altKey) return;
      const target = event.target;
      if (target instanceof Element && target.closest('input,textarea,select,[contenteditable]:not([contenteditable="false"]),[role="slider"],[role="dialog"],[role="alertdialog"],[role="menu"]')) return;
      const value = playSelection.getSnapshot();
      if (value.phase !== 'ready' || (escape ? !value.selection?.active : !value.rows.length)) return;
      try {
        if (escape) playSelection.clear();
        else {const result = playSelection.selectLoaded(); if (!result.ok) return;}
        event.preventDefault();
      } catch (error) {reportError(error);}
    };
    document.addEventListener('keydown', key);
    return () => document.removeEventListener('keydown', key);
  }, [playSelection, reportError]);
  if (current.phase !== 'ready' || !current.selection?.active) return null;
  const summary = current.selection.summary, whole = descriptor?.(), selectWhole = playSelection.selectWhole;
  // Play dispatches explicit picks only; a whole view needs the server queue capture
  // that does not exist yet, so the button is not drawn for it.
  const canPlay = summary.every.play && current.selection.target?.mode === 'explicit';
  // Whole-view Select all N is offered only with the server's exact descriptor and
  // a model that can hold it; never as a local guess from the loaded window.
  const queryHeld = current.selection.target?.mode === 'query';
  const visibleCount = visible && current.rows.length === visible.last - visible.first + 1
    && current.rows.every((row, index) => row.position === visible.first + index) ? current.rows.length : undefined;
  const selectAll = queryHeld ? {kind: 'clear' as const} : whole && selectWhole ? {kind: 'view' as const, total: whole.total} : undefined;
  const offers = personalOffers(current.selection, actions, playSelection), leafItems = explicitLeafItems(current.selection);
  const queueLease = !!playSelection.captureOperation && !!playSelection.isOperationCurrent && !!actions?.signal
    && !actions.signal.aborted && actions.isCurrent();
  const canQueue = !!leafItems && !!enqueue && queueLease && summary.every.queue, canPlaylist = !!leafItems && (!!destinations?.playlist || !!props.pickers) && queueLease && summary.every.playlist,
    canCollection = !!leafItems && (!!destinations?.collection || !!props.pickers) && queueLease && summary.every.collection;
  return <SelectionBar
    selection={{count: summary.count, kind: summary.types.length === 1 ? summary.types[0].kind : undefined}}
    actions={{
      ...(props.deletion && summary.library.kind === 'single' && summary.count <= 200 && summary.every.delete && current.selection.target?.mode === 'explicit' && queueLease ? {onDelete: () => {try {props.openDelete();} catch (error) {reportError(error);}}} : {}),
...(props.owner && props.deletion && summary.library.kind !== 'single' ? {deleteDisabledReason: defaultI18n.t('next.selection.oneLibrary')} : {}),
      ...(props.owner && props.deletion && summary.library.kind === 'single' && summary.count > 200 && queueLease ? {onDelete: () => {try {props.openDelete();} catch (error) {reportError(error);}}, deleteExplanation: defaultI18n.t('next.selection.tooMany')} : {}),
      ...(props.editor && summary.every.edit && (summary.types.length === 1 || !!playSelection.keepOnly) && (current.selection.target?.mode === 'explicit' || summary.types.length > 1 && !!playSelection.keepOnly || current.selection.target?.mode === 'query' && selectionQuerySelector(current.selection.target).ok) && queueLease ? {onEdit: (anchor: HTMLElement) => {try {props.openEditor(anchor);} catch (error) {reportError(error);}}, editMode: summary.types.length > 1 ? 'mixed' as const : 'editable' as const} : {}),
      ...(canPlay ? {onPlay: () => { void playSelection.play().catch(reportError); }} : {}),
      ...(offers.watched ? {onWatchedChange: (watched: boolean) => { void personal({watched}).catch(reportError); }} : {}),
      ...(offers.myList ? {onMyList: () => { void personal({watchlist: true}).catch(reportError); }, onRemoveFromMyList: () => { void personal({watchlist: false}).catch(reportError); }} : {}),
      ...(offers.favorite ? {onFavorite: () => { void personal({favorite: true}).catch(reportError); }} : {}),
      ...(canQueue ? {onQueue: (placement: 'next' | 'end') => { void queue(placement).catch(reportError); }} : {}),
      ...(canPlaylist ? {onPlaylist: (anchor: HTMLElement) => { void addToPlaylist(anchor).catch(reportError); }} : {}),
      ...(canCollection ? {onCollection: (anchor: HTMLElement) => { void addToCollection(anchor).catch(reportError); }} : {}),
    }}
    context={progress && actions?.isCurrent() && !actions.signal?.aborted && playSelection.isOperationCurrent?.(progress.operation)
      ? defaultI18n.t(progress.value.totalKnown ? 'next.selection.jobProgress' : 'next.selection.jobPreparing', {done: progress.value.done, total: progress.value.total}) : undefined}
    allInView={visibleCount !== undefined && current.selection.target?.mode === 'query' && selectionQuerySelector(current.selection.target).ok ? {visible: visibleCount, scope: 'view'} : undefined}
    selectAll={selectAll}
    onSelectAll={selectAll ? () => { try {
      if (queryHeld) playSelection.clear();
      else void Promise.resolve(selectWhole!(wholeViewQuery(whole!))).catch(reportError);
    } catch (error) { reportError(error); } } : undefined}
    onExit={() => { try { playSelection.clear(); } catch (error) { reportError(error); } }}
    sticky />;
}

export function createLibrarySelection({playSelection, owner, descriptor, actions, editor, deletion, pickers, enqueue, destinations, reportError}: LibrarySelectionOptions): LibrarySelection {
  const visibleListeners = new Set<() => void>();
  let visibleValue: VisibleRange | undefined;
  const visible: VisibleStore = {subscribe: listener => {visibleListeners.add(listener); return () => {visibleListeners.delete(listener);};}, getSnapshot: () => visibleValue};
  const editorListeners = new Set<() => void>();
  let editorSession: LibraryEditorSession | undefined;
  const closeEditor = () => {editorSession?.lifetime.abort(); editorSession = undefined; for (const listener of editorListeners) listener();};
  const editorStore = {subscribe: (listener: () => void) => {editorListeners.add(listener); return () => {editorListeners.delete(listener);};}, getSnapshot: () => editorSession};
  const openEditor = (anchor: HTMLElement) => {
    const selection = playSelection.getSnapshot().selection, operation = playSelection.captureOperation?.();
    if (!editor || !editor.signal || !editor.isCurrent() || !selection || !operation || operation.target !== selection.target || (operation.target.mode !== 'explicit' && !(selection.summary.types.length > 1 && playSelection.keepOnly) && !selectionQuerySelector(operation.target).ok)
      || !selection.summary.every.edit || (selection.summary.types.length !== 1 && !playSelection.keepOnly) || !playSelection.isOperationCurrent?.(operation)) throw new Error('selection-editor-unavailable');
    closeEditor();
    const lifetime = new AbortController();
    const current = () => !lifetime.signal.aborted && !editor.signal!.aborted && editor.isCurrent() && playSelection.isOperationCurrent?.(operation) === true;
    editorSession = Object.freeze({operation, anchor, lifetime, current});
    for (const listener of editorListeners) listener();
  };
  const deleteListeners = new Set<() => void>();
  let deleteSession: LibraryEditorSession | undefined;
  const closeDelete = () => {deleteSession?.lifetime.abort(); deleteSession = undefined; for (const listener of deleteListeners) listener();};
  const deleteStore = {subscribe: (listener: () => void) => {deleteListeners.add(listener); return () => {deleteListeners.delete(listener);};}, getSnapshot: () => deleteSession};
  const openDelete = () => {
    const selection = playSelection.getSnapshot().selection, operation = playSelection.captureOperation?.();
    if (!deletion || deletion.signal.aborted || !deletion.isCurrent() || !selection || selection.summary.library.kind !== 'single' || !operation || operation.target !== selection.target || !playSelection.isOperationCurrent?.(operation)
      || (selection.summary.count <= 200 && (!selection.summary.every.delete || selection.summary.library.kind !== 'single' || operation.target.mode !== 'explicit'))) throw new Error('selection-delete-unavailable');
    closeDelete();
    const lifetime = new AbortController();
    const current = () => !lifetime.signal.aborted && !deletion.signal.aborted && deletion.isCurrent() && playSelection.isOperationCurrent?.(operation) === true;
    const anchor = document.activeElement instanceof HTMLElement ? document.activeElement : document.body;
    deleteSession = Object.freeze({operation, anchor, lifetime, current});
    for (const listener of deleteListeners) listener();
  };
  const pickerListeners = new Set<() => void>();
  let pickerSession: LibraryPickerSession | undefined;
  const closePicker = () => {pickerSession?.lifetime.abort(); pickerSession = undefined; for (const listener of pickerListeners) listener();};
  const pickerStore = {subscribe: (listener: () => void) => {pickerListeners.add(listener); return () => {pickerListeners.delete(listener);};}, getSnapshot: () => pickerSession};
  const openPicker = (kind: EntryDestination, anchor: HTMLElement) => {
    const selection = playSelection.getSnapshot().selection, operation = playSelection.captureOperation?.();
    if (!pickers || !pickers.pickerIdentity || pickers.signal.aborted || !pickers.isCurrent() || !selection || !operation || operation.target !== selection.target
      || operation.target.mode !== 'explicit' || !explicitLeafItems(selection) || !selection.summary.every[kind] || !playSelection.isOperationCurrent?.(operation)) throw new Error('selection-picker-unavailable');
    closePicker();
    const lifetime = new AbortController(), signal = AbortSignal.any([lifetime.signal, pickers.signal]);
    const current = () => !signal.aborted && pickers.isCurrent() && playSelection.isOperationCurrent?.(operation) === true;
    const check = () => {if (!current()) throw new DOMException('Selection retired', 'AbortError');};
    const ports: EntryActionPorts = {...pickers, isCurrent: current, api: {request: async<T>(path: string, method?: string, body?: unknown, caller?: AbortSignal): Promise<T> => {
      check(); const value = await pickers.api.request<T>(path, method, body, caller ? AbortSignal.any([signal, caller]) : signal); check(); return value;
    }}};
    pickerSession = Object.freeze({operation, anchor, lifetime, current, kind, ports, items: Object.freeze(operation.target.items.map(pick => pick.item))});
    for (const listener of pickerListeners) listener();
  };
  const leafItems = () => {
    const items = explicitLeafItems(playSelection.getSnapshot().selection);
    if (!items) throw new Error('leaf-explicit-selection-required');
    return items;
  };
  const queue = async (placement: 'next' | 'end') => {
    if (!enqueue || !actions?.signal || !playSelection.captureOperation || !playSelection.isOperationCurrent) throw new Error('selection-queue-unavailable');
    const signal = actions.signal;
    const operation = playSelection.captureOperation();
    const snapshot = playSelection.getSnapshot().selection;
    if (!operation || operation.target !== snapshot?.target) throw new Error('selection-target-required');
    const items = explicitLeafItems(snapshot);
    if (!items) throw new Error('leaf-explicit-selection-required');
    const isCurrent = () => !signal.aborted && actions.isCurrent() && playSelection.isOperationCurrent!(operation)
      && playSelection.getSnapshot().selection?.target === operation.target;
    if (!isCurrent()) throw new DOMException('Library selection retired', 'AbortError');
    await enqueue({items, placement, signal, isCurrent});
  };
  const addToPlaylist = async (anchor: HTMLElement) => {
    if (pickers) {openPicker('playlist', anchor); return;}
    if (!destinations?.playlist) throw new Error('selection-playlist-unavailable');
    await destinations.playlist(leafItems(), anchor);
  };
  const addToCollection = async (anchor: HTMLElement) => {
    if (pickers) {openPicker('collection', anchor); return;}
    if (!destinations?.collection) throw new Error('selection-collection-unavailable');
    await destinations.collection(leafItems(), anchor);
  };
  const progressListeners = new Set<() => void>();
  let progressValue: PersonalProgress | undefined;
  const progress: ProgressStore = {subscribe: listener => {progressListeners.add(listener); return () => {progressListeners.delete(listener);};}, getSnapshot: () => progressValue};
  const personalRunner = actions ? createSelectionActions({...actions, reportError, progress: (operation, value, jobs) => {
    if (!playSelection.isOperationCurrent?.(operation as SelectionOperation<BrowseSelectionContext>)) return;
    progressValue = Object.freeze({operation: operation as SelectionOperation<BrowseSelectionContext>, value});
    for (const listener of progressListeners) listener();
    actions.progress?.(operation, value, jobs);
  }}) : undefined;
  const personal = async (action: PersonalSelectionAction) => {
    if (!actions || !playSelection.captureOperation || !playSelection.isOperationCurrent) throw new Error('selection-actions-unavailable');
    const selection = playSelection.getSnapshot().selection, target = selection?.target;
    if (!target) throw new Error('selection-target-required');
    const chosen = [action.watched, action.favorite, action.watchlist].filter(value => value !== undefined).length;
    const fact = action.watched !== undefined ? 'watched' : action.favorite !== undefined ? 'favorite' : action.watchlist !== undefined ? 'myList' : undefined;
    if (chosen !== 1 || !fact) throw new Error('mixed-personal-command');
    // The producer rechecks the chosen action's own grant on the exact target,
    // not just the toolbar's earlier visibility decision.
    if (selection!.summary.every[fact] !== true) throw new Error('selection-grant-unavailable');
    const operation = playSelection.captureOperation?.() ?? null;
    if (!operation || operation.target !== target) throw new Error('selection-operation-required');
    try {await personalRunner!.personal(operation, action, actions.signal,
      () => playSelection.isOperationCurrent?.(operation) === true);}
    finally {if (progressValue?.operation === operation) {progressValue = undefined; for (const listener of progressListeners) listener();}}
  };
  const snapshot = () => playSelection.getSnapshot(), rowFor = (member: Pick<BrowseMember, 'entry'>, index: number) => {
    const rows = snapshot().rows;
    return rows.find(row => row.entry === member.entry) ?? rows.find(row => row.position === index && row.entry.id === member.entry.id && row.entry.kind === member.entry.kind && row.entry.libraryId === member.entry.libraryId);
  };
  const run = (work: () => void) => {
    try {
      work();
    } catch (error) {
      reportError(error);
    }
  };
  return {
    toolbar: <><LibrarySelectionToolbar visible={visible} pickers={pickers} deletion={deletion} openDelete={openDelete} editor={editor} openEditor={openEditor} progress={progress} owner={owner} playSelection={playSelection} descriptor={descriptor} actions={actions} enqueue={enqueue} destinations={destinations} reportError={reportError} personal={personal} queue={queue} addToPlaylist={addToPlaylist} addToCollection={addToCollection} />{editor && <LibraryEditorHost playSelection={playSelection} store={editorStore} options={editor} close={closeEditor}/>} {deletion && <LibraryDeleteHost playSelection={playSelection} store={deleteStore} options={deletion} close={closeDelete}/>} {pickers && <LibraryPickerHost playSelection={playSelection} store={pickerStore} options={pickers} close={closePicker}/>}</>,
    entry: (member, index) => {
      const current = snapshot(), row = rowFor(member, index);
      if (!row || current.phase !== 'ready') return undefined;
      const checked = playSelection.selected?.(row) ?? current.selection?.selectedKeys.includes(row.key) === true, active = current.selection?.active === true;
      return {
        active, checked,
        ...(active && playSelection.preview?.(row) === true ? {preview: true} : {}),
        onToggle: intent => run(() => { playSelection.toggle(row, intent.range); }),
        onExit: () => run(() => { playSelection.clear(); }),
      };
    },
    registerWindow: (first, last) => {
      const next = Number.isSafeInteger(first) && Number.isSafeInteger(last) && first >= 0 && last >= first ? Object.freeze({first, last}) : undefined;
      if (next?.first !== visibleValue?.first || next?.last !== visibleValue?.last) {visibleValue = next; for (const listener of visibleListeners) listener();}
      void playSelection.registerWindow(first, last).catch(reportError);
    },
    personal, queue, addToPlaylist, addToCollection,
    playSelection,
  };
}


function LibraryEditorHost({playSelection, store, options, close}: {playSelection: LibrarySelectionHost; store: Readonly<{subscribe: (listener: () => void) => () => void; getSnapshot: () => LibraryEditorSession | undefined}>; options: SelectionEditorOptions; close: () => void}) {
  const session = useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot);
  useSyncExternalStore(playSelection.subscribe, playSelection.getSnapshot, playSelection.getSnapshot);
  const valid = !!session?.current();
  useEffect(() => {if (session && !valid) close();}, [session, valid, close]);
  useEffect(() => {const abort = () => close(); options.signal?.addEventListener('abort', abort, {once: true}); return () => {options.signal?.removeEventListener('abort', abort); close();};}, [options, close]);
  if (session && valid && session.operation.target.mode === 'query' && session.operation.target.summary.types.length === 1) return <QuerySelectionEditorSurface session={session} options={options} onClose={close}/>;
  return session && valid ? <SelectionEditorSurface session={session} options={options} onKeepOnly={kind => {if (session.current()) {playSelection.keepOnly?.(kind); close();}}} onClose={close}/> : null;
}

function LibraryDeleteHost({playSelection, store, options, close}: {playSelection: LibrarySelectionHost; store: Readonly<{subscribe: (listener: () => void) => () => void; getSnapshot: () => LibraryEditorSession | undefined}>; options: LibraryDeleteOptions; close: () => void}) {
  const session = useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot);
  useSyncExternalStore(playSelection.subscribe, playSelection.getSnapshot, playSelection.getSnapshot);
  const valid = !!session?.current();
  useEffect(() => {if (session && !valid) close();}, [session, valid, close]);
  useEffect(() => {const abort = () => close(); options.signal.addEventListener('abort', abort, {once: true}); return () => {options.signal.removeEventListener('abort', abort); close();};}, [options, close]);
  return session && valid ? <LibraryDeleteSurface session={session} options={options} onClose={close}/> : null;
}

type LibraryPickerSession = LibraryEditorSession & Readonly<{kind: EntryDestination; ports: EntryActionPorts; items: readonly SelectionItem[]}>;
function LibraryPickerHost({playSelection, store, options, close}: {playSelection: LibrarySelectionHost; store: Readonly<{subscribe: (listener: () => void) => () => void; getSnapshot: () => LibraryPickerSession | undefined}>; options: LibraryPickerOptions; close: () => void}) {
  const session = useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot);
  useSyncExternalStore(playSelection.subscribe, playSelection.getSnapshot, playSelection.getSnapshot);
  const valid = !!session?.current();
  useEffect(() => {if (session && !valid) close();}, [session, valid, close]);
  useEffect(() => {const abort = () => close(); options.signal.addEventListener('abort', abort, {once: true}); return () => {options.signal.removeEventListener('abort', abort); close();};}, [options, close]);
  return session && valid ? <LibraryPickerSurface destination={session.kind} target={session.items} ports={session.ports} anchor={session.anchor} canCreate={options.canCreateCollection} onClose={close}/> : null;
}
