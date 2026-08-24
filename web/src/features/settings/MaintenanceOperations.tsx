import type { BackupInfo, ScheduledTask, SystemReleaseInfo, SystemStorageReport } from '@porticomediaserver/client-core';
import { AlertTriangle, ArchiveRestore, CheckCircle2, Clock3, DatabaseBackup, HardDrive, Play, RefreshCw } from '#portico-icons';
import { useCallback, useEffect, useRef, useState } from 'react';
import { PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { reviewedProductErrorText } from '../../components/ProductLanguage';
import { InlineNotice, SettingsGroup, ToggleControl } from './SettingsControls';
import { useAbortableMutation } from './settingsHooks';
import type { RestoreWorkflowResponse, SettingsDataSource, SettingsOperationalSnapshot } from './settingsTypes';

function bytes(value: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = Math.max(0, value);
  let index = 0;
  while (size >= 1024 && index < units.length - 1) { size /= 1024; index += 1; }
  return `${size >= 10 || index === 0 ? size.toFixed(0) : size.toFixed(1)} ${units[index]}`;
}

const RESTORE_PENDING_STORAGE_KEY = 'portico.restore.pending.v1';
const RESTORE_LAST_STORAGE_KEY = 'portico.restore.last.v1';

export type RestoreCapability = {
  operationId: string;
  statusToken: string;
  name: string;
  state?: string;
  phase?: string;
  progress?: number;
  instruction?: string;
  recoveryRequired?: boolean;
};

function readRestoreStorage(key: string): RestoreCapability | undefined {
  if (typeof window === 'undefined') return undefined;
  try {
    const value = JSON.parse(window.sessionStorage.getItem(key) ?? 'null') as Partial<RestoreCapability> | null;
    if (!value || typeof value.operationId !== 'string' || typeof value.statusToken !== 'string' || typeof value.name !== 'string') return undefined;
    if (!value.operationId || !value.statusToken || !value.name) return undefined;
    return {
      operationId: value.operationId,
      statusToken: value.statusToken,
      name: value.name,
      state: typeof value.state === 'string' ? value.state : undefined,
      phase: typeof value.phase === 'string' ? value.phase : undefined,
      progress: typeof value.progress === 'number' ? value.progress : undefined,
      instruction: typeof value.instruction === 'string' ? value.instruction : undefined,
      recoveryRequired: value.recoveryRequired === true,
    };
  } catch {
    return undefined;
  }
}

function writeRestoreStorage(key: string, value: RestoreCapability): void {
  if (typeof window === 'undefined') return;
  try { window.sessionStorage.setItem(key, JSON.stringify(value)); } catch { /* private storage may be unavailable */ }
}

function clearRestoreStorage(key: string): void {
  if (typeof window === 'undefined') return;
  try { window.sessionStorage.removeItem(key); } catch { /* private storage may be unavailable */ }
}

function waitForRestoreDelay(milliseconds: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) { reject(signal.reason ?? new DOMException('Restore polling was cancelled.', 'AbortError')); return; }
    let timer = 0;
    const onAbort = () => {
      window.clearTimeout(timer);
      signal.removeEventListener('abort', onAbort);
      reject(signal.reason ?? new DOMException('Restore polling was cancelled.', 'AbortError'));
    };
    timer = window.setTimeout(() => {
      signal.removeEventListener('abort', onAbort);
      resolve();
    }, milliseconds);
    signal.addEventListener('abort', onAbort, { once: true });
  });
}

function restoreStatusErrorStatus(reason: unknown): number | undefined {
  if (!reason || typeof reason !== 'object') return undefined;
  const status = (reason as { status?: unknown }).status;
  return typeof status === 'number' && Number.isFinite(status) ? status : undefined;
}

function isRetryableRestoreStatusError(reason: unknown): boolean {
  const status = restoreStatusErrorStatus(reason);
  if (status === undefined) return reason instanceof TypeError;
  return status === 0 || [408, 425, 429, 500, 502, 503, 504].includes(status);
}

