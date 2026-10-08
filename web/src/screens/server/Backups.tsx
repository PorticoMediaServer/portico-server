import {useEffect, useRef, useState} from 'react';
import {backupProgressFraction, isRestoreRefusalCode, parseEnvelope, parseFilesystemPage, type BackupKind, type BackupSummary, type FilesystemPage, type LastRestore, type RestoreRefusalCode} from '@core/administration.ts';
import {deleteBackup, getBackups, isAuthenticationFailure, restoreBackup, startBackup, waitForBackupCompletion, waitForServerAnswer} from '@core/backups.ts';
import {bytes, problem, useAction, useRead} from '../../admin/console';
import {filesystemPath, mergeFilesystemPages} from './LibrarySettings';
import {createOperationIds} from './operation-ids';
import {useSession} from '../../app/session';
import {useI18n, type MessageId} from '../../app/i18n';
import {Button, ConfirmDialog, Dialog, Freshness, IconButton, Input, ListRow, Loading, Notice, ProgressBar, SettingsGroup, SettingsRow, Surface, Text} from '../../ui';

/** Catalogue copy for every restore/backup refusal code (spec §4). Anything else falls back
 * to the shared presenter. Tested in test/backups.test.ts. */
export const RESTORE_REFUSAL_MESSAGE_IDS: Record<RestoreRefusalCode, MessageId> = {
  restore_source_invalid: 'web.maintenance.restoreSourceInvalid',
  restore_schema_newer: 'web.maintenance.restoreSchemaNewer',
  restore_pre_release: 'web.maintenance.restorePreRelease',
  restore_integrity_failed: 'web.maintenance.restoreIntegrityFailed',
  restore_not_portico: 'web.maintenance.restoreNotPortico',
  insufficient_disk: 'web.maintenance.restoreNoSpace',
};

/** backupKindMessageId names the three spec §1 kinds. Unknown kinds never reach here:
 * the parser skips them. Tested in test/backups.test.ts. */
export function backupKindMessageId(kind: BackupKind): MessageId {
  return kind === 'scheduled' ? 'web.maintenance.backupKindScheduled' : kind === 'manual' ? 'web.maintenance.backupKindManual' : 'web.maintenance.backupKindPreRestore';
}

/** isBackupSourceFile answers whether a server path can be picked as a bare-database restore
 * source: a `.db` file. Tested in test/backups.test.ts. */
export function isBackupSourceFile(path: string): boolean {
  return path.toLowerCase().endsWith('.db');
}

const whenBackup = (iso: string) => (iso ? new Date(iso).toLocaleString(undefined, {dateStyle: 'medium', timeStyle: 'short'}) : '—');

type RestoreTarget = {kind: 'backup'; backup: BackupSummary} | {kind: 'path'; path: string};

/** Backups and restore, Plex model: plain SQLite files, one operation id per logical
 * submission (M19), progress by polling GET every 2 s, and a plain confirmation for
 * restore. The schedule and retention settings live in MaintenanceWindows and stay as
 * they are. */
