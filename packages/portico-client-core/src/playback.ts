import type { MediaItem, MediaSegment, MediaTrickplaySet, PlaybackResource, PlaybackResponse } from "./types.js";

export type SubtitleSize = "standard" | "large" | "xlarge";
export type SubtitleStyle = "default" | "shadow" | "contrast";
export type PlayerContentMode = "video" | "music" | "audiobook" | "live";

export class PlaybackResourceUnavailableError extends Error {
  readonly code = "playback_resource_unavailable";
  readonly messageId = "playback.unavailable";

  constructor() {
    super("playback.unavailable");
    this.name = "PlaybackResourceUnavailableError";
  }
}

export function normalizePlaybackResponse(playback: PlaybackResponse): PlaybackResponse {
  const repeatMode = playback.repeatMode === "one" || playback.repeatMode === "all" ? playback.repeatMode : "off";
  return {
    ...playback,
    nextEventSequence: Number.isFinite(playback.nextEventSequence) && playback.nextEventSequence > 0 ? Math.trunc(playback.nextEventSequence) : 1,
    timeline: playback.timeline ?? {
      type: playback.isLive || playback.media.type === "live_channel" ? "live" : "vod",
      durationSeconds: playback.media.durationSeconds,
      segmentSeconds: playback.isLive ? undefined : 4
    },
    generation: Number.isFinite(playback.generation) ? playback.generation : 0,
    qualities: asArray(playback.qualities),
    audioStreams: asArray(playback.audioStreams),
    subtitleStreams: asArray(playback.subtitleStreams),
    chapters: asArray(playback.chapters),
    queue: asArray(playback.queue),
    resources: asArray(playback.resources),
    repeatMode,
    queueRevision: Number.isFinite(playback.queueRevision) && playback.queueRevision >= 0 ? Math.trunc(playback.queueRevision) : 0
  };
}

export function playbackResourceUrl(playback: PlaybackResponse, path: string, resourceUrl: (path: string) => string, baseHref = "http://portico.local"): string {
  void playback;
  const resolved = resourceUrl(path);
  const url = new URL(resolved, baseHref);
  url.searchParams.delete("media_grant");
  url.searchParams.delete("download_grant");
  url.searchParams.delete("access_token");
  return url.toString();
}