function isInvalidRestoreCapabilityError(reason: unknown): boolean {
  const status = restoreStatusErrorStatus(reason);
  return status === 401 || status === 404;
}

function isRestoreTerminalState(state?: string): boolean {
  return state === 'complete' || state === 'failed' || state === 'recovery-required';
}

export type RestoreStatusPollOptions = {
  onUpdate?: (response: RestoreWorkflowResponse) => void;
  onRetry?: (response: RestoreWorkflowResponse, reason: unknown) => void;
  sleep?: (milliseconds: number, signal: AbortSignal) => Promise<void>;
};

/**
 * Polls the capability-bound status route without treating a missing echoed
 * token or a transient host/network failure as a terminal restore state. The
 * capability remains caller-owned (normally sessionStorage) for the entire
 * loop, including reload/resume.
 */
export async function pollRestoreStatus(
  source: Pick<SettingsDataSource, 'restoreStatus'>,
  initial: RestoreWorkflowResponse,
  capability: RestoreCapability,
  signal: AbortSignal,
  options: RestoreStatusPollOptions = {},
): Promise<RestoreWorkflowResponse> {
  const sleep = options.sleep ?? waitForRestoreDelay;
  let current = initial;
  let transientDelay = 1_000;
  while (true) {
    options.onUpdate?.(current);
    if (isRestoreTerminalState(current.state) || !capability.operationId || !capability.statusToken) return current;
    await sleep(1_000, signal);
    while (true) {
      try {
        current = await source.restoreStatus(capability.operationId, capability.statusToken, signal);
        transientDelay = 1_000;
        break;
      } catch (reason) {
        if (signal.aborted || !isRetryableRestoreStatusError(reason)) throw reason;
        options.onRetry?.(current, reason);
        await sleep(transientDelay, signal);
        transientDelay = Math.min(30_000, transientDelay * 2);
      }
    }
  }
}

function TasksPanel({ tasks, source, onChanged }: { tasks: ScheduledTask[]; source: SettingsDataSource; onChanged: () => void }) {
  const mutation = useAbortableMutation();
  const [feedback, setFeedback] = useState('');
  const [error, setError] = useState('');
  const update = async (task: ScheduledTask, enabled: boolean) => {
    setFeedback(''); setError('');
    try { await mutation.run((signal) => source.updateScheduledTask(task.id, { enabled }, signal)); setFeedback(`${task.title} ${enabled ? 'enabled' : 'disabled'}.`); onChanged(); }
    catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: `update ${task.title}` })); }
  };
  const run = async (task: ScheduledTask) => {
    setFeedback(''); setError('');
    try { const response = await mutation.run((signal) => source.runScheduledTask(task.id, signal)); setFeedback(`${task.title} queued ${response.jobs.length} ${response.jobs.length === 1 ? 'job' : 'jobs'}.`); onChanged(); }
    catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: `start ${task.title}` })); }
  };
  return <SettingsGroup title="Scheduled tasks" description="Individual maintenance jobs and their current schedules.">
    {(feedback || error) && <InlineNotice tone={error ? 'error' : 'success'}>{error || feedback}</InlineNotice>}
    {tasks.length === 0 ? <div className="portico-settings-state"><Clock3 /><strong>No scheduled tasks</strong><p>This server did not report any runnable maintenance tasks.</p></div> : <div className="portico-task-list">{tasks.map((task) => <article key={task.id}><span className={`portico-task-icon ${task.running ? 'running' : ''}`}>{task.running ? <RefreshCw className="portico-settings-spinner" /> : <Clock3 />}</span><span><strong>{task.title}</strong><small>{task.description} · {task.schedule}{task.lastJob ? ` · last ${task.lastJob.status}` : ''}</small></span><div><label className="portico-inline-toggle"><span>Enabled</span><ToggleControl label={`Enable ${task.title}`} value={task.enabled} disabled={mutation.busy} onChange={(enabled) => void update(task, enabled)} /></label><SecondaryButton disabled={mutation.busy || task.running} onClick={() => void run(task)}><Play />{task.running ? 'Running' : 'Run now'}</SecondaryButton></div></article>)}</div>}
  </SettingsGroup>;
}

