import assert from "node:assert/strict";
import {createHash, createPublicKey, generateKeyPairSync, sign, verify} from "node:crypto";
import test from "node:test";

import {
  canonicalOfflineDownloadAuthorizationPayload,
  parseOfflineDownloadAuthorizationReceipt,
  pinnedServerIdentityFromDurableSource,
  revalidateOfflineDownloadAuthorization,
  validateOfflineDownloadAuthorizationReceipt
} from "../dist/offlineDownloadAuthorization.js";

const {privateKey, publicKey} = generateKeyPairSync("ed25519");
const publicDER = publicKey.export({format: "der", type: "spki"});
const rawPublicKey = new Uint8Array(publicDER.subarray(publicDER.length - 32));
const base64url = value => Buffer.from(value).toString("base64url");
const fingerprint = `sha256:${base64url(createHash("sha256").update(rawPublicKey).digest())}`;
const runtime = {
  encodeBase64: base64url,
  decodeBase64: value => new Uint8Array(Buffer.from(value, "base64")),
  encodeText: value => new TextEncoder().encode(value),
  sha256: value => new Uint8Array(createHash("sha256").update(value).digest()),
  verifyEd25519: ({publicKey: raw, signature, message}) => {
    const key = createPublicKey({key: Buffer.concat([Buffer.from("302a300506032b6570032100", "hex"), Buffer.from(raw)]), format: "der", type: "spki"});
    return verify(null, Buffer.from(message), key, Buffer.from(signature));
  }
};

const scopeA = Object.freeze({authority: "local", accountId: "account-a", profileId: "profile-a", serverId: "server-a", authorizationRevision: "revision-7"});
const scopeB = Object.freeze({...scopeA, profileId: "profile-b", authorizationRevision: "revision-9"});
const artifact = Object.freeze({sha256: createHash("sha256").update("protected-bytes").digest("hex"), sizeBytes: 15});
const pinnedIdentity = Object.freeze({serverId: "server-a", publicKeyFingerprint: fingerprint, publicKey: rawPublicKey});
const encodedPublicKey = Buffer.from(rawPublicKey).toString("base64").replace(/=+$/u, "");

function signedReceipt({receiptId = "receipt-1", viewerScope = scopeA, artifactOverride = artifact, lastVerifiedAt = "2026-08-30T12:00:00.000Z"} = {}) {
  const receipt = {
    version: 1,
    purpose: "offline-download-authorization",
    receiptId,
    viewerScope: {scopeKind: "server-bound", ...viewerScope},
    issuer: {serverId: viewerScope.serverId, signingKeyFingerprint: fingerprint},
    preparation: {preparationId: "preparation-1", mediaId: "media-1", mediaVersionId: "version-1", qualityId: "source"},
    artifact: artifactOverride,
    lastVerifiedAt,
    verifyBy: new Date(Date.parse(lastVerifiedAt) + 2_592_000_000).toISOString(),
    signature: ""
  };
  receipt.signature = sign(null, Buffer.from(canonicalOfflineDownloadAuthorizationPayload(receipt)), privateKey).toString("base64url");
  return Object.freeze(receipt);
}

function context(activeViewerScope = scopeA, overrides = {}) {
  return {
    binding: {
      storedViewerScope: scopeA,
      originatingServerId: "server-a",
      preparationId: "preparation-1",
      mediaId: "media-1",
      mediaVersionId: "version-1",
      qualityId: "source",
      artifactSha256: artifact.sha256,
      artifactSizeBytes: artifact.sizeBytes,
      ...overrides
    },
    activeViewerScope,
    pinnedIdentity,
    runtimeAdapters: runtime,
    now: Date.parse("2026-08-31T12:00:00Z")
  };
}

test("durable connection identity supplies the exact offline receipt pin", async () => {
  const derived = await pinnedServerIdentityFromDurableSource({
    serverId: "server-a",
    serverPublicKey: encodedPublicKey,
    serverPublicKeyFingerprint: fingerprint
  }, runtime);
  assert.equal(derived.serverId, "server-a");
  assert.equal(derived.publicKeyFingerprint, fingerprint);
  assert.deepEqual(derived.publicKey, rawPublicKey);
  await assert.rejects(pinnedServerIdentityFromDurableSource({
    serverId: "server-a",
    serverPublicKey: encodedPublicKey,
    serverPublicKeyFingerprint: `sha256:${"A".repeat(43)}`
  }, runtime), /does not match/);
  await assert.rejects(pinnedServerIdentityFromDurableSource({
    serverId: "server-a",
    serverPublicKey: "",
    serverPublicKeyFingerprint: fingerprint
  }, runtime), /incomplete/);
});

