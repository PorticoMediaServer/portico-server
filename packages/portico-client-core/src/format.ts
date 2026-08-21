export function formatRuntime(seconds = 0): string {
  if (!seconds) return "";
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

export function formatPlayerTime(seconds = 0): string {
  const safe = Math.max(0, Math.floor(seconds));
  const hours = Math.floor(safe / 3600);
  const minutes = Math.floor((safe % 3600) / 60);
  const remainingSeconds = safe % 60;
  const mm = String(minutes).padStart(hours ? 2 : 1, "0");
  const ss = String(remainingSeconds).padStart(2, "0");
  return hours ? `${hours}:${mm}:${ss}` : `${mm}:${ss}`;
}

export function progressPercent(progress = 0, duration = 0): number {
  if (!duration) return 0;
  return Math.min(100, Math.max(0, (progress / duration) * 100));
}

export function mediaTypeLabel(type: string): string {
  switch (type) {
    case "movie":
      return "Movie";
    case "show":
      return "TV Show";
    case "anime":
      return "Anime";
    case "episode":
      return "Episode";
    case "album":
      return "Album";
    case "track":
      return "Track";
    case "audiobook":
      return "Audiobook";
    case "recording":
      return "Recording";
    default:
      return type ? type[0].toUpperCase() + type.slice(1) : "Media";
  }
}

export function compactDate(value?: string): string {
  if (!value) return "";
  return new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric" }).format(new Date(value));
}

export function formatBytes(value = 0): string {
  if (!Number.isFinite(value) || value <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let size = value;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit++;
  }
  return `${size >= 10 || unit === 0 ? size.toFixed(0) : size.toFixed(1)} ${units[unit]}`;
}
