import {useEffect, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {Button} from '../../../kit/controls';
import type {DownloadsUsageSource} from './downloads-usage-source';
import styles from '../server/server.module.css';

const t = defaultI18n.t;

/** Authorized names and actual retained bytes; cache removal has no matching producer. */
export function DownloadsUsage({source, label}: Readonly<{source: DownloadsUsageSource; label: string}>) {
  const state = useSyncExternalStore(source.subscribe, source.getSnapshot, source.getSnapshot);
  useEffect(() => {void source.load();}, [source]);
  const busy = state.phase === 'loading';
  return <div className={styles.storage} role="group" aria-label={label} aria-busy={busy}>
    {state.phase === 'idle' || busy ? <p role="status">{t('next.settings.loading')}</p> : null}
    {state.phase === 'error' ? <p className={styles.storageFailure} role="alert"><span>{t('next.saved.loadFailed')}</span>
      <button type="button" onClick={() => {void source.load('retry');}}>{t('next.shell.tryAgain')}</button></p> : null}
    {state.page && <ul className={styles.storageList}>{state.page.rows.map(row => <li key={row.profileKey}>
      <div className={styles.storageText}><b>{row.profileName || row.accountName || '—'}</b>
        {row.accountName && row.accountName !== row.profileName ? <small>{row.accountName}</small> : null}</div>
      <span className={styles.storageSize}>{defaultI18n.bytes(row.storedBytes)}</span>
    </li>)}</ul>}
    {state.phase === 'ready' && !state.page?.rows.length ? <p>{t('web.downloads.emptyTitle')}</p> : null}
    <div>
      {state.previous && <Button size="small" disabled={busy} onClick={() => {void source.load('previous');}}>{t('next.reader.previous')}</Button>}
      {state.page?.next && <Button size="small" disabled={busy} onClick={() => {void source.load('next');}}>{t('next.reader.next')}</Button>}
    </div>
  </div>;
}
