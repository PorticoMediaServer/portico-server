import {deleteBackup, getBackups, getBackupSettings, inspectBackupSource, isAuthenticationFailure, restoreBackup, startBackup, updateBackupSettings, waitForBackupCompletion, type BackupSettings, type BackupSettingsPatch, type BackupTransport} from '@core/backups.ts';
import type {BackupsDocument, RestoreSource, RestoreStaged} from '@core/administration.ts';
import {randomId} from '@core/random-id.ts';

export type SettingsBackupSnapshot = Readonly<{settings?: BackupSettings; document?: BackupsDocument; staged?: RestoreStaged; busy: boolean}>;
export type SettingsRestoreSelection = Readonly<{encrypted: boolean; isCurrent: () => boolean}>;
const empty: SettingsBackupSnapshot = Object.freeze({busy: false});

/** Owner-scoped backup facts and commands. Secret inputs remain request-local. */
export class SettingsBackupSource {
  private state = empty;
  private readonly lifetime = new AbortController();
  private loading?: Promise<void>;
  private candidate?: AbortController;
  private readonly selections = new WeakMap<SettingsRestoreSelection, Readonly<{source: RestoreSource; signal: AbortSignal}>>();
  constructor(private readonly api: BackupTransport, private readonly serverId: string, private readonly isCurrent: () => boolean, private readonly notify: () => void) {}
  private current = () => !this.lifetime.signal.aborted && this.isCurrent();
  getSnapshot = () => this.current() ? this.state : empty;
  private publish(state: SettingsBackupSnapshot) {if (this.current()) {this.state = Object.freeze(state);this.notify();}}
  private requireCurrent() {if (!this.current()) throw new DOMException('The backup scope ended.', 'AbortError');}
  private refuse(error: unknown): never {
    if (isAuthenticationFailure(error)) {this.dispose();this.notify();}
    throw error;
  }
  load = (): Promise<void> => {
    if (this.lifetime.signal.aborted) return Promise.reject(new DOMException('The backup scope ended.', 'AbortError'));
    if (!this.current() || this.state.busy) return Promise.resolve();
    if (this.loading) return this.loading;
    const task = Promise.all([getBackupSettings(this.api, this.serverId, this.lifetime.signal), getBackups(this.api, this.serverId, this.lifetime.signal)]).then(([settings, document]) => {
      this.publish({...this.state, settings, document});
    }).catch(error => this.refuse(error)).finally(() => {if (this.loading === task) this.loading = undefined;});
    this.loading = task;return task;
  };
  async save(patch: BackupSettingsPatch, signal?: AbortSignal): Promise<boolean> {
    const requestSignal = signal ? AbortSignal.any([signal, this.lifetime.signal]) : this.lifetime.signal;
    if (!this.current() || requestSignal.aborted || this.loading || this.state.busy || !this.state.settings) return false;
    this.publish({...this.state, busy: true});
    try {
      requestSignal.throwIfAborted();
      const settings = await updateBackupSettings(this.api, this.serverId, patch, requestSignal);
      if (!this.current() || requestSignal.aborted) return false;
      this.publish({...this.state, settings});return true;
    } catch (error) {if (isAuthenticationFailure(error) || !requestSignal.aborted) this.refuse(error);} finally {this.publish({...this.state, busy: false});}
    return false;
  }
  async backupNow(options: Readonly<{intervalMs?: number; timeoutMs?: number}> = {}): Promise<boolean> {
    if (!this.current() || this.loading || this.state.busy || !this.state.document || this.state.document.running || this.state.document.restore) return false;
    this.publish({...this.state, busy: true});
    try {
      const jobId = await startBackup(this.api, this.serverId, randomId(), this.lifetime.signal);
      this.requireCurrent();
      const document = await waitForBackupCompletion(() => {
        this.requireCurrent();return getBackups(this.api, this.serverId, this.lifetime.signal);
      }, {...options, expectedJobId: jobId, signal: this.lifetime.signal, onProgress: document => this.publish({...this.state, document})});
      if (!this.current()) return false;
      this.publish({...this.state, document});return true;
    } catch (error) {this.refuse(error);} finally {this.publish({...this.state, busy: false});}
  }
  async inspect(source: RestoreSource, signal: AbortSignal) {
    this.requireCurrent();
    if ('backupId' in source && !this.state.document?.backups.some(backup => backup.id === source.backupId)) throw new Error('The selected backup is no longer listed.');
    const selected = Object.freeze('backupId' in source ? {backupId: source.backupId} : {path: source.path});
    const request = new AbortController(), abort = () => request.abort();
    this.lifetime.signal.addEventListener('abort', abort, {once: true});signal.addEventListener('abort', abort, {once: true});
    if (signal.aborted) request.abort();
    try {
      request.signal.throwIfAborted();
      const inspection = await inspectBackupSource(this.api, this.serverId, selected, request.signal);
      this.requireCurrent();request.signal.throwIfAborted();return inspection;
    } catch (error) {this.refuse(error);} finally {this.lifetime.signal.removeEventListener('abort', abort);signal.removeEventListener('abort', abort);}
  }
  /** Inspection establishes the exact candidate. A later command cannot retarget its path or ID. */
  async prepareRestore(source: RestoreSource, signal: AbortSignal): Promise<SettingsRestoreSelection> {
    this.requireCurrent();this.cancelRestoreSelection();
    const candidate = new AbortController();this.candidate = candidate;
    const selected = Object.freeze('backupId' in source ? {backupId: source.backupId} : {path: source.path});
    const request = this.linkSignals([signal, candidate.signal]);
    try {
      const inspection = await this.inspect(selected, request.signal);
      request.signal.throwIfAborted();this.requireCurrent();
      const selection: SettingsRestoreSelection = Object.freeze({encrypted: inspection.encrypted,
        isCurrent: () => this.current() && this.candidate === candidate && !candidate.signal.aborted && !signal.aborted &&
          (!('backupId' in selected) || !!this.state.document?.backups.some(backup => backup.id === selected.backupId)),
      });
      this.selections.set(selection, {source: selected, signal});return selection;
    } finally {request.dispose();}
  }
  async restore(selection: SettingsRestoreSelection, passphrase: string | undefined, signal: AbortSignal): Promise<RestoreStaged | undefined> {
    this.requireCurrent();
    const captured = this.selections.get(selection);
    if (!captured || !selection.isCurrent()) throw new DOMException('The restore selection ended.', 'AbortError');
    if (this.loading || this.state.busy || this.state.document?.running || this.state.document?.restore || this.state.staged) return undefined;
    if (selection.encrypted ? typeof passphrase !== 'string' || !passphrase.length : passphrase !== undefined) throw new Error('Invalid restore passphrase input.');
    const request = this.linkSignals([captured.signal, signal, this.candidate!.signal]);
    this.publish({...this.state, busy: true});
    try {
      request.signal.throwIfAborted();
      const staged = await restoreBackup(this.api, this.serverId, randomId(), captured.source, request.signal, passphrase);
      this.requireCurrent();request.signal.throwIfAborted();
      if (!selection.isCurrent()) throw new DOMException('The restore selection ended.', 'AbortError');
      this.publish({...this.state, staged});return staged;
    } catch (error) {this.refuse(error);} finally {request.dispose();this.publish({...this.state, busy: false});}
  }
  /** Delete one backup the owner chose from the current list; an unlisted id is refused. */
  async remove(backupId: string, signal: AbortSignal): Promise<boolean> {
    this.requireCurrent();
    const document = this.state.document;
    if (!document?.backups.some(backup => backup.id === backupId)) throw new Error('The selected backup is no longer listed.');
    if (this.loading || this.state.busy || document.running || document.restore) return false;
    const request = this.linkSignals([signal]);
    this.publish({...this.state, busy: true});
    try {
      request.signal.throwIfAborted();
      await deleteBackup(this.api, this.serverId, backupId, request.signal);
      this.requireCurrent();request.signal.throwIfAborted();
      const document = await getBackups(this.api, this.serverId, this.lifetime.signal);
      if (!this.current()) return false;
      this.publish({...this.state, document});return true;
    } catch (error) {this.refuse(error);} finally {request.dispose();this.publish({...this.state, busy: false});}
  }
  cancelRestoreSelection() {this.candidate?.abort();this.candidate = undefined;}  private linkSignals(signals: readonly AbortSignal[]) {
    const request = new AbortController(), abort = () => request.abort(), all = [this.lifetime.signal, ...signals];
    for (const signal of all) {signal.addEventListener('abort', abort, {once: true});if (signal.aborted) request.abort();}
    return {signal: request.signal, dispose: () => {for (const signal of all) signal.removeEventListener('abort', abort);}};
  }
  dispose() {this.lifetime.abort();this.cancelRestoreSelection();this.loading = undefined;this.state = empty;}
}