function BackupsPanel({ backups, source, onChanged }: { backups: BackupInfo[]; source: SettingsDataSource; onChanged: () => void }) {
  const mutation = useAbortableMutation();
  const [confirmRestore, setConfirmRestore] = useState('');
  const [password, setPassword] = useState('');
  const [importFile, setImportFile] = useState<File | undefined>();
  const [feedback, setFeedback] = useState('');
  const [error, setError] = useState('');
  const [operation, setOperation] = useState<{ name: string; phase: string; progress: number; instruction: string; recoveryRequired?: boolean }>();
  const pollingOperation = useRef<string | undefined>(undefined);
  const mounted = useRef(true);

  const rememberRestore = useCallback((response: RestoreWorkflowResponse): RestoreCapability | undefined => {
    if (!response.operationId || !response.statusToken) return undefined;
    const value = { operationId: response.operationId, statusToken: response.statusToken, name: response.name };
    writeRestoreStorage(RESTORE_PENDING_STORAGE_KEY, value);
    return value;
  }, []);

  const waitForRestore = useCallback(async (initial: RestoreWorkflowResponse, capability: RestoreCapability, signal: AbortSignal) => {
    const final = await pollRestoreStatus(source, initial, capability, signal, {
      onUpdate: (current) => {
        if (mounted.current) setOperation({ name: current.name || capability.name, phase: current.phase || current.state, progress: current.progress ?? 0, instruction: current.instruction, recoveryRequired: current.recoveryRequired || current.state === 'recovery-required' });
      },
      onRetry: (current) => {
        if (mounted.current) setFeedback(`Restore ${current.phase || current.state || 'operation'} is still in progress; retrying status…`);
      },
    });
    if (capability.operationId && capability.statusToken && isRestoreTerminalState(final.state)) {
      const statusRecord = {
        operationId: capability.operationId,
        statusToken: capability.statusToken,
        name: final.name || capability.name,
        state: final.state,
        phase: final.phase,
        progress: final.progress,
        instruction: final.instruction,
        recoveryRequired: final.recoveryRequired || final.state === 'recovery-required',
      };
      clearRestoreStorage(RESTORE_PENDING_STORAGE_KEY);
      writeRestoreStorage(RESTORE_LAST_STORAGE_KEY, statusRecord);
    }
    return final;
  }, [source]);

  useEffect(() => {
    mounted.current = true;
    const pending = readRestoreStorage(RESTORE_PENDING_STORAGE_KEY);
    const last = readRestoreStorage(RESTORE_LAST_STORAGE_KEY);
    if (last) {
      setOperation({
        name: last.name,
        phase: last.phase || last.state || 'complete',
        progress: last.progress ?? (last.state === 'complete' ? 100 : 0),
        instruction: last.instruction || (last.state === 'complete' ? 'The restore completed. Sign in again to continue.' : 'The restore reached a terminal state.'),
        recoveryRequired: last.recoveryRequired || last.state === 'recovery-required',
      });
      clearRestoreStorage(RESTORE_LAST_STORAGE_KEY);
    }
    if (!pending || pollingOperation.current) return () => { mounted.current = false; };
    const controller = new AbortController();
    pollingOperation.current = pending.operationId;
    const resumePlaceholder: RestoreWorkflowResponse = {
      ok: false,
      name: pending.name,
      operationId: pending.operationId,
      state: (pending.state as RestoreWorkflowResponse['state']) || 'validating',
      phase: pending.phase as RestoreWorkflowResponse['phase'],
      progress: pending.progress,
      instruction: pending.instruction || 'Resuming restore status.',
      recoveryRequired: pending.recoveryRequired === true,
    };
    void waitForRestore(resumePlaceholder, pending, controller.signal)
      .then((response) => {
        if (!mounted.current) return;
        setFeedback(response.instruction || `Restore ${response.state}.`);
        if (response.state === 'recovery-required' || response.recoveryRequired) setError('Portico requires supervised recovery before normal service can resume.');
        if (response.state === 'complete' && typeof window !== 'undefined') window.setTimeout(() => window.location.reload(), 250);
      })
      .catch((reason) => {
        if (!mounted.current || controller.signal.aborted) return;
        if (isInvalidRestoreCapabilityError(reason)) clearRestoreStorage(RESTORE_PENDING_STORAGE_KEY);
        setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'resume restore status' }));
      })
      .finally(() => { if (pollingOperation.current === pending.operationId) pollingOperation.current = undefined; });
    return () => { mounted.current = false; controller.abort(); };
  }, [source, waitForRestore]);
  const create = async () => {
    setFeedback(''); setError('');
    try { const backup = await mutation.run((signal) => source.createBackup(signal)); setFeedback(`${backup.name} created.`); onChanged(); }
    catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'create a backup' })); }
  };
  const restore = async (backup: BackupInfo) => {
    setFeedback(''); setError(''); setOperation(undefined);
    try {
      const final = await mutation.run(async (signal) => {
        const response = await source.restoreBackup(backup.name, password, `restore:${backup.name}`, signal);
        const capability = rememberRestore(response);
        if (!capability) throw new Error('The restore did not return a status capability.');
        pollingOperation.current = response.operationId;
        return waitForRestore(response, capability, signal);
      });
      pollingOperation.current = undefined;
      setConfirmRestore(''); setPassword('');
      setFeedback(final.instruction || `Restore ${final.state}.`);
      if (final.state === 'recovery-required' || final.recoveryRequired) setError('Portico requires supervised recovery before normal service can resume.');
      if (final.state === 'complete' && typeof window !== 'undefined') window.setTimeout(() => window.location.reload(), 250);
      onChanged();
    } catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: `restore ${backup.name}` })); }
  };
  const upload = async () => {
    if (!importFile) return;
    setFeedback(''); setError(''); setOperation(undefined);
    try {
      const final = await mutation.run(async (signal) => {
        const response = await source.restoreUploadedDatabase(importFile, password, 'restore:uploaded-database', signal);
        const capability = rememberRestore(response);
        if (!capability) throw new Error('The database import did not return a status capability.');
        pollingOperation.current = response.operationId;
        return waitForRestore(response, capability, signal);
      });
      pollingOperation.current = undefined;
      setImportFile(undefined); setPassword('');
      setFeedback(final.instruction || `Database import ${final.state}.`);
      if (final.state === 'recovery-required' || final.recoveryRequired) setError('Portico requires supervised recovery before normal service can resume.');
      if (final.state === 'complete' && typeof window !== 'undefined') window.setTimeout(() => window.location.reload(), 250);
      onChanged();
    } catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'import a database' })); }
  };
  return <SettingsGroup title="Backups" description="Verified SQLite backups and supervised database restore." actions={<PrimaryButton disabled={mutation.busy} onClick={() => void create()}><DatabaseBackup />{mutation.busy ? 'Working…' : 'Back up now'}</PrimaryButton>}>
    {(feedback || error) && <InlineNotice tone={error ? 'error' : operation?.recoveryRequired ? 'warn' : 'success'}>{error || feedback}</InlineNotice>}
    {operation && !error && <InlineNotice tone="warn">{operation.phase} · {operation.progress}% · {operation.instruction}</InlineNotice>}
    <div className="portico-inline-confirm">
      <label>Account password <input type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" /></label>
      <label>Import database <input type="file" accept=".db,application/vnd.sqlite3,application/octet-stream" onChange={(event) => setImportFile(event.target.files?.[0])} /></label>
      <button type="button" className="danger" disabled={mutation.busy || !importFile || !password} onClick={() => void upload()}>Verify and import</button>
    </div>
    {backups.length === 0 ? <div className="portico-settings-state"><DatabaseBackup /><strong>No backups found</strong><p>Create a backup before changing storage or performing server maintenance.</p></div> : <div className="portico-backup-list">{backups.map((backup) => <article key={backup.name}><span className={backup.integrity === 'ok' ? 'healthy' : 'danger'}>{backup.integrity === 'ok' ? <CheckCircle2 /> : <AlertTriangle />}</span><span><strong>{backup.name}</strong><small>{new Date(backup.createdAt).toLocaleString()} · {bytes(backup.sizeBytes)} · {backup.manifestPresent ? 'verified manifest' : `not restore-ready${backup.validationCode ? ` · ${backup.validationCode}` : ''}`}</small></span><div>{backup.restoreReady ? (confirmRestore === backup.name ? <div className="portico-inline-confirm"><span>Confirm <code>restore:{backup.name}</code></span><button type="button" onClick={() => setConfirmRestore('')}>Cancel</button><button type="button" className="danger" disabled={mutation.busy || !password} onClick={() => void restore(backup)}>Restore</button></div> : <SecondaryButton disabled={mutation.busy} onClick={() => setConfirmRestore(backup.name)}><ArchiveRestore /> Restore</SecondaryButton>) : <span className="portico-settings-capability">Not restore-ready</span>}</div></article>)}</div>}
  </SettingsGroup>;
}

