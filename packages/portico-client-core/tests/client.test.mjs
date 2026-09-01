import assert from "node:assert/strict";
import { generateKeyPairSync, sign, verify } from "node:crypto";
import { readFileSync } from "node:fs";
import test from "node:test";
import {
  ApiError,
  PorticoTransportError,
  browserPlaybackClientProfile,
  CredentialCleanupUncertainError,
  connectHostedServer,
  discoverHostedServerRoute,
  createHostedConnectionRuntime,
  createHostedServicesClient,
  createMemorySessionStore as createCoreMemorySessionStore,
  createPorticoClient,
  dataTagsForMutation,
  groupLibraryCategories,
  hostedDirectRouteAllowed,
  HostedRouteDiscoveryTimeoutError,
  HostedRoutePublicationPendingError,
  HostedRouteRetryLaterError,
  HostedTerminalMutationCommittedError,
  HostedTerminalMutationUncertainError,
  LocalNetworkRouteUnavailableError,
  NearbyRouteAvailableError,
  playbackQualitySelectionFor,
  playbackQualitySelectionKey,
  preferredRoute,
  routesForConnection,
  routeIsUsableCandidate,
  trustedHostedDocumentKeysFromKeySet,
  verifyHostedRouteDocument,
  libraryTabs,
  normalizePlaybackResponse,
  parseRetryAfter,
  isAmbiguousPorticoError,
  isRetryablePorticoError,
  playbackSourceFor,
  playbackResourceUrl,
  positiveFullJitterDelay,
  typedFilterForTab
} from "../dist/index.js";
import {
  createAttachmentMethods,
  createAttachmentRuntime,
  testServerIdentity,
  testServerPublicKey,
  testServerPublicKeyFingerprint
} from "./helpers/porticoAttachment.mjs";

const createMemorySessionStore = initial => createCoreMemorySessionStore(
  initial ? {...testServerIdentity(), ...initial} : initial
);

const hostedSystemFixture = Object.freeze(JSON.parse(readFileSync(new URL("../fixtures/hosted-api-v1-conformance.json", import.meta.url), "utf8")).system);
const goSerializedRouteFixture = Object.freeze(JSON.parse(readFileSync(new URL("../fixtures/signing/document-signing-fixture.json", import.meta.url), "utf8")).routeDocument);

function jsonResponse(body, init = {}) {
  if (body?.tokenType === "Bearer" && body.user && body.device && body.accessToken && body.refreshToken) {
    body = {...testServerIdentity(), ...body};
  }
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
    ...init
  });
}

function acceptedPlayback(sessionId, nextEventSequence = 1) {
  return {
    sessionId,
    media: { id: "m1", title: "Movie", type: "movie", state: {} },
    sourceUrl: "/api/media/m1/stream",
    directPlay: true,
    generation: 1,
    nextEventSequence,
    playbackRevision: 1,
    queueRevision: 1,
    repeatMode: "off",
    timeline: { type: "vod", durationSeconds: 60, canPause: true, canSeek: true },
    continuationCredential: { token: "continuation", origin: "https://server.example", generation: 1, expiresAt: "2026-08-06T01:00:00Z" },
    mediaGrant: { token: "grant", expiresAt: "2026-08-06T01:00:00Z" },
    decision: { mode: "direct_play", reason: "", requiresTranscode: false, isProxied: true, isServerCached: false },
    qualityOffers: {contractId: "PC-PLAYBACK", schemaVersion: "quality-offers.v1", mediaId: "m1", versionId: "qver-m1", sourceRevision: "qsrc-m1", offerRevision: "qrev-m1", offers: [{selectionId: "qsel-auto", label: "Automatic", kind: "automatic"}, {selectionId: "qsel-original", label: "Original Quality", kind: "original"}]},
    qualitySelection: {mode: "automatic"}, audioStreams: [], subtitleStreams: [], chapters: [], queue: [],
    resources: [{ id: "movie-active", sourceUrl: "/api/media/m1/stream", streamFormat: "http", default: true }],
  };
}

function playbackTerminalReceipt(sessionId, request, duplicate = false) {
  return {
    requestId: request.requestId,
    accepted: true,
    duplicate,
    sessionId,
    terminal: request.terminal,
  };
}

function compatibleLocalClient(serverId, audience, overrides = {}) {
  const issued = {
    ...testServerIdentity(),
    tokenType: "Bearer",
    accessToken: "server-access",
    accessExpiresAt: "2026-05-23T01:00:00Z",
    refreshToken: "server-refresh",
    refreshExpiresAt: "2026-06-23T00:00:00Z",
    authority: "hosted",
    accountId: "account-1",
    serverId,
    profileId: "profile-1",
    authorizationRevision: "policy-1",
    user: { id: "usr" },
    device: { id: "device" }
  };
  return {
    checkServerCompatibility: async () => ({ apiVersion: "v1" }),
    checkCompatibility: async () => ({ apiVersion: "v1" }),
    ...createAttachmentMethods({serverId, audience, credentials: issued, now: "2026-05-23T00:01:00Z"}),
    me: async () => ({
      authenticated: true,
      authority: "hosted",
      accountId: "account-1",
      serverId,
      profileId: "profile-1",
      authorizationRevision: "policy-2",
      user: { id: "usr", username: "Owner", role: "owner" }
    }),
    checkProductContractCompatibility: async () => ({
      apiVersion: "v1",
      serverCapabilities: []
    }),
    ...overrides
  };
}

function hostedProfileBinding(serverId, installationId = "installation-test") {
  return {
    clientIdentity: {installationId, deviceName: "Test Device", app: "Portico Test", platform: "test"},
    selectionEnvelope: {accountId: "account-1", serverId, profileId: "profile-1", installationId}
  };
}

const hostedDocumentTestKeys = generateKeyPairSync("ed25519");
const hostedDocumentTestKeyId = "hosted-documents-test";
const hostedDocumentTestPublicKey = hostedDocumentTestKeys.publicKey.export({ format: "der", type: "spki" }).subarray(-32).toString("base64");
const clientAttachmentRuntime = createAttachmentRuntime("2026-05-23T00:01:00Z");

function signedRouteDocument(document) {
  const unsigned = {
    kind: "route-document",
    documentVersion: 1,
    endpointGeneration: 1,
    audience: "portico-media-server",
    signatureAlgorithm: "ed25519",
    signatureKeyId: hostedDocumentTestKeyId,
    ...document,
    ...testServerIdentity()
  };
  delete unsigned.signature;
  const canonical = `portico-signed-document:route-document:v1\n${JSON.stringify(sortJSONValue(unsigned))}`;
  return {
    ...unsigned,
    signature: sign(null, Buffer.from(canonical), hostedDocumentTestKeys.privateKey).toString("base64url")
  };
}

function sortJSONValue(value) {
  if (Array.isArray(value)) return value.map(sortJSONValue);
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(Object.entries(value).sort(([left], [right]) => left < right ? -1 : left > right ? 1 : 0).map(([key, nested]) => [key, sortJSONValue(nested)]));
}

function setGlobal(name, value) {
  const descriptor = Object.getOwnPropertyDescriptor(globalThis, name);
  Object.defineProperty(globalThis, name, {
    configurable: true,
    writable: true,
    value
  });
  return () => {
    if (descriptor) Object.defineProperty(globalThis, name, descriptor);
    else delete globalThis[name];
  };
}

test("local client sends bearer auth, JSON body, CSRF, and mutation tags", async () => {
  const calls = [];
  const sessionStore = createMemorySessionStore({
    apiBaseUrl: "https://server.example",
    bootstrapAccessToken: "token-123"
  });
  const tags = [];
  const client = createPorticoClient({
    sessionStore,
    csrfToken: "csrf-local",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return jsonResponse({ id: "m1", title: "Movie", type: "movie", state: {} });
      }
    },
    onMutation(nextTags) {
      tags.push(nextTags);
    }
  });

  await client.setWatchlist("m1", true);
  assert.equal(calls[0].input, "https://server.example/api/media/m1/watchlist");
  assert.equal(calls[0].init.method, "POST");
  assert.equal(calls[0].init.headers.Authorization, "Bearer token-123");
  assert.equal(calls[0].init.headers["X-Portico-CSRF"], "csrf-local");
  assert.equal(calls[0].init.body, JSON.stringify({ watchlisted: true }));
  assert.deepEqual(tags[0], ["media-state", "library-items"]);
});

test("Server API-key clients validate strictly and never require refreshable session state", async () => {
  const apiKey = `ptc_api_${"A".repeat(43)}`;
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    apiKey,
    transport: { fetch: async (input, init) => {
      calls.push({input: String(input), init});
      return jsonResponse({items: [], total: 0});
    }}
  });
  await client.libraries();
  assert.equal(calls[0].init.headers.Authorization, `Bearer ${apiKey}`);
  assert.throws(() => createPorticoClient({apiKey: "not-a-key"}), /valid ptc_api_/);
  assert.throws(() => createPorticoClient({apiKey, sessionStore: createMemorySessionStore()}), /cannot be combined/);

  let providedKey = apiKey;
  const dynamic = createPorticoClient({
    apiBaseUrl: "https://server.example",
    apiKey: () => providedKey,
    transport: { fetch: async () => jsonResponse({items: [], total: 0}) }
  });
  providedKey = "ptc_api_invalid";
  await assert.rejects(() => dynamic.libraries(), /valid ptc_api_/);
});

test("supervised restore client methods send step-up, confirmation, bounded upload, and status capability", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    csrfToken: "csrf-local",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        if (String(input).endsWith("/api/backups/restore-authorization-context")) {
          return jsonResponse({ restoreSecurityEpoch: 7 });
        }
        return jsonResponse({
          ok: true,
          name: "portico-backup.db",
          operationId: "restore-1",
          state: "staged",
          recoveryRequired: false,
          instruction: "staged"
        });
      }
    }
  });

  await client.restoreBackup("portico-backup.db", { password: "account-password", confirmation: "restore:portico-backup.db" });
  assert.equal(calls[0].init.method, "POST");
  assert.deepEqual(JSON.parse(calls[0].init.body), { password: "account-password", confirmation: "restore:portico-backup.db" });

  const file = new File(["sqlite-bytes"], "manual-import.db", { type: "application/octet-stream" });
  await client.restoreUploadedDatabase({
    file,
    password: "account-password",
    confirmation: "restore:uploaded-database",
    manifest: "{\"version\":2}"
  });
  const formEntries = [...calls[1].init.body.entries()].map(([key, value]) => [key, typeof value === "string" ? value : value.name]);
  assert.deepEqual(formEntries.map(([key]) => key), ["password", "confirmation", "databaseBytes", "manifest", "database"]);
  assert.equal(formEntries[0][1], "account-password");
  assert.equal(formEntries[1][1], "restore:uploaded-database");
  assert.equal(formEntries[2][1], String(file.size));
  assert.equal(formEntries[4][1], "manual-import.db");

  await client.restoreStatus("restore-1", "opaque-status-token");
  assert.equal(calls[2].init.headers["X-Portico-Restore-Status"], "opaque-status-token");

  const context = await client.restoreAuthorizationContext();
  assert.equal(context.restoreSecurityEpoch, 7);
  assert.equal(calls[3].input, "https://server.example/api/backups/restore-authorization-context");

  const hostedAuthorization = {
    kind: "restore-authorization", version: 1, audience: "portico-media-server", authorizationId: "sra_test",
    purpose: "server-restore", serverId: "server-1", accountId: "account-1", restoreSecurityEpoch: 7,
    issuedAt: "2026-08-30T13:00:00Z", expiresAt: "2026-08-30T13:05:00Z",
    signatureAlgorithm: "ed25519", signatureKeyId: "key-1", signature: "signed"
  };
  await client.restoreUploadedDatabase({ file, hostedAuthorization, confirmation: "restore:uploaded-database" });
  const hostedFormEntries = [...calls[4].init.body.entries()];
  assert.deepEqual(hostedFormEntries.map(([key]) => key), ["hostedAuthorization", "confirmation", "databaseBytes", "database"]);
  assert.deepEqual(JSON.parse(hostedFormEntries[0][1]), hostedAuthorization);
});

test("Watch With Friends host mutations preserve required idempotency keys", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return jsonResponse({
          id: "group 1", mediaId: "movie 1", name: "Movie Night", ownerProfileId: "profile-1",
          revision: 1, reconnectGeneration: 1, playbackRevision: 1, positionSeconds: 0, playbackRate: 1,
          state: "playing", serverTime: "2026-08-06T00:00:00Z", positionUpdatedAt: "2026-08-06T00:00:00Z",
          createdAt: "2026-08-06T00:00:00Z", updatedAt: "2026-08-06T00:00:00Z",
          members: [], queue: [], command: { id: "command-1", action: "play", issuedAt: "2026-08-06T00:00:00Z" }
        });
      }
    }
  });

  await client.updateWatchWithFriendsState("group 1", { action: "play", expectedRevision: 4, idempotencyKey: "state key" });
  await client.updateWatchWithFriendsSettings("group 1", { repeatMode: "all", expectedRevision: 5, idempotencyKey: "settings key" });
  await client.addWatchWithFriendsQueueItem("group 1", { mediaId: "movie 2", expectedRevision: 6, idempotencyKey: "add key" });
  await client.reorderWatchWithFriendsQueue("group 1", { entryId: "entry 2", destinationEntryId: "entry 1", placement: "before", expectedRevision: 7, idempotencyKey: "order key" });
  await client.removeWatchWithFriendsQueueItem("group 1", "entry 2", 8, "remove key");
  await client.endWatchWithFriendsGroup("group 1", 9, "end key");

  assert.deepEqual(calls.slice(0, 4).map((call) => JSON.parse(call.init.body)), [
    { action: "play", expectedRevision: 4, idempotencyKey: "state key" },
    { repeatMode: "all", expectedRevision: 5, idempotencyKey: "settings key" },
    { mediaId: "movie 2", expectedRevision: 6, idempotencyKey: "add key" },
    { entryId: "entry 2", destinationEntryId: "entry 1", placement: "before", expectedRevision: 7, idempotencyKey: "order key" }
  ]);
  assert.equal(calls[4].input, "https://server.example/api/watch-with-friends/groups/group%201/queue/entry%202?expectedRevision=8&idempotencyKey=remove%20key");
  assert.equal(calls[5].input, "https://server.example/api/watch-with-friends/groups/group%201?expectedRevision=9&idempotencyKey=end%20key");
  assert.deepEqual(calls.map((call) => call.init.method), ["PATCH", "PATCH", "POST", "PATCH", "DELETE", "DELETE"]);
});

test("playlist mutations do not invalidate home surfaces", () => {
  assert.deepEqual(dataTagsForMutation("/api/playlists"), ["playlists"]);
  assert.deepEqual(dataTagsForMutation("/api/playlists/plist_1/items/bulk"), ["playlists"]);
  assert.deepEqual(dataTagsForMutation("/api/collections"), ["collections"]);
  assert.deepEqual(dataTagsForMutation("/api/collections/collection_1/items/bulk"), ["collections"]);
});

test("library navigation uses the account-scoped cross-platform contract", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return jsonResponse({ pinnedLibraryIds: ["lib_music", "lib_movies"] });
      }
    }
  });

  assert.deepEqual(await client.libraryNavigation(), { pinnedLibraryIds: ["lib_music", "lib_movies"] });
  assert.deepEqual(await client.updateLibraryNavigation({ pinnedLibraryIds: ["lib_movies"] }), { pinnedLibraryIds: ["lib_music", "lib_movies"] });
  assert.equal(calls[0].input, "https://server.example/api/account/library-navigation");
  assert.equal(calls[1].init.method, "PATCH");
  assert.equal(calls[1].init.body, JSON.stringify({ pinnedLibraryIds: ["lib_movies"] }));
});

test("discovery release methods use canonical people, history, and hierarchy routes", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (input, init = {}) => {
        const url = String(input);
        calls.push({ input: url, init });
        if (url.includes("/search/history") && init.method === "DELETE") return new Response(null, { status: 204 });
        return jsonResponse({});
      }
    }
  });

  await client.person("person_RnJhbmNlcw", { limit: 25, cursor: "person-cursor" });
  await client.mediaChildren("show/one", { limit: 50, cursor: "child-cursor" });
  await client.searchHistory();
  await client.clearSearchHistory("  Fargo  ");

  assert.deepEqual(calls.map(call => call.input), [
    "https://server.example/api/people/person_RnJhbmNlcw?limit=25&cursor=person-cursor",
    "https://server.example/api/media/show%2Fone/children?limit=50&cursor=child-cursor",
    "https://server.example/api/search/history",
    "https://server.example/api/search/history?query=Fargo"
  ]);
  assert.equal(calls[3].init.method, "DELETE");
  assert.deepEqual(dataTagsForMutation("/api/search/history?query=Fargo"), ["search-history"]);
});

test("native auth and Quick Connect keep credentials out of URLs", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return jsonResponse({
          tokenType: "Bearer",
          accessToken: "access",
          accessExpiresAt: "2026-01-01T00:15:00Z",
          refreshToken: "refresh",
          refreshExpiresAt: "2026-01-31T00:00:00Z",
          authority: "local",
          accountId: "account-1",
          serverId: "server-1",
          profileId: "profile-1",
          authorizationRevision: "1",
          user: {},
          device: {},
          status: "pending"
        });
      }
    }
  });
  await client.createNativeSession({ login: "owner", password: "secret", installationId: "native-install-0001", deviceName: "Phone", app: "Portico", platform: "iOS" });
  await client.refreshNativeSession("refresh-secret");
  await client.revokeNativeSession("refresh-secret");
  await client.quickConnectStatus("pairing-secret");
  assert.deepEqual(calls.map((call) => call.input), [
    "https://server.example/api/auth/sessions",
    "https://server.example/api/auth/sessions/refresh",
    "https://server.example/api/auth/sessions/revoke",
    "https://server.example/api/auth/quick-connect/status"
  ]);
  assert.equal(calls.every((call) => !call.input.includes("refresh-secret") && !call.input.includes("pairing-secret")), true);
  assert.equal(JSON.parse(calls[3].init.body).secret, "pairing-secret");
});

test("concurrent 401 responses share one refresh, await durable rotation, and retry once", async () => {
  const sessionStore = createMemorySessionStore({
    apiBaseUrl: "https://server.example",
    accessToken: "access-old",
    refreshToken: "refresh-old",
    serverId: "server-1"
  });
  const calls = [];
  let refreshCalls = 0;
  let persisted = false;
  let savedSession;
  const client = createPorticoClient({
    sessionStore,
    credentialAdapter: {
      save: async (session) => {
        await new Promise((resolve) => setTimeout(resolve, 5));
        savedSession = session;
        persisted = true;
      },
      clear: async () => {}
    },
    transport: {
      fetch: async (input, init) => {
        const url = String(input);
        calls.push({ url, authorization: init.headers.Authorization });
        if (url.endsWith("/api/auth/sessions/refresh")) {
          refreshCalls++;
          await new Promise((resolve) => setTimeout(resolve, 5));
          return jsonResponse({
            tokenType: "Bearer",
            accessToken: "access-new",
            accessExpiresAt: "2026-07-11T01:00:00Z",
            refreshToken: "refresh-new",
            refreshExpiresAt: "2026-08-11T00:00:00Z",
            user: {},
            device: {}
          });
        }
        if (init.headers.Authorization === "Bearer access-old") {
          return jsonResponse({ code: "token_expired", detail: "Expired" }, { status: 401 });
        }
        assert.equal(persisted, true, "the rotated token must be durable before requests retry");
        return jsonResponse(url.endsWith("/api/system") ? { setupRequired: false } : { name: "Portico" });
      }
    }
  });

  await Promise.all([client.system(), client.branding()]);

  assert.equal(refreshCalls, 1);
  assert.equal(calls.filter((call) => !call.url.endsWith("/refresh")).length, 4);
  assert.equal(savedSession.accessToken, "access-new");
  assert.equal(savedSession.refreshToken, "refresh-new");
  assert.equal(savedSession.serverId, "server-1");
  assert.equal(sessionStore.get().accessToken, "access-new");
});

test("one caller aborting does not cancel a shared credential refresh", async () => {
  const sessionStore = createMemorySessionStore({
    apiBaseUrl: "https://server.example",
    accessToken: "access-old",
    refreshToken: "refresh-old"
  });
  const controller = new AbortController();
  let releaseRefresh;
  const refreshGate = new Promise((resolve) => { releaseRefresh = resolve; });
  let announceRefresh;
  const refreshStarted = new Promise((resolve) => { announceRefresh = resolve; });
  let refreshCalls = 0;
  const client = createPorticoClient({
    sessionStore,
    transport: {
      fetch: async (input, init) => {
        const url = String(input);
        if (url.endsWith("/api/auth/sessions/refresh")) {
          refreshCalls++;
          announceRefresh();
          await refreshGate;
          if (init.signal?.aborted) throw init.signal.reason;
          return jsonResponse({
            tokenType: "Bearer", accessToken: "access-new", refreshToken: "refresh-new",
            accessExpiresAt: "2026-07-11T01:00:00Z", refreshExpiresAt: "2026-08-11T00:00:00Z",
            user: {}, device: {}
          });
        }
        if (init.headers.Authorization === "Bearer access-old") {
          return jsonResponse({code: "token_expired", detail: "Expired"}, {status: 401});
        }
        return jsonResponse({setupRequired: false});
      }
    }
  });

  const first = client.request("/api/system", {signal: controller.signal});
  const second = client.request("/api/system?waiter=two");
  await refreshStarted;
  controller.abort(new Error("caller stopped waiting"));
  releaseRefresh();

  await assert.rejects(first);
  await second;
  assert.equal(refreshCalls, 1);
  assert.equal(sessionStore.get().accessToken, "access-new");
});

test("an automatically refreshed request is retried at most once", async () => {
  const sessionStore = createMemorySessionStore({
    apiBaseUrl: "https://server.example",
    accessToken: "access-old",
    refreshToken: "refresh-old"
  });
  let protectedCalls = 0;
  let refreshCalls = 0;
  const client = createPorticoClient({
    sessionStore,
    transport: { fetch: async (input) => {
      if (String(input).endsWith("/api/auth/sessions/refresh")) {
        refreshCalls++;
        return jsonResponse({
          tokenType: "Bearer", accessToken: "access-new", refreshToken: "refresh-new",
          accessExpiresAt: "2026-07-11T01:00:00Z", refreshExpiresAt: "2026-08-11T00:00:00Z",
          user: {}, device: {}
        });
      }
      protectedCalls++;
      return jsonResponse({ code: "still_unauthorized", detail: "No access" }, { status: 401 });
    } }
  });

  await assert.rejects(client.system(), (error) => error instanceof ApiError && error.code === "still_unauthorized");
  assert.equal(refreshCalls, 1);
  assert.equal(protectedCalls, 2);
  assert.equal(sessionStore.get().accessToken, "access-new");
});

test("authenticated requests reject absolute paths and reserved credential headers before fetch", async () => {
  let calls = 0;
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    sessionStore: createMemorySessionStore({ apiBaseUrl: "https://server.example", accessToken: "viewer-secret" }),
    transport: { fetch: async () => { calls++; return jsonResponse({}); } }
  });
  await assert.rejects(client.request("https://evil.example/api/collect"), TypeError);
  await assert.rejects(client.request("//evil.example/api/collect"), TypeError);
  await assert.rejects(client.request("/api/%2f%2fevil.example/collect"), TypeError);
  await assert.rejects(client.request("/api/..\\\\evil.example/collect"), TypeError);
  await assert.rejects(client.request("/api/system", { headers: { Authorization: "Bearer attacker-choice" } }), TypeError);
  await assert.rejects(client.request("/api/system", { headers: { "x-portico-csrf": "attacker-choice" } }), TypeError);
  await assert.rejects(client.request("/api/system", { headers: { "X-Portico-Profile-Admin-Proof": "attacker-choice" } }), TypeError);
  assert.equal(calls, 0);
});

test("a 401 cannot transplant a newly selected server token onto the captured origin", async () => {
  const sessionStore = createMemorySessionStore({
    apiBaseUrl: "https://server-x.example", serverId: "server-x", accountId: "account-x",
    profileId: "profile-x", accessToken: "token-x", refreshToken: "refresh-x"
  });
  const calls = [];
  const client = createPorticoClient({
    sessionStore,
    transport: { fetch: async (input, init) => {
      calls.push({ url: String(input), authorization: init.headers.Authorization });
      sessionStore.set({
        apiBaseUrl: "https://server-y.example", serverId: "server-y", accountId: "account-y",
        profileId: "profile-y", accessToken: "token-y", refreshToken: "refresh-y"
      });
      return jsonResponse({ code: "token_expired", detail: "Expired" }, { status: 401 });
    } }
  });
  await assert.rejects(client.system(), error => error instanceof ApiError && error.code === "request_superseded");
  assert.deepEqual(calls, [{ url: "https://server-x.example/api/system", authorization: "Bearer token-x" }]);
});

test("playback continuation authorization never falls back to viewer refresh", async () => {
  let calls = 0;
  const client = createPorticoClient({
    sessionStore: createMemorySessionStore({
      apiBaseUrl: "https://server.example", accessToken: "viewer-token", refreshToken: "viewer-refresh"
    }),
    transport: { fetch: async (_input, init) => {
      calls++;
      assert.equal(init.headers.Authorization, "PorticoPlayback continuation-token");
      return jsonResponse({ code: "continuation_expired", detail: "Expired" }, { status: 401 });
    } }
  });
  await assert.rejects(client.getPlaybackContinuationState("session-1", {
    token: "continuation-token", origin: "https://server.example", generation: 1, expiresAt: "2026-08-05T00:00:00Z"
  }), error => error instanceof ApiError && error.code === "continuation_expired");
  assert.equal(calls, 1);
});

test("playback continuation preserves its captured scheme and origin", async () => {
  const calls = [];
  const client = createPorticoClient({
    sessionStore: createMemorySessionStore({
      apiBaseUrl: "https://new-viewer-server.example", accessToken: "viewer-token", refreshToken: "viewer-refresh"
    }),
    transport: { fetch: async (input, init) => {
      calls.push({ url: String(input), authorization: init.headers.Authorization });
      return jsonResponse({
        sessionId: "session-1", state: "playing", generation: 7,
        highestEventSequence: 0, playbackRevision: 1, queueRevision: 1, positionSeconds: 0
      });
    } }
  });
  await client.getPlaybackContinuationState("session-1", {
    token: "continuation-token", origin: "http://captured-server.example:8096", generation: 7,
    expiresAt: "2026-08-05T00:00:00Z"
  });
  assert.deepEqual(calls, [{
    url: "http://captured-server.example:8096/api/playback-sessions/session-1/continuation",
    authorization: "PorticoPlayback continuation-token"
  }]);
});

test("viewer retry is rejected before refresh when the selected profile changes with the same token", async () => {
  const sessionStore = createMemorySessionStore({
    apiBaseUrl: "https://server.example", serverId: "server-1", accountId: "account-1",
    profileId: "profile-a", accessToken: "shared-token", refreshToken: "refresh-a"
  });
  let refreshCalls = 0;
  const client = createPorticoClient({ sessionStore, transport: { fetch: async (input) => {
    if (String(input).endsWith("/api/auth/sessions/refresh")) {
      refreshCalls++;
      return jsonResponse({});
    }
    sessionStore.set({
      apiBaseUrl: "https://server.example", serverId: "server-1", accountId: "account-1",
      profileId: "profile-b", accessToken: "shared-token", refreshToken: "refresh-b"
    });
    return jsonResponse({ code: "token_expired", detail: "Expired" }, { status: 401 });
  } } });
  await assert.rejects(client.system(), error => error instanceof ApiError && error.code === "request_superseded");
  assert.equal(refreshCalls, 0);
  assert.equal(sessionStore.get().profileId, "profile-b");
});

test("explicit server-scoped revocation clears credentials without recursive refresh", async () => {
  const sessionStore = createMemorySessionStore({
    apiBaseUrl: "https://server.example",
    accessToken: "access-old",
    refreshToken: "refresh-old"
  });
  let calls = 0;
  let adapterClears = 0;
  const client = createPorticoClient({
    sessionStore,
    credentialAdapter: {
      save: async () => {},
      clear: async () => { adapterClears++; }
    },
    transport: { fetch: async (input) => {
      calls++;
      if (String(input).endsWith("/api/auth/sessions/refresh")) {
        return jsonResponse({ code: "server_session_revoked", detail: "Session revoked" }, { status: 401 });
      }
      return jsonResponse({ code: "token_expired", detail: "Expired" }, { status: 401 });
    } }
  });

  await assert.rejects(client.system(), (error) => error instanceof ApiError && error.code === "server_session_revoked");
  assert.equal(calls, 2);
  assert.equal(adapterClears, 1);
  assert.equal(sessionStore.get(), undefined);
});

test("transient refresh failure preserves credentials for a later attempt", async () => {
  const sessionStore = createMemorySessionStore({
    apiBaseUrl: "https://server.example",
    accessToken: "access-old",
    refreshToken: "refresh-old"
  });
  let adapterClears = 0;
  const client = createPorticoClient({
    sessionStore,
    credentialAdapter: { save: async () => {}, clear: async () => { adapterClears++; } },
    transport: { fetch: async (input) => String(input).endsWith("/refresh")
      ? jsonResponse({ code: "temporarily_unavailable", detail: "Try later" }, { status: 503 })
      : jsonResponse({ code: "token_expired", detail: "Expired" }, { status: 401 }) }
  });

  await assert.rejects(client.system(), (error) => error instanceof ApiError && error.status === 503);
  assert.equal(adapterClears, 0);
  assert.equal(sessionStore.get().refreshToken, "refresh-old");
});

test("credential adapter can load a session and explicit revoke never refreshes", async () => {
  const calls = [];
  let loads = 0;
  let clears = 0;
  const client = createPorticoClient({
    credentialAdapter: {
      load: async () => {
        loads++;
        return { apiBaseUrl: "https://loaded.example", accessToken: "loaded-access", refreshToken: "loaded-refresh" };
      },
      save: async () => {},
      clear: async () => { clears++; }
    },
    transport: { fetch: async (input, init) => {
      calls.push({ input: String(input), init });
      return jsonResponse({ ok: true });
    } }
  });

  await client.revokeNativeSession("loaded-refresh");
  assert.equal(loads, 1);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].input, "https://loaded.example/api/auth/sessions/revoke");
  assert.equal(calls[0].init.headers.Authorization, "Bearer loaded-access");
  assert.equal(clears, 1);
});

test("failed durable token rotation preserves the old family and retries the committed receipt key", async () => {
  const sessionStore = createMemorySessionStore({
    apiBaseUrl: "https://server.example",
    accessToken: "access-old",
    refreshToken: "refresh-old"
  });
	let durable = structuredClone(sessionStore.get());
	let pending;
  let clears = 0;
  let failSuccessorSave = true;
	const refreshBodies = [];
	const adapter = {
		load: async () => structuredClone(durable),
		save: async (session) => {
			if (failSuccessorSave && session.refreshToken === "refresh-new") throw new Error("Keychain unavailable");
			durable = structuredClone(session);
		},
		clear: async () => { clears++; durable = undefined; },
		loadPendingRotation: async () => structuredClone(pending),
		savePendingRotation: async (value) => { pending = structuredClone(value); },
		clearPendingRotation: async () => { pending = undefined; }
	};
	const transport = { fetch: async (input, init) => {
		if (String(input).endsWith("/refresh")) {
			refreshBodies.push(JSON.parse(init.body));
			return jsonResponse({
          tokenType: "Bearer", accessToken: "access-new", refreshToken: "refresh-new",
          accessExpiresAt: "2026-07-11T01:00:00Z", refreshExpiresAt: "2026-08-11T00:00:00Z",
          user: {}, device: {}
        });
		}
		return jsonResponse({ code: "token_expired", detail: "Expired" }, { status: 401 });
	} };
  const client = createPorticoClient({
    sessionStore,
    credentialAdapter: adapter,
		transport
  });

  await assert.rejects(client.system(), (error) => error instanceof ApiError && error.code === "credential_persistence_failed");
	assert.equal(clears, 0);
	assert.equal(sessionStore.get().refreshToken, "refresh-old");
	assert.equal(durable.refreshToken, "refresh-old");
	assert.equal(pending.oldRefreshToken, "refresh-old");
	assert.match(pending.rotationKey, /^[A-Za-z0-9_-]{43}$/);

	// Simulate a process restart after the server committed rotation but before
	// the successor reached durable storage. The exact old-token/key pair must
	// recover the deterministic successor rather than revoke the family.
	failSuccessorSave = false;
	const restartedStore = createMemorySessionStore(structuredClone(durable));
	const restarted = createPorticoClient({ sessionStore: restartedStore, credentialAdapter: adapter, transport });
	await assert.rejects(restarted.system(), (error) => error instanceof ApiError && error.code === "token_expired");
	assert.equal(refreshBodies.length, 2);
	assert.deepEqual(refreshBodies[1], refreshBodies[0]);
	assert.equal(restartedStore.get().refreshToken, "refresh-new");
	assert.equal(durable.refreshToken, "refresh-new");
	assert.equal(pending, undefined);
});

test("malformed successful refresh response preserves the exact rotation receipt for a safe retry", async () => {
	const sessionStore = createMemorySessionStore({ apiBaseUrl: "https://server.example", accessToken: "access-old", refreshToken: "refresh-old" });
	let pending;
	let clears = 0;
	let refreshAttempts = 0;
	const refreshBodies = [];
	const client = createPorticoClient({
		sessionStore,
		credentialAdapter: {
			save: async (session) => { sessionStore.set(session); },
			clear: async () => { clears++; },
			loadPendingRotation: async () => structuredClone(pending),
			savePendingRotation: async (value) => { pending = structuredClone(value); },
			clearPendingRotation: async () => { pending = undefined; },
		},
		transport: { fetch: async (input, init) => {
			if (String(input).endsWith("/refresh")) {
				refreshBodies.push(JSON.parse(init.body));
				refreshAttempts++;
				if (refreshAttempts === 1) return new Response("{", { status: 200, headers: { "Content-Type": "application/json" } });
				return jsonResponse({
					tokenType: "Bearer", accessToken: "access-new", refreshToken: "refresh-new",
					accessExpiresAt: "2026-07-11T01:00:00Z", refreshExpiresAt: "2026-08-11T00:00:00Z",
					user: {}, device: {},
				});
			}
			return jsonResponse({ code: "token_expired", detail: "Expired" }, { status: 401 });
		} },
	});

	await assert.rejects(client.system(), (error) => error instanceof ApiError && error.code === "invalid_refresh_response");
	assert.equal(clears, 0);
	assert.equal(sessionStore.get().refreshToken, "refresh-old");
	assert.equal(pending.oldRefreshToken, "refresh-old");
	await assert.rejects(client.system(), (error) => error instanceof ApiError && error.code === "token_expired");
	assert.deepEqual(refreshBodies[1], refreshBodies[0]);
	assert.equal(sessionStore.get().refreshToken, "refresh-new");
	assert.equal(pending, undefined);
	assert.equal(clears, 0);
});

test("SessionStore publication failure rejects credential creation and attempts every credential deletion", async () => {
  let durable;
  let memoryClearCalls = 0;
  let durableClearCalls = 0;
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    sessionStore: {
      get: () => undefined,
      set: () => { throw new Error("runtime credential publication failed"); },
      clear: () => { memoryClearCalls += 1; throw new Error("memory cleanup failed"); }
    },
    credentialAdapter: {
      save: async session => { durable = structuredClone(session); },
      clear: async () => { durableClearCalls += 1; durable = undefined; }
    },
    transport: { fetch: async () => jsonResponse({
      tokenType: "Bearer",
      authority: "local",
      accountId: "local-account",
      serverId: "server-1",
      profileId: "profile-1",
      authorizationRevision: "policy-1",
      accessToken: "access-new",
      accessExpiresAt: "2026-07-11T01:00:00Z",
      refreshToken: "refresh-new",
      refreshExpiresAt: "2026-08-11T00:00:00Z",
      user: {},
      device: {}
    }, { status: 201 }) }
  });

  await assert.rejects(client.createNativeSession({
    login: "owner",
    password: "secret",
    installationId: "native-install-0001",
    deviceName: "Phone",
    app: "Portico",
    platform: "iOS"
  }), error => error instanceof CredentialCleanupUncertainError
    && error.code === "credential_cleanup_uncertain"
    && error.failClosed === true
    && error.rollbackFailures.length === 1
    && error.rollbackFailures[0]?.message === "memory cleanup failed"
    && error.details?.cleanupFailureCount === 1);

  assert.equal(memoryClearCalls, 1);
  assert.equal(durableClearCalls, 1, "durable deletion still runs after the memory deletion fails");
  assert.equal(durable, undefined);
});

