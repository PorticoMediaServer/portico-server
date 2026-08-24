import { playbackCapabilityProfile, type PlaybackCapabilityFacts, type PlaybackCapabilityTuple, type PlaybackClientFamily, type VersionedPlaybackCapabilityProfile } from "./playbackProfiles.js";

export const PLAYBACK_CONFORMANCE_FIXTURE_VERSION = "playback-conformance.v1" as const;

export const PLAYBACK_CAPABILITY_FIXTURE_VERSION = "playback-capability-fixture.v1" as const;

export type PlaybackMediaDimension =
  | "baseline-video" | "hdr-video" | "audio-only" | "multichannel-audio" | "object-audio-fallback"
  | "subtitle" | "live-tv" | "dvr";

export interface PlaybackCapabilityFixture {
  id: string;
  version: typeof PLAYBACK_CAPABILITY_FIXTURE_VERSION;
  family: PlaybackClientFamily;
  dimensions: readonly PlaybackMediaDimension[];
  facts: PlaybackCapabilityFacts;
  /** Exact profile shape placed in PlaybackStartRequest.clientProfile. */
  wireProfile: VersionedPlaybackCapabilityProfile;
}

const baselineDimensions: readonly PlaybackMediaDimension[] = ["baseline-video", "audio-only", "subtitle", "live-tv", "dvr"];

function capabilityFixture(
  family: PlaybackClientFamily, platform: string, minVersion: string, maxVersion: string | undefined,
  overrides: Partial<PlaybackCapabilityFacts> & {tuples?: readonly PlaybackCapabilityTuple[]} = {},
  dimensions: readonly PlaybackMediaDimension[] = baselineDimensions
): PlaybackCapabilityFixture {
  const versionBand = maxVersion ? `${minVersion}-${maxVersion}` : `${minVersion}+`;
  const tuples = overrides.tuples ?? baselineTuples();
  const facts: PlaybackCapabilityFacts = {
      family, clientVersion: minVersion, platform,
      device: `Conformance ${family} ${platform}`,
      containers: ["hls", "mp4"], videoCodecs: ["h264"], audioCodecs: ["aac", "mp3"],
      maxWidth: 1920, maxHeight: 1080, maxAudioChannels: 2, maxVideoBitDepth: 8, supportsHls: true,
      ...overrides,
      evidence: {...(overrides.evidence ?? {}), id: `${family}-${platform}-${versionBand}`, source: "unauthenticated_probe", confidence: "low", producer: "portico-client-core-conformance", reviewedAt: new Date().toISOString(), minVersion, ...(maxVersion ? {maxVersion} : {}), tuples}
  };
  return {
    id: `${family}-${platform}-${versionBand}`,
    version: PLAYBACK_CAPABILITY_FIXTURE_VERSION,
    family,
    dimensions,
    facts,
    wireProfile: playbackCapabilityProfile(facts)
  };
}

function baselineTuples(): PlaybackCapabilityTuple[] {
  const video = {codec: "h264", profile: "high", pixelFormat: "yuv420p", chroma: "4:2:0", dynamicRange: "sdr" as const, bitDepth: 8, maxWidth: 1920, maxHeight: 1080, maxFrameRate: 60};
  const audio = {codec: "aac", layout: "stereo", route: "decode", maxChannels: 2};
  return [
    {mediaKind: "audiovisual", protocol: "hls", container: "mpegts", video, subtitle: {mode: "none"}},
    {mediaKind: "audiovisual", protocol: "hls", container: "mpegts", video, audio, subtitle: {mode: "none"}},
    {mediaKind: "audiovisual", protocol: "http", container: "mp4", video, subtitle: {mode: "none"}},
    {mediaKind: "audiovisual", protocol: "http", container: "mp4", video, audio, subtitle: {codec: "webvtt", kind: "text", mode: "native"}},
    {mediaKind: "audio", protocol: "http", container: "m4a", audio, subtitle: {mode: "none"}}
  ];
}

