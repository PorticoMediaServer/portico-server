import type { PlaybackClientProfile } from "./types.js";
import { genericPlaybackClientProfile } from "./genericPlaybackProfile.js";

export { genericPlaybackClientProfile } from "./genericPlaybackProfile.js";

export const PLAYBACK_CAPABILITY_CONTRACT_VERSION = "playback-capability-v2" as const;

export type PlaybackClientFamily =
  | "chromium" | "edge" | "safari" | "firefox" | "avkit" | "media3"
  | "fire-tv" | "roku" | "cast" | "tizen" | "webos" | "dlna";

export interface PlaybackCapabilityEvidence {
  id: string;
  source: "native_runtime" | "authenticated_runtime" | "unauthenticated_probe";
  confidence: "low" | "medium" | "high";
  producer: string;
  producerVersion?: string;
  reviewedAt: string;
  minVersion?: string;
  maxVersion?: string;
  tuples: readonly PlaybackCapabilityTuple[];
}

export type PlaybackCapabilityTuple = {
  mediaKind: "audio" | "audiovisual";
  protocol: "http" | "hls";
  container: string;
  video?: {codec: string; profile?: string; level?: string; tag?: string; pixelFormat?: string; chroma?: string; dynamicRange?: "sdr" | "pq" | "hlg" | "hdr10plus" | "dolby_vision"; bitDepth: number; dolbyVisionProfile?: number; maxWidth: number; maxHeight: number; maxFrameRate: number};
  /** Omitted only for an exact silent-video capability tuple. */
  audio?: {codec: string; profile?: string; layout?: string; route?: string; maxChannels: number; objectPassthrough?: boolean};
  subtitle: {codec?: string; kind?: "text" | "bitmap"; mode: "none" | "native" | "convert" | "burn"};
};

export interface PlaybackCapabilityFacts {
  family: PlaybackClientFamily;
  clientVersion: string;
  platform: string;
  device: string;
  evidence: PlaybackCapabilityEvidence;
  containers: readonly string[];
  videoCodecs: readonly string[];
  audioCodecs: readonly string[];
  maxWidth: number;
  maxHeight: number;
  maxAudioChannels?: number;
  maxVideoBitDepth?: 8 | 10 | 12;
  hdrFormats?: readonly ("hdr10" | "hlg" | "dolby_vision")[];
  dolbyVisionProfiles?: readonly string[];
  videoProfiles?: readonly string[];
  pixelFormats?: readonly string[];
  supportsHls: boolean;
  supportsMse?: boolean;
  supportsMpegTs?: boolean;
  supportsAc3?: boolean;
  supportsEac3?: boolean;
}

export interface VersionedPlaybackCapabilityProfile {
  version: typeof PLAYBACK_CAPABILITY_CONTRACT_VERSION;
  family: PlaybackClientFamily;
  evidence: Readonly<PlaybackCapabilityEvidence>;
  clientProfile: PlaybackClientProfile;
}

/**
 * Canonical boundary for facts measured by a browser or native-player adapter.
 * It rejects dependent HDR/10-bit claims unless the same evidence advertises HEVC.
 */