test("failed durable credential deletion is a typed fail-closed cleanup uncertainty", async () => {
  let durable;
  let fetchCalls = 0;
  const adapter = {
    load: async () => durable,
    save: async session => { durable = structuredClone(session); },
    clear: async () => { throw new Error("durable credential deletion failed"); }
  };
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    sessionStore: {
      get: () => undefined,
      set: () => { throw new Error("runtime credential publication failed"); },
      clear: () => {}
    },
    credentialAdapter: adapter,
    transport: { fetch: async () => {
      fetchCalls += 1;
      return jsonResponse({
        tokenType: "Bearer",
        authority: "local",
        accountId: "local-account",
        serverId: "server-1",
        profileId: "profile-1",
        authorizationRevision: "policy-1",
        accessToken: "access-new",
        accessExpiresAt: "2026-07-11T01:00:00Z",
        refreshToken: "refresh-new",
        refreshExpiresAt: "2026-08-11T00:00:00Z",
        user: {},
        device: {}
      }, { status: 201 });
    } }
  });

  await assert.rejects(client.createNativeSession({
    login: "owner",
    password: "secret",
    installationId: "native-install-0001",
    deviceName: "Phone",
    app: "Portico",
    platform: "iOS"
  }), error => error instanceof CredentialCleanupUncertainError
    && error.cause?.message === "runtime credential publication failed"
    && error.rollbackFailures.some(failure => failure?.message === "durable credential deletion failed")
    && error.failClosed === true);

  assert.equal(fetchCalls, 1);
  assert.equal(durable?.refreshToken, "refresh-new", "the typed error truthfully reports that a durable copy may remain");

  // A fresh process can still observe the retained copy. Platform runtimes
  // must use the typed error to publish a separate cleanup quarantine before
  // permitting restore; Client Core never claims process memory solves this.
  assert.equal((await adapter.load())?.refreshToken, "refresh-new");
});

test("adapter restore cannot publish a stale credential when SessionStore.set fails", async () => {
  let adapterClearCalls = 0;
  let memoryClearCalls = 0;
  let fetchCalls = 0;
  const client = createPorticoClient({
    sessionStore: {
      get: () => undefined,
      set: () => { throw new Error("runtime restore rejected"); },
      clear: () => { memoryClearCalls += 1; }
    },
    credentialAdapter: {
      load: async () => ({
        serverId: "stale-server",
        apiBaseUrl: "https://stale.example",
        accessToken: "stale-access",
        refreshToken: "stale-refresh"
      }),
      save: async () => {},
      clear: async () => { adapterClearCalls += 1; }
    },
    transport: { fetch: async () => { fetchCalls += 1; return jsonResponse({ authenticated: true }); } }
  });

  await assert.rejects(client.me(), error => error instanceof ApiError
    && error.code === "credential_persistence_failed");
  assert.equal(fetchCalls, 0, "a credential rejected by the active store is never used for a request");
  assert.equal(memoryClearCalls, 1);
  assert.equal(adapterClearCalls, 1);
});

test("binary bitrate requests use the same one-time refresh lifecycle", async () => {
  const sessionStore = createMemorySessionStore({
    apiBaseUrl: "https://server.example",
    accessToken: "access-old",
    refreshToken: "refresh-old"
  });
  const calls = [];
  const client = createPorticoClient({
    sessionStore,
    now: (() => {
      let value = 100;
      return () => (value += 10);
    })(),
    transport: { fetch: async (input, init) => {
      calls.push({ input: String(input), authorization: init.headers.Authorization });
      if (String(input).endsWith("/api/auth/sessions/refresh")) {
        return jsonResponse({
          tokenType: "Bearer", accessToken: "access-new", refreshToken: "refresh-new",
          accessExpiresAt: "2026-07-11T01:00:00Z", refreshExpiresAt: "2026-08-11T00:00:00Z",
          user: {}, device: {}
        });
      }
      if (init.headers.Authorization === "Bearer access-old") {
        return jsonResponse({ code: "token_expired" }, { status: 401 });
      }
      return new Response(new Uint8Array([1, 2, 3, 4]));
    } }
  });

  const result = await client.bitrateTest(4);
  assert.equal(result.bytes, 4);
  assert.deepEqual(calls.map((call) => call.authorization), [
    "Bearer access-old",
    undefined,
    "Bearer access-new"
  ]);
});

test("watchlist list uses dedicated API path", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return jsonResponse({ items: [], pageInfo: { nextCursor: null, hasMore: false } });
      }
    }
  });

  const result = await client.watchlist({ limit: 25, cursor: "watch-next" });
  assert.equal(calls[0].input, "https://server.example/api/watchlist?limit=25&cursor=watch-next");
  assert.equal(result.pageInfo.hasMore, false);

  await client.watchlist({ limit: 72, filter: "inProgress", sort: "title", order: "asc" });
  assert.equal(calls[1].input, "https://server.example/api/watchlist?limit=72&filter=inProgress&sort=title&order=asc");
});

test("favorites are a first-class saved surface with cancellation", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return jsonResponse({ items: [], pageInfo: { nextCursor: null, hasMore: false } });
      }
    }
  });

  const controller = new AbortController();
  await client.favorites(
    { limit: 72, cursor: "favorite-next", filter: "unwatched", sort: "progress", order: "desc" },
    { signal: controller.signal }
  );
  await client.setFavorite("media 1", true, { signal: controller.signal });

  assert.equal(calls[0].input, "https://server.example/api/favorites?limit=72&cursor=favorite-next&filter=unwatched&sort=progress&order=desc");
  assert.equal(calls[0].init.signal.aborted, false);
  assert.equal(calls[1].input, "https://server.example/api/media/media%201/favorite");
  assert.equal(calls[1].init.method, "POST");
  assert.equal(calls[1].init.signal.aborted, false);
  assert.deepEqual(JSON.parse(calls[1].init.body), { favorite: true });
});

test("full search forwards the canonical sort and direction contract", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return jsonResponse({ query: "arrival", sort: "dateAdded", direction: "desc", groups: [] });
      }
    }
  });

  const response = await client.search({
    query: "arrival",
    group: "movies",
    sort: "dateAdded",
    direction: "desc",
    limit: 24
  });

  assert.equal(calls[0].input, "https://server.example/api/search");
  assert.equal(calls[0].init.method, "POST");
  assert.deepEqual(JSON.parse(calls[0].init.body), {
    query: "arrival",
    group: "movies",
    sort: "dateAdded",
    direction: "desc",
    limit: 24
  });
  assert.equal(response.sort, "dateAdded");
  assert.equal(response.direction, "desc");
});

test("Live TV paging and DVR operational status preserve server-side browse controls", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        if (String(input).includes("/guide")) {
          return jsonResponse({
            source: { id: "source 1", name: "Tuner", type: "m3u", enabled: true, hasM3uText: false, hasEpgText: false, hasXtreamPassword: false, streamBufferSeconds: 0, maxRetrySeconds: 0, refreshIntervalHours: 12, filterRequireEpg: false, channelCount: 1, programCount: 1, logoCount: 0, hiddenChannelCount: 0, favoriteChannelCount: 1, actions: [] },
            channels: [], programs: [], channelGroups: ["Local News", "Sports"], from: "2026-07-11T12:00:00Z", to: "2026-07-11T16:00:00Z", serverTime: "2026-07-11T12:00:00Z", pageInfo: { nextCursor: null, hasMore: false, total: 0 },
            capabilities: { canPlay: true, canFavoriteChannels: true, canScheduleRecordings: true, canManageRecordingRules: false, canManageSources: false }
          });
        }
        if (String(input).includes("/channels")) return jsonResponse({ items: [], pageInfo: { nextCursor: null, hasMore: false, total: 0 }, groups: ["Local News", "Sports"] });
        return jsonResponse({ configured: true, available: true, capabilities: { canScheduleRecordings: true, canManageRecordingRules: false, actions: [] }, guide: { state: "current" }, conflicts: [], tuners: [{ id: "source 1", name: "Tuner", state: "idle" }], storage: { usedBytes: 0, availableBytes: 1024, state: "healthy" }, generatedAt: "2026-07-11T12:00:00Z" });
      }
    }
  });
  const controller = new AbortController();

  const guide = await client.liveTvGuide("source 1", {
    from: "2026-07-11T12:00:00Z", hours: 4, limit: 25, cursor: "guide-next", count: "exact", query: "evening news",
    filter: "favorites", sort: "now", order: "desc", group: "Local News"
  }, { signal: controller.signal });
  const channels = await client.liveTvChannels("source 1", {
    limit: 25, cursor: "channel-next", count: "exact", query: "news 7", favoritesOnly: true, group: "Local News"
  }, { signal: controller.signal });
  const operational = await client.dvrStatus("source 1", { signal: controller.signal });

  assert.equal(calls[0].input, "https://server.example/api/live-tv/sources/source%201/guide?from=2026-07-11T12%3A00%3A00Z&hours=4&limit=25&cursor=guide-next&count=exact&query=evening+news&filter=favorites&sort=now&order=desc&group=Local+News");
  assert.equal(calls[1].input, "https://server.example/api/live-tv/sources/source%201/channels?limit=25&cursor=channel-next&count=exact&query=news+7&favoritesOnly=true&group=Local+News");
  assert.equal(calls[2].input, "https://server.example/api/dvr/status?sourceId=source+1");
  assert.equal(calls.every((call) => call.init.signal instanceof AbortSignal && !call.init.signal.aborted), true);
  assert.deepEqual(guide.channelGroups, ["Local News", "Sports"]);
  assert.deepEqual(channels.groups, ["Local News", "Sports"]);
  assert.equal(operational.available, true);
});

test("home rows use opaque cursors and never emit offsets", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: { fetch: async (input, init) => {
      calls.push({ input: String(input), init });
      return jsonResponse({ id: "recent_movies", title: "Recent", type: "poster", defaultVisible: true, items: [] });
    } }
  });
  await client.homeRow("recent_movies", { limit: 12, cursor: "opaque-token" });
  assert.equal(calls[0].input, "https://server.example/api/home/rows/recent_movies?limit=12&cursor=opaque-token");
  assert.equal(calls[0].input.includes("offset="), false);
});

test("canonical library browse uses POST bodies and cursor envelopes", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: { fetch: async (input, init) => {
      calls.push({ input: String(input), init });
      if (String(input).endsWith("/api/product-contract")) {
        return jsonResponse({ apiVersion: "v1", libraryKinds: [], entityKinds: [], browseFields: [], browseSorts: [], browseOperators: [], presentationFields: [], queryLimits: {}, serverCapabilities: [] });
      }
      if (String(input).endsWith("/browse-capabilities")) {
        return jsonResponse({ apiVersion: "v1", library: { id: "lib movies", name: "Movies", kind: "movies" }, pivots: [], fields: [], sorts: [], presentationFields: [], actions: ["browse"], queryLimits: {} });
      }
      return jsonResponse({ items: [], pageInfo: { nextCursor: null, hasMore: false }, applied: { pivot: "movies", sort: [{ field: "title", direction: "asc" }], presentationFields: [] } });
    } }
  });

  await client.productContract();
  await client.libraryBrowseCapabilities("lib movies");
  const request = { pivot: "movies", sort: [{ field: "title", direction: "asc" }], cursor: "opaque", limit: 24 };
  const response = await client.browseLibrary("lib movies", request);

  assert.equal(calls[0].input, "https://server.example/api/product-contract");
  assert.equal(calls[1].input, "https://server.example/api/libraries/lib%20movies/browse-capabilities");
  assert.equal(calls[2].input, "https://server.example/api/libraries/lib%20movies/browse");
  assert.equal(calls[2].init.method, "POST");
  assert.equal(calls[2].init.body, JSON.stringify(request));
  assert.equal(response.pageInfo.nextCursor, null);
});

test("library sources use opaque cursor pagination", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: { fetch: async (input) => {
      calls.push(String(input));
      return jsonResponse({ items: [], pageInfo: { nextCursor: null, hasMore: false } });
    } }
  });

  const response = await client.librarySources("lib movies", { limit: 25, cursor: "source-next" });

  assert.equal(calls[0], "https://server.example/api/libraries/lib%20movies/sources?limit=25&cursor=source-next");
  assert.equal(response.pageInfo.hasMore, false);
});

test("managed remote storage methods preserve owner input and encoded routes", async () => {
  const calls = [];
  const source = {
    kind: "webdav",
    name: "Cloud Movies",
    endpoint: "https://dav.example.test/media",
    root: "Movies",
    analysisMode: "basic",
    username: "owner",
    password: "write-only-secret"
  };
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: { fetch: async (input, init = {}) => {
      calls.push({ input: String(input), init });
      if (init.method === "DELETE") return new Response(null, { status: 204 });
      if (String(input).endsWith("/inventory")) return jsonResponse({ id: "job-1", type: "library_scan", status: "queued", title: "Remote inventory", progress: 0, priority: 0, createdAt: "2026-08-22T12:00:00Z", updatedAt: "2026-08-22T12:00:00Z" }, { status: 202 });
      if (init.method === "POST") return jsonResponse({ id: "source-1", libraryId: "lib movies", kind: "webdav", name: "Cloud Movies", endpoint: source.endpoint, root: "Movies", health: "unknown", inventoryStatus: "never", objects: 0, missingObjects: 0, credentialPresent: true, updatedAt: "2026-08-22T12:00:00Z" }, { status: 201 });
      return jsonResponse({ items: [], total: 0, limit: 0 });
    } }
  });

  await client.remoteStorageSources("lib movies");
  await client.createRemoteStorageSource("lib movies", source);
  await client.updateRemoteStorageSourceAnalysisMode("lib movies", "source/1", { analysisMode: "file_list_only" });
  await client.inventoryRemoteStorageSource("lib movies", "source/1");
  await client.deleteRemoteStorageSource("lib movies", "source/1");

  assert.equal(calls[0].input, "https://server.example/api/libraries/lib%20movies/remote-storage-sources");
  assert.equal(calls[1].init.method, "POST");
  assert.equal(calls[1].init.body, JSON.stringify(source));
  assert.equal(calls[2].input, "https://server.example/api/libraries/lib%20movies/remote-storage-sources/source%2F1");
  assert.equal(calls[2].init.method, "PATCH");
  assert.equal(calls[2].init.body, JSON.stringify({ analysisMode: "file_list_only" }));
  assert.equal(calls[3].input, "https://server.example/api/libraries/lib%20movies/remote-storage-sources/source%2F1/inventory");
  assert.equal(calls[3].init.method, "POST");
  assert.equal(calls[4].input, "https://server.example/api/libraries/lib%20movies/remote-storage-sources/source%2F1");
  assert.equal(calls[4].init.method, "DELETE");
});

test("auth me forwards abort signals", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return new Promise((resolve, reject) => {
          init.signal.addEventListener("abort", () => reject(new DOMException("The operation was aborted.", "AbortError")), { once: true });
        });
      }
    }
  });

  const controller = new AbortController();
  const pending = client.me({ signal: controller.signal });
  controller.abort();

  await assert.rejects(pending, { name: "AbortError" });
  assert.equal(calls[0].input, "https://server.example/api/auth/me");
  assert.equal(calls[0].init.signal.aborted, true);
});

test("local account password changes use the authenticated account contract", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return jsonResponse({ ok: true });
      }
    }
  });
  const controller = new AbortController();
  const body = { currentPassword: "current-password", newPassword: "new-password-123" };

  const result = await client.changeAccountPassword(body, { signal: controller.signal });

  assert.deepEqual(result, { ok: true });
  assert.equal(calls[0].input, "https://server.example/api/account/password");
  assert.equal(calls[0].init.method, "POST");
  assert.equal(calls[0].init.body, JSON.stringify(body));
  assert.equal(calls[0].init.signal.aborted, false);
});

test("playlist detail and items use canonical cursor paths", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        if (String(input).includes("/items")) return jsonResponse({ items: [], pageInfo: { nextCursor: "next", hasMore: true } });
        return jsonResponse({ id: "plist 1", ownerUserId: "user_1", title: "Queue", visibility: "private", itemCount: 275, canEdit: true, shares: [], createdAt: "2026-01-01T00:00:00Z", updatedAt: "2026-01-01T00:00:00Z" });
      }
    }
  });

  const controller = new AbortController();
  const playlist = await client.playlist("plist 1", { signal: controller.signal });
  const page = await client.playlistItems("plist 1", { limit: 50, cursor: "opaque" });

  assert.equal(calls[0].input, "https://server.example/api/playlists/plist%201");
  assert.equal(calls[0].init.signal.aborted, false);
  assert.equal(calls[1].input, "https://server.example/api/playlists/plist%201/items?cursor=opaque&limit=50");
  assert.equal(playlist.itemCount, 275);
  assert.equal(page.pageInfo.nextCursor, "next");
  assert.equal(page.pageInfo.hasMore, true);
});

test("saved sharing candidate discovery uses the narrow cancellable contract", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return jsonResponse({ items: [{ userId: "user_2", displayName: "Alex Rivera" }], hasMore: false });
      }
    }
  });
  const controller = new AbortController();

  const page = await client.savedShareCandidates({ query: "Alex Rivera", limit: 12 }, { signal: controller.signal });

  assert.equal(calls[0].input, "https://server.example/api/saved/share-candidates?q=Alex+Rivera&limit=12");
  assert.equal(calls[0].init.signal instanceof AbortSignal, true);
  assert.equal(calls[0].init.signal.aborted, false);
  assert.deepEqual(page, { items: [{ userId: "user_2", displayName: "Alex Rivera" }], hasMore: false });
});

test("saved resource mutations use atomic batch routes", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return jsonResponse({ added: 1, removed: 1, unchanged: 0, playlist: { id: "p1", itemCount: 2 } });
      }
    }
  });

  await client.mutatePlaylistItems("p1", {
    addMediaIds: ["m3"],
    removeEntryIds: ["entry-1"],
    orderEntryIds: ["entry-3", "entry-2"],
    expectedUpdatedAt: "2026-01-01T00:00:00Z"
  });

  assert.equal(calls[0].input, "https://server.example/api/playlists/p1/items:batch");
  assert.equal(calls[0].init.method, "POST");
  assert.deepEqual(JSON.parse(calls[0].init.body), {
    addMediaIds: ["m3"],
    removeEntryIds: ["entry-1"],
    orderEntryIds: ["entry-3", "entry-2"],
    expectedUpdatedAt: "2026-01-01T00:00:00Z"
  });
});

test("all saved resource operations expose cancellation on canonical routes", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return jsonResponse({ items: [], pageInfo: { nextCursor: null, hasMore: false } });
      }
    }
  });
  const controller = new AbortController();
  const request = { signal: controller.signal };

  await client.playlists({ cursor: "next", limit: 25, libraryId: "lib 1" }, request);
  await client.collections({ cursor: "next", limit: 25, libraryId: "lib 1" }, request);
  await client.playlist("playlist 1", request);
  await client.collection("collection 1", request);
  await client.playlistItems("playlist 1", { cursor: "items", limit: 50 }, request);
  await client.collectionItems("collection 1", { cursor: "items", limit: 50 }, request);
  await client.createPlaylist({ title: "Road trip" }, request);
  await client.createCollection({ title: "Noir" }, request);
  await client.updatePlaylist("playlist 1", { title: "Road trip 2" }, request);
  await client.updateCollection("collection 1", { title: "Neo-noir" }, request);
  await client.deletePlaylist("playlist 1", request);
  await client.deleteCollection("collection 1", request);
  await client.mutatePlaylistItems("playlist 1", { addMediaIds: ["m1"] }, request);
  await client.mutateCollectionMemberships("collection 1", { addMediaIds: ["m1"] }, request);
  await client.savedViews({ cursor: "next", limit: 25, libraryId: "lib 1" }, request);
  await client.savedView("view 1", request);
  await client.createSavedView({ title: "Unwatched", libraryId: "lib 1", query: {} }, request);
  await client.updateSavedView("view 1", { title: "Unwatched movies" }, request);
  await client.deleteSavedView("view 1", request);
  await client.browseSavedView("view 1", { cursor: "page 2", limit: 50 }, request);

  assert.deepEqual(calls.map((call) => call.input), [
    "https://server.example/api/playlists?cursor=next&limit=25&libraryId=lib+1",
    "https://server.example/api/collections?cursor=next&limit=25&libraryId=lib+1",
    "https://server.example/api/playlists/playlist%201",
    "https://server.example/api/collections/collection%201",
    "https://server.example/api/playlists/playlist%201/items?cursor=items&limit=50",
    "https://server.example/api/collections/collection%201/items?cursor=items&limit=50",
    "https://server.example/api/playlists",
    "https://server.example/api/collections",
    "https://server.example/api/playlists/playlist%201",
    "https://server.example/api/collections/collection%201",
    "https://server.example/api/playlists/playlist%201",
    "https://server.example/api/collections/collection%201",
    "https://server.example/api/playlists/playlist%201/items:batch",
    "https://server.example/api/collections/collection%201/memberships:batch",
    "https://server.example/api/saved-views?cursor=next&limit=25&libraryId=lib+1",
    "https://server.example/api/saved-views/view%201",
    "https://server.example/api/saved-views",
    "https://server.example/api/saved-views/view%201",
    "https://server.example/api/saved-views/view%201",
    "https://server.example/api/saved-views/view%201/browse"
  ]);
  assert.equal(calls.every((call) => call.init.signal instanceof AbortSignal && !call.init.signal.aborted), true);
  assert.deepEqual(calls.slice(6).map((call) => call.init.method), [
    "POST", "POST", "PATCH", "PATCH", "DELETE", "DELETE", "POST", "POST",
    "GET", "GET", "POST", "PATCH", "DELETE", "POST"
  ]);
});

test("settings use revisioned documents and PATCH envelopes", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        if (String(input).endsWith("/summary")) return jsonResponse({ groups: [] });
        return jsonResponse({
          revision: "settings-rev-2",
          updatedAt: "2026-07-11T12:00:00Z",
          groups: { server: { friendlyName: "Portico" } },
          restartRequired: false,
          restartRequiredFields: [],
          applyImpact: { changedFields: ["server.friendlyName"], restartRequired: false, restartRequiredFields: [] }
        });
      }
    }
  });
  const controller = new AbortController();
  const request = { signal: controller.signal };

  const current = await client.settings(request);
  await client.settingsSummary(request);
  const updated = await client.updateSettings({
    expectedRevision: current.revision,
    groups: { server: { friendlyName: "Living Room" } }
  }, request);

  assert.equal(current.revision, "settings-rev-2");
  assert.equal(updated.applyImpact.changedFields[0], "server.friendlyName");
  assert.deepEqual(calls.map((call) => [call.input, call.init.method]), [
    ["https://server.example/api/settings", "GET"],
    ["https://server.example/api/settings/summary", "GET"],
    ["https://server.example/api/settings", "PATCH"]
  ]);
  assert.deepEqual(JSON.parse(calls[2].init.body), {
    expectedRevision: "settings-rev-2",
    groups: { server: { friendlyName: "Living Room" } }
  });
  assert.equal(calls.every((call) => call.init.signal instanceof AbortSignal && !call.init.signal.aborted), true);
});

test("library discover uses dedicated library API path", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return jsonResponse({ items: [], rows: [], total: 0, generatedAt: "2026-06-08T00:00:00Z" });
      }
    }
  });

  const result = await client.libraryDiscover("lib movies", { limit: 12 });
  assert.equal(calls[0].input, "https://server.example/api/libraries/lib%20movies/discover?limit=12");
  assert.equal(result.generatedAt, "2026-06-08T00:00:00Z");
});

test("playback history export never places account credentials in the URL", () => {
  const sessionStore = createMemorySessionStore({
    apiBaseUrl: "https://server.example",
    bootstrapAccessToken: "token-123"
  });
  const client = createPorticoClient({ sessionStore });

  const url = client.playbackHistoryExportUrl({
    userId: "all",
    libraryId: "lib 1",
    type: "movie",
    query: " Roku Ultra ",
    period: "90d"
  });

  assert.equal(
    url,
    "https://server.example/api/playback/history/export.csv?libraryId=lib+1&type=movie&query=Roku+Ultra&period=90d"
  );
});

test("clear watch history sends DELETE and invalidates history surfaces", async () => {
  const calls = [];
  const tags = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    csrfToken: "csrf-local",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return jsonResponse({ ok: true, clearedAt: "2026-06-19T12:00:00Z" });
      }
    },
    onMutation(nextTags) {
      tags.push(nextTags);
    }
  });

  const response = await client.clearWatchHistory();
  assert.deepEqual(response, { ok: true, clearedAt: "2026-06-19T12:00:00Z" });
  assert.equal(calls[0].input, "https://server.example/api/account/watch-history");
  assert.equal(calls[0].init.method, "DELETE");
  assert.equal(calls[0].init.headers["X-Portico-CSRF"], "csrf-local");
  assert.deepEqual(tags[0], ["account", "auth", "media-state", "playback-progress", "dashboard:history", "home", "playback"]);
});

test("local client coalesces identical in-flight JSON GETs", async () => {
  let resolveFetch;
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return new Promise((resolve) => {
          resolveFetch = () => resolve(jsonResponse({ pivots: ["Home"], rows: [] }));
        });
      }
    }
  });

  const first = client.home();
  const second = client.home();
  assert.equal(calls.length, 1);
  assert.equal(calls[0].input, "https://server.example/api/home");

  resolveFetch();
  const [firstResult, secondResult] = await Promise.all([first, second]);
  assert.deepEqual(firstResult, secondResult);
  assert.deepEqual(firstResult.rows, []);
});

test("identical GETs never coalesce across client instances or transports", async () => {
  const calls = [];
  const create = (name) => createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: { fetch: async () => {
      calls.push(name);
      return jsonResponse({ pivots: [name], rows: [] });
    } }
  });
  const [left, right] = await Promise.all([create("left").home(), create("right").home()]);
  assert.deepEqual(calls.sort(), ["left", "right"]);
  assert.deepEqual(left.pivots, ["left"]);
  assert.deepEqual(right.pivots, ["right"]);
});

test("local client keeps shared GET alive until every subscriber aborts", async () => {
  let resolveFetch;
  let sharedSignal;
  const firstController = new AbortController();
  const secondController = new AbortController();
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (_input, init) => {
        sharedSignal = init.signal;
        return new Promise((resolve, reject) => {
          resolveFetch = () => resolve(jsonResponse({ items: [{ id: "m1", title: "Movie", type: "movie" }], total: 1 }));
          init.signal.addEventListener("abort", () => reject(new DOMException("The operation was aborted.", "AbortError")), { once: true });
        });
      }
    }
  });

  const first = client.search("blade", { signal: firstController.signal });
  const second = client.search("blade", { signal: secondController.signal });
  firstController.abort();

  await assert.rejects(first, { name: "AbortError" });
  assert.equal(sharedSignal.aborted, false);

  resolveFetch();
  const result = await second;
  assert.equal(result.total, 1);
});

test("local client aborts shared GET when every subscriber aborts", async () => {
  let sharedSignal;
  const firstController = new AbortController();
  const secondController = new AbortController();
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (_input, init) => {
        sharedSignal = init.signal;
        return new Promise((_resolve, reject) => {
          init.signal.addEventListener("abort", () => reject(new DOMException("The operation was aborted.", "AbortError")), { once: true });
        });
      }
    }
  });

  const first = client.search("blade", { signal: firstController.signal });
  const second = client.search("blade", { signal: secondController.signal });

  firstController.abort();
  await assert.rejects(first, { name: "AbortError" });
  assert.equal(sharedSignal.aborted, false);

  secondController.abort();
  await assert.rejects(second, { name: "AbortError" });
  assert.equal(sharedSignal.aborted, true);
});

test("local client starts a fresh GET when a replacement subscriber follows the final abort", async () => {
  let calls = 0;
  const firstController = new AbortController();
  const secondController = new AbortController();
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (_input, init) => {
        calls++;
        if (calls === 1) {
          return new Promise((_resolve, reject) => {
            init.signal.addEventListener("abort", () => reject(new DOMException("The operation was aborted.", "AbortError")), { once: true });
          });
        }
        return jsonResponse({
          setupRequired: false,
          localCredentialsEnabled: true,
          porticoAccountAuthEnabled: true,
          publicUserPickerEnabled: false,
          visibleUsers: []
        });
      }
    }
  });

  const first = client.request("/api/auth/capabilities", { signal: firstController.signal });
  firstController.abort();
  const replacement = client.request("/api/auth/capabilities", { signal: secondController.signal });

  await assert.rejects(first, { name: "AbortError" });
  assert.equal(calls, 2);
  assert.equal((await replacement).setupRequired, false);
});

test("transport deadline aborts a fetch that never resolves", async () => {
  let requestSignal;
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    requestTimeoutMs: 20,
    transport: {
      fetch: async (_input, init) => {
        requestSignal = init.signal;
        return new Promise(() => {});
      }
    }
  });

  await assert.rejects(
    client.request("/api/health"),
    (error) => error instanceof PorticoTransportError && error.code === "request_timeout" && isRetryablePorticoError(error)
  );
  assert.equal(requestSignal.aborted, true);
});

test("caller cancellation reaches the shared transport and rejects promptly", async () => {
  let requestSignal;
  const controller = new AbortController();
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (_input, init) => {
        requestSignal = init.signal;
        return new Promise(() => {});
      }
    }
  });

  const pending = client.request("/api/health", { signal: controller.signal });
  await new Promise((resolve) => setTimeout(resolve, 0));
  controller.abort();
  await assert.rejects(pending, { name: "AbortError" });
  assert.equal(requestSignal.aborted, true);
});

test("caller cancellation reaches media and diagnostics mutations", async () => {
	for (const invoke of [
		(client, signal) => client.uploadClientLogs({ events: [] }, { signal }),
		(client, signal) => client.deleteMediaImage("movie-1", "poster-1", 7, { signal }),
		(client, signal) => client.updateSubtitle("movie-1", "subtitle-1", { offsetMs: 250 }, { signal }),
	]) {
		let requestSignal;
		const controller = new AbortController();
		const client = createPorticoClient({
			apiBaseUrl: "https://server.example",
			transport: {
				fetch: async (_input, init) => {
					requestSignal = init.signal;
					return new Promise(() => {});
				}
			}
		});
		const pending = invoke(client, controller.signal);
		await new Promise((resolve) => setTimeout(resolve, 0));
		controller.abort();
		await assert.rejects(pending, { name: "AbortError" });
		assert.equal(requestSignal.aborted, true);
	}
});

test("transport deadline covers a response body that never resolves", async () => {
  const body = new ReadableStream({ start() {} });
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    requestTimeoutMs: 20,
    transport: {
      fetch: async () => new Response(body, { status: 200, headers: { "Content-Type": "application/json" } })
    }
  });

  await assert.rejects(client.request("/api/health"), (error) => error instanceof PorticoTransportError && error.code === "request_timeout");
});

test("Hosted safe-read retry budget is bounded and mutations are ambiguous on timeout", async () => {
  let readAttempts = 0;
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    retryBudget: 1,
    retryDelaysMs: [0],
    transport: {
      fetch: async () => {
        readAttempts++;
        return new Response(JSON.stringify({ code: "busy", message: "try again" }), {
          status: 503,
          headers: { "Content-Type": "application/json" }
        });
      }
    }
  });

  await assert.rejects(hosted.request("/api/system"), (error) => error instanceof ApiError && error.status === 503);
  assert.equal(readAttempts, 2);

  let mutationAttempts = 0;
  const local = createPorticoClient({
    apiBaseUrl: "https://server.example",
    requestTimeoutMs: 20,
    transport: {
      fetch: async () => {
        mutationAttempts++;
        return new Promise(() => {});
      }
    }
  });
  await assert.rejects(
    local.request("/api/auth/login", { method: "POST", body: { login: "user", password: "secret" } }),
    (error) => error instanceof PorticoTransportError && isAmbiguousPorticoError(error) && !isRetryablePorticoError(error)
  );
  assert.equal(mutationAttempts, 1);
});

test("Hosted safe reads honor Retry-After without waiting beyond the request deadline", async () => {
  let attempts = 0;
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    requestTimeoutMs: 250,
    retryBudget: 1,
    retryDelaysMs: [0],
    transport: {
      fetch: async () => {
        attempts++;
        if (attempts === 1) {
          return new Response(JSON.stringify({ code: "busy", message: "try again" }), {
            status: 429,
            headers: { "Content-Type": "application/json", "Retry-After": "0.02" }
          });
        }
        return jsonResponse({ ok: true });
      }
    }
  });

  const startedAt = Date.now();
  assert.deepEqual(await hosted.request("/api/system"), { ok: true });
  assert.equal(attempts, 2);
  assert.ok(Date.now() - startedAt >= 15, "the retry must not run before Retry-After");

  attempts = 0;
  const deadlineBound = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    requestTimeoutMs: 50,
    retryBudget: 1,
    retryDelaysMs: [0],
    transport: {
      fetch: async () => {
        attempts++;
        return new Response(JSON.stringify({ code: "busy", message: "try again" }), {
          status: 429,
          headers: { "Content-Type": "application/json", "Retry-After": "10" }
        });
      }
    }
  });
  await assert.rejects(
    deadlineBound.request("/api/system"),
    error => error instanceof ApiError && error.status === 429 && error.retryAfterMs === 10_000
  );
  assert.equal(attempts, 1, "a retry scheduled beyond the deadline must be returned to the caller");
});

test("callers cannot override the credential policy for local or Hosted requests", async () => {
  const localCalls = [];
  const local = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (_input, init) => {
        localCalls.push(init);
        return jsonResponse({ ok: true });
      }
    }
  });

  await local.request("/api/system", { credentials: "omit" });
  await local.request("/api/health", { authorization: { mode: "anonymous" }, credentials: "include" });
  assert.equal(localCalls[0].credentials, "include");
  assert.equal(localCalls[1].credentials, "omit");

  const hostedCalls = [];
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    transport: {
      fetch: async (_input, init) => {
        hostedCalls.push(init);
        return jsonResponse({ ok: true });
      }
    }
  });

  await hosted.request("/api/system", { credentials: "omit" });
  assert.equal(hostedCalls[0].credentials, "include");
});

test("retry and ambiguity classification fails closed for ordinary errors", () => {
  assert.equal(isRetryablePorticoError(new Error("temporary-looking failure")), false);
  assert.equal(isAmbiguousPorticoError(new Error("unknown result")), false);
  assert.equal(isRetryablePorticoError({ retryable: true }), true);
  assert.equal(isAmbiguousPorticoError({ ambiguous: true }), true);
});

test("form uploads avoid JSON content-type and keep CSRF", async () => {
  const form = new FormData();
  form.set("file", new Blob(["vtt"]), "subtitles.vtt");
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    csrfToken: "1",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return jsonResponse({ id: "m1", title: "Movie", type: "movie", state: {} });
      }
    }
  });

  await client.uploadSubtitle("m1", form);
  assert.equal(calls[0].init.headers["Content-Type"], undefined);
  assert.equal(calls[0].init.headers["X-Portico-CSRF"], "1");
  assert.equal(calls[0].init.body, form);
});

test("atomic playback stop sends its required terminal body with CSRF", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    csrfToken: "1",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        const request = JSON.parse(init.body);
        return jsonResponse(playbackTerminalReceipt("session-1", request));
      }
    }
  });

  client.acceptPlaybackSession(acceptedPlayback("session-1"));
  await client.stopPlayback("session-1", { disposition: "stopped", positionSeconds: 12, durationSeconds: 60 });
  assert.equal(calls[0].input, "https://server.example/api/playback-sessions/session-1");
  assert.equal(calls[0].init.method, "DELETE");
  assert.equal(calls[0].init.headers["Content-Type"], "application/json");
  assert.equal(calls[0].init.headers["X-Portico-CSRF"], "1");
  const stopped = JSON.parse(calls[0].init.body);
  assert.match(stopped.requestId, /^[A-Za-z0-9._:-]{8,128}$/);
  assert.deepEqual({ ...stopped.terminal, recordedAt: "<timestamp>" }, {
    disposition: "stopped", generation: 1, eventSequence: 1,
    recordedAt: "<timestamp>", positionSeconds: 12, durationSeconds: 60,
  });
  assert.match(stopped.terminal.recordedAt, /^\d{4}-\d{2}-\d{2}T/);
});

