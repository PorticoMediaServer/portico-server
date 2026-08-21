import type { PlaybackClientProfile } from "./types.js";
import { appleFamilyPlaybackCapabilityProfile, type PlaybackCapabilityTuple } from "./playbackProfiles.js";

export const APPLE_PLAYBACK_CONTRACT_VERSION = "apple-playback.v1" as const;
const appleCapabilityBuilderLoadedAt = new Date().toISOString();

export type ApplePlaybackPlatform = "ios" | "tvos" | "macos";
export type ApplePlaybackRoute = "local" | "remote";
export type AppleSubtitleMode = "native_embedded" | "native_sidecar" | "burn_in" | "off";
export type ApplePlaybackDelivery = "direct_play" | "hls" | "remux" | "transcode";

/**
 * Capability facts supplied by the Apple shell after inspecting the current
 * AVFoundation device/runtime. Client Core deliberately does not guess these
 * from a model name or OS version.
 */
export interface ApplePlaybackDeviceCapabilities {
  platform: ApplePlaybackPlatform;
  deviceName: string;
  /** Native OS/player runtime version observed by the bridge. */
  clientVersion?: string;
  /** RFC 3339 instant at which the native bridge measured these facts. */
  observedAt?: string;
  maxWidth: number;
  maxHeight: number;
  maxFrameRate?: number;
  maxAudioChannels: number;
  supportsHevc: boolean;
  supportsHdr10: boolean;
  supportsHlg: boolean;
  supportsDolbyVision: boolean;
  dolbyVisionProfiles?: readonly string[];
  supportsAc3: boolean;
  supportsEac3: boolean;
  supportsAtmos: boolean;
  supportedAudioCodecs?: readonly string[];
  supportedSubtitleCodecs?: readonly string[];
}

export interface ApplePlaybackRouteLimits {
  route: ApplePlaybackRoute;
  /** Kilobits per second. Omit or use zero for no client-requested ceiling. */
  maxBitrateKbps?: number;
  /** Remote policy may constrain resolution independently of the device. */
  maxWidth?: number;
  maxHeight?: number;
}

export interface ApplePlaybackCapabilityProfile {
  version: typeof APPLE_PLAYBACK_CONTRACT_VERSION;
  playerAdapter: "avkit";
  platform: ApplePlaybackPlatform;
  route: ApplePlaybackRoute;
  delivery: Readonly<Record<ApplePlaybackDelivery, boolean>>;
  video: {
    codecs: readonly string[];
    profiles: readonly string[];
    pixelFormats: readonly string[];
    hdrFormats: readonly string[];
    dolbyVisionProfiles: readonly string[];
    maxBitDepth: 8 | 10;
  };
  audio: {
    codecs: readonly string[];
    maxChannels: number;
    atmos: boolean;
  };
  subtitles: {
    modes: readonly AppleSubtitleMode[];
    nativeCodecs: readonly string[];
    unsupportedCodecFallback: "burn_in";
  };
  limits: {
    maxWidth: number;
    maxHeight: number;
    maxBitrateKbps?: number;
  };
  clientProfile: PlaybackClientProfile;
}

/**
 * Produces the single Apple playback request/profile contract shared by iOS
 * and tvOS. AVKit remains app-owned; this object is safe to pass through a
 * React Native bridge and contains no browser or native framework types.
 */
