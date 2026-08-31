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

test("Hosted account credentials use the generated Hosted contract without local server authority fields", () => {
  const credentials = JSON.parse('{"authority":"hosted","tokenType":"Bearer","accessToken":"account-access","accessExpiresAt":"2026-08-07T00:05:00Z","refreshToken":"account-refresh","refreshExpiresAt":"2026-09-07T00:00:00Z","user":{"id":"usr_1"},"device":{"id":"dev_1"}}');
  assert.equal(decodeHighRiskResponse("/api/auth/sessions", "POST", credentials, "hosted"), credentials);
  assert.throws(
    () => decodeHighRiskResponse("/api/auth/sessions", "POST", {...credentials, authority: undefined}, "hosted"),
    /credential authority is invalid/
  );
  assert.throws(
    () => decodeHighRiskResponse("/api/auth/sessions", "POST", credentials),
    /credential accountId is invalid/
  );

  const serverCredentials = {
    ...credentials,
    authority: "local",
    accountId: "acct_1",
    serverId: "srv_1",
    profileId: "profile_1",
    authorizationRevision: "1",
  };
  assert.equal(decodeHighRiskResponse("/api/auth/sessions", "POST", serverCredentials, "server"), serverCredentials);
  for (const field of ["accountId", "serverId", "profileId", "authorizationRevision"]) {
    const missing = {...serverCredentials};
    delete missing[field];
    assert.throws(
      () => decodeHighRiskResponse("/api/auth/sessions", "POST", missing, "server"),
      new RegExp(`credential ${field} is invalid`)
    );
  }
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
	for (const legacyType of ["direct", "direct_ip_encoded"]) {
		assert.throws(() => decodeHighRiskResponse("/api/account/servers/server/routes", "GET", {
			kind: "route-document", documentVersion: 1, serverId: "server", serverName: "Home",
			serverPublicKey: "key", serverPublicKeyFingerprint: "sha256:fingerprint", signature: "signature",
			signatureAlgorithm: "ed25519", audience: "portico-media-server",
			issuedAt: "2026-08-07T00:00:00Z", expiresAt: "2026-08-07T00:05:00Z",
			routes: [{type: legacyType, url: "https://home.example", quality: "reachable"}]
		}), /route type is invalid/i);
	}
});

test("high-risk grants and playback state reject invalid runtime fields", () => {
  assert.throws(() => decodeHighRiskResponse("/api/download-preparations/preparation/grant", "POST", {
    downloadUrl: "/api/media/media/download?profile=source", expiresAt: "invalid", profile: "source"
  }), /download grant expiry is invalid/);
  assert.throws(() => decodeHighRiskResponse("/api/playback-sessions/session/continuation", "GET", {
    generation: 1, highestEventSequence: 2, playbackRevision: 1, queueRevision: 1,
    positionSeconds: Number.NaN, sessionId: "session", state: "playing"
  }), /playback continuation position is invalid/);
});

test("playback terminal receipts are exact, positive acknowledgements", () => {
  const receipt = {
    requestId: "terminal-request-1",
    accepted: true,
    duplicate: true,
    sessionId: "session-1",
    terminal: {
      disposition: "completed",
      generation: 2,
      eventSequence: 9,
      recordedAt: "2026-08-30T18:20:00.000Z",
      positionSeconds: 60,
      durationSeconds: 60,
    },
  };
  assert.equal(
    decodeHighRiskResponse("/api/playback-sessions/session-1", "DELETE", receipt),
    receipt,
  );
  assert.throws(
    () => decodeHighRiskResponse(
      "/api/playback-sessions/session-1/continuation",
      "DELETE",
      { ...receipt, accepted: false },
    ),
    /receipt state/,
  );
  assert.throws(
    () => decodeHighRiskResponse(
      "/api/playback-sessions/session-1",
      "DELETE",
      { ...receipt, terminal: { ...receipt.terminal, eventSequence: 0 } },
    ),
    /ordering authority/,
  );
});