test("continuation revoke requires and sends the canonical atomic terminal body", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: { fetch: async (input, init) => {
      calls.push({ input: String(input), init });
      const request = JSON.parse(init.body);
      return jsonResponse(playbackTerminalReceipt("session-1", request));
    } }
  });
  const credential = { token: "continuation", origin: "https://server.example", generation: 1, expiresAt: "2026-08-28T20:00:00Z" };
  const body = {
    requestId: "terminal-native-1",
    terminal: {
      disposition: "stopped", generation: 1, eventSequence: 7,
      recordedAt: "2026-08-28T18:00:00.000Z", positionSeconds: 18, durationSeconds: 60,
    },
  };

  await client.revokePlaybackContinuation("session-1", credential, body);

  assert.equal(calls[0].input, "https://server.example/api/playback-sessions/session-1/continuation");
  assert.equal(calls[0].init.method, "DELETE");
  assert.deepEqual(JSON.parse(calls[0].init.body), body);
});

test("base stop forwards a native-owned complete request without reallocating authority", async () => {
  const calls = [];
  let generatedRequestIds = 0;
  const body = {
    requestId: "terminal-native-base-1",
    terminal: {
      disposition: "completed", generation: 4, eventSequence: 29,
      recordedAt: "2026-08-30T18:20:00.000Z", positionSeconds: 180, durationSeconds: 180,
    },
  };
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    requestId: () => {
      generatedRequestIds += 1;
      return `transport-request-${generatedRequestIds}`;
    },
    transport: { fetch: async (_input, init) => {
      calls.push(String(init.body));
      return jsonResponse(playbackTerminalReceipt("session-native-base", body));
    } },
  });

  const receipt = await client.stopPlayback("session-native-base", body);

  assert.deepEqual(JSON.parse(calls[0]), body);
  assert.equal(receipt.requestId, body.requestId);
  assert.equal(generatedRequestIds, 1);
});

test("Core-owned terminal retry reuses one request ID and byte-identical event after a lost response", async () => {
  const calls = [];
  let requestIds = 0;
  const records = new Map();
  const durability = {
    load: async () => [...records.values()].map(value => structuredClone(value)),
    save: async record => { records.set(record.key, structuredClone(record)); },
    remove: async key => { records.delete(key); },
  };
  const first = createPorticoClient({
    apiBaseUrl: "https://server.example",
    playbackProgressDurabilityAdapter: durability,
    requestId: () => {
      requestIds += 1;
      return `terminal-request-${requestIds}`;
    },
    now: () => Date.parse("2026-08-30T18:20:00.000Z"),
    transport: { fetch: async (_input, init) => {
      const serialized = String(init.body);
      calls.push(serialized);
      throw new TypeError("response was lost");
    } },
  });
  first.acceptPlaybackSession(acceptedPlayback("session-retry-stop", 7));

  const input = { disposition: "completed", positionSeconds: 60, durationSeconds: 60 };
  await assert.rejects(first.stopPlayback("session-retry-stop", input), /response was lost/);
  const restarted = createPorticoClient({
    apiBaseUrl: "https://server.example",
    playbackProgressDurabilityAdapter: durability,
    requestId: () => {
      requestIds += 1;
      return `terminal-request-${requestIds}`;
    },
    transport: { fetch: async (_input, init) => {
      const serialized = String(init.body);
      calls.push(serialized);
      const request = JSON.parse(serialized);
      return jsonResponse(playbackTerminalReceipt("session-retry-stop", request, true));
    } },
  });
  const pending = await restarted.pendingPlaybackTerminalMutation("session-retry-stop");
  assert.equal(pending.operation, "stop");
  assert.equal(pending.request.requestId, "terminal-request-1");
  assert.equal(Object.isFrozen(pending.request), true);
  assert.equal(Object.isFrozen(pending.request.terminal), true);
  const pendingOutbox = await restarted.pendingPlaybackTerminalMutations();
  assert.equal(Object.isFrozen(pendingOutbox), true);
  assert.deepEqual(
    pendingOutbox.map(record => [record.sessionId, record.operation]),
    [["session-retry-stop", "stop"]],
  );
  const receipt = await restarted.stopPlayback(
    "session-retry-stop",
    pending.request,
  );

  assert.equal(calls.length, 2);
  assert.equal(calls[0], calls[1]);
  assert.equal(receipt.duplicate, true);
  assert.equal(receipt.requestId, "terminal-request-1");
});

test("a 401 or 404 is never accepted as a lost terminal response receipt", async () => {
  const records = new Map();
  const durability = {
    load: async () => [...records.values()].map(value => structuredClone(value)),
    save: async record => { records.set(record.key, structuredClone(record)); },
    remove: async key => { records.delete(key); },
  };
  const first = createPorticoClient({
    apiBaseUrl: "https://server.example",
    playbackProgressDurabilityAdapter: durability,
    transport: { fetch: async () => { throw new TypeError("response was lost"); } },
  });
  first.acceptPlaybackSession(acceptedPlayback("session-unreconciled-stop", 3));
  await assert.rejects(first.stopPlayback("session-unreconciled-stop", {
    disposition: "stopped", positionSeconds: 20, durationSeconds: 60,
  }), /response was lost/);

  const restarted = createPorticoClient({
    apiBaseUrl: "https://server.example",
    playbackProgressDurabilityAdapter: durability,
    transport: { fetch: async () => jsonResponse({
      code: "playback_session_not_found",
      detail: "The playback session was not found.",
    }, { status: 404 }) },
  });
  const pending = await restarted.pendingPlaybackTerminalMutation(
    "session-unreconciled-stop",
  );
  await assert.rejects(
    restarted.stopPlayback("session-unreconciled-stop", pending.request),
    error => error instanceof ApiError && error.status === 404,
  );

  assert.equal(
    (await restarted.pendingPlaybackTerminalMutation("session-unreconciled-stop"))
      .request.requestId,
    pending.request.requestId,
  );
  await assert.rejects(
    restarted.touchPlayback("session-unreconciled-stop", { positionSeconds: 21 }),
    error => error instanceof ApiError && error.code === "playback_stopping",
  );
});

test("handoff allocates a Core-owned terminal once and retries the same request body", async () => {
  const calls = [];
  let attempts = 0;
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    now: () => Date.parse("2026-08-30T18:20:00.000Z"),
    transport: { fetch: async (_input, init) => {
      const serialized = String(init.body);
      calls.push(serialized);
      attempts += 1;
      if (attempts === 1) throw new TypeError("handoff response was lost");
      return jsonResponse(acceptedPlayback("session-next"));
    } },
  });
  client.acceptPlaybackSession(acceptedPlayback("session-handoff", 11));
  const request = {
    requestId: "handoff-request-1",
    entryId: "queue-entry-2",
    startSeconds: 0,
    previousTerminal: {
      disposition: "stopped",
      positionSeconds: 42,
      durationSeconds: 60,
    },
  };

  await assert.rejects(client.handoffPlayback("session-handoff", request), /handoff response was lost/);
  await assert.rejects(
    client.touchPlayback("session-handoff", { positionSeconds: 43 }),
    error => error instanceof ApiError && error.code === "playback_stopping",
  );
  await assert.rejects(
    client.renegotiatePlayback("session-handoff", {}),
    error => error instanceof ApiError && error.code === "playback_stopping",
  );
  await assert.rejects(
    client.mutatePlaybackSessionQueue("session-handoff", {
      operation: "shuffle",
      expectedQueueRevision: 1,
      requestId: "queue-blocked-1",
    }),
    error => error instanceof ApiError && error.code === "playback_stopping",
  );
  assert.equal(calls.length, 1);
  await client.handoffPlayback("session-handoff", request);

  assert.equal(calls.length, 2);
  assert.equal(calls[0], calls[1]);
  const body = JSON.parse(calls[0]);
  assert.equal(body.startSeconds, 0);
  assert.deepEqual(body.previousTerminal, {
    disposition: "stopped",
    generation: 1,
    eventSequence: 11,
    recordedAt: "2026-08-30T18:20:00.000Z",
    positionSeconds: 42,
    durationSeconds: 60,
  });
});

for (const inProgressCode of [
  "handoff_in_progress",
  "prepared_handoff_in_progress",
]) test(`${inProgressCode} preserves the exact durable request and source fence`, async () => {
  const calls = [];
  const records = new Map();
  const durability = {
    load: async () => [...records.values()].map(value => structuredClone(value)),
    save: async record => { records.set(record.key, structuredClone(record)); },
    remove: async key => { records.delete(key); },
  };
  let attempts = 0;
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    playbackProgressDurabilityAdapter: durability,
    transport: { fetch: async (_input, init) => {
      calls.push(String(init.body));
      attempts += 1;
      if (attempts === 1) {
        return jsonResponse({
          code: inProgressCode,
          detail: "The same handoff is still resolving.",
        }, { status: 409 });
      }
      return jsonResponse(acceptedPlayback("session-handoff-reconciled"));
    } },
  });
  client.acceptPlaybackSession(acceptedPlayback("session-handoff-in-progress", 5));
  const request = {
    requestId: "handoff-in-progress-1",
    entryId: "queue-entry-2",
    previousTerminal: {
      disposition: "stopped",
      positionSeconds: 20,
      durationSeconds: 60,
    },
  };

  await assert.rejects(
    client.handoffPlayback("session-handoff-in-progress", request),
    error => error instanceof ApiError && error.code === inProgressCode,
  );
  assert.equal(
    (await client.pendingPlaybackTerminalMutation("session-handoff-in-progress"))
      .request.requestId,
    request.requestId,
  );
  await assert.rejects(
    client.mutatePlaybackSessionQueue("session-handoff-in-progress", {
      operation: "shuffle", expectedQueueRevision: 1, requestId: "blocked-queue-1",
    }),
    error => error instanceof ApiError && error.code === "playback_stopping",
  );
  await client.handoffPlayback("session-handoff-in-progress", request);

  assert.equal(calls.length, 2);
  assert.equal(calls[0], calls[1]);
  assert.equal(records.size, 0);
});

test("a restarted Core exposes and exact-replays the durable pending handoff", async () => {
  const records = new Map();
  const durability = {
    load: async () => [...records.values()].map(value => structuredClone(value)),
    save: async record => { records.set(record.key, structuredClone(record)); },
    remove: async key => { records.delete(key); },
  };
  const firstBodies = [];
  const first = createPorticoClient({
    apiBaseUrl: "https://server.example",
    playbackProgressDurabilityAdapter: durability,
    now: () => Date.parse("2026-08-30T18:20:00.000Z"),
    transport: { fetch: async (_input, init) => {
      firstBodies.push(String(init.body));
      throw new TypeError("handoff response was lost");
    } },
  });
  first.acceptPlaybackSession(acceptedPlayback("session-restart-handoff", 11));
  await assert.rejects(first.handoffPlayback("session-restart-handoff", {
    requestId: "handoff-restart-1",
    entryId: "queue-entry-2",
    previousTerminal: {
      disposition: "completed",
      positionSeconds: 60,
      durationSeconds: 60,
    },
  }), /handoff response was lost/);

  const replayBodies = [];
  const restarted = createPorticoClient({
    apiBaseUrl: "https://server.example",
    playbackProgressDurabilityAdapter: durability,
    transport: { fetch: async (_input, init) => {
      replayBodies.push(String(init.body));
      return jsonResponse(acceptedPlayback("session-restart-next"));
    } },
  });
  const pending = await restarted.pendingPlaybackTerminalMutation("session-restart-handoff");
  assert.equal(pending.operation, "handoff");
  assert.equal(Object.isFrozen(pending.request), true);
  assert.equal(Object.isFrozen(pending.request.previousTerminal), true);
  const pendingOutbox = await restarted.pendingPlaybackTerminalMutations();
  assert.equal(pendingOutbox[0].sessionId, "session-restart-handoff");
  assert.equal(pendingOutbox[0].request.requestId, "handoff-restart-1");
  await restarted.handoffPlayback("session-restart-handoff", pending.request);

  assert.equal(replayBodies[0], firstBodies[0]);
  assert.equal(records.size, 0);
});

for (const [isolation, foreignSession] of [
  ["server", {
    serverId: "server-b", authority: "hosted",
    accountId: "account-a", profileId: "profile-a",
    authorizationRevision: "authorization-1",
  }],
  ["viewer", {
    serverId: "server-a", authority: "hosted",
    accountId: "account-a", profileId: "profile-b",
    authorizationRevision: "authorization-1",
  }],
  ["authorization revision", {
    serverId: "server-a", authority: "hosted",
    accountId: "account-a", profileId: "profile-a",
    authorizationRevision: "authorization-2",
  }],
]) test(`durable terminal recovery never crosses ${isolation} authority`, async () => {
  const records = new Map();
  const durability = {
    load: async () => [...records.values()].map(value => structuredClone(value)),
    save: async record => { records.set(record.key, structuredClone(record)); },
    remove: async key => { records.delete(key); },
  };
  const sourceSession = {
    serverId: "server-a", authority: "hosted",
    accountId: "account-a", profileId: "profile-a",
    authorizationRevision: "authorization-1",
  };
  const first = createPorticoClient({
    apiBaseUrl: "https://server.example",
    sessionStore: createMemorySessionStore(sourceSession),
    playbackProgressDurabilityAdapter: durability,
    transport: { fetch: async () => { throw new TypeError("response was lost"); } },
  });
  first.acceptPlaybackSession(acceptedPlayback("session-scoped-terminal", 4));
  await assert.rejects(first.stopPlayback("session-scoped-terminal", {
    disposition: "stopped", positionSeconds: 20, durationSeconds: 60,
  }), /response was lost/);

  const [stored] = [...records.values()];
  assert.equal(stored.version, "v2");
  assert.deepEqual(stored.scope, [
    "server-a", "hosted", "account-a", "profile-a", "authorization-1",
  ]);
  const foreign = createPorticoClient({
    apiBaseUrl: "https://server.example",
    sessionStore: createMemorySessionStore(foreignSession),
    playbackProgressDurabilityAdapter: durability,
  });
  assert.equal(
    await foreign.pendingPlaybackTerminalMutation("session-scoped-terminal"),
    undefined,
  );
  assert.deepEqual(await foreign.pendingPlaybackTerminalMutations(), []);
  assert.equal(records.size, 1);

  const restartedSource = createPorticoClient({
    apiBaseUrl: "https://server.example",
    sessionStore: createMemorySessionStore(sourceSession),
    playbackProgressDurabilityAdapter: durability,
  });
  assert.equal(
    (await restartedSource.pendingPlaybackTerminalMutation(
      "session-scoped-terminal",
    )).operation,
    "stop",
  );
});

test("durable terminal recovery follows an exact authority switch in the same Core instance", async () => {
  const records = new Map();
  const durability = {
    load: async () => [...records.values()].map(value => structuredClone(value)),
    save: async record => { records.set(record.key, structuredClone(record)); },
    remove: async key => { records.delete(key); },
  };
  const sourceSession = {
    serverId: "server-a", authority: "hosted",
    accountId: "account-a", profileId: "profile-a",
    authorizationRevision: "authorization-1",
  };
  const first = createPorticoClient({
    apiBaseUrl: "https://server.example",
    sessionStore: createMemorySessionStore(sourceSession),
    playbackProgressDurabilityAdapter: durability,
    transport: { fetch: async () => { throw new TypeError("response was lost"); } },
  });
  first.acceptPlaybackSession(acceptedPlayback("session-scope-switch", 4));
  await assert.rejects(first.stopPlayback("session-scope-switch", {
    disposition: "stopped", positionSeconds: 20, durationSeconds: 60,
  }), /response was lost/);

  const sessionStore = createMemorySessionStore({
    ...sourceSession,
    profileId: "profile-b",
  });
  const resumed = createPorticoClient({
    apiBaseUrl: "https://server.example",
    sessionStore,
    playbackProgressDurabilityAdapter: durability,
  });
  assert.equal(
    await resumed.pendingPlaybackTerminalMutation("session-scope-switch"),
    undefined,
  );

  sessionStore.set({...testServerIdentity(), ...sourceSession});
  assert.equal(
    (await resumed.pendingPlaybackTerminalMutation("session-scope-switch"))
      .request.requestId,
    [...records.values()][0].request.requestId,
  );
});

test("unscoped prerelease terminal records are purged instead of replayed under guessed authority", async () => {
  const records = new Map([["terminal:stop:legacy-session", {
    version: "v1",
    kind: "terminal",
    key: "terminal:stop:legacy-session",
    sessionId: "legacy-session",
    operation: "stop",
    request: {
      requestId: "legacy-request-1",
      terminal: {
        disposition: "stopped", generation: 1, eventSequence: 2,
        recordedAt: "2026-08-30T18:20:00.000Z",
        positionSeconds: 20, durationSeconds: 60,
      },
    },
    updatedAt: "2026-08-30T18:20:00.000Z",
  }]]);
  const durability = {
    load: async () => [...records.values()].map(value => structuredClone(value)),
    save: async record => { records.set(record.key, structuredClone(record)); },
    remove: async key => { records.delete(key); },
  };
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    playbackProgressDurabilityAdapter: durability,
  });

  assert.deepEqual(await client.pendingPlaybackTerminalMutations(), []);
  assert.equal(records.size, 0);
});

test("four-part prerelease terminal scopes are purged instead of crossing authorization revisions", async () => {
  const request = {
    requestId: "legacy-scoped-request-1",
    terminal: {
      disposition: "stopped", generation: 1, eventSequence: 2,
      recordedAt: "2026-08-30T18:20:00.000Z",
      positionSeconds: 20, durationSeconds: 60,
    },
  };
  const scope = ["server-a", "hosted", "account-a", "profile-a"];
  const key = JSON.stringify([
    ...scope, "terminal", "stop", "legacy-scoped-session", request.requestId,
  ]);
  const records = new Map([[key, {
    version: "v2", kind: "terminal", key, scope,
    sessionId: "legacy-scoped-session", operation: "stop", request,
    updatedAt: "2026-08-30T18:20:00.000Z",
  }]]);
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    sessionStore: createMemorySessionStore({
      serverId: "server-a", authority: "hosted", accountId: "account-a",
      profileId: "profile-a", authorizationRevision: "authorization-2",
    }),
    playbackProgressDurabilityAdapter: {
      load: async () => [...records.values()].map(value => structuredClone(value)),
      save: async record => { records.set(record.key, structuredClone(record)); },
      remove: async recordKey => { records.delete(recordKey); },
    },
  });

  assert.deepEqual(await client.pendingPlaybackTerminalMutations(), []);
  assert.equal(records.size, 0);
});

test("prerelease progress identities without authorization revision are purged", async () => {
  const key = JSON.stringify([
    "server-a", "hosted", "account-a", "profile-a", "legacy-progress", 1,
  ]);
  const records = new Map([[key, {
    version: "v1", kind: "progress", key,
    events: [{
      state: "playing", positionSeconds: 12, eventSequence: 1,
      recordedAt: "2026-08-30T18:20:00.000Z",
    }],
    updatedAt: "2026-08-30T18:20:00.000Z",
  }]]);
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    sessionStore: createMemorySessionStore({
      serverId: "server-a", authority: "hosted", accountId: "account-a",
      profileId: "profile-a", authorizationRevision: "authorization-2",
    }),
    playbackProgressDurabilityAdapter: {
      load: async () => [...records.values()].map(value => structuredClone(value)),
      save: async record => { records.set(record.key, structuredClone(record)); },
      remove: async recordKey => { records.delete(recordKey); },
    },
  });

  assert.deepEqual(await client.pendingPlaybackTerminalMutations(), []);
  assert.equal(records.size, 0);
});

test("handoff forwards a native-owned terminal event without resequencing", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: { fetch: async (_input, init) => {
      calls.push(JSON.parse(init.body));
      return jsonResponse(acceptedPlayback("session-native-next"));
    } },
  });
  const previousTerminal = {
    disposition: "completed",
    generation: 4,
    eventSequence: 29,
    recordedAt: "2026-08-30T18:20:00.000Z",
    positionSeconds: 180,
    durationSeconds: 180,
  };

  await client.handoffPlayback("session-native-source", {
    requestId: "handoff-native-1",
    preparedSessionId: "prepared-1",
    entryId: "queue-entry-2",
    previousTerminal,
  });

  assert.deepEqual(calls[0].previousTerminal, previousTerminal);
});

test("Core-owned target replacement allocates one durable terminal and seeds the accepted actor", async () => {
  const calls = [];
  const records = new Map();
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    now: () => Date.parse("2026-08-30T18:20:00.000Z"),
    requestId: () => "replacement-request-1",
    playbackProgressDurabilityAdapter: {
      load: async () => [...records.values()].map(value => structuredClone(value)),
      save: async record => { records.set(record.key, structuredClone(record)); },
      remove: async key => { records.delete(key); },
    },
    transport: { fetch: async (input, init) => {
      const body = JSON.parse(init.body);
      calls.push({ input: String(input), method: init.method, body });
      if (init.method === "PATCH") {
        return jsonResponse({
          accepted: true, duplicate: false, stale: false,
          highestEventSequence: body.eventSequence, sessionState: "playing",
        });
      }
      return jsonResponse(acceptedPlayback("session-target", 17));
    } },
  });
  client.acceptPlaybackSession(acceptedPlayback("session-source", 8));

  const outcome = await client.replacePlaybackTarget({
    kind: "media",
    mediaId: "media-target",
    playbackOptions: { startSeconds: 12 },
  }, {
    sourceSessionId: "session-source",
    expectedQueueRevision: 4,
    expectedPlaybackRevision: 6,
    previousTerminal: {
      disposition: "stopped", positionSeconds: 31, durationSeconds: 60,
    },
  });

  assert.equal(outcome.outcome, "accepted");
  assert.equal(outcome.value.sessionId, "session-target");
  assert.equal(calls[0].input, "https://server.example/api/playback-sessions");
  assert.equal(calls[0].method, "POST");
  assert.equal(calls[0].body.mediaId, "media-target");
  assert.equal(calls[0].body.startSeconds, 12);
  assert.deepEqual(calls[0].body.replacement, {
    sourceSessionId: "session-source",
    requestId: "replacement-request-1",
    previousTerminal: {
      disposition: "stopped", generation: 1, eventSequence: 8,
      recordedAt: "2026-08-30T18:20:00.000Z",
      positionSeconds: 31, durationSeconds: 60,
    },
    expectedQueueRevision: 4,
    expectedPlaybackRevision: 6,
  });
  assert.equal(records.size, 0);

  await client.touchPlayback("session-target", { positionSeconds: 2 });
  assert.equal(calls[1].method, "PATCH");
  assert.equal(calls[1].body.eventSequence, 17);
});

test("target replacement fences source progress before terminal persistence", async () => {
  const calls = [];
  const records = new Map();
  let releaseTerminalSave;
  let markTerminalSaveStarted;
  const terminalSaveGate = new Promise(resolve => { releaseTerminalSave = resolve; });
  const terminalSaveStarted = new Promise(resolve => { markTerminalSaveStarted = resolve; });
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    requestId: () => "replacement-fenced-1",
    playbackProgressDurabilityAdapter: {
      load: async () => [...records.values()].map(value => structuredClone(value)),
      save: async record => {
        if (record.kind === "terminal") {
          markTerminalSaveStarted();
          await terminalSaveGate;
        }
        records.set(record.key, structuredClone(record));
      },
      remove: async key => { records.delete(key); },
    },
    transport: { fetch: async (_input, init) => {
      calls.push(init.method);
      return jsonResponse(acceptedPlayback("session-fenced-target"));
    } },
  });
  client.acceptPlaybackSession(acceptedPlayback("session-fenced-source", 4));
  const replacement = client.replacePlaybackTarget({
    kind: "media", mediaId: "media-fenced-target",
  }, {
    sourceSessionId: "session-fenced-source",
    previousTerminal: {
      disposition: "stopped", positionSeconds: 20, durationSeconds: 60,
    },
  });
  await terminalSaveStarted;

  await assert.rejects(
    client.touchPlayback("session-fenced-source", { positionSeconds: 21 }),
    error => error instanceof ApiError && error.code === "playback_stopping",
  );
  assert.deepEqual(calls, []);
  releaseTerminalSave();
  assert.equal((await replacement).outcome, "accepted");
  assert.deepEqual(calls, ["POST"]);
  assert.equal(records.size, 0);
});

test("a lost target replacement response is exact-replayed from the durable Core outbox", async () => {
  const records = new Map();
  const durability = {
    load: async () => [...records.values()].map(value => structuredClone(value)),
    save: async record => { records.set(record.key, structuredClone(record)); },
    remove: async key => { records.delete(key); },
  };
  const bodies = [];
  const session = {
    serverId: "server-a", authority: "hosted", accountId: "account-a",
    profileId: "profile-a", authorizationRevision: "authorization-1",
  };
  const first = createPorticoClient({
    apiBaseUrl: "https://server.example",
    sessionStore: createMemorySessionStore(session),
    requestId: () => "replacement-retry-1",
    playbackProgressDurabilityAdapter: durability,
    transport: { fetch: async (_input, init) => {
      bodies.push(String(init.body));
      throw new TypeError("replacement response was lost");
    } },
  });
  first.acceptPlaybackSession(acceptedPlayback("session-replacement-retry", 3));
  await assert.rejects(first.replacePlaybackTarget({
    kind: "live-tv-stream", channelId: "channel-1",
  }, {
    sourceSessionId: "session-replacement-retry",
    previousTerminal: {
      disposition: "stopped", positionSeconds: 18, durationSeconds: 60,
    },
  }), /replacement response was lost/);
  const [stored] = [...records.values()];
  assert.equal(stored.operation, "replacement");
  assert.equal(stored.scope[4], "authorization-1");
  assert.equal(stored.target.kind, "live-tv-stream");

  const restarted = createPorticoClient({
    apiBaseUrl: "https://server.example",
    sessionStore: createMemorySessionStore(session),
    playbackProgressDurabilityAdapter: durability,
    transport: { fetch: async (input, init) => {
      assert.equal(String(input), "https://server.example/api/live-tv/streams/channel-1/open");
      bodies.push(String(init.body));
      return jsonResponse(acceptedPlayback("session-replacement-retried"));
    } },
  });
  const pending = await restarted.pendingPlaybackTerminalMutation(
    "session-replacement-retry",
  );
  assert.equal(pending.operation, "replacement");
  assert.equal(Object.isFrozen(pending.request), true);
  assert.equal(Object.isFrozen(pending.target.body), true);
  const outcome = await restarted.retryPendingPlaybackTerminalMutation(
    "session-replacement-retry",
  );

  assert.equal(outcome.outcome, "accepted");
  assert.equal(outcome.value.sessionId, "session-replacement-retried");
  assert.equal(bodies.length, 2);
  assert.equal(bodies[0], bodies[1]);
  assert.equal(records.size, 0);
});

for (const [status, code] of [
  [401, "viewer_authorization_required"],
  [404, "playback_session_not_found"],
  [409, "handoff_in_progress"],
]) test(`target replacement ${status} ${code} preserves its exact request and source fence`, async () => {
  const records = new Map();
  const calls = [];
  let attempts = 0;
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    requestId: () => "replacement-unresolved-1",
    playbackProgressDurabilityAdapter: {
      load: async () => [...records.values()].map(value => structuredClone(value)),
      save: async record => { records.set(record.key, structuredClone(record)); },
      remove: async key => { records.delete(key); },
    },
    transport: { fetch: async (_input, init) => {
      calls.push(String(init.body));
      attempts += 1;
      if (attempts === 1) {
        return jsonResponse({ code, detail: "The outcome remains unresolved." }, { status });
      }
      return jsonResponse(acceptedPlayback("session-unresolved-reconciled"));
    } },
  });
  client.acceptPlaybackSession(acceptedPlayback("session-unresolved-source", 5));
  await assert.rejects(client.replacePlaybackTarget({
    kind: "media", mediaId: "media-unresolved-target",
  }, {
    sourceSessionId: "session-unresolved-source",
    previousTerminal: {
      disposition: "stopped", positionSeconds: 20, durationSeconds: 60,
    },
  }), error => error instanceof ApiError && error.code === code);

  assert.equal(records.size, 1);
  assert.equal(
    (await client.pendingPlaybackTerminalMutation("session-unresolved-source"))
      .operation,
    "replacement",
  );
  await assert.rejects(
    client.touchPlayback("session-unresolved-source", { positionSeconds: 21 }),
    error => error instanceof ApiError && error.code === "playback_stopping",
  );
  const outcome = await client.retryPendingPlaybackTerminalMutation(
    "session-unresolved-source",
  );

  assert.equal(outcome.outcome, "accepted");
  assert.equal(calls.length, 2);
  assert.equal(calls[0], calls[1]);
  assert.equal(records.size, 0);
});

test("a definitive deliberate replacement rejection retains source authority", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    requestId: () => "replacement-retained-1",
    transport: { fetch: async (_input, init) => {
      const body = JSON.parse(init.body);
      calls.push({ method: init.method, body });
      if (init.method === "POST") {
        return jsonResponse({
          code: "handoff_source_revision_conflict",
          detail: "The source revision changed.",
        }, { status: 409 });
      }
      return jsonResponse({
        accepted: true, duplicate: false, stale: false,
        highestEventSequence: body.eventSequence, sessionState: "playing",
      });
    } },
  });
  client.acceptPlaybackSession(acceptedPlayback("session-retained", 4));
  const outcome = await client.replacePlaybackTarget({
    kind: "dvr", recordingId: "recording-1",
  }, {
    sourceSessionId: "session-retained",
    previousTerminal: {
      disposition: "stopped", positionSeconds: 20, durationSeconds: 60,
    },
  });

  assert.deepEqual(outcome, {
    outcome: "source-retained",
    sourceSessionId: "session-retained",
    rejection: {
      status: 409,
      code: "handoff_source_revision_conflict",
      detail: "The source revision changed.",
    },
  });
  await client.touchPlayback("session-retained", { positionSeconds: 21 });
  assert.deepEqual(calls.map(call => call.method), ["POST", "PATCH"]);
});

for (const disposition of ["stopped", "completed"]) test(
  `replacement_source_inactive is canonical for a ${disposition} terminal`,
  async () => {
    const records = new Map();
    const calls = [];
    const client = createPorticoClient({
      apiBaseUrl: "https://server.example",
      requestId: () => `replacement-inactive-${disposition}`,
      playbackProgressDurabilityAdapter: {
        load: async () => [...records.values()].map(value => structuredClone(value)),
        save: async record => { records.set(record.key, structuredClone(record)); },
        remove: async key => { records.delete(key); },
      },
      transport: { fetch: async (_input, init) => {
        calls.push(init.method);
        return jsonResponse({
          code: "replacement_source_inactive",
          detail: "The source playback is already inactive.",
        }, { status: 409 });
      } },
    });
    client.acceptPlaybackSession(acceptedPlayback(`session-inactive-${disposition}`, 4));
    const outcome = await client.replacePlaybackTarget({
      kind: "media", mediaId: "media-inactive-target",
    }, {
      sourceSessionId: `session-inactive-${disposition}`,
      previousTerminal: {
        disposition, positionSeconds: 60, durationSeconds: 60,
      },
    });

    assert.deepEqual(outcome, {
      outcome: "source-inactive",
      sourceSessionId: `session-inactive-${disposition}`,
      rejection: {
        status: 409,
        code: "replacement_source_inactive",
        detail: "The source playback is already inactive.",
      },
    });
    assert.deepEqual(calls, ["POST"], "completed does not issue fallback DELETE");
    assert.equal(records.size, 0);
    assert.equal(
      await client.pendingPlaybackTerminalMutation(`session-inactive-${disposition}`),
      undefined,
    );
  },
);

test("a definitive natural replacement rejection closes with the exact terminal under a new stop request", async () => {
  const calls = [];
  let requestIds = 0;
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    now: () => Date.parse("2026-08-30T18:20:00.000Z"),
    requestId: () => `replacement-natural-${++requestIds}`,
    transport: { fetch: async (_input, init) => {
      const body = JSON.parse(init.body);
      calls.push({ method: init.method, body });
      if (init.method === "POST") {
        return jsonResponse({
          code: "handoff_source_revision_conflict",
          detail: "The queue changed.",
        }, { status: 409 });
      }
      return jsonResponse(playbackTerminalReceipt("session-natural-source", body));
    } },
  });
  client.acceptPlaybackSession(acceptedPlayback("session-natural-source", 9));
  const outcome = await client.replacePlaybackTarget({
    kind: "live-tv", channelId: "channel-1",
  }, {
    sourceSessionId: "session-natural-source",
    previousTerminal: {
      disposition: "completed", positionSeconds: 60, durationSeconds: 60,
    },
  });

  assert.equal(outcome.outcome, "source-closed");
  assert.deepEqual(calls.map(call => call.method), ["POST", "DELETE"]);
  assert.equal(calls[0].body.replacement.requestId, "replacement-natural-1");
  assert.notEqual(
    calls[1].body.requestId,
    calls[0].body.replacement.requestId,
  );
  assert.deepEqual(
    calls[1].body.terminal,
    calls[0].body.replacement.previousTerminal,
  );
});

test("committed replacement restore-required remains durable until exact restore", async () => {
  const records = new Map();
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    requestId: () => "replacement-restore-1",
    playbackProgressDurabilityAdapter: {
      load: async () => [...records.values()].map(value => structuredClone(value)),
      save: async record => { records.set(record.key, structuredClone(record)); },
      remove: async key => { records.delete(key); },
    },
    transport: { fetch: async input => {
      if (new URL(input).pathname === "/api/playback/active") {
        return jsonResponse({
          active: true,
          playback: acceptedPlayback("session-safe-replacement", 8),
        });
      }
      return jsonResponse({
        code: "playback_replacement_committed_restore_required",
        detail: "The replacement committed; restore active playback.",
        replacementSessionId: "session-safe-replacement",
      }, { status: 409 });
    } },
  });
  client.acceptPlaybackSession(acceptedPlayback("session-restore-source", 2));
  const outcome = await client.replacePlaybackTarget({
    kind: "library-channel", channelId: "library-channel-1",
  }, {
    sourceSessionId: "session-restore-source",
    previousTerminal: {
      disposition: "stopped", positionSeconds: 12, durationSeconds: 60,
    },
  });

  assert.deepEqual(outcome, {
    outcome: "committed-restore-required",
    sourceSessionId: "session-restore-source",
    replacementSessionId: "session-safe-replacement",
  });
  assert.equal(records.size, 1);
  assert.deepEqual(
    (await client.pendingPlaybackTerminalMutation("session-restore-source"))
      .committedOutcome,
    outcome,
  );
  const restored = await client.restoreCommittedPlaybackReplacement(outcome);
  assert.equal(restored.sessionId, "session-safe-replacement");
  assert.equal(records.size, 0);
  assert.equal(
    await client.pendingPlaybackTerminalMutation("session-restore-source"),
    undefined,
  );
  await assert.rejects(
    client.restoreCommittedPlaybackReplacement({
      ...outcome,
      replacementSessionId: "different-session",
    }),
    error => error instanceof ApiError &&
      error.code === "playback_replacement_restore_mismatch",
  );
});

