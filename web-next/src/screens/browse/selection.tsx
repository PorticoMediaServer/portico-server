import {useSyncExternalStore, type ReactNode} from 'react';
import type {BrowseWholeSelection} from '@core/browse.ts';
import type {SelectionQuery} from '@core/selection-model/index.ts';
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
 * preview; these become available when the route supplies them. */
export type LibrarySelectionHost = LibraryPlaySelection & Readonly<{
  selectWhole?: (query: SelectionQuery) => unknown;
  preview?: (row: LibraryPlayRow) => boolean;
}>;

export type LibrarySelectionOptions = Readonly<{
  playSelection: LibrarySelectionHost;
  /** The server's optional descriptor for this exact page, when the controller holds one. */
  descriptor?: () => BrowseWholeSelection | undefined;
  /** The real job ports. Absent means the bar offers no personal action at all. */
  actions?: SelectionActionsOptions;
  reportError: (error: unknown) => void;
}>;

export type LibrarySelection = Readonly<{
  toolbar: ReactNode;
  entry: (member: BrowseMember, index: number) => CardSelection | undefined;
  registerWindow: (first: number, last: number) => void;
  /** The one path the bar's personal buttons use; also the test seam. */
  personal: (action: PersonalSelectionAction) => Promise<void>;
  playSelection: LibraryPlaySelection;
}>;

/** A personal action is offered only when the model says every selected title has
 * it and the target can carry a personal job at all (leaf picks or a leaf view). */
function personalOffers(selection: {summary: {every: {watched: boolean; myList: boolean; favorite: boolean}}; target: {mode: string; items?: readonly {item: {kind: string}}[]} | null} | undefined, actions: SelectionActionsOptions | undefined) {
  if (!actions || !selection?.target) return {watched: false, myList: false, favorite: false};
  const usable = personalTargetIsLeaf(selection.target as never);
  return {watched: usable && selection.summary.every.watched, myList: usable && selection.summary.every.myList, favorite: usable && selection.summary.every.favorite};
}

function LibrarySelectionToolbar(props: LibrarySelectionOptions & {personal: (action: PersonalSelectionAction) => Promise<void>}) {
  const {playSelection, descriptor, actions, personal, reportError} = props;
  const current = useSyncExternalStore(playSelection.subscribe, playSelection.getSnapshot, playSelection.getSnapshot);
  if (current.phase !== 'ready' || !current.selection?.active) return null;
  const summary = current.selection.summary, whole = descriptor?.(), selectWhole = playSelection.selectWhole;
  // Whole-view Select all N is offered only with the server's exact descriptor and
  // a model that can hold it; never as a local guess from the loaded window.
  const selectAll = whole && selectWhole ? {kind: 'view' as const, total: whole.total} : undefined;
  const offers = personalOffers(current.selection, actions);
  return <SelectionBar
    selection={{count: summary.count, kind: summary.types.length === 1 ? summary.types[0].kind : undefined}}
    actions={{
      ...(summary.every.play ? {onPlay: () => { void playSelection.play().catch(reportError); }} : {}),
      ...(offers.watched ? {onWatchedChange: (watched: boolean) => { void personal({watched}); }} : {}),
      ...(offers.myList ? {onMyList: () => { void personal({watchlist: true}); }} : {}),
      ...(offers.favorite ? {onFavorite: () => { void personal({favorite: true}); }} : {}),
    }}
    selectAll={selectAll}
    onSelectAll={selectAll ? () => { try { void Promise.resolve(selectWhole!(wholeViewQuery(whole!))).catch(reportError); } catch (error) { reportError(error); } } : undefined}
    onExit={() => { try { playSelection.clear(); } catch (error) { reportError(error); } }}
    sticky />;
}

export function createLibrarySelection({playSelection, descriptor, actions, reportError}: LibrarySelectionOptions): LibrarySelection {
  const personal = async (action: PersonalSelectionAction) => {
    const target = playSelection.getSnapshot().selection?.target;
    if (!actions) throw new Error('selection-actions-unavailable');
    if (!target) throw new Error('selection-target-required');
    const runner = createSelectionActions({...actions, reportError});
    await runner.personal({target} as never, action, actions.signal ?? new AbortController().signal);
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
    toolbar: <LibrarySelectionToolbar playSelection={playSelection} descriptor={descriptor} actions={actions} reportError={reportError} personal={personal} />,
    entry: (member, index) => {
      const current = snapshot(), row = rowFor(member, index);
      if (!row || current.phase !== 'ready') return undefined;
      const checked = current.selection?.selectedKeys.includes(row.key) === true, active = current.selection?.active === true;
      return {
        active, checked,
        ...(active && playSelection.preview?.(row) === true ? {preview: true} : {}),
        onToggle: intent => run(() => { playSelection.toggle(row, intent.range); }),
        onExit: () => run(() => { playSelection.clear(); }),
      };
    },
    registerWindow: (first, last) => { void playSelection.registerWindow(first, last).catch(reportError); },
    personal,
    playSelection,
  };
}
