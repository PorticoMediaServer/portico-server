import {useSyncExternalStore, type ReactNode} from 'react';
import type {BrowseWholeSelection} from '@core/browse.ts';
import type {SelectionQuery} from '@core/selection-model/index.ts';
import {SelectionBar} from '../../kit/selection';
import type {CardSelection} from '../../kit/cards';
import type {BrowseMember} from './controller';
import type {LibraryPlayRow, LibraryPlaySelection} from './play-selection';
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
  reportError: (error: unknown) => void;
}>;

export type LibrarySelection = Readonly<{
  toolbar: ReactNode;
  entry: (member: BrowseMember, index: number) => CardSelection | undefined;
  registerWindow: (first: number, last: number) => void;
  playSelection: LibraryPlaySelection;
}>;

function LibrarySelectionToolbar({playSelection, descriptor, reportError}: LibrarySelectionOptions) {
  const current = useSyncExternalStore(playSelection.subscribe, playSelection.getSnapshot, playSelection.getSnapshot);
  if (current.phase !== 'ready' || !current.selection?.active) return null;
  const summary = current.selection.summary, whole = descriptor?.(), selectWhole = playSelection.selectWhole;
  // Whole-view Select all N is offered only with the server's exact descriptor and
  // a model that can hold it; never as a local guess from the loaded window.
  const selectAll = whole && selectWhole ? {kind: 'view' as const, total: whole.total} : undefined;
  return <SelectionBar
    selection={{count: summary.count, kind: summary.types.length === 1 ? summary.types[0].kind : undefined}}
    actions={{...(summary.every.play ? {onPlay: () => { void playSelection.play().catch(reportError); }} : {})}}
    selectAll={selectAll}
    onSelectAll={selectAll ? () => { try { void Promise.resolve(selectWhole!(wholeViewQuery(whole!))).catch(reportError); } catch (error) { reportError(error); } } : undefined}
    onExit={() => { try { playSelection.clear(); } catch (error) { reportError(error); } }}
    sticky />;
}

export function createLibrarySelection({playSelection, descriptor, reportError}: LibrarySelectionOptions): LibrarySelection {
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
    toolbar: <LibrarySelectionToolbar playSelection={playSelection} descriptor={descriptor} reportError={reportError} />,
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
    playSelection,
  };
}