test("a lost committed restore response survives restart without resending the target", async () => {
  const records = new Map();
  const durability = {
    load: async () => [...records.values()].map(value => structuredClone(value)),
    save: async record => { records.set(record.key, structuredClone(record)); },
    remove: async key => { records.delete(key); },
  };
  const session = {
    serverId: "server-a", authority: "hosted", accountId: "account-a",
    profileId: "profile-a", authorizationRevision: "authorization-1",
  };
  const first = createPorticoClient({
    apiBaseUrl: "https://server.example",
    sessionStore: createMemorySessionStore(session),
    requestId: () => "replacement-committed-restart-1",
    playbackProgressDurabilityAdapter: durability,
    transport: { fetch: async input => {
      if (new URL(input).pathname === "/api/playback/active") {
        throw new TypeError("restore response was lost");
      }
      return jsonResponse({
        code: "playback_replacement_committed_restore_required",
        detail: "The replacement committed; restore active playback.",
        replacementSessionId: "session-committed-target",
      }, { status: 409 });
    } },
  });
  first.acceptPlaybackSession(acceptedPlayback("session-committed-source", 2));
  const outcome = await first.replacePlaybackTarget({
    kind: "media", mediaId: "media-committed-target",
  }, {
    sourceSessionId: "session-committed-source",
    previousTerminal: {
      disposition: "stopped", positionSeconds: 12, durationSeconds: 60,
    },
  });
  await assert.rejects(
    first.restoreCommittedPlaybackReplacement(outcome),
    /restore response was lost/,
  );
  assert.equal(records.size, 1);

  let activeRestores = 0;
  let targetRequests = 0;
  const restarted = createPorticoClient({
    apiBaseUrl: "https://server.example",
    sessionStore: createMemorySessionStore(session),
    playbackProgressDurabilityAdapter: durability,
    transport: { fetch: async input => {
      const path = new URL(input).pathname;
      if (path !== "/api/playback/active") {
        targetRequests += 1;
        throw new Error(`unexpected target resend: ${path}`);
      }
      activeRestores += 1;
      return jsonResponse({
        active: true,
        playback: acceptedPlayback(
          activeRestores === 1
            ? "different-active-session"
            : "session-committed-target",
          8,
        ),
      });
    } },
  });
  const pending = await restarted.pendingPlaybackTerminalMutation(
    "session-committed-source",
  );
  assert.deepEqual(pending.committedOutcome, outcome);
  assert.deepEqual(
    await restarted.retryPendingPlaybackTerminalMutation(
      "session-committed-source",
    ),
    outcome,
  );
  assert.equal(targetRequests, 0);
  await assert.rejects(
    restarted.restoreCommittedPlaybackReplacement(outcome),
    error => error instanceof ApiError &&
      error.code === "playback_replacement_restore_mismatch",
  );
  assert.equal(records.size, 1, "mismatched restore retains exact durable identity");
  const restored = await restarted.restoreCommittedPlaybackReplacement(outcome);
  assert.equal(restored.sessionId, "session-committed-target");
  assert.equal(records.size, 0, "exact restore removes the committed durable identity");
  assert.equal(targetRequests, 0);
});

test("a native-owned target replacement is forwarded without competing allocation", async () => {
  const calls = [];
  let generated = 0;
  const previousTerminal = {
    disposition: "completed",
    generation: 4,
    eventSequence: 29,
    recordedAt: "2026-08-30T18:20:00.000Z",
    positionSeconds: 180,
    durationSeconds: 180,
  };
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    requestId: () => { generated += 1; return "must-not-generate"; },
    transport: { fetch: async (_input, init) => {
      calls.push(JSON.parse(init.body));
      return jsonResponse(acceptedPlayback("session-native-replacement"));
    } },
  });
  const outcome = await client.replacePlaybackTarget({
    kind: "media", mediaId: "media-native-target",
  }, {
    sourceSessionId: "session-native-source",
    requestId: "native-replacement-request-1",
    previousTerminal,
  });

  assert.equal(outcome.outcome, "accepted");
  // The transport still receives an independent diagnostic request ID; the
  // playback replacement identity itself remains exactly native-owned.
  assert.equal(generated, 1);
  assert.equal(
    calls[0].replacement.requestId,
    "native-replacement-request-1",
  );
  assert.deepEqual(calls[0].replacement.previousTerminal, previousTerminal);
});

test("terminal operations reject invalid evidence before transport", async () => {
  let calls = 0;
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: { fetch: async () => {
      calls += 1;
      return jsonResponse(acceptedPlayback("unused"));
    } },
  });

  await assert.rejects(client.handoffPlayback("session-1", {
    requestId: "short",
    entryId: "queue-entry-2",
    previousTerminal: {
      disposition: "completed",
      generation: 1,
      eventSequence: 2,
      recordedAt: "2026-08-30T18:20:00.000Z",
      positionSeconds: 60,
      durationSeconds: 60,
    },
  }), /request ID/);
  await assert.rejects(client.handoffPlayback("session-1", {
    requestId: "terminal-partial-1",
    entryId: "queue-entry-2",
    previousTerminal: {
      disposition: "completed",
      generation: 1,
      positionSeconds: 60,
      durationSeconds: 60,
    },
  }), /ordering authority must be complete/);
  await assert.rejects(client.handoffPlayback("session-1", {
    requestId: "terminal-fractional-start-1",
    entryId: "queue-entry-2",
    startSeconds: 0.5,
    previousTerminal: {
      disposition: "completed",
      generation: 1,
      eventSequence: 2,
      recordedAt: "2026-08-30T18:20:00.000Z",
      positionSeconds: 60,
      durationSeconds: 60,
    },
  }), /non-negative integer/);
  await assert.rejects(client.handoffPlayback("session-1", {
    requestId: "terminal-non-final-completed-1",
    entryId: "queue-entry-2",
    previousTerminal: {
      disposition: "completed",
      generation: 1,
      eventSequence: 2,
      recordedAt: "2026-08-30T18:20:00.000Z",
      positionSeconds: 59.5,
      durationSeconds: 60,
    },
  }), /position must equal its duration/);
  assert.throws(() => client.revokePlaybackContinuation(
      "session-1",
      { token: "continuation", origin: "https://server.example", generation: 1, expiresAt: "2026-08-30T19:20:00.000Z" },
      {
        requestId: "terminal-invalid-1",
        terminal: {
          disposition: "completed",
          generation: 1,
          eventSequence: 2,
          recordedAt: "not-a-time",
          positionSeconds: 60,
          durationSeconds: 60,
        },
      },
    ), /observation time/);
  assert.equal(calls, 0);
});

test("stop dispatches its authoritative DELETE when progress delivery stalls", async () => {
  const calls = [];
  const records = new Map();
  const durability = {
    load: async () => [...records.values()].map(value => structuredClone(value)),
    save: async record => { records.set(record.key, structuredClone(record)); },
    remove: async key => { records.delete(key); },
  };
  let releaseProgress;
  let markProgressStarted;
  const stalledProgress = new Promise(resolve => { releaseProgress = resolve; });
  const progressStarted = new Promise(resolve => { markProgressStarted = resolve; });
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    csrfToken: "1",
    playbackProgressDurabilityAdapter: durability,
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        if (init.method === "PATCH") {
          markProgressStarted();
          return stalledProgress;
        }
        const request = JSON.parse(init.body);
        return jsonResponse(playbackTerminalReceipt("session-stalled", request));
      }
    }
  });

  client.acceptPlaybackSession(acceptedPlayback("session-stalled"));
  const progress = client.touchPlayback("session-stalled", { positionSeconds: 12 });
  await progressStarted;
  await client.stopPlayback("session-stalled", { disposition: "completed", positionSeconds: 60, durationSeconds: 60 });

  assert.deepEqual(calls.map(call => call.init.method), ["PATCH", "DELETE"]);
  assert.equal(calls[1].input, "https://server.example/api/playback-sessions/session-stalled");
  assert.equal(calls.filter(call => call.init.method === "DELETE").length, 1);
  const completed = JSON.parse(calls[1].init.body);
  assert.deepEqual({ ...completed.terminal, recordedAt: "<timestamp>" }, {
    disposition: "completed", generation: 1, eventSequence: 2,
    recordedAt: "<timestamp>", positionSeconds: 60, durationSeconds: 60,
  });
  assert.match(completed.terminal.recordedAt, /^\d{4}-\d{2}-\d{2}T/);
  assert.equal(records.size, 0);
  releaseProgress(jsonResponse({ accepted: true, duplicate: false, stale: false, highestEventSequence: 1 }));
  await assert.rejects(progress, error => error instanceof ApiError && error.code === "playback_progress_stopped");
  assert.equal(records.size, 0);
});

test("accepted terminal waits for and removes a late durable progress save", async () => {
  const calls = [];
  const records = new Map();
  let releaseProgressSave;
  let markProgressSaveStarted;
  let markTerminalAccepted;
  const progressSaveGate = new Promise(resolve => { releaseProgressSave = resolve; });
  const progressSaveStarted = new Promise(resolve => { markProgressSaveStarted = resolve; });
  const terminalAccepted = new Promise(resolve => { markTerminalAccepted = resolve; });
  const durability = {
    load: async () => [...records.values()].map(value => structuredClone(value)),
    save: async record => {
      if (record.kind !== "terminal") {
        markProgressSaveStarted();
        await progressSaveGate;
      }
      records.set(record.key, structuredClone(record));
    },
    remove: async key => { records.delete(key); },
  };
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    playbackProgressDurabilityAdapter: durability,
    transport: { fetch: async (_input, init) => {
      const body = JSON.parse(init.body);
      calls.push({ method: init.method, body });
      if (init.method === "PATCH") {
        throw new Error("superseded progress must not reach the wire");
      }
      markTerminalAccepted();
      return jsonResponse(
        playbackTerminalReceipt("session-late-progress-save", body),
      );
    } },
  });
  client.acceptPlaybackSession(acceptedPlayback("session-late-progress-save", 1));

  const progressResult = client
    .touchPlayback("session-late-progress-save", { positionSeconds: 12 })
    .catch(error => error);
  await progressSaveStarted;
  const stop = client.stopPlayback("session-late-progress-save", {
    disposition: "completed", positionSeconds: 60, durationSeconds: 60,
  });
  await terminalAccepted;
  assert.equal(calls.length, 1);
  assert.equal(calls[0].method, "DELETE");
  assert.equal(calls[0].body.terminal.eventSequence, 2);

  // Let the accepted response enter cleanup while the earlier progress save is
  // still unresolved, then release the write and verify it cannot resurrect.
  await new Promise(resolve => setImmediate(resolve));
  releaseProgressSave();
  const receipt = await stop;
  const progressError = await progressResult;

  assert.equal(receipt.accepted, true);
  assert.equal(progressError.code, "playback_progress_stopped");
  assert.equal(records.size, 0);
});

test("progress persistence stays ordered while terminal cleanup supersedes a queued successor", async () => {
  const records = new Map();
  let progressSaveCount = 0;
  let activeProgressSaves = 0;
  let maximumActiveProgressSaves = 0;
  let releaseSecondProgressSave;
  let markSecondProgressSaveStarted;
  let releaseFirstProgressResponse;
  let markFirstProgressSent;
  let markTerminalAccepted;
  const secondProgressSaveGate = new Promise(resolve => {
    releaseSecondProgressSave = resolve;
  });
  const secondProgressSaveStarted = new Promise(resolve => {
    markSecondProgressSaveStarted = resolve;
  });
  const firstProgressResponse = new Promise(resolve => {
    releaseFirstProgressResponse = resolve;
  });
  const firstProgressSent = new Promise(resolve => {
    markFirstProgressSent = resolve;
  });
  const terminalAccepted = new Promise(resolve => {
    markTerminalAccepted = resolve;
  });
  const durability = {
    load: async () => [...records.values()].map(value => structuredClone(value)),
    save: async record => {
      if (record.kind !== "terminal") {
        progressSaveCount += 1;
        activeProgressSaves += 1;
        maximumActiveProgressSaves = Math.max(
          maximumActiveProgressSaves,
          activeProgressSaves,
        );
        if (progressSaveCount === 2) {
          markSecondProgressSaveStarted();
          await secondProgressSaveGate;
        }
        records.set(record.key, structuredClone(record));
        activeProgressSaves -= 1;
        return;
      }
      records.set(record.key, structuredClone(record));
    },
    remove: async key => { records.delete(key); },
  };
  let patchCount = 0;
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    playbackProgressDurabilityAdapter: durability,
    transport: { fetch: async (_input, init) => {
      const body = JSON.parse(init.body);
      if (init.method === "PATCH") {
        patchCount += 1;
        if (patchCount !== 1) {
          throw new Error("superseded progress must not reach the wire");
        }
        markFirstProgressSent();
        return firstProgressResponse;
      }
      markTerminalAccepted();
      return jsonResponse(
        playbackTerminalReceipt("session-ordered-persistence", body),
      );
    } },
  });
  client.acceptPlaybackSession(acceptedPlayback("session-ordered-persistence", 1));

  const first = client.touchPlayback(
    "session-ordered-persistence",
    { positionSeconds: 10 },
  );
  await firstProgressSent;
  const second = client.touchPlayback(
    "session-ordered-persistence",
    { positionSeconds: 20 },
  ).catch(error => error);
  releaseFirstProgressResponse(jsonResponse({
    accepted: true, duplicate: false, stale: false, highestEventSequence: 1,
  }));
  await first;
  await secondProgressSaveStarted;
  const third = client.touchPlayback(
    "session-ordered-persistence",
    { positionSeconds: 30 },
  ).catch(error => error);
  const stop = client.stopPlayback("session-ordered-persistence", {
    disposition: "completed", positionSeconds: 60, durationSeconds: 60,
  });
  await terminalAccepted;
  await new Promise(resolve => setImmediate(resolve));

  assert.equal(progressSaveCount, 2);
  assert.equal(maximumActiveProgressSaves, 1);
  assert.equal(patchCount, 1);
  releaseSecondProgressSave();

  assert.equal((await stop).accepted, true);
  assert.equal((await second).code, "playback_progress_stopped");
  assert.equal((await third).code, "playback_progress_stopped");
  assert.equal(records.size, 0);
  assert.equal(maximumActiveProgressSaves, 1);
});

test("a rejected durable progress event cannot veto terminal authority", async () => {
  const calls = [];
  const records = new Map();
  const durability = {
    load: async () => [...records.values()].map(value => structuredClone(value)),
    save: async record => { records.set(record.key, structuredClone(record)); },
    remove: async key => { records.delete(key); },
  };
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    playbackProgressDurabilityAdapter: durability,
    transport: { fetch: async (_input, init) => {
      const body = JSON.parse(init.body);
      calls.push({ method: init.method, body });
      if (init.method === "PATCH") {
        return jsonResponse({
          code: "playback_progress_rejected",
          detail: "The progress event was rejected.",
        }, { status: 409 });
      }
      return jsonResponse(
        playbackTerminalReceipt("session-rejected-progress", body),
      );
    } },
  });
  client.acceptPlaybackSession(acceptedPlayback("session-rejected-progress", 1));

  await assert.rejects(
    client.touchPlayback("session-rejected-progress", { positionSeconds: 12 }),
    error => error instanceof ApiError && error.status === 409,
  );
  const receipt = await client.stopPlayback("session-rejected-progress", {
    disposition: "completed", positionSeconds: 60, durationSeconds: 60,
  });

  assert.equal(receipt.accepted, true);
  assert.deepEqual(calls.map(call => call.method), ["PATCH", "PATCH", "DELETE"]);
  assert.equal(calls[2].body.terminal.eventSequence, 2);
  assert.equal(records.size, 0);
});

test("playback progress events carry a monotonic session sequence and observation time", async () => {
  const calls = [];
  const observedAt = Date.parse("2026-07-10T12:00:00Z");
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    csrfToken: "1",
    now: () => observedAt,
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return jsonResponse({ accepted: true, duplicate: false, stale: false, highestEventSequence: calls.length, sessionState: "playing" });
      }
    }
  });

  await client.touchPlayback("session-ordered", { state: "playing", positionSeconds: 120.25, progressSeconds: 120.25, durationSeconds: 3600.8 });
  await client.touchPlayback("session-ordered", { state: "playing", positionSeconds: 90 });

  assert.equal(calls[0].input, "https://server.example/api/playback-sessions/session-ordered");
  assert.deepEqual(JSON.parse(calls[0].init.body), {
    state: "playing",
    positionSeconds: 120.25,
    progressSeconds: 120,
    durationSeconds: 3601,
    eventSequence: 1,
    recordedAt: "2026-07-10T12:00:00.000Z"
  });
  assert.deepEqual(JSON.parse(calls[1].init.body), {
    state: "playing",
    positionSeconds: 90,
    eventSequence: 2,
    recordedAt: "2026-07-10T12:00:00.000Z"
  });
});

test("playback progress keeps one immutable event in flight and coalesces only the newest successor", async () => {
  const calls = [];
  const releases = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (_input, init) => {
        const event = JSON.parse(init.body);
        calls.push(event);
        return new Promise((resolve) => releases.push(() => resolve(jsonResponse({
          accepted: true,
          duplicate: false,
          stale: false,
          highestEventSequence: event.eventSequence,
          sessionState: event.completed ? "stopped" : "playing"
        }))));
      }
    }
  });

  const first = client.touchPlayback("session-coalesced", { state: "playing", positionSeconds: 10 });
  await new Promise(resolve => setImmediate(resolve));
  const second = client.touchPlayback("session-coalesced", { state: "playing", positionSeconds: 20 });
  const newest = client.touchPlayback("session-coalesced", { state: "playing", positionSeconds: 30 });
  await new Promise(resolve => setImmediate(resolve));

  assert.equal(calls.length, 1);
  assert.equal(calls[0].positionSeconds, 10);
  releases.shift()();
  await first;
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(calls.length, 2);
  assert.equal(calls[1].positionSeconds, 30);
  assert.equal(calls[1].eventSequence, 3);
  releases.shift()();
  const [secondAck, newestAck] = await Promise.all([second, newest]);
  assert.equal(secondAck.highestEventSequence, 3);
  assert.equal(newestAck.highestEventSequence, 3);
});

test("playback progress retries an uncertain event exactly before delivering its successor", async () => {
  const calls = [];
  let attempt = 0;
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (_input, init) => {
        const event = JSON.parse(init.body);
        calls.push(event);
        attempt++;
        if (attempt === 1) throw new TypeError("response was lost");
        return jsonResponse({
          accepted: attempt === 3,
          duplicate: attempt === 2,
          stale: false,
          highestEventSequence: event.eventSequence,
          sessionState: "playing"
        });
      }
    }
  });

  await assert.rejects(
    client.touchPlayback("session-retry", { state: "playing", positionSeconds: 12 }),
    /response was lost/
  );
  const successorAck = await client.touchPlayback("session-retry", { state: "playing", positionSeconds: 24 });

  assert.deepEqual(calls.map(event => [event.eventSequence, event.positionSeconds]), [
    [1, 12],
    [1, 12],
    [2, 24]
  ]);
  assert.equal(successorAck.highestEventSequence, 2);
});

test("legacy completed progress is discarded and completion uses only atomic stop", async () => {
  const records = new Map();
  const durability = {
    load: async () => [...records.values()].map(value => structuredClone(value)),
    save: async record => { records.set(record.key, structuredClone(record)); },
    remove: async key => { records.delete(key); }
  };
  const sessionStore = createMemorySessionStore({
    apiBaseUrl: "https://server.example", serverId: "server-1", authority: "local",
    accountId: "account-1", profileId: "profile-1", accessToken: "access"
  });
  const legacyKey = JSON.stringify(["server-1", "local", "account-1", "profile-1", "session-1", 1]);
  records.set(legacyKey, {
    version: "v1",
    key: legacyKey,
    events: [{ completed: true, positionSeconds: 12, eventSequence: 1, recordedAt: "2026-08-29T01:00:00.000Z" }],
    updatedAt: "2026-08-29T01:00:00.000Z"
  });

  const calls = [];
  const restarted = createPorticoClient({
    sessionStore,
    playbackProgressDurabilityAdapter: durability,
    transport: { fetch: async (input, init) => {
      const body = init.body ? JSON.parse(init.body) : undefined;
      calls.push({url: String(input), method: init.method, body});
      return jsonResponse(init.method === "PATCH"
        ? {accepted: true, duplicate: false, stale: false, highestEventSequence: 1}
        : playbackTerminalReceipt("session-1", body));
    } }
  });
  restarted.acceptPlaybackSession(acceptedPlayback("session-1", 1));
  await restarted.stopPlayback("session-1", { disposition: "completed", positionSeconds: 60, durationSeconds: 60 });
  assert.deepEqual(calls.map(call => call.method), ["DELETE"]);
  assert.equal(calls[0].body.terminal.disposition, "completed");
  assert.equal(records.size, 0);
});

test("playback progress cannot use the retired completion flag", async () => {
  const client = createPorticoClient({ apiBaseUrl: "https://server.example" });
  await assert.rejects(
    client.touchPlayback("session-1", { positionSeconds: 12, completed: true }),
    /atomic stop operation/,
  );
});

test("chunked JSON responses are cancelled at the byte limit before full buffering", async () => {
  let cancelled = false;
  const oversized = new Uint8Array(4 * 1024 * 1024 + 1);
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: { fetch: async () => new Response(new ReadableStream({
      start(controller) { controller.enqueue(oversized); },
      cancel() { cancelled = true; }
    }), {status: 200, headers: {"Content-Type": "application/json"}}) }
  });
  await assert.rejects(client.system(), error => error instanceof ApiError && error.code === "response_too_large");
  assert.equal(cancelled, true);
});

test("chunked JSON responses work in React Native fetch without a readable body stream", async () => {
  let textReads = 0;
  const payload = JSON.stringify({items: [], total: 0});
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: { fetch: async () => ({
      ok: true,
      status: 200,
      headers: new Headers({"Content-Type": "application/json"}),
      body: undefined,
      text: async () => {
        textReads++;
        return payload;
      }
    }) }
  });

  assert.deepEqual(await client.libraries(), {items: [], total: 0});
  assert.equal(textReads, 1);
});

test("React Native fetch fallback rejects an oversized chunked response after native buffering", async () => {
  const oversized = `{"value":"${"x".repeat(4 * 1024 * 1024)}"}`;
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: { fetch: async () => ({
      ok: true,
      status: 200,
      headers: new Headers({"Content-Type": "application/json"}),
      body: undefined,
      text: async () => oversized
    }) }
  });

  await assert.rejects(
    client.libraries(),
    error => error instanceof ApiError && error.code === "response_too_large"
  );
});

test("a queued playback successor is rebased above a newer durable server sequence", async () => {
  const calls = [];
  let releaseFirst;
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (_input, init) => {
        const event = JSON.parse(init.body);
        calls.push(event);
        if (calls.length === 1) {
          return new Promise((resolve) => {
            releaseFirst = () => resolve(jsonResponse({
              accepted: true,
              duplicate: false,
              stale: false,
              highestEventSequence: 10,
              sessionState: "playing"
            }));
          });
        }
        return jsonResponse({
          accepted: true,
          duplicate: false,
          stale: false,
          highestEventSequence: event.eventSequence,
          sessionState: "playing"
        });
      }
    }
  });

  const first = client.touchPlayback("session-rebase", { state: "playing", positionSeconds: 1 });
  await new Promise(resolve => setImmediate(resolve));
  const successor = client.touchPlayback("session-rebase", { state: "playing", positionSeconds: 2 });
  releaseFirst();
  await Promise.all([first, successor]);

  assert.deepEqual(calls.map(event => event.eventSequence), [1, 11]);
  assert.equal(calls[1].positionSeconds, 2);
});

test("playback progress mailboxes do not cross Client instances or authenticated principals", async () => {
  const events = [];
  const transport = {
    fetch: async (_input, init) => {
      const event = JSON.parse(init.body);
      events.push(event);
      return jsonResponse({
        accepted: true,
        duplicate: false,
        stale: false,
        highestEventSequence: event.eventSequence,
        sessionState: "playing"
      });
    }
  };
  const firstStore = createMemorySessionStore({
    apiBaseUrl: "https://server.example",
    authority: "hosted",
    accountId: "account-a",
    profileId: "profile-a",
    authorizationRevision: "revision-a"
  });
  const firstClient = createPorticoClient({ sessionStore: firstStore, transport });
  const secondClient = createPorticoClient({
    sessionStore: createMemorySessionStore({
      apiBaseUrl: "https://server.example",
      authority: "hosted",
      accountId: "account-a",
      profileId: "profile-a",
      authorizationRevision: "revision-a"
    }),
    transport
  });

  await firstClient.touchPlayback("shared-session", { state: "playing", positionSeconds: 1 });
  await secondClient.touchPlayback("shared-session", { state: "playing", positionSeconds: 2 });
  firstStore.set({
    apiBaseUrl: "https://server.example",
    authority: "hosted",
    accountId: "account-b",
    profileId: "profile-b",
    authorizationRevision: "revision-b"
  });
  await firstClient.touchPlayback("shared-session", { state: "playing", positionSeconds: 3 });

  assert.deepEqual(events.map(event => event.eventSequence), [1, 1, 1]);
});

test("continuation progress is generation-fenced and acknowledgement seeding never regresses", async () => {
  const events = [];
  const acknowledgements = [10, 5, 12, 1];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (_input, init) => {
        const event = JSON.parse(init.body);
        events.push(event);
        return jsonResponse({
          accepted: true,
          duplicate: false,
          stale: false,
          highestEventSequence: acknowledgements.shift(),
          sessionState: "playing"
        });
      }
    }
  });
  const generationOne = { token: "token-one", origin: "https://server.example", expiresAt: "2026-07-21T01:00:00Z", generation: 1 };
  const generationTwo = { token: "token-two", origin: "https://server.example", expiresAt: "2026-07-21T01:00:00Z", generation: 2 };

  await client.touchPlaybackContinuation("session-generation", generationOne, { state: "playing", positionSeconds: 1 });
  await client.touchPlaybackContinuation("session-generation", generationOne, { state: "playing", positionSeconds: 2 });
  await client.touchPlaybackContinuation("session-generation", generationOne, { state: "playing", positionSeconds: 3 });
  await client.touchPlaybackContinuation("session-generation", generationTwo, { state: "playing", positionSeconds: 4 });

  assert.deepEqual(events.map(event => event.eventSequence), [1, 11, 12, 1]);
});

test("playback stop commands carry viewer messages through JSON requests", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    csrfToken: "1",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return jsonResponse({ id: "pcmd-1", action: "stop", message: "Server restarting in 5 minutes." });
      }
    }
  });

  const command = await client.issuePlaybackCommand("session-1", { action: "stop", message: "Server restarting in 5 minutes." });
  assert.equal(command.message, "Server restarting in 5 minutes.");
  assert.equal(calls[0].input, "https://server.example/api/playback-sessions/session-1/command");
  assert.equal(calls[0].init.method, "POST");
  assert.equal(calls[0].init.headers["Content-Type"], "application/json");
  assert.equal(calls[0].init.headers["X-Portico-CSRF"], "1");
  assert.deepEqual(JSON.parse(calls[0].init.body), { action: "stop", message: "Server restarting in 5 minutes." });
});

test("media state mutations use focused invalidation tags", () => {
  assert.deepEqual(dataTagsForMutation("/api/account/watch-history"), ["account", "auth", "media-state", "playback-progress", "dashboard:history", "home", "playback"]);
  assert.deepEqual(dataTagsForMutation("/api/media/m1/watched"), ["media-state", "library-items"]);
  assert.deepEqual(dataTagsForMutation("/api/media/bulk/state"), ["media-state", "library-items"]);
  assert.deepEqual(dataTagsForMutation("/api/media/bulk/jobs"), ["jobs", "dashboard:jobs"]);
  assert.deepEqual(dataTagsForMutation("/api/media/bulk/metadata"), ["media", "library-items"]);
  assert.deepEqual(dataTagsForMutation("/api/media/m1"), ["media", "library-items"]);
  assert.equal(dataTagsForMutation("/api/media/m1").includes("home"), false);
});

test("resource URLs never add account tokens to query strings", () => {
  const sessionStore = createMemorySessionStore({
    apiBaseUrl: "https://server.example",
    bootstrapAccessToken: "token-123"
  });
  const client = createPorticoClient({ sessionStore, baseHref: "https://web.example" });

  assert.equal(client.resourceUrl("/api/artwork/abc/poster"), "https://server.example/api/artwork/abc/poster");
  assert.equal(client.resourceUrl("/api/live-tv/hls/chan1/playlist.m3u8"), "https://server.example/api/live-tv/hls/chan1/playlist.m3u8");
  assert.equal(client.resourceUrl("/api/system/identity"), "https://server.example/api/system/identity");
  assert.equal(client.resourceUrl("https://cdn.example/image.jpg"), "https://cdn.example/image.jpg");
  assert.equal(client.imageResourceUrl("/api/people/person-id/artwork", {rendition: "small"}), "https://server.example/api/people/person-id/artwork?rendition=small");
  assert.equal(client.imageResourceUrl("/api/artwork/media/backdrop", {rendition: "large"}), "https://server.example/api/artwork/media/backdrop?rendition=large");
});

test("hosted connector accepts only the two published route qualities", () => {
	const reachableRoute = { type: "public_direct", url: "https://reachable.example", quality: "reachable" };
	const probeRoute = { type: "public_direct", url: "https://probe.example", quality: "probe_required" };

	assert.equal(routeIsUsableCandidate(reachableRoute), true);
	assert.equal(routeIsUsableCandidate(probeRoute), true);
	for (const quality of ["reported", "checking", "fallback", "stale", "failed", "http_failed", "tls_failed", "identity_mismatch", "repairing", "repair_requested", "unknown", ""]) {
		assert.equal(routeIsUsableCandidate({ type: "public_direct", url: `https://${quality || "empty"}.example`, quality }), false, quality);
	}
	assert.equal(preferredRoute({ kind: "route-document", documentVersion: 1, endpointGeneration: 1, routes: [probeRoute, reachableRoute] }).url, "https://reachable.example");
	assert.equal(preferredRoute({ kind: "route-document", documentVersion: 1, endpointGeneration: 1, routes: [{ type: "public_direct", url: "https://legacy.example", quality: "identity_mismatch" }] }), undefined);
});

test("Hosted document key sets retain staged rotation keys and fail closed", () => {
  const stagedKey = generateKeyPairSync("ed25519").publicKey.export({ format: "der", type: "spki" }).subarray(-32).toString("base64");
  const trusted = trustedHostedDocumentKeysFromKeySet({
    schemaVersion: 1,
    activeKeyId: hostedDocumentTestKeyId,
    keys: [
      { keyId: hostedDocumentTestKeyId, algorithm: "ed25519", publicKeyB64: hostedDocumentTestPublicKey, state: "active" },
      { keyId: "hosted-documents-next", algorithm: "ed25519", publicKeyB64: stagedKey, state: "verification" }
    ]
  });
  assert.equal(trusted[hostedDocumentTestKeyId], hostedDocumentTestPublicKey);
  assert.equal(trusted["hosted-documents-next"], stagedKey);
  assert.throws(() => trustedHostedDocumentKeysFromKeySet({
    schemaVersion: 1,
    activeKeyId: "missing",
    keys: [{ keyId: hostedDocumentTestKeyId, algorithm: "ed25519", publicKeyB64: hostedDocumentTestPublicKey, state: "verification" }]
  }), /active key/);
});

