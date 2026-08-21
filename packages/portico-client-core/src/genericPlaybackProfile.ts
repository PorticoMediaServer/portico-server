import type { PlaybackClientProfile } from "./types.js";

/**
 * Conservative playback defaults for TypeScript clients that have not yet
 * supplied device-specific capability facts.
 *
 * This helper is platform-neutral. Native clients should normally replace
 * these defaults with facts from their player adapter before starting
 * playback.
 */
export function genericPlaybackClientProfile(overrides: PlaybackClientProfile = {}): PlaybackClientProfile {
  return {
    device: "Portico TypeScript Client",
    platform: "typescript",
    supportsHls: true,
    supportsMse: false,
    supportsMpegTs: false,
    supportedContainers: ["hls", "mp4", "m4v", "mov"],
    supportedVideoCodecs: ["h264"],
    supportedAudioCodecs: ["aac", "mp3"],
    maxAudioChannels: 2,
    maxVideoBitDepth: 8,
    supportedVideoProfiles: ["h264:baseline", "h264:main", "h264:high"],
    supportedPixelFormats: ["yuv420p", "yuvj420p"],
    supportedHdrFormats: [],
    supportedDolbyVisionProfiles: [],
    prefersServerProxy: true,
    requiresServerProxy: true,
    ...overrides
  };
}
