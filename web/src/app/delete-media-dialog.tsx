import {useEffect, useState} from 'react';
import {parseDeletePreview, parseDeleteResult, parseEnvelope, deletionAllowed, type DeletePreview} from '@core/administration.ts';
import {useSession} from './session';
import {useLibrariesContext} from './libraries';
import {useI18n} from './i18n';
import {errorText} from './errors';
import {Button, Dialog, Input, Loading, Notice, Switch, Text} from '../ui';
import type {DeleteRequest} from './delete-media';
import {createDeleteOperationIds, deletePayloadKey} from './delete-media';

/**
 * One delete dialog for the whole app. The server decides what may be deleted
 * and what must be typed to confirm; this only shows its answer. Split from
 * `delete-media.tsx` so the owner-only administration parsers load on first
 * use (PERF-25), not at startup.
 */
/** CD-44: destructive selections are never chunked; over-limit is blocked. */
export const DELETE_LIMIT = 200;
export function deleteOverLimit(itemIds: readonly string[]): boolean {
  return new Set(itemIds).size > DELETE_LIMIT;
}
export function DeleteMediaDialog({request, onClose}: {request: DeleteRequest; onClose: () => void}) {
  const {api, system, session} = useSession();
  const serverId = system?.id ?? session?.viewer.serverId ?? '';
  const [preview, setPreview] = useState<DeletePreview>();
  const [error, setError] = useState('');
  const [typed, setTyped] = useState('');
  const [deleteFiles, setDeleteFiles] = useState(true);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState('');
  // BE-API-10: one idempotency key per logical delete; retries reuse it, edits mint a new one.
  const [opIds] = useState(createDeleteOperationIds);
  const i18n = useI18n();
  const libraries = useLibrariesContext();
  // CD-44: never submit an over-limit delete; do not chunk a confirmed scope.
  const overLimit = deleteOverLimit(request.itemIds);
  // X-04: errors are presented by code, never from error.message.
  const say = (e: unknown, operation: 'load' | 'action') => errorText(e, 'server-console', operation);
  useEffect(() => {
    if (overLimit) return;
    let active = true;
    api.request<unknown>('/v1/admin/media/delete/preview', 'POST', {itemIds: request.itemIds})
      .then(raw => { if (active) setPreview(parseEnvelope(raw, serverId, parseDeletePreview).result); }, e => { if (active) setError(say(e, 'load')); });
    return () => { active = false; };
  }, [api, request.itemIds, serverId, overLimit]);
  const allowed = preview ? deletionAllowed(preview) : false;
  const confirm = async () => {
    if (!preview || overLimit) return;
    setBusy(true); setError('');
    try {
      const key = deletePayloadKey({itemIds: request.itemIds, revision: preview.revision, deleteFiles, confirmation: typed.trim()});
      const result = parseEnvelope(await api.request<unknown>('/v1/admin/media/delete', 'POST', {itemIds: request.itemIds, deleteFiles, confirmation: typed.trim(), expectedRevision: preview.revision, operationId: opIds.forPayload(key)}), serverId, parseDeleteResult).result;
      opIds.release();
      setDone(result.failed ? i18n.t('deleteMedia.partial', {removed: result.removed, failed: result.failed}) : deleteFiles && preview.trashRetentionDays > 0 ? i18n.t('deleteMedia.removedKept', {days: preview.trashRetentionDays}) : i18n.t('deleteMedia.removed'));
      request.onDeleted?.();
    } catch (e) { setError(say(e, 'action')); } finally { setBusy(false); }
  };
  const title = request.titles.length === 1 ? i18n.t('deleteMedia.titleOne', {title: request.titles[0]!}) : i18n.t('deleteMedia.titleMany', {count: request.itemIds.length});
  if (done) return <Dialog open onOpenChange={o => !o && onClose()} title={i18n.t('action.done')} width={440} actions={<Button variant="primary" label={i18n.t('action.close')} onClick={onClose} />}><Text as="p" variant="body" tone="secondary">{done}</Text></Dialog>;
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={title} width={520}
      actions={<><Button variant="ghost" label={i18n.t('action.cancel')} onClick={onClose} /><Button variant="danger" label={i18n.t('action.delete')} disabled={overLimit || !preview || !allowed || typed.trim() !== preview.confirmation} loading={busy} onClick={() => void confirm()} /></>}>
      {overLimit ? <Notice tone="error" compact>{i18n.t('deleteMedia.tooMany', {max: 200})}</Notice> : null}
      {!preview && !error && !overLimit ? <Loading label={i18n.t('deleteMedia.checking')} /> : null}
      {error ? <Notice tone="error" compact>{error}</Notice> : null}
      {preview ? (
        <div style={{display: 'flex', flexDirection: 'column', gap: 16}}>
          {/* WEB-MENU-02: `blocked` lists library ids; name the library, never show the id. */}
          {preview.blocked.map(b => <Notice key={b} tone="warning" compact>{i18n.t('deleteMedia.blockedLibrary', {library: preview.targets.find(t => t.libraryId === b)?.libraryName || libraries.items.find(l => l.id === b)?.name || '—'})}</Notice>)}
          <Text as="p" variant="body" tone="secondary">{i18n.t('deleteMedia.files', {count: preview.totalFiles, size: preview.totalBytesText})}{preview.targets.some(t => t.dependents.length) ? ' ' + preview.targets.flatMap(t => t.dependents).map(d => d.description).filter((v, i, a) => a.indexOf(v) === i).join(' ') : ''}</Text>
          {preview.targets.some(t => t.files.some(f => f.shared)) ? <Notice tone="info" compact>{i18n.t('deleteMedia.sharedKept')}</Notice> : null}
          {allowed ? (
            <>
              <Switch checked={deleteFiles} onCheckedChange={setDeleteFiles} label={i18n.t('deleteMedia.removeFiles')} />
              <Text variant="caption" tone="tertiary">{deleteFiles ? (preview.trashRetentionDays > 0 ? i18n.t('deleteMedia.trashKept', {days: preview.trashRetentionDays}) : i18n.t('deleteMedia.removedNow')) : i18n.t('deleteMedia.recordOnly')}</Text>
              <Input label={preview.confirmationKind === 'count' ? i18n.t('deleteMedia.typeCount', {count: preview.confirmation}) : i18n.t('deleteMedia.typeTitle')} help={preview.confirmationKind === 'title' ? preview.confirmation : undefined} value={typed} onChange={e => setTyped(e.target.value)} autoFocus autoComplete="off" />
            </>
          ) : <Notice tone="info" compact>{i18n.t('deleteMedia.disabled')}</Notice>}
        </div>
      ) : null}
    </Dialog>
  );
}
