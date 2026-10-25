import {useCallback, useEffect, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import type {LibraryContentApi} from '@core/library-content.ts';
import {browseServerFilesystem} from '@core/library-management.ts';
import {Button, TextField} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays/Dialog';
import styles from './libraries.module.css';

const t = defaultI18n.t;

/** The server's own folder explorer, folders only (owner-libraries.html#9). A
 * typed path is always available; Places are the roots the server publishes. */
export function FolderPicker({open, onOpenChange, api, onChoose}: Readonly<{
  open: boolean; onOpenChange: (open: boolean) => void; api: LibraryContentApi; onChoose: (path: string) => void;
}>) {
  const [page, setPage] = useState<Awaited<ReturnType<typeof browseServerFilesystem>>>();
  const [typed, setTyped] = useState(''), [failed, setFailed] = useState(false), [busy, setBusy] = useState(false);
  const request = useRef<AbortController | undefined>(undefined);
  const read = useCallback(async (path?: string) => {
    request.current?.abort();
    const active = new AbortController();
    request.current = active;
    // A pending or failed destination must never leave the previous folder choosable.
    setPage(undefined); setTyped(path ?? ''); setBusy(true); setFailed(false);
    try {
      const next = await browseServerFilesystem(api, {...(path ? {path} : {}), includeFiles: false, limit: 200}, active.signal);
      if (active.signal.aborted || request.current !== active) return;
      setPage(next); setTyped(next.path);
    } catch {
      if (!active.signal.aborted && request.current === active) setFailed(true);
    } finally {
      if (!active.signal.aborted && request.current === active) setBusy(false);
    }
  }, [api]);
  useEffect(() => {
    if (open) void read(undefined);
    return () => { request.current?.abort(); request.current = undefined; };
  }, [open, read]);
  const changeOpen = (next: boolean) => {
    if (!next) { request.current?.abort(); request.current = undefined; }
    onOpenChange(next);
  };
  const go = (path: string) => { void read(path); };
  return <Dialog open={open} onOpenChange={changeOpen} size="md" title={t('next.settings.chooseFolder')} description={t('next.settings.folderBreadcrumbs')}
    footer={<><Button onClick={() => changeOpen(false)}>{t('next.settings.cancel')}</Button>
      <Button variant="primary" disabled={!page?.path || busy || failed} onClick={() => { if (page?.path) { onChoose(page.path); changeOpen(false); } }}>{t('next.settings.chooseThisFolder')}</Button></>}>
    <div className={styles.picker}>
      <div className={styles.pickerPath}>
        <TextField label={t('next.settings.folderPath')} value={typed} onChange={event => setTyped(event.currentTarget.value)} />
        <Button disabled={busy} onClick={() => go(typed)}>{t('next.settings.go')}</Button>
        <Button disabled={busy || !page?.parent} onClick={() => { if (page?.parent) go(page.parent); }}>{t('next.settings.up')}</Button>
      </div>
      {failed && <p role="alert" className={styles.pickerNote}>{t('next.settings.folderUnreadable')}</p>}
      {busy && !page && <p role="status">{t('next.settings.loading')}</p>}
      {page && !page.path && <ul className={styles.pickerList}>{page.roots.map(root => <li key={root.path}>
        <button type="button" className={styles.pickerRow} onClick={() => go(root.path)}><span>{root.name}</span><small>{root.path}</small></button></li>)}</ul>}
      {page && page.path && (page.entries.length
        ? <ul className={styles.pickerList}>{page.entries.filter(entry => entry.kind === 'directory').map(entry => <li key={entry.path}>
          <button type="button" className={styles.pickerRow} disabled={!entry.readable} onClick={() => go(entry.path)}>
            <span>{entry.name}</span>{!entry.readable && <small>{t('next.settings.folderUnreadable')}</small>}</button></li>)}</ul>
        : <p className={styles.pickerNote}>{t('next.settings.noFolders')}</p>)}
    </div>
  </Dialog>;
}
