import assert from "node:assert/strict";
import test from "node:test";

import {
  applyHomeRowPreferences,
  applyPreferencePatch,
  createPreferenceDocument,
  defaultAccountServerInstallationPreferences,
  defaultProfileDeviceClassPreferences,
  normalizeProfileDeviceClassPreferences,
  normalizeProfileServerPreferences,
  parsePreferenceDocument,
  playbackIntentFromPreferences,
  PORTICO_PREFERENCES_VERSION,
  PreferenceConflictError,
  preferenceCapabilitiesForDeviceClass,
  preferenceStorageKeys,
  recordRecentPreferenceQuery,
  resolveDeliveryPreference,
  resolveMotionPreference,
  resolveQualityRequest
} from "../dist/preferences.js";

test("V1 preferences use explicit ownership scopes and platform defaults", () => {
  assert.equal(PORTICO_PREFERENCES_VERSION, "v1");
  assert.equal(defaultAccountServerInstallationPreferences("television").profileSelection, "ask");
  assert.equal(defaultAccountServerInstallationPreferences("mobile").profileSelection, "last-used");
  assert.deepEqual(
    Object.fromEntries(Object.entries(defaultProfileDeviceClassPreferences("mobile").playback.quality).map(([network, quality]) => [network, quality.mode])),
    { local: "original", wifi: "original", cellular: "original", unknown: "original" }
  );
  assert.deepEqual(defaultProfileDeviceClassPreferences("mobile").playback.deliveryRequest, {
    directPlay: "prefer", directStream: "allow", transcode: "allow"
  });
});

test("missing network quality defaults to Original while explicit lower requests survive normalization", () => {
  const normalized = normalizeProfileDeviceClassPreferences({
    playback: { quality: { wifi: { mode: "standard" }, cellular: { mode: "data-saver", maxVideoBitrateMbps: 3 } } }
  }, "mobile");
  assert.equal(normalized.playback.quality.local.mode, "original");
  assert.equal(normalized.playback.quality.unknown.mode, "original");
  assert.equal(normalized.playback.quality.wifi.mode, "standard");
  assert.deepEqual(normalized.playback.quality.cellular, {
    mode: "data-saver", maxVideoBitrateMbps: 3, maxAudioBitrateKbps: undefined, maxVideoHeight: undefined, allowHDR: true
  });
});

test("documents reject unknown versions and patches preserve omitted fields", () => {
  const document = createPreferenceDocument(normalizeProfileServerPreferences({ playback: { autoplayNext: true, skipBackSeconds: 10 } }), 7);
  assert.throws(() => parsePreferenceDocument({ ...document, version: "future" }, normalizeProfileServerPreferences), /unsupported/);
  assert.throws(() => parsePreferenceDocument({ version: "v1", revision: 1, values: null }, normalizeProfileServerPreferences), /object/);
  const updated = applyPreferencePatch(document, {
    version: "v1",
    expectedRevision: 7,
    changes: { playback: { autoplayNext: false } }
  }, normalizeProfileServerPreferences);
  assert.equal(updated.revision, 8);
  assert.equal(updated.values.playback.autoplayNext, false);
  assert.equal(updated.values.playback.skipBackSeconds, 10);
  assert.throws(() => applyPreferencePatch(updated, { version: "v1", expectedRevision: 7, changes: {} }, normalizeProfileServerPreferences), PreferenceConflictError);
  assert.throws(() => applyPreferencePatch({ ...updated, version: "future" }, { version: "v1", expectedRevision: 8, changes: {} }, normalizeProfileServerPreferences), /unsupported/);

  const cleared = applyPreferencePatch(createPreferenceDocument(normalizeProfileServerPreferences({ search: { recentQueries: ["private"] } }), 1), {
    version: "v1", expectedRevision: 1, changes: { search: { recentQueries: { $clear: true } } }
  }, normalizeProfileServerPreferences);
  assert.deepEqual(cleared.values.search.recentQueries, []);
});