function premiumTelevisionTuples(): PlaybackCapabilityTuple[] {
  const tuples = baselineTuples();
  const hevc = {codec: "hevc", profile: "main10", pixelFormat: "yuv420p10le", chroma: "4:2:0", dynamicRange: "dolby_vision" as const, bitDepth: 10, dolbyVisionProfile: 5, maxWidth: 3840, maxHeight: 2160, maxFrameRate: 60};
  tuples.push(
    // HEVC/HDR HLS is declared only with Portico's CMAF/fMP4 executor path.
    {mediaKind: "audiovisual", protocol: "hls", container: "mp4", video: hevc, subtitle: {mode: "none"}},
    {mediaKind: "audiovisual", protocol: "hls", container: "mp4", video: hevc, audio: {codec: "eac3", layout: "5.1", route: "decode", maxChannels: 6}, subtitle: {codec: "webvtt", kind: "text", mode: "native"}},
    {mediaKind: "audiovisual", protocol: "hls", container: "mp4", video: hevc, audio: {codec: "eac3", profile: "joc", layout: "7.1", route: "passthrough", maxChannels: 8, objectPassthrough: true}, subtitle: {codec: "pgs", kind: "bitmap", mode: "burn"}},
    {mediaKind: "audiovisual", protocol: "hls", container: "mp4", video: hevc, audio: {codec: "aac", layout: "stereo", route: "decode", maxChannels: 2}, subtitle: {mode: "none"}}
  );
  return tuples;
}

/** Build-time typed, conservative evidence matrix. Native shells replace these
 * fixtures with measured facts before sending a wire profile. */
export const playbackCapabilityFixtures: readonly PlaybackCapabilityFixture[] = [
  capabilityFixture("chromium", "web", "120", "139", {supportsMse: true}), capabilityFixture("edge", "web", "120", "139", {supportsMse: true}),
  capabilityFixture("safari", "web", "17", "18"), capabilityFixture("firefox", "web", "121", "140", {supportsMse: true}),
  capabilityFixture("avkit", "ios", "17", "18"), capabilityFixture("avkit", "ipados", "17", "18"),
  capabilityFixture("avkit", "tvos", "17", "18", {tuples: premiumTelevisionTuples(), videoCodecs: ["h264", "hevc"], audioCodecs: ["aac", "eac3"], maxVideoBitDepth: 10, maxAudioChannels: 8, hdrFormats: ["dolby_vision"], dolbyVisionProfiles: ["5"], supportsEac3: true}, [...baselineDimensions, "hdr-video", "multichannel-audio", "object-audio-fallback"]),
  capabilityFixture("media3", "android", "10", "15"), capabilityFixture("media3", "android-tv", "10", "15"),
  capabilityFixture("fire-tv", "fireos", "7", "8", {tuples: premiumTelevisionTuples(), videoCodecs: ["h264", "hevc"], audioCodecs: ["aac", "eac3"], maxVideoBitDepth: 10, maxAudioChannels: 8, hdrFormats: ["dolby_vision"], dolbyVisionProfiles: ["5"], supportsEac3: true}, [...baselineDimensions, "hdr-video", "multichannel-audio", "object-audio-fallback"]),
  capabilityFixture("roku", "roku", "12", "14"), capabilityFixture("cast", "cast", "3", undefined),
  capabilityFixture("tizen", "tizen", "6", "8"), capabilityFixture("webos", "webos", "6", "24"),
  capabilityFixture("dlna", "dlna", "1", undefined),
] as const;

export type PlaybackConformanceScenario =
  | "direct_play"
  | "remux"
  | "transcode"
  | "grant_expiry"
  | "queue_conflict"
  | "restore"
  | "playback_failure";

export interface PlaybackConformanceFixture {
  id: string;
  version: typeof PLAYBACK_CONFORMANCE_FIXTURE_VERSION;
  scenario: PlaybackConformanceScenario;
  request: Readonly<Record<string, unknown>>;
  response: Readonly<Record<string, unknown>>;
  expected: {
    decisionMode?: "direct_play" | "direct_stream" | "transcode_required";
    httpStatus?: number;
    code?: string;
    recovery: "play" | "renew_grant" | "reload_queue" | "restore_session" | "present_failure";
  };
}

/**
 * Transport-neutral fixtures for every playback state a first-party client
 * must handle before real media wiring. Apps may consume these directly in
 * adapter tests; they intentionally contain no React Native or AVKit values.
 */
