import assert from "node:assert/strict";
import test from "node:test";
import {
  APPLE_PLAYBACK_CONTRACT_VERSION,
  PLAYBACK_CAPABILITY_CONTRACT_VERSION,
  applePlaybackCapabilityProfile,
  playbackCapabilityFixtures,
  playbackCapabilityProfile,
  playbackConformanceFixture,
  playbackConformanceFixtures
} from "../dist/index.js";

const appleTV = {
  platform: "tvos",
  deviceName: "Living Room Apple TV",
  maxWidth: 3840,
  maxHeight: 2160,
  maxAudioChannels: 8,
  supportsHevc: true,
  supportsHdr10: true,
  supportsHlg: true,
  supportsDolbyVision: true,
  dolbyVisionProfiles: ["5", "8"],
  supportsAc3: true,
  supportsEac3: true,
  supportsAtmos: true,
  supportedAudioCodecs: ["aac", "alac"],
  supportedSubtitleCodecs: ["webvtt", "mov_text"]
};

test("Apple playback profile exposes explicit AVKit capabilities and route limits", () => {
  const profile = applePlaybackCapabilityProfile(appleTV, {
    route: "remote",
    maxBitrateKbps: 20_000,
    maxWidth: 1920,
    maxHeight: 1080
  });

  assert.equal(profile.version, APPLE_PLAYBACK_CONTRACT_VERSION);
  assert.equal(profile.playerAdapter, "avkit");
  assert.deepEqual(profile.delivery, { direct_play: true, hls: true, remux: true, transcode: true });
  assert.equal(profile.limits.maxWidth, 1920);
  assert.equal(profile.limits.maxHeight, 1080);
  assert.equal(profile.limits.maxBitrateKbps, 20_000);
  assert.equal(profile.clientProfile.maxBitrate, 20_000_000);
  assert.equal(profile.clientProfile.maxAudioChannels, 8);
  assert.equal(profile.clientProfile.capabilitySchemaVersion, "playback-capability-v2");
  assert.equal(profile.clientProfile.clientFamily, "avkit");
  assert.equal(profile.clientProfile.capabilityEvidence[0].source, "native_runtime");
  assert.ok(profile.clientProfile.capabilityEvidence[0].tuples.length >= 6);
  const appleTuples = profile.clientProfile.capabilityEvidence[0].tuples;
  assert.ok(appleTuples.some(tuple => tuple.mediaKind === "audiovisual" && tuple.video?.codec === "h264" && tuple.audio === undefined));
  assert.ok(appleTuples.some(tuple => tuple.protocol === "hls" && tuple.container === "mp4" && tuple.video?.codec === "hevc"));
  assert.equal(appleTuples.some(tuple => tuple.protocol === "hls" && tuple.container === "mpegts" && tuple.video?.codec === "hevc"), false);
  assert.equal(profile.video.maxBitDepth, 10);
  assert.deepEqual(profile.video.dolbyVisionProfiles, ["5", "8"]);
  assert.ok(profile.video.hdrFormats.includes("hdr10"));
  assert.ok(profile.audio.codecs.includes("eac3"));
  assert.equal(profile.audio.atmos, true);
  assert.deepEqual(profile.subtitles.modes, ["native_embedded", "native_sidecar", "burn_in", "off"]);
  assert.equal(profile.subtitles.unsupportedCodecFallback, "burn_in");
  assert.equal(profile.clientProfile.requiresServerProxy, true);
});

test("day-one capability fixtures are version-banded, evidence-backed, and coherent", () => {
  const expectedFamilies = ["chromium", "edge", "safari", "firefox", "avkit", "avkit", "avkit", "media3", "media3", "fire-tv", "roku", "cast", "tizen", "webos", "dlna"];
  assert.deepEqual(playbackCapabilityFixtures.map((fixture) => fixture.family), expectedFamilies);
  for (const fixture of playbackCapabilityFixtures) {
    assert.ok(fixture.facts.evidence.minVersion.length > 0);
    assert.ok(fixture.dimensions.includes("baseline-video"));
    assert.ok(fixture.dimensions.includes("audio-only"));
    assert.ok(fixture.dimensions.includes("subtitle"));
    assert.ok(fixture.dimensions.includes("live-tv"));
    assert.ok(fixture.dimensions.includes("dvr"));
    const built = playbackCapabilityProfile(fixture.facts);
    assert.equal(built.version, PLAYBACK_CAPABILITY_CONTRACT_VERSION);
    assert.equal(built.family, fixture.family);
    assert.deepEqual(built, fixture.wireProfile);
    assert.equal(built.clientProfile.capabilitySchemaVersion, "playback-capability-v2");
    assert.equal(built.clientProfile.capabilityEvidence[0].tuples.length, fixture.facts.evidence.tuples.length);
    assert.ok(built.clientProfile.capabilityEvidence[0].tuples.some(tuple => tuple.mediaKind === "audiovisual" && tuple.audio === undefined), `${fixture.id} lacks exact silent-video evidence`);
    assert.ok(built.clientProfile.maxWidth > 0);
    assert.ok(built.clientProfile.maxHeight > 0);
    if (!built.clientProfile.supportsHevc) {
      assert.equal(built.clientProfile.maxVideoBitDepth, 8);
      assert.equal(built.clientProfile.supportsHdr, false);
      assert.deepEqual(built.clientProfile.supportedHdrFormats, []);
      assert.deepEqual(built.clientProfile.supportedDolbyVisionProfiles, []);
    }
  }
});

