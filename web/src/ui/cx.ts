/** Joins class names, skipping falsy values. */
export function cx(...parts: Array<string | false | null | undefined | 0 | 0n>): string {
  return parts.filter(Boolean).join(' ');
}
