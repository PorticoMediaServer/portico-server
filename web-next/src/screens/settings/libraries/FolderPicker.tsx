import {useCallback, useEffect, useId, useRef, useState} from 'react';
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
  const [showHidden, setShowHidden] = useState(false);
  const suggestionId = useId();
  const [suggestions, setSuggestions] = useState<readonly string[]>([]);
  useEffect(() => {
    setSuggestions([]);
    if (!open || !typed || typed === page?.path) return;
    const active = new AbortController();
    const timer = setTimeout(() => {
      void browseServerFilesystem(api, {path: typed, includeFiles: false, showHidden, limit: 8}, active.signal).then(next => {
        if (!active.signal.aborted) setSuggestions(next.suggestions);
      }, () => {});
    }, 250);
    return () => { clearTimeout(timer); active.abort(); };
  }, [api, open, typed, page?.path, showHidden]);
  const request = useRef<AbortController | undefined>(undefined);
  const read = useCallback(async (path?: string, hidden = false, cursor?: string) => {
    request.current?.abort();
    const active = new AbortController();
    request.current = active;
    // A pending or failed destination must never leave the previous folder choosable.
    if (!cursor) { setPage(undefined); setTyped(path ?? ''); }
    setBusy(true); setFailed(false);
    try {
      const next = await browseServerFilesystem(api, {...(path ? {path} : {}), includeFiles: false, showHidden: hidden, limit: 200, ...(cursor ? {cursor} : {})}, active.signal);
      if (active.signal.aborted || request.current !== active) return;
      setPage(previous => cursor && previous?.path === next.path
        ? {...next, entries: [...previous.entries, ...next.entries.filter(entry => !previous.entries.some(value => value.path === entry.path))]}
        : next);
      setTyped(next.path);
    } catch {
      if (!active.signal.aborted && request.current === active) setFailed(true);
    } finally {
      if (!active.signal.aborted && request.current === active) setBusy(false);
    }
  }, [api]);
  useEffect(() => {
    if (open) { setShowHidden(false); void read(undefined); }
    return () => { request.current?.abort(); request.current = undefined; };
  }, [open, read]);
  const changeOpen = (next: boolean) => {
    if (!next) { request.current?.abort(); request.current = undefined; }
    onOpenChange(next);
  };
  const go = (path: string) => { void read(path, showHidden); };
  const breadcrumbs: {path: string; label: string}[] = [];
  if (page?.path) {
    const separator = page.separator;
    const root = [...page.roots].sort((a, b) => b.path.length - a.path.length).find(value =>
      page.path === value.path || page.path.startsWith(value.path.endsWith(separator) ? value.path : value.path + separator));
    if (root) {
      breadcrumbs.push({path: root.path, label: root.name});
      let path = root.path;
      for (const part of page.path.slice(root.path.length).split(separator).filter(Boolean)) {
        path = path.endsWith(separator) ? path + part : path + separator + part;
        breadcrumbs.push({path, label: part});
      }
    }
  }
  return <Dialog open={open} onOpenChange={changeOpen} size="md" title={t('next.settings.chooseFolder')} description={t('next.settings.folderBreadcrumbs')}
    footer={<><Button onClick={() => changeOpen(false)}>{t('next.settings.cancel')}</Button>
      <Button variant="primary" disabled={!page?.path || busy || failed} onClick={() => { if (page?.path) { onChoose(page.path); changeOpen(false); } }}>{t('next.settings.chooseThisFolder')}</Button></>}>
    <div className={styles.picker}>
      {breadcrumbs.length > 0 && <nav aria-label={t('next.settings.folderPath')}>
        {breadcrumbs.map((crumb, index) => <span key={crumb.path}>
          {index > 0 && <span aria-hidden="true"> {page?.separator} </span>}
          <Button variant="text" size="small" disabled={busy} aria-current={index === breadcrumbs.length - 1 ? 'location' : undefined}
            onClick={() => go(crumb.path)}>{crumb.label}</Button>
        </span>)}
      </nav>}
      <div className={styles.pickerPath}>
        <TextField label={t('next.settings.folderPath')} list={suggestionId} autoComplete="off" value={typed} onChange={event => setTyped(event.currentTarget.value)} onKeyDown={event => {
          if (event.key === 'Enter' && !event.nativeEvent.isComposing && !busy) { event.preventDefault(); go(typed); }
        }} />
        <datalist id={suggestionId}>{suggestions.map(path => <option key={path} value={path}/>)}</datalist>
        <Button disabled={busy} onClick={() => go(typed)}>{t('next.settings.go')}</Button>
        <Button disabled={busy || !page?.parent} onClick={() => { if (page?.parent) go(page.parent); }}>{t('next.settings.up')}</Button>
      </div>
      <label className={styles.pickerNote}>
        <input type="checkbox" checked={showHidden} disabled={busy} onChange={event => {
          const next = event.currentTarget.checked;
          setShowHidden(next); void read(page?.path, next);
        }} /> {t('next.settings.hiddenFolders')}
      </label>
      {failed && <div role="alert" className={styles.pickerNote}>
        <p>{t('next.ownerLibraries.explorer.failed')}</p>
        <p>{t('next.ownerLibraries.explorer.failedHelp')}</p>
      </div>}
      {busy && !page && <p role="status">{t('next.ownerLibraries.explorer.reading')}</p>}
      {page && page.roots.length > 0 && <nav aria-label={t('next.ownerLibraries.explorer.places')}>
        <p className={styles.pickerNote}>{t('next.ownerLibraries.explorer.places')}</p>
        <ul className={styles.pickerList}>{page.roots.map(root => <li key={root.path}>
          <button type="button" className={styles.pickerRow} disabled={busy} aria-current={page.path === root.path ? 'location' : undefined}
            onClick={() => go(root.path)}><span>{root.name}</span><small>{root.path}</small></button>
        </li>)}</ul>
      </nav>}
      {page && page.path && (page.entries.length
        ? <ul className={styles.pickerList}>{page.entries.filter(entry => entry.kind === 'directory').map(entry => <li key={entry.path}>
          <button type="button" className={styles.pickerRow} disabled={!entry.readable} onClick={() => go(entry.path)}>
            <span>{entry.name}</span>{entry.symlink && <small>{t('next.ownerLibraries.explorer.link')}</small>}{!entry.readable && <small>{t('next.ownerLibraries.explorer.noAccess')}</small>}</button></li>)}</ul>
        : <p className={styles.pickerNote}>{t('next.ownerLibraries.explorer.noFolders')}</p>)}
      {page?.nextCursor && <Button disabled={busy} onClick={() => { void read(page.path, showHidden, page.nextCursor); }}>{t('next.ownerLibraries.explorer.more')}</Button>}
    </div>
  </Dialog>;
}
