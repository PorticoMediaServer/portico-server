import assert from "node:assert/strict";
import {createHash, createPublicKey, generateKeyPairSync, sign, verify} from "node:crypto";
import test from "node:test";
import {
  applyNativeDownloadProgress,
  applyNativeDownloadPreparation,
  authorizeNativeDownloadTransfer,
  beginNativeDownloadPreparation,
  canonicalOfflineDownloadAuthorizationPayload,
  nativeDownloadSchedule,
  nativeDownloadTombstone,
  nativeDownloadQueueItemFromPreparation,
  nativeDownloadVariants,
  nativeDownloadsToDeleteAfterWatch,
  nativePlaybackCoordinationPlan,
  portableSleepTimer,
  publishNativeDownloadArtifact,
  refreshNativeDownloadPreparation,
  reduceNativeDownloadQueueItem,
  reconcileNativeDownloads,
  sleepTimerShouldStop
} from "../dist/index.js";

const {privateKey: receiptPrivateKey, publicKey: receiptPublicKey} = generateKeyPairSync("ed25519");
const receiptPublicDER = receiptPublicKey.export({format: "der", type: "spki"});
const receiptRawPublicKey = new Uint8Array(receiptPublicDER.subarray(receiptPublicDER.length - 32));
const receiptBase64URL = value => Buffer.from(value).toString("base64url");
const receiptFingerprint = `sha256:${receiptBase64URL(createHash("sha256").update(receiptRawPublicKey).digest())}`;
const receiptViewerScope = Object.freeze({authority: "local", accountId: "account", profileId: "profile", serverId: "server", authorizationRevision: "revision-1"});
const receiptPinnedIdentity = Object.freeze({serverId: "server", publicKeyFingerprint: receiptFingerprint, publicKey: receiptRawPublicKey});
const receiptRuntime = {
  encodeBase64: receiptBase64URL,
  decodeBase64: value => new Uint8Array(Buffer.from(value, "base64")),
  encodeText: value => new TextEncoder().encode(value),
  sha256: value => new Uint8Array(createHash("sha256").update(value).digest()),
  verifyEd25519: ({publicKey, signature, message}) => {
    const key = createPublicKey({key: Buffer.concat([Buffer.from("302a300506032b6570032100", "hex"), Buffer.from(publicKey)]), format: "der", type: "spki"});
    return verify(null, Buffer.from(message), key, Buffer.from(signature));
  }
};

function signedNativeReceipt({
  receiptId = "receipt-native", preparationId = "prep", mediaId = "movie", mediaVersionId = "version-1",
  qualityId = "source", sizeBytes = 100, lastVerifiedAt = "2026-07-16T00:00:00.000Z"
} = {}) {
  const receipt = {
    version: 1,
    purpose: "offline-download-authorization",
    receiptId,
    viewerScope: {scopeKind: "server-bound", ...receiptViewerScope},
    issuer: {serverId: "server", signingKeyFingerprint: receiptFingerprint},
    preparation: {preparationId, mediaId, mediaVersionId, qualityId},
    artifact: {sha256: createHash("sha256").update(`${preparationId}:${mediaId}:${qualityId}:${sizeBytes}`).digest("hex"), sizeBytes},
    lastVerifiedAt,
    verifyBy: new Date(Date.parse(lastVerifiedAt) + 2_592_000_000).toISOString(),
    signature: ""
  };
  receipt.signature = sign(null, Buffer.from(canonicalOfflineDownloadAuthorizationPayload(receipt)), receiptPrivateKey).toString("base64url");
  return Object.freeze(receipt);
}

function nativeGrantBinding(scope, preparationId, attemptId, generation) {
  return {...scope, trustedRouteUrl: "https://server.example", preparationId, attemptId, generation, viewerScope: receiptViewerScope, pinnedServerIdentity: receiptPinnedIdentity};
}

