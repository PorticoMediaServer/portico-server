/** The one honest answer for a protected restore the server refuses to open (v2 Root §1). */
export type ProtectedRestoreResult = 'unopened' | 'failed';

/**
 * The v2 envelope authenticates the header and the encrypted records together, so a
 * wrong derived key and damaged authenticated bytes fail the same check. The refusal
 * code proves only that this backup cannot be opened now; it never proves which input
 * is wrong. Every other refusal keeps its own path.
 */
export function protectedRestoreResult(error: unknown): ProtectedRestoreResult {
  return (error as {code?: unknown})?.code === 'restore_integrity_failed' ? 'unopened' : 'failed';
}
