import {useEffect, useMemo, useState} from 'react';
import type {TitleFile} from '@core/detail.ts';
import {deletionAllowed, versionDeletionService, type VersionDeletePreview} from '@core/administration.ts';
import {errorText} from '../../app/errors';
import {serverChanged} from '../../app/server-changes';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Button, Dialog, Input, Loading, Notice, Text} from '../../ui';

/**
 * Delete one version of a title (owners). The server's preview names the one file that goes and
 * what must be typed; the title and its other versions stay, with everyone's history. The file
 * is deleted from disk at once and cannot be recovered. When it is the last version, the title
 * is left marked missing, as if the file had been removed outside Portico.
 */
export function DeleteVersionDialog({itemId, title, file, summary, onClose, onDeleted}: {itemId: string; title: string; file: TitleFile; summary: string; onClose: () => void; onDeleted: () => void}) {
  const {t} = useI18n();
  const {api} = useSession();
  const service = useMemo(() => versionDeletionService(<T,>(path: string, method?: string, body?: unknown) => api.request<T>(path, method, body)), [api]);
  const [preview, setPreview] = useState<VersionDeletePreview>();
  const [typed, setTyped] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  // One operation for one confirmed preview: a retry of the same delete is answered with its receipt.
  const [operationId] = useState(() => crypto.randomUUID());
  const read = () => { setError(''); setPreview(undefined); service.preview(itemId, file.id).then(setPreview, e => setError(errorText(e, 'server-console', 'load'))); };
  useEffect(read, [service, itemId, file.id]); // eslint-disable-line react-hooks/exhaustive-deps
  const allowed = preview ? deletionAllowed(preview) : false;
  const confirm = () => {
    if (!preview) return;
    setBusy(true); setError('');
    service.remove(preview, typed.trim(), operationId).then(result => {
      const kept = result.receipts.find(r => !r.removed);
      if (kept) { setBusy(false); setError(t(kept.code === 'file_changed' ? 'deleteMedia.fileChanged' : 'deleteMedia.fileNotRemoved')); return; }
      serverChanged();
      onDeleted();
    }, e => { setBusy(false); setError(errorText(e, 'server-console', 'action')); });
  };
  return (
    <Dialog open onOpenChange={o => !o && !busy && onClose()} dismissable={!busy} title={t('title.deleteVersion.title')} description={`${title} · ${summary}`} width={500}
      actions={<><Button variant="ghost" label={t('action.cancel')} disabled={busy} onClick={onClose} /><Button variant="danger" label={t('action.delete')} disabled={!preview || !allowed || typed.trim() !== preview.confirmation} loading={busy} onClick={confirm} /></>}>
      {error ? <Notice tone="error" compact action={preview ? undefined : {label: t('action.tryAgain'), onClick: read}}>{error}</Notice> : null}
      {!preview && !error ? <Loading label={t('deleteMedia.checking')} /> : null}
      {preview ? (
        <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
          <Text as="p" variant="body" tone="secondary">{t('title.deleteVersion.body', {size: preview.totalBytesText})}</Text>
          {file.path ? <Text as="p" variant="mono" tone="tertiary" style={{fontSize: 12, overflowWrap: 'anywhere'}}>{file.path}</Text> : null}
          {allowed
            ? <Input label={preview.confirmationKind === 'count' ? t('deleteMedia.typeCount', {count: preview.confirmation}) : t('deleteMedia.typeTitle')} help={preview.confirmationKind === 'title' ? preview.confirmation : undefined} value={typed} onChange={e => setTyped(e.target.value)} autoFocus autoComplete="off" />
            : <Notice tone="info" compact>{t('deleteMedia.disabled')}</Notice>}
        </div>
      ) : null}
    </Dialog>
  );
}