test("native playback plans expose only shell-reported capabilities", () => {
  const capabilities = { pictureInPicture: true, nowPlaying: true, backgroundAudio: true };
  assert.equal(nativePlaybackCoordinationPlan("video", capabilities).allowBackgroundAudio, false);
  assert.equal(nativePlaybackCoordinationPlan("music", capabilities).allowPictureInPicture, false);
  assert.equal(nativePlaybackCoordinationPlan("audiobook", capabilities).allowBackgroundAudio, true);
  const timer = portableSleepTimer(15, 1000);
  assert.equal(sleepTimerShouldStop(timer, { type: "tick", now: 1000 + 15 * 60_000 }), true);
});

test("native downloads remain scoped and wait for network, storage, and renewed grants", () => {
  const scope = { serverId: "server", accountId: "account", profileId: "profile", installationId: "phone" };
  const other = { ...scope, profileId: "other" };
  const queue = [
    { id: "one", preparationId: "prep", mediaId: "movie", scope, state: "queued", expectedBytes: 500, transferredBytes: 0, transferGrantExpiresAt: new Date(10_000).toISOString() },
    { id: "two", preparationId: "prep-2", mediaId: "private", scope: other, state: "queued", expectedBytes: 10, transferredBytes: 0, transferGrantExpiresAt: new Date(10_000).toISOString() }
  ];
  assert.equal(nativeDownloadSchedule(queue, scope, "cellular", { wifiOnly: true, usedStorageBytes: 0, deleteWatched: false }, 1000)[0].state, "waiting-network");
  assert.equal(nativeDownloadSchedule(queue, scope, "wifi", { wifiOnly: true, storageLimitBytes: 400, usedStorageBytes: 0, deleteWatched: false }, 1000)[0].state, "waiting-storage");
  assert.equal(nativeDownloadSchedule(queue, scope, "wifi", { wifiOnly: true, storageLimitBytes: 1000, usedStorageBytes: 0, deleteWatched: false }, 1000)[0].state, "transferring");
  assert.equal(nativeDownloadSchedule(queue, scope, "wifi", { wifiOnly: true, storageLimitBytes: 1000, usedStorageBytes: 0, deleteWatched: false }, 20_000)[0].state, "waiting-grant");
  assert.equal(nativeDownloadSchedule(queue, scope, "wifi", { wifiOnly: true, storageLimitBytes: 1000, usedStorageBytes: 0, deleteWatched: false }, 1000)[1].state, "queued");
});

test("native progress enters verification and only receipt-verified publication becomes offline authority", async () => {
  const scope = { serverId: "server", accountId: "account", profileId: "profile", installationId: "phone" };
  const receipt = signedNativeReceipt();
  const verifying = applyNativeDownloadProgress({
    id: "one", attemptId: "attempt", generation: 0, preparationId: "prep", mediaId: "movie", scope,
    state: "transferring", variant: {optionId: "source", kind: "source", qualityProfile: "source", label: "Original"},
    transferredBytes: 50, watched: true, authorizationReceipt: receipt, authorizationViewerScope: receiptViewerScope
  }, 100, 100, {attemptId: "attempt", generation: 0});
  assert.equal(verifying.state, "verifying");
  assert.deepEqual(nativeDownloadsToDeleteAfterWatch([verifying], scope, true), []);
  const available = await publishNativeDownloadArtifact(verifying, {
    viewerScope: receiptViewerScope,
    pinnedServerIdentity: receiptPinnedIdentity,
    artifactSha256: receipt.artifact.sha256,
    artifactSizeBytes: receipt.artifact.sizeBytes,
    offline: {title: "Movie", mediaType: "movie", localAssetId: "asset", artworkAssetIds: ["poster"]}
  }, {attemptId: "attempt", generation: 0}, receiptRuntime, Date.parse("2026-07-17T00:00:00Z"));
  assert.equal(available.state, "available-offline");
  assert.deepEqual(nativeDownloadsToDeleteAfterWatch([available], scope, true), ["one"]);
});

