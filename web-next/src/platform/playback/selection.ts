import type {HomeSelectionPlaybackItem} from '@core/home-selection/types.ts';
import type {SegmentInput} from '@core/playback-v1/queue.ts';
import {planSelectionPlayback} from '@core/selection-playback-plan.ts';

export {selectionStartSeconds} from '@core/selection-playback-plan.ts';

/** Keep the supplied display/pick order. Each aggregate is captured by the existing server
 * selector, while consecutive actual playable leaves share a capped items segment. Play is the
 * one queue-create request, so its 16-segment ceiling is checked before any replacement starts. */
export function selectionSegments(items: readonly HomeSelectionPlaybackItem[], playing: boolean): readonly SegmentInput[] {
  return planSelectionPlayback(items, playing ? 'replace' : 'insert').segments;
}
