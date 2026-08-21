export const PORTICO_PASSWORD_POLICY = Object.freeze({
  minimumCharacters: 8,
  maximumUtf8Bytes: 72,
  required: Object.freeze({ uppercase: true, lowercase: true, numberOrSpecial: true })
});

export type PorticoPasswordStrength = "weak" | "medium" | "strong";

export type PorticoPasswordEvaluation = {
  valid: boolean;
  strength: PorticoPasswordStrength;
  characterCount: number;
  utf8Bytes: number;
  checks: {
    minimumCharacters: boolean;
    maximumUtf8Bytes: boolean;
    uppercase: boolean;
    lowercase: boolean;
    numberOrSpecial: boolean;
  };
};

/**
 * Evaluates the portable Portico password policy without network access or a
 * platform-specific dependency. Strength is intentionally informational: only
 * `valid` represents the server-enforced submission boundary.
 */
export function evaluatePorticoPassword(password: string): PorticoPasswordEvaluation {
  const value = String(password ?? "");
  const characterCount = Array.from(value).length;
  const utf8Bytes = utf8ByteLength(value);
  const uppercase = /\p{Lu}/u.test(value);
  const lowercase = /\p{Ll}/u.test(value);
  const digit = /\p{Nd}/u.test(value);
  const special = /[^\p{L}\p{N}]/u.test(value);
  const checks = {
    minimumCharacters: characterCount >= PORTICO_PASSWORD_POLICY.minimumCharacters,
    maximumUtf8Bytes: utf8Bytes <= PORTICO_PASSWORD_POLICY.maximumUtf8Bytes,
    uppercase,
    lowercase,
    numberOrSpecial: digit || special
  };
  const valid = Object.values(checks).every(Boolean);

  // Reward length and variety without pretending to be an entropy estimator.
  let score = 0;
  if (characterCount >= 12) score += 1;
  if (characterCount >= 16) score += 1;
  if (characterCount >= 24) score += 1;
  if (digit && special) score += 1;
  if (/\p{L}/u.test(value) && /[^\x00-\x7F]/u.test(value)) score += 1;
  const strength: PorticoPasswordStrength = valid && score >= 4 ? "strong" : valid && score >= 1 ? "medium" : "weak";

  return { valid, strength, characterCount, utf8Bytes, checks };
}

/** UTF-8 length without relying on DOM or a host-provided TextEncoder. */
export function utf8ByteLength(value: string): number {
  let bytes = 0;
  for (const character of String(value ?? "")) {
    const codePoint = character.codePointAt(0) ?? 0;
    bytes += codePoint <= 0x7f ? 1 : codePoint <= 0x7ff ? 2 : codePoint <= 0xffff ? 3 : 4;
  }
  return bytes;
}
