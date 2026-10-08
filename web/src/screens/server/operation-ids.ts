/** BE-API-10: one operation ID per logical write.
 *
 * A logical submission (one dialog Save, one toggle, one broadcast) gets a
 * single ID. An automatic or user retry of the same submission — after a
 * network error or an ambiguous response — reuses it, so the server replays
 * its receipt instead of applying the write twice. A new ID is minted only
 * when the input changes. This mirrors ConsoleClient, which already retains
 * mutation payloads for recovery.
 */
/** 24 random bytes as 48 lowercase hex characters. Every write route accepts this shape: the Live
 * TV routes require exactly it, and the others take any 8–128 letters, digits, `-`, `_` or `.`
 * (a UUID fails the Live TV check, so no source could be saved). */
export const operationId = () => Array.from(crypto.getRandomValues(new Uint8Array(24)), v => v.toString(16).padStart(2, '0')).join('');

export function createOperationIds(make: () => string = operationId): {
  forPayload: (key: string) => string;
  release: () => void;
} {
  let current: {key: string; id: string} | null = null;
  return {
    forPayload: (key: string): string => {
      if (current && current.key === key) return current.id;
      const id = make();
      current = {key, id};
      return id;
    },
    release: (): void => {
      current = null;
    },
  };
}