export function BackupsGroup() {
  const {api, system, session} = useSession();
  const serverId = system?.id ?? session?.viewer.serverId ?? '';
  const {t} = useI18n();
  const read = useRead(() => getBackups(api, serverId), [api, serverId]);
  const action = useAction();
  // M19: one ID per logical write; retries reuse it.
  const [opIds] = useState(createOperationIds);
  const doc = read.data;
  const [backupError, setBackupError] = useState('');
  const [backupDone, setBackupDone] = useState(false);
  const [starting, setStarting] = useState(false);
  const [progress, setProgress] = useState<{phase: string; fraction: number | undefined}>();
  const [deleting, setDeleting] = useState<BackupSummary>();
  const [picking, setPicking] = useState(false);
  const [confirming, setConfirming] = useState<RestoreTarget>();
  const [staging, setStaging] = useState(false);
  const [restarting, setRestarting] = useState(false);
  const [restoreError, setRestoreError] = useState('');
  const waiters = useRef<AbortController[]>([]);
  useEffect(() => () => {
    for (const ctl of waiters.current) ctl.abort();
    waiters.current = [];
  }, []);
  const track = (ctl: AbortController) => {
    waiters.current.push(ctl);
    return ctl;
  };

  // While `running` is present, poll GET every 2 s for the phase and the byte progress;
  // the waiter resolves when `running` clears, which stops the polling.
  const watching = useRef(false);
  const running = doc?.running;
  useEffect(() => {
    if (!running || watching.current) return;
    watching.current = true;
    const ctl = track(new AbortController());
    setProgress({phase: running.phase, fraction: backupProgressFraction(running)});
    void waitForBackupCompletion(() => getBackups(api, serverId, ctl.signal), {
      signal: ctl.signal,
      onProgress: next => {
        if (next.running) setProgress({phase: next.running.phase, fraction: backupProgressFraction(next.running)});
      },
    }).then(
      () => {
        watching.current = false;
        if (ctl.signal.aborted) return;
        setProgress(undefined);
        setBackupError('');
        setBackupDone(true);
        read.reload();
      },
      e => {
        watching.current = false;
        if (ctl.signal.aborted || isAuthenticationFailure(e) || (e as {code?: string})?.code === 'cancelled' || (e as {name?: string})?.name === 'AbortError') return;
        setProgress(undefined);
        setBackupError((e as {code?: string})?.code === 'insufficient_disk' ? t('web.maintenance.backupNoSpace') : problem(e, 'action'));
      },
    );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api, serverId, running ? running.jobId : '']);

  const beginBackup = () => {
    if (starting || doc?.running) return;
    setStarting(true);
    setBackupError('');
    setBackupDone(false);
    action.clear();
    const key = 'start-backup';
    startBackup(api, serverId, opIds.forPayload(key))
      .then(() => {
        opIds.release();
        read.reload();
      })
      .catch(e => {
        opIds.release();
        if (!isAuthenticationFailure(e)) setBackupError((e as {code?: string})?.code === 'insufficient_disk' ? t('web.maintenance.backupNoSpace') : problem(e, 'action'));
      })
      .finally(() => setStarting(false));
  };

  const confirmDelete = (backup: BackupSummary) => {
    action.clear();
    setDeleting(backup);
  };

  const refusalCopy = (e: unknown, start: boolean): string => {
    const code = (e as {code?: string})?.code;
    if (typeof code === 'string' && isRestoreRefusalCode(code)) return t(code === 'insufficient_disk' && start ? 'web.maintenance.backupNoSpace' : RESTORE_REFUSAL_MESSAGE_IDS[code]);
    return problem(e, 'action');
  };

  const doRestore = () => {
    const target = confirming;
    if (!target || staging || restarting) return;
    const source = target.kind === 'backup' ? {backupId: target.backup.id} : {path: target.path};
    const key = JSON.stringify(source);
    setStaging(true);
    setRestoreError('');
    restoreBackup(api, serverId, opIds.forPayload(key), source)
      // The parser guarantees {staged: true, restartRequired: true}; anything else
      // arrives as a thrown refusal and is mapped below.
      .then(() => {
        opIds.release();
        setConfirming(undefined);
        setStaging(false);
        setRestarting(true);
        const ctl = track(new AbortController());
        void waitForServerAnswer(() => getBackups(api, serverId, ctl.signal), {signal: ctl.signal}).then(
          () => {
            if (ctl.signal.aborted) return;
            setRestarting(false);
            read.reload();
          },
          e => {
            if (ctl.signal.aborted || (e as {code?: string})?.code === 'cancelled' || (e as {name?: string})?.name === 'AbortError') return;
            // An ended sign-in goes to the sign-in screen through the session layer; anything
            // else is a restore outcome the console says in catalogue copy.
            setRestarting(false);
            if (!isAuthenticationFailure(e)) setRestoreError(refusalCopy(e, false));
          },
        );
      })
      .catch(e => {
        opIds.release();
        setStaging(false);
        if (!isAuthenticationFailure(e)) setRestoreError(refusalCopy(e, false));
      });
  };

  const lastRestore = doc?.lastRestore;
  return (
    <SettingsGroup title={t('web.maintenance.backups')} description={t('web.maintenance.backupsLedePlain')}>
      {read.error ? <div style={{padding: 12}}><Notice tone={doc ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></div> : null}
      {action.error ? <div style={{padding: 12}}><Notice tone="error" compact>{action.error}</Notice></div> : null}
      {backupError ? <div style={{padding: 12}}><Notice tone="error" compact>{backupError}</Notice></div> : null}
      {backupDone && !doc?.running ? <div style={{padding: 12}}><Notice tone="success" compact>{t('web.maintenance.backupComplete')}</Notice></div> : null}
      {restoreError ? <div style={{padding: 12}}><Notice tone="error" compact>{restoreError}</Notice></div> : null}
      {lastRestore ? <div style={{padding: 12}}><RestoreResultNotice result={lastRestore} /></div> : null}
      <div style={{padding: '4px 16px 0'}}><Text variant="caption" tone="tertiary">{t('web.maintenance.backupsOpenFiles')}</Text></div>
      {(starting || progress) && !read.error ? (
        <div style={{padding: '12px 16px 0', display: 'flex', flexDirection: 'column', gap: 8}}>
          <ProgressBar value={progress?.fraction} indeterminate={progress?.fraction === undefined} label={t('web.maintenance.backingUp')} />
          <Text variant="caption" tone="tertiary">{progress?.phase || t('web.maintenance.backingUp')}</Text>
        </div>
      ) : null}
      {restarting ? (
        <div style={{padding: '12px 16px 0', display: 'flex', flexDirection: 'column', gap: 8}}>
          <Loading label={t('web.maintenance.restartingTitle')} />
          <Text variant="caption" tone="tertiary">{t('web.maintenance.restartingBody')}</Text>
        </div>
      ) : null}
      {doc?.backups.map(b => (
        <SettingsRow key={b.id} icon="storage" label={whenBackup(b.createdAt)}
          help={t('web.maintenance.backupDetails', {kind: t(backupKindMessageId(b.kind)), size: bytes(b.bytes), schema: b.schemaVersion, version: b.serverVersion || '—'})}
          meta={<span style={{display: 'inline-flex', alignItems: 'center', gap: 4, minWidth: 0}}><Text variant="mono" tone="tertiary" style={{fontSize: 12, overflow: 'hidden', textOverflow: 'ellipsis'}}>{b.path}</Text><IconButton name="copy" label={t('web.twoStep.copy')} size="sm" variant="ghost" onClick={() => void navigator.clipboard?.writeText(b.path)} /></span>}
          control={<div style={{display: 'flex', gap: 8}}><Button size="sm" variant="secondary" label={t('web.maintenance.restoreBackup')} disabled={starting || !!progress || staging || restarting} onClick={() => { setRestoreError(''); action.clear(); setConfirming({kind: 'backup', backup: b}); }} /><Button size="sm" variant="ghost" icon="trash" label={t('action.delete')} disabled={starting || !!progress || staging || restarting} onClick={() => confirmDelete(b)} /></div>} />
      ))}
      {doc && !doc.backups.length ? <div style={{padding: 16}}><Text variant="caption" tone="tertiary">{t('web.maintenance.noBackups')}</Text></div> : null}
      <div style={{padding: '8px 16px 16px', display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center'}}>
        <Button size="sm" variant="secondary" icon="plus" label={t('web.maintenance.backUpNow')} loading={starting} disabled={starting || !!doc?.running || staging || restarting} onClick={beginBackup} />
        <Button size="sm" variant="ghost" icon="folder" label={t('web.maintenance.restoreFromFile')} disabled={starting || staging || restarting} onClick={() => { setRestoreError(''); action.clear(); setPicking(true); }} />
        <Freshness at={read.at} refreshing={read.loading} onRefresh={read.reload} />
      </div>

      <Dialog open={!!confirming} onOpenChange={o => !o && !staging && setConfirming(undefined)} title={t(confirming?.kind === 'path' ? 'web.maintenance.restoreFileTitle' : 'web.maintenance.restoreTitle')} description={confirming?.kind === 'backup' ? whenBackup(confirming.backup.createdAt) : confirming?.path} width={480}
        actions={<><Button variant="ghost" label={t('action.cancel')} disabled={staging} onClick={() => setConfirming(undefined)} /><Button variant="primary" label={t('web.maintenance.restoreConfirm')} loading={staging} onClick={doRestore} /></>}>
        <Text variant="body">{t('web.maintenance.restoreBody')}</Text>
      </Dialog>
      <ConfirmDialog open={!!deleting} onOpenChange={o => !o && setDeleting(undefined)} title={t('web.maintenance.deleteBackupTitle')} body={t('web.maintenance.deleteBackupBody', {id: deleting?.id ?? ''})} confirmLabel={t('web.maintenance.deleteBackupConfirm')} typedConfirmation={deleting?.id} busy={action.busy} error={action.error}
        onConfirm={() => void action.run(async () => { if (!deleting) return; await deleteBackup(api, serverId, deleting.id); setDeleting(undefined); read.reload(); }, t('web.maintenance.backupDeleted'))} />
      <BackupSourcePicker open={picking} onOpenChange={setPicking} onPick={path => { setRestoreError(''); action.clear(); setConfirming({kind: 'path', path}); }} />
    </SettingsGroup>
  );
}

/** RestoreResultNotice says the `lastRestore` outcome in catalogue copy: "Restored", or
 * "Couldn't restore; your previous data was put back" plus the server's reason. */
export function RestoreResultNotice({result}: {result: LastRestore}) {
  const {t} = useI18n();
  const rolledBack = result.outcome === 'rolled_back';
  return (
    <Notice tone={rolledBack ? 'warning' : 'success'} compact>
      {rolledBack ? (result.reason ? `${t('web.maintenance.restoreRolledBack')}: ${result.reason}` : t('web.maintenance.restoreRolledBack')) : t('web.maintenance.restored')}
    </Notice>
  );
}

/** BackupSourcePicker browses the server's disks for a restore source: a backup folder or
 * a bare `.db` file. Directories navigate; `.db` files select. Anything else is not shown. */
export function BackupSourcePicker({open, onOpenChange, onPick, start = ''}: {open: boolean; onOpenChange: (open: boolean) => void; onPick: (path: string) => void; start?: string}) {
  const {api, system, session} = useSession();
  const serverId = system?.id ?? session?.viewer.serverId ?? '';
  const {t} = useI18n();
  const [path, setPath] = useState(start);
  const [page, setPage] = useState<FilesystemPage>();
  const [selected, setSelected] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [typed, setTyped] = useState(start);
  const [more, setMore] = useState(false);
  useEffect(() => { if (page?.path) setTyped(page.path); }, [page?.path]);
  useEffect(() => { setSelected(''); }, [path]);
  const read = (cursor = ''): Promise<FilesystemPage> => api.request<unknown>(filesystemPath(path, cursor)).then(raw => parseEnvelope(raw, serverId, parseFilesystemPage).result);
  useEffect(() => {
    if (!open) return;
    let active = true;
    setLoading(true);
    setError('');
    read()
      .then(next => { if (active) setPage(next); }, e => { if (active) setError(problem(e)); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, path, api, serverId]);
  const loadMore = () => {
    if (!page?.nextCursor || more) return;
    const current = page;
    setMore(true);
    read(current.nextCursor)
      .then(next => setPage(p => (p === current && next.path === current.path ? mergeFilesystemPages(current, next) : p)), e => setError(problem(e)))
      .finally(() => setMore(false));
  };
  const folders = page?.entries.filter(e => e.kind === 'directory') ?? [];
  const files = page?.entries.filter(e => e.kind === 'file' && isBackupSourceFile(e.path)) ?? [];
  const typedIsFile = isBackupSourceFile(typed.trim());
  const choice = selected || (typedIsFile ? typed.trim() : '');
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title={t('web.maintenance.chooseBackupTitle')} description={t('web.maintenance.chooseBackupLede')} width={560}
      actions={<><Button variant="ghost" label={t('action.cancel')} onClick={() => onOpenChange(false)} /><Button variant="primary" label={t('web.maintenance.useBackupSource')} disabled={!choice && !page?.path} onClick={() => { const next = choice || page?.path; if (next) { onPick(next); onOpenChange(false); } }} /></>}>
      <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
        <form onSubmit={e => { e.preventDefault(); const next = typed.trim(); if (next) setPath(next); }}>
          <Input label={t('web.folderPicker.path')} hideLabel mono value={typed} onChange={e => setTyped(e.target.value)} />
        </form>
        {error ? <Notice tone="error" compact action={{label: t('action.back'), onClick: () => setPath(page?.parent ?? '')}}>{error}</Notice> : null}
        <div style={{display: 'flex', gap: 8, flexWrap: 'wrap'}}>
          {page?.parent && page.parent !== page.path ? <Button size="sm" variant="secondary" icon="back" label={t('web.folderPicker.up')} onClick={() => setPath(page.parent)} /> : null}
          {page?.roots.map(r => <Button key={r.path} size="sm" variant="ghost" label={r.name || r.path} onClick={() => setPath(r.path)} />)}
        </div>
        <Surface padless style={{maxHeight: 340, overflow: 'auto'}}>
          {loading && !page ? <div style={{padding: 16}}><Loading label={t('web.folderPicker.reading')} /></div> : null}
          {folders.map(f => <ListRow key={f.path} icon="folder" title={f.name} meta={!f.readable ? t('web.folderPicker.noAccess') : f.symlink ? t('web.folderPicker.link') : undefined} onClick={f.readable ? () => setPath(f.path) : undefined} />)}
          {files.map(f => <ListRow key={f.path} icon="storage" title={f.name} meta={f.symlink ? t('web.folderPicker.link') : undefined} selected={selected === f.path} onClick={() => setSelected(f.path)} />)}
          {page && !folders.length && !files.length ? <div style={{padding: 16}}><Text variant="caption" tone="tertiary">{t('web.maintenance.noBackupSources')}</Text></div> : null}
        </Surface>
        {page?.nextCursor ? <div><Button size="sm" variant="ghost" label={t('web.folderPicker.more')} loading={more} onClick={loadMore} /></div> : null}
      </div>
    </Dialog>
  );
}