test("server-owned identifiers are partitioned by server, account, and profile", () => {
  const first = preferenceStorageKeys({ authority: "hosted", accountId: "a", serverId: "s1", profileId: "p1", deviceClass: "mobile", installationId: "i" });
  const otherServer = preferenceStorageKeys({ authority: "hosted", accountId: "a", serverId: "s2", profileId: "p1", deviceClass: "mobile", installationId: "i" });
  const otherProfile = preferenceStorageKeys({ authority: "hosted", accountId: "a", serverId: "s1", profileId: "p2", deviceClass: "mobile", installationId: "i" });
  const otherDeviceClass = preferenceStorageKeys({ authority: "hosted", accountId: "a", serverId: "s1", profileId: "p1", deviceClass: "television", installationId: "i" });
  const local = preferenceStorageKeys({ authority: "local", accountId: "a", serverId: "s1", profileId: "p1", deviceClass: "mobile", installationId: "i" });
  assert.notEqual(first.profileDeviceClass, otherServer.profileDeviceClass);
  assert.notEqual(first.profileServer, otherProfile.profileServer);
  assert.notEqual(first.accountServerInstallation, otherServer.accountServerInstallation);
  assert.equal(first.accountServerInstallation, otherDeviceClass.accountServerInstallation);
  assert.notEqual(first.profileServer, local.profileServer);
  assert.equal(first.installation, otherServer.installation);
  assert.throws(() => preferenceStorageKeys({ authority: "hosted", accountId: "", serverId: "s", profileId: "p", deviceClass: "mobile", installationId: "i" }), /accountId/);
});

test("turning off search history clears retained profile search queries", () => {
  const preferences = normalizeProfileServerPreferences({ search: { rememberHistory: false, recentQueries: ["private query"] } });
  assert.deepEqual(preferences.search.recentQueries, []);
});

test("server policy clamps delivery and quality requests", () => {
  const preferences = normalizeProfileDeviceClassPreferences({
    playback: {
      deliveryRequest: { directPlay: "prefer", directStream: "never", transcode: "allow" },
      quality: { cellular: { mode: "high", maxVideoBitrateMbps: 20, maxVideoHeight: 2160, allowHDR: true } }
    }
  }, "mobile");
  const policy = {
    networkAllowed: { local: true, wifi: true, cellular: true, unknown: true },
    deliveryAllowed: { directPlay: false, directStream: true, transcode: true },
    maximumVideoBitrateMbps: 8,
    maximumVideoHeight: 1080,
    allowHDR: false
  };
  assert.deepEqual(resolveDeliveryPreference(preferences, policy), { directPlay: "never", directStream: "never", transcode: "allow" });
  assert.deepEqual(resolveQualityRequest(preferences, "cellular", policy), {
    mode: "high", allowed: true, maxVideoBitrateMbps: 8, maxVideoHeight: 1080, maxAudioBitrateKbps: undefined, allowHDR: false
  });
  const blocked = resolveQualityRequest(preferences, "cellular", { ...policy, networkAllowed: { ...policy.networkAllowed, cellular: false } });
  assert.equal(blocked.allowed, false);
});

test("V1 preference vocabulary rejects removed pre-release values", () => {
  assert.throws(() => normalizeProfileServerPreferences({ playback: { upNextCountdownSeconds: 20 } }), /choice/);
  assert.throws(() => normalizeProfileServerPreferences({ playback: { introSkip: "never" } }), /choice/);
  assert.throws(() => normalizeProfileDeviceClassPreferences({ playback: { quality: { wifi: { mode: "capped" } } } }, "web"), /choice/);
  assert.throws(() => normalizeProfileDeviceClassPreferences({ playback: { deliveryRequest: "prefer-direct-stream" } }, "web"), /object/);
  assert.throws(() => normalizeProfileDeviceClassPreferences({ playback: { deliveryRequest: { directPlay: "prefer", directStream: "prefer", transcode: "allow" } } }, "web"), /only one/);
  assert.throws(() => normalizeProfileDeviceClassPreferences({ playback: { deliveryRequest: { directPlay: "allow", directStream: "allow", transcode: "require" } } }, "web"), /disable direct/);
});