export function segmentLabel(type: string): string {
  switch (type) {
    case "intro":
      return "Intro";
    case "recap":
      return "Recap";
    case "commercial":
      return "Commercial";
    case "outro":
      return "Outro";
    case "credits":
      return "Credits";
    default:
      return type.replace(/[-_]+/g, " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
  }
}

export function activePlaybackSegment(segments: MediaSegment[] | undefined, currentTime: number, dismissedIds: string[] = [], isLive = false): MediaSegment | undefined {
  if (isLive) return undefined;
  return asArray(segments).find((segment) =>
    segment.endSeconds > segment.startSeconds &&
    currentTime >= segment.startSeconds &&
    currentTime < segment.endSeconds &&
    !dismissedIds.includes(segment.id)
  );
}

export function supportsTrickplayPreview(item: MediaItem): boolean {
  return ["movie", "episode"].includes(item.type);
}

export function activeTrickplaySet(sets: MediaTrickplaySet[]): MediaTrickplaySet | undefined {
  return sets.find((set) => !set.stale && set.tileCount > 0 && set.intervalSeconds > 0) ?? sets.find((set) => set.tileCount > 0 && set.intervalSeconds > 0);
}

export function watchWithFriendsTargetPosition(positionSeconds: number, timestamp: string | undefined, moving: boolean, now = Date.now()): number {
  const base = Number.isFinite(positionSeconds) ? positionSeconds : 0;
  if (!moving || !timestamp) return base;
  const issuedAt = new Date(timestamp).getTime();
  if (!Number.isFinite(issuedAt)) return base;
  return base + Math.max(0, (now - issuedAt) / 1000);
}

export function subtitleSizeLabel(size: SubtitleSize): string {
  if (size === "large") return "Large";
  if (size === "xlarge") return "Extra Large";
  return "Standard";
}

export function subtitleStyleLabel(style: SubtitleStyle): string {
  if (style === "shadow") return "Drop Shadow";
  if (style === "contrast") return "High Contrast";
  return "Default";
}

export function isAudioContent(item: MediaItem): boolean {
  return ["album", "track", "audiobook", "music"].includes(item.type);
}

export function playerContentMode(item: MediaItem, isLive = false): PlayerContentMode {
  if (isLive || item.type === "live_channel") return "live";
  if (item.type === "audiobook") return "audiobook";
  if (["album", "track", "music"].includes(item.type)) return "music";
  return "video";
}

export function playbackSourceFor(
  playback: PlaybackResponse,
  resourceUrl: (path: string) => string,
  options: { streamFormat?: string; quality?: string; burnInSubtitleId?: string; textSubtitleId?: string; audioStreamId?: string; baseHref?: string } = {}
): string {
  void options.baseHref;
  const resources = asArray(playback.resources);
  const hasStreamSelection = options.streamFormat !== undefined;
  const hasQualitySelection = options.quality !== undefined;
  const hasAudioSelection = Object.prototype.hasOwnProperty.call(options, "audioStreamId");
  const hasBurnInSelection = Object.prototype.hasOwnProperty.call(options, "burnInSubtitleId");
  const hasTextSelection = Object.prototype.hasOwnProperty.call(options, "textSubtitleId");
  const hasSubtitleSelection = hasBurnInSelection || hasTextSelection;

  if (resources.length === 0) {
    if (hasStreamSelection || hasQualitySelection || hasAudioSelection || hasSubtitleSelection) {
      throw new PlaybackResourceUnavailableError();
    }
    return resourceUrl(playback.sourceUrl);
  }

  const requestedSubtitle = requestedPlaybackSubtitle(options);
  const matches = resources.filter((candidate) =>
    (!hasStreamSelection || candidate.streamFormat === options.streamFormat) &&
    (!hasQualitySelection || candidate.qualityId === options.quality) &&
    (!hasAudioSelection || (candidate.audioStreamId ?? "") === (options.audioStreamId ?? "")) &&
    (!hasSubtitleSelection ||
      (candidate.subtitleMode ?? "off") === requestedSubtitle.mode &&
      (candidate.subtitleStreamId ?? "") === requestedSubtitle.id)
  );
  const selected = matches.find((candidate) => candidate.default) ?? (matches.length === 1 ? matches[0] : undefined);
  if (!selected?.sourceUrl) throw new PlaybackResourceUnavailableError();
  return resourceUrl(selected.sourceUrl);
}

function requestedPlaybackSubtitle(options: { burnInSubtitleId?: string; textSubtitleId?: string }): { mode: NonNullable<PlaybackResource["subtitleMode"]>; id: string } {
  if (options.textSubtitleId) return { mode: "text", id: options.textSubtitleId };
  if (options.burnInSubtitleId) return { mode: "burn_in", id: options.burnInSubtitleId };
  return { mode: "off", id: "" };
}

export function defaultPlaybackQuality(playback: PlaybackResponse): string {
  // Older servers and deliberately minimal test/third-party payloads may omit
  // optional selection collections.  Playback defaults must remain usable and
  // must never crash the player while a contract mismatch is being surfaced.
  const available = (playback.qualities ?? []).filter((quality) => quality.available !== false);
  // A new session starts at source quality unless a caller supplied an
  // explicit saved preference. selectedQualityId/default resources describe
  // the server's initial transport decision, not a viewer preference.
  return available.find((quality) => quality.id === "original")?.id ??
    available[0]?.id ??
    "original";
}

export function playbackSelectionRequiresHLS(playback: PlaybackResponse, quality: string, audioStreamId: string): boolean {
  if (playback.isLive || playback.media.type === "live_channel") return false;
  if (playback.streamFormat === "hls") return true;
  const selectedQuality = (playback.qualities ?? []).find((candidate) => candidate.id === quality);
  if (quality && quality !== "auto" && quality !== "original") return true;
  if (selectedQuality?.requiresTranscode) return true;
  const baselineAudioStreamId = playback.selectedAudioStreamId ?? playback.audioStreams?.[0]?.id;
  return Boolean(audioStreamId && baselineAudioStreamId && audioStreamId !== baselineAudioStreamId);
}

export function burnInSubtitleIDFor(streams: PlaybackResponse["subtitleStreams"], selectedID: string): string {
  if (!selectedID || selectedID === "sub_none") return "";
  const stream = streams.find((candidate) => candidate.id === selectedID);
  return stream && !stream.sourceUrl ? stream.id : "";
}

export function selectedSubtitleLabel(streams: PlaybackResponse["subtitleStreams"], id: string): string {
  const stream = streams.find((candidate) => candidate.id === id);
  return stream?.displayTitle || stream?.language || stream?.codec || "Subtitles enabled";
}

export function selectedQualityLabel(qualities: PlaybackResponse["qualities"], id: string): string {
  return qualities.find((quality) => quality.id === id)?.label || id || "Original";
}

export function playbackDecisionLabel(mode: string): string {
  switch (mode) {
    case "direct_play":
      return "Direct Play";
    case "direct_stream":
      return "Direct Stream";
    case "transcode_required":
      return "Transcode";
    case "optimized_version":
      return "Optimized Version";
    default:
      return mode ? mode.replace(/_/g, " ").replace(/\b\w/g, (letter) => letter.toUpperCase()) : "Playback";
  }
}

export function effectivePlaybackVolume(volume: number, normalization: MediaItem["audioNormalization"], mode: "off" | "attenuate"): number {
  const base = Math.min(1, Math.max(0, volume));
  if (mode !== "attenuate" || !normalization) return base;
  const gainDb = playbackAttenuationDb(normalization);
  if (gainDb >= 0) return base;
  const scale = Math.pow(10, gainDb / 20);
  return Math.min(1, Math.max(0, base * scale));
}

export function playbackAttenuationDb(normalization: MediaItem["audioNormalization"]): number {
  if (!normalization) return 0;
  if (typeof normalization.trackGainDb === "number" && normalization.trackGainDb < 0) return normalization.trackGainDb;
  if (typeof normalization.albumGainDb === "number" && normalization.albumGainDb < 0) return normalization.albumGainDb;
  if (typeof normalization.integratedLufs === "number") return Math.min(0, -16 - normalization.integratedLufs);
  return 0;
}

export function asArray<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : [];
}
