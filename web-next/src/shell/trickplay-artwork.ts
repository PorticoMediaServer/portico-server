import {encodedRoutePath} from '../../../packages/contracts/src/routes.ts';
import {trickplayFrameAt, type TrickplaySet} from '@core/trickplay.ts';
import type {ArtworkRequestOptions} from '@core/artwork/index.ts';
import {playbackFence, type QueuePlaybackFence} from '../../../packages/player-core/src/index.ts';
import {protectedImageRuntime, type ProtectedImageOptions} from './protected-image';

/** Structural input from Webmedia's current analysis + descriptor selection. No second reader. */
export function createTrickplayArtwork(options: ProtectedImageOptions, selection: Readonly<{
  set: TrickplaySet;fence: QueuePlaybackFence;viewerFence: string;sourceRevision: string;isCurrent: () => boolean;
}>, subscribe: (listener: () => void) => () => void) {
  const fence = playbackFence(selection.fence), set = Object.freeze({...selection.set});
  if (fence.kind !== 'queue' || !/^[a-f0-9]{64}$/.test(set.id) || !/^[a-f0-9]{64}$/.test(set.revision) || !selection.viewerFence || !selection.sourceRevision
    || set.tileUrls !== undefined || set.sourceId !== fence.sourceId || set.sourceRevision !== selection.sourceRevision || set.stale) throw new Error('invalid_trickplay_artwork_selection');
  const tileBase = encodedRoutePath('get_items_id', {id: encodeURIComponent(fence.itemId)}) + '/trickplay/' + set.id + '/tiles';
  if (set.tilesUrl !== tileBase) throw new Error('invalid_trickplay_artwork_selection');
  const allowed = new Set<string>();let closed = false, off: (() => void) | undefined;
  const current = () => !closed && options.isCurrent() && selection.isCurrent();
  const images = protectedImageRuntime({...options, isCurrent: current, scopeKey: JSON.stringify([options.scopeKey, fence, selection.viewerFence, selection.sourceRevision, set.id, set.revision])}, request =>
    current() && request.kind === 'foreground' && allowed.has(request.url) ? {maxBytes: 12 * 1024 * 1024, width: set.width, height: set.height} : undefined);
  const dispose = () => {if (closed) return;closed = true;off?.();off = undefined;allowed.clear();images.dispose();};
  off = subscribe(() => {if (!current()) dispose();});
  if (closed) {off();off = undefined;} else if (!current()) dispose();
  return Object.freeze({
    fence,
    isCurrent: current,
    frameAt(seconds: number, request?: Omit<ArtworkRequestOptions, 'signal'>) {
      if (!current()) {dispose();return null;}
      const frame = trickplayFrameAt(set, seconds);
      if (!frame || frame.tileUrl !== tileBase + '/' + frame.tileIndex + '.jpg') return null;
      allowed.add(frame.tileUrl);
      return Object.freeze({set, frame, source: images.source({url: frame.tileUrl, cacheKey: JSON.stringify([set.id, set.revision, frame.tileIndex])}, request)});
    },
    dispose,
  });
}
