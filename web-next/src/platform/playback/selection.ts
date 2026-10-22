import type {HomeSelectionPlaybackItem} from '@core/home-selection/types.ts';
import type {SegmentInput} from '@core/playback-v1/queue.ts';
import {id} from '@core/playback-v1/types.ts';

const containers = new Set(['show', 'season', 'album', 'artist', 'book', 'collection']);
const leaves = new Set(['movie', 'episode', 'song', 'audiobook_file', 'extra']);
const invalid = (): never => { throw new Error('invalid_selection_playback_target'); };

/** Keep the supplied display/pick order. Each aggregate is captured by the existing server
 * selector, while consecutive actual playable leaves share a capped items segment. */
export function selectionSegments(items: readonly HomeSelectionPlaybackItem[], playing: boolean): readonly SegmentInput[] {
  if (!items.length) invalid();
  const segments: SegmentInput[] = [];
  for (const [index, item] of items.entries()) {
    const {identity, entry, target} = item;
    if (identity.kind === 'channel' || identity.kind === 'person' || identity.id !== entry.id || identity.kind !== entry.kind || entry.libraryId !== undefined && identity.libraryId !== entry.libraryId || entry.available === false || entry.missing) invalid();
    id(identity.id, 'selection id'); id(target.itemId, 'playback target');
    if (target.startSeconds !== undefined && (!Number.isFinite(target.startSeconds) || target.startSeconds < 0 || target.startSeconds > 1e9)) invalid();
    if (containers.has(identity.kind)) {
      segments.push({source: {container: {kind: identity.kind, id: identity.id}}, ...(playing && index === 0 ? {anchor: {itemId: target.itemId}} : {})});
    } else {
      if (!leaves.has(identity.kind) || target.itemId !== identity.id) invalid();
      const last = segments.at(-1);
      if (last && 'items' in last.source && last.source.items.ids.length < 500) segments[segments.length - 1] = {...last, source: {items: {ids: [...last.source.items.ids, identity.id]}}};
      else segments.push({source: {items: {ids: [identity.id]}}});
    }
  }
  return segments;
}
