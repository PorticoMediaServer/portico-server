import {useCallback, useEffect, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import {browseServerFilesystem} from '@core/library-management.ts';
import type {LibraryContentApi} from '@core/library-content.ts';
import type {FilesystemPage} from '@core/administration.ts';
import {Dialog} from '../overlays/Dialog';
import {Button, TextField} from '../controls';
import {Icon} from '../Icon';
import styles from './settings.module.css';

const t = defaultI18n.t;
export type ServerFolderPickerProps = Readonly<{open: boolean; onClose: () => void; api: LibraryContentApi; initialPath?: string; onChoose: (paths: readonly string[]) => void; multiple?: boolean; returnFocusRef?: React.RefObject<HTMLElement | null>}>;
/** Uses the real folders-only server explorer, including roots and bounded pages. */
export function ServerFolderPicker({open, onClose, api, initialPath = '', onChoose, multiple = false, returnFocusRef}: ServerFolderPickerProps) {
  const [page, setPage] = useState<FilesystemPage>(), [path, setPath] = useState(initialPath), [hidden, setHidden] = useState(false), [busy, setBusy] = useState(false), [failed, setFailed] = useState(false), [selected, setSelected] = useState<readonly string[]>([]);
  const request = useRef<AbortController | undefined>(undefined), initial = useRef(initialPath); initial.current = initialPath;
  const load = useCallback(async (next: string, showHidden: boolean, cursor?: string) => {
    request.current?.abort(); const controller = new AbortController(); request.current = controller;
    setBusy(true); setFailed(false);
    try {
      const result = await browseServerFilesystem(api, {path: next || undefined, showHidden, ...(cursor ? {cursor} : {})}, controller.signal);
      if (controller.signal.aborted) return;
      setPage(old => cursor && old?.path === result.path ? {...result, entries: [...old.entries, ...result.entries.filter(entry => !old.entries.some(before => before.path === entry.path))]} : result);
      setPath(result.path);
    } catch { if (!controller.signal.aborted) setFailed(true); }
    finally {if (!controller.signal.aborted) setBusy(false);}
  }, [api]);
  useEffect(() => {
    if (!open) return;
    setPage(undefined); setSelected([]); setHidden(false); void load(initial.current, false);
    return () => request.current?.abort();
  }, [open, load]);
  const choose = () => {if (!page || busy || failed) return; const paths = multiple && selected.length ? selected : [page.path]; onChoose(paths); onClose();};
  const segments = page?.path.split(page.separator).filter(Boolean) ?? [];
  return <Dialog open={open} onOpenChange={value => {if (!value) onClose();}} title={t(multiple ? 'next.settings.chooseFolders' : 'next.settings.chooseFolder')} size="lg" width={720} returnFocusRef={returnFocusRef} bodyClassName={styles.folderBody}
    footer={<><Button size="compact" onClick={onClose}>{t('next.settings.cancel')}</Button><Button size="compact" variant="primary" disabled={!page?.path || busy || failed} onClick={choose}>{t(selected.length ? 'next.settings.chooseSelected' : 'next.settings.chooseThisFolder', {count: selected.length})}</Button></>}>
    <form className={styles.folderPath} onSubmit={event => {event.preventDefault(); void load(path, hidden);}}><TextField aria-label={t('next.settings.folderPath')} value={path} onChange={event => setPath(event.currentTarget.value)} /><Button type="submit" size="xs" disabled={busy}>{t('next.settings.go')}</Button></form>
    <nav className={styles.crumbs} aria-label={t('next.settings.folderBreadcrumbs')}><Button size="xs" variant="text" onClick={() => {void load('', hidden);}}>{t('next.settings.roots')}</Button>{segments.map((segment, index) => {
      const full = (page!.path.startsWith(page!.separator) ? page!.separator : '') + segments.slice(0, index + 1).join(page!.separator) + (page!.separator === '\\' && index === 0 ? '\\' : '');
      return <Button size="xs" variant="text" key={full} onClick={() => {void load(full, hidden);}}>{segment}</Button>;
    })}</nav>
    <div className={styles.folderTools}><Button size="xs" disabled={!page?.parent || busy} icon="i-arrow-up" onClick={() => {void load(page!.parent, hidden);}}>{t('next.settings.up')}</Button><label><input type="checkbox" checked={hidden} onChange={event => {setHidden(event.currentTarget.checked); void load(page?.path ?? path, event.currentTarget.checked);}} />{t('next.settings.hiddenFolders')}</label></div>
    {failed ? <div className={styles.folderNotice} role="alert">{t('next.settings.folderFailed')}<Button size="xs" onClick={() => {void load(page?.path ?? path, hidden);}}>{t('next.states.tryAgain')}</Button></div> : <div className={styles.folderEntries} aria-busy={busy}>
      {!page?.path && page?.roots.map(root => <button type="button" key={root.path} onClick={() => {void load(root.path, hidden);}}><Icon name="i-folder" size={22} /><span>{root.name}<small>{root.path}</small></span><Icon name="i-right" size={18} /></button>)}
      {page?.entries.filter(entry => entry.kind === 'directory').map(entry => <div className={styles.folderEntry} key={entry.path}>
        {multiple && <input type="checkbox" disabled={!entry.readable || busy} checked={selected.includes(entry.path)} aria-label={t('next.settings.selectRow', {name: entry.name})} onChange={event => {const checked = event.currentTarget.checked; setSelected(old => checked ? [...old, entry.path] : old.filter(path => path !== entry.path));}} />}
        <button type="button" disabled={!entry.readable || busy} onClick={() => {void load(entry.path, hidden);}}><Icon name="i-folder" size={22} /><span>{entry.name}{!entry.readable && <small>{t('next.settings.folderUnreadable')}</small>}</span><Icon name="i-right" size={18} /></button>
      </div>)}
      {page?.path && page.entries.length === 0 && !busy && <p className={styles.folderNotice}>{t('next.settings.noFolders')}</p>}
      {page?.truncated && <p className={styles.folderNotice}>{t('next.settings.foldersTruncated')}</p>}
      {page?.nextCursor && <Button size="xs" disabled={busy} onClick={() => {void load(page.path, hidden, page.nextCursor);}}>{t('next.settings.loadMore')}</Button>}
    </div>}
    {busy && <p className={styles.folderNotice} role="status">{t('next.settings.loading')}</p>}
  </Dialog>;
}
