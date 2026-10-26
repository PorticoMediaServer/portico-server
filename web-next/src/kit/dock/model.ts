import {sameFence, type DockState, type RemoteDock} from '../../../../packages/player-core/src/index.ts';
import {clockText} from '@core/presentation/kit-model/text.ts';
import type {DockAction, DockBinding, DockIntent, DockPresentation, DockView} from './types';

export {clockText};
export function dockView(state: DockState, presentations: readonly DockPresentation[], retainExpandedMusic = false, retainExpandedVideo = false): DockView | null {
  const remote = state.remote, playback = remote?.playback ?? state.local;
  if (!playback || (!remote && playback.expanded && !(retainExpandedMusic && playback.kind === 'music' || retainExpandedVideo && (playback.kind === 'video' || playback.kind === 'live')))) return null;
  const presentation = presentations.find(item => sameFence(item.fence, playback.fence));
  if (!presentation) return null;
  return {mode: remote?.kind ?? playback.kind, playback, presentation, ...(remote ? {remote} : {})};
}
export function actionCurrent(state: DockState, action: DockAction): boolean {
  if (action.kind === 'restore-audio') return !!state.parkedAudio && sameFence(state.parkedAudio.fence, action.fence);
  if (action.kind === 'restore-video') return !!state.resumeVideo && sameFence(state.resumeVideo.fence, action.fence);
  if (action.remote) return !!state.remote && state.remote.id === action.remote.id && state.remote.sequence === action.remote.sequence && sameFence(state.remote.playback.fence, action.fence);
  return !state.remote && !!state.local && sameFence(state.local.fence, action.fence);
}
export function dispatchDockAction(binding: DockBinding, action: DockAction): boolean {
  if (!actionCurrent(binding.getSnapshot(), action)) return false;
  if (action.kind === 'seek' && (action.seconds === undefined || !Number.isFinite(action.seconds) || action.seconds < 0)) return false;
  if (action.kind === 'volume' && (action.volume === undefined || !Number.isFinite(action.volume) || action.volume < 0 || action.volume > 1)) return false;
  binding.onAction(action); return true;
}
export function commandFor(view: DockView, action: DockIntent): DockAction {
  return {...action, fence: view.playback.fence, ...(view.remote ? {remote: {id: view.remote.id, sequence: view.remote.sequence}} : {})};
}
/** phone-player f7d-7f: connecting is a pending ring; playing names the device and its
 * clock and offers Back, Pause and Stop; a terminal state keeps the bar with the position,
 * Continue here and Dismiss. The remote's own phase is the only input. */
export const remoteTerminal = (remote: RemoteDock | undefined) => !!remote && ['stopped', 'replaced', 'error'].includes(remote.phase);
export const remoteReady = (remote: RemoteDock | undefined) => !remote || remote.phase === 'playing';
export function seekBounds(view: DockView): {start: number; end: number} | null {
  const {seekWindow, durationSeconds} = view.presentation;
  const start = seekWindow?.startSeconds ?? 0, end = seekWindow?.endSeconds ?? durationSeconds;
  return Number.isFinite(start) && start >= 0 && end !== undefined && Number.isFinite(end) && end > start ? {start, end} : null;
}
export function positionOf(view: DockView): number { return view.remote?.positionSeconds ?? view.playback.positionSeconds; }
export function progressOf(view: DockView): number {
  const bounds = seekBounds(view); return bounds ? Math.min(1, Math.max(0, (positionOf(view) - bounds.start) / (bounds.end - bounds.start))) : 0;
}
export function seekTarget(view: DockView, value: number): number | null {
  const bounds = seekBounds(view); return bounds && Number.isFinite(value) ? Math.min(bounds.end, Math.max(bounds.start, value)) : null;
}