test("Hosted route verification runs entirely through injected binary and Ed25519 adapters", async () => {
  const document = signedRouteDocument({
    serverId: "srv_native",
    serverName: "Native",
    assignedHostname: "native.direct.getportico.tv",
    issuedAt: "2026-05-23T00:00:00Z",
    expiresAt: "2026-05-23T00:05:00Z",
    serverPublicKeyFingerprint: "sha256:native",
    authModes: ["portico"],
    certificate: { status: "valid" },
    membership: { role: "owner" },
    routes: [{
      type: "public_direct_ip_encoded",
      url: "https://203-0-113-10.ptc-native.direct.getportico.tv:32500",
		quality: "probe_required",
      lastCheckedAt: "2026-05-23T00:00:30Z",
      lastCheckError: "Reachability is still being verified."
    }]
  });
  const calls = [];
  const runtime = createHostedConnectionRuntime({
    decodeBase64(value) {
      calls.push("base64");
      return Uint8Array.from(Buffer.from(value, "base64"));
    },
    encodeText(value) {
      calls.push("text");
      return Uint8Array.from(Buffer.from(value, "utf8"));
    },
    verifyEd25519({ publicKey, signature, message }) {
      calls.push("ed25519");
      assert.equal(publicKey.byteLength, 32);
      return verify(null, Buffer.from(message), hostedDocumentTestKeys.publicKey, Buffer.from(signature));
    }
  });

  await verifyHostedRouteDocument(
    document,
    "srv_native",
    { [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey },
    new Date("2026-05-23T00:01:00Z"),
    runtime
  );

  assert.deepEqual(calls, ["base64", "base64", "base64", "text", "ed25519"]);
});

test("Hosted route verification and raw route helpers reject a wrong document kind", async () => {
  const wrongKind = signedRouteDocument({
    kind: "policy-snapshot",
    serverId: "srv_wrong_kind",
    serverName: "Wrong kind",
    issuedAt: "2026-05-23T00:00:00Z",
    expiresAt: "2026-05-23T00:05:00Z",
    authModes: ["portico"],
    certificate: { status: "valid" },
    membership: { role: "owner" },
    routes: []
  });
  await assert.rejects(
    verifyHostedRouteDocument(wrongKind, "srv_wrong_kind", {[hostedDocumentTestKeyId]: hostedDocumentTestPublicKey}, new Date("2026-05-23T00:01:00Z"), clientAttachmentRuntime),
    /kind/i
  );
  assert.throws(() => routesForConnection(wrongKind), /kind/i);
  assert.throws(() => preferredRoute(wrongKind), /kind/i);
});

test("Client Core verifies the canonical Hosted signing fixture without private reachability diagnostics", async () => {
  const fixture = JSON.parse(readFileSync("fixtures/signing/document-signing-fixture.json", "utf8"));
  assert.equal(fixture.routeDocument.kind, "route-document");
  assert.equal(Object.hasOwn(fixture.routeDocument.routes[0], "lastCheckError"), false);
  assert.equal(fixture.routeDocument.routes[0].lastCheckedAt.length > 0, true);
  await verifyHostedRouteDocument(
    fixture.routeDocument,
    fixture.routeDocument.serverId,
    { [fixture.routeDocument.signatureKeyId]: fixture.publicKeyB64 },
    new Date("2026-07-09T12:01:00.000Z")
  );
});

test("Client Core verifies Hosted canonical UTF-8, HTML-sensitive text, and Unicode separators", async () => {
  const fixture = JSON.parse(readFileSync("fixtures/signing/document-signing-adversarial-fixture.json", "utf8"));
  assert.match(fixture.routeDocument.serverName, /Cinéma <Portico> & 雪/);
  assert.equal(fixture.routeDocument.kind, "route-document");
  assert.equal(Object.hasOwn(fixture.routeDocument.routes[0], "lastCheckError"), false);
  await verifyHostedRouteDocument(
    fixture.routeDocument,
    fixture.routeDocument.serverId,
    { [fixture.routeDocument.signatureKeyId]: fixture.publicKeyB64 },
    new Date("2026-07-13T12:01:00.000Z")
  );
});

test("Hosted connector uses injected fetch, clock, abort, and timer facilities", async () => {
  const calls = [];
  const timeoutHandle = { kind: "native-timer" };
  let sessionSet;
  const runtime = {
    ...clientAttachmentRuntime,
    fetch: async (input, init) => {
      calls.push(["fetch", String(input), Boolean(init.signal)]);
      return jsonResponse({ serverId: "srv_runtime", serverPublicKeyFingerprint: testServerPublicKeyFingerprint, remoteAccessEnabled: true });
    },
    decodeBase64: (value) => Uint8Array.from(Buffer.from(value, "base64")),
    encodeText: (value) => Uint8Array.from(Buffer.from(value, "utf8")),
    verifyEd25519: ({ publicKey, signature, message }) => Buffer.from(publicKey).equals(Buffer.from(hostedDocumentTestPublicKey, "base64"))
      ? verify(null, Buffer.from(message), hostedDocumentTestKeys.publicKey, Buffer.from(signature))
      : true,
    createAbortController: () => {
      calls.push(["abort-controller"]);
      return new AbortController();
    },
    setTimeout: (_callback, milliseconds) => {
      calls.push(["set-timeout", milliseconds]);
      return timeoutHandle;
    },
    clearTimeout: (handle) => calls.push(["clear-timeout", handle === timeoutHandle]),
    now: () => new Date("2026-05-23T00:01:00Z")
  };

  await connectHostedServer({ ...testServerIdentity(), id: "srv_runtime", name: "Runtime", preferredAuthMode: "portico" }, {
    ...hostedProfileBinding("srv_runtime"),
    hostedClient: {
      routes: async () => signedRouteDocument({
        serverId: "srv_runtime",
        serverName: "Runtime",
        assignedHostname: "runtime.direct.getportico.tv",
        issuedAt: "2026-05-23T00:00:00Z",
        expiresAt: "2026-05-23T00:05:00Z",
        serverPublicKeyFingerprint: "sha256:runtime",
        authModes: ["portico"],
        certificate: { status: "valid" },
        membership: { role: "owner" },
        routes: [{ type: "public_direct", url: "https://runtime.direct.getportico.tv", quality: "reachable" }]
      }),
      porticoSession: async () => ({ accessToken: "access", refreshToken: "refresh", accessExpiresAt: "2026-05-23T01:00:00Z", refreshExpiresAt: "2026-06-23T00:00:00Z" }),
      reportRouteFailure: async () => ({ ok: true, matched: true })
    },
    localClient: compatibleLocalClient("srv_runtime", "https://runtime.direct.getportico.tv"),
    sessionStore: { set: (session) => { sessionSet = session; }, clear: () => {} },
    runtime,
    retryDelaysMs: [],
    trustedHostedDocumentKeys: { [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey }
  });

  assert.equal(sessionSet.apiBaseUrl, "https://runtime.direct.getportico.tv");
  assert.deepEqual(calls.map(call => call[0]), [
    "abort-controller", "set-timeout", "abort-controller", "abort-controller", "set-timeout",
    "fetch", "clear-timeout", "clear-timeout"
  ]);
  assert.ok(calls[1][1] > 0 && calls[1][1] <= 30_000, "discovery owns a bounded absolute timer");
  assert.equal(calls[4][1], 3500);
  assert.deepEqual(calls[5], ["fetch", "https://runtime.direct.getportico.tv/api/remote-access/health", true]);
  assert.deepEqual(calls.slice(6), [["clear-timeout", true], ["clear-timeout", true]]);
});

test("Hosted connector clears the provisional session and stops before auth when the server API is incompatible", async () => {
  let sessionSet;
  let clearCalls = 0;
  let meCalls = 0;
  const mismatch = new Error("This app does not support the server API revision.");

  await assert.rejects(
    connectHostedServer({ ...testServerIdentity(), id: "srv_incompatible", name: "Incompatible", preferredAuthMode: "portico" }, {
      ...hostedProfileBinding("srv_incompatible"),
      hostedClient: {
        routes: async () => signedRouteDocument({
          serverId: "srv_incompatible",
          serverName: "Incompatible",
          assignedHostname: "incompatible.direct.getportico.tv",
          issuedAt: "2026-05-23T00:00:00Z",
          expiresAt: "2026-05-23T00:05:00Z",
          serverPublicKeyFingerprint: "sha256:incompatible",
          authModes: ["portico"],
          certificate: { status: "valid" },
          membership: { role: "owner" },
          routes: [{ type: "public_direct", url: "https://incompatible.direct.getportico.tv", quality: "reachable" }]
        }),
        porticoSession: async () => ({ accessToken: "access", refreshToken: "refresh", accessExpiresAt: "2026-05-23T01:00:00Z", refreshExpiresAt: "2026-06-23T00:00:00Z" }),
        reportRouteFailure: async () => ({ ok: true, matched: true })
      },
      localClient: compatibleLocalClient("srv_incompatible", "https://incompatible.direct.getportico.tv", {
        checkServerCompatibility: async () => { throw mismatch; },
        me: async () => {
          meCalls++;
          return { authenticated: true, user: { id: "usr", username: "Owner", role: "owner" } };
        }
      }),
      sessionStore: {
        set: (session) => { sessionSet = session; },
        clear: () => { clearCalls++; sessionSet = undefined; }
      },
      routeProbeFetch: async () => jsonResponse({
        serverId: "srv_incompatible",
        serverPublicKeyFingerprint: testServerPublicKeyFingerprint,
        remoteAccessEnabled: true
      }),
      retryDelaysMs: [],
      trustedHostedDocumentKeys: { [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey },
      runtime: clientAttachmentRuntime,
      now: () => new Date("2026-05-23T00:01:00Z")
    }),
    (error) => error === mismatch
  );

  assert.equal(sessionSet, undefined);
  assert.equal(clearCalls, 1);
  assert.equal(meCalls, 0);
});

test("Hosted runtime reports an actionable missing base64 capability", () => {
  const originalAtob = globalThis.atob;
  try {
    Object.defineProperty(globalThis, "atob", { configurable: true, writable: true, value: undefined });
    assert.throws(
      () => trustedHostedDocumentKeysFromKeySet({
        schemaVersion: 1,
        activeKeyId: hostedDocumentTestKeyId,
        keys: [{ keyId: hostedDocumentTestKeyId, algorithm: "ed25519", publicKeyB64: hostedDocumentTestPublicKey, state: "active" }]
      }),
      /capability 'base64'.*runtime\.decodeBase64/
    );
  } finally {
    Object.defineProperty(globalThis, "atob", { configurable: true, writable: true, value: originalAtob });
  }
});

test("Hosted runtime reports an actionable missing Ed25519 capability", async () => {
  const cryptoDescriptor = Object.getOwnPropertyDescriptor(globalThis, "crypto");
  try {
    Object.defineProperty(globalThis, "crypto", { configurable: true, writable: true, value: undefined });
    const runtime = createHostedConnectionRuntime();
    await assert.rejects(
      runtime.verifyEd25519({ publicKey: new Uint8Array(32), signature: new Uint8Array(64), message: new Uint8Array() }),
      /capability 'ed25519'.*runtime\.verifyEd25519/
    );
  } finally {
    if (cryptoDescriptor) Object.defineProperty(globalThis, "crypto", cryptoDescriptor);
    else delete globalThis.crypto;
  }
});

test("hosted connector ranks published quality before the configured LAN/public type precedence", () => {
	const reachablePublic = { type: "public_direct", url: "https://public.example", quality: "reachable" };
	const reachableLan = { type: "lan", url: "https://10.0.0.5:32500", quality: "reachable" };
	const encodedLan = { type: "lan_ip_encoded", url: "https://10-0-0-5.ptc.direct.example", quality: "probe_required" };
	const probePublic = { type: "public_direct", url: "https://probe.example", quality: "probe_required" };
	const encodedProbePublic = { type: "public_direct_ip_encoded", url: "https://old-public.ptc.direct.example", quality: "probe_required" };
	const checkingPublic = { type: "public_direct", url: "https://checking.example", quality: "checking" };
	const verifiedFallback = { type: "public_direct_ip_encoded", url: "https://old-public.ptc.direct.example", quality: "fallback" };
	const staleLan = { type: "lan_ip_encoded", url: "https://10-0-0-4.ptc.direct.example", quality: "stale" };
	const insecureLan = { type: "lan_ip_encoded", url: "http://10-0-0-6.ptc.direct.example", quality: "probe_required" };

  assert.deepEqual(routesForConnection({
    kind: "route-document",
    documentVersion: 1,
    endpointGeneration: 1,
    authModes: ["portico"],
		routes: [checkingPublic, reachableLan, probePublic, reachablePublic, encodedLan, staleLan, insecureLan, verifiedFallback, encodedProbePublic]
	}).map((route) => route.url), [
		"https://10.0.0.5:32500",
		"https://public.example",
		"https://10-0-0-5.ptc.direct.example",
		"https://old-public.ptc.direct.example",
		"https://probe.example"
	]);
  assert.deepEqual(routesForConnection({
    kind: "route-document",
    documentVersion: 1,
    endpointGeneration: 1,
    authModes: ["portico"],
		routes: [checkingPublic, reachableLan, probePublic, reachablePublic, encodedLan, staleLan, insecureLan, verifiedFallback, encodedProbePublic]
	}, "public-first").map((route) => route.url), [
		"https://public.example",
		"https://10.0.0.5:32500",
		"https://old-public.ptc.direct.example",
		"https://probe.example",
		"https://10-0-0-5.ptc.direct.example",
	]);
});

test("hosted connector retries transient route verification before failing", async () => {
  let routeCalls = 0;
  let verifyCalls = 0;
  let sessionSet = null;
  const hostedClient = {
    routes: async () => {
      routeCalls++;
      return signedRouteDocument({
        serverId: "srv_retry",
        serverName: "Retry",
        assignedHostname: "retry.direct.getportico.tv",
        issuedAt: "2026-05-23T00:00:00Z",
        expiresAt: "2026-05-23T00:05:00Z",
        serverPublicKeyFingerprint: testServerPublicKeyFingerprint,
        authModes: ["portico"],
        certificate: { status: "valid" },
        membership: { role: "owner" },
        routes: [{
          type: "public_direct",
          url: "https://retry.direct.getportico.tv",
          host: "retry.direct.getportico.tv",
          address: "198.51.100.90",
          port: 443,
          scheme: "https",
			quality: "probe_required"
        }]
      });
    },
    porticoSession: async () => ({ tokenType: "Bearer", accessToken: "portico-token", accessExpiresAt: "2026-05-23T01:00:00Z", refreshToken: "refresh-token", refreshExpiresAt: "2026-06-23T00:00:00Z" }),
    reportRouteFailure: async () => ({ ok: true, matched: true })
  };
  const localClient = compatibleLocalClient("srv_retry", "https://retry.direct.getportico.tv");

  await connectHostedServer({ ...testServerIdentity(), id: "srv_retry", name: "Retry", preferredAuthMode: "portico" }, {
    ...hostedProfileBinding("srv_retry"),
    hostedClient,
    localClient,
    sessionStore: {
      set: (session) => { sessionSet = session; },
      clear: () => { sessionSet = null; }
    },
    routeProbeFetch: async () => {
      verifyCalls++;
      if (verifyCalls === 1) throw new TypeError("TLS handshake failed");
      return jsonResponse({
        serverId: "srv_retry",
        serverPublicKeyFingerprint: testServerPublicKeyFingerprint,
        remoteAccessEnabled: true
      });
    },
    retryDelaysMs: [0],
    retryDelay: async () => {},
    trustedHostedDocumentKeys: { [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey },
    runtime: clientAttachmentRuntime,
    now: () => new Date("2026-05-23T00:01:00Z")
  });

  assert.equal(routeCalls, 2);
  assert.equal(verifyCalls, 2);
  assert.equal(sessionSet.apiBaseUrl, "https://retry.direct.getportico.tv");
  assert.equal(sessionSet.bootstrapAccessToken, undefined);
  assert.equal(sessionSet.refreshToken, undefined);
});

test("route discovery owns one three-request retry budget with stable cohort jitter", async () => {
  const server = { ...testServerIdentity(), id: "srv_three_routes", name: "Three routes", preferredAuthMode: "portico" };
  const document = signedRouteDocument({
    serverId: server.id,
    serverName: server.name,
    assignedHostname: "three-routes.direct.getportico.tv",
    issuedAt: "2026-05-23T00:00:00Z",
    expiresAt: "2026-05-23T00:05:00Z",
    authModes: ["portico"],
    certificate: {status: "valid"},
    membership: {role: "owner"},
    routes: [{type: "public_direct", url: "https://three-routes.direct.getportico.tv", quality: "reachable"}],
  });
  const run = async () => {
    const requestOptions = [];
    const waits = [];
    let calls = 0;
    const result = await discoverHostedServerRoute(server, {
      hostedClient: {
        routes: async (_serverId, init) => {
          requestOptions.push(init);
          calls += 1;
          if (calls < 3) {
            throw new PorticoTransportError("hosted_transport_failed", "Hosted unavailable", new TypeError("offline"), {method: "GET", retryable: true});
          }
          return document;
        },
        reportRouteFailure: async () => ({ok: true, matched: true}),
      },
      routeProbeFetch: async () => jsonResponse({
        serverId: server.id,
        serverPublicKeyFingerprint: testServerPublicKeyFingerprint,
        remoteAccessEnabled: true,
      }),
      retryDelaysMs: [10, 20, 30, 40],
      retryDelay: async milliseconds => { waits.push(milliseconds); },
      retryCohort: "persisted-installation-1",
      trustedHostedDocumentKeys: {[hostedDocumentTestKeyId]: hostedDocumentTestPublicKey},
      runtime: clientAttachmentRuntime,
      now: () => new Date("2026-05-23T00:01:00Z"),
    });
    assert.equal(result.routeDocument.serverId, server.id);
    assert.equal(calls, 3);
    assert.deepEqual(requestOptions.map(init => init.retryBudget), [0, 0, 0]);
    assert.ok(requestOptions.every(init => Number.isFinite(init.deadlineAt) && init.deadlineAt === requestOptions[0].deadlineAt));
    return waits;
  };
  const first = await run();
  const restarted = await run();
  assert.deepEqual(restarted, first);
  assert.ok(first[0] >= 1 && first[0] <= 10);
  assert.ok(first[1] >= 1 && first[1] <= 20);
});

test("route discovery preserves an explicit publication-pending outcome until a route generation appears", async () => {
  const server = { ...testServerIdentity(), id: "srv_setup_pending", name: "Setup pending", preferredAuthMode: "portico" };
  const pending = signedRouteDocument({
    endpointGeneration: 0,
    serverId: server.id,
    serverName: server.name,
    assignedHostname: "setup-pending.direct.getportico.tv",
    issuedAt: "2026-05-23T00:00:00Z",
    expiresAt: "2026-05-23T00:05:00Z",
    authModes: ["portico"],
    certificate: {status: "pending"},
    membership: {role: "owner"},
    routes: [],
  });
  const ready = signedRouteDocument({
    endpointGeneration: 1,
    serverId: server.id,
    serverName: server.name,
    assignedHostname: "setup-pending.direct.getportico.tv",
    issuedAt: "2026-05-23T00:00:00Z",
    expiresAt: "2026-05-23T00:05:00Z",
    authModes: ["portico"],
    certificate: {status: "valid"},
    membership: {role: "owner"},
    routes: [{type: "public_direct", url: "https://setup-pending.direct.getportico.tv", quality: "reachable"}],
  });
  let calls = 0;
  const result = await discoverHostedServerRoute(server, {
    hostedClient: {routes: async () => (++calls < 3 ? pending : ready)},
    routeProbeFetch: async () => jsonResponse({serverId: server.id, serverPublicKeyFingerprint: testServerPublicKeyFingerprint, remoteAccessEnabled: true}),
    retryDelaysMs: [1, 1],
    retryDelay: async () => {},
    trustedHostedDocumentKeys: {[hostedDocumentTestKeyId]: hostedDocumentTestPublicKey},
    runtime: clientAttachmentRuntime,
    now: () => new Date("2026-05-23T00:01:00Z"),
  });
  assert.equal(calls, 3);
  assert.equal(result.routeDocument.endpointGeneration, 1);
});

test("route discovery does not classify current route probe failures as publication pending", async () => {
  const server = { ...testServerIdentity(), id: "srv_current_probe_failure", name: "Current probe failure", preferredAuthMode: "portico" };
  const document = signedRouteDocument({
    endpointGeneration: 7,
    serverId: server.id,
    serverName: server.name,
    assignedHostname: "current-probe-failure.direct.getportico.tv",
    issuedAt: "2026-05-23T00:00:00Z",
    expiresAt: "2026-05-23T00:05:00Z",
    authModes: ["portico"], certificate: {status: "valid"}, membership: {role: "owner"},
    routes: [{type: "public_direct", url: "https://current-probe-failure.direct.getportico.tv", quality: "reachable"}],
  });
  await assert.rejects(discoverHostedServerRoute(server, {
    hostedClient: {routes: async () => document},
    routeProbeFetch: async () => { throw new TypeError("transport failed"); },
    retryDelaysMs: [],
    trustedHostedDocumentKeys: {[hostedDocumentTestKeyId]: hostedDocumentTestPublicKey},
    runtime: clientAttachmentRuntime,
    now: () => new Date("2026-05-23T00:01:00Z"),
  }), error => !(error instanceof HostedRoutePublicationPendingError));
});

test("route discovery exposes exhausted setup and certificate projection as publication pending", async () => {
  const server = { ...testServerIdentity(), id: "srv_cert_pending", name: "Certificate pending", preferredAuthMode: "portico" };
  const document = signedRouteDocument({
    endpointGeneration: 3,
    serverId: server.id, serverName: server.name,
    assignedHostname: "cert-pending.direct.getportico.tv",
    issuedAt: "2026-05-23T00:00:00Z", expiresAt: "2026-05-23T00:05:00Z",
    authModes: ["portico"], certificate: {status: "issued"}, membership: {role: "owner"}, routes: [],
  });
  await assert.rejects(discoverHostedServerRoute(server, {
    hostedClient: {routes: async () => document},
    retryDelaysMs: [],
    trustedHostedDocumentKeys: {[hostedDocumentTestKeyId]: hostedDocumentTestPublicKey},
    runtime: clientAttachmentRuntime,
    now: () => new Date("2026-05-23T00:01:00Z"),
  }), HostedRoutePublicationPendingError);
});

test("route retry full jitter spreads a 100K persisted fleet across the complete cap", () => {
  const cap = 2500;
  const bucketWidth = 100;
  const buckets = Array.from({length: Math.ceil(cap / bucketWidth)}, () => 0);
  for (let index = 0; index < 100_000; index += 1) {
    const delay = positiveFullJitterDelay(cap, `installation-${index}:srv-fleet`, 0);
    assert.ok(delay >= 1 && delay <= cap);
    buckets[Math.min(buckets.length - 1, Math.floor((delay - 1) / bucketWidth))] += 1;
  }
  assert.ok(Math.max(...buckets) < 5000, `largest 100ms retry bucket=${Math.max(...buckets)}`);
  assert.ok(Math.min(...buckets) > 3000, `smallest 100ms retry bucket=${Math.min(...buckets)}`);
});

test("an explicit empty retry cohort falls back to persisted installation identity across restarts", async () => {
  const server = {...testServerIdentity(), id: "srv_empty_cohort", name: "Empty cohort", preferredAuthMode: "portico"};
  const document = signedRouteDocument({
    serverId: server.id,
    serverName: server.name,
    assignedHostname: "empty-cohort.direct.getportico.tv",
    issuedAt: "2026-05-23T00:00:00Z",
    expiresAt: "2026-05-23T00:05:00Z",
    authModes: ["portico"],
    certificate: {status: "valid"},
    membership: {role: "owner"},
    routes: [{type: "public_direct", url: "https://empty-cohort.direct.getportico.tv", quality: "reachable"}],
  });
  const run = async () => {
    let routeCalls = 0;
    const waits = [];
    await connectHostedServer(server, {
      ...hostedProfileBinding(server.id, "persisted-installation-empty-cohort"),
      hostedClient: {
        routes: async () => {
          routeCalls += 1;
          if (routeCalls === 1) throw new PorticoTransportError("hosted_transport_failed", "offline", new TypeError("offline"), {method: "GET", retryable: true});
          return document;
        },
        porticoSession: async () => ({accessToken: "access", refreshToken: "refresh", accessExpiresAt: "2026-05-23T01:00:00Z", refreshExpiresAt: "2026-06-23T00:00:00Z"}),
        reportRouteFailure: async () => ({ok: true, matched: true}),
      },
      localClient: compatibleLocalClient(server.id, "https://empty-cohort.direct.getportico.tv"),
      sessionStore: {set() {}, clear() {}},
      routeProbeFetch: async () => jsonResponse({serverId: server.id, serverPublicKeyFingerprint: testServerPublicKeyFingerprint, remoteAccessEnabled: true}),
      retryDelaysMs: [2500],
      retryDelay: async milliseconds => { waits.push(milliseconds); },
      retryCohort: "",
      trustedHostedDocumentKeys: {[hostedDocumentTestKeyId]: hostedDocumentTestPublicKey},
      runtime: clientAttachmentRuntime,
      now: () => new Date("2026-05-23T00:01:00Z"),
    });
    return waits;
  };
  assert.deepEqual(await run(), await run());
});

test("route discovery never retries terminal Hosted authorization failures", async () => {
  let calls = 0;
  await assert.rejects(discoverHostedServerRoute(
    {...testServerIdentity(), id: "srv_terminal_route", name: "Terminal route", preferredAuthMode: "portico"},
    {
      hostedClient: {
        routes: async () => {
          calls += 1;
          throw new ApiError(403, "forbidden", "Forbidden");
        },
      },
      retryDelaysMs: [1, 1],
      retryDelay: async () => { throw new Error("terminal failures must not schedule a retry"); },
      trustedHostedDocumentKeys: {[hostedDocumentTestKeyId]: hostedDocumentTestPublicKey},
      runtime: clientAttachmentRuntime,
    },
  ), error => error instanceof ApiError && error.status === 403);
  assert.equal(calls, 1);
});

test("route discovery surfaces Retry-After that exceeds the foreground deadline", async () => {
  let calls = 0;
  await assert.rejects(discoverHostedServerRoute(
    {...testServerIdentity(), id: "srv_retry_later", name: "Retry later", preferredAuthMode: "portico"},
    {
      hostedClient: {
        routes: async () => {
          calls += 1;
          throw new ApiError(429, "hosted_busy", "Busy", undefined, {retryable: true, retryAfter: "10", retryAfterMs: 10_000});
        },
      },
      retryDelaysMs: [1, 1],
      retryCohort: "persisted-installation-2",
      discoveryDeadlineAt: Date.now() + 100,
      trustedHostedDocumentKeys: {[hostedDocumentTestKeyId]: hostedDocumentTestPublicKey},
      runtime: clientAttachmentRuntime,
    },
  ), error => error instanceof HostedRouteRetryLaterError &&
    error.retryAfterMs === 10_000 &&
    !error.message.includes("Hosted Services"));
  assert.equal(calls, 1);
});

test("route discovery aborts an in-flight route request at its one absolute deadline", async () => {
  let calls = 0;
  await assert.rejects(discoverHostedServerRoute(
    {...testServerIdentity(), id: "srv_route_deadline", name: "Route deadline", preferredAuthMode: "portico"},
    {
      hostedClient: {
        routes: async (_serverId, init) => {
          calls += 1;
          return new Promise((_resolve, reject) => {
            init.signal.addEventListener("abort", () => reject(init.signal.reason), {once: true});
          });
        },
      },
      discoveryDeadlineAt: Date.now() + 10,
      trustedHostedDocumentKeys: {[hostedDocumentTestKeyId]: hostedDocumentTestPublicKey},
      runtime: clientAttachmentRuntime,
    },
  ), HostedRouteDiscoveryTimeoutError);
  assert.equal(calls, 1);
});

test("route discovery deadline settles a never-resolving custom retry delay", async () => {
  let delayStarted = false;
  await assert.rejects(discoverHostedServerRoute(
    {...testServerIdentity(), id: "srv_delay_deadline", name: "Delay deadline", preferredAuthMode: "portico"},
    {
      hostedClient: {
        routes: async () => { throw new PorticoTransportError("hosted_transport_failed", "offline", new TypeError("offline"), {method: "GET", retryable: true}); },
      },
      retryDelaysMs: [1],
      retryDelay: async () => { delayStarted = true; return new Promise(() => {}); },
      discoveryDeadlineAt: Date.now() + 25,
      trustedHostedDocumentKeys: {[hostedDocumentTestKeyId]: hostedDocumentTestPublicKey},
      runtime: clientAttachmentRuntime,
    },
  ), HostedRouteDiscoveryTimeoutError);
  assert.equal(delayStarted, true);
});

test("route discovery caller cancellation settles a custom retry delay and removes its listener", async () => {
  const controller = new AbortController();
  let retryStarted;
  const started = new Promise(resolve => { retryStarted = resolve; });
  let callerListeners = 0;
  const originalAdd = controller.signal.addEventListener.bind(controller.signal);
  const originalRemove = controller.signal.removeEventListener.bind(controller.signal);
  controller.signal.addEventListener = (...args) => {
    if (args[0] === "abort") callerListeners += 1;
    return originalAdd(...args);
  };
  controller.signal.removeEventListener = (...args) => {
    if (args[0] === "abort") callerListeners -= 1;
    return originalRemove(...args);
  };
  const cancellation = new Error("newer server selected");
  const discovery = discoverHostedServerRoute(
    {...testServerIdentity(), id: "srv_delay_cancel", name: "Delay cancel", preferredAuthMode: "portico"},
    {
      hostedClient: {
        routes: async () => { throw new PorticoTransportError("hosted_transport_failed", "offline", new TypeError("offline"), {method: "GET", retryable: true}); },
      },
      retryDelaysMs: [1000],
      retryDelay: async () => { retryStarted(); return new Promise(() => {}); },
      signal: controller.signal,
      trustedHostedDocumentKeys: {[hostedDocumentTestKeyId]: hostedDocumentTestPublicKey},
      runtime: clientAttachmentRuntime,
    },
  );
  await started;
  controller.abort(cancellation);
  await assert.rejects(discovery, error => error === cancellation);
  assert.equal(callerListeners, 0);
});

test("route discovery does not start a queued custom delay after its signal settles", async () => {
  const cancellation = new Error("connection selection canceled before retry callback");
  let aborted = false;
  let abortReason;
  let delayCalls = 0;
  const listeners = new Set();
  const signal = {
    get aborted() { return aborted; },
    get reason() { return abortReason; },
    addEventListener(type, listener) {
      if (type !== "abort") return;
      listeners.add(listener);
      if (!aborted) {
        aborted = true;
        abortReason = cancellation;
        listener.call(signal, new Event("abort"));
      }
    },
    removeEventListener(type, listener) {
      if (type === "abort") listeners.delete(listener);
    },
  };
  const controller = {
    signal,
    abort(reason = new DOMException("Aborted", "AbortError")) {
      if (aborted) return;
      aborted = true;
      abortReason = reason;
      for (const listener of [...listeners]) listener.call(signal, new Event("abort"));
    },
  };
  await assert.rejects(discoverHostedServerRoute(
    {...testServerIdentity(), id: "srv_queued_delay_cancel", name: "Queued delay cancel", preferredAuthMode: "portico"},
    {
      hostedClient: {
        routes: async () => { throw new PorticoTransportError("hosted_transport_failed", "offline", new TypeError("offline"), {method: "GET", retryable: true}); },
      },
      retryDelaysMs: [1000],
      retryDelay: async () => { delayCalls += 1; },
      trustedHostedDocumentKeys: {[hostedDocumentTestKeyId]: hostedDocumentTestPublicKey},
      runtime: {...clientAttachmentRuntime, createAbortController: () => controller},
    },
  ), error => error === cancellation);
  assert.equal(delayCalls, 0);
  assert.equal(listeners.size, 0);
});

test("route discovery clamps oversized deadlines and filters invalid delay caps before slicing", async () => {
  const server = {...testServerIdentity(), id: "srv_normalized_retry", name: "Normalized retry", preferredAuthMode: "portico"};
  const document = signedRouteDocument({
    serverId: server.id, serverName: server.name, assignedHostname: "normalized.direct.getportico.tv",
    issuedAt: "2026-05-23T00:00:00Z", expiresAt: "2026-05-23T00:05:00Z", authModes: ["portico"],
    certificate: {status: "valid"}, membership: {role: "owner"},
    routes: [{type: "public_direct", url: "https://normalized.direct.getportico.tv", quality: "reachable"}],
  });
  let calls = 0;
  const waits = [];
  const timerDelays = [];
  const runtime = {
    ...clientAttachmentRuntime,
    setTimeout(callback, milliseconds) {
      timerDelays.push(milliseconds);
      return setTimeout(callback, milliseconds);
    },
    clearTimeout(handle) { clearTimeout(handle); },
  };
  await discoverHostedServerRoute(server, {
    hostedClient: {
      routes: async () => {
        calls += 1;
        if (calls < 3) throw new PorticoTransportError("hosted_transport_failed", "offline", new TypeError("offline"), {method: "GET", retryable: true});
        return document;
      },
    },
    routeProbeFetch: async () => jsonResponse({serverId: server.id, serverPublicKeyFingerprint: testServerPublicKeyFingerprint, remoteAccessEnabled: true}),
    retryDelaysMs: [Number.NaN, -5, 10, 20, 30],
    retryDelay: async milliseconds => { waits.push(milliseconds); },
    retryCohort: "normalized-installation",
    discoveryDeadlineAt: Date.now() + 10 * 60_000,
    trustedHostedDocumentKeys: {[hostedDocumentTestKeyId]: hostedDocumentTestPublicKey},
    runtime,
    now: () => new Date("2026-05-23T00:01:00Z"),
  });
  assert.equal(calls, 3);
  assert.equal(waits.length, 2);
  assert.ok(waits[0] >= 1 && waits[0] <= 10);
  assert.ok(waits[1] >= 1 && waits[1] <= 20);
  assert.ok(timerDelays[0] > 0 && timerDelays[0] <= 30_000);
});

test("route discovery retries transient media-server 503 and 429 responses with Retry-After", async () => {
  const server = {...testServerIdentity(), id: "srv_probe_retry", name: "Probe retry", preferredAuthMode: "portico"};
  const document = signedRouteDocument({
    serverId: server.id, serverName: server.name, assignedHostname: "probe-retry.direct.getportico.tv",
    issuedAt: "2026-05-23T00:00:00Z", expiresAt: "2026-05-23T00:05:00Z", authModes: ["portico"],
    certificate: {status: "valid"}, membership: {role: "owner"},
    routes: [{type: "public_direct", url: "https://probe-retry.direct.getportico.tv", quality: "reachable"}],
  });
  for (const status of [503, 429]) {
    let probes = 0;
    let routes = 0;
    const waits = [];
    const cleanupOrder = [];
    await discoverHostedServerRoute(server, {
      hostedClient: {routes: async () => { routes += 1; return document; }},
      routeProbeFetch: async () => {
        probes += 1;
        if (probes === 1) return new Response(new ReadableStream({
          cancel() { cleanupOrder.push("body-canceled"); },
        }), {status, headers: {"Retry-After": "0.05"}});
        return jsonResponse({serverId: server.id, serverPublicKeyFingerprint: testServerPublicKeyFingerprint, remoteAccessEnabled: true});
      },
      retryDelaysMs: [100],
      retryDelay: async milliseconds => {
        cleanupOrder.push("retry-delay");
        waits.push(milliseconds);
      },
      retryCohort: `probe-${status}`,
      trustedHostedDocumentKeys: {[hostedDocumentTestKeyId]: hostedDocumentTestPublicKey},
      runtime: clientAttachmentRuntime,
      now: () => new Date("2026-05-23T00:01:00Z"),
    });
    assert.equal(routes, 2);
    assert.equal(probes, 2);
    assert.ok(waits[0] >= 51 && waits[0] <= 150, `status ${status} wait=${waits[0]}`);
    assert.deepEqual(cleanupOrder, ["body-canceled", "retry-delay"]);
  }
});

test("route discovery keeps media-server authorization and identity failures terminal", async () => {
  const server = {...testServerIdentity(), id: "srv_probe_terminal", name: "Probe terminal", preferredAuthMode: "portico"};
  const document = signedRouteDocument({
    serverId: server.id, serverName: server.name, assignedHostname: "probe-terminal.direct.getportico.tv",
    issuedAt: "2026-05-23T00:00:00Z", expiresAt: "2026-05-23T00:05:00Z", authModes: ["portico"],
    certificate: {status: "valid"}, membership: {role: "owner"},
    routes: [{type: "public_direct", url: "https://probe-terminal.direct.getportico.tv", quality: "reachable"}],
  });
	for (const response of [
		() => new Response("forbidden", {status: 403}),
		() => jsonResponse({serverId: "wrong-server", serverPublicKeyFingerprint: testServerPublicKeyFingerprint, remoteAccessEnabled: true}),
		() => jsonResponse({serverId: server.id, serverPublicKeyFingerprint: "sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", remoteAccessEnabled: true}),
	]) {
    let routes = 0;
    await assert.rejects(discoverHostedServerRoute(server, {
      hostedClient: {routes: async () => { routes += 1; return document; }},
      routeProbeFetch: async () => response(),
      retryDelaysMs: [100, 100],
      retryDelay: async () => { throw new Error("terminal probe failures must not retry"); },
      trustedHostedDocumentKeys: {[hostedDocumentTestKeyId]: hostedDocumentTestPublicKey},
      runtime: clientAttachmentRuntime,
      now: () => new Date("2026-05-23T00:01:00Z"),
    }));
    assert.equal(routes, 1);
  }
});

test("route-failure reporting sends only opaque bounded evidence and deduplicates a route generation", async () => {
  const reports = [];
  const document = signedRouteDocument({
    endpointGeneration: 23,
    serverId: "srv_private_failure",
    serverName: "Private failure",
    assignedHostname: "private-failure.direct.getportico.tv",
    issuedAt: "2026-05-23T00:00:00Z",
    expiresAt: "2026-05-23T00:05:00Z",
    authModes: ["portico"],
    certificate: {status: "valid"},
    membership: {role: "owner"},
    routes: [{
      type: "public_direct",
      url: "https://private-failure.direct.getportico.tv:32500",
      host: "private-failure.direct.getportico.tv",
      address: "203.0.113.222",
      quality: "reachable"
    }]
  });
  const options = {
    hostedClient: {
      routes: async () => document,
      reportRouteFailure: async (_serverId, body) => { reports.push(structuredClone(body)); return {ok: true, matched: true}; }
    },
    routeProbeFetch: async () => { throw new TypeError("raw TLS failure at 203.0.113.222"); },
    retryDelaysMs: [],
    routePreference: "public-only",
    trustedHostedDocumentKeys: {[hostedDocumentTestKeyId]: hostedDocumentTestPublicKey},
    runtime: clientAttachmentRuntime,
    now: () => new Date("2026-05-23T00:01:00Z")
  };
  const server = {...testServerIdentity(), id: "srv_private_failure", name: "Private failure", preferredAuthMode: "portico"};
  await assert.rejects(discoverHostedServerRoute(server, options), /secure connection/i);
  await new Promise(resolve => setImmediate(resolve));
  await assert.rejects(discoverHostedServerRoute(server, options), /secure connection/i);
  await new Promise(resolve => setImmediate(resolve));

  assert.deepEqual(reports, [{routeType: "public_direct", endpointGeneration: 23, category: "transport_failed"}]);
  const encoded = JSON.stringify(reports);
  for (const privateValue of ["private-failure.direct.getportico.tv", "203.0.113.222", "raw TLS failure", "https://"]) {
    assert.equal(encoded.includes(privateValue), false);
  }
});

test("hosted connector never retries route-document integrity failures", async () => {
  let routeCalls = 0;
  const invalidDocument = signedRouteDocument({
    serverId: "srv_integrity",
    serverName: "Integrity",
    assignedHostname: "integrity.direct.getportico.tv",
    issuedAt: "2026-05-23T00:00:00Z",
    expiresAt: "2026-05-23T00:05:00Z",
    serverPublicKeyFingerprint: testServerPublicKeyFingerprint,
    authModes: ["portico"],
    certificate: { status: "valid" },
    membership: { role: "owner" },
    routes: [{
      type: "public_direct",
      url: "https://integrity.direct.getportico.tv",
      host: "integrity.direct.getportico.tv",
      address: "198.51.100.91",
      port: 443,
      scheme: "https",
		quality: "probe_required"
    }]
  });
  invalidDocument.signature = "invalid-signature";
  const hostedClient = {
    routes: async () => { routeCalls++; return invalidDocument; },
    reportRouteFailure: async () => ({ ok: true, matched: true })
  };

  await assert.rejects(connectHostedServer({ ...testServerIdentity(), id: "srv_integrity", name: "Integrity", preferredAuthMode: "portico" }, {
    ...hostedProfileBinding("srv_integrity"),
    hostedClient,
    localClient: compatibleLocalClient("srv_integrity", "https://integrity.direct.getportico.tv"),
    sessionStore: { set: () => {}, clear: () => {} },
    retryDelaysMs: [0, 0],
    retryDelay: async () => {},
    trustedHostedDocumentKeys: { [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey },
    runtime: { ...clientAttachmentRuntime, verifyEd25519: () => false },
    now: () => new Date("2026-05-23T00:01:00Z")
  }), /signature/i);

  assert.equal(routeCalls, 1);
});

test("hosted connector uses LAN without probing a public route when LAN is healthy", async () => {
  let verifyCalls = 0;
  let sessionSet = null;
  const hostedClient = {
    routes: async () => signedRouteDocument({
      serverId: "srv_lan",
      serverName: "LAN",
      assignedHostname: "lan.direct.getportico.tv",
      issuedAt: "2026-05-23T00:00:00Z",
      expiresAt: "2026-05-23T00:05:00Z",
      serverPublicKeyFingerprint: "sha256:server",
      authModes: ["portico"],
      certificate: { status: "valid" },
      membership: { role: "owner" },
      routes: [
		{ type: "public_direct", url: "https://public.direct.getportico.tv", host: "public.direct.getportico.tv", port: 443, scheme: "https", quality: "reachable" },
		{ type: "lan_ip_encoded", url: "https://10-0-0-5.ptc.direct.getportico.tv:32500", host: "10-0-0-5.ptc.direct.getportico.tv", address: "10.0.0.5", port: 32500, scheme: "https", quality: "probe_required" }
      ]
    }),
    porticoSession: async () => ({ tokenType: "Bearer", accessToken: "portico-token", accessExpiresAt: "2026-05-23T01:00:00Z", refreshToken: "refresh-token", refreshExpiresAt: "2026-06-23T00:00:00Z" }),
    reportRouteFailure: async () => ({ ok: true, matched: true })
  };
  const localClient = compatibleLocalClient("srv_lan", "https://10-0-0-5.ptc.direct.getportico.tv:32500");

  await connectHostedServer({ ...testServerIdentity(), id: "srv_lan", name: "LAN", preferredAuthMode: "portico" }, {
    ...hostedProfileBinding("srv_lan"),
    hostedClient,
    localClient,
    sessionStore: {
      set: (session) => { sessionSet = session; },
      clear: () => { sessionSet = null; }
    },
    routeProbeFetch: async (input) => {
      verifyCalls++;
      if (String(input).startsWith("https://public.direct.getportico.tv")) {
        throw new TypeError("public route unavailable from this network");
      }
      return jsonResponse({
        serverId: "srv_lan",
        serverPublicKeyFingerprint: testServerPublicKeyFingerprint,
        remoteAccessEnabled: true
      });
    },
    retryDelaysMs: [],
    retryDelay: async () => {},
    trustedHostedDocumentKeys: { [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey },
    runtime: clientAttachmentRuntime,
    now: () => new Date("2026-05-23T00:01:00Z")
  });

  assert.equal(verifyCalls, 1);
  assert.equal(sessionSet.apiBaseUrl, "https://10-0-0-5.ptc.direct.getportico.tv:32500");
});

test("browser public-first routing never probes LAN while a verified public route works", async () => {
  const calls = [];
  let sessionSet;
  const hostedClient = {
    routes: async () => signedRouteDocument({
      serverId: "srv_browser_public",
      serverName: "Browser Public",
      assignedHostname: "browser-public.direct.getportico.tv",
      issuedAt: "2026-05-23T00:00:00Z",
      expiresAt: "2026-05-23T00:05:00Z",
      serverPublicKeyFingerprint: "sha256:server",
      authModes: ["portico"],
      certificate: { status: "valid" },
      membership: { role: "owner" },
      routes: [
		{ type: "lan", url: "https://192.168.1.10:32500", quality: "probe_required" },
        { type: "public_direct", url: "https://browser-public.direct.getportico.tv", quality: "reachable" }
      ]
    }),
    porticoSession: async () => ({ tokenType: "Bearer", accessToken: "access", accessExpiresAt: "2026-05-23T01:00:00Z", refreshToken: "refresh", refreshExpiresAt: "2026-06-23T00:00:00Z" }),
    reportRouteFailure: async () => ({ ok: true, matched: true })
  };
  await connectHostedServer({ ...testServerIdentity(), id: "srv_browser_public", name: "Browser Public", preferredAuthMode: "portico" }, {
    ...hostedProfileBinding("srv_browser_public"),
    hostedClient,
    localClient: compatibleLocalClient("srv_browser_public", "https://browser-public.direct.getportico.tv"),
    sessionStore: { set: (session) => { sessionSet = session; }, clear: () => {} },
    routeProbeFetch: async (input) => {
      const url = String(input);
      calls.push(url);
      if (url.includes("192.168.1.10")) throw new Error("LAN must not be probed while public direct is healthy");
      return jsonResponse({ serverId: "srv_browser_public", serverPublicKeyFingerprint: testServerPublicKeyFingerprint, remoteAccessEnabled: true });
    },
    routePreference: "public-first",
    retryDelaysMs: [],
    trustedHostedDocumentKeys: { [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey },
    runtime: clientAttachmentRuntime,
    now: () => new Date("2026-05-23T00:01:00Z")
  });

  assert.deepEqual(calls, ["https://browser-public.direct.getportico.tv/api/remote-access/health"]);
  assert.equal(sessionSet.apiBaseUrl, "https://browser-public.direct.getportico.tv");
});

test("browser public routing falls back across independently verified address families", async () => {
  const calls = [];
  let sessionSet;
  const ipv6Route = "https://2001-db8--10-mac.ptc-dual.direct.getportico.tv:32500";
  const ipv4Route = "https://198-51-100-10-mac.ptc-dual.direct.getportico.tv:32500";
  const hostedClient = {
    routes: async () => signedRouteDocument({
      serverId: "srv_dual_public",
      serverName: "Dual Public",
      assignedHostname: "ptc-dual.direct.getportico.tv",
      issuedAt: "2026-05-23T00:00:00Z",
      expiresAt: "2026-05-23T00:05:00Z",
      serverPublicKeyFingerprint: testServerPublicKeyFingerprint,
      authModes: ["portico"],
      certificate: { status: "valid" },
      membership: { role: "owner" },
      routes: [
        { type: "public_direct_ip_encoded", url: ipv6Route, quality: "reachable" },
        { type: "public_direct_ip_encoded", url: ipv4Route, quality: "reachable" }
      ]
    }),
    porticoSession: async () => ({ tokenType: "Bearer", accessToken: "access", accessExpiresAt: "2026-05-23T01:00:00Z", refreshToken: "refresh", refreshExpiresAt: "2026-06-23T00:00:00Z" }),
    reportRouteFailure: async () => ({ ok: true, matched: true })
  };
  await connectHostedServer({ ...testServerIdentity(), id: "srv_dual_public", name: "Dual Public", preferredAuthMode: "portico" }, {
    ...hostedProfileBinding("srv_dual_public"),
    hostedClient,
    localClient: compatibleLocalClient("srv_dual_public", ipv4Route),
    sessionStore: { set: (session) => { sessionSet = session; }, clear: () => {} },
    routeProbeFetch: async (input) => {
      const url = String(input);
      calls.push(url);
      if (url.startsWith(ipv6Route)) throw new TypeError("IPv6 is unavailable on this client");
      return jsonResponse({ serverId: "srv_dual_public", serverPublicKeyFingerprint: testServerPublicKeyFingerprint, remoteAccessEnabled: true });
    },
    routePreference: "public-first",
    retryDelaysMs: [],
    trustedHostedDocumentKeys: { [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey },
    runtime: clientAttachmentRuntime,
    now: () => new Date("2026-05-23T00:01:00Z")
  });

  assert.equal(calls.length, 2);
  assert.ok(calls.includes(`${ipv6Route}/api/remote-access/health`));
  assert.ok(calls.includes(`${ipv4Route}/api/remote-access/health`));
  assert.equal(sessionSet.apiBaseUrl, ipv4Route);
});

test("browser public routing falls through failed direct addresses to the verified public console origin", async () => {
  const calls = [];
  const reports = [];
  let sessionSet;
  const ipv4Route = "https://198-51-100-12-mac.ptc-console.direct.getportico.tv:32500";
  const ipv6Route = "https://2001-db8--12-mac.ptc-console.direct.getportico.tv:32500";
  const consoleRoute = "https://demo.getportico.tv";
  const hostedClient = {
    routes: async () => signedRouteDocument({
      serverId: "srv_console_fallback", serverName: "Console Fallback", assignedHostname: "ptc-console.direct.getportico.tv",
      issuedAt: "2026-05-23T00:00:00Z", expiresAt: "2026-05-23T00:05:00Z",
      serverPublicKeyFingerprint: testServerPublicKeyFingerprint, authModes: ["portico"], certificate: {status: "valid"},
      membership: {role: "owner"}, routes: [
        {type: "public_direct_ip_encoded", url: ipv4Route, quality: "reachable"},
        {type: "public_direct_ip_encoded", url: ipv6Route, quality: "reachable"},
        {type: "public_console_origin", url: consoleRoute, quality: "reachable"}
      ]
    }),
    porticoSession: async () => ({tokenType: "Bearer", accessToken: "access", accessExpiresAt: "2026-05-23T01:00:00Z", refreshToken: "refresh", refreshExpiresAt: "2026-06-23T00:00:00Z"}),
    reportRouteFailure: async (_serverId, report) => { reports.push(report); return {ok: true, matched: true}; }
  };
  await connectHostedServer({...testServerIdentity(), id: "srv_console_fallback", name: "Console Fallback", preferredAuthMode: "portico"}, {
    ...hostedProfileBinding("srv_console_fallback"), hostedClient,
    localClient: compatibleLocalClient("srv_console_fallback", consoleRoute),
    sessionStore: {set: (session) => { sessionSet = session; }, clear: () => {}},
    routeProbeFetch: async (input) => {
      const url = String(input); calls.push(url);
      if (!url.startsWith(consoleRoute)) throw new TypeError("direct address unavailable");
      return jsonResponse({serverId: "srv_console_fallback", serverPublicKeyFingerprint: testServerPublicKeyFingerprint, remoteAccessEnabled: true});
    },
    routePreference: "public-first", retryDelaysMs: [],
    trustedHostedDocumentKeys: {[hostedDocumentTestKeyId]: hostedDocumentTestPublicKey}, runtime: clientAttachmentRuntime,
    now: () => new Date("2026-05-23T00:01:00Z")
  });
  assert.equal(sessionSet.apiBaseUrl, consoleRoute);
  assert.ok(calls.includes(`${consoleRoute}/api/remote-access/health`));
  assert.ok(reports.length >= 1);
  assert.ok(reports.every((report) => report.routeType === "public_direct_ip_encoded"));
});

test("browser public routing also falls back from IPv4 to IPv6", async () => {
  const calls = [];
  let sessionSet;
  const ipv4Route = "https://198-51-100-11-mac.ptc-dual-inverse.direct.getportico.tv:32500";
  const ipv6Route = "https://2001-db8--11-mac.ptc-dual-inverse.direct.getportico.tv:32500";
  const hostedClient = {
    routes: async () => signedRouteDocument({
      serverId: "srv_dual_public_inverse",
      serverName: "Dual Public Inverse",
      assignedHostname: "ptc-dual-inverse.direct.getportico.tv",
      issuedAt: "2026-05-23T00:00:00Z",
      expiresAt: "2026-05-23T00:05:00Z",
      serverPublicKeyFingerprint: testServerPublicKeyFingerprint,
      authModes: ["portico"],
      certificate: { status: "valid" },
      membership: { role: "owner" },
      routes: [
        { type: "public_direct_ip_encoded", url: ipv4Route, quality: "reachable" },
        { type: "public_direct_ip_encoded", url: ipv6Route, quality: "reachable" }
      ]
    }),
    porticoSession: async () => ({ tokenType: "Bearer", accessToken: "access", accessExpiresAt: "2026-05-23T01:00:00Z", refreshToken: "refresh", refreshExpiresAt: "2026-06-23T00:00:00Z" }),
    reportRouteFailure: async () => ({ ok: true, matched: true })
  };
  await connectHostedServer({ ...testServerIdentity(), id: "srv_dual_public_inverse", name: "Dual Public Inverse", preferredAuthMode: "portico" }, {
    ...hostedProfileBinding("srv_dual_public_inverse"),
    hostedClient,
    localClient: compatibleLocalClient("srv_dual_public_inverse", ipv6Route),
    sessionStore: { set: (session) => { sessionSet = session; }, clear: () => {} },
    routeProbeFetch: async (input) => {
      const url = String(input);
      calls.push(url);
      if (url.startsWith(ipv4Route)) throw new TypeError("IPv4 is unavailable on this client");
      return jsonResponse({ serverId: "srv_dual_public_inverse", serverPublicKeyFingerprint: testServerPublicKeyFingerprint, remoteAccessEnabled: true });
    },
    routePreference: "public-first",
    retryDelaysMs: [],
    trustedHostedDocumentKeys: { [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey },
    runtime: clientAttachmentRuntime,
    now: () => new Date("2026-05-23T00:01:00Z")
  });

  assert.equal(calls.length, 2);
  assert.ok(calls.includes(`${ipv4Route}/api/remote-access/health`));
  assert.ok(calls.includes(`${ipv6Route}/api/remote-access/health`));
  assert.equal(sessionSet.apiBaseUrl, ipv6Route);
});

test("browser public-first routing falls back to LAN only after public verification fails", async () => {
  const calls = [];
  let sessionSet;
  const hostedClient = {
    routes: async () => signedRouteDocument({
      serverId: "srv_browser_fallback",
      serverName: "Browser Fallback",
      assignedHostname: "browser-fallback.direct.getportico.tv",
      issuedAt: "2026-05-23T00:00:00Z",
      expiresAt: "2026-05-23T00:05:00Z",
      serverPublicKeyFingerprint: "sha256:server",
      authModes: ["portico"],
      certificate: { status: "valid" },
      membership: { role: "owner" },
      routes: [
		{ type: "lan", url: "https://192.168.1.10:32500", quality: "probe_required" },
        { type: "public_direct", url: "https://browser-fallback.direct.getportico.tv", quality: "reachable" }
      ]
    }),
    porticoSession: async () => ({ tokenType: "Bearer", accessToken: "access", accessExpiresAt: "2026-05-23T01:00:00Z", refreshToken: "refresh", refreshExpiresAt: "2026-06-23T00:00:00Z" }),
    reportRouteFailure: async () => ({ ok: true, matched: true })
  };
  await connectHostedServer({ ...testServerIdentity(), id: "srv_browser_fallback", name: "Browser Fallback", preferredAuthMode: "portico" }, {
    ...hostedProfileBinding("srv_browser_fallback"),
    hostedClient,
    localClient: compatibleLocalClient("srv_browser_fallback", "https://192.168.1.10:32500"),
    sessionStore: { set: (session) => { sessionSet = session; }, clear: () => {} },
    routeProbeFetch: async (input) => {
      const url = String(input);
      calls.push(url);
      if (url.includes("browser-fallback.direct")) throw new TypeError("public route unavailable");
      return jsonResponse({ serverId: "srv_browser_fallback", serverPublicKeyFingerprint: testServerPublicKeyFingerprint, remoteAccessEnabled: true });
    },
    routePreference: "public-first",
    retryDelaysMs: [],
    trustedHostedDocumentKeys: { [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey },
    runtime: clientAttachmentRuntime,
    now: () => new Date("2026-05-23T00:01:00Z")
  });

  assert.deepEqual(calls, [
    "https://browser-fallback.direct.getportico.tv/api/remote-access/health",
    "https://192.168.1.10:32500/api/remote-access/health"
  ]);
  assert.equal(sessionSet.apiBaseUrl, "https://192.168.1.10:32500");
});

test("browser public-only routing offers nearby recovery without probing LAN", async () => {
  const calls = [];
  const hostedClient = {
    routes: async () => signedRouteDocument({
      serverId: "srv_browser_nearby",
      serverName: "Browser Nearby",
      assignedHostname: "browser-nearby.direct.getportico.tv",
      issuedAt: "2026-05-23T00:00:00Z",
      expiresAt: "2026-05-23T00:05:00Z",
      serverPublicKeyFingerprint: "sha256:server",
      authModes: ["portico"],
      certificate: { status: "valid" },
      membership: { role: "owner" },
      routes: [
		{ type: "lan", url: "https://192.168.1.10:32500", quality: "probe_required" },
        { type: "public_direct", url: "https://browser-nearby.direct.getportico.tv", quality: "reachable" }
      ]
    }),
    porticoSession: async () => { throw new Error("credentials must not be minted without a verified route"); },
    reportRouteFailure: async () => ({ ok: true, matched: true })
  };

  await assert.rejects(() => connectHostedServer({ ...testServerIdentity(), id: "srv_browser_nearby", name: "Browser Nearby", preferredAuthMode: "portico" }, {
    ...hostedProfileBinding("srv_browser_nearby"),
    hostedClient,
    localClient: compatibleLocalClient("srv_browser_nearby", "https://browser-nearby.direct.getportico.tv"),
    sessionStore: { set: () => {}, clear: () => {} },
    routeProbeFetch: async (input) => {
      const url = String(input);
      calls.push(url);
      if (url.includes("192.168.1.10")) throw new Error("public-only mode must not probe LAN");
      throw new TypeError("public route unavailable");
    },
    routePreference: "public-only",
    retryDelaysMs: [],
    trustedHostedDocumentKeys: { [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey },
    runtime: clientAttachmentRuntime,
    now: () => new Date("2026-05-23T00:01:00Z")
  }), NearbyRouteAvailableError);

  assert.deepEqual(calls, ["https://browser-nearby.direct.getportico.tv/api/remote-access/health"]);
});

test("browser lan-only recovery reports Local Network denial without probing public routes", async () => {
  const calls = [];
  const hostedClient = {
    routes: async () => signedRouteDocument({
      serverId: "srv_browser_lan_denied",
      serverName: "Browser LAN Denied",
      assignedHostname: "browser-lan-denied.direct.getportico.tv",
      issuedAt: "2026-05-23T00:00:00Z",
      expiresAt: "2026-05-23T00:05:00Z",
      serverPublicKeyFingerprint: "sha256:server",
      authModes: ["portico"],
      certificate: { status: "valid" },
      membership: { role: "owner" },
      routes: [
		{ type: "lan", url: "https://192.168.1.10:32500", quality: "probe_required" },
        { type: "public_direct", url: "https://browser-lan-denied.direct.getportico.tv", quality: "reachable" }
      ]
    }),
    porticoSession: async () => { throw new Error("credentials must not be minted without a verified route"); },
    reportRouteFailure: async () => ({ ok: true, matched: true })
  };

  await assert.rejects(() => connectHostedServer({ ...testServerIdentity(), id: "srv_browser_lan_denied", name: "Browser LAN Denied", preferredAuthMode: "portico" }, {
    ...hostedProfileBinding("srv_browser_lan_denied"),
    hostedClient,
    localClient: compatibleLocalClient("srv_browser_lan_denied", "https://192.168.1.10:32500"),
    sessionStore: { set: () => {}, clear: () => {} },
    routeProbeFetch: async (input) => {
      const url = String(input);
      calls.push(url);
      if (url.includes("browser-lan-denied.direct")) throw new Error("lan-only mode must not probe public routes");
      throw new TypeError("Local Network permission denied");
    },
    routePreference: "lan-only",
    retryDelaysMs: [],
    trustedHostedDocumentKeys: { [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey },
    runtime: clientAttachmentRuntime,
    now: () => new Date("2026-05-23T00:01:00Z")
  }), LocalNetworkRouteUnavailableError);

  assert.deepEqual(calls, ["https://192.168.1.10:32500/api/remote-access/health"]);
});

test("hosted connector probes LAN candidates in a bounded parallel batch before public fallback", async () => {
  let activeProbes = 0;
  let maxActiveProbes = 0;
  const calls = [];
  let sessionSet;
  const hostedClient = {
    routes: async () => signedRouteDocument({
      serverId: "srv_parallel",
      serverName: "Parallel",
      assignedHostname: "parallel.direct.getportico.tv",
      issuedAt: "2026-05-23T00:00:00Z",
      expiresAt: "2026-05-23T00:05:00Z",
      serverPublicKeyFingerprint: "sha256:server",
      authModes: ["portico"],
      certificate: { status: "valid" },
      membership: { role: "owner" },
      routes: [
		{ type: "lan", url: "https://192.168.1.10:32500", quality: "probe_required" },
		{ type: "lan", url: "https://192.168.1.11:32500", quality: "probe_required" },
        { type: "public_direct", url: "https://parallel.direct.getportico.tv", quality: "reachable" }
      ]
    }),
    porticoSession: async () => ({ tokenType: "Bearer", accessToken: "access", accessExpiresAt: "2026-05-23T01:00:00Z", refreshToken: "refresh", refreshExpiresAt: "2026-06-23T00:00:00Z" }),
    reportRouteFailure: async () => ({ ok: true, matched: true })
  };
  await connectHostedServer({ ...testServerIdentity(), id: "srv_parallel", name: "Parallel", preferredAuthMode: "portico" }, {
    ...hostedProfileBinding("srv_parallel"),
    hostedClient,
    localClient: compatibleLocalClient("srv_parallel", "https://parallel.direct.getportico.tv"),
    sessionStore: { set: (session) => { sessionSet = session; }, clear: () => {} },
    routeProbeFetch: async (input) => {
      const url = String(input);
      calls.push(url);
      activeProbes++;
      maxActiveProbes = Math.max(maxActiveProbes, activeProbes);
      await new Promise((resolve) => setTimeout(resolve, 5));
      activeProbes--;
      if (url.includes("192.168.1.")) throw new TypeError("LAN unavailable");
      return jsonResponse({ serverId: "srv_parallel", serverPublicKeyFingerprint: testServerPublicKeyFingerprint, remoteAccessEnabled: true });
    },
    retryDelaysMs: [],
    maxParallelRouteProbes: 2,
    trustedHostedDocumentKeys: { [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey },
    runtime: clientAttachmentRuntime,
    now: () => new Date("2026-05-23T00:01:00Z")
  });

  assert.equal(maxActiveProbes, 2);
  assert.equal(calls.filter((url) => url.includes("192.168.1.")).length, 2);
  assert.equal(calls.at(-1).startsWith("https://parallel.direct.getportico.tv"), true);
  assert.equal(sessionSet.apiBaseUrl, "https://parallel.direct.getportico.tv");
});

test("a wrong-identity LAN candidate cannot suppress a valid public route", async () => {
  let sessionSet;
  const publicRoute = "https://identity-race.direct.getportico.tv";
  const hostedClient = {
    routes: async () => signedRouteDocument({
      serverId: "srv_identity_race",
      serverName: "Identity race",
      assignedHostname: "identity-race.direct.getportico.tv",
      issuedAt: "2026-05-23T00:00:00Z",
      expiresAt: "2026-05-23T00:05:00Z",
      authModes: ["portico"],
      certificate: { status: "valid" },
      membership: { role: "owner" },
      routes: [
		{ type: "lan", url: "https://192.168.1.50:32500", quality: "probe_required" },
        { type: "public_direct", url: publicRoute, quality: "reachable" }
      ]
    }),
    porticoSession: async () => ({ tokenType: "Bearer", accessToken: "access", accessExpiresAt: "2026-05-23T01:00:00Z", refreshToken: "refresh", refreshExpiresAt: "2026-06-23T00:00:00Z" }),
    reportRouteFailure: async () => ({ ok: true, matched: true })
  };

  await connectHostedServer({ ...testServerIdentity(), id: "srv_identity_race", name: "Identity race", preferredAuthMode: "portico" }, {
    ...hostedProfileBinding("srv_identity_race"),
    hostedClient,
    localClient: compatibleLocalClient("srv_identity_race", publicRoute),
    sessionStore: { set: (session) => { sessionSet = session; }, clear: () => {} },
    routeProbeFetch: async input => String(input).includes("192.168.1.50")
      ? jsonResponse({ serverId: "spoofed-server", serverPublicKeyFingerprint: testServerPublicKeyFingerprint, remoteAccessEnabled: true })
      : jsonResponse({ serverId: "srv_identity_race", serverPublicKeyFingerprint: testServerPublicKeyFingerprint, remoteAccessEnabled: true }),
    retryDelaysMs: [],
    trustedHostedDocumentKeys: { [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey },
    runtime: clientAttachmentRuntime,
    now: () => new Date("2026-05-23T00:01:00Z")
  });

  assert.equal(sessionSet.apiBaseUrl, publicRoute);
});

test("documented resource URL helpers do not leak account credentials", () => {
  const sessionStore = createMemorySessionStore({
    apiBaseUrl: "https://server.example",
    bootstrapAccessToken: "token-123"
  });
  const client = createPorticoClient({ sessionStore, baseHref: "https://web.example" });

  assert.equal(client.mediaStreamUrl("m 1"), "https://server.example/api/media/m%201/stream");
  assert.equal(client.mediaAttachmentUrl("m1", "font/1"), "https://server.example/api/media/m1/attachments/font%2F1");
  assert.equal(client.mediaTrickplayPlaylistUrl("m1", "set1"), "https://server.example/api/media/m1/trickplay/set1/tiles.m3u8");
  assert.equal(client.mediaTrickplayTileUrl("m1", "set1", 4), "https://server.example/api/media/m1/trickplay/set1/tiles/4.jpg");
  assert.equal(client.artworkUrl("m1", "poster.jpg", { rendition: "small" }), "https://server.example/api/artwork/m1/poster.jpg?rendition=small");
  assert.equal(client.liveTvStreamUrl("chan 1"), "https://server.example/api/live-tv/streams/chan%201");
  assert.equal(client.liveTvLogoUrl("chan1"), "https://server.example/api/live-tv/logos/chan1");
  assert.equal(client.watchWithFriendsGroupEventsUrl("group1"), "https://server.example/api/watch-with-friends/groups/group1/events");
  assert.equal(client.logsStreamUrl(), "https://server.example/api/logs/stream");
});

test("current public metadata and live TV methods use documented API paths", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    csrfToken: "csrf-local",
    playbackClientProfile: () => ({ platform: "tv-test", supportsHls: true, maxAudioChannels: 6 }),
    playbackClientInstanceId: () => "test-client",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        if (String(input).endsWith("/api/live-tv/streams/chan1/close")) {
          const request = JSON.parse(init.body);
          return jsonResponse(playbackTerminalReceipt(request.sessionId, request));
        }
        return jsonResponse({
          ok: true,
          items: [],
          authenticated: true,
          sessionId: "live1",
          media: { id: "chan1", title: "Channel", type: "live_channel", state: {} },
          sourceUrl: "/api/live-tv/streams/chan1",
          directPlay: true,
          generation: 1,
          nextEventSequence: 1,
          playbackRevision: 1,
          queueRevision: 1,
          repeatMode: "off",
          timeline: { type: "live", canPause: false, canSeek: false },
          continuationCredential: { token: "continuation", origin: "https://server.example", generation: 1, expiresAt: "2026-08-06T01:00:00Z" },
          mediaGrant: { token: "grant", expiresAt: "2026-08-06T01:00:00Z" },
          isLive: true,
          decision: { mode: "direct_play", reason: "", requiresTranscode: false, isProxied: true, isServerCached: false },
          qualityOffers: {contractId: "PC-PLAYBACK", schemaVersion: "quality-offers.v1", mediaId: "m1", versionId: "qver-m1", sourceRevision: "qsrc-m1", offerRevision: "qrev-m1", offers: [{selectionId: "qsel-auto", label: "Automatic", kind: "automatic"}, {selectionId: "qsel-original", label: "Original Quality", kind: "original"}]},
          qualitySelection: {mode: "automatic"},
          audioStreams: [],
          subtitleStreams: [],
          chapters: [],
          queue: [],
          resources: [{id: "live-active", sourceUrl: "/api/live-tv/streams/chan1", streamFormat: "hls", default: true}]
        });
      }
    }
  });

  await client.system();
  await client.branding();
  await client.remoteAccessHealth();
  await client.openLiveTvStream("chan1", { intent: { transportClass: "wifi", quality: {mode: "automatic"} } });
  await client.closeLiveTvStream("chan1", "live1", {
    disposition: "stopped", positionSeconds: 30, durationSeconds: 60,
  });
  await client.playDvrRecording("rec 1", { startSeconds: 42 });

  assert.deepEqual(calls.map((call) => call.input), [
    "https://server.example/api/system",
    "https://server.example/api/branding",
    "https://server.example/api/remote-access/health",
    "https://server.example/api/live-tv/streams/chan1/open",
    "https://server.example/api/live-tv/streams/chan1/close",
    "https://server.example/api/dvr/recordings/rec%201/playback"
  ]);
  assert.equal(JSON.parse(calls[3].init.body).clientProfile.maxAudioChannels, 6);
  assert.equal(JSON.parse(calls[3].init.body).clientInstanceId, "test-client");
  assert.deepEqual(JSON.parse(calls[3].init.body).intent, { transportClass: "wifi", quality: {mode: "automatic"} });
  assert.equal(JSON.parse(calls[4].init.body).sessionId, "live1");
  assert.equal(JSON.parse(calls[4].init.body).terminal.disposition, "stopped");
  assert.equal(typeof JSON.parse(calls[4].init.body).requestId, "string");
  assert.deepEqual(JSON.parse(calls[5].init.body), {
    startSeconds: 42,
    intent: { quality: { mode: "automatic" } },
    clientInstanceId: "test-client",
    clientProfile: { platform: "tv-test", supportsHls: true, maxAudioChannels: 6 }
  });
});

test("playback methods inject provided playback profile and accept canonical arrays", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    playbackClientProfile: () => ({ platform: "test", supportsHls: true }),
    playbackClientInstanceId: () => "web-test",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return jsonResponse({
          sessionId: "s1",
          media: { id: "m1", title: "Movie", type: "movie", state: {} },
          sourceUrl: "/api/media/m1/stream",
          directPlay: true,
          generation: 1,
          nextEventSequence: 1,
          playbackRevision: 1,
          queueRevision: 1,
          repeatMode: "off",
          timeline: { type: "vod", canPause: true, canSeek: true },
          continuationCredential: { token: "continuation", origin: "https://server.example", generation: 1, expiresAt: "2026-08-06T01:00:00Z" },
          mediaGrant: { token: "grant", expiresAt: "2026-08-06T01:00:00Z" },
          decision: { mode: "direct_play", reason: "", requiresTranscode: false, isProxied: true, isServerCached: false },
          qualityOffers: {contractId: "PC-PLAYBACK", schemaVersion: "quality-offers.v1", mediaId: "m1", versionId: "qver-m1", sourceRevision: "qsrc-m1", offerRevision: "qrev-m1", offers: [{selectionId: "qsel-auto", label: "Automatic", kind: "automatic"}, {selectionId: "qsel-original", label: "Original Quality", kind: "original"}]},
          qualitySelection: {mode: "automatic"},
          audioStreams: [],
          subtitleStreams: [],
          chapters: [],
          queue: [],
          resources: [{id: "movie-active", sourceUrl: "/api/media/m1/stream", streamFormat: "http", default: true}]
        });
      }
    }
  });

  const playback = await client.startPlayback("m1", { audioStreamId: "audio_commentary", startSeconds: 12, repeatMode: "all", versionId: "version-director" });
  assert.equal(JSON.parse(calls[0].init.body).clientProfile.platform, "test");
  assert.equal(JSON.parse(calls[0].init.body).clientInstanceId, "web-test");
  assert.equal(JSON.parse(calls[0].init.body).audioStreamId, "audio_commentary");
  assert.equal(JSON.parse(calls[0].init.body).repeatMode, "all");
  assert.equal(JSON.parse(calls[0].init.body).versionId, "version-director");
  assert.equal(playbackQualitySelectionKey(playback.qualitySelection), "automatic");
  assert.deepEqual(playbackQualitySelectionFor(playback, "qsel-original"), {mode: "explicit", selectionId: "qsel-original", qualityOfferRevision: "qrev-m1"});
});

test("playback receiver methods bind Core installation identity and preserve exact handoff reconciliation", async () => {
  const calls = [];
  const receiver = {
    id: "receiver-1",
    serverId: "server-1",
    name: "Living Room",
    app: "Portico",
    platform: "Fire TV",
    receiverPublicKey: "receiver-public-key",
    receiverPublicKeyFingerprint: "receiver-fingerprint",
    supportedCommands: ["load", "play", "pause", "seek", "stop"],
    expiresAt: "2026-08-31T22:00:00Z",
    createdAt: "2026-08-31T21:00:00Z",
    lastSeenAt: "2026-08-31T21:00:00Z",
  };
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    playbackClientInstanceId: () => "stable-installation",
    transport: {
      fetch: async (input, init = {}) => {
        const url = String(input);
        calls.push({input: url, init});
        if (url.endsWith("/commit"))
          return jsonResponse(acceptedPlayback("receiver-session-1"));
        if (url.includes("/handoffs/"))
          return jsonResponse({
            outcome: "accepted",
            requestId: "handoff-request-1",
            sourceSessionId: "source-session-1",
            receiverSessionId: "receiver-session-1",
          });
        if (url.endsWith("/handoff"))
          return jsonResponse(acceptedPlayback("receiver-session-1"));
        if (url.endsWith("/authorizations"))
          return jsonResponse({
            authorizationId: "authorization-1",
            receiverId: receiver.id,
            serverId: receiver.serverId,
            receiverPublicKey: receiver.receiverPublicKey,
            receiverPublicKeyFingerprint: receiver.receiverPublicKeyFingerprint,
            allowedCommands: receiver.supportedCommands,
            authorizationRevision: "revision-1",
            expiresAt: receiver.expiresAt,
          });
        return jsonResponse(receiver);
      },
    },
  });

  await client.registerPlaybackReceiver({
    requestId: "register-1",
    receiverId: receiver.id,
    name: receiver.name,
    app: receiver.app,
    platform: receiver.platform,
    receiverPublicKey: receiver.receiverPublicKey,
    receiverPublicKeyFingerprint: receiver.receiverPublicKeyFingerprint,
    supportedCommands: receiver.supportedCommands,
  });
  await client.authorizePlaybackReceiver(receiver.id, {
    requestId: "authorize-1",
    controllerId: "controller-1",
    controllerPublicKey: "controller-public-key",
    allowedCommands: receiver.supportedCommands,
  });
  const handoff = {
    authorizationId: "authorization-1",
    receiverPublicKeyFingerprint: receiver.receiverPublicKeyFingerprint,
    playback: {
      mediaId: "m1",
      clientProfile: {platform: "fire-tv"},
      replacement: {
        sourceSessionId: "source-session-1",
        requestId: "handoff-request-1",
        expectedQueueRevision: 3,
        expectedPlaybackRevision: 4,
        previousTerminal: {
          disposition: "stopped",
          positionSeconds: 12,
          durationSeconds: 60,
          eventSequence: 5,
          generation: 1,
          recordedAt: "2026-08-31T21:00:00Z",
        },
      },
    },
  };
  await client.handoffPlaybackToReceiver(receiver.id, handoff);
  await client.commitPlaybackReceiverHandoff(receiver.id, handoff.playback.replacement.requestId, {
    authorizationId: handoff.authorizationId,
    receiverPublicKeyFingerprint: handoff.receiverPublicKeyFingerprint,
    sourceSessionId: handoff.playback.replacement.sourceSessionId,
    receiverSessionId: "receiver-session-1",
    readiness: "playing",
  });
  await client.playbackReceiverHandoffStatus(receiver.id, {
    authorizationId: handoff.authorizationId,
    receiverPublicKeyFingerprint: handoff.receiverPublicKeyFingerprint,
    requestId: handoff.playback.replacement.requestId,
    sourceSessionId: handoff.playback.replacement.sourceSessionId,
  });

  assert.equal(JSON.parse(calls[0].init.body).clientInstanceId, "stable-installation");
  assert.equal(JSON.parse(calls[1].init.body).clientInstanceId, "stable-installation");
  assert.deepEqual(JSON.parse(calls[2].init.body), handoff);
  assert.match(calls[3].input, /\/handoffs\/handoff-request-1\/commit$/);
  assert.equal(JSON.parse(calls[3].init.body).receiverSessionId, "receiver-session-1");
  assert.match(calls[4].input, /\/handoffs\/handoff-request-1\?/);
  assert.match(calls[4].input, /authorizationId=authorization-1/);
  assert.match(calls[4].input, /receiverPublicKeyFingerprint=receiver-fingerprint/);
  assert.match(calls[4].input, /sourceSessionId=source-session-1/);
});

test("Cast bootstrap binds the configured installation rather than a UI-supplied session id", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    playbackClientInstanceId: () => "stable-installation",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        const request = JSON.parse(init.body);
        return jsonResponse({
          version: "v1", bootstrapEnvelope: "sealed", bootstrapId: "cast-1",
          receiverId: request.receiverId, receiverOrigin: request.receiverOrigin,
          serverOrigin: "https://server.example", generation: 1,
          expiresAt: "2026-08-30T19:00:00Z", capabilities: request.capabilities,
          requestId: request.requestId, replacementSessionId: "cast-playback-1",
          transferStatus: "pending",
        });
      }
    }
  });

  await client.createCastBootstrap({
    clientInstanceId: "playback-session-id",
    clientProfile: { platform: "google-cast" },
    sourceKind: "media",
    sourceId: "m1",
    receiverId: "receiver-1",
    receiverOrigin: "https://cast.getportico.tv",
    receiverPublicKey: "public-key-123456",
    receiverChallenge: "challenge-1234567",
    capabilities: ["load", "control"]
  });

  const request = JSON.parse(calls[0].init.body);
  assert.equal(request.clientInstanceId, "stable-installation");
  assert.equal(typeof request.requestId, "string");
  assert.equal("sourcePlaybackSessionId" in request, false);
});

