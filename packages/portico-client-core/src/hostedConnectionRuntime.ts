export interface Ed25519VerificationInput {
  publicKey: Uint8Array;
  signature: Uint8Array;
  message: Uint8Array;
}

export interface PorticoAttachmentCryptInput {
  peerPublicKey: Uint8Array;
  transcript: Uint8Array;
  nonce: Uint8Array;
  additionalData: Uint8Array;
  payload: Uint8Array;
}

export interface PorticoAttachmentKeyAgreement {
  publicKey: Uint8Array;
  seal: (input: PorticoAttachmentCryptInput) => Promise<Uint8Array>;
  open: (input: PorticoAttachmentCryptInput) => Promise<Uint8Array>;
}

export interface HostedConnectionRuntimeAdapters {
  fetch?: typeof fetch;
  decodeBase64?: (value: string) => Uint8Array;
  encodeBase64?: (value: Uint8Array) => string;
  encodeText?: (value: string) => Uint8Array;
  decodeText?: (value: Uint8Array) => string;
  secureRandom?: (length: number) => Uint8Array;
  sha256?: (value: Uint8Array) => Promise<Uint8Array> | Uint8Array;
  createAttachmentKeyAgreement?: () => Promise<PorticoAttachmentKeyAgreement>;
  verifyEd25519?: (input: Ed25519VerificationInput) => boolean | Promise<boolean>;
  createAbortController?: () => AbortController;
  setTimeout?: (callback: () => void, milliseconds: number) => unknown;
  clearTimeout?: (handle: unknown) => void;
  now?: () => Date;
}

export interface ResolvedHostedConnectionRuntimeAdapters {
  fetch: typeof fetch;
  decodeBase64: (value: string) => Uint8Array;
  encodeBase64: (value: Uint8Array) => string;
  encodeText: (value: string) => Uint8Array;
  decodeText: (value: Uint8Array) => string;
  secureRandom: (length: number) => Uint8Array;
  sha256: (value: Uint8Array) => Promise<Uint8Array>;
  createAttachmentKeyAgreement: () => Promise<PorticoAttachmentKeyAgreement>;
  verifyEd25519: (input: Ed25519VerificationInput) => Promise<boolean>;
  createAbortController: () => AbortController;
  setTimeout: (callback: () => void, milliseconds: number) => unknown;
  clearTimeout: (handle: unknown) => void;
  now: () => Date;
}

export type HostedRuntimeCapability = "fetch" | "base64" | "base64-encoding" | "text-encoding" | "text-decoding" | "secure-random" | "sha256" | "p256-aes-gcm" | "ed25519" | "abort-controller" | "timers";

export class HostedRuntimeCapabilityError extends Error {
  readonly capability: HostedRuntimeCapability;

  constructor(capability: HostedRuntimeCapability, integrationHint: string) {
    super(`Hosted connection runtime capability '${capability}' is unavailable. ${integrationHint}`);
    this.name = "HostedRuntimeCapabilityError";
    this.capability = capability;
  }
}

export function createHostedConnectionRuntime(
  adapters: HostedConnectionRuntimeAdapters = {}
): ResolvedHostedConnectionRuntimeAdapters {
  return {
    fetch: adapters.fetch ?? browserFetch,
    decodeBase64: adapters.decodeBase64 ?? browserDecodeBase64,
    encodeBase64: adapters.encodeBase64 ?? browserEncodeBase64,
    encodeText: adapters.encodeText ?? browserEncodeText,
    decodeText: adapters.decodeText ?? browserDecodeText,
    secureRandom: adapters.secureRandom ?? browserSecureRandom,
    sha256: async (value) => Uint8Array.from(await (adapters.sha256 ?? browserSHA256)(value)),
    createAttachmentKeyAgreement: adapters.createAttachmentKeyAgreement ?? browserCreateAttachmentKeyAgreement,
    verifyEd25519: async (input) => {
      const verifier = adapters.verifyEd25519 ?? browserVerifyEd25519;
      return await verifier(input);
    },
    createAbortController: adapters.createAbortController ?? browserCreateAbortController,
    setTimeout: adapters.setTimeout ?? browserSetTimeout,
    clearTimeout: adapters.clearTimeout ?? browserClearTimeout,
    now: adapters.now ?? (() => new Date())
  };
}

async function browserFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  if (typeof globalThis.fetch !== "function") {
    throw new HostedRuntimeCapabilityError("fetch", "Provide runtime.fetch from the platform networking adapter.");
  }
  return globalThis.fetch(input, init);
}

function browserDecodeBase64(value: string): Uint8Array {
  if (typeof globalThis.atob !== "function") {
    throw new HostedRuntimeCapabilityError("base64", "Provide runtime.decodeBase64; React Native apps should use a maintained binary/base64 library.");
  }
  const decoded = globalThis.atob(value);
  return Uint8Array.from(decoded, (character) => character.charCodeAt(0));
}

