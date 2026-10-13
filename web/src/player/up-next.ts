/**
 * When to offer the next title before this one has ended: while its closing credits run.
 *
 * The server's credits marker is the trigger, so the offer appears when the story is over, not a
 * fixed number of seconds before the file ends. A marker in the first half of a title is not a
 * closing one (opening credits). An episode without a marker gets the offer for its last thirty
 * seconds. Nothing is offered when there is no next title, when the viewer turned credits prompts
 * off, or when the server wants to ask whether anyone is still watching.
 */
export type ClosingMarker = Readonly<{id: string; kind: string; startSeconds: number; endSeconds: number; automaticSafe?: boolean}>;
export type UpNextOffer = Readonly<{
  /** Stable for as long as the same offer stands; a dismissal is remembered against it. */
  id: string;
  /** The credits marker behind the offer, when there is one. */
  marker?: ClosingMarker;
}>;

export function upNextOffer(input: Readonly<{
  markers?: readonly ClosingMarker[] | null; positionSeconds: number; durationSeconds: number; kind?: string;
  mode: 'ask' | 'auto' | 'off'; nextAvailable: boolean; passoutDue: boolean;
}>): UpNextOffer | null {
  const {positionSeconds: position, durationSeconds: duration} = input;
  if (!input.nextAvailable || input.passoutDue || input.mode === 'off' || !(duration > 0) || !(position >= 0)) return null;
  const closing = (input.markers ?? []).filter(m => (m.kind === 'credits' || m.kind === 'outro') && m.startSeconds >= duration / 2 && m.endSeconds > m.startSeconds);
  if (closing.length) {
    const marker = closing.find(m => position >= m.startSeconds && position < m.endSeconds - 0.5);
    // Credits the viewer asked to have skipped unattended are skipped by the skip prompt; there is nothing to offer.
    if (!marker || (input.mode === 'auto' && marker.automaticSafe)) return null;
    return {id: 'marker:' + marker.id, marker};
  }
  if (input.kind === 'episode' && duration >= 600 && position >= duration - 30 && position < duration - 1) return {id: 'tail'};
  return null;
}