test("receipt JCS is member-order independent and preserves Unicode scalars", () => {
  const receipt = signedReceipt({receiptId: "réceipt-😀"});
  const reordered = {
    signature: receipt.signature, verifyBy: receipt.verifyBy, lastVerifiedAt: receipt.lastVerifiedAt,
    artifact: {sizeBytes: receipt.artifact.sizeBytes, sha256: receipt.artifact.sha256},
    preparation: {...receipt.preparation}, issuer: {...receipt.issuer}, viewerScope: {...receipt.viewerScope},
    receiptId: receipt.receiptId, purpose: receipt.purpose, version: receipt.version
  };
  assert.equal(canonicalOfflineDownloadAuthorizationPayload(reordered), canonicalOfflineDownloadAuthorizationPayload(receipt));
  assert.equal(parseOfflineDownloadAuthorizationReceipt(reordered).receiptId, "réceipt-😀");
  assert.throws(() => parseOfflineDownloadAuthorizationReceipt({...reordered, receiptId: "bad\ud800"}));
  assert.throws(() => parseOfflineDownloadAuthorizationReceipt({...reordered, signature: `${receipt.signature.slice(0, -1)}B`}));
});

test("valid receipt is strict-before verifyBy and wrong active profile is non-destructive", async () => {
  const receipt = signedReceipt();
  assert.equal((await validateOfflineDownloadAuthorizationReceipt(receipt, context())).state, "valid");
  const wrongActiveProfile = await validateOfflineDownloadAuthorizationReceipt(receipt, context(scopeB));
  assert.deepEqual(wrongActiveProfile, {state: "out-of-scope", deleteProtectedArtifact: false});
  const corruptStoredReceiptUnderOtherProfile = await validateOfflineDownloadAuthorizationReceipt({}, context(scopeB));
  assert.deepEqual(corruptStoredReceiptUnderOtherProfile, {state: "out-of-scope", deleteProtectedArtifact: false});
  const atBoundary = await validateOfflineDownloadAuthorizationReceipt(receipt, {...context(), now: Date.parse(receipt.verifyBy)});
  assert.equal(atBoundary.state, "authorization-unverified");
  const intrinsicDigestMismatch = await validateOfflineDownloadAuthorizationReceipt(receipt, context(scopeA, {artifactSha256: "0".repeat(64)}));
  assert.deepEqual(intrinsicDigestMismatch, {state: "invalid", deleteProtectedArtifact: true});
  const otherKey = generateKeyPairSync("ed25519").publicKey.export({format: "der", type: "spki"}).subarray(-32);
  const wrongPinnedKey = await validateOfflineDownloadAuthorizationReceipt(receipt, {...context(), pinnedIdentity: {...pinnedIdentity, publicKey: otherKey}});
  assert.equal(wrongPinnedKey.state, "invalid");
});

test("replacement is accepted only after full pinned signature and binding validation", async () => {
  const receipt = signedReceipt();
  const replacement = signedReceipt({receiptId: "receipt-2", lastVerifiedAt: "2026-08-31T12:00:01.000Z"});
  const valid = await revalidateOfflineDownloadAuthorization({
    async revalidateOfflineDownloadAuthorization() { return {outcome: "valid-replacement", receipt: replacement}; }
  }, receipt, context());
  assert.deepEqual(valid, {action: "replace", receipt: replacement});

  const hostile = [
    {...replacement, signature: `${replacement.signature.slice(0, -1)}${replacement.signature.endsWith("A") ? "Q" : "A"}`},
    signedReceipt({receiptId: "receipt-wrong-artifact", artifactOverride: {sha256: "0".repeat(64), sizeBytes: 15}}),
    signedReceipt({receiptId: "receipt-wrong-scope", viewerScope: {...scopeA, profileId: "profile-c"}}),
    signedReceipt({receiptId: "receipt-expired", lastVerifiedAt: "2026-06-01T00:00:00.000Z"})
  ];
  for (const candidate of hostile) {
    const decision = await revalidateOfflineDownloadAuthorization({
      async revalidateOfflineDownloadAuthorization() { return {outcome: "valid-replacement", receipt: candidate}; }
    }, receipt, context());
    assert.deepEqual(decision, {action: "preserve", outcome: "transport-failure"});
  }
  assert.deepEqual(await revalidateOfflineDownloadAuthorization({
    async revalidateOfflineDownloadAuthorization() { throw new Error("offline"); }
  }, receipt, context()), {action: "preserve", outcome: "transport-failure"});
  assert.deepEqual(await revalidateOfflineDownloadAuthorization({
    async revalidateOfflineDownloadAuthorization() { return {outcome: "revoked"}; }
  }, receipt, context()), {action: "delete", outcome: "revoked"});
  assert.deepEqual(await revalidateOfflineDownloadAuthorization({
    async revalidateOfflineDownloadAuthorization() { return {outcome: "out-of-scope"}; }
  }, receipt, context()), {action: "preserve", outcome: "out-of-scope"});
});
