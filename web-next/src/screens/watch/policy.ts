import {sameFence, UNDO_SECONDS, type PolicyView, type QueuePlaybackFence} from '../../../../packages/player-core/src/index.ts';
import type {WebPlaybackRuntimeSnapshot} from '../../platform/playback';

/** Structural subset of the media owner's action port; no policy execution here. */
export type WatchMarkerAction = Readonly<{kind: 'skip' | 'dismiss' | 'undo'; markerId: string}>;
export type WatchPolicyBinding = Readonly<{
  getSnapshot: () => WebPlaybackRuntimeSnapshot;
  isCurrent: () => boolean;
  policyAction: (fence: QueuePlaybackFence, action: WatchMarkerAction) => Promise<void>;
  reportError: (error: unknown) => void;
}>;

export function currentWatchPolicy(binding: WatchPolicyBinding, shown: WebPlaybackRuntimeSnapshot) {
  const live = binding.getSnapshot(), fence = shown.fence;
  if (!binding.isCurrent() || shown.disposed || live.disposed || fence?.kind !== 'queue' || !sameFence(live.fence, fence)
    || shown.authority !== live.authority || live.dock.remote?.phase === 'playing'
    || shown.presentation?.kind !== 'video' || !shown.presentation.expanded || live.presentation?.kind !== 'video' || !live.presentation.expanded) return null;
  if (!sameFence(shown.presentation.fence, fence) || !sameFence(live.presentation.fence, fence)) return null;
  return {fence, shown: shown.presentation.policy, live: live.presentation.policy};
}

/** The policy owns this lifetime, including expiry while paused. No renderer clock. */
export function watchUndoOffer(binding: WatchPolicyBinding, shown: WebPlaybackRuntimeSnapshot) {
  const current = currentWatchPolicy(binding, shown), policy = current?.shown, live = current?.live;
  if (!policy || !live || policy.attention || live.attention || policy.finished || live.finished
    || !policy.undo || !live.undo || policy.undo.marker.id !== live.undo.marker.id || policy.undo.marker.kind !== live.undo.marker.kind) return null;
  if ([policy.undo.remainingSeconds, live.undo.remainingSeconds].some(value => !Number.isFinite(value) || value <= 0 || value > UNDO_SECONDS)) return null;
  const remainingSeconds = Math.min(policy.undo.remainingSeconds, live.undo.remainingSeconds);
  return {marker: policy.undo.marker, remainingSeconds, fraction: remainingSeconds / UNDO_SECONDS};
}

function offersMarker(policy: PolicyView | null, action: WatchMarkerAction): boolean {
  if (!policy || policy.attention || policy.finished) return false;
  return action.kind === 'undo'
    ? policy.undo?.marker.id === action.markerId && policy.undo.remainingSeconds > 0
    : policy.prompt?.kind === 'skip' && policy.prompt.marker.id === action.markerId;
}

/** Marker identity goes to the real policy owner; the UI never supplies a seek position. */
export async function dispatchWatchMarker(binding: WatchPolicyBinding, shown: WebPlaybackRuntimeSnapshot, action: WatchMarkerAction): Promise<boolean> {
  const current = currentWatchPolicy(binding, shown);
  if (!current || !offersMarker(current.shown, action) || !offersMarker(current.live, action)) return false;
  await binding.policyAction(current.fence, action);
  return true;
}