export const playbackConformanceFixtures: readonly PlaybackConformanceFixture[] = [
  {
    id: "apple-direct-play-h264-aac",
    version: PLAYBACK_CONFORMANCE_FIXTURE_VERSION,
    scenario: "direct_play",
    request: { mediaId: "movie_direct", route: "local", container: "mp4", videoCodec: "h264", audioCodec: "aac" },
    response: { sessionId: "play_direct", sourceUrl: "/api/playback-resources/direct", resources: [{ id: "direct", sourceUrl: "/api/playback-resources/direct", streamFormat: "mp4", qualityId: "original", subtitleMode: "off", default: true }], streamFormat: "mp4", decision: { mode: "direct_play", requiresTranscode: false, requiresRemux: false } },
    expected: { decisionMode: "direct_play", recovery: "play" }
  },
  {
    id: "apple-remux-mkv-h264",
    version: PLAYBACK_CONFORMANCE_FIXTURE_VERSION,
    scenario: "remux",
    request: { mediaId: "movie_remux", route: "local", container: "mkv", videoCodec: "h264", audioCodec: "aac" },
    response: { sessionId: "play_remux", sourceUrl: "/api/playback-resources/remux", resources: [{ id: "remux", sourceUrl: "/api/playback-resources/remux", streamFormat: "hls", qualityId: "original", subtitleMode: "off", default: true }], streamFormat: "hls", decision: { mode: "direct_stream", requiresTranscode: false, requiresRemux: true } },
    expected: { decisionMode: "direct_stream", recovery: "play" }
  },
  {
    id: "apple-transcode-incompatible-video",
    version: PLAYBACK_CONFORMANCE_FIXTURE_VERSION,
    scenario: "transcode",
    request: { mediaId: "movie_transcode", route: "remote", container: "mkv", videoCodec: "vp9", audioCodec: "dts" },
    response: { sessionId: "play_transcode", sourceUrl: "/api/playback-resources/transcode", resources: [{ id: "transcode", sourceUrl: "/api/playback-resources/transcode", streamFormat: "hls", qualityId: "standard", subtitleMode: "off", default: true }], streamFormat: "hls", decision: { mode: "transcode_required", requiresTranscode: true, videoTranscode: true, audioTranscode: true } },
    expected: { decisionMode: "transcode_required", recovery: "play" }
  },
  {
    id: "apple-expired-media-grant",
    version: PLAYBACK_CONFORMANCE_FIXTURE_VERSION,
    scenario: "grant_expiry",
    request: { sessionId: "play_expired", resource: "/api/media/movie_direct/stream", mediaGrant: "expired" },
    response: { status: 401, code: "media_grant_expired", detail: "The playback media grant has expired." },
    expected: { httpStatus: 401, code: "media_grant_expired", recovery: "renew_grant" }
  },
  {
    id: "apple-stale-queue-revision",
    version: PLAYBACK_CONFORMANCE_FIXTURE_VERSION,
    scenario: "queue_conflict",
    request: { sessionId: "play_queue", expectedRevision: 3, action: "reorder", fromIndex: 0, toIndex: 2 },
    response: { status: 409, code: "queue_revision_conflict", detail: "The playback queue changed. Reload it and try again.", details: { currentRevision: 4 } },
    expected: { httpStatus: 409, code: "queue_revision_conflict", recovery: "reload_queue" }
  },
  {
    id: "apple-restore-active-session",
    version: PLAYBACK_CONFORMANCE_FIXTURE_VERSION,
    scenario: "restore",
    request: { clientInstanceId: "apple-installation-player" },
    response: { active: true, playback: { sessionId: "play_restore", nextEventSequence: 9, resumePositionSeconds: 1260, queueRevision: 2 } },
    expected: { recovery: "restore_session" }
  },
  {
    id: "apple-playback-source-failure",
    version: PLAYBACK_CONFORMANCE_FIXTURE_VERSION,
    scenario: "playback_failure",
    request: { mediaId: "movie_missing" },
    response: { status: 404, code: "media_file_missing", detail: "This file is missing on the server." },
    expected: { httpStatus: 404, code: "media_file_missing", recovery: "present_failure" }
  }
] as const;

export function playbackConformanceFixture(scenario: PlaybackConformanceScenario): PlaybackConformanceFixture {
  const fixture = playbackConformanceFixtures.find((candidate) => candidate.scenario === scenario);
  if (!fixture) throw new Error(`Missing playback conformance fixture: ${scenario}`);
  return fixture;
}
