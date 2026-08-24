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
    kind: "route-document",
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
  assert.throws(() => decodeHighRiskResponse("/api/account/servers/server/routes", "GET", {
    kind: "policy-snapshot",
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
    routes: []
  }), /kind/i);
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

test("high-risk playback responses require one credential-free default resource", () => {
  const response = {
    sessionId: "session", sourceUrl: "/api/media/movie/hls/master.m3u8", directPlay: false,
    generation: 1, nextEventSequence: 1, playbackRevision: 0, queueRevision: 0,
    decision: {}, media: {},
    mediaGrant: {token: "grant", expiresAt: "2099-08-07T00:00:00Z"},
    continuationCredential: {token: "continuation", origin: "https://server.example", expiresAt: "2099-08-07T00:00:00Z", generation: 1},
    selectedQualityId: "auto", selectedSubtitleMode: "off",
    resources: [{id: "active", sourceUrl: "/api/media/movie/hls/master.m3u8", streamFormat: "hls", qualityId: "auto", subtitleMode: "off", default: true}],
    audioStreams: [], subtitleStreams: [], chapters: [], qualities: [], queue: []
  };
  assert.equal(decodeHighRiskResponse("/api/playback-sessions", "POST", response), response);
  assert.throws(() => decodeHighRiskResponse("/api/playback-sessions", "POST", {
    ...response,
    sourceUrl: "/api/media/movie/hls/master.m3u8?media_grant=secret",
    resources: [{...response.resources[0], sourceUrl: "/api/media/movie/hls/master.m3u8?media_grant=secret"}]
  }), /playback source URL is invalid/);
  assert.throws(() => decodeHighRiskResponse("/api/playback-sessions", "POST", {
    ...response,
    sourceUrl: "javascript:alert(document.domain)",
    resources: [{...response.resources[0], sourceUrl: "javascript:alert(document.domain)"}]
  }), /playback source URL is invalid/);
  assert.throws(() => decodeHighRiskResponse("/api/playback-sessions", "POST", {
    ...response,
    resources: [{...response.resources[0], default: false}]
  }), /playback default resource is invalid/);
  assert.throws(() => decodeHighRiskResponse("/api/playback-sessions", "POST", {
    ...response,
    resources: [response.resources[0], {...response.resources[0], id: "inactive", default: false}]
  }), /playback resources are invalid/);
  assert.throws(() => decodeHighRiskResponse("/api/playback-sessions", "POST", {
    ...response,
    selectedQualityId: "original"
  }), /selectedQualityId does not match/);

  assert.equal(decodeHighRiskResponse("/api/playback/active", "POST", {
    active: true,
    playback: response
  }).playback, response);
  assert.throws(() => decodeHighRiskResponse("/api/playback/active", "POST", {
    active: true,
    playback: {...response, resources: []}
  }), /resources/);
  assert.throws(() => decodeHighRiskResponse("/api/playback/active", "POST", {
    active: false,
    playback: response
  }), /inactive/);

  const prepared = {
    preparedSessionId: "prepared-1",
    handoffMode: "gapless",
    preloadPolicy: "metadata",
    expiresAt: "2099-08-07T00:00:00Z",
    playbackRevision: 2,
    queueRevision: 4,
    queue: [],
    playback: response
  };
  assert.equal(decodeHighRiskResponse("/api/playback-sessions/session/prepare-next", "POST", prepared), prepared);
  assert.throws(() => decodeHighRiskResponse("/api/playback-sessions/session/prepare-next", "POST", {
    ...prepared,
    playback: {...response, sourceUrl: "/api/media/movie/not-the-default"}
  }), /default resource/);

  for (const path of [
    "/api/live-tv/play",
    "/api/live-tv/streams/channel/open",
    "/api/dvr/recordings/recording/playback",
    "/api/library-channels/channel/tune"
  ]) {
    assert.throws(() => decodeHighRiskResponse(path, "POST", {ok: true}), /playback/);
  }
});

test("pagination envelopes enforce bounded arrays and cursors", () => {
  assert.throws(() => decodeHighRiskResponse("/api/watchlist", "GET", {
    items: [], pageInfo: { nextCursor: "x".repeat(4_097) }
  }), /pagination cursor is invalid/);
  assert.throws(() => decodeHighRiskResponse("/api/watchlist", "GET", {
    items: new Array(10_001), pageInfo: { nextCursor: null }
  }), /pagination items are invalid/);
});