test("playback intent consumes the structured preference schema without a remote bucket or transcode downgrade", () => {
  const preferences = {
    playback: {
      deliveryRequest: { directPlay: "allow", directStream: "never", transcode: "prefer" },
      quality: {
        local: { mode: "original", allowHDR: true },
        wifi: { mode: "high", maxVideoBitrateMbps: 20, allowHDR: true },
        cellular: { mode: "data-saver", maxVideoBitrateMbps: 4, maxAudioBitrateKbps: 128, maxVideoHeight: 720, allowHDR: false },
        unknown: { mode: "standard", maxVideoBitrateMbps: 8, allowHDR: false }
      }
    }
  };
  const policy = {
    networkAllowed: { local: true, wifi: true, cellular: true, unknown: true },
    deliveryAllowed: { directPlay: true, directStream: true, transcode: true },
    maximumVideoBitrateMbps: 3,
    maximumAudioBitrateKbps: 96,
    maximumVideoHeight: 720,
    allowHDR: false
  };
  const portablePreferences = {
    playback: {
      preferredAudioLanguages: ["original", "fr-CA"],
      preferredSubtitleLanguages: ["en-CA"],
      subtitlesEnabled: true
    }
  };
  assert.deepEqual(playbackIntentFromPreferences(preferences, "cellular", policy, portablePreferences), {
    networkClass: "cellular",
    transportClass: "cellular",
    qualityProfile: "data_saver",
    directPlayPolicy: "allow",
    directStreamPolicy: "never",
    transcodePolicy: "prefer",
    maxVideoBitrateMbps: 3,
    maxAudioBitrateKbps: 96,
    maxVideoHeight: 720,
    allowHdr: false,
    preferredAudioLanguage: "fr-CA",
    preferredSubtitleLanguage: "en-CA",
    preferredSubtitleMode: "text"
  });
  assert.throws(() => playbackIntentFromPreferences({ ...preferences, playback: { ...preferences.playback, quality: { ...preferences.playback.quality, unknown: { mode: "off" } } } }, "unknown", policy), /disabled/);
});

test("home customization defaults deny and cannot move fixed server rows", () => {
  const preferences = normalizeProfileServerPreferences({ home: { hiddenRowIds: ["critical", "optional"], rowOrder: ["later"] } });
  const rows = applyHomeRowPreferences([
    { id: "critical", priority: 100 },
    { id: "optional", hideable: true, reorderable: true, priority: 90 },
    { id: "later", hideable: true, reorderable: true, priority: 10 }
  ], preferences);
  assert.deepEqual(rows.map(row => row.id), ["later", "critical"]);

  const serverOrder = applyHomeRowPreferences([
    { id: "first", hideable: true, reorderable: true, priority: 10 },
    { id: "second", hideable: true, reorderable: true, priority: 20 }
  ], normalizeProfileServerPreferences({}));
  assert.deepEqual(serverOrder.map(row => row.id), ["first", "second"]);
});

test("normalizers bound strings and reject unknown nested fields", () => {
  assert.throws(() => normalizeProfileServerPreferences({ playback: { futureOption: true } }), /unknown field/);
  assert.throws(() => normalizeProfileServerPreferences({ search: { recentQueries: ["x".repeat(300), "safe"] } }), /string list/);
  assert.throws(() => normalizeProfileServerPreferences({ privacy: { showActivityToMembers: "false" } }), /boolean/);
  assert.throws(() => normalizeProfileServerPreferences({ search: "not-an-object" }), /object/);
});

test("privacy recording, capabilities, motion, and exported defaults fail safe", () => {
  assert.deepEqual(recordRecentPreferenceQuery(["private"], "another", false), []);
  assert.deepEqual(recordRecentPreferenceQuery([], "Fargo", true), ["Fargo"]);
  assert.equal(preferenceCapabilitiesForDeviceClass("television").downloads, false);
  assert.equal(preferenceCapabilitiesForDeviceClass("mobile").cellularQuality, true);
  assert.equal(resolveMotionPreference("full", true), "reduced");
  assert.equal(resolveMotionPreference("full", false), "full");
});
