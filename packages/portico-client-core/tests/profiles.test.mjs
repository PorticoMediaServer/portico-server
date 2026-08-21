import assert from "node:assert/strict";
import test from "node:test";

import {
  automaticProfileTrustFromGrant,
  decideProfileSelection,
  intersectProfilePolicy,
  parseProfileDirectory,
  parseProfilePolicy,
  profilePolicyAllowsFeature,
  profilePolicyAllowsRating,
  unrestrictedProfilePolicy
} from "../dist/profiles.js";

function policy(overrides = {}) {
  return { ...unrestrictedProfilePolicy, blockedLabels: [], ...overrides };
}

test("profile policy uses the exact flattened portable V1 wire contract", () => {
  const parsed = parseProfilePolicy(policy({ allowDvr: false }));
  assert.equal(parsed.version, "v1");
  assert.equal(parsed.allowDvr, false);
  assert.equal(profilePolicyAllowsFeature(parsed, "dvr"), false);
  assert.equal("features" in parsed, false);
  assert.throws(() => parseProfilePolicy({ ...policy(), features: {} }), /missing or unknown/);
});

test("profile policy rejects missing, unknown, malformed, duplicate, and overlong values", () => {
  assert.throws(() => parseProfilePolicy({}), /missing|unsupported/);
  assert.throws(() => parseProfilePolicy({ ...policy(), futureField: true }), /missing or unknown/);
  assert.throws(() => parseProfilePolicy(policy({ maximumAgeRating: undefined })), /0 through 21/);
  assert.throws(() => parseProfilePolicy(policy({ maximumAgeRating: 22 })), /0 through 21/);
  assert.throws(() => parseProfilePolicy(policy({ blockedLabels: ["Violence", " violence "] })), /blocked labels/);
  assert.throws(() => parseProfilePolicy(policy({ blockedLabels: ["Café", "Cafe\u0301"] })), /blocked labels/);
  assert.doesNotThrow(() => parseProfilePolicy(policy({ blockedLabels: ["😀".repeat(128)] })));
  assert.throws(() => parseProfilePolicy(policy({ blockedLabels: ["😀".repeat(129)] })), /blocked labels/);
});

test("profile policy intersection can only reduce membership access", () => {
  const effective = intersectProfilePolicy(
    policy({ maximumAgeRating: 16, allowUnrated: false, blockedLabels: ["Violence"], allowDvr: false }),
    policy({ maximumAgeRating: 13, allowUnrated: true, blockedLabels: ["Explicit"], allowDvr: true, allowDownloads: false })
  );
  assert.equal(effective.maximumAgeRating, 13);
  assert.equal(effective.allowUnrated, false);
  assert.deepEqual(effective.blockedLabels, ["Explicit", "Violence"]);
  assert.equal(effective.allowDvr, false);
  assert.equal(effective.allowDownloads, false);
});

test("rating and label checks reject malformed source ratings and labels closed", () => {
  const restricted = policy({ maximumAgeRating: 13, allowUnrated: false, blockedLabels: ["Explicit"] });
  assert.equal(profilePolicyAllowsRating(restricted, 12), true);
  assert.equal(profilePolicyAllowsRating(restricted, 16), false);
  assert.equal(profilePolicyAllowsRating(restricted, null), false);
  assert.equal(profilePolicyAllowsRating(restricted, 7, ["explicit"]), false);
  assert.equal(profilePolicyAllowsRating(restricted, Number.NaN), false);
  assert.equal(profilePolicyAllowsRating(restricted, -1), false);
  assert.equal(profilePolicyAllowsRating(restricted, 7, [""]), false);
});

function profile(id, overrides = {}) {
  return {
    id,
    name: id,
    isPrimary: id === "primary",
    isAccountAdmin: id === "primary",
    hasPIN: false,
    pinRevision: 0,
    sortOrder: id === "primary" ? 0 : 1,
    policy: policy(),
    ...overrides
  };
}

test("profile directories enforce identity, primary, and account-admin invariants", () => {
  const valid = { authority: "hosted", accountId: "a", serverId: "server-a", profilesAllowed: true, profiles: [profile("primary"), profile("kids")] };
  assert.equal(parseProfileDirectory(valid).profiles.length, 2);
  assert.throws(() => parseProfileDirectory({ ...valid, profiles: [profile("kids")] }), /identities/);
  assert.throws(() => parseProfileDirectory({ ...valid, profiles: [profile("primary"), profile("primary")] }), /identities/);
  assert.throws(() => parseProfileDirectory({ ...valid, profiles: [profile("primary", { isAccountAdmin: false })] }), /primary/);
});

