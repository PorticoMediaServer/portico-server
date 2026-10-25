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

const formatBytes = (bytes: number) => bytes >= 1024 ** 3 ? `${(bytes / 1024 ** 3).toFixed(1)} GB` : bytes >= 1024 ** 2 ? `${Math.round(bytes / 1024 ** 2)} MB` : `${Math.round(bytes / 1024)} KB`;

/** The drawn data-folder folders and the conversion folder outside them, from
 * the server's own layout measurement. No per-row sweep is drawn. */
const folderRows = [
  ['Database', 'database'],
  ['Metadata', 'metadata'],
  ['Cache', 'cache'],
  ['Backups', 'backups'],
  ['Logs', 'logs'],
  ['Keys', 'keys'],
  ['Temp', 'temp'],
] as const;

/** The one data folder, as the server reports it. */
export function StorageDataFolder({source, label}: RowProps) {
  const state = useStorage(source);
  if (state.phase === 'error') return <Failure source={source} />;
  if (state.phase !== 'ready' || !state.report) return <p role="status">{t('next.settings.loading')}</p>;
  return <code className={styles.storagePath} aria-label={label}>{state.report.stateDirectory}</code>;
}

/** What's using space, by the server's own data-folder layout. */
export function StorageUsage({source, label}: RowProps) {
  const state = useStorage(source);
  if (state.phase === 'error') return <Failure source={source} />;
  if (state.phase !== 'ready' || !state.report || !state.usage) return <p role="status">{t('next.settings.loading')}</p>;
  const usage = state.usage;
  const rows = folderRows.flatMap(([name, key]) => {
    const folder = usage.folders.find(candidate => candidate.name === name);
    return folder ? [{id: name, name: t(`server.space.${key}`), help: t(`server.space.${key}Help`), bytes: folder.bytes}] : [];
  });
  rows.push({id: 'Conversions', name: t('server.space.conversions'), help: t('server.space.conversionsHelp', {path: usage.transcodeDirectory || '/transcode'}), bytes: usage.transcodeBytes});
  const total = rows.reduce((sum, row) => sum + row.bytes, 0);
  return <div className={styles.storage} role="group" aria-label={label}>
    <div className={styles.storageTotal}><b>{t('web.maintenance.totalLabel')}</b><span>{formatBytes(total)}</span></div>
    <ul className={styles.storageList}>{rows.map(row => {
      const share = total > 0 ? Math.max(0, Math.min(1, row.bytes / total)) : 0;
      return <li key={row.id}><div className={styles.storageText}><b>{row.name}</b><small>{row.help}</small></div><span className={styles.storageSize}>{formatBytes(row.bytes)}</span>
        <span className={styles.storageBar} aria-hidden="true"><i style={{width: `${(share * 100).toFixed(1)}%`}} /></span></li>;
    })}</ul>
  </div>;
}
