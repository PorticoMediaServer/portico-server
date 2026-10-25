import {useEffect, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import type {LibraryContentApi} from '@core/library-content.ts';
import type {LibraryManagementService} from '@core/library-management.ts';
import {Button} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays/Dialog';
import {skippedFiles, type SkippedPage} from './skipped-files';
import styles from './libraries.module.css';
const t = defaultI18n.t;

export function SkippedFiles({libraryId, api, service, isCurrent, label}: Readonly<{
  libraryId: string; api: LibraryContentApi; service: LibraryManagementService; isCurrent: () => boolean; label: string;
}>) {
  const [open, setOpen] = useState(false), [page, setPage] = useState<SkippedPage>(), [failed, setFailed] = useState(false), [busy, setBusy] = useState(false);
  const lifetime = useRef<AbortController | undefined>(undefined), job = useRef<string | undefined>(undefined), running = useRef(false);
  const current = (capture: AbortController) => lifetime.current === capture && !capture.signal.aborted && isCurrent();
  const load = async (capture: AbortController, after = 0) => {
    if (!current(capture) || running.current || !job.current) return;
    running.current = true; setBusy(true); setFailed(false);
    try {
      const next = await skippedFiles(api, libraryId, job.current, after, capture.signal);
      if (current(capture)) setPage(before => after && before ? {...next, items: [...before.items, ...next.items]} : next);
    } catch { if (current(capture)) setFailed(true); }
    finally { if (current(capture)) {running.current = false; setBusy(false);} }
  };
  useEffect(() => {
    if (!open) return;
    const capture = new AbortController(); lifetime.current = capture; running.current = false; job.current = undefined;
    setPage(undefined); setFailed(false); setBusy(true);
    void service.selectLibrary(libraryId).then(async () => {
      if (!current(capture)) return;
      const selected = service.getSnapshot().selected.data?.library;
      if (!selected || selected.id !== libraryId) throw new Error('library_changed');
      job.current = selected.lastScan?.id;
      if (job.current) await load(capture);
      else {setPage({items: [], count: 0, hasMore: false, next: 0}); setBusy(false);}
    }).catch(() => {if (current(capture)) {setFailed(true);setBusy(false);}});
    return () => {capture.abort(); if (lifetime.current === capture) lifetime.current = undefined;};
  }, [open, libraryId, api, service, isCurrent]);
  const scan = async () => {
    const capture = lifetime.current;
    if (!capture || !current(capture) || running.current) return;
    running.current = true;setBusy(true);setFailed(false);
    try {await service.selectLibrary(libraryId);if (current(capture)) await service.scan();}
    catch {if (current(capture)) setFailed(true);}
    finally {if (current(capture)) {running.current = false;setBusy(false);}}
  };
  return <>
    <Button variant="glass" size="small" onClick={() => {if (isCurrent()) setOpen(true);}}>{label}</Button>
    <Dialog open={open} onOpenChange={setOpen} size="lg" title={t('next.ownerLibraries.filesSkippedTitle')}
      description={t('next.ownerLibraries.filesSkippedLede')} footer={<>
        {page && <span>{t('next.ownerLibraries.filesCount', {count: page.count})}</span>}
        <Button onClick={() => setOpen(false)}>{t('next.overlays.close')}</Button>
        <Button variant="primary" icon="i-refresh" loading={busy} disabled={busy} onClick={() => {void scan();}}>{t('next.ownerLibraries.scanAgain')}</Button>
      </>}>
      {failed ? <p role="alert">{t('next.states.pageFailedBody')}</p> : null}
      {!page && busy ? <p role="status">{t('next.settings.loading')}</p> : null}
      {page && !page.items.length ? <p className={styles.empty}>{t('next.ownerLibraries.skippedEmpty')}</p> : null}
      <ul className={styles.skippedList}>{page?.items.map(file => <li key={file.sequence}><code>{file.path}</code><small>{t(file.reason)}</small></li>)}</ul>
      {page?.hasMore && <Button disabled={busy} onClick={() => {const capture = lifetime.current;if (capture) void load(capture, page.next);}}>{t('next.ownerLibraries.showMore')}</Button>}
    </Dialog>
  </>;
}
