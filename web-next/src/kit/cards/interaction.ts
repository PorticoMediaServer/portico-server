import type {CardSelection, CardSelectionIntent} from './types';

export function activateCard(navigate: () => void, selection: CardSelection | undefined, intent: CardSelectionIntent): void {
  if (selection?.active || intent.range && selection) selection!.onToggle(intent);
  else navigate();
}

/** Q6: touch holds toggle an existing selection, otherwise open the real menu. */
export function holdCard(more: (() => void) | undefined, selection: CardSelection | undefined): void {
  if (selection?.active) selection.onToggle({range: false, source: 'long-press'});
  else more?.();
}

/** Unknown progress stays unknown; overshooting never paints beyond the card. */
export function boundedProgress(progress: number | undefined): number | undefined {
  return progress === undefined || !Number.isFinite(progress) ? undefined : Math.max(0, Math.min(1, progress));
}