test("generated Client Core types consume the canonical replacement, Cast, and Live TV terminal schema", () => {
  const schema = JSON.parse(readFileSync(
    new URL("../../../api/openapi/portico-server.openapi.json", import.meta.url),
    "utf8",
  ));
  const components = schema.components.schemas;
  assert.deepEqual(components.PlaybackReplacementRequest.required, [
    "sourceSessionId", "requestId", "previousTerminal",
  ]);
  assert.equal(components.CastBootstrapRequest.properties.replacement.$ref,
    "#/components/schemas/PlaybackReplacementRequest");
  assert.ok(components.CastBootstrapRequest.required.includes("requestId"));
  assert.equal(components.CastBootstrapResponse.properties.transferStatus.const, "pending");
  assert.deepEqual(components.CastStopRequest.required, [
    "generation", "requestId", "terminal",
  ]);
  assert.deepEqual(components.CastAdvanceRequest.required, [
    "generation", "advanceId", "requestId", "previousTerminal",
  ]);
  assert.deepEqual(
    schema.paths["/live-tv/streams/{channelId}/close"].post.requestBody
      .content["application/json"].schema.required,
    ["sessionId", "requestId", "terminal"],
  );
  for (const [path, method] of [
    ["/playback-sessions", "post"],
    ["/live-tv/play", "post"],
    ["/live-tv/streams/{channelId}/open", "post"],
    ["/dvr/recordings/{id}/playback", "post"],
    ["/library-channels/{channelId}/tune", "post"],
  ]) {
    const requestSchema = schema.paths[path][method].requestBody
      .content["application/json"].schema;
    const resolved = requestSchema.$ref
      ? components[requestSchema.$ref.split("/").at(-1)]
      : requestSchema;
    assert.equal(resolved.properties.replacement.$ref,
      "#/components/schemas/PlaybackReplacementRequest", path);
  }
});

