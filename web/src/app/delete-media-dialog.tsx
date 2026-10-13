import {useEffect, useState} from 'react';
import {parseDeletePreview, parseDeleteResult, parseEnvelope, deletionAllowed, type DeletePreview} from '@core/administration.ts';
import {useSession} from './session';
import {useLibrariesContext} from './libraries';
import {useI18n} from './i18n';
import {errorText} from './errors';
import {isStaleAnswer} from '@core/freshness-actions.ts';
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
/** Titles named with why they were kept, before "and N more". */
const KEPT_SHOWN = 5;
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
  const [done, setDone] = useState<{summary: string; kept: readonly {title: string; why: string}[]}>();
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
      const send = (revision: number) => api.request<unknown>('/v1/admin/media/delete', 'POST', {itemIds: request.itemIds, deleteFiles, confirmation: typed.trim(), expectedRevision: revision, operationId: opIds.forPayload(deletePayloadKey({itemIds: request.itemIds, revision, deleteFiles, confirmation: typed.trim()}))});
      let raw: unknown;
      try { raw = await send(preview.revision); }
      catch (e) {
        if (!isStaleAnswer(e)) throw e;
        // Something in the library moved while the name was being typed. What will be deleted is read again: if it is exactly what was confirmed, it is deleted as asked; if not, the new answer is shown and nothing is deleted.
        const fresh = parseEnvelope(await api.request<unknown>('/v1/admin/media/delete/preview', 'POST', {itemIds: request.itemIds}), serverId, parseDeletePreview).result;
        if (fresh.confirmation !== preview.confirmation || fresh.totalFiles !== preview.totalFiles || fresh.totalBytesText !== preview.totalBytesText || !deletionAllowed(fresh)) { setPreview(fresh); setTyped(''); throw e; }
        raw = await send(fresh.revision);
      }
      const result = parseEnvelope(raw, serverId, parseDeleteResult).result;
      opIds.release();
      // A title that stayed says why: the file on disk is not the catalogued one, or storage refused.
      const kept = result.receipts.filter(r => !r.removed && (r.code === 'file_changed' || r.code === 'file_not_removed')).map(r => ({title: r.title, why: i18n.t(r.code === 'file_changed' ? 'deleteMedia.fileChanged' : 'deleteMedia.fileNotRemoved')}));
      setDone({summary: result.failed ? i18n.t('deleteMedia.partial', {removed: result.removed, failed: result.failed}) : i18n.t('deleteMedia.removed'), kept});
      request.onDeleted?.();
    } catch (e) { setError(say(e, 'action')); } finally { setBusy(false); }
  };
  const title = request.titles.length === 1 ? i18n.t('deleteMedia.titleOne', {title: request.titles[0]!}) : i18n.t('deleteMedia.titleMany', {count: request.itemIds.length});
  if (done) return (
    <Dialog open onOpenChange={o => !o && onClose()} title={i18n.t('action.done')} width={440} actions={<Button variant="primary" label={i18n.t('action.close')} onClick={onClose} />}>
      <Text as="p" variant="body" tone="secondary">{done.summary}</Text>
      {done.kept.slice(0, KEPT_SHOWN).map((k, i) => <Notice key={i} tone="warning" compact title={k.title}>{k.why}</Notice>)}
      {done.kept.slice(KEPT_SHOWN).length ? <Text as="p" variant="caption" tone="tertiary">{i18n.t('deleteMedia.keptMore', {count: done.kept.slice(KEPT_SHOWN).length})}</Text> : null}
    </Dialog>
  );
  const titlesCovered = preview ? preview.targets.reduce((sum, target) => sum + target.titles, 0) : 0;
  const coversMore = !!preview && titlesCovered > preview.targets.length;
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
          {/* A show, season, album or book takes everything beneath it: say how many titles that is. */}
          {coversMore ? <Text as="p" variant="bodyStrong">{i18n.t('deleteMedia.titlesCovered', {count: titlesCovered})}</Text> : null}
          <Text as="p" variant="body" tone="secondary">{i18n.t('deleteMedia.files', {count: preview.totalFiles, size: preview.totalBytesText})}{preview.targets.some(t => t.dependents.length) ? ' ' + preview.targets.flatMap(t => t.dependents).map(d => d.description).filter((v, i, a) => a.indexOf(v) === i).join(' ') : ''}</Text>
          {preview.targets.some(t => t.files.some(f => f.shared)) ? <Notice tone="info" compact>{i18n.t('deleteMedia.sharedKept')}</Notice> : null}
          {allowed ? (
            <>
              <Switch checked={deleteFiles} onCheckedChange={setDeleteFiles} label={i18n.t('deleteMedia.removeFiles')} />
              <Text variant="caption" tone="tertiary">{deleteFiles ? i18n.t('deleteMedia.removedNow') : i18n.t('deleteMedia.recordOnly')}</Text>
              <Input label={preview.confirmationKind === 'count' ? i18n.t('deleteMedia.typeCount', {count: preview.confirmation}) : i18n.t('deleteMedia.typeTitle')} help={preview.confirmationKind === 'title' ? preview.confirmation : undefined} value={typed} onChange={e => setTyped(e.target.value)} autoFocus autoComplete="off" />
            </>
          ) : <Notice tone="info" compact>{i18n.t('deleteMedia.disabled')}</Notice>}
        </div>
      ) : null}
    </Dialog>
  );
}
