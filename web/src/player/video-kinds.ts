/** Kinds that never carry video: nothing about a picture (its resolution, its HDR) is read for them. */
const AUDIO_ONLY_KINDS = new Set(['track', 'song', 'album', 'artist', 'audiobook', 'audiobook_file', 'chapter', 'music', 'podcast', 'episode_audio']);
/** Whether an entry of this kind may carry video (an unknown kind may). */
export function mayCarryVideo(kind: string | undefined): boolean {
  return !kind || !AUDIO_ONLY_KINDS.has(kind);
}