test("canonical builder drops contradictory HDR and Dolby Vision claims", () => {
  const baseline = playbackCapabilityFixtures.find(fixture => fixture.family === "safari").facts;
  const profile = playbackCapabilityProfile({
    ...baseline, family: "safari", clientVersion: "17", platform: "web", device: "Safari",
    containers: ["hls", "mp4"], videoCodecs: ["h264"], audioCodecs: ["aac"],
    maxWidth: 1920, maxHeight: 1080, maxVideoBitDepth: 10, supportsHls: true,
    hdrFormats: ["hdr10", "dolby_vision"], dolbyVisionProfiles: ["5", "8"]
  }).clientProfile;
  assert.equal(profile.supportsHevc, false);
  assert.equal(profile.maxVideoBitDepth, 8);
  assert.deepEqual(profile.supportedHdrFormats, []);
  assert.deepEqual(profile.supportedDolbyVisionProfiles, []);
});

test("Apple profile never advertises HDR, Dolby Vision, or Atmos without device support", () => {
  const profile = applePlaybackCapabilityProfile({
    ...appleTV,
    platform: "ios",
    deviceName: "iPhone",
    maxAudioChannels: 2,
    supportsHevc: false,
    supportsHdr10: false,
    supportsHlg: false,
    supportsDolbyVision: false,
    supportsEac3: false,
    supportsAtmos: false
  }, { route: "local" });

  assert.equal(profile.video.maxBitDepth, 8);
  assert.deepEqual(profile.video.hdrFormats, []);
  assert.deepEqual(profile.video.dolbyVisionProfiles, []);
  assert.equal(profile.audio.atmos, false);
  assert.equal(profile.clientProfile.supportsHevc, false);
  assert.equal(profile.clientProfile.supportsHdr, false);
  assert.equal(profile.limits.maxBitrateKbps, undefined);
});

test("Apple profile fails conservative on contradictory or incomplete native facts", () => {
  const profile = applePlaybackCapabilityProfile({
    ...appleTV,
    maxWidth: Number.NaN,
    maxHeight: 0,
    maxAudioChannels: Number.NaN,
    supportsHevc: false,
    supportsHdr10: true,
    supportsHlg: true,
    supportsDolbyVision: true,
    dolbyVisionProfiles: undefined
  }, { route: "remote", maxBitrateKbps: 8_000 });

  assert.equal(profile.limits.maxWidth, 1920);
  assert.equal(profile.limits.maxHeight, 1080);
  assert.equal(profile.audio.maxChannels, 2);
  assert.deepEqual(profile.video.hdrFormats, []);
  assert.deepEqual(profile.video.dolbyVisionProfiles, []);
  assert.equal(profile.clientProfile.maxBitrate, 8_000_000);
});

test("playback conformance fixtures cover every required native outcome", () => {
  const scenarios = playbackConformanceFixtures.map((fixture) => fixture.scenario);
  assert.deepEqual(scenarios, [
    "direct_play",
    "remux",
    "transcode",
    "grant_expiry",
    "queue_conflict",
    "restore",
    "playback_failure"
  ]);
  assert.equal(new Set(playbackConformanceFixtures.map((fixture) => fixture.id)).size, playbackConformanceFixtures.length);

  assert.equal(playbackConformanceFixture("direct_play").expected.decisionMode, "direct_play");
  assert.equal(playbackConformanceFixture("remux").expected.decisionMode, "direct_stream");
  assert.equal(playbackConformanceFixture("transcode").expected.decisionMode, "transcode_required");
  for (const scenario of ["direct_play", "remux", "transcode"]) {
    const fixture = playbackConformanceFixture(scenario);
    assert.equal(fixture.response.sourceUrl, fixture.response.resources[0].sourceUrl);
    assert.equal(fixture.response.resources[0].default, true);
  }
  assert.deepEqual(playbackConformanceFixture("grant_expiry").expected, {
    httpStatus: 401,
    code: "media_grant_expired",
    recovery: "renew_grant"
  });
  assert.equal(playbackConformanceFixture("queue_conflict").expected.recovery, "reload_queue");
  assert.equal(playbackConformanceFixture("restore").expected.recovery, "restore_session");
  assert.equal(playbackConformanceFixture("playback_failure").expected.recovery, "present_failure");
});