test("multi-profile automatic selection requires distinct current authority-issued installation trust", () => {
  const now = new Date("2026-07-16T12:00:00Z");
  const directory = {
    authority: "hosted", accountId: "a", serverId: "server-a", profilesAllowed: true,
    profiles: [profile("primary", { hasPIN: true, pinRevision: 2 }), profile("kids")]
  };
  const rawGrant = {
    version: "v1", purpose: "automatic-profile-selection",
    token: "opaque-token", authority: "hosted", accountId: "a", serverId: "server-a", profileId: "primary", pinRevision: 2,
    installationId: "install-a", expiresAt: "2026-07-16T12:02:00Z"
  };
  const trusted = automaticProfileTrustFromGrant(rawGrant, { authority: "hosted", accountId: "a", serverId: "server-a", installationId: "install-a", now });
  assert.ok(trusted);
  assert.equal(decideProfileSelection(directory, { profileSelection: "last-used", lastProfileId: "primary" }, trusted, { serverId: "server-a", installationId: "install-a", now }).kind, "open");
  assert.equal(decideProfileSelection(directory, { profileSelection: "last-used", lastProfileId: "primary" }, trusted, { serverId: "server-a", installationId: "install-b", now }).kind, "select");
  assert.equal(decideProfileSelection(directory, { profileSelection: "last-used", lastProfileId: "primary" }, trusted, { serverId: "server-a", installationId: "install-a", now: new Date("2026-07-16T12:03:00Z") }).kind, "select");
  assert.equal(decideProfileSelection(directory, { profileSelection: "last-used", lastProfileId: "primary" }, { ...trusted, token: null }, { serverId: "server-a", installationId: "install-a", now }).kind, "select");
  assert.equal(automaticProfileTrustFromGrant(rawGrant, { authority: "local", accountId: "a", serverId: "server-a", installationId: "install-a", now }), undefined);
  assert.equal(automaticProfileTrustFromGrant(rawGrant, { authority: "hosted", accountId: "other", serverId: "server-a", installationId: "install-a", now }), undefined);
  assert.equal(automaticProfileTrustFromGrant(rawGrant, { authority: "hosted", accountId: "a", serverId: "server-b", installationId: "install-a", now }), undefined);
  assert.equal(decideProfileSelection({ ...directory, serverId: "server-b" }, { profileSelection: "last-used", lastProfileId: "primary" }, trusted, { serverId: "server-b", installationId: "install-a", now }).kind, "select");
  assert.throws(() => automaticProfileTrustFromGrant({ ...rawGrant, purpose: "profile-selection" }, { authority: "hosted", accountId: "a", serverId: "server-a", installationId: "install-a", now }), /purpose/);
});

test("profile selection remains consistent across one-profile, ask, and last-used modes", () => {
  const one = { authority: "hosted", accountId: "a", serverId: "server-a", profilesAllowed: true, profiles: [profile("primary")] };
  assert.equal(decideProfileSelection(one, { profileSelection: "ask" }).kind, "open");
  assert.equal(decideProfileSelection({ ...one, profiles: [profile("primary", { hasPIN: true })] }, { profileSelection: "ask" }).kind, "pin");
  const household = { ...one, profiles: [profile("primary"), profile("kids")] };
  assert.equal(decideProfileSelection(household, { profileSelection: "ask" }).kind, "select");
  const lastUsed = decideProfileSelection(household, { profileSelection: "last-used", lastProfileId: "kids" });
  assert.equal(lastUsed.kind, "open");
  assert.equal(lastUsed.profile.id, "kids");
  const denied = { ...household, profilesAllowed: false };
  assert.equal(decideProfileSelection(denied, { profileSelection: "ask" }).profile.id, "primary");
});

test("canonical unrestricted policy cannot be mutated process-wide", () => {
  assert.throws(() => { unrestrictedProfilePolicy.blockedLabels.push("mutated"); }, TypeError);
  assert.throws(() => { unrestrictedProfilePolicy.allowDownloads = false; }, TypeError);
});
