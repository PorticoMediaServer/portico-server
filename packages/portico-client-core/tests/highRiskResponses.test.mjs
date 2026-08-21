import assert from "node:assert/strict";
import test from "node:test";

import { decodeHighRiskResponse } from "../dist/internal/highRiskResponses.js";

test("high-risk credential responses reject missing secrets and invalid dates", () => {
  assert.throws(() => decodeHighRiskResponse("/api/auth/sessions", "POST", {
    tokenType: "Bearer", authority: "local", accessToken: "access", refreshToken: "",
    accessExpiresAt: "not-a-date", refreshExpiresAt: "2026-08-07T00:00:00Z",
    accountId: "account", serverId: "server", profileId: "profile", authorizationRevision: "1",
    user: {}, device: {}
  }), /credential refreshToken is invalid|credential access expiry is invalid/);
  assert.throws(() => decodeHighRiskResponse("/api/auth/me", "GET", {
    authenticated: true,
    user: {id: "viewer"},
    access_token: "do-not-expose",
    sessionToken: "do-not-expose"
  }), error => error instanceof TypeError && !error.message.includes("do-not-expose"));
});

test("high-risk route responses reject credential-shaped fields without echoing them", () => {
  assert.throws(() => decodeHighRiskResponse("/api/account/servers/server/routes", "GET", {
    documentVersion: 1,
    serverId: "server",
    serverName: "Home",
    serverPublicKey: "key",
    serverPublicKeyFingerprint: "sha256:fingerprint",
    signature: "signature",
    signatureAlgorithm: "ed25519",
    audience: "portico-media-server",
    issuedAt: "2026-08-07T00:00:00Z",
    expiresAt: "2026-08-07T00:05:00Z",
    routes: [{type: "public_direct", url: "https://home.example", quality: "healthy", serverToken: "do-not-expose"}]
  }), error => error instanceof TypeError && !error.message.includes("do-not-expose"));
});

test("high-risk grants and playback state reject invalid runtime fields", () => {
  assert.throws(() => decodeHighRiskResponse("/api/media/media/download-grants", "POST", {
    downloadUrl: "/api/download", expiresAt: "invalid", grantToken: "grant", profile: "source"
  }), /download grant expiry is invalid/);
  assert.deepEqual(decodeHighRiskResponse("/api/media/media/download-grants", "POST", {
    downloadUrl: "/api/media/media/download?profile=source", expiresAt: "2099-08-07T00:00:00Z", profile: "source"
  }), {
    downloadUrl: "/api/media/media/download?profile=source", expiresAt: "2099-08-07T00:00:00Z", profile: "source"
  });
  assert.throws(() => decodeHighRiskResponse("/api/download-preparations/preparation/grant", "POST", {
    downloadUrl: "/api/media/media/download?profile=source", expiresAt: "2099-08-07T00:00:00Z", profile: "source"
  }), /download grant token is invalid/);
  assert.throws(() => decodeHighRiskResponse("/api/playback-sessions/session/continuation", "GET", {
    generation: 1, highestEventSequence: 2, playbackRevision: 1, queueRevision: 1,
    positionSeconds: Number.NaN, sessionId: "session", state: "playing"
  }), /playback continuation position is invalid/);
});

test("pagination envelopes enforce bounded arrays and cursors", () => {
  assert.throws(() => decodeHighRiskResponse("/api/watchlist", "GET", {
    items: [], pageInfo: { nextCursor: "x".repeat(4_097) }
  }), /pagination cursor is invalid/);
  assert.throws(() => decodeHighRiskResponse("/api/watchlist", "GET", {
    items: new Array(10_001), pageInfo: { nextCursor: null }
  }), /pagination items are invalid/);
});