test("a lost fresh Cast bootstrap is durably reconciled and exact-replayed", async () => {
  const records = new Map();
  const durability = {
    load: async () => [...records.values()].map(value => structuredClone(value)),
    save: async record => { records.set(record.key, structuredClone(record)); },
    remove: async key => { records.delete(key); },
  };
  const requestBodies = [];
  const bootstrapInput = {
    sourceKind: "media", sourceId: "m1",
    clientProfile: { platform: "google-cast" },
    receiverId: "receiver-1", receiverOrigin: "https://cast.getportico.tv",
    receiverPublicKey: "public-key-123456", receiverChallenge: "challenge-1234567",
    capabilities: ["load", "progress"],
  };
  const first = createPorticoClient({
    apiBaseUrl: "https://server.example",
    playbackClientInstanceId: () => "stable-installation",
    requestId: () => "cast-fresh-request-1",
    playbackProgressDurabilityAdapter: durability,
    transport: { fetch: async (_input, init) => {
      requestBodies.push(String(init.body));
      throw new TypeError("bootstrap response was lost");
    } },
  });
  await assert.rejects(first.createCastBootstrap(bootstrapInput), /response was lost/);
  assert.equal(records.size, 1);
  assert.deepEqual(await first.pendingCastTransfers(), [{requestId: "cast-fresh-request-1"}]);

  const restarted = createPorticoClient({
    apiBaseUrl: "https://server.example",
    playbackClientInstanceId: () => "stable-installation",
    playbackProgressDurabilityAdapter: durability,
    transport: { fetch: async (input, init) => {
      const path = new URL(input).pathname;
      if (path.endsWith("/transfer-status")) {
        return jsonResponse({
          version: "v1", requestId: "cast-fresh-request-1",
          replacementSessionId: "cast-playback-1", status: "pending",
          requestFingerprint: "fingerprint-1",
        });
      }
      requestBodies.push(String(init.body));
      const request = JSON.parse(init.body);
      return jsonResponse({
        version: "v1", bootstrapEnvelope: "sealed", bootstrapId: "cast-1",
        receiverId: request.receiverId, receiverOrigin: request.receiverOrigin,
        serverOrigin: "https://server.example", generation: 1,
        expiresAt: "2026-08-30T21:00:00Z", capabilities: request.capabilities,
        requestId: request.requestId, replacementSessionId: "cast-playback-1",
        transferStatus: "pending",
      });
    } },
  });
  const outcome = await restarted.retryPendingCastTransfer("cast-fresh-request-1");
  assert.equal(outcome.outcome, "pending");
  assert.equal(outcome.value.bootstrapEnvelope, "sealed");
  assert.equal(requestBodies.length, 2);
  assert.equal(requestBodies[0], requestBodies[1]);
  assert.equal(records.size, 1);
});

test("Cast replacement retains source authority until exact committed status", async () => {
  const records = new Map();
  const durability = {
    load: async () => [...records.values()].map(value => structuredClone(value)),
    save: async record => { records.set(record.key, structuredClone(record)); },
    remove: async key => { records.delete(key); },
  };
  const previousTerminal = {
    disposition: "stopped", generation: 3, eventSequence: 9,
    recordedAt: "2026-08-30T20:00:00.000Z",
    positionSeconds: 42, durationSeconds: 60,
  };
  const first = createPorticoClient({
    apiBaseUrl: "https://server.example",
    playbackClientInstanceId: () => "stable-installation",
    playbackProgressDurabilityAdapter: durability,
    transport: { fetch: async () => { throw new TypeError("response was lost"); } },
  });
  await assert.rejects(first.replacePlaybackWithCast({
    sourceKind: "media", sourceId: "m1",
    clientProfile: { platform: "google-cast" },
    receiverId: "receiver-1", receiverOrigin: "https://cast.getportico.tv",
    receiverPublicKey: "public-key-123456", receiverChallenge: "challenge-1234567",
    capabilities: ["load", "progress"],
  }, {
    sourceSessionId: "source-playback-1",
    requestId: "cast-replace-request-1",
    previousTerminal,
  }), /response was lost/);
  assert.deepEqual(await first.pendingCastTransfers(), [{
    requestId: "cast-replace-request-1", sourceSessionId: "source-playback-1",
  }]);
  await assert.rejects(first.touchPlayback("source-playback-1", {
    state: "playing", positionSeconds: 43,
  }), error => error instanceof ApiError && error.code === "playback_stopping");

  const restarted = createPorticoClient({
    apiBaseUrl: "https://server.example",
    playbackClientInstanceId: () => "stable-installation",
    playbackProgressDurabilityAdapter: durability,
    transport: { fetch: async (input, init) => {
      assert.equal(new URL(input).pathname, "/api/playback/cast/transfer-status");
      assert.deepEqual(JSON.parse(init.body), {
        clientInstanceId: "stable-installation",
        requestId: "cast-replace-request-1",
        sourceSessionId: "source-playback-1",
      });
      return jsonResponse({
        version: "v1", requestId: "cast-replace-request-1",
        sourceSessionId: "source-playback-1",
        replacementSessionId: "cast-playback-1", status: "committed",
        requestFingerprint: "fingerprint-1", previousTerminal,
      });
    } },
  });
  const outcome = await restarted.castTransferStatus("cast-replace-request-1");
  assert.equal(outcome.outcome, "accepted");
  assert.equal(outcome.value.replacementSessionId, "cast-playback-1");
  assert.equal(records.size, 0);
  assert.deepEqual(await restarted.pendingCastTransfers(), []);
});

for (const reconcile of [false, true]) test(
  `Cast replacement maps replacement_source_inactive during ${reconcile ? "reconciliation" : "bootstrap"}`,
  async () => {
    const records = new Map();
    const durability = {
      load: async () => [...records.values()].map(value => structuredClone(value)),
      save: async record => { records.set(record.key, structuredClone(record)); },
      remove: async key => { records.delete(key); },
    };
    const input = {
      sourceKind: "media", sourceId: "m1",
      clientProfile: { platform: "google-cast" },
      receiverId: "receiver-1", receiverOrigin: "https://cast.getportico.tv",
      receiverPublicKey: "public-key-123456", receiverChallenge: "challenge-1234567",
      capabilities: ["load", "progress"],
    };
    const replacement = {
      sourceSessionId: `cast-inactive-source-${reconcile}`,
      requestId: `cast-inactive-request-${reconcile}`,
      previousTerminal: {
        disposition: "stopped", generation: 3, eventSequence: 9,
        recordedAt: "2026-08-30T20:00:00.000Z",
        positionSeconds: 42, durationSeconds: 60,
      },
    };
    const inactiveResponse = () => jsonResponse({
      code: "replacement_source_inactive",
      detail: "The Cast replacement source is already inactive.",
    }, { status: 409 });
    const first = createPorticoClient({
      apiBaseUrl: "https://server.example",
      playbackClientInstanceId: () => "stable-installation",
      playbackProgressDurabilityAdapter: durability,
      transport: { fetch: async () => {
        if (reconcile) throw new TypeError("bootstrap response was lost");
        return inactiveResponse();
      } },
    });
    if (reconcile) {
      await assert.rejects(
        first.replacePlaybackWithCast(input, replacement),
        /bootstrap response was lost/,
      );
    }
    const client = reconcile
      ? createPorticoClient({
          apiBaseUrl: "https://server.example",
          playbackClientInstanceId: () => "stable-installation",
          playbackProgressDurabilityAdapter: durability,
          transport: { fetch: async () => inactiveResponse() },
        })
      : first;
    const outcome = reconcile
      ? await client.castTransferStatus(replacement.requestId)
      : await client.replacePlaybackWithCast(input, replacement);

    assert.deepEqual(outcome, {
      outcome: "source-inactive",
      sourceSessionId: replacement.sourceSessionId,
      rejection: {
        status: 409,
        code: "replacement_source_inactive",
        detail: "The Cast replacement source is already inactive.",
      },
    });
    assert.equal(records.size, 0);
    assert.deepEqual(await client.pendingCastTransfers(), []);
  },
);

test("Cast receiver routes use only receiver authority and exact terminal shapes", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://viewer.example",
    transport: { fetch: async (input, init) => {
      calls.push({input: String(input), init});
      const path = new URL(input).pathname;
      if (path.endsWith("/stop")) {
        const body = JSON.parse(init.body);
        return jsonResponse({
          ok: true, generation: body.generation,
          terminal: playbackTerminalReceipt("playback-1", {
            requestId: body.requestId, terminal: body.terminal,
          }),
        });
      }
      return jsonResponse({
        version: "v1", status: "prepared", requestId: "advance-request-1",
        requestFingerprint: "advance-fingerprint-1", previousTerminal: {
          disposition: "completed", generation: 1, eventSequence: 5,
          recordedAt: "2026-08-30T20:00:00.000Z",
          positionSeconds: 60, durationSeconds: 60,
        }, sourceSessionId: "playback-1", replacementSessionId: "playback-2",
        generation: 2, automaticAdvances: 1,
        automation: {autoplayNext: true, creditsSkip: "off", introSkip: "off", passoutAfterEpisodes: 3, passoutProtection: true, upNextCountdownSeconds: 10},
      });
    } },
  });
  const credential = {token: "receiver-secret", origin: "https://cast-server.example"};
  const terminal = {
    disposition: "completed", generation: 1, eventSequence: 5,
    recordedAt: "2026-08-30T20:00:00.000Z",
    positionSeconds: 60, durationSeconds: 60,
  };
  await client.stopCastReceiver("receiver-session-1", credential, {
    generation: 1, requestId: "stop-request-1", terminal,
  });
  await client.advanceCastReceiver("receiver-session-1", credential, {
    generation: 1, advanceId: "advance-1", requestId: "advance-request-1",
    previousTerminal: terminal,
  });

  assert.deepEqual(calls.map(call => new URL(call.input).pathname), [
    "/api/playback/cast/sessions/receiver-session-1/stop",
    "/api/playback/cast/sessions/receiver-session-1/advance",
  ]);
  for (const call of calls) {
    assert.equal(call.init.headers.Authorization, "PorticoReceiver receiver-secret");
    assert.equal(call.init.headers["X-Portico-CSRF"], undefined);
    assert.equal(call.init.credentials, "omit");
  }
  assert.deepEqual(JSON.parse(calls[1].init.body).previousTerminal, terminal);
});

test("Live TV close exact-replays one durable terminal after restart", async () => {
  const records = new Map();
  const durability = {
    load: async () => [...records.values()].map(value => structuredClone(value)),
    save: async record => { records.set(record.key, structuredClone(record)); },
    remove: async key => { records.delete(key); },
  };
  const bodies = [];
  const first = createPorticoClient({
    apiBaseUrl: "https://server.example",
    playbackProgressDurabilityAdapter: durability,
    requestId: () => "live-close-request-1",
    transport: { fetch: async (_input, init) => {
      bodies.push(String(init.body));
      throw new TypeError("close response was lost");
    } },
  });
  first.acceptPlaybackSession(acceptedPlayback("live-session-1", 7));
  await assert.rejects(first.closeLiveTvStream("channel-1", "live-session-1", {
    disposition: "stopped", positionSeconds: 30, durationSeconds: 60,
  }), /response was lost/);

  const restarted = createPorticoClient({
    apiBaseUrl: "https://server.example",
    playbackProgressDurabilityAdapter: durability,
    transport: { fetch: async (_input, init) => {
      bodies.push(String(init.body));
      const request = JSON.parse(init.body);
      return jsonResponse(playbackTerminalReceipt(request.sessionId, request));
    } },
  });
  const outcome = await restarted.retryPendingPlaybackTerminalMutation("live-session-1");
  assert.equal(outcome.outcome, "accepted");
  assert.equal(bodies[0], bodies[1]);
  assert.equal(records.size, 0);
});

test("playback queue methods preserve the authoritative revision and repeat contract", async () => {
  const calls = [];
  const queueState = {
    sessionId: "play 1",
    current: { entryId: "entry-m1", media: { id: "m1", title: "Current", type: "movie", state: {} } },
    items: [],
    history: [],
    total: 0,
    canMutate: true,
    repeatMode: "one",
    revision: 8
  };
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        return jsonResponse(queueState);
      }
    }
  });

  await client.playbackSessionQueue("play 1");
  await client.updatePlaybackSessionQueue("play 1", {
    expectedRevision: 8,
    idempotencyKey: "replace-key",
    mediaIds: ["m2", "m3"],
    repeatMode: "all"
  });
  await client.mutatePlaybackSessionQueue("play 1", {
    expectedRevision: 9,
    idempotencyKey: "repeat-key",
    action: "set_repeat",
    repeatMode: "off"
  });

  assert.deepEqual(calls.map((call) => call.input), [
    "https://server.example/api/playback-sessions/play%201/queue",
    "https://server.example/api/playback-sessions/play%201/queue",
    "https://server.example/api/playback-sessions/play%201/queue"
  ]);
  assert.equal(calls[0].init.method, "GET");
  assert.equal(calls[1].init.method, "PUT");
  assert.deepEqual(JSON.parse(calls[1].init.body), {
    expectedRevision: 8,
    idempotencyKey: "replace-key",
    mediaIds: ["m2", "m3"],
    repeatMode: "all"
  });
  assert.equal(calls[2].init.method, "PATCH");
  assert.deepEqual(JSON.parse(calls[2].init.body), {
    expectedRevision: 9,
    idempotencyKey: "repeat-key",
    action: "set_repeat",
    repeatMode: "off"
  });
});

test("playbackSourceFor consumes the exact server-issued HLS resource", () => {
  const source = playbackSourceFor({
    sessionId: "s1",
    media: { id: "m1", title: "Movie", type: "movie", state: {} },
    sourceUrl: "/opaque/playback/rendition-1?signature=server-owned",
    streamFormat: "hls",
    directPlay: false,
    decision: { mode: "direct_stream", reason: "", requiresTranscode: true, isProxied: true, isServerCached: true },
    qualityOffers: acceptedPlayback("s1").qualityOffers,
    qualitySelection: {mode: "automatic"},
    audioStreams: [],
    subtitleStreams: [],
    chapters: [],
    queue: [],
    resources: [{
      id: "original-main-off",
      sourceUrl: "/opaque/playback/rendition-1?signature=server-owned",
      streamFormat: "hls",
      audioStreamId: "",
      subtitleMode: "off",
      default: true
    }]
}, (path) => `https://server.example${path}`);
  assert.equal(source, "https://server.example/opaque/playback/rendition-1?signature=server-owned");
});

test("playbackSourceFor does not rewrite server-owned query semantics", () => {
  const source = playbackSourceFor({
    sessionId: "s1",
    media: { id: "m1", title: "Movie", type: "movie", state: {} },
    sourceUrl: "/opaque/rendition?start=server-value&startSeconds=server-value",
    streamFormat: "hls",
    directPlay: false,
    decision: { mode: "transcode_required", reason: "", requiresTranscode: true, isProxied: true, isServerCached: true },
    qualityOffers: acceptedPlayback("s1").qualityOffers,
    qualitySelection: {mode: "automatic"},
    audioStreams: [],
    subtitleStreams: [],
    chapters: [],
    queue: [],
    resources: [{
      id: "original",
      sourceUrl: "/opaque/rendition?start=server-value&startSeconds=server-value",
      streamFormat: "hls",
      subtitleMode: "off",
      default: true
    }]
  }, (path) => `https://server.example${path}`);
  assert.equal(source, "https://server.example/opaque/rendition?start=server-value&startSeconds=server-value");
});

test("playbackSourceFor selects a server-issued HLS audio resource", () => {
  const source = playbackSourceFor({
    sessionId: "s1",
    media: { id: "m1", title: "Movie", type: "movie", state: {} },
    sourceUrl: "/opaque/audio-commentary?rendition=commentary",
    streamFormat: "hls",
    directPlay: false,
    decision: { mode: "direct_stream", reason: "", requiresTranscode: true, isProxied: true, isServerCached: true },
    qualityOffers: acceptedPlayback("s1").qualityOffers,
    qualitySelection: {mode: "automatic"},
    audioStreams: [],
    subtitleStreams: [],
    chapters: [],
    queue: [],
    resources: [
      { id: "commentary", sourceUrl: "/opaque/audio-commentary?rendition=commentary", streamFormat: "hls", audioStreamId: "audio_commentary", subtitleMode: "off", default: true }
    ]
  }, (path) => `https://server.example${path}`);
  assert.equal(source, "https://server.example/opaque/audio-commentary?rendition=commentary");
});

test("playbackSourceFor selects the exact server-issued subtitle resource", () => {
  const source = playbackSourceFor({
    sessionId: "s1",
    media: { id: "m1", title: "Movie", type: "movie", state: {} },
    sourceUrl: "/opaque/subtitle-text",
    streamFormat: "hls",
    directPlay: false,
    decision: { mode: "direct_stream", reason: "", requiresTranscode: false, isProxied: true, isServerCached: true },
    qualityOffers: acceptedPlayback("s1").qualityOffers,
    qualitySelection: {mode: "automatic"},
    audioStreams: [],
    subtitleStreams: [],
    chapters: [],
    queue: [],
    resources: [
      { id: "text", sourceUrl: "/opaque/subtitle-text", streamFormat: "hls", subtitleStreamId: "sidecar", subtitleMode: "text", default: true }
    ]
  }, (path) => `https://server.example${path}`);
  assert.equal(source, "https://server.example/opaque/subtitle-text");
});

test("playbackSourceFor fails closed instead of manufacturing a missing variant", () => {
  let resolved = false;
  assert.throws(() => playbackSourceFor({
    sessionId: "s1",
    media: { id: "m1", title: "Movie", type: "movie", state: {} },
    sourceUrl: "/api/media/m1/stream",
    streamFormat: "direct",
    directPlay: true,
    decision: { mode: "direct_play", reason: "", requiresTranscode: false, isProxied: true, isServerCached: false },
    qualityOffers: acceptedPlayback("s1").qualityOffers,
    qualitySelection: {mode: "automatic"},
    audioStreams: [], subtitleStreams: [], chapters: [], queue: [], resources: []
  }, () => { resolved = true; return "https://server.example/should-not-resolve"; }), (error) => error.code === "playback_resource_unavailable" && error.messageId === "playback.unavailable");
  assert.equal(resolved, false);
});

test("hosted services client uses hosted CSRF on mutations", async () => {
  const calls = [];
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    csrfToken: "hosted-csrf",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        if (String(input).endsWith("/api/system")) {
          return jsonResponse(hostedSystemFixture);
        }
        return jsonResponse({ authenticated: true, user: {} });
      }
    }
  });

  await hosted.login({ email: "a@example.test", password: "secret" });
  assert.equal(calls[0].input, "https://api.example/api/system");
  assert.equal(calls[1].input, "https://api.example/api/auth/login");
  assert.equal(calls[1].init.headers["X-Portico-CSRF"], "hosted-csrf");
  assert.match(calls[1].init.headers["Idempotency-Key"], /\S{8,}/);
});

test("exported Hosted routes path consumes the exact Go-serialized route shape and rejects a separate server token", async () => {
  const calls = [];
  let routeBody = goSerializedRouteFixture;
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    transport: {fetch: async (input) => {
      const url = String(input);
      calls.push(url);
      if (url.endsWith("/api/system")) return jsonResponse(hostedSystemFixture);
      if (url.endsWith("/api/account/servers/srv_fixture/routes")) return jsonResponse(routeBody);
      throw new Error(`Unexpected request ${url}`);
    }}
  });

  assert.deepEqual(await hosted.routes("srv_fixture"), goSerializedRouteFixture);
  assert.deepEqual(calls, [
    "https://api.example/api/system",
    "https://api.example/api/account/servers/srv_fixture/routes"
  ]);

  routeBody = {
    ...goSerializedRouteFixture,
    routes: [{...goSerializedRouteFixture.routes[0], serverToken: "must-not-be-exposed"}]
  };
  await assert.rejects(hosted.routes("srv_fixture"), error =>
    error instanceof ApiError && error.code === "invalid_response" && !error.message.includes("must-not-be-exposed")
  );
});

test("hosted account and claim mutations forward cancellation signals", async () => {
  const calls = [];
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        const url = String(input);
        if (url.endsWith("/api/system")) return jsonResponse(hostedSystemFixture);
        if (url.endsWith("/api/auth/login")) return jsonResponse({ authenticated: true, user: {} });
        return jsonResponse({ authenticated: true, user: {}, server: {}, serverCredential: "credential" });
      }
    }
  });
  const controller = new AbortController();

  await hosted.login({ login: "owner", password: "secret", installationId: "install-1" }, { signal: controller.signal });
  await hosted.completeClaim({ claimCode: "claim-1" }, { signal: controller.signal });

  assert.equal(calls[1].init.signal.aborted, false);
  assert.equal(calls[3].init.signal.aborted, false);
});

test("hosted services client captures API-origin CSRF bootstrap headers for later mutations", async () => {
  const calls = [];
  const observed = [];
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    onCSRFToken: (token) => observed.push(token),
    transport: {
      fetch: async (input, init) => {
        const url = String(input);
        calls.push({ input: url, init });
        if (url.endsWith("/api/system")) return jsonResponse(hostedSystemFixture);
        if (url.endsWith("/api/auth/me")) {
          return jsonResponse({ authenticated: true, user: {} }, { headers: { "Content-Type": "application/json", "X-Portico-CSRF": "api-owned-csrf" } });
        }
        return jsonResponse({ user: { id: "user-1" } });
      }
    }
  });

  await hosted.me();
  await hosted.updateAccount({ username: "viewer" });

  assert.deepEqual(observed, ["api-owned-csrf"]);
  assert.equal(calls[2].init.headers["X-Portico-CSRF"], "api-owned-csrf");
});

test("hosted services client renews stale browser CSRF and retries a rejected mutation once", async () => {
  const calls = [];
  let csrf = "stale-csrf";
  let loginAttempts = 0;
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    csrfToken: () => csrf,
    transport: {
      fetch: async (input, init) => {
        const url = String(input);
        calls.push({ input: url, init });
        if (url.endsWith("/api/system")) return jsonResponse(hostedSystemFixture);
        if (url.endsWith("/api/auth/me")) {
          csrf = "fresh-csrf";
          return jsonResponse({ authenticated: true, user: {} });
        }
        if (url.endsWith("/api/auth/login")) {
          loginAttempts++;
          if (loginAttempts === 1) {
            return jsonResponse({ code: "csrf_failed", detail: "stale request verification" }, { status: 403 });
          }
          return jsonResponse({ authenticated: true, user: {} });
        }
        throw new Error(`Unexpected request ${url}`);
      }
    }
  });

  await hosted.login({ email: "a@example.test", password: "secret" });

  assert.deepEqual(calls.map((call) => call.input), [
    "https://api.example/api/system",
    "https://api.example/api/auth/login",
    "https://api.example/api/auth/me",
    "https://api.example/api/auth/login",
    "https://api.example/api/system"
  ]);
  assert.equal(calls[1].init.headers["X-Portico-CSRF"], "stale-csrf");
  assert.equal(calls[3].init.headers["X-Portico-CSRF"], "fresh-csrf");
  assert.equal(calls[3].init.headers["Idempotency-Key"], calls[1].init.headers["Idempotency-Key"]);
  assert.equal(loginAttempts, 2);
});

test("hosted services client recovers a missing browser CSRF token without retrying more than once", async () => {
  const calls = [];
  let csrf = "";
  let loginAttempts = 0;
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    csrfToken: () => csrf,
    transport: {
      fetch: async (input, init) => {
        const url = String(input);
        calls.push({ input: url, init });
        if (url.endsWith("/api/system")) return jsonResponse(hostedSystemFixture);
        if (url.endsWith("/api/auth/me")) {
          csrf = "renewed-csrf";
          return jsonResponse({ authenticated: true });
        }
        if (url.endsWith("/api/auth/login")) {
          loginAttempts++;
          return jsonResponse({ code: "csrf_failed", detail: "request verification failed" }, { status: 403 });
        }
        throw new Error(`Unexpected request ${url}`);
      }
    }
  });

  await assert.rejects(
    hosted.login({ email: "a@example.test", password: "secret" }),
    (error) => error?.code === "csrf_failed"
  );

  assert.deepEqual(calls.map((call) => call.input), [
    "https://api.example/api/system",
    "https://api.example/api/auth/login",
    "https://api.example/api/auth/me",
    "https://api.example/api/auth/login"
  ]);
  assert.equal(calls[1].init.headers["X-Portico-CSRF"], undefined);
  assert.equal(calls[3].init.headers["X-Portico-CSRF"], "renewed-csrf");
  assert.equal(loginAttempts, 2);
});

test("hosted services client confirms account deletion with the current password", async () => {
  const calls = [];
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    csrfToken: "hosted-csrf",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        if (String(input).endsWith("/api/system")) return jsonResponse(hostedSystemFixture);
        return jsonResponse({ ok: true, deletedAt: "2026-07-16T20:00:00Z" });
      }
    }
  });

  const signal = new AbortController().signal;
  await hosted.deleteAccount({ password: "current-password" }, { signal });

  assert.equal(calls[1].input, "https://api.example/api/account/me");
  assert.equal(calls[1].init.method, "DELETE");
  assert.equal(calls[1].init.body, JSON.stringify({ password: "current-password" }));
  assert.equal(calls[1].init.signal.aborted, false);
  assert.equal(calls[1].init.headers["X-Portico-CSRF"], "hosted-csrf");
  assert.match(calls[1].init.headers["Idempotency-Key"], /\S{8,}/);
});

test("hosted services client uses the current proof-bound server lifecycle endpoints", async () => {
  const calls = [];
  let requestSequence = 0;
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    csrfToken: "hosted-csrf",
    requestId: () => `server-lifecycle-${++requestSequence}`,
    transport: {
      fetch: async (input, init) => {
        const url = String(input);
        calls.push({ input: url, init });
        if (url.endsWith("/api/system")) return jsonResponse(hostedSystemFixture);
        if (url.endsWith("/deletion-proofs"))
          return jsonResponse({ proof: "ptc_sdp_proof-1", expiresAt: "2026-08-30T13:05:00Z" }, { status: 201 });
        if (url.endsWith("/restore-authorizations"))
          return jsonResponse({
            kind: "restore-authorization", version: 1, audience: "portico-media-server", authorizationId: "sra_issued",
            purpose: "server-restore", serverId: "server/owned", accountId: "account-1", restoreSecurityEpoch: 7,
            issuedAt: "2026-08-30T13:00:00Z", expiresAt: "2026-08-30T13:05:00Z",
            signatureAlgorithm: "ed25519", signatureKeyId: "key-1", signature: "signed"
          }, { status: 201 });
        return jsonResponse({ ok: true });
      },
    },
  });
  const signal = new AbortController().signal;

  const issued = await hosted.createServerDeletionProof(
    "server/owned",
    { password: "current-password", mfaCode: "123456" },
    { signal },
  );
  await hosted.deleteServer(
    "server/owned",
    { confirmation: "Family Media", proof: issued.proof },
    { signal },
  );
  const restoreAuthorization = await hosted.createServerRestoreAuthorization(
    "server/owned",
    { restoreSecurityEpoch: 7, recoveryCode: "restore-recovery" },
    { signal },
  );
  await hosted.leaveServer("server/shared", { signal });

  assert.equal(calls[1].input, "https://api.example/api/account/servers/server%2Fowned/deletion-proofs");
  assert.equal(calls[1].init.method, "POST");
  assert.equal(calls[1].init.body, JSON.stringify({ password: "current-password", mfaCode: "123456" }));
  assert.equal(calls[2].input, "https://api.example/api/account/servers/server%2Fowned");
  assert.equal(calls[2].init.method, "DELETE");
  assert.equal(calls[2].init.body, JSON.stringify({ confirmation: "Family Media", proof: "ptc_sdp_proof-1" }));
  assert.equal(calls[3].input, "https://api.example/api/account/servers/server%2Fowned/restore-authorizations");
  assert.equal(calls[3].init.method, "POST");
  assert.equal(calls[3].init.body, JSON.stringify({ restoreSecurityEpoch: 7, recoveryCode: "restore-recovery" }));
  assert.equal(restoreAuthorization.authorizationId, "sra_issued");
  assert.equal(calls[4].input, "https://api.example/api/account/servers/server%2Fshared/membership");
  assert.equal(calls[4].init.method, "DELETE");
  assert.equal(calls[4].init.body, undefined);
  assert.match(calls[2].init.headers["Idempotency-Key"], /^[A-Za-z0-9._:-]{8,128}$/);
  assert.match(calls[4].init.headers["Idempotency-Key"], /^[A-Za-z0-9._:-]{8,128}$/);
});

test("hosted self-leave reconciles a committed response loss without replaying membership deletion", async () => {
  const calls = [];
  const saved = [];
  const removed = [];
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    requestId: () => "membership-leave-key",
    terminalMutationDurabilityAdapter: {
      save: async record => saved.push(record),
      remove: async key => removed.push(key),
    },
    transport: {
      fetch: async (input, init) => {
        const url = String(input);
        calls.push({ url, init });
        if (url.endsWith("/api/system")) return jsonResponse(hostedSystemFixture);
        if (url.endsWith("/membership"))
          return jsonResponse({ code: "terminal_mutation_outcome_unknown" }, {
            status: 503,
            headers: { "X-Portico-Terminal-Outcome": "outcome_unknown" },
          });
        if (url.endsWith("/api/account/terminal-mutations"))
          return jsonResponse({ outcome: "committed", receipt: { receiptId: "tmr_leave" } });
        throw new Error(`Unexpected request ${url}`);
      },
    },
  });

  await assert.rejects(
    hosted.leaveServer("server-shared"),
    error => error instanceof HostedTerminalMutationCommittedError && error.committed === true,
  );
  assert.equal(saved[0].path, "/api/account/servers/server-shared/membership");
  assert.deepEqual(removed, ["membership-leave-key"]);
  assert.equal(calls.filter(call => call.url.endsWith("/membership")).length, 1);
  assert.equal(calls.find(call => call.url.endsWith("/api/account/terminal-mutations")).init.headers["Idempotency-Key"], "membership-leave-key");
});

test("hosted services client reconciles a lost terminal response by the original idempotency key", async () => {
  const calls = [];
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    transport: {
      fetch: async (input, init) => {
        calls.push({ input: String(input), init });
        if (String(input).endsWith("/api/system")) return jsonResponse(hostedSystemFixture);
        return jsonResponse({
          outcome: "committed",
          receipt: {
            receiptId: "tmr_1",
            auditEventId: "aud_1",
            action: "account.deleted",
            targetType: "user",
            targetId: "usr_1",
            actorType: "user",
            actorId: "usr_1",
            createdAt: "2026-08-25T12:00:00Z"
          }
        });
      }
    }
  });

  const result = await hosted.reconcileAccountTerminalMutation("lost-response-key-1");
  assert.equal(result.outcome, "committed");
  assert.equal(calls[1].input, "https://api.example/api/account/terminal-mutations");
  assert.equal(calls[1].init.method, "GET");
  assert.equal(calls[1].init.headers["Idempotency-Key"], "lost-response-key-1");
  assert.throws(
    () => hosted.reconcileAccountTerminalMutation("bad key"),
    /valid terminal mutation idempotency key/,
  );
});

test("hosted terminal mutations persist before dispatch and reconcile an unknown outcome without replay", async () => {
  const calls = [];
  const saved = [];
  const removed = [];
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    requestId: () => "terminal-key-1",
    terminalMutationDurabilityAdapter: {
      save: async record => saved.push(record),
      remove: async key => removed.push(key)
    },
    transport: {
      fetch: async (input, init) => {
        const url = String(input);
        calls.push({ url, init });
        if (url.endsWith("/api/system")) return jsonResponse(hostedSystemFixture);
        if (url.endsWith("/api/account/me")) {
          return jsonResponse({
            code: "terminal_mutation_outcome_unknown",
            detail: "Portico is still confirming this change."
          }, {
            status: 503,
            headers: {
              "Retry-After": "1",
              "X-Portico-Terminal-Outcome": "outcome_unknown",
              "X-Portico-Terminal-Receipt": "tmr_1"
            }
          });
        }
        if (url.endsWith("/api/account/terminal-mutations")) {
          return jsonResponse({ outcome: "committed", receipt: { receiptId: "tmr_1" } });
        }
        throw new Error(`Unexpected request ${url}`);
      }
    }
  });

  await assert.rejects(
    hosted.deleteAccount({ password: "secret" }),
    error =>
      error instanceof HostedTerminalMutationCommittedError &&
      error.committed === true &&
      error.idempotencyKey === "terminal-key-1",
  );
  assert.equal(saved.length, 1);
  assert.equal(saved[0].path, "/api/account/me");
  assert.deepEqual(removed, ["terminal-key-1"]);
  assert.equal(calls.filter(call => call.url.endsWith("/api/account/me")).length, 1);
  const reconciliation = calls.find(call => call.url.endsWith("/api/account/terminal-mutations"));
  assert.equal(reconciliation.init.headers["Idempotency-Key"], "terminal-key-1");
});

test("hosted terminal mutation uncertainty retains its durable key when reconciliation is unavailable", async () => {
  const saved = [];
  const removed = [];
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    requestId: () => "terminal-key-2",
    terminalMutationDurabilityAdapter: {
      save: async record => saved.push(record),
      remove: async key => removed.push(key)
    },
    transport: {
      fetch: async (input) => {
        const url = String(input);
        if (url.endsWith("/api/system")) return jsonResponse(hostedSystemFixture);
        if (url.endsWith("/api/account/me")) throw new Error("response lost");
        if (url.endsWith("/api/account/terminal-mutations")) {
          return jsonResponse({ code: "receipt_not_found", detail: "Not found." }, { status: 404 });
        }
        throw new Error(`Unexpected request ${url}`);
      }
    },
  });

  await assert.rejects(
    hosted.deleteAccount({ password: "secret" }),
    error =>
      error instanceof HostedTerminalMutationUncertainError &&
      error.ambiguous === true &&
      error.idempotencyKey === "terminal-key-2",
  );
  assert.equal(saved.length, 1);
  assert.deepEqual(removed, []);
});

