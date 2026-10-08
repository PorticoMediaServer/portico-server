/**
 * Dolby Vision pre-play warning (M27): the server flags a Profile 5 conversion whose colors are
 * approximate with reason `dolby_vision_colors_approximate`. It appears in `decision.video.reasons`
 * of `GET /v1/items/{id}/playback-options` (pre-play) and of the session presentation. Rendered
 * behind presence: absent on older servers, which read as no warning.
 */
export const DOLBY_VISION_APPROXIMATE_REASON = 'dolby_vision_colors_approximate';

function reasonsInclude(reasons: unknown): boolean {
  return Array.isArray(reasons) && reasons.includes(DOLBY_VISION_APPROXIMATE_REASON);
}

/** Whether raw playback-options (or a session presentation) carries the approximate-colors reason. */
export function dolbyVisionWarningNeeded(raw: unknown): boolean {
  if (!raw || typeof raw !== 'object') return false;
  const o = raw as Record<string, unknown>;
  // New server pre-play decision: {decision: {video: {reasons: [...]}}}.
  const decision = o.decision as Record<string, unknown> | undefined;
  if (decision && typeof decision === 'object') {
    const video = (decision as Record<string, unknown>).video as Record<string, unknown> | undefined;
    if (video && typeof video === 'object' && reasonsInclude((video as Record<string, unknown>).reasons)) return true;
  }
  // Existing plan shape: {plan: {streams: [{reasons: [...]}}]} (the plan is for the chosen version).
  const plan = o.plan as Record<string, unknown> | undefined;
  if (plan && typeof plan === 'object') {
    const streams = (plan as Record<string, unknown>).streams;
    if (Array.isArray(streams)) {
      for (const s of streams) {
        if (s && typeof s === 'object' && reasonsInclude((s as Record<string, unknown>).reasons)) return true;
      }
    }
  }
  // Session presentation shape, if ever passed directly.
  const presentation = o.presentation as Record<string, unknown> | undefined;
  if (presentation && typeof presentation === 'object') {
    const pDecision = (presentation as Record<string, unknown>).decision as Record<string, unknown> | undefined;
    if (pDecision && typeof pDecision === 'object') {
      const video = (pDecision as Record<string, unknown>).video as Record<string, unknown> | undefined;
      if (video && typeof video === 'object' && reasonsInclude((video as Record<string, unknown>).reasons)) return true;
    }
  }
  return false;
}

/** Kinds that never carry video, so the pre-play Dolby Vision check is skipped for them. */
const AUDIO_ONLY_KINDS = new Set(['track', 'song', 'album', 'artist', 'audiobook', 'audiobook_file', 'chapter', 'music', 'podcast', 'episode_audio']);
/** Whether an entry of this kind may carry video (unknown kinds are checked). */
export function mayCarryVideo(kind: string | undefined): boolean {
  return !kind || !AUDIO_ONLY_KINDS.has(kind);
}
