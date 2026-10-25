import {decodeGetAdminStorageUsageResponse} from '../../../../../packages/contracts/src/decoders.ts';
import {routePath} from '../../../../../packages/contracts/src/routes.ts';

export type StorageCategory = Readonly<{
  id: string; name: string; description: string; bytes: number; bytesText: string;
  cleanable: boolean; regenerable: boolean; present: boolean; fileCount: number; oldestDays: number; retentionKey: string;
}>;
export type StorageReport = Readonly<{
  stateDirectory: string; totalBytes: number; totalBytesText: string; truncated: boolean; categories: readonly StorageCategory[];
}>;
export type StorageSnapshot = Readonly<{phase: 'idle' | 'loading' | 'ready' | 'error'}> & Readonly<{report?: StorageReport; error?: boolean}>;

const idleStorageSnapshot: StorageSnapshot = Object.freeze({phase: 'idle'});

export type StorageTransport = Readonly<{request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T>}>;

/** The generated decoder owns the wire shape; this keeps only the drawn facts. */
export function storageReport(value: unknown, serverId: string): StorageReport {
  const decoded = decodeGetAdminStorageUsageResponse(value);
  if (decoded.serverId !== serverId) throw Object.assign(new Error('other_server'), {code: 'other_server'});
  const result = decoded.result;
  return Object.freeze({stateDirectory: result.stateDirectory, totalBytes: result.totalBytes, totalBytesText: result.totalBytesText, truncated: result.truncated,
    categories: Object.freeze(result.categories.map(category => Object.freeze({id: category.id, name: category.name, description: category.description, bytes: category.bytes,
      bytesText: category.bytesText, cleanable: category.cleanable, regenerable: category.regenerable, present: category.present, fileCount: category.fileCount,
      oldestDays: category.oldestDays, retentionKey: category.retentionKey}))) });
}

/** One selected-server owner read, shared by every storage row on the page. */
export class SettingsStorageSource {
  private state: StorageSnapshot = idleStorageSnapshot;
  private readonly listeners = new Set<() => void>();
  private readonly lifetime = new AbortController();
  private reading?: Promise<void>;
  constructor(private readonly transport: StorageTransport, private readonly serverId: string, private readonly isCurrent: () => boolean) {}
  getSnapshot = (): StorageSnapshot => this.current() ? this.state : idleStorageSnapshot;
  subscribe = (listener: () => void): (() => void) => {this.listeners.add(listener); return () => {this.listeners.delete(listener);};};
  private current() {return !this.lifetime.signal.aborted && this.isCurrent();}
  private publish(state: StorageSnapshot) {if (this.current()) {this.state = Object.freeze(state);for (const listener of [...this.listeners]) listener();}}
  load = (): Promise<void> => {
    if (!this.current() || this.reading || this.state.phase === 'ready') return this.reading ?? Promise.resolve();
    const task = Promise.resolve().then(async () => {
      this.publish({phase: 'loading'});
      if (!this.current()) return;
      try {
        const value = await this.transport.request<unknown>(routePath('get_admin_storage_usage'), 'GET', undefined, this.lifetime.signal);
        if (!this.current()) return;
        this.publish({phase: 'ready', report: storageReport(value, this.serverId)});
      } catch {if (this.current() && !this.lifetime.signal.aborted) this.publish({phase: 'error', error: true});}
      finally {this.reading = undefined;}
    });
    this.reading = task;
    return task;
  };
  retry = (): Promise<void> => {this.state = idleStorageSnapshot;return this.load();};
  dispose(): void {if (!this.lifetime.signal.aborted) {this.lifetime.abort();this.listeners.clear();this.state = idleStorageSnapshot;}}
}