test("high-risk playback responses require one credential-free default resource", () => {
  const response = {
    sessionId: "session", sourceUrl: "/api/media/movie/hls/master.m3u8", directPlay: false,
    generation: 1, nextEventSequence: 1, playbackRevision: 0, queueRevision: 0,
    decision: {}, media: {},
    mediaGrant: {token: "grant", expiresAt: "2099-08-07T00:00:00Z"},
    continuationCredential: {token: "continuation", origin: "https://server.example", expiresAt: "2099-08-07T00:00:00Z", generation: 1},
    qualityOffers: {contractId: "PC-PLAYBACK", schemaVersion: "quality-offers.v1", mediaId: "movie", versionId: "qver-movie", sourceRevision: "qsrc-movie", offerRevision: "qrev-movie", offers: [{selectionId: "qsel-auto", label: "Automatic", kind: "automatic"}, {selectionId: "qsel-original", label: "Original Quality", kind: "original"}]},
    qualitySelection: {mode: "automatic"}, selectedSubtitleMode: "off",
    resources: [{id: "active", sourceUrl: "/api/media/movie/hls/master.m3u8", streamFormat: "hls", subtitleMode: "off", default: true}],
    audioStreams: [], subtitleStreams: [], chapters: [], queue: []
  };
  assert.equal(decodeHighRiskResponse("/api/playback-sessions", "POST", response), response);
  assert.throws(() => decodeHighRiskResponse("/api/playback-sessions", "POST", {
    ...response,
    sourceUrl: "/api/media/movie/hls/master.m3u8?sessionCredential=secret",
    resources: [{...response.resources[0], sourceUrl: "/api/media/movie/hls/master.m3u8?sessionCredential=secret"}]
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
    qualitySelection: {mode: "explicit", selectionId: "qsel-original", qualityOfferRevision: "qrev-stale"}
  }), /explicit quality selection is stale/);
  const audioOnlyFixed = {...response, qualityOffers: {...response.qualityOffers, offers: [...response.qualityOffers.offers, {selectionId: "qsel-audio", label: "Audio 128 kbps", kind: "fixed", maxAudioBitrateBps: 128000}]}};
  assert.equal(decodeHighRiskResponse("/api/playback-sessions", "POST", audioOnlyFixed), audioOnlyFixed);
  assert.throws(() => decodeHighRiskResponse("/api/playback-sessions", "POST", {
    ...response,
    qualityOffers: {...response.qualityOffers, offers: [...response.qualityOffers.offers, {selectionId: "qsel-empty", label: "Empty", kind: "fixed"}]}
  }), /quality offer target is invalid/);
  assert.throws(() => decodeHighRiskResponse("/api/playback-sessions", "POST", {
    ...response,
    qualityOffers: {...response.qualityOffers, offers: [{...response.qualityOffers.offers[0], maxAudioBitrateBps: 128000}, response.qualityOffers.offers[1]]}
  }), /quality offer target is invalid/);

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
    playback: {...response, continuationCredential: null}
  };
  assert.equal(decodeHighRiskResponse("/api/playback-sessions/session/prepare-next", "POST", prepared), prepared);
  const credentialOmitted = {
    ...prepared,
    playback: Object.fromEntries(Object.entries(prepared.playback).filter(([key]) => key !== "continuationCredential"))
  };
  assert.equal(decodeHighRiskResponse("/api/playback-sessions/session/prepare-next", "POST", credentialOmitted), credentialOmitted);
  assert.throws(() => decodeHighRiskResponse("/api/playback-sessions/session/prepare-next", "POST", {
    ...prepared,
    playback: response
  }), /contains a continuation credential/);
  assert.throws(() => decodeHighRiskResponse("/api/playback-sessions/session/prepare-next", "POST", {
    ...prepared,
    playback: {...response, continuationCredential: null, sourceUrl: "/api/media/movie/not-the-default"}
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
