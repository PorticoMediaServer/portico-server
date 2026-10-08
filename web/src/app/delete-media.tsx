import React, {Suspense, createContext, lazy, useCallback, useContext, useMemo, useState} from 'react';

/** One delete dialog for the whole app, living in the shell so it outlives the menu or
 * selection bar that opened it. The dialog (and the owner-only administration
 * parsers it needs) loads on first use (PERF-25), not at startup. */
export type DeleteRequest = {itemIds: readonly string[]; titles: readonly string[]; onDeleted?: () => void};
type Value = {open: (request: DeleteRequest) => void};
/** BE-API-10: one idempotency key per logical delete. A retry of the same
 * submission — same items, server revision, file handling and confirmation —
 * reuses the key so the server replays its receipt instead of deleting twice.
 * Changing any input starts a new operation. */
export type DeleteOperationScope = {itemIds: readonly string[]; revision: number; deleteFiles: boolean; confirmation: string};
export function deletePayloadKey(scope: DeleteOperationScope): string {
  return JSON.stringify({ids: [...scope.itemIds], revision: scope.revision, deleteFiles: scope.deleteFiles, confirmation: scope.confirmation});
}
/** BE-API-10, same contract as `screens/server/operation-ids.ts` but kept here so
 * this shell-level dialog does not take a static dependency on a console screen. */
export function createDeleteOperationIds(make: () => string = () => crypto.randomUUID()): {
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
const Ctx = createContext<Value | null>(null);
export const useDeleteMedia = () => useContext(Ctx);

const DeleteMediaDialog = lazy(() => import('./delete-media-dialog').then(m => ({default: m.DeleteMediaDialog})));

export function DeleteMediaProvider({children}: {children: React.ReactNode}) {
  const [request, setRequest] = useState<DeleteRequest | null>(null);
  const open = useCallback((r: DeleteRequest) => setRequest(r.itemIds.length ? r : null), []);
  const value = useMemo(() => ({open}), [open]);
  return <Ctx.Provider value={value}>{children}{request ? <Suspense fallback={null}><DeleteMediaDialog request={request} onClose={() => setRequest(null)} /></Suspense> : null}</Ctx.Provider>;
}
