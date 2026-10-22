import type {HomeSelectionController, HomeSelectionSnapshot} from '@core/home-selection/index.ts';
import type {SelectionAction} from '@core/selection-model/index.ts';
import type {SelectionActions} from '../kit/selection';

/** Start before the menu's close event clears its canonical target. */
export function runSelectionCommand(work: () => void | Promise<unknown>, reportError: (error: unknown) => void): void {
  try { void Promise.resolve(work()).catch(reportError); } catch (error) { reportError(error); }
}

/** Optional renderer callbacks follow canonical all-of rules; the controller executes every effect. */
export function selectionCallbacks(snapshot: HomeSelectionSnapshot, controller: HomeSelectionController, anchor: (element: HTMLElement) => void, reportError: (error: unknown) => void): SelectionActions {
  const ready = (action: SelectionAction) => snapshot.actions[action].state === 'ready';
  const execute = (action: SelectionAction) => runSelectionCommand(() => controller.execute(action), reportError);
  return {
    ...(ready('play') ? {onPlay: () => execute('play')} : {}),
    ...(ready('play-next') && ready('play-later') ? {onQueue: (placement: 'next' | 'end') => execute(placement === 'next' ? 'play-next' : 'play-later')} : {}),
    ...(ready('playlist') ? {onPlaylist: (element: HTMLElement) => { anchor(element); void Promise.resolve(controller.openPicker('playlist')).catch(reportError); }} : {}),
    ...(ready('my-list') ? {onMyList: () => execute('my-list')} : {}),
    ...(ready('mark-watched') && ready('mark-unwatched') ? {onWatchedChange: (watched: boolean) => execute(watched ? 'mark-watched' : 'mark-unwatched')} : {}),
    ...(snapshot.actions.edit.state === 'ready' || snapshot.actions.edit.state === 'explain' ? {onEdit: (element: HTMLElement) => { anchor(element); void Promise.resolve(controller.openEditor()).catch(reportError); }, editMode: snapshot.actions.edit.state === 'ready' ? 'editable' as const : 'mixed' as const} : {}),
    ...(ready('collection') ? {onCollection: (element: HTMLElement) => { anchor(element); void Promise.resolve(controller.openPicker('collection')).catch(reportError); }} : {}),
    ...(ready('favorite') ? {onFavorite: () => execute('favorite')} : {}),
    ...(ready('remove-my-list') ? {onRemoveFromMyList: () => execute('remove-my-list')} : {}),
    ...(ready('refresh') ? {onRefresh: () => execute('refresh')} : {}),
    ...(ready('delete') || snapshot.actions.delete.state === 'disabled' ? {onDelete: () => { void Promise.resolve(controller.openDelete()).catch(reportError); }} : {}),
  };
}
