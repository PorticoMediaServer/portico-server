import { afterEach, describe, expect, it, vi } from "vitest";
import { secureRandomUUID } from "./secureRandomUUID";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("secureRandomUUID", () => {
  it("uses the browser UUID primitive when available", () => {
    const randomUUID = vi.fn(() => "0198f9dd-cb11-4a29-8c89-8b88af6ba152");
    vi.stubGlobal("crypto", { randomUUID, getRandomValues: vi.fn() });

    expect(secureRandomUUID()).toBe("0198f9dd-cb11-4a29-8c89-8b88af6ba152");
    expect(randomUUID).toHaveBeenCalledOnce();
  });

  it("creates an RFC 4122 version-4 UUID from secure bytes on ordinary LAN HTTP", () => {
    const getRandomValues = vi.fn((target: Uint8Array) => {
      target.set([
        0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0xff, 0x77,
        0xff, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff,
      ]);
      return target;
    });
    vi.stubGlobal("crypto", { getRandomValues });

    expect(secureRandomUUID()).toBe("00112233-4455-4f77-bf99-aabbccddeeff");
    expect(getRandomValues).toHaveBeenCalledOnce();
  });

  it("fails closed instead of falling back to predictable randomness", () => {
    vi.stubGlobal("crypto", undefined);
    expect(() => secureRandomUUID()).toThrow(
      "Secure random values are unavailable in this browser.",
    );
  });
});