export function playbackCapabilityProfile(facts: PlaybackCapabilityFacts): VersionedPlaybackCapabilityProfile {
  validateCapabilityFacts(facts);
  const videoCodecs = unique(facts.videoCodecs);
  const supportsHevc = videoCodecs.includes("hevc") || videoCodecs.includes("h265");
  const hdrFormats = supportsHevc ? unique(facts.hdrFormats ?? []) : [];
  const maxVideoBitDepth = supportsHevc ? facts.maxVideoBitDepth ?? 8 : 8;
  const profile: PlaybackClientProfile = {
    capabilitySchemaVersion: PLAYBACK_CAPABILITY_CONTRACT_VERSION,
    clientFamily: facts.family,
    clientVersion: facts.clientVersion,
    capabilityEvidence: [{...facts.evidence, tuples: facts.evidence.tuples.map(tuple => ({...tuple, video: tuple.video ? {...tuple.video} : undefined, audio: tuple.audio ? {...tuple.audio} : undefined, subtitle: {...tuple.subtitle}}))}],
    device: facts.device,
    platform: facts.platform,
    supportsHls: facts.supportsHls,
    supportsMse: facts.supportsMse ?? false,
    supportsMpegTs: facts.supportsMpegTs ?? false,
    supportedContainers: unique(facts.containers),
    supportedVideoCodecs: videoCodecs,
    supportedAudioCodecs: unique(facts.audioCodecs),
    maxWidth: positiveInteger(facts.maxWidth),
    maxHeight: positiveInteger(facts.maxHeight),
    maxAudioChannels: positiveInteger(facts.maxAudioChannels) ?? 2,
    maxVideoBitDepth,
    supportsHevc,
    supportsHdr: hdrFormats.length > 0,
    supportsAc3: facts.supportsAc3 ?? false,
    supportsEac3: facts.supportsEac3 ?? false,
    supportedVideoProfiles: unique(facts.videoProfiles ?? []),
    supportedPixelFormats: unique(facts.pixelFormats ?? ["yuv420p"]),
    supportedHdrFormats: hdrFormats,
    supportedDolbyVisionProfiles: hdrFormats.includes("dolby_vision") ? unique(facts.dolbyVisionProfiles ?? []) : [],
    prefersServerProxy: true,
    requiresServerProxy: true
  };
  return { version: PLAYBACK_CAPABILITY_CONTRACT_VERSION, family: facts.family, evidence: { ...facts.evidence }, clientProfile: profile };
}

type FamilyFacts<Family extends PlaybackClientFamily> = PlaybackCapabilityFacts & { family: Family };

export function webPlaybackCapabilityProfile(facts: FamilyFacts<"chromium" | "edge" | "safari" | "firefox">): VersionedPlaybackCapabilityProfile {
  return playbackCapabilityProfile(facts);
}

export function appleFamilyPlaybackCapabilityProfile(facts: FamilyFacts<"avkit">): VersionedPlaybackCapabilityProfile {
  return playbackCapabilityProfile(facts);
}

export function androidPlaybackCapabilityProfile(facts: FamilyFacts<"media3" | "fire-tv">): VersionedPlaybackCapabilityProfile {
  return playbackCapabilityProfile(facts);
}

export function televisionPlaybackCapabilityProfile(facts: FamilyFacts<"roku" | "cast" | "tizen" | "webos" | "dlna">): VersionedPlaybackCapabilityProfile {
  return playbackCapabilityProfile(facts);
}