test("native queue transitions preserve portable pause, retry, grant, and removal semantics", () => {
  const scope = { serverId: "server", accountId: "account", profileId: "profile", installationId: "phone" };
  const queued = { id: "one", attemptId: "attempt", generation: 0, preparationId: "prep", mediaId: "movie", scope, state: "queued", transferredBytes: 0, offline: { title: "Movie", mediaType: "movie", localAssetId: "asset", artworkAssetIds: ["poster"] } };
  const paused = reduceNativeDownloadQueueItem(queued, { type: "pause", attemptId: "attempt", generation: 0 });
  assert.equal(paused.state, "paused");
  assert.equal(reduceNativeDownloadQueueItem(paused, { type: "resume", attemptId: "attempt", generation: 0 }).state, "queued");
  const failed = reduceNativeDownloadQueueItem(queued, { type: "fail", attemptId: "attempt", generation: 0, messageId: "download.storage-full" });
  assert.equal(failed.failureMessageId, "download.storage-full");
  assert.equal(reduceNativeDownloadQueueItem(failed, { type: "retry", attemptId: "attempt", generation: 0, nextAttemptId: "attempt-2" }).state, "queued");
  const removed = reduceNativeDownloadQueueItem(queued, { type: "remove", attemptId: "attempt", generation: 0 });
  assert.equal(removed.state, "removed");
  assert.equal(removed.offline.localAssetId, undefined);
  assert.deepEqual(removed.offline.artworkAssetIds, []);
});

test("native download workflow keeps optimized options first-class through preparation and grant", async () => {
  const calls = [];
  const running = {
    id: "prep-optimized", mediaId: "movie", mediaTitle: "Movie", qualityProfile: "video-standard",
    state: "running", progress: 40, sizeKind: "estimated", sizeBytes: 800,
    canPause: true, canCancel: true, canRetry: false, canRemove: false,
    createdAt: "2026-07-16T00:00:00Z", updatedAt: "2026-07-16T00:00:01Z"
  };
  const ready = { ...running, state: "ready", progress: 100, sizeKind: "exact", sizeBytes: 760, artifactExpiresAt: "2026-07-17T00:00:00Z" };
  const api = {
    async downloadOptions(mediaId) {
      calls.push(["options", mediaId]);
      return {
        media: { id: mediaId }, canDownload: true, optimizedVersions: [], profiles: [], defaultProfile: "video-standard",
        options: [
          { id: "source", kind: "source", label: "Original", available: true, sizeBytes: 1200 },
          { id: "standard-option", kind: "optimized", profile: "video-standard", label: "Standard", available: true, sizeBytes: 800 },
          { id: "future", kind: "optimized", profile: "video-low", label: "Low", available: false }
        ]
      };
    },
    async createDownloadPreparation(body) { calls.push(["prepare", body]); return running; },
    async downloadPreparation(id) { calls.push(["refresh", id]); return ready; },
    async createDownloadPreparationGrant(id) {
      calls.push(["grant", id]);
      return {
        downloadUrl: "/opaque/download", grantToken: "secret", expiresAt: "2026-07-16T01:00:00Z", profile: "video-standard",
        authorizationReceipt: signedNativeReceipt({preparationId: "prep-optimized", mediaId: "movie", qualityId: "video-standard", sizeBytes: 760})
      };
    }
  };

  const options = await api.downloadOptions("movie");
  assert.deepEqual(nativeDownloadVariants(options).map(({ kind, qualityProfile }) => ({ kind, qualityProfile })), [
    { kind: "source", qualityProfile: "source" },
    { kind: "optimized", qualityProfile: "video-standard" }
  ]);
  calls.length = 0;
  const started = await beginNativeDownloadPreparation(api, "movie", "standard-option");
  assert.equal(started.variant.kind, "optimized");
  assert.deepEqual(calls[1], ["prepare", { mediaId: "movie", qualityProfile: "video-standard" }]);
  const scope = { serverId: "server", accountId: "account", profileId: "profile", installationId: "phone" };
  const preparing = nativeDownloadQueueItemFromPreparation("queue-1", started, scope);
  assert.equal(preparing.state, "preparing");
  assert.equal(preparing.expectedBytes, 800);
  const refreshed = await refreshNativeDownloadPreparation(api, started);
  const queued = applyNativeDownloadPreparation(preparing, refreshed.preparation, {attemptId: preparing.attemptId, generation: preparing.generation});
  assert.equal(queued.state, "queued");
  assert.equal(queued.sizeKind, "exact");
  assert.equal(queued.expectedBytes, 760);
  const transfer = await authorizeNativeDownloadTransfer(api, refreshed, Date.parse("2026-07-16T00:30:00Z"), nativeGrantBinding(scope, refreshed.preparation.id, queued.attemptId, queued.generation), receiptRuntime);
  assert.equal(transfer.downloadUrl, "https://server.example/opaque/download");
  assert.equal(transfer.authorization, "PorticoDownload secret");
  assert.equal(transfer.qualityProfile, "video-standard");
  assert.equal(transfer.expectedBytes, 760);
});

