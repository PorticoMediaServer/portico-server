import {getBackups, getBackupSettings, inspectBackupSource, isAuthenticationFailure, startBackup, updateBackupSettings, waitForBackupCompletion, type BackupSettings, type BackupSettingsPatch, type BackupTransport} from '@core/backups.ts';
import type {BackupsDocument, RestoreSource} from '@core/administration.ts';
import {randomId} from '@core/random-id.ts';

export type SettingsBackupSnapshot = Readonly<{settings?: BackupSettings; document?: BackupsDocument; busy: boolean}>;
const empty: SettingsBackupSnapshot = Object.freeze({busy: false});

/** Owner-scoped backup facts and commands. Secret inputs remain request-local. */
export class SettingsBackupSource {
  private state = empty;
  private readonly lifetime = new AbortController();
  private loading?: Promise<void>;
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
  async save(patch: BackupSettingsPatch): Promise<boolean> {
    if (!this.current() || this.loading || this.state.busy || !this.state.settings) return false;
    this.publish({...this.state, busy: true});
    try {
      const settings = await updateBackupSettings(this.api, this.serverId, patch, this.lifetime.signal);
      if (!this.current()) return false;
      this.publish({...this.state, settings});return true;
    } catch (error) {this.refuse(error);} finally {this.publish({...this.state, busy: false});}
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
  dispose() {this.lifetime.abort();this.loading = undefined;this.state = empty;}
}
