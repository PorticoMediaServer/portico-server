import {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {parseDeletePreview, parseDeleteResult, type DeletePreview, type DeleteResult} from '@core/administration.ts';
import type {ContentEntryKind} from '@core/library-content.ts';
import {routePath} from '../../../packages/contracts/src/routes.ts';
import {DeleteMediaDialog} from '../kit/selection';

/** One title a screen can act on; facts only, never a projection or a movie-only shape. */
export type EntryActionTarget = Readonly<{libraryId: string; itemId: string; kind: ContentEntryKind; title: string}>;
type Api = Readonly<{request: <T>(path: string, method?: string, body?: unknown, signal?: AbortSignal) => Promise<T>}>;
export type EntryActionPorts = Readonly<{api: Api; serverId: string; viewerId: string; isCurrent: () => boolean; requestId: () => Promise<string>}>;

type OpenDelete = Readonly<{kind: 'delete'; target: EntryActionTarget; anchor: HTMLElement | null; ports: EntryActionPorts}>;
type Open = OpenDelete;

/** One shell-owned surface at a time; the effects open it with the captured authority and the
 * host owns the preview, the typed confirmation and the actual removal. */
export class EntryActionStore {
  private state: Open | null = null;
  private listeners = new Set<() => void>();
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => {this.listeners.delete(listener);}; };
  getSnapshot = (): Open | null => this.state;
  openDelete(target: EntryActionTarget, anchor: HTMLElement | null, ports: EntryActionPorts) { this.state = {kind: 'delete', target, anchor, ports}; this.emit(); }
  close() { if (!this.state) return; this.state = null; this.emit(); }
  private emit() { for (const listener of [...this.listeners]) listener(); }
}
export const entryActions = new EntryActionStore();

/** A screen's optional effect. It opens the shared dialog and resolves once it is open. */
export function deleteTitleEffect(ports: EntryActionPorts) {
  return async (target: EntryActionTarget, anchor: HTMLElement): Promise<void> => {
    if (!ports.isCurrent()) throw new DOMException('Screen authority changed', 'AbortError');
    entryActions.openDelete(target, anchor, ports);
  };
}

function deleteEnvelope(raw: unknown, serverId: string): unknown {
  const value = raw as {protocolVersion?: unknown; serverId?: unknown; result?: unknown};
  if (value?.protocolVersion !== '1.0' || value.serverId !== serverId) throw new Error('The delete answer did not match this server.');
  return value.result;
}

/** Mounted once by the shell. It renders nothing until a screen effect opens a surface. */
export function EntryActionHost() {
  const open = useSyncExternalStore(entryActions.subscribe, entryActions.getSnapshot, () => null);
  const anchor = useRef<HTMLElement | null>(null), alive = useRef(true);
  const [preview, setPreview] = useState<DeletePreview | null>(null), [confirmation, setConfirmation] = useState('');
  const [removeFiles, setRemoveFiles] = useState(true), [result, setResult] = useState<DeleteResult | undefined>(undefined);
  const [busy, setBusy] = useState(false), [error, setError] = useState<string | undefined>(undefined);
  useEffect(() => {
    alive.current = true;
    if (!open) return () => {alive.current = false;};
    const request = new AbortController();
    anchor.current = open.anchor;
    setPreview(null); setConfirmation(''); setRemoveFiles(true); setResult(undefined); setError(undefined); setBusy(true);
    void open.ports.api.request(routePath('post_admin_media_delete_preview'), 'POST', {itemIds: [open.target.itemId]}, request.signal)
      .then(raw => {
        if (!alive.current || request.signal.aborted || !open.ports.isCurrent()) return;
        const value = parseDeletePreview(deleteEnvelope(raw, open.ports.serverId));
        const target = value.targets[0];
        if (value.targets.length !== 1 || !target || target.itemId !== open.target.itemId || target.libraryId !== open.target.libraryId) throw new Error('The delete preview named a different title.');
        setPreview(value);
      })
      .catch(error => { if (alive.current && !request.signal.aborted) setError(String((error as Error)?.message ?? error)); })
      .finally(() => { if (alive.current && !request.signal.aborted) setBusy(false); });
    return () => {alive.current = false; request.abort();};
  }, [open]);
  if (!open) return null;
  const close = () => { if (!busy) entryActions.close(); };
  const remove = async (input: {preview: DeletePreview; confirmation: string; deleteFiles: boolean}) => {
    setBusy(true); setError(undefined);
    try {
      const raw = await open.ports.api.request(routePath('post_admin_media_delete'), 'POST', {
        itemIds: [open.target.itemId], expectedRevision: input.preview.revision, confirmation: input.confirmation,
        deleteFiles: input.deleteFiles, operationId: await open.ports.requestId(),
      });
      if (!open.ports.isCurrent()) throw new DOMException('Screen authority changed', 'AbortError');
      setResult(parseDeleteResult(deleteEnvelope(raw, open.ports.serverId)));
    } catch (error) { setError(String((error as Error)?.message ?? error)); }
    finally { setBusy(false); }
  };
  return <DeleteMediaDialog owner open onOpenChange={value => {if (!value) close();}} returnFocusRef={anchor} selectionCount={1}
    preview={preview} confirmation={confirmation} onConfirmationChange={setConfirmation} removeFiles={removeFiles}
    onRemoveFilesChange={setRemoveFiles} onDelete={remove} busy={busy} error={error} result={result}
    ownerTag={<span>{defaultI18n.t('next.selection.owner')}</span>} />;
}
