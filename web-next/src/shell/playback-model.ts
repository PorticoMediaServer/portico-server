import {playbackIdentity, type TextEnvironment} from '@core/presentation/kit-model/index.ts';
import type {WebPlaybackRuntimeSnapshot} from '../platform/playback';
import type {DockPresentation} from '../kit/dock';
import type {ShellArtwork} from './artwork';
import {playbackFence} from '../../../packages/player-core/src/index.ts';

export const fenceKey = (fence: NonNullable<WebPlaybackRuntimeSnapshot['fence']>) => JSON.stringify(playbackFence(fence));
export function playbackPresentation(snapshot: WebPlaybackRuntimeSnapshot, environment: TextEnvironment, artwork: ShellArtwork, skips: {back: number; forward: number}): DockPresentation | undefined {
  const current = snapshot.presentation;
  if (!current) return undefined;
  const audio = current.kind !== 'video';
  return {fence: current.fence, identity: playbackIdentity(current.subject, 'dock', environment), durationSeconds: current.durationSeconds,
    controls: current.controls, finished: current.finished, rate: current.rate, volume: current.volume, muted: current.muted,
    skipBackSeconds: skips.back, skipForwardSeconds: skips.forward,
    artwork: artwork.foreground(current.posterUrl ?? current.backdropUrl, (audio ? 64 : 98) * (globalThis.devicePixelRatio || 1), audio ? 1 : 16 / 9, {group: 'playback', priority: 'high'}),
    ambient: artwork.ambient(current.backdropUrl ?? current.posterUrl, {group: 'playback', priority: 'high'}),
  };
}
/** Keep only actual active/bookmarked identities. A private presentation cannot survive a source teardown. */
export class PlaybackPresentations {
  private readonly values = new Map<string, DockPresentation>();
  update(snapshot: WebPlaybackRuntimeSnapshot, current: DockPresentation | undefined): readonly DockPresentation[] {
    if (current) this.values.set(fenceKey(current.fence), current);
    const state = snapshot.dock;
    const retained = new Set([state.local?.fence, state.parkedAudio?.fence, state.resumeVideo?.fence, state.remote?.playback.fence].flatMap(fence => fence ? [fenceKey(fence)] : []));
    for (const key of this.values.keys()) if (!retained.has(key)) this.values.delete(key);
    return Object.freeze([...this.values.values()]);
  }
  clear() { this.values.clear(); }
}
export function hasDock(snapshot: WebPlaybackRuntimeSnapshot): boolean {
  return !!snapshot.dock.remote || !!snapshot.dock.local && !snapshot.dock.local.expanded;
}
