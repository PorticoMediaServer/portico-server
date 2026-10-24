import {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {ContentScope, LibraryContentApi} from '@core/library-content.ts';
import {LibraryInventoryService, type InventorySource} from '@core/library-inventory.ts';
import {Button, RichSelect, TextField} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays/Dialog';
import {Menu} from '../../../kit/overlays/Popover';
import {folderRows, folderIntervalOptions} from './model';
import styles from './libraries.module.css';

const t = defaultI18n.t;
const label = (id: string) => t(('next.settingsSpec.server.libraries.panels.library.tabs.1.groups.' + id) as Parameters<typeof t>[0]);

/** The library page's Folders tab: where this library reads from, each folder's
 * kind and health, and the drawn folder menu — scan this folder, check
 * availability, edit and remove. Every command runs through the owner's own
 * inventory service and reconciles from the server's answer. */
export function LibraryFolders({libraryId, api, scope, isCurrent, onError}: Readonly<{
  libraryId: string; api: LibraryContentApi; scope: ContentScope; isCurrent: () => boolean; onError: (error: unknown) => void;
}>) {
  const service = useMemo(() => new LibraryInventoryService({api, scope, libraryId}), [api, scope, libraryId]);
  const snapshot = useSyncExternalStore(service.subscribe, service.getSnapshot, service.getSnapshot);
  const [menu, setMenu] = useState<string>(), [editing, setEditing] = useState<InventorySource>(), [removing, setRemoving] = useState<InventorySource>(), [busy, setBusy] = useState(false);
  useEffect(() => { void service.refresh().catch(onError); return () => service.dispose(); }, [service, onError]);
  const run = async (command: () => Promise<boolean>) => {
    if (!isCurrent()) return;
    setBusy(true);
    try { await command(); } catch (error) { onError(error); } finally { setBusy(false); }
  };
  if (!snapshot.config) return <p role="status" className={styles.formNote}>{t(snapshot.loading ? 'next.settings.loading' : 'next.settings.folderFailed')}</p>;
  const config = snapshot.config, rows = folderRows(config.sources);
  const source = (id: string) => config.sources.find(value => value.id === id);
  if (!rows.length) return <p className={styles.formNote}>{t('next.settings.libraries.noFolders')}</p>;
  return <>
    <ul className={styles.folders} aria-label={t('next.settingsSpec.server.libraries.panels.library.tabs.1.title')}>
      {rows.map(row => <li key={row.id} className={styles.folder} data-folder-row={row.id}>
        <span className={styles.folderName}>{row.name}</span>
        <small className={styles.folderPath}>{row.path}</small>
        <span className={styles.folderFacts}>
          <span>{t(row.network ? 'next.settingsSpec.server.libraries.panels.library.tabs.1.groups.1.rows.2.control.options.1.label'
            : 'next.settingsSpec.server.libraries.panels.library.tabs.1.groups.1.rows.2.control.options.0.label')}</span>
          {(!row.enabled || !/^healthy|available|ok$/i.test(row.health)) && <span className={styles.folderUnavailable}>{t('next.settings.libraries.folderUnavailable')}</span>}
        </span>
        <span className={styles.folderActions}>
          <Menu open={menu === row.id} onOpenChange={open => setMenu(open ? row.id : undefined)} title={row.name} items={[
            {id: 'scan', label: t('next.settings.libraries.scanFolder'), onSelect: () => { const found = source(row.id); if (found) void run(() => service.scanSource(found.id)); }},
            {id: 'check', label: t('next.settings.libraries.checkFolder'), onSelect: () => { const found = source(row.id); if (found) void run(() => service.checkSource(found.id)); }},
            {id: 'edit', label: t('next.settings.libraries.editFolder'), onSelect: () => { const found = source(row.id); if (found) setEditing(found); }},
            {id: 'remove', label: t('next.settings.libraries.removeFolderTitle'), destructive: true, onSelect: () => { const found = source(row.id); if (found) setRemoving(found); }},
          ]} trigger={<Button variant="icon" icon="i-more" iconOnly aria-label={t('next.settings.actions')} disabled={busy} />} />
        </span>
      </li>)}
    </ul>
    <FolderEditor open={!!editing} onOpenChange={open => { if (!open) setEditing(undefined); }} source={editing}
      busy={busy} onSave={edit => { if (!editing) return;
        void run(() => service.saveSource(editing.id, {...edit, expectedRevision: config.revision, acceptReplacement: false})); setEditing(undefined); }} />
    <Dialog open={!!removing} onOpenChange={open => { if (!open) setRemoving(undefined); }} size="sm" role="alertdialog"
      title={t('next.settings.libraries.removeFolderTitle')} description={t('next.settingsSpec.server.libraries.panels.library.tabs.1.groups.0.rows.0.control.itemActions.1.confirm')}
      footer={<><Button onClick={() => setRemoving(undefined)}>{t('next.settings.cancel')}</Button>
        <Button variant="primary" loading={busy} onClick={() => { if (!removing) return; void run(() => service.removeSource(removing.id, config.revision)); setRemoving(undefined); }}>
          {t('next.settings.libraries.removeFolderTitle')}</Button></>}>
      <p className={styles.formNote}>{removing?.path}</p>
    </Dialog>
  </>;
}

/** One existing folder's editable facts: its name, its path, and how often the
 * server checks it for changes. The location is read-only here. */
function FolderEditor({open, onOpenChange, source, busy, onSave}: Readonly<{
  open: boolean; onOpenChange: (open: boolean) => void; source?: InventorySource; busy: boolean;
  onSave: (edit: Readonly<{name: string; path: string; classification: 'local' | 'network'; followSymlinks: boolean; intervalSeconds: number}>) => void;
}>) {
  const [name, setName] = useState(''), [path, setPath] = useState(''), [interval, setInterval] = useState('0');
  useEffect(() => { if (!source) return; setName(source.name); setPath(source.path); setInterval(String(source.intervalSeconds)); }, [source]);
  return <Dialog open={open} onOpenChange={onOpenChange} size="md" title={t('next.settings.libraries.editFolder')}
    footer={<><Button onClick={() => onOpenChange(false)}>{t('next.settings.cancel')}</Button>
      <Button variant="primary" loading={busy} disabled={!source || !name.trim() || !path.trim()} onClick={() => {
        if (source) onSave({name: name.trim(), path: path.trim(), classification: source.classification,
          followSymlinks: source.followSymlinks, intervalSeconds: Number(interval) || 0});
      }}>{t('next.settings.libraries.saveFolder')}</Button></>}>
    <div className={styles.form}>
      <TextField label={label('1.rows.1.label')} value={name} onChange={event => setName(event.currentTarget.value)} />
      <TextField label={label('1.rows.0.label')} value={path} onChange={event => setPath(event.currentTarget.value)} />
      <RichSelect label={label('1.rows.3.label')} value={interval} options={folderIntervalOptions().map(option => ({...option, label: t(option.label)}))}
        onValueChange={setInterval} />
    </div>
  </Dialog>;
}
