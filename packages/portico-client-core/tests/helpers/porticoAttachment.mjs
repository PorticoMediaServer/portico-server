import { createHash } from "node:crypto";

const serverIdentity = Buffer.alloc(32, 0x5a);

export const testServerPublicKey = serverIdentity.toString("base64");
export const testServerPublicKeyURL = serverIdentity.toString("base64url");
export const testServerPublicKeyFingerprint = `sha256:${createHash("sha256").update(serverIdentity).digest("base64url")}`;

export function testServerIdentity(overrides = {}) {
  return {
    serverPublicKey: testServerPublicKey,
    serverPublicKeyFingerprint: testServerPublicKeyFingerprint,
    ...overrides
  };
}

export function createAttachmentRuntime(now = "2026-07-11T00:01:00.000Z", overrides = {}) {
  const encodeBase64 = value => Buffer.from(value).toString("base64url");
  return {
    decodeBase64: value => Uint8Array.from(Buffer.from(value, "base64")),
    encodeBase64,
    encodeText: value => Uint8Array.from(Buffer.from(value, "utf8")),
    decodeText: value => Buffer.from(value).toString("utf8"),
    secureRandom: length => Uint8Array.from(Buffer.alloc(length, 0x31)),
    sha256: value => Uint8Array.from(createHash("sha256").update(value).digest()),
    verifyEd25519: () => true,
    createAttachmentKeyAgreement: async () => ({
      publicKey: Uint8Array.from(Buffer.concat([Buffer.from([0x04]), Buffer.alloc(64, 0x41)])),
      seal: async input => Uint8Array.from(input.payload),
      open: async input => Uint8Array.from(input.payload)
    }),
    now: () => new Date(now),
    ...overrides
  };
}

export function createAttachmentMethods({
  sessionStore,
  audience,
  serverId,
  credentials,
  now = "2026-07-11T00:01:00.000Z",
  onPlaintextRequest,
  onAccept
}) {
  let handshakeId = 0;
  let responseCredentials = credentials;
  return {
    createPorticoAttachmentHandshake: async request => {
      const issuedAt = new Date(now);
      const expiresAt = new Date(issuedAt.getTime() + 60_000);
      const apiBaseUrl = audience ?? sessionStore?.get()?.apiBaseUrl;
      if (!apiBaseUrl) throw new Error("The test attachment requires the provisional server session.");
      return {
        version: 1,
        handshakeId: `test-handshake-${++handshakeId}`,
        serverId,
        serverPublicKey: testServerPublicKeyURL,
        serverPublicKeyFingerprint: testServerPublicKeyFingerprint,
        clientPublicKey: request.clientPublicKey,
        clientNonce: request.clientNonce,
        serverEphemeralPublicKey: Buffer.concat([Buffer.from([0x04]), Buffer.alloc(64, 0x42)]).toString("base64url"),
        audience: new URL(apiBaseUrl).origin,
        issuedAt: issuedAt.toISOString(),
        expiresAt: expiresAt.toISOString(),
        signatureAlgorithm: "ed25519",
        signature: Buffer.alloc(64, 0x43).toString("base64url")
      };
    },
    exchangeEncryptedPorticoSession: async request => {
      const plaintext = JSON.parse(Buffer.from(request.ciphertext, "base64").toString("utf8"));
      onPlaintextRequest?.(plaintext);
      const resolved = typeof responseCredentials === "function"
        ? await responseCredentials(plaintext)
        : responseCredentials;
      return {
        ok: true,
        status: 200,
        payload: {
          version: 1,
          handshakeId: request.handshakeId,
          ciphertext: Buffer.from(JSON.stringify({status: 200, body: resolved})).toString("base64url")
        }
      };
    },
    acceptPorticoSessionCredentials: async next => {
      onAccept?.(next);
      return next;
    },
    setAttachmentCredentials(next) {
      responseCredentials = next;
    }
  };
}
