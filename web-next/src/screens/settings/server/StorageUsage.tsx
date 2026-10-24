import {useEffect, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {SettingsStorageSource} from './storage-source';
import styles from './server.module.css';

const t = defaultI18n.t;
type RowProps = Readonly<{source: SettingsStorageSource; label: string}>;

function useStorage(source: SettingsStorageSource) {
  useEffect(() => {void source.load();}, [source]);
  return useSyncExternalStore(source.subscribe, source.getSnapshot, source.getSnapshot);
}
function Failure({source}: {source: SettingsStorageSource}) {
  return <p className={styles.storageFailure} role="alert"><span>{t('next.settings.folderFailed')}</span><button type="button" onClick={() => {void source.retry();}}>{t('next.shell.tryAgain')}</button></p>;
}

/** The one data folder, as the server reports it. */
export function StorageDataFolder({source, label}: RowProps) {
  const state = useStorage(source);
  if (state.phase === 'error') return <Failure source={source} />;
  if (state.phase !== 'ready' || !state.report) return <p role="status">{t('next.settings.loading')}</p>;
  return <code className={styles.storagePath} aria-label={label}>{state.report.stateDirectory}</code>;
}

/** Usage by folder, with the server's own names, descriptions and shared total. */
export function StorageUsage({source, label}: RowProps) {
  const state = useStorage(source);
  if (state.phase === 'error') return <Failure source={source} />;
  if (state.phase !== 'ready' || !state.report) return <p role="status">{t('next.settings.loading')}</p>;
  const report = state.report, shown = report.categories.filter(category => category.present);
  return <div className={styles.storage} role="group" aria-label={label}>
    <div className={styles.storageTotal}><b>{t('web.maintenance.totalLabel')}</b><span>{report.totalBytesText}</span></div>
    <ul className={styles.storageList}>{shown.map(category => {
      const share = report.totalBytes > 0 ? Math.max(0, Math.min(1, category.bytes / report.totalBytes)) : 0;
      const description = category.regenerable ? t('web.maintenance.regenerableHelp', {description: category.description}) : category.description;
      return <li key={category.id}><div className={styles.storageText}><b>{category.name}</b><small>{description}</small></div><span className={styles.storageSize}>{category.bytesText}</span>
        <span className={styles.storageBar} aria-hidden="true"><i style={{width: `${(share * 100).toFixed(1)}%`}} /></span></li>;
    })}</ul>
    {report.truncated && <p className={styles.storageNote}>{t('web.maintenance.truncatedHelp')}</p>}
  </div>;
}
