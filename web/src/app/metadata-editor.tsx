import React, {createContext, useCallback, useContext, useMemo, useState} from 'react';
import type {ContentEntry} from '@core/library-content.ts';
import type {RepairTarget} from '@core/metadata-repair.ts';

/**
 * One editor for the whole app. Any surface (card actions, detail page,
 * selection bar) opens it with one or more repair targets; the dialog lives
 * in the shell so it outlives the screen that opened it.
 */
export type EditorRequest = {targets: readonly RepairTarget[]; titles: readonly string[]; onSaved?: () => void};
type Value = {request: EditorRequest | null; open: (request: EditorRequest) => void; close: () => void};
const Ctx = createContext<Value | null>(null);

export function MetadataEditorProvider({children}: {children: React.ReactNode}) {
  const [request, setRequest] = useState<EditorRequest | null>(null);
  const open = useCallback((r: EditorRequest) => setRequest(r.targets.length ? r : null), []);
  const close = useCallback(() => setRequest(null), []);
  const value = useMemo(() => ({request, open, close}), [request, open, close]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
export const useMetadataEditor = () => useContext(Ctx);

/** Which repair target an entry maps to; null for entries that carry no editable metadata (people, categories). */
export function repairTargetFor(entry: Pick<ContentEntry, 'id' | 'kind' | 'libraryId' | 'navigation'>): RepairTarget | null {
  if (!entry.libraryId) return null;
  const id = entry.navigation?.entityId ?? entry.id;
  switch (entry.kind) {
    case 'movie': case 'episode': case 'song': case 'audiobook_file': return {kind: 'item', id: entry.id, libraryId: entry.libraryId};
    case 'show': return {kind: 'show', id, libraryId: entry.libraryId};
    case 'season': return {kind: 'season', id, libraryId: entry.libraryId};
    case 'album': return {kind: 'album', id, libraryId: entry.libraryId};
    case 'artist': return {kind: 'artist', id, libraryId: entry.libraryId};
    case 'book': return {kind: 'book', id, libraryId: entry.libraryId};
    default: return null;
  }
}
