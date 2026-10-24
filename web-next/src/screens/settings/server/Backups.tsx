import {useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {backupProgressFraction, type BackupSummary} from '@core/administration.ts';
import type {LibraryContentApi} from '@core/library-content.ts';
import {Button} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays/Dialog';
import {BackupConfirmationDialog, BackupList, BackupPassphraseDialog, ServerFolderPicker, SettingsAction} from '../../../kit/settings';
import type {SettingsBackupSource, SettingsRestoreSelection} from '../backup-source';
import {protectedRestoreResult} from './restore';
import styles from './server.module.css';

const t = defaultI18n.t;

export type BackupSurfaceProps = Readonly<{
  source: SettingsBackupSource;
  subscribe: (listener: () => void) => () => void;
  isCurrent: () => boolean;
  scopeKey: string;
  reportError: (error: unknown) => void;
}>;

function useBackups(props: BackupSurfaceProps) {return useSyncExternalStore(props.subscribe, props.source.getSnapshot, props.source.getSnapshot);}

/** "Back up to": the server's own backup folder, and the drawn folder picker that changes it. */
export function StorageBackupFolder(props: BackupSurfaceProps & Readonly<{api: LibraryContentApi}>) {
  const state = useBackups(props), folder = state.settings?.backupFolder;
  const [open, setOpen] = useState(false);
  return <div className={styles.backupFolder}>
    {folder ? <code className={styles.storagePath}>{folder}</code> : <span className={styles.storagePath}>{t('next.settings.loading')}</span>}
    <Button variant="glass" size="small" disabled={!folder || state.busy || !props.isCurrent()} onClick={() => setOpen(true)}>{t('next.settings.changeFolder')}</Button>
    <ServerFolderPicker open={open} onClose={() => setOpen(false)} api={props.api} initialPath={folder} purpose="folder"
      onChoose={paths => {const path = paths[0];if (!path) return;void props.source.save({backupFolder: path}).catch(props.reportError);}} />
  </div>;
}

/** "Back up now" and the real fraction the server reports while it runs. */
export function StorageBackupNow(props: BackupSurfaceProps) {
  const state = useBackups(props), document = state.document, running = document?.running;
  const fraction = running ? backupProgressFraction(running) : undefined;
  // Restore and delete stay disabled until a running backup finishes, so one is offered at a time.
  const blocked = !document || state.busy || !!running || !!document.restore;
  return <div className={styles.backupNow}>
    <Button variant="glass" size="small" disabled={blocked || !props.isCurrent()} onClick={() => {void props.source.backupNow().catch(props.reportError);}}>
      {t(running ? 'web.maintenance.backingUp' : 'web.maintenance.backUpNow')}
    </Button>
    {running && <span className={styles.backupProgress} role="progressbar" aria-label={t('web.maintenance.backingUp')} aria-valuemin={0} aria-valuemax={100}
      aria-valuenow={fraction === undefined ? undefined : Math.round(fraction * 100)}><i style={{width: `${((fraction ?? 0) * 100).toFixed(1)}%`}} /></span>}
    {!running && document?.lastBackup?.outcome === 'failed' && <p className={styles.backupFailure} role="alert">{t(document.lastBackup.errorCode === 'insufficient_disk' ? 'web.maintenance.backupNoSpace' : 'web.console.jobFailed')}</p>}
  </div>;
}

/** The server's own backup list. Restore and delete ask first, and only a captured row is ever sent. */
export function StorageBackups(props: BackupSurfaceProps) {
  const state = useBackups(props), document = state.document;
  const [confirm, setConfirm] = useState<Readonly<{mode: 'restore' | 'delete'; backup: BackupSummary}>>();
  const [protectedBackup, setProtectedBackup] = useState<Readonly<{selection: SettingsRestoreSelection}>>();
  if (!document) return <p role="status">{t('next.settings.loading')}</p>;
  if (!document.backups.length) return <p className={styles.backupEmpty}>{t('web.maintenance.noBackups')}</p>;
  const submit = async (backup: BackupSummary, signal: AbortSignal) => {
    if (!confirm) return;
    if (confirm.mode === 'delete') {await props.source.remove(backup.id, signal);return;}
    const selection = await props.source.prepareRestore({backupId: backup.id}, signal);
    // A protected backup is opened by its own passphrase step (ruling 17.4), after this confirmation.
    if (selection.encrypted) {setProtectedBackup({selection});return;}
    await props.source.restore(selection, undefined, signal);
  };
  const openPassphrase = async (passphrase: string, signal: AbortSignal) => {
    if (!protectedBackup) return 'done' as const;
    try {await props.source.restore(protectedBackup.selection, passphrase, signal);setProtectedBackup(undefined);return 'done' as const;}
    // A wrong passphrase and a damaged archive are one refusal (v2 Root §1), so they say the same thing.
    catch (error) {if (protectedRestoreResult(error) === 'unopened') return 'unopened' as const;throw error;}
  };
  return <>
    <BackupList backups={document.backups} busy={state.busy || !!document.running || !!document.restore} scopeKey={props.scopeKey}
      isCurrent={props.isCurrent} subscribeAuthority={props.subscribe} format={defaultI18n}
      requestRestore={backup => setConfirm({mode: 'restore', backup})} requestDelete={backup => setConfirm({mode: 'delete', backup})} />
    {confirm && <BackupConfirmationDialog open mode={confirm.mode} backup={confirm.backup} scopeKey={props.scopeKey} isCurrent={props.isCurrent}
      subscribeAuthority={props.subscribe} format={defaultI18n} submit={submit} onClose={() => setConfirm(undefined)} onFailure={props.reportError} />}
    <BackupPassphraseDialog open={!!protectedBackup} mode="restore" scopeKey={props.scopeKey} isCurrent={props.isCurrent} subscribeAuthority={props.subscribe}
      submit={openPassphrase} onDone={() => setProtectedBackup(undefined)} onFailure={() => props.reportError(new Error('backup_passphrase_rejected'))}
      onClose={() => {props.source.cancelRestoreSelection();setProtectedBackup(undefined);}} />
  </>;
}

/** "Restore from a file": the bounded explorer, then the drawn confirmation, then the real restore. */
export function StorageRestoreFile(props: BackupSurfaceProps & Readonly<{api: LibraryContentApi}>) {
  const state = useBackups(props), busyNow = state.busy || !!state.document?.running || !!state.document?.restore;
  const [choosing, setChoosing] = useState(false), [chosen, setChosen] = useState<string>(), [busy, setBusy] = useState(false);
  const [protectedSelection, setProtectedSelection] = useState<SettingsRestoreSelection>();
  const request = useRef<AbortController | undefined>(undefined), cancelButton = useRef<HTMLButtonElement>(null), opener = useRef<HTMLButtonElement>(null);
  const close = () => {request.current?.abort();request.current = undefined;setChosen(undefined);setBusy(false);props.source.cancelRestoreSelection();};
  const restore = async () => {
    if (!chosen || request.current) return;
    const controller = request.current = new AbortController();
    setBusy(true);
    try {
      const selection = await props.source.prepareRestore({path: chosen}, controller.signal);
      if (selection.encrypted) {setProtectedSelection(selection);setChosen(undefined);return;}
      await props.source.restore(selection, undefined, controller.signal);
      if (!controller.signal.aborted) setChosen(undefined);
    } catch (error) {if (!controller.signal.aborted) props.reportError(error);}
    finally {if (request.current === controller) request.current = undefined;setBusy(false);}
  };
  return <>
    <Button ref={opener} variant="glass" size="small" disabled={busyNow || !props.isCurrent()} onClick={() => setChoosing(true)}>{t('next.settingsSpec.server.storage.groups.1.rows.5.control.label')}</Button>
    <ServerFolderPicker open={choosing} onClose={() => setChoosing(false)} api={props.api} purpose="backup" returnFocusRef={opener}
      onChoose={paths => {const path = paths[0];if (path) setChosen(path);}} />
    <Dialog open={!!chosen} onOpenChange={open => {if (!open) close();}} title={t('web.maintenance.restoreFileTitle')} initialFocusRef={cancelButton} returnFocusRef={opener}
      footer={<><Button ref={cancelButton} size="small" onClick={close}>{t('next.settings.cancel')}</Button><SettingsAction size="small" appearance="primary" loading={busy} disabled={busy} onClick={() => {void restore();}}>{t('web.maintenance.restoreConfirm')}</SettingsAction></>}>
      <p>{t('web.maintenance.restoreBody')}</p>
    </Dialog>
    <BackupPassphraseDialog open={!!protectedSelection} mode="restore" scopeKey={props.scopeKey} isCurrent={props.isCurrent} subscribeAuthority={props.subscribe}
      submit={async (passphrase, signal) => {
        if (!protectedSelection) return 'done' as const;
        try {await props.source.restore(protectedSelection, passphrase, signal);setProtectedSelection(undefined);return 'done' as const;}
        catch (error) {if (protectedRestoreResult(error) === 'unopened') return 'unopened' as const;throw error;}
      }}
      onDone={() => setProtectedSelection(undefined)} onFailure={() => props.reportError(new Error('backup_passphrase_rejected'))}
      onClose={() => {props.source.cancelRestoreSelection();setProtectedSelection(undefined);}} />
  </>;
}
