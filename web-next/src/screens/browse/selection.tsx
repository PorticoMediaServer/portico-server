import {useSyncExternalStore, type ReactNode} from 'react';
import {SelectionBar} from '../../kit/selection';
import type {CardSelection} from '../../kit/cards';
import type {BrowseMember} from './controller';
import type {LibraryPlaySelection} from './play-selection';

/** The library's selection surface. It rides the existing headless play adapter,
 * which already registers the mounted window, reads each entry's real grants and
 * plays explicit picks in display order; this file only draws the bar and the
 * circles the cards position, and it never offers an action the adapter cannot
 * perform. */
export type LibrarySelectionOptions = Readonly<{
  playSelection: LibraryPlaySelection;
  reportError: (error: unknown) => void;
}>;

export type LibrarySelection = Readonly<{
  toolbar: ReactNode;
  entry: (member: BrowseMember, index: number) => CardSelection | undefined;
  registerWindow: (first: number, last: number) => void;
  playSelection: LibraryPlaySelection;
}>;

function LibrarySelectionToolbar({playSelection, reportError}: LibrarySelectionOptions) {
  const current = useSyncExternalStore(playSelection.subscribe, playSelection.getSnapshot, playSelection.getSnapshot);
  if (current.phase !== 'ready' || !current.selection?.active) return null;
  const summary = current.selection.summary;
  return <SelectionBar
    selection={{count: summary.count, kind: summary.types.length === 1 ? summary.types[0].kind : undefined}}
    actions={{...(summary.every.play ? {onPlay: () => { void playSelection.play().catch(reportError); }} : {})}}
    // The whole-view Select all N belongs to the descriptor cohort; until it is
    // wired here the bar offers only what this adapter can really perform.
    onExit={() => { try { playSelection.clear(); } catch (error) { reportError(error); } }}
    sticky />;
}

export function createLibrarySelection({playSelection, reportError}: LibrarySelectionOptions): LibrarySelection {
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
    toolbar: <LibrarySelectionToolbar playSelection={playSelection} reportError={reportError} />,
    entry: (member, index) => {
      const current = snapshot(), row = rowFor(member, index);
      if (!row || current.phase !== 'ready') return undefined;
      const checked = current.selection?.selectedKeys.includes(row.key) === true, active = current.selection?.active === true;
      return {
        active, checked,
        onToggle: intent => run(() => { playSelection.toggle(row, intent.range); }),
        onExit: () => run(() => { playSelection.clear(); }),
      };
    },
    registerWindow: (first, last) => { void playSelection.registerWindow(first, last).catch(reportError); },
    playSelection,
  };
}
