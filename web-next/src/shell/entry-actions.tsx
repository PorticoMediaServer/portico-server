import {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {parseDeletePreview, parseDeleteResult, type DeletePreview, type DeleteResult} from '@core/administration.ts';
import {routePath} from '../../../packages/contracts/src/routes.ts';
import {DeleteMediaDialog} from '../kit/selection';
import {deleteEnvelope, entryActions, type EntryActionTarget} from './entry-actions';

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
