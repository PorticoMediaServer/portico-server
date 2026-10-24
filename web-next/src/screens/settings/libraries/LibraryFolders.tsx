import {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {ContentScope, LibraryContentApi} from '@core/library-content.ts';
import {LibraryInventoryService} from '@core/library-inventory.ts';
import {Button} from '../../../kit/controls';
import {Menu} from '../../../kit/overlays/Popover';
import {folderRows} from './model';
import styles from './libraries.module.css';

const t = defaultI18n.t;

/** The library page's Folders tab: where this library reads from, each folder's
 * kind and health, and the two commands the frame puts on every folder — scan
 * this folder and check availability. Edit and Remove folder are the next
 * dialog slice; nothing here invents a folder or a scan state. */
export function LibraryFolders({libraryId, api, scope, isCurrent, onError}: Readonly<{
  libraryId: string; api: LibraryContentApi; scope: ContentScope; isCurrent: () => boolean; onError: (error: unknown) => void;
}>) {
  const service = useMemo(() => new LibraryInventoryService({api, scope, libraryId}), [api, scope, libraryId]);
  const snapshot = useSyncExternalStore(service.subscribe, service.getSnapshot, service.getSnapshot);
  const [busy, setBusy] = useState<string>();
  useEffect(() => { void service.refresh().catch(onError); return () => service.dispose(); }, [service, onError]);
  const run = async (id: string, command: () => Promise<boolean>) => {
    if (!isCurrent()) return;
    setBusy(id);
    try { if (!(await command())) return; } catch (error) { onError(error); } finally { setBusy(undefined); }
  };
  if (!snapshot.config) return <p role="status" className={styles.formNote}>{t(snapshot.loading ? 'next.settings.loading' : 'next.settings.folderFailed')}</p>;
  const rows = folderRows(snapshot.config.sources);
  if (!rows.length) return <p className={styles.formNote}>{t('next.settings.libraries.noFolders')}</p>;
  return <ul className={styles.folders} aria-label={t('next.settingsSpec.server.libraries.panels.library.tabs.1.title')}>
    {rows.map(row => <li key={row.id} className={styles.folder} data-folder-row={row.id}>
      <span className={styles.folderName}>{row.name}</span>
      <small className={styles.folderPath}>{row.path}</small>
      <span className={styles.folderFacts}>
        <span>{t(row.network ? 'next.settingsSpec.server.libraries.panels.library.tabs.1.groups.1.rows.2.control.options.1.label'
          : 'next.settingsSpec.server.libraries.panels.library.tabs.1.groups.1.rows.2.control.options.0.label')}</span>
        {(!row.enabled || !/^healthy|available|ok$/i.test(row.health)) && <span className={styles.folderUnavailable}>{t('next.settings.libraries.folderUnavailable')}</span>}
      </span>
      <span className={styles.folderActions}>
        <Button variant="glass" size="small" loading={busy === row.id} disabled={!!busy}
          onClick={() => { void run(row.id, () => service.scanSource(row.id)); }}>
          {t('next.settingsSpec.server.libraries.panels.library.tabs.1.groups.0.rows.0.control.itemActions.0.label')}
        </Button>
        <Menu open={busy === 'menu:' + row.id} onOpenChange={open => setBusy(open ? 'menu:' + row.id : undefined)} title={row.name} items={[
          {id: 'check', label: t('next.settings.libraries.checkAvailability'), disabled: !!busy && busy !== 'menu:' + row.id,
            onSelect: () => { setBusy(row.id); void service.checkSource(row.id).catch(onError).finally(() => setBusy(undefined)); }},
        ]} trigger={<Button variant="icon" icon="i-more" iconOnly aria-label={t('next.settings.actions')} disabled={!!busy && busy !== 'menu:' + row.id} />} />
      </span>
    </li>)}
  </ul>;
}
