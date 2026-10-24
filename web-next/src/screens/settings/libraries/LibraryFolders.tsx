import {defaultI18n} from '@i18n';
import type {InventoryConfig} from '@core/library-inventory.ts';
import {folderRows} from './model';
import styles from './libraries.module.css';

const t = defaultI18n.t;

/** The library page's Folders tab: where this library reads from, each folder's
 * kind and whether the server can reach it. Read-only in this slice; scanning a
 * folder, changing it and removing it come with the dialogs slice. */
export function LibraryFolders({config, loading, failed}: Readonly<{config?: InventoryConfig; loading: boolean; failed: boolean}>) {
  if (failed) return <p role="alert" className={styles.formNote}>{t('next.settings.folderFailed')}</p>;
  if (!config) return <p role="status" className={styles.formNote}>{t(loading ? 'next.settings.loading' : 'next.settings.libraries.noFolders')}</p>;
  const rows = folderRows(config.sources);
  if (!rows.length) return <p className={styles.formNote}>{t('next.settings.libraries.noFolders')}</p>;
  return <ul className={styles.folders} aria-label={t('next.settingsSpec.server.libraries.panels.library.tabs.1.title')}>
    {rows.map(row => <li key={row.id} className={styles.folder}>
      <span className={styles.folderName}>{row.name}</span>
      <small className={styles.folderPath}>{row.path}</small>
      <span className={styles.folderFacts}>
        <span>{t(row.network ? 'next.settingsSpec.server.libraries.panels.library.tabs.1.groups.1.rows.2.control.options.1.label'
          : 'next.settingsSpec.server.libraries.panels.library.tabs.1.groups.1.rows.2.control.options.0.label')}</span>
        {(!row.enabled || !/^healthy|available|ok$/i.test(row.health)) && <span className={styles.folderUnavailable}>{t('next.settings.libraries.folderUnavailable')}</span>}
      </span>
    </li>)}
  </ul>;
}