function browserEncodeBase64(value: Uint8Array): string {
  if (typeof globalThis.btoa !== "function") {
    throw new HostedRuntimeCapabilityError("base64-encoding", "Provide runtime.encodeBase64; React Native apps should use a maintained binary/base64 library.");
  }
  let binary = "";
  for (const byte of value) binary += String.fromCharCode(byte);
  return globalThis.btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

function browserEncodeText(value: string): Uint8Array {
  if (typeof globalThis.TextEncoder !== "function") {
    throw new HostedRuntimeCapabilityError("text-encoding", "Provide runtime.encodeText with a UTF-8 encoder.");
  }
  return new globalThis.TextEncoder().encode(value);
}

function browserDecodeText(value: Uint8Array): string {
  if (typeof globalThis.TextDecoder !== "function") {
    throw new HostedRuntimeCapabilityError("text-decoding", "Provide runtime.decodeText with a strict UTF-8 decoder.");
  }
  return new globalThis.TextDecoder("utf-8", {fatal: true}).decode(Uint8Array.from(value));
}

function browserSecureRandom(length: number): Uint8Array {
  if (!Number.isInteger(length) || length < 1 || length > 1024 || !globalThis.crypto?.getRandomValues) {
    throw new HostedRuntimeCapabilityError("secure-random", "Provide runtime.secureRandom using the platform cryptographic random source.");
  }
  return globalThis.crypto.getRandomValues(new Uint8Array(length));
}

async function browserSHA256(value: Uint8Array): Promise<Uint8Array> {
  if (!globalThis.crypto?.subtle) {
    throw new HostedRuntimeCapabilityError("sha256", "Provide runtime.sha256 using the platform cryptographic digest implementation.");
  }
  return new Uint8Array(await globalThis.crypto.subtle.digest("SHA-256", Uint8Array.from(value)));
}

async function browserCreateAttachmentKeyAgreement(): Promise<PorticoAttachmentKeyAgreement> {
  if (!globalThis.crypto?.subtle) {
    throw new HostedRuntimeCapabilityError("p256-aes-gcm", "Provide runtime.createAttachmentKeyAgreement using reviewed native P-256, HKDF-SHA256, and AES-256-GCM primitives.");
  }
  const subtle = globalThis.crypto.subtle;
  const pair = await subtle.generateKey({name: "ECDH", namedCurve: "P-256"}, false, ["deriveBits"]);
  const publicKey = new Uint8Array(await subtle.exportKey("raw", pair.publicKey));
  const crypt = async (input: PorticoAttachmentCryptInput, decrypt: boolean): Promise<Uint8Array> => {
    const peer = await subtle.importKey("raw", Uint8Array.from(input.peerPublicKey), {name: "ECDH", namedCurve: "P-256"}, false, []);
    const shared = new Uint8Array(await subtle.deriveBits({name: "ECDH", public: peer}, pair.privateKey, 256));
    const salt = await subtle.digest("SHA-256", Uint8Array.from(input.transcript));
    const material = await subtle.importKey("raw", shared, "HKDF", false, ["deriveKey"]);
    const key = await subtle.deriveKey({
      name: "HKDF", hash: "SHA-256", salt,
      info: Uint8Array.from(browserEncodeText("portico-attachment-aead-v1")).buffer
    }, material, {name: "AES-GCM", length: 256}, false, [decrypt ? "decrypt" : "encrypt"]);
    const algorithm = {name: "AES-GCM", iv: Uint8Array.from(input.nonce), additionalData: Uint8Array.from(input.additionalData), tagLength: 128};
    const output = decrypt
      ? await subtle.decrypt(algorithm, key, Uint8Array.from(input.payload))
      : await subtle.encrypt(algorithm, key, Uint8Array.from(input.payload));
    return new Uint8Array(output);
  };
  return {
    publicKey,
    seal: (input) => crypt(input, false),
    open: (input) => crypt(input, true)
  };
}

async function browserVerifyEd25519(input: Ed25519VerificationInput): Promise<boolean> {
  if (!globalThis.crypto?.subtle) {
    throw new HostedRuntimeCapabilityError("ed25519", "Provide runtime.verifyEd25519 using a reviewed native or JavaScript Ed25519 implementation.");
  }
  const publicKey = Uint8Array.from(input.publicKey);
  const signature = Uint8Array.from(input.signature);
  const message = Uint8Array.from(input.message);
  const key = await globalThis.crypto.subtle.importKey("raw", publicKey, { name: "Ed25519" }, false, ["verify"]);
  return globalThis.crypto.subtle.verify({ name: "Ed25519" }, key, signature, message);
}

function browserCreateAbortController(): AbortController {
  if (typeof globalThis.AbortController !== "function") {
    throw new HostedRuntimeCapabilityError("abort-controller", "Provide runtime.createAbortController from the platform networking adapter.");
  }
  return new globalThis.AbortController();
}

function browserSetTimeout(callback: () => void, milliseconds: number): unknown {
  if (typeof globalThis.setTimeout !== "function") {
    throw new HostedRuntimeCapabilityError("timers", "Provide runtime.setTimeout and runtime.clearTimeout.");
  }
  return globalThis.setTimeout(callback, milliseconds);
}

function browserClearTimeout(handle: unknown): void {
  if (typeof globalThis.clearTimeout !== "function") {
    throw new HostedRuntimeCapabilityError("timers", "Provide runtime.setTimeout and runtime.clearTimeout.");
  }
  globalThis.clearTimeout(handle as ReturnType<typeof globalThis.setTimeout>);
}
