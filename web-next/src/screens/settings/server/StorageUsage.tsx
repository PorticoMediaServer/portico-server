import {useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {SettingsStorageSource, StorageCategory} from './storage-source';
import type {SettingsCleanupSource} from './cleanup-source';
import {Button, TextField} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays/Dialog';
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
export function StorageUsage({source, label, cleanup}: RowProps & {cleanup: SettingsCleanupSource}) {
  const state = useStorage(source), action = useSyncExternalStore(cleanup.subscribe, cleanup.getSnapshot, cleanup.getSnapshot);
  const [chosen, setChosen] = useState<{cleanup:SettingsCleanupSource;category:StorageCategory}>(), [days,setDays] = useState(30), [typed,setTyped] = useState('');
  const dialog = useRef<typeof chosen>(undefined), owner = useRef(cleanup);
  if (owner.current !== cleanup) {owner.current = cleanup;dialog.current = undefined;}
  useLayoutEffect(() => {setChosen(undefined);return () => {dialog.current = undefined;};}, [cleanup]);
  const held = chosen?.cleanup === cleanup && dialog.current === chosen ? chosen : undefined;
  const close = () => {dialog.current = undefined;setChosen(undefined);setTyped('');};
  const open = (category:StorageCategory) => {if (source.getSnapshot().phase !== 'ready' || action.busy) return;const pending = cleanup.getSnapshot().pending;const next = {cleanup,category:pending?.category ?? category};dialog.current = next;setChosen(next);setDays(pending?.days ?? 30);setTyped('');};
  const submit = async () => {if (!held || dialog.current !== held || typed !== held.category.name || action.busy) return;const done = cleanup.getSnapshot().pending ? await cleanup.retry() : await cleanup.cleanup(held.category,days,typed);if (done && dialog.current === held) close();};
  if (state.phase === 'error') return <Failure source={source} />;
  if (state.phase !== 'ready' || !state.report) return <p role="status">{t('next.settings.loading')}</p>;
  const report = state.report, shown = report.categories.filter(category => category.present);
  return <div className={styles.storage} role="group" aria-label={label}>
    <div className={styles.storageTotal}><b>{t('web.maintenance.totalLabel')}</b><span>{report.totalBytesText}</span></div>
    <ul className={styles.storageList}>{shown.map(category => {
      const share = report.totalBytes > 0 ? Math.max(0, Math.min(1, category.bytes / report.totalBytes)) : 0;
      const description = category.regenerable ? t('web.maintenance.regenerableHelp', {description: category.description}) : category.description;
      return <li key={category.id}><div className={styles.storageText}><b>{category.name}</b><small>{description}</small></div><span className={styles.storageSize}>{category.bytesText}</span>
        <span className={styles.storageBar} aria-hidden="true"><i style={{width: `${(share * 100).toFixed(1)}%`}} /></span>{category.cleanable && category.bytes > 0 && <div className={styles.storageCleanup}><Button size="xs" variant="text" disabled={action.busy || !!action.pending && action.pending.category.id !== category.id} onClick={() => open(category)}>{t('server.cleanUp')}</Button></div>}</li>;
    })}</ul>
    {action.error && <p role="alert" className={styles.backupFailure}>{t('error.unconfirmedChange')}</p>}
    {action.done && <p role="status" className={styles.storageNote}>{t('server.cleanup.done')}</p>}
    <Dialog open={!!held} onOpenChange={value => {if (!value && !action.busy) close();}} role="alertdialog" title={t('web.maintenance.cleanUpTitle',{name:held?.category.name ?? ''})} description={t('server.cleanup.body',{canRecreate:held?.category.regenerable ? 'yes':'no'})} width={520} dismissOnBack={!action.busy} dismissOnOutsidePress={!action.busy} showCloseButton={!action.busy}
      footer={<><Button size="compact" variant="glass" disabled={action.busy} onClick={close}>{t('action.cancel')}</Button><Button size="compact" variant={held?.category.regenerable ? 'primary':'glass'} className={held?.category.regenerable ? undefined:styles.cleanupDanger} disabled={action.busy || typed !== held?.category.name || !Number.isSafeInteger(days) || days < 0 || days > 3650} onClick={() => {void submit();}}>{t(action.pending ? 'action.tryAgain':'server.cleanUp')}</Button></>}>
      <div className={styles.cleanupFields}><TextField label={t('web.maintenance.olderThan')} type="number" min={0} max={3650} step={1} value={days} disabled={action.busy || !!action.pending} onChange={event => setDays(Number(event.target.value))}/><TextField label={t('web.deleteAccount.typeLabel',{value:held?.category.name ?? ''})} autoComplete="off" value={typed} disabled={action.busy} onChange={event => setTyped(event.target.value)}/>{action.error && <p role="alert" className={styles.backupFailure}>{t('error.unconfirmedChange')}</p>}</div>
    </Dialog>
    {report.truncated && <p className={styles.storageNote}>{t('web.maintenance.truncatedHelp')}</p>}
  </div>;
}
