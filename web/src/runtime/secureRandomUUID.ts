const UUID_BYTE_LENGTH = 16;

export function secureRandomUUID(): string {
  const cryptoAPI = globalThis.crypto;
  if (!cryptoAPI || typeof cryptoAPI.getRandomValues !== "function") {
    throw new Error("Secure random values are unavailable in this browser.");
  }
  if (typeof cryptoAPI.randomUUID === "function") {
    return cryptoAPI.randomUUID();
  }

  const bytes = cryptoAPI.getRandomValues(new Uint8Array(UUID_BYTE_LENGTH));
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0"));
  return `${hex.slice(0, 4).join("")}-${hex.slice(4, 6).join("")}-${hex.slice(6, 8).join("")}-${hex.slice(8, 10).join("")}-${hex.slice(10).join("")}`;
}