test("native scheduler conservatively reserves concurrent unknown-size jobs", () => {
  const scope = { serverId: "server", accountId: "account", profileId: "profile", installationId: "phone" };
  const expiresAt = new Date(10_000).toISOString();
  const queue = ["one", "two"].map((id) => ({
    id, preparationId: `prep-${id}`, mediaId: `movie-${id}`, scope, state: "queued",
    sizeKind: "unknown", transferredBytes: 0, transferGrantExpiresAt: expiresAt
  }));
  const basePolicy = { wifiOnly: false, storageLimitBytes: 1000, usedStorageBytes: 0, deleteWatched: false };
  assert.deepEqual(nativeDownloadSchedule(queue, scope, "wifi", basePolicy, 1000).map((item) => item.state), ["waiting-storage", "waiting-storage"]);
  assert.deepEqual(nativeDownloadSchedule(queue, scope, "wifi", { ...basePolicy, unknownSizeReservationBytes: 600 }, 1000).map((item) => item.state), ["transferring", "waiting-storage"]);
});

test("native transfer completion records a transport fact but does not publish offline assets", async () => {
  const scope = { serverId: "server", accountId: "account", profileId: "profile", installationId: "phone" };
  const receipt = signedNativeReceipt({preparationId: "prep", mediaId: "book", sizeBytes: 720});
  const transferring = {
    id: "one", attemptId: "attempt", generation: 0, preparationId: "prep", mediaId: "book", scope, state: "transferring",
    sizeKind: "unknown", transferredBytes: 719,
    variant: {optionId: "source", kind: "source", qualityProfile: "source", label: "Original"},
    authorizationReceipt: receipt, authorizationViewerScope: receiptViewerScope
  };
  const offline = {
    title: "Book",
    mediaType: "audiobook",
    durationSeconds: 3600,
    localAssetId: "local-media-1",
    artworkAssetIds: ["local-cover-1"]
  };
  const verifying = reduceNativeDownloadQueueItem(transferring, { type: "complete", attemptId: "attempt", generation: 0, finalBytes: 720 });
  assert.equal(verifying.state, "verifying");
  assert.equal(verifying.sizeKind, "exact");
  assert.equal(verifying.expectedBytes, 720);
  assert.equal(verifying.transferredBytes, 720);
  assert.equal(verifying.offline, undefined);
  const available = await publishNativeDownloadArtifact(verifying, {
    viewerScope: receiptViewerScope, pinnedServerIdentity: receiptPinnedIdentity,
    artifactSha256: receipt.artifact.sha256, artifactSizeBytes: 720, offline
  }, {attemptId: "attempt", generation: 0}, receiptRuntime, Date.parse("2026-07-17T00:00:00Z"));
  assert.equal(available.state, "available-offline");
  assert.deepEqual(available.offline, offline);
});

test("estimated progress waits for completion and authorization becomes unverified exactly at verifyBy", () => {
  const scope = { serverId: "server", accountId: "account", profileId: "profile", installationId: "phone" };
  const estimated = {
    id: "one", attemptId: "attempt", generation: 0, preparationId: "prep", mediaId: "movie", scope, state: "transferring",
    sizeKind: "estimated", expectedBytes: 100, transferredBytes: 90
  };
  assert.equal(applyNativeDownloadProgress(estimated, 100, undefined, {attemptId: "attempt", generation: 0}).state, "transferring");
  const receipt = signedNativeReceipt({sizeBytes: 110});
  const available = {...estimated, state: "available-offline", transferredBytes: 110, expectedBytes: 110, sizeKind: "exact", authorizationReceipt: receipt};
  assert.equal(nativeDownloadSchedule([available], scope, "offline", { wifiOnly: false, usedStorageBytes: 110, deleteWatched: false }, Date.parse(receipt.verifyBy))[0].state, "authorization-unverified");
});