export function applePlaybackCapabilityProfile(
  capabilities: ApplePlaybackDeviceCapabilities,
  route: ApplePlaybackRouteLimits
): ApplePlaybackCapabilityProfile {
  const supportsHdr10 = capabilities.supportsHevc && capabilities.supportsHdr10;
  const supportsHlg = capabilities.supportsHevc && capabilities.supportsHlg;
  const supportsDolbyVision = capabilities.supportsHevc && capabilities.supportsDolbyVision;
  const hdrFormats = compact([
    supportsHdr10 ? "hdr10" : "",
    supportsHlg ? "hlg" : "",
    supportsDolbyVision ? "dolby_vision" : ""
  ]);
  const dolbyVisionProfiles = supportsDolbyVision
    ? unique(capabilities.dolbyVisionProfiles ?? [])
    : [];
  const videoCodecs = compact(["h264", "avc1", capabilities.supportsHevc ? "hevc" : "", capabilities.supportsHevc ? "h265" : ""]);
  const audioCodecs = unique([
    "aac",
    "mp3",
    ...(capabilities.supportedAudioCodecs ?? []),
    capabilities.supportsAc3 ? "ac3" : "",
    capabilities.supportsEac3 ? "eac3" : ""
  ]);
  const nativeSubtitleCodecs = unique(capabilities.supportedSubtitleCodecs ?? ["webvtt", "mov_text", "tx3g"]);
  const maxWidth = positiveMinimum(capabilities.maxWidth, route.maxWidth, 1920);
  const maxHeight = positiveMinimum(capabilities.maxHeight, route.maxHeight, 1080);
  const maxBitrateKbps = positive(route.maxBitrateKbps);
  const maxBitDepth: 8 | 10 = capabilities.supportsHevc && hdrFormats.length > 0 ? 10 : 8;
  const videoProfiles = compact([
    "h264:baseline",
    "h264:main",
    "h264:high",
    capabilities.supportsHevc ? "hevc:main" : "",
    capabilities.supportsHevc && maxBitDepth === 10 ? "hevc:main 10" : ""
  ]);
  const pixelFormats = maxBitDepth === 10 ? ["yuv420p", "yuv420p10le", "p010le"] : ["yuv420p"];

  const clientVersion = capabilities.clientVersion?.trim() || "unknown";
  const observedAt = validObservationTime(capabilities.observedAt) ?? appleCapabilityBuilderLoadedAt;
  const tuples = appleCapabilityTuples({capabilities, maxWidth, maxHeight, maxBitDepth, hdrFormats});
  const canonical = appleFamilyPlaybackCapabilityProfile({
    family: "avkit",
    clientVersion,
    platform: capabilities.platform,
    device: capabilities.deviceName,
    evidence: {
      id: `portico-apple-runtime-${capabilities.platform}-${clientVersion}`,
      source: "native_runtime",
      confidence: "high",
      producer: "portico-apple-native",
      producerVersion: clientVersion,
      reviewedAt: observedAt,
      minVersion: clientVersion,
      maxVersion: clientVersion,
      tuples
    },
    containers: ["hls", "mp4", "m4v", "mov", "mpegts", "m4a", "mp3"],
    videoCodecs,
    audioCodecs,
    maxWidth,
    maxHeight,
    maxAudioChannels: positiveInteger(capabilities.maxAudioChannels) ?? 2,
    maxVideoBitDepth: maxBitDepth,
    hdrFormats: hdrFormats as Array<"hdr10" | "hlg" | "dolby_vision">,
    dolbyVisionProfiles,
    videoProfiles,
    pixelFormats,
    supportsHls: true,
    supportsMse: false,
    supportsMpegTs: true,
    supportsEac3: capabilities.supportsEac3,
    supportsAc3: capabilities.supportsAc3
  }).clientProfile;
  const clientProfile: PlaybackClientProfile = {
    ...canonical,
    // PlaybackClientProfile.maxBitrate is expressed in bits per second; the
    // route-facing Apple contract uses kbps to match native networking APIs.
    ...(maxBitrateKbps ? {maxBitrate: maxBitrateKbps * 1_000} : {})
  };

  return {
    version: APPLE_PLAYBACK_CONTRACT_VERSION,
    playerAdapter: "avkit",
    platform: capabilities.platform,
    route: route.route,
    delivery: { direct_play: true, hls: true, remux: true, transcode: true },
    video: { codecs: videoCodecs, profiles: videoProfiles, pixelFormats, hdrFormats, dolbyVisionProfiles, maxBitDepth },
    audio: {
      codecs: audioCodecs,
      maxChannels: clientProfile.maxAudioChannels ?? 2,
      atmos: capabilities.supportsAtmos && capabilities.supportsEac3
    },
    subtitles: {
      modes: ["native_embedded", "native_sidecar", "burn_in", "off"],
      nativeCodecs: nativeSubtitleCodecs,
      unsupportedCodecFallback: "burn_in"
    },
    limits: { maxWidth, maxHeight, ...(maxBitrateKbps ? { maxBitrateKbps } : {}) },
    clientProfile
  };
}

