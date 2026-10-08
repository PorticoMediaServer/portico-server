import type { AudioPlan } from '@core/audio-selection';
type Track = { groupId: string; name: string; url: string };
/** Only exact plan-owned resources map; engine-local indexes never leave the adapter. */
export function mapAudioTracks(
  plan: AudioPlan,
  tracks: readonly Track[],
  masterUrl: string
): Map<string, number> | null {
  try {
    if (tracks.length !== plan.renditions.length) return null;
    const master = new URL(masterUrl),
      result = new Map<string, number>();
    for (const rendition of plan.renditions) {
      const expected = new URL(rendition.manifestIdentity.playlistFile, master);
      const indices = tracks.flatMap((track, index) => {
        const u = new URL(track.url, master);
        return track.groupId === rendition.manifestIdentity.groupId &&
          track.name === rendition.manifestIdentity.name &&
          !u.username &&
          !u.password &&
          u.origin === expected.origin &&
          u.pathname === expected.pathname &&
          u.search === expected.search &&
          u.hash === ''
          ? [index]
          : [];
      });
      if (indices.length !== 1) return null;
      result.set(rendition.id, indices[0]);
    }
    return new Set(result.values()).size === plan.renditions.length ? result : null;
  } catch {
    return null;
  }
}