test("stale native callbacks cannot revive removed or superseded attempts", () => {
  const scope = { serverId: "server", accountId: "account", profileId: "profile", installationId: "phone" };
  const item = { id: "one", attemptId: "attempt-1", generation: 1, preparationId: "prep", mediaId: "movie", scope, state: "transferring", transferredBytes: 2 };
  const removed = reduceNativeDownloadQueueItem(item, { type: "remove", attemptId: "attempt-1", generation: 1 });
  assert.equal(reduceNativeDownloadQueueItem(removed, { type: "complete", attemptId: "attempt-1", generation: 1, finalBytes: 3 }).state, "removed");
  const cancelled = reduceNativeDownloadQueueItem(item, { type: "cancel", attemptId: "attempt-1", generation: 1 });
  const retried = reduceNativeDownloadQueueItem(cancelled, { type: "retry", attemptId: "attempt-1", generation: 1, nextAttemptId: "attempt-2" });
  assert.equal(reduceNativeDownloadQueueItem(retried, {
    type: "grant", attemptId: "attempt-1", generation: 1, expiresAt: new Date(Date.now() + 60_000).toISOString(),
    receipt: signedNativeReceipt(), viewerScope: receiptViewerScope
  }).transferGrantExpiresAt, undefined);
});

test("restart reconciliation retains tombstones until platform work is gone", () => {
  const scope = {serverId: "server", accountId: "account", profileId: "profile", installationId: "phone"};
  const item = {id: "one", attemptId: "attempt", generation: 2, preparationId: "prep", mediaId: "movie", scope, state: "removed", transferredBytes: 0};
  const tombstone = nativeDownloadTombstone(item, new Date("2026-08-01T00:00:00Z"));
  const task = {attemptId: "attempt", generation: 2, state: "running"};
  const pending = reconcileNativeDownloads([item], [tombstone], [task]);
  assert.deepEqual(pending.queue, []);
  assert.deepEqual(pending.cancelPlatformTasks, [task]);
  assert.deepEqual(pending.confirmedTombstones, []);
  assert.deepEqual(reconcileNativeDownloads([], [tombstone], []).confirmedTombstones, [tombstone]);
});

test("native transfer grants are bound to trusted route and principal attempt", async () => {
  const preparation = {id: "prep", mediaId: "movie", qualityProfile: "source", state: "ready", progress: 100, sizeKind: "exact", sizeBytes: 10};
  const result = {preparation, variant: {optionId: "source", kind: "source", qualityProfile: "source", label: "Original"}};
  const scope = {serverId: "server", accountId: "account", profileId: "profile", installationId: "phone"};
  const binding = nativeGrantBinding(scope, "prep", "attempt", 3);
  const receipt = signedNativeReceipt({sizeBytes: 10, lastVerifiedAt: "2026-08-01T00:00:00.000Z"});
  for (const downloadUrl of ["http://server.example/file", "https://user@server.example/file", "https://server.example/file#secret", "https://evil.example/file"]) {
    const api = {async createDownloadPreparationGrant() { return {downloadUrl, grantToken: "secret", expiresAt: "2026-09-01T00:00:00Z", profile: "source", authorizationReceipt: receipt}; }};
    await assert.rejects(authorizeNativeDownloadTransfer(api, result, Date.parse("2026-08-01T00:00:00Z"), binding, receiptRuntime), error => error.code === "grant_invalid");
  }
  const api = {async createDownloadPreparationGrant() { return {downloadUrl: "/file", grantToken: "secret", expiresAt: "2026-09-01T00:00:00Z", profile: "source", authorizationReceipt: receipt}; }};
  await assert.rejects(authorizeNativeDownloadTransfer(api, result, Date.parse("2026-08-01T00:00:00Z"), {...binding, preparationId: "other"}, receiptRuntime), error => error.code === "grant_invalid");
});