function appleCapabilityTuples(input: {
  capabilities: ApplePlaybackDeviceCapabilities;
  maxWidth: number;
  maxHeight: number;
  maxBitDepth: 8 | 10;
  hdrFormats: readonly string[];
}): PlaybackCapabilityTuple[] {
  const {capabilities, maxWidth, maxHeight, maxBitDepth, hdrFormats} = input;
  const maxFrameRate = positive(capabilities.maxFrameRate) ?? 60;
  const stereoAAC = {codec: "aac", profile: "lc", layout: "stereo", route: "decode", maxChannels: 2} as const;
  const none = {mode: "none"} as const;
  const webvtt = {codec: "webvtt", kind: "text", mode: "native"} as const;
  const h264 = {codec: "h264", profile: "high", pixelFormat: "yuv420p", chroma: "4:2:0", dynamicRange: "sdr", bitDepth: 8, maxWidth, maxHeight, maxFrameRate} as const;
  const tuples: PlaybackCapabilityTuple[] = [
    {mediaKind: "audiovisual", protocol: "http", container: "mp4", video: h264, audio: stereoAAC, subtitle: none},
    {mediaKind: "audiovisual", protocol: "http", container: "mp4", video: h264, audio: stereoAAC, subtitle: webvtt},
    {mediaKind: "audiovisual", protocol: "hls", container: "mpegts", video: h264, audio: stereoAAC, subtitle: none},
    {mediaKind: "audiovisual", protocol: "hls", container: "mpegts", video: h264, audio: stereoAAC, subtitle: webvtt},
    {mediaKind: "audio", protocol: "http", container: "m4a", audio: stereoAAC, subtitle: none},
    {mediaKind: "audio", protocol: "http", container: "mp3", audio: {codec: "mp3", layout: "stereo", route: "decode", maxChannels: 2}, subtitle: none}
  ];
  if (!capabilities.supportsHevc) return tuples;
  const ranges = hdrFormats.length ? hdrFormats : ["sdr"];
  for (const range of ranges) {
    const dynamicRange = range === "hdr10" ? "pq" : range;
    const profile = maxBitDepth === 10 ? "main10" : "main";
    const dolbyVisionProfile = dynamicRange === "dolby_vision"
      ? positiveInteger(Number.parseInt(capabilities.dolbyVisionProfiles?.[0] ?? "", 10))
      : undefined;
    const hevc = {
      codec: "hevc", profile, pixelFormat: maxBitDepth === 10 ? "yuv420p10le" : "yuv420p",
      chroma: "4:2:0", dynamicRange: dynamicRange as "sdr" | "pq" | "hlg" | "dolby_vision",
      bitDepth: maxBitDepth, maxWidth, maxHeight, maxFrameRate,
      ...(dolbyVisionProfile ? {dolbyVisionProfile} : {})
    };
    tuples.push({mediaKind: "audiovisual", protocol: "http", container: "mp4", video: hevc, audio: stereoAAC, subtitle: none});
  }
  if (capabilities.supportsEac3) {
    const channels = Math.min(positiveInteger(capabilities.maxAudioChannels) ?? 2, 8);
    tuples.push({mediaKind: "audiovisual", protocol: "http", container: "mp4", video: h264,
      audio: {codec: "eac3", layout: channels > 6 ? "7.1" : channels > 2 ? "5.1" : "stereo", route: "decode", maxChannels: channels}, subtitle: none});
  }
  return tuples;
}

function validObservationTime(value: string | undefined): string | undefined {
  return value && Number.isFinite(Date.parse(value)) ? new Date(value).toISOString() : undefined;
}

function compact(values: readonly string[]): string[] {
  return unique(values);
}

function unique(values: readonly string[]): string[] {
  return [...new Set(values.map((value) => value.trim().toLowerCase()).filter(Boolean))];
}

function positive(value: number | undefined): number | undefined {
  return value !== undefined && Number.isFinite(value) && value > 0 ? Math.trunc(value) : undefined;
}

function positiveInteger(value: number | undefined): number | undefined {
  return value !== undefined && Number.isFinite(value) && value > 0 ? Math.trunc(value) : undefined;
}

function positiveMinimum(primary: number, limit: number | undefined, fallback: number): number {
  const normalizedPrimary = positiveInteger(primary) ?? fallback;
  const normalizedLimit = positive(limit);
  return normalizedLimit ? Math.min(normalizedPrimary, normalizedLimit) : normalizedPrimary;
}