export function browserPlaybackClientProfile(): PlaybackClientProfile {
  const global = globalThis as typeof globalThis & {
    document?: Document;
    navigator?: Navigator;
    screen?: Screen;
    MediaSource?: unknown;
    devicePixelRatio?: number;
  };
  const video = global.document?.createElement("video");
  if (!video) return genericPlaybackClientProfile({ platform: "browser-unavailable" });
  const canPlay = (type: string) => video.canPlayType(type) !== "";
  const supportsNativeHls = canPlay("application/vnd.apple.mpegurl") || canPlay("application/x-mpegURL");
  const supportsMse = typeof global.MediaSource !== "undefined";
  const supportsHls = supportsNativeHls || supportsMse;
  const supportsMpegTs = canPlay("video/mp2t");
  const userAgent = global.navigator?.userAgent ?? "";
  const hostPlatform = global.navigator?.platform ?? "unknown";
  const platform = "web";
  const { family, version } = browserFamilyAndVersion(userAgent);
  const ratio = global.devicePixelRatio || 1;
  const webAudioCodecs = unique([
    canPlay('audio/mp4; codecs="mp4a.40.2"') ? "aac" : "",
    canPlay("audio/mpeg") ? "mp3" : "",
    canPlay('audio/webm; codecs="opus"') ? "opus" : "",
    canPlay('audio/webm; codecs="vorbis"') ? "vorbis" : ""
  ]);
  const containers = unique([
    canPlay("video/mp4") ? "mp4" : "",
    canPlay("video/mp4") ? "m4v" : "",
    canPlay("video/mp4") ? "mov" : "",
    canPlay("video/webm") ? "webm" : "",
    supportsHls ? "hls" : "",
    supportsMpegTs ? "mpegts" : ""
  ]);
  const videoCodecs = unique([
    canPlay('video/mp4; codecs="avc1.42E01E"') ? "h264" : "",
    canPlay('video/mp4; codecs="avc1.42E01E"') ? "avc1" : "",
    canPlay('video/webm; codecs="vp8"') ? "vp8" : "",
    canPlay('video/webm; codecs="vp9"') ? "vp9" : "",
    canPlay('video/mp4; codecs="av01.0.05M.08"') ? "av1" : ""
  ]);
  const maxWidth = global.screen ? Math.max(1, Math.round(global.screen.width * ratio)) : 1280;
  const maxHeight = global.screen ? Math.max(1, Math.round(global.screen.height * ratio)) : 720;
  const tuples = browserCapabilityTuples({ containers, videoCodecs, audioCodecs: webAudioCodecs, supportsHls, maxWidth, maxHeight });
  if (tuples.length === 0) return genericPlaybackClientProfile({ device: userAgent || "Browser", platform });
  return webPlaybackCapabilityProfile({
    family,
    clientVersion: version,
    device: [userAgent || family, hostPlatform].filter(Boolean).join(" · "),
    platform,
    evidence: {
      id: `portico-web-runtime-${family}-${version}`,
      source: "unauthenticated_probe",
      confidence: "medium",
      producer: "portico-web-runtime",
      reviewedAt: new Date().toISOString(),
      minVersion: version,
      maxVersion: version,
      tuples
    },
    containers,
    // Browser canPlayType() is too optimistic for smooth HEVC web playback.
    // Native clients should opt into HEVC with their own platform profile.
    videoCodecs,
    audioCodecs: webAudioCodecs,
    maxWidth,
    maxHeight,
    maxAudioChannels: 2,
    maxVideoBitDepth: 8,
    supportsEac3: false,
    supportsAc3: false,
    videoProfiles: ["h264:baseline", "h264:main", "h264:high"],
    pixelFormats: ["yuv420p", "yuvj420p"],
    hdrFormats: [],
    dolbyVisionProfiles: [],
    supportsHls,
    supportsMse,
    supportsMpegTs
  }).clientProfile;
}

