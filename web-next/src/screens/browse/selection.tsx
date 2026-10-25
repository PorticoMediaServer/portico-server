import {defaultI18n} from '@i18n';
import type {HomeSelectionProgress} from '@core/home-selection/types.ts';
import {useSyncExternalStore, type ReactNode} from 'react';
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
export type LibrarySelectionHost = Omit<LibraryPlaySelection, 'selectWhole' | 'preview' | 'selected'> & Readonly<{
  selectWhole?: (query: SelectionQuery) => unknown;
  preview?: (row: LibraryPlayRow) => boolean;
  /** The model's own membership: explicit picks and whole-view query members. */
  selected?: (row: LibraryPlayRow) => boolean;
  captureOperation?: () => SelectionOperation<BrowseSelectionContext> | null;
  isOperationCurrent?: (operation: SelectionOperation<BrowseSelectionContext>) => boolean;
}>;

/** A browse-safe queue add. Items are the explicit display ids in order. */
export type LibraryEnqueueInput = Readonly<{items: readonly string[]; placement: 'next' | 'end'; signal: AbortSignal; isCurrent: () => boolean}>;
/** The shared shell picker takes the explicit ids and the anchoring button. */
export type LibraryDestinations = Readonly<{
  playlist?: (items: readonly string[], anchor: HTMLElement) => Promise<unknown>;
  collection?: (items: readonly string[], anchor: HTMLElement) => Promise<unknown>;
}>;

export type LibrarySelectionOptions = Readonly<{
  playSelection: LibrarySelectionHost;
  /** The server's optional descriptor for this exact page, when the controller holds one. */
  descriptor?: () => BrowseWholeSelection | undefined;
  /** The real job ports. Absent means the bar offers no personal action at all. */
  actions?: SelectionActionsOptions;
  enqueue?: (input: LibraryEnqueueInput) => Promise<unknown>;
  destinations?: LibraryDestinations;
  reportError: (error: unknown) => void;
}>;

export type LibrarySelection = Readonly<{
  toolbar: ReactNode;
  entry: (member: BrowseMember, index: number) => CardSelection | undefined;
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
  for (const pick of target.displayItems) {
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
type ProgressStore = Readonly<{subscribe: (listener: () => void) => () => void; getSnapshot: () => PersonalProgress | undefined}>;

function LibrarySelectionToolbar(props: LibrarySelectionOptions & {progress: ProgressStore; personal: (action: PersonalSelectionAction) => Promise<void>; queue: (placement: 'next' | 'end') => Promise<void>; addToPlaylist: (anchor: HTMLElement) => Promise<void>; addToCollection: (anchor: HTMLElement) => Promise<void>}) {
  const {playSelection, descriptor, actions, enqueue, destinations, personal, queue, addToPlaylist, addToCollection, reportError} = props;
  const progress = useSyncExternalStore(props.progress.subscribe, props.progress.getSnapshot, props.progress.getSnapshot);
  const current = useSyncExternalStore(playSelection.subscribe, playSelection.getSnapshot, playSelection.getSnapshot);
  if (current.phase !== 'ready' || !current.selection?.active) return null;
  const summary = current.selection.summary, whole = descriptor?.(), selectWhole = playSelection.selectWhole;
  // Play dispatches explicit picks only; a whole view needs the server queue capture
  // that does not exist yet, so the button is not drawn for it.
  const canPlay = summary.every.play && current.selection.target?.mode === 'explicit';
  // Whole-view Select all N is offered only with the server's exact descriptor and
  // a model that can hold it; never as a local guess from the loaded window.
  const queryHeld = current.selection.target?.mode === 'query';
  const selectAll = queryHeld ? {kind: 'clear' as const} : whole && selectWhole ? {kind: 'view' as const, total: whole.total} : undefined;
  const offers = personalOffers(current.selection, actions, playSelection), leafItems = explicitLeafItems(current.selection);
  const queueLease = !!playSelection.captureOperation && !!playSelection.isOperationCurrent && !!actions?.signal
    && !actions.signal.aborted && actions.isCurrent();
  const canQueue = !!leafItems && !!enqueue && queueLease && summary.every.queue, canPlaylist = !!leafItems && !!destinations?.playlist && summary.every.playlist,
    canCollection = !!leafItems && !!destinations?.collection && summary.every.collection;
  return <SelectionBar
    selection={{count: summary.count, kind: summary.types.length === 1 ? summary.types[0].kind : undefined}}
    actions={{
      ...(canPlay ? {onPlay: () => { void playSelection.play().catch(reportError); }} : {}),
      ...(offers.watched ? {onWatchedChange: (watched: boolean) => { void personal({watched}); }} : {}),
      ...(offers.myList ? {onMyList: () => { void personal({watchlist: true}); }} : {}),
      ...(offers.favorite ? {onFavorite: () => { void personal({favorite: true}); }} : {}),
      ...(canQueue ? {onQueue: (placement: 'next' | 'end') => { void queue(placement); }} : {}),
      ...(canPlaylist ? {onPlaylist: (anchor: HTMLElement) => { void addToPlaylist(anchor); }} : {}),
      ...(canCollection ? {onCollection: (anchor: HTMLElement) => { void addToCollection(anchor); }} : {}),
    }}
    context={progress && actions?.isCurrent() && !actions.signal?.aborted && playSelection.isOperationCurrent?.(progress.operation)
      ? defaultI18n.t(progress.value.totalKnown ? 'next.selection.jobProgress' : 'next.selection.jobPreparing', {done: progress.value.done, total: progress.value.total}) : undefined}
    selectAll={selectAll}
    onSelectAll={selectAll ? () => { try {
      if (queryHeld) playSelection.clear();
      else void Promise.resolve(selectWhole!(wholeViewQuery(whole!))).catch(reportError);
    } catch (error) { reportError(error); } } : undefined}
    onExit={() => { try { playSelection.clear(); } catch (error) { reportError(error); } }}
    sticky />;
}

export function createLibrarySelection({playSelection, descriptor, actions, enqueue, destinations, reportError}: LibrarySelectionOptions): LibrarySelection {
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
    if (!destinations?.playlist) throw new Error('selection-playlist-unavailable');
    await destinations.playlist(leafItems(), anchor);
  };
  const addToCollection = async (anchor: HTMLElement) => {
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
  const snapshot = () => playSelection.getSnapshot(), rowFor = (member: BrowseMember, index: number) => {
    const rows = snapshot().rows;
    return rows.find(row => row.entry === member.entry) ?? rows.find(row => row.position === index);
  };
  const run = (work: () => void) => {
    try {
      work();
    } catch (error) {
      reportError(error);
    }
  };
  return {
    toolbar: <LibrarySelectionToolbar progress={progress} playSelection={playSelection} descriptor={descriptor} actions={actions} enqueue={enqueue} destinations={destinations} reportError={reportError} personal={personal} queue={queue} addToPlaylist={addToPlaylist} addToCollection={addToCollection} />,
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
    registerWindow: (first, last) => { void playSelection.registerWindow(first, last).catch(reportError); },
    personal, queue, addToPlaylist, addToCollection,
    playSelection,
  };
}