function StoragePanel({ storage }: { storage: SystemStorageReport }) {
  return <SettingsGroup title="Storage" description={`Portico currently manages ${bytes(storage.totalBytes)} across the paths below.`}>
    <div className="portico-storage-list">{storage.categories.map((category) => <article key={category.key}><HardDrive /><span><strong>{category.label}</strong><small>{category.available ? `${bytes(category.sizeBytes)} · ${category.fileCount} ${category.fileCount === 1 ? 'file' : 'files'} · ${category.writable ? 'writable' : 'read only'}` : category.error || 'Unavailable'}</small></span>{category.cleanupSupported && <span className="portico-settings-capability configured">Cleanup supported</span>}</article>)}</div>
  </SettingsGroup>;
}

function UpdaterPanel({ release }: { release?: SystemReleaseInfo }) {
  const installMethod = release?.installMethod?.trim() || 'Installation method not reported';
  const status = release?.updateStatus || 'unavailable';
  return <SettingsGroup title="Server updates" description="Update controls appear only when this installed distribution advertises a verified updater.">
    <div className="portico-settings-actions portico-update-status" role="status">
      <span className="portico-setting-readonly"><strong>Updates unavailable</strong><small>{installMethod} · release state: {status}</small></span>
      <span className="portico-setting-readonly">Use the documented update procedure for this installation.</span>
    </div>
  </SettingsGroup>;
}

export function MaintenanceOperations({ tasks, backups, storage, release, failures, source, onChanged }: { tasks: ScheduledTask[]; backups: BackupInfo[]; storage: SystemStorageReport; release?: SystemReleaseInfo; failures?: SettingsOperationalSnapshot['failures']; source: SettingsDataSource; onChanged: () => void }) {
  const unavailable = (title: string) => <SettingsGroup title={title} description="This panel could not be refreshed independently."><div className="portico-settings-state error"><AlertTriangle /><strong>{title} are unavailable</strong><p>Retry the failed panel before making changes. No empty result is being inferred.</p></div></SettingsGroup>;
  return <div className="portico-settings-form">
    {failures?.release ? unavailable('Server updates') : <UpdaterPanel release={release} />}
    {failures?.tasks ? unavailable('Scheduled tasks') : <TasksPanel tasks={tasks} source={source} onChanged={onChanged} />}
    {failures?.backups ? unavailable('Backups') : <BackupsPanel backups={backups} source={source} onChanged={onChanged} />}
    {failures?.storage ? unavailable('Storage') : <StoragePanel storage={storage} />}
  </div>;
}
