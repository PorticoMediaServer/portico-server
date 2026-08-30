import type { MediaItem } from "./types.js";

export function episodeCompactLabel(item: MediaItem): string {
  const season = item.seasonNumber ? `S${item.seasonNumber}` : "";
  const episode = item.episodeNumber ? `E${item.episodeNumber}` : "";
  return [season, episode].filter(Boolean).join(" · ");
}

export function episodeExplicitLabel(item: MediaItem): string {
  const season = item.seasonNumber ? `Season ${item.seasonNumber}` : "";
  const episode = item.episodeNumber ? `Episode ${item.episodeNumber}` : "";
  return [season, episode].filter(Boolean).join(" · ");
}

export function episodeOnlyLabel(item: MediaItem): string {
  return item.episodeNumber ? `Episode ${item.episodeNumber}` : episodeExplicitLabel(item);
}

export function ownedEpisodeCount(item: MediaItem): number {
  const children = item.children?.filter((child) => child.entityKind === "episode").length ?? 0;
  return children || item.fileCount || 0;
}

export function ownedEpisodeCountLabel(item: MediaItem): string {
  const count = ownedEpisodeCount(item);
  if (!count) return "";
  return `${count} episode${count === 1 ? "" : "s"}`;
}

export function formatRuntime(seconds: number): string {
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
}