function browserFamilyAndVersion(userAgent: string): {family: "chromium" | "edge" | "safari" | "firefox"; version: string} {
  const candidates: Array<{family: "chromium" | "edge" | "safari" | "firefox"; expression: RegExp}> = [
    { family: "edge", expression: /Edg\/([\d.]+)/ },
    { family: "firefox", expression: /Firefox\/([\d.]+)/ },
    { family: "safari", expression: /Version\/([\d.]+).*Safari\// },
    { family: "chromium", expression: /(?:Chrome|Chromium)\/([\d.]+)/ }
  ];
  for (const candidate of candidates) {
    const match = userAgent.match(candidate.expression);
    if (match?.[1]) return { family: candidate.family, version: match[1] };
  }
  return { family: "chromium", version: "0" };
}

function browserCapabilityTuples(input: {containers: readonly string[]; videoCodecs: readonly string[]; audioCodecs: readonly string[]; supportsHls: boolean; maxWidth: number; maxHeight: number}): PlaybackCapabilityTuple[] {
  const tuples: PlaybackCapabilityTuple[] = [];
  const noSubtitle = { mode: "none" } as const;
  const textSubtitle = { codec: "webvtt", kind: "text", mode: "native" } as const;
  const addAudiovisual = (protocol: "http" | "hls", container: string, video: NonNullable<PlaybackCapabilityTuple["video"]>, audio?: PlaybackCapabilityTuple["audio"]) => {
    // A silent source is an exact audiovisual capability, not an AAC source
    // with a missing stream. Keep its tuple independent of audio support.
    tuples.push({ mediaKind: "audiovisual", protocol, container, video, subtitle: noSubtitle });
    if (audio) {
      tuples.push({ mediaKind: "audiovisual", protocol, container, video, audio, subtitle: noSubtitle });
      tuples.push({ mediaKind: "audiovisual", protocol, container, video, audio, subtitle: textSubtitle });
    }
  };
  if (input.containers.includes("mp4") && input.videoCodecs.includes("h264")) {
    const audio = input.audioCodecs.includes("aac") ? { codec: "aac", profile: "lc", layout: "stereo", route: "decode", maxChannels: 2 } as const : undefined;
    for (const profile of ["baseline", "main", "high"] as const) {
      const video = { codec: "h264", profile, pixelFormat: "yuv420p", chroma: "4:2:0", dynamicRange: "sdr", bitDepth: 8, maxWidth: input.maxWidth, maxHeight: input.maxHeight, maxFrameRate: 60 } as const;
      addAudiovisual("http", "mp4", video, audio);
      // Portico's H.264 HLS executor emits MPEG-TS segments. The Web player
      // consumes them through native HLS or the bundled managed HLS runtime.
      if (input.supportsHls) addAudiovisual("hls", "mpegts", video, audio);
    }
  }
  if (input.containers.includes("webm") && input.videoCodecs.includes("vp9")) {
    const audio = input.audioCodecs.includes("opus") ? { codec: "opus", layout: "stereo", route: "decode", maxChannels: 2 } as const : undefined;
    addAudiovisual("http", "webm", { codec: "vp9", pixelFormat: "yuv420p", chroma: "4:2:0", dynamicRange: "sdr", bitDepth: 8, maxWidth: input.maxWidth, maxHeight: input.maxHeight, maxFrameRate: 60 }, audio);
  }
  if (input.audioCodecs.includes("mp3")) tuples.push({ mediaKind: "audio", protocol: "http", container: "mp3", audio: { codec: "mp3", layout: "stereo", route: "decode", maxChannels: 2 }, subtitle: { mode: "none" } });
  return tuples;
}

function unique(items: readonly string[]): string[] {
  return [...new Set(items.map((item) => item.trim()).filter(Boolean))];
}

function positiveInteger(value: number | undefined): number | undefined {
  return value !== undefined && Number.isFinite(value) && value > 0 ? Math.trunc(value) : undefined;
}

function validateCapabilityFacts(facts: PlaybackCapabilityFacts): void {
  if (!facts.clientVersion.trim() || !facts.platform.trim() || !facts.device.trim()) throw new TypeError("Playback client identity is incomplete.");
  const evidence = facts.evidence;
  if (!evidence.id.trim() || !evidence.producer.trim() || !Number.isFinite(Date.parse(evidence.reviewedAt)) || evidence.tuples.length === 0 || evidence.tuples.length > 256) {
    throw new TypeError("Playback capability evidence is invalid.");
  }
  if ((evidence.source === "native_runtime" || evidence.source === "authenticated_runtime") && !evidence.producerVersion?.trim()) {
    throw new TypeError("Authoritative runtime evidence requires a producer version.");
  }
  for (const tuple of evidence.tuples) {
    if (!tuple.protocol.trim() || !tuple.container.trim()) throw new TypeError("Playback capability tuple is invalid.");
    if (tuple.mediaKind === "audio") {
      if (!tuple.audio || !tuple.audio.codec.trim() || tuple.audio.maxChannels < 1) throw new TypeError("Audio-only tuples require bounded audio facts.");
      if (tuple.video || tuple.subtitle.mode !== "none") throw new TypeError("Audio-only tuples cannot declare video or subtitles.");
    } else if (!tuple.video || tuple.video.bitDepth < 1 || tuple.video.maxWidth < 1 || tuple.video.maxHeight < 1 || tuple.video.maxFrameRate <= 0) {
      throw new TypeError("Audiovisual tuples require bounded video facts.");
    }
    if (tuple.audio && (!tuple.audio.codec.trim() || tuple.audio.maxChannels < 1)) throw new TypeError("Playback capability audio facts are invalid.");
    if (tuple.video?.dolbyVisionProfile && tuple.video.dynamicRange !== "dolby_vision") throw new TypeError("Dolby Vision profiles require a Dolby Vision tuple.");
    if (tuple.audio?.objectPassthrough && tuple.audio.route !== "passthrough") throw new TypeError("Object audio requires passthrough.");
    if (tuple.subtitle.mode === "none" ? Boolean(tuple.subtitle.codec || tuple.subtitle.kind) : !tuple.subtitle.codec || !tuple.subtitle.kind) throw new TypeError("Subtitle tuple is incoherent.");
  }
}