test("hosted profile-image upload uses the same durable terminal reconciliation path", async () => {
  const calls = [];
  const saved = [];
  const removed = [];
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    requestId: () => "terminal-upload-key",
    terminalMutationDurabilityAdapter: {
      save: async record => saved.push(record),
      remove: async key => removed.push(key),
    },
    transport: {
      fetch: async (input, init) => {
        const url = String(input);
        calls.push({ url, init });
        if (url.endsWith("/api/system")) return jsonResponse(hostedSystemFixture);
        if (url.endsWith("/api/account/me/image")) {
          return jsonResponse({ code: "terminal_mutation_outcome_unknown" }, {
            status: 503,
            headers: { "X-Portico-Terminal-Outcome": "outcome_unknown" },
          });
        }
        if (url.endsWith("/api/account/terminal-mutations"))
          return jsonResponse({ outcome: "committed", receipt: { receiptId: "tmr_upload" } });
        throw new Error(`Unexpected request ${url}`);
      },
    },
  });
  const form = new FormData();
  form.set("image", new Blob(["image"]), "avatar.png");

  await assert.rejects(
    hosted.uploadAccountImage(form),
    error => error instanceof HostedTerminalMutationCommittedError,
  );
  assert.equal(saved[0].path, "/api/account/me/image");
  assert.deepEqual(removed, ["terminal-upload-key"]);
  assert.equal(calls.filter(call => call.url.endsWith("/api/account/me/image")).length, 1);
});

test("hosted services client exposes the complete TV setup flow", async () => {
  const calls = [];
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    accessToken: "account-access",
    transport: {
      fetch: async (input, init) => {
        calls.push({input: String(input), init});
        if (String(input).endsWith("/api/system")) {
          return jsonResponse(hostedSystemFixture);
        }
        if (String(input).endsWith("/api/tv-setup/sessions") && init.method === "POST") {
          return jsonResponse({setupSessionId: "setup 1", code: "ABCD-EFGH", pollSecret: "poll-secret", status: "pending", protocolVersion: 1}, {status: 201});
        }
        return jsonResponse({ok: true, setupSessionId: "setup 1", status: "grant_ready"});
      }
    }
  });

  await hosted.createTVSetupSession({appVersion: "1.0", authModeHint: "portico-account", deviceName: "Living Room", devicePublicKey: "public-key", platform: "tvOS"});
  await hosted.tvSetupSession("setup 1", "poll-secret");
  await hosted.authorizeTVSetupGrant({code: "ABCD-EFGH", devicePublicKey: "public-key", serverId: "server-1", setupSessionId: "setup 1"});
  await hosted.redeemTVSetupSession("setup 1", "poll-secret");

  assert.deepEqual(calls.map(call => call.input), [
    "https://api.example/api/system",
    "https://api.example/api/tv-setup/sessions",
    "https://api.example/api/tv-setup/sessions/setup%201",
    "https://api.example/api/tv-setup/grants",
    "https://api.example/api/tv-setup/sessions/setup%201/redeem"
  ]);
  assert.equal(calls[2].init.headers["X-Portico-TV-Setup-Poll-Secret"], "poll-secret");
  assert.equal(calls[4].init.headers["X-Portico-TV-Setup-Poll-Secret"], "poll-secret");
  assert.equal(calls[3].init.headers.Authorization, "Bearer account-access");
});

test("hosted services client exposes generic device authorization without putting device codes in URLs", async () => {
  const calls = [];
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    accessToken: "account-access",
    transport: {
      fetch: async (input, init) => {
        calls.push({input: String(input), init});
        if (String(input).endsWith("/api/system")) {
          return jsonResponse(hostedSystemFixture);
        }
        return jsonResponse({status: "approved", authorizationSessionId: "authorization 1"}, {status: String(input).endsWith("/sessions") ? 201 : 200});
      }
    }
  });

  await hosted.createDeviceAuthorizationSession({deviceName: "Living Room", platform: "limited-tv", appVersion: "1.0", installationId: "installation-1"});
  await hosted.pollDeviceAuthorizationSession("authorization 1", "device-secret");
  await hosted.previewDeviceAuthorization({userCode: "ABCD-EFGH"});
  await hosted.decideDeviceAuthorization({userCode: "ABCD-EFGH", decision: "approve"});
  await hosted.redeemDeviceAuthorizationSession("authorization 1", "device-secret");

  assert.deepEqual(calls.map(call => call.input), [
    "https://api.example/api/system",
    "https://api.example/api/device-authorization/sessions",
    "https://api.example/api/device-authorization/sessions/authorization%201",
    "https://api.example/api/device-authorizations/preview",
    "https://api.example/api/device-authorizations",
    "https://api.example/api/device-authorization/sessions/authorization%201/redeem"
  ]);
  assert.equal(calls[2].init.headers["X-Portico-Device-Code"], "device-secret");
  assert.equal(calls[5].init.headers["X-Portico-Device-Code"], "device-secret");
  assert.equal(calls[2].input.includes("device-secret"), false);
  assert.equal(calls[5].input.includes("device-secret"), false);
  assert.deepEqual(JSON.parse(calls[3].init.body), {userCode: "ABCD-EFGH"});
  assert.deepEqual(JSON.parse(calls[4].init.body), {userCode: "ABCD-EFGH", decision: "approve"});
  assert.equal(calls[3].init.headers.Authorization, "Bearer account-access");
  assert.equal(calls[4].init.headers.Authorization, "Bearer account-access");
});

test("hosted services client exposes the complete account profile lifecycle without leaking PINs", async () => {
  const calls = [];
  const policy = {
    version: "v1",
    maximumAgeRating: 13,
    allowUnrated: false,
    blockedLabels: [],
    allowDownloads: false,
    allowDvr: false,
    allowFeedback: true,
    allowLiveTV: true,
    allowWatchWithFriends: false
  };
  const profile = {
    id: "kids/profile",
    isAccountAdmin: false,
    isPrimary: false,
    name: "Kids",
    hasPIN: true,
    pinRevision: 2,
    policy,
    sortOrder: 1,
  };
  const primaryProfile = {
    ...profile,
    id: "primary-profile",
    name: "Justin",
    isPrimary: true,
    isAccountAdmin: true,
    hasPIN: false,
    pinRevision: 0,
    sortOrder: 0,
    policy: {...policy, maximumAgeRating: null, allowUnrated: true, allowDownloads: true, allowDvr: true, allowWatchWithFriends: true}
  };
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    accessToken: "account-access",
    csrfToken: "csrf",
    transport: {
      fetch: async (input, init) => {
        calls.push({input: String(input), init});
        if (String(input).endsWith("/api/system")) {
          return jsonResponse(hostedSystemFixture);
        }
        if (String(input).endsWith("/profile-administration/sessions")) {
          return jsonResponse({token: "ptc_pad_proof", expiresAt: "2026-07-16T12:05:00Z"}, {status: 201});
        }
        if (String(input).endsWith("/selection-assertions")) {
          return jsonResponse({
            version: "v1",
            assertionId: "assertion-1",
            audience: "portico-media-server",
            accountId: "account-1",
            profileId: profile.id,
            serverId: "server-1",
            deviceId: "device-1",
            installationId: "installation-1",
			accountRevision: 4,
            pinRevision: 2,
			profiles: [{
			  id: profile.id, accountId: "account-1", name: profile.name, sortOrder: 0,
			  isPrimary: true, isAccountAdmin: true, hasPIN: true, pinRevision: 2, policy,
			  policyUpdatedAt: "2026-07-16T12:00:00Z"
			}],
            issuedAt: "2026-07-16T12:00:00Z",
            expiresAt: "2026-07-16T12:05:00Z",
            signatureAlgorithm: "ed25519",
            signatureKeyId: "key-1",
            signature: "signature"
          }, {status: 201});
        }
        if (String(input).endsWith("/pin") && init.method === "PUT") return jsonResponse({ok: true, pinRevision: 2});
        if (String(input).endsWith("/profiles") || String(input).endsWith("/reorder")) {
		  return jsonResponse(init.method === "POST" && !String(input).endsWith("/reorder") ? {profile, revision: 4} : {accountId: "account-1", profiles: [primaryProfile, profile], total: 2, revision: 4}, {status: init.method === "POST" && !String(input).endsWith("/reorder") ? 201 : 200});
        }
        if (init.method === "GET") return jsonResponse(profile);
        if (init.method === "PUT") return jsonResponse({profile, revision: 5});
        return jsonResponse({ok: true});
      }
    }
  });

  const administration = {token: "ptc_pad_proof"};
  await hosted.profiles();
  await hosted.profile(profile.id);
  await hosted.createProfileAdministrationSession({installationId: "installation-1", pin: "2468"});
  await hosted.createProfile({name: "Kids", restrictions: policy}, administration);
	await hosted.updateProfile(profile.id, {name: "Kids", restrictions: policy, expectedRevision: 4}, administration);
	await hosted.reorderProfiles({profileIds: [profile.id], expectedRevision: 5}, administration);
  await hosted.setProfilePIN(profile.id, {password: "account-secret", pin: "2468"}, administration);
  await hosted.clearProfilePIN(profile.id, {password: "account-secret"}, administration);
  await hosted.createProfileSelectionEnvelope(profile.id, {installationId: "installation-1", serverId: "server-1", pin: "2468"});
  await hosted.deleteProfile(profile.id, administration);

  assert.deepEqual(calls.slice(1).map(call => [call.init.method, call.input]), [
    ["GET", "https://api.example/api/account/profiles"],
    ["GET", "https://api.example/api/account/profiles/kids%2Fprofile"],
    ["POST", "https://api.example/api/account/profile-administration/sessions"],
    ["POST", "https://api.example/api/account/profiles"],
    ["PUT", "https://api.example/api/account/profiles/kids%2Fprofile"],
    ["POST", "https://api.example/api/account/profiles/reorder"],
    ["PUT", "https://api.example/api/account/profiles/kids%2Fprofile/pin"],
    ["DELETE", "https://api.example/api/account/profiles/kids%2Fprofile/pin"],
    ["POST", "https://api.example/api/account/profiles/kids%2Fprofile/selection-assertions"],
    ["DELETE", "https://api.example/api/account/profiles/kids%2Fprofile"]
  ]);
  assert.equal(calls.some(call => call.input.includes("2468") || call.input.includes("account-secret")), false);
  for (const index of [4, 5, 6, 7, 8, 10]) {
    assert.equal(calls[index].init.headers["X-Portico-Profile-Admin"], administration.token);
    assert.equal(calls[index].init.headers["X-Portico-Installation-ID"], undefined);
  }
  assert.deepEqual(JSON.parse(calls[3].init.body), {installationId: "installation-1", pin: "2468"});
	assert.equal(JSON.parse(calls[5].init.body).expectedRevision, 4);
	assert.equal(JSON.parse(calls[6].init.body).expectedRevision, 5);
  assert.deepEqual(JSON.parse(calls[7].init.body), {password: "account-secret", pin: "2468"});
  assert.deepEqual(JSON.parse(calls[9].init.body), {installationId: "installation-1", serverId: "server-1", pin: "2468"});
  await assert.rejects(
    hosted.createProfile({name: "Kids", restrictions: policy}, {token: "", installationId: "installation-1"}),
    /proof is required/
  );
});

test("clients preserve canonical server and Hosted problem details", async () => {
  const server = createPorticoClient({
    transport: { fetch: async () => jsonResponse({ code: "media_missing", detail: "The item is unavailable.", messageId: "problem.not-found", requestId: "req_1", status: 404, title: "Not Found", type: "about:blank" }, { status: 404 }) }
  });
  await assert.rejects(server.media("missing"), (error) => {
    assert.equal(error.code, "media_missing");
    assert.equal(error.message, "The item is unavailable.");
    assert.equal(error.requestId, "req_1");
    assert.equal(error.messageId, "problem.not-found");
    assert.equal(error.details, undefined);
    return true;
  });

  const hosted = createHostedServicesClient({
    transport: { fetch: async (input) => String(input).endsWith("/api/system")
      ? jsonResponse(hostedSystemFixture)
      : jsonResponse({ code: "server_not_found", message: "That server is unavailable.", requestId: "req_2", details: { serverId: "srv_missing" } }, { status: 404 }) }
  });
  await assert.rejects(hosted.server("srv_missing"), (error) => {
    assert.equal(error.code, "server_not_found");
    assert.equal(error.message, "That server is unavailable.");
    assert.deepEqual(error.details, { serverId: "srv_missing" });
    assert.equal(error.requestId, "req_2");
    return true;
  });
});

test("successful HTTP responses with malformed JSON retain structured protocol diagnostics", async () => {
  const client = createPorticoClient({
    transport: {
      fetch: async () => new Response("{not-json", {
        status: 200,
        headers: {"Content-Type": "application/json", "X-Request-ID": "protocol-request-1"}
      })
    }
  });
  await assert.rejects(client.system(), (error) => {
    assert.ok(error instanceof ApiError);
    assert.equal(error.status, 200);
    assert.equal(error.code, "invalid_response");
    assert.equal(error.messageId, "problem.request-failed");
    assert.equal(error.requestId, "protocol-request-1");
    assert.deepEqual(error.details, {method: "GET", path: "/api/system"});
    return true;
  });
});

test("API key creation validates the one-time bearer and its public identity", async () => {
  const token = `ptc_api_${"A".repeat(39)}WXYZ`;
  const response = {
    key: {
      id: "apikey_fixture",
      name: "Home automation",
      userId: "owner_fixture",
      username: "owner",
      lastFour: "WXYZ",
      scopes: ["read", "playMedia"],
      createdAt: "2026-08-31T12:00:00Z"
    },
    token,
    futurePresentationHint: "benign unknown fields remain forward compatible"
  };
  const client = createPorticoClient({
    transport: { fetch: async () => jsonResponse(response, {status: 201}) }
  });

  assert.deepEqual(await client.createAPIKey({name: "Home automation", scopes: ["playMedia"]}), response);
});

test("API key creation rejects malformed or inconsistent one-time credentials", async () => {
  const token = `ptc_api_${"A".repeat(39)}WXYZ`;
  const valid = {
    key: {
      id: "apikey_fixture",
      name: "Home automation",
      userId: "owner_fixture",
      lastFour: "WXYZ",
      scopes: ["read", "playMedia"],
      createdAt: "2026-08-31T12:00:00Z"
    },
    token
  };
  const malformed = [
    {...valid, token: `ptc_loc_${"A".repeat(39)}WXYZ`},
    {...valid, token: `ptc_api_${"A".repeat(38)}WXYZ`},
    {...valid, key: {...valid.key, lastFour: "NOPE"}},
    {...valid, key: {...valid.key, id: ""}},
    {...valid, key: {...valid.key, createdAt: "not-a-date"}},
    {...valid, key: {...valid.key, scopes: ["read", "manageServer"]}},
    {...valid, key: {...valid.key, scopes: ["playMedia"]}},
    {...valid, key: {...valid.key, scopes: ["read", "read"]}},
    {...valid, refreshToken: "must-not-cross-this-boundary"},
    {...valid, key: {...valid.key, secret: "must-not-cross-this-boundary"}}
  ];

  for (const body of malformed) {
    const client = createPorticoClient({
      transport: { fetch: async () => jsonResponse(body, {status: 201}) }
    });
    await assert.rejects(
      client.createAPIKey({name: "Home automation", scopes: ["playMedia"]}),
      error => error instanceof ApiError && error.code === "invalid_response" && !error.message.includes("must-not-cross")
    );
  }
});

test("media detail preserves the server-selected show playback target", async () => {
  const target = {
    id: "episode-9",
    type: "episode",
    title: "The Castle",
    sortTitle: "The Castle",
    genres: [],
    tags: [],
    labels: [],
    addedAt: "2026-07-12T12:00:00Z",
    seasonNumber: 2,
    episodeNumber: 9,
    images: {},
    state: { watched: false, progressSeconds: 330 },
    actions: ["play"]
  };
  const client = createPorticoClient({
    transport: { fetch: async () => jsonResponse({
      id: "show-1",
      type: "show",
      title: "Fargo",
      sortTitle: "Fargo",
      genres: [],
      tags: [],
      labels: [],
      addedAt: "2026-07-12T12:00:00Z",
      images: {},
      state: {},
      actions: [],
      children: [{ id: "season-2", type: "season", title: "Season 2", sortTitle: "Season 2", genres: [], tags: [], labels: [], addedAt: "2026-07-12T12:00:00Z", images: {}, state: {}, actions: [] }],
      playbackTarget: target
    }) }
  });

  const detail = await client.media("show-1");
  assert.equal(detail.children.length, 1);
  assert.equal(detail.children[0].children, undefined);
  assert.equal(detail.playbackTarget.id, "episode-9");
  assert.equal(detail.playbackTarget.state.progressSeconds, 330);
});

test("ApiError exposes native-safe problem, request, response, and retry metadata", async () => {
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    requestId: () => "client-request-1",
    transport: {
      fetch: async (_input, init) => {
        assert.equal(init.headers["X-Request-ID"], "client-request-1");
        return jsonResponse({
          type: "https://getportico.tv/problems/server-busy",
          title: "Server busy",
          status: 503,
          code: "server_busy",
          detail: "Try again shortly.\u0000",
          requestId: "server-request-9",
          details: { job: "scan", progress: 42 }
        }, {
          status: 503,
          statusText: "Service Unavailable",
          headers: {
            "Content-Type": "application/problem+json",
            "Retry-After": "2.5",
            "X-Request-ID": "server-request-header"
          }
        });
      }
    }
  });

  await assert.rejects(client.system(), (error) => {
    assert.equal(error instanceof ApiError, true);
    assert.equal(error.name, "ApiError");
    assert.equal(error.status, 503);
    assert.equal(error.code, "server_busy");
    assert.equal(error.type, "https://getportico.tv/problems/server-busy");
    assert.equal(error.title, "Server busy");
    assert.equal(error.detail, "Try again shortly.");
    assert.equal(error.message, error.detail);
    assert.deepEqual(error.details, { job: "scan", progress: 42 });
    assert.equal(error.requestId, "server-request-9");
    assert.equal(error.responseHeaders["retry-after"], "2.5");
    assert.equal(error.retryAfter, "2.5");
    assert.equal(error.retryAfterMs, 2500);
    assert.match(error.retryAt, /^\d{4}-/);
    return true;
  });
});

test("ApiError falls back to response request ID and preserves extension details", async () => {
  const client = createPorticoClient({
    transport: {
      fetch: async () => jsonResponse({
        code: "invalid_field",
        title: "Invalid field",
        field: "email"
      }, {
        status: 400,
        statusText: "Bad Request",
        headers: { "X-Request-ID": "request-from-header" }
      })
    }
  });
  await assert.rejects(client.system(), (error) => {
    assert.equal(error.detail, "Invalid field");
    assert.equal(error.requestId, "request-from-header");
    assert.deepEqual(error.details, { field: "email" });
    return true;
  });
});

test("ApiError preserves terminal reconciliation headers and marks an unknown outcome ambiguous", async () => {
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    transport: {
      fetch: async (input) => {
        if (String(input).endsWith("/api/system")) return jsonResponse(hostedSystemFixture);
        return jsonResponse({
          code: "terminal_mutation_outcome_unknown",
          detail: "Portico is still confirming this change."
        }, {
          status: 503,
          headers: {
            "Retry-After": "1",
            "X-Portico-Terminal-Outcome": "outcome_unknown",
            "X-Portico-Terminal-Receipt": "tmr_example"
          }
        });
      }
    }
  });

  await assert.rejects(hosted.deleteAccount({ password: "secret" }), (error) => {
    assert.equal(error instanceof ApiError, true);
    assert.equal(error.ambiguous, true);
    assert.equal(error.responseHeaders["x-portico-terminal-outcome"], "outcome_unknown");
    assert.equal(error.responseHeaders["x-portico-terminal-receipt"], "tmr_example");
    return true;
  });
});

test("parseRetryAfter handles HTTP dates and invalid values", () => {
  assert.deepEqual(parseRetryAfter("Wed, 21 Oct 2015 07:28:00 GMT", Date.parse("2015-10-21T07:27:58Z")), {
    retryAfter: "Wed, 21 Oct 2015 07:28:00 GMT",
    retryAfterMs: 2000,
    retryAt: "2015-10-21T07:28:00.000Z"
  });
  assert.deepEqual(parseRetryAfter("later"), { retryAfter: "later" });
});

test("event streams use an injected React Native-compatible body and decoder adapter", async () => {
  const restoreTextDecoder = setGlobal("TextDecoder", undefined);
  const nativeDecoder = new (await import("node:util")).TextDecoder("utf-8");
  const encoded = Buffer.from(
    "event: data.changed\r\ndata: {\"id\":1,\"type\":\"data.changed\",\"createdAt\":\"2026-07-11T12:00:00Z\",\r\n" +
    "data: \"tags\":[\"home\",\"media\"]}\r\n\r\n"
  );
  const events = [];
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    requestId: () => "stream-request-1",
    transport: {
      fetch: async (_input, init) => {
        calls.push(init);
        return new Response(null, { status: 200, headers: { "Content-Type": "text/event-stream" } });
      }
    },
    eventStream: {
      async *read(_response, signal) {
        assert.equal(signal.aborted, false);
        yield encoded.subarray(0, 37);
        yield encoded.subarray(37);
      },
      decode(chunk, options) {
        return nativeDecoder.decode(chunk, options);
      },
      flush() {
        return nativeDecoder.decode();
      }
    }
  });

  try {
    await client.streamAppEvents(new AbortController().signal, (event) => events.push(event));
  } finally {
    restoreTextDecoder();
  }
  assert.equal(calls[0].headers.Accept, "text/event-stream");
  assert.equal(calls[0].headers["X-Request-ID"], "stream-request-1");
  assert.deepEqual(events.map((event) => event.tags), [["home", "media"]]);
});

test("event streams retain browser ReadableStream and TextDecoder defaults", async () => {
  const payload = new TextEncoder().encode(
    "data: {\"id\":2,\"type\":\"data.changed\",\"createdAt\":\"2026-07-11T12:00:01Z\",\"tags\":[\"settings\"]}\n\n"
  );
  const events = [];
  const client = createPorticoClient({
    transport: {
      fetch: async () => new Response(new ReadableStream({
        start(controller) {
          controller.enqueue(payload.subarray(0, 11));
          controller.enqueue(payload.subarray(11));
          controller.close();
        }
      }), { status: 200, headers: { "Content-Type": "text/event-stream" } })
    }
  });
  await client.streamAppEvents(new AbortController().signal, (event) => events.push(event));
  assert.deepEqual(events.map((event) => event.tags), [["settings"]]);
});

test("application SSE rejects the same malformed event shape as long-poll", async () => {
  const client = createPorticoClient({
    transport: {
      fetch: async () => new Response(null, { status: 200, headers: { "Content-Type": "text/event-stream" } })
    },
    eventStream: {
      async *read() {
        yield 'data: {"tags":["home"]}\n\n';
      }
    }
  });
  await assert.rejects(
    client.streamAppEvents(new AbortController().signal, () => undefined),
    error => error?.name === "PorticoEventProtocolError"
  );
});

test("Watch With Friends snapshots use the authenticated portable event-stream adapter", async () => {
  const snapshots = [];
  const calls = [];
  const group = {
    id: "group-one", name: "Movie Night", mediaId: "movie-one", mediaTitle: "One",
    ownerName: "Host", ownerProfileId: "profile-one", members: [], queue: [],
    permissions: {canControl: true, canManageQueue: true, isHost: true},
    command: {action: "pause"}, state: "paused", positionSeconds: 12,
    playbackRate: 1, revision: 3, playbackRevision: 2, reconnectGeneration: 1,
    repeatMode: "none", shuffleEnabled: false, createdAt: "2026-07-11T12:00:00Z",
    updatedAt: "2026-07-11T12:00:00Z", positionUpdatedAt: "2026-07-11T12:00:00Z",
    serverTime: "2026-07-11T12:00:00Z"
  };
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {fetch: async (input, init) => {
      calls.push({input: String(input), init});
      return new Response(null, {status: 200, headers: {"Content-Type": "text/event-stream"}});
    }},
    eventStream: {
      async *read() { yield `event: group\ndata: ${JSON.stringify(group)}\n\n`; }
    }
  });
  await client.streamWatchWithFriendsGroupEvents(
    "group-one",
    new AbortController().signal,
    snapshot => snapshots.push(snapshot)
  );
  assert.equal(calls[0].input, "https://server.example/api/watch-with-friends/groups/group-one/events");
  assert.equal(calls[0].init.headers.Accept, "text/event-stream");
  assert.deepEqual(snapshots.map(snapshot => snapshot.revision), [3]);
});

test("domain helpers preserve web UI filtering and route policy behavior", () => {
  assert.deepEqual(libraryTabs("music"), ["Discover", "Artists", "Albums", "Tracks", "Categories", "Collections"]);
  assert.equal(typedFilterForTab("Artists", "all", "music"), "type:artist");
  assert.equal(typedFilterForTab("Albums", "favorites", "music"), "type:album;favorites");
  assert.equal(typedFilterForTab("Tracks", "all", "music"), "type:track");
  assert.equal(typedFilterForTab("Library", "unwatched", "show"), "type:show;unwatched");
  assert.deepEqual(groupLibraryCategories([
    { id: "b", name: "Beta", group: "genre", filter: "genre:beta", count: 1 },
    { id: "a", name: "Alpha", group: "genre", filter: "genre:alpha", count: 4 }
  ])[0].items.map((item) => item.name), ["Alpha", "Beta"]);
  assert.equal(hostedDirectRouteAllowed("https://abc.direct.getportico.tv"), true);
  assert.equal(hostedDirectRouteAllowed("http://abc.direct.getportico.tv"), false);
  assert.deepEqual(dataTagsForMutation("/api/playlists/one/items"), ["playlists"]);
  assert.deepEqual(dataTagsForMutation("/api/playback-sessions/session-1"), ["playback"]);
});

test("browser playback profile falls back outside a browser", () => {
  assert.equal(browserPlaybackClientProfile().requiresServerProxy, true);
});

test("browser playback profile seals progressive audio formats and an exact generated HLS target", () => {
  const playable = new Set([
    "video/mp4",
    'video/mp4; codecs="avc1.42E01E"',
    'audio/mp4; codecs="mp4a.40.2"',
    "audio/mpeg",
    "audio/flac",
    'audio/ogg; codecs="opus"',
    'audio/webm; codecs="opus"',
    'audio/ogg; codecs="vorbis"',
    'audio/webm; codecs="vorbis"'
  ]);
  const restore = [
    setGlobal("document", { createElement: () => ({ canPlayType: (type) => playable.has(type) ? "probably" : "" }) }),
    setGlobal("navigator", {
      userAgent: "Mozilla/5.0 Chrome/148.0.0.0 Safari/537.36",
      platform: "MacIntel"
    }),
    setGlobal("screen", { width: 1920, height: 1080 }),
    setGlobal("MediaSource", function MediaSource() {}),
    setGlobal("devicePixelRatio", 2)
  ];
  try {
    const profile = browserPlaybackClientProfile();
    const tuples = profile.capabilityEvidence[0].tuples;
    for (const [container, codec] of [["mp3", "mp3"], ["flac", "flac"], ["ogg", "opus"], ["webm", "opus"], ["ogg", "vorbis"], ["webm", "vorbis"]]) {
      for (const [layout, channels] of [["mono", 1], ["stereo", 2]]) {
        assert.ok(tuples.some((tuple) => tuple.mediaKind === "audio" && tuple.protocol === "http" && tuple.container === container && tuple.audio?.codec === codec && tuple.audio.layout === layout && tuple.audio.maxChannels === channels));
      }
    }
    for (const [layout, channels] of [["mono", 1], ["stereo", 2]]) {
      assert.ok(tuples.some((tuple) => tuple.mediaKind === "audio" && tuple.protocol === "hls" && tuple.container === "mpegts" && tuple.audio?.codec === "aac" && tuple.audio.profile === "lc" && tuple.audio.layout === layout && tuple.audio.maxChannels === channels));
    }
    assert.equal(profile.supportedContainers.includes("flac"), true);
    assert.equal(profile.supportedContainers.includes("ogg"), true);
    assert.equal(profile.supportedAudioCodecs.includes("flac"), true);
    assert.equal(profile.supportedAudioCodecs.includes("opus"), true);
    assert.equal(profile.supportedAudioCodecs.includes("vorbis"), true);
  } finally {
    for (const restoreGlobal of restore.reverse()) restoreGlobal();
  }
});

test("browser playback profile keeps Safari audio and HEVC conservative", () => {
  const restore = [
    setGlobal("document", {
      createElement: () => ({
        canPlayType: (type) => {
          if (type === "application/vnd.apple.mpegurl" || type === "application/x-mpegURL") return "probably";
          if (type === "video/mp4" || type.includes("avc1") || type.includes("hvc1") || type.includes("hev1")) return "probably";
          if (type.includes("mp4a.40.2") || type.includes("ec-3") || type.includes("ac-3")) return "probably";
          return "";
        }
      })
    }),
    setGlobal("navigator", {
      userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
      platform: "MacIntel"
    }),
    setGlobal("screen", { width: 1920, height: 1080 }),
    setGlobal("MediaSource", function MediaSource() {}),
    setGlobal("devicePixelRatio", 2),
    setGlobal("matchMedia", () => ({ matches: true }))
  ];
  try {
    const profile = browserPlaybackClientProfile();
    assert.equal(profile.platform, "web");
    assert.match(profile.device, /MacIntel/);
    assert.ok(profile.capabilityEvidence[0].tuples.some((tuple) => tuple.protocol === "hls" && tuple.container === "mpegts" && tuple.subtitle.mode === "none"));
    assert.ok(profile.capabilityEvidence[0].tuples.some((tuple) => tuple.protocol === "hls" && tuple.container === "mpegts" && tuple.subtitle.mode === "native"));
    assert.equal(profile.supportsHls, true);
    assert.equal(profile.maxAudioChannels, 2);
    assert.equal(profile.supportsHevc, false);
    assert.equal(profile.supportsAc3, false);
    assert.equal(profile.supportsEac3, false);
    assert.equal(profile.supportedVideoCodecs.includes("hevc"), false);
    assert.equal(profile.supportedVideoCodecs.includes("h265"), false);
    assert.equal(profile.maxVideoBitDepth, 8);
    assert.equal(profile.supportsHdr, false);
    assert.deepEqual(profile.supportedHdrFormats, []);
    assert.deepEqual(profile.supportedDolbyVisionProfiles, []);
    assert.equal(profile.supportedVideoProfiles.includes("hevc:main 10"), false);
    assert.equal(profile.supportedPixelFormats.includes("p010le"), false);
    assert.equal(profile.supportedAudioCodecs.includes("ac3"), false);
    assert.equal(profile.supportedAudioCodecs.includes("eac3"), false);
  } finally {
    for (const restoreGlobal of restore.reverse()) restoreGlobal();
  }
});

test("browser playback profile keeps Chromium HEVC conservative even when reported", () => {
  const restore = [
    setGlobal("document", {
      createElement: () => ({
        canPlayType: (type) => {
          if (type === "application/vnd.apple.mpegurl" || type === "application/x-mpegURL") return "";
          if (type === "video/mp4" || type.includes("avc1") || type.includes("hvc1") || type.includes("hev1")) return "probably";
          if (type.includes("mp4a.40.2") || type.includes("ec-3") || type.includes("ac-3")) return "probably";
          return "";
        }
      })
    }),
    setGlobal("navigator", {
      userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36",
      platform: "MacIntel"
    }),
    setGlobal("screen", { width: 1920, height: 1080 }),
    setGlobal("MediaSource", function MediaSource() {}),
    setGlobal("devicePixelRatio", 2),
    setGlobal("matchMedia", () => ({ matches: true }))
  ];
  try {
    const profile = browserPlaybackClientProfile();
    assert.equal(profile.platform, "web");
    assert.match(profile.device, /MacIntel/);
    assert.equal(profile.supportsHls, true);
    assert.equal(profile.maxAudioChannels, 2);
    assert.equal(profile.supportsHevc, false);
    assert.equal(profile.supportsAc3, false);
    assert.equal(profile.supportsEac3, false);
    assert.equal(profile.supportedVideoCodecs.includes("hevc"), false);
    assert.equal(profile.supportedVideoCodecs.includes("h265"), false);
    assert.equal(profile.maxVideoBitDepth, 8);
    assert.deepEqual(profile.supportedHdrFormats, []);
    assert.equal(profile.supportedVideoProfiles.includes("hevc:main 10"), false);
    assert.equal(profile.supportedPixelFormats.includes("yuv422p10le"), false);
    assert.equal(profile.supportedAudioCodecs.includes("ac3"), false);
    assert.equal(profile.supportedAudioCodecs.includes("eac3"), false);
  } finally {
    for (const restoreGlobal of restore.reverse()) restoreGlobal();
  }
});

test("failed requests throw ApiError with server payload details", async () => {
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async () => new Response(JSON.stringify({
        type: "https://portico.media/problems/nope",
        title: "Bad Request",
        status: 400,
        code: "nope",
        detail: "Nope",
        field: "login",
        requestId: "req_test"
      }), {
        status: 400,
        statusText: "Bad Request",
        headers: { "Content-Type": "application/json" }
      })
    }
  });

  await assert.rejects(() => client.me(), (error) => {
    assert.equal(error instanceof ApiError, true);
    assert.equal(error.status, 400);
    assert.equal(error.code, "nope");
    assert.equal(error.details.field, "login");
    return true;
  });
});

test("normalizePlaybackResponse rejects an obsolete incomplete playback shape", () => {
  assert.throws(() => normalizePlaybackResponse({
    sessionId: "s1",
    media: { id: "m1", title: "Movie", type: "movie", state: {} },
    sourceUrl: "/api/media/m1/stream",
    directPlay: true,
    decision: { mode: "direct_play", reason: "", requiresTranscode: false, isProxied: true, isServerCached: false },
    qualityOffers: {offers: undefined},
    audioStreams: undefined,
    subtitleStreams: undefined,
    chapters: undefined,
    queue: undefined
  }), /qualityOffers must be an array/);
});

test("download and Watch With Friends lifecycle requests forward cancellation signals", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: (url, init) => new Promise((resolve, reject) => {
        calls.push({ url: String(url), init });
        init.signal?.addEventListener("abort", () => reject(init.signal.reason ?? new DOMException("Aborted", "AbortError")), { once: true });
      })
    }
  });
  const controller = new AbortController();
  const signal = controller.signal;

  const pending = [
    client.downloadOptions("media-1", { signal }),
    client.watchWithFriendsGroup("group-1", { signal }),
    client.leaveWatchWithFriendsGroup("group-1", { signal }),
    client.updateWatchWithFriendsMemberState("group-1", { state: "paused", positionSeconds: 12 }, { signal }),
  ];
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(calls.length, 4);
  controller.abort();
  await Promise.allSettled(pending);
  assert.equal(calls.every(call => call.init.signal?.aborted === true), true);
});

test("library scan client exposes the durable operation lifecycle and required modes", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (url, init = {}) => {
        calls.push({ url: String(url), init });
        return jsonResponse({});
      }
    }
  });

  await client.scanLibrary("library/1", { mode: "quick" });
  await client.libraryScanOperations("library/1");
  await client.libraryScanReview("library/1", { limit: 25, cursor: "next page" });
  await client.libraryScanRuns("library/1", { limit: 10 });
  await client.cancelLibraryScan("library/1");
  await client.retryLibraryScan("library/1", { runId: "run-7" });

  assert.equal(calls[0].url.endsWith("/api/libraries/library%2F1/scan"), true);
  assert.equal(calls[0].init.body, JSON.stringify({ mode: "quick" }));
  assert.equal(calls[1].url.endsWith("/api/libraries/library%2F1/scan-operations"), true);
  assert.equal(calls[2].url.includes("/scan-review?limit=25&cursor=next+page"), true);
  assert.equal(calls[3].url.endsWith("/scan-runs?limit=10"), true);
  assert.equal(calls[4].url.endsWith("/scan/cancel"), true);
  assert.equal(calls[5].url.endsWith("/scan/retry"), true);
  assert.equal(calls[5].init.body, JSON.stringify({ runId: "run-7" }));
});
