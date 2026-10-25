import {encodedRoutePath, routeBody, routePath} from '../../contracts/src/routes.ts';
import type {ContentScope, LibraryContentApi} from './library-content.ts';

/** Imported configuration is write-only. Only receipt identity survives a command. */
export type ImportedSourceDraft = Readonly<{name: string; remote: string; config: string}>;
export type ImportedSourceReceipt = Readonly<{operationId: string; accepted: true; sourceId: string; generation: number; removed: boolean}>;
export type ImportedSourceOperation = Readonly<{operationId: string; sourceId?: string; expectedGeneration?: number; remove: boolean}>;
const record = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const text = (value: unknown, max: number): value is string => typeof value === 'string' && value.length > 0 && value.length <= max && !/[\x00-\x1f\x7f]/.test(value);
const fail = (code: string): never => {throw Object.assign(new Error(code), {code});};
const terminal = (error: unknown) => record(error) && (error.status === 401 || error.status === 403 || error.statusCode === 401 || error.statusCode === 403
  || error.code === 'unauthorized' || error.code === 'forbidden' || error.code === 'not_permitted');
const codes = new Set(['unauthorized', 'forbidden', 'not_permitted', 'invalid_request', 'source_credentials_required', 'storage_command_conflict',
  'storage_operation_expired', 'storage_command_capacity', 'storage_allocation_recovery_required', 'request_in_progress', 'not_found', 'administration_unavailable']);

export class ImportedSourceService {
  private disposed = false;
  private busy = false;
  private viewerFence?: string;
  private controllers = new Set<AbortController>();
  private pending?: ImportedSourceOperation;
  constructor(private readonly api: LibraryContentApi, private readonly scope: ContentScope,
    private readonly operationId: () => Promise<string>, private readonly isCurrent: () => boolean) {
    if (!scope.serverId || !scope.viewerId) fail('forbidden');
  }
  getOperation = (): ImportedSourceOperation | undefined => this.pending;
  private admit(guard: () => boolean) {if (this.disposed || !this.isCurrent() || !guard()) fail('remote_source_changed');}
  private fence(raw: unknown) {
    if (!record(raw) || !record(raw.scope) || raw.scope.serverId !== this.scope.serverId || !text(raw.scope.viewerFence, 256)
      || this.viewerFence !== undefined && raw.scope.viewerFence !== this.viewerFence) fail('remote_source_changed');
    this.viewerFence = raw.scope.viewerFence;
  }
  private async request(path: string, method: string, body: unknown, guard: () => boolean): Promise<unknown> {
    this.admit(guard);
    const controller = new AbortController(); this.controllers.add(controller);
    let timeout: ReturnType<typeof setTimeout> | undefined;
    try {
      const raw = await Promise.race([this.api.request(path, method, body, controller.signal), new Promise<never>((_, reject) => {
        timeout = setTimeout(() => {controller.abort(); reject(new Error('timeout'));}, 45000);
      })]);
      this.admit(guard); this.fence(raw); return raw;
    } catch (error) {
      if (terminal(error)) this.dispose();
      if (record(error) && typeof error.code === 'string' && codes.has(error.code)) fail(error.code);
      fail('remote_invalid_configuration');
    } finally {if (timeout !== undefined) clearTimeout(timeout); this.controllers.delete(controller); body = undefined;}
  }
  private async flight<T>(guard: () => boolean, action: () => Promise<T>): Promise<T> {
    this.admit(guard); if (this.busy) fail('request_in_progress'); this.busy = true;
    try {return await action();} finally {this.busy = false;}
  }
  async providers(guard: () => boolean): Promise<readonly string[]> {
    return this.flight(guard, async () => {
      const raw = await this.request(routePath('get_storage_source_rclone_providers'), 'GET', undefined, guard);
      if (!record(raw) || !Array.isArray(raw.providers) || raw.providers.length > 1024 || !raw.providers.every(value => text(value, 256))) fail('remote_invalid_configuration');
      return Object.freeze([...new Set(raw.providers as string[])]);
    });
  }
  async test(input: Pick<ImportedSourceDraft, 'remote' | 'config'>, guard: () => boolean): Promise<Readonly<{version: string}>> {
    return this.flight(guard, async () => {
      if (this.pending) fail('request_in_progress');
      const body = routeBody('test_storage_source_rclone', {remote: input.remote, config: input.config}); input = {remote: '', config: ''};
      const raw = await this.request(routePath('test_storage_source_rclone'), 'POST', body, guard);
      if (!record(raw) || !record(raw.result) || !text(raw.result.version, 256)) fail('remote_invalid_configuration');
      return Object.freeze({version: raw.result.version});
    });
  }
  private receipt(raw: unknown, pending: ImportedSourceOperation): ImportedSourceReceipt {
    if (!record(raw) || !record(raw.receipt)) fail('remote_invalid_configuration');
    const r = raw.receipt;
    if (r.operationId !== pending.operationId || r.accepted !== true || !text(r.sourceId, 256) || !Number.isSafeInteger(r.generation)
      || (r.generation as number) < 1 || r.removed !== undefined && typeof r.removed !== 'boolean'
      || pending.sourceId !== undefined && r.sourceId !== pending.sourceId || (r.removed === true) !== pending.remove
      || pending.expectedGeneration !== undefined && (pending.remove ? r.generation !== pending.expectedGeneration : r.generation !== pending.expectedGeneration + 1)) fail('remote_source_changed');
    return Object.freeze({operationId: pending.operationId, accepted: true, sourceId: r.sourceId, generation: r.generation as number, removed: r.removed === true});
  }
  async save(input: ImportedSourceDraft, source: Readonly<{id: string; generation: number}> | undefined, guard: () => boolean): Promise<ImportedSourceReceipt> {
    return this.flight(guard, async () => {
      if (this.pending) fail('request_in_progress');
      const operationId = await this.operationId(); this.admit(guard);
      if (!/^\d{13}-[0-9a-f-]{36}$/i.test(operationId)) fail('invalid_request');
      const pending = Object.freeze({operationId, remove: false, ...(source ? {sourceId: source.id, expectedGeneration: source.generation} : {})});
      const body = source ? routeBody('update_storage_source_rclone', {...input, operationId, expectedGeneration: source.generation})
        : routeBody('create_storage_source_rclone', {...input, operationId});
      input = {name: '', remote: '', config: ''}; this.pending = pending;
      const raw = await this.request(source ? encodedRoutePath('update_storage_source_rclone', {id: encodeURIComponent(source.id)}) : routePath('create_storage_source_rclone'), source ? 'PUT' : 'POST', body, guard);
      const receipt = this.receipt(raw, pending); this.pending = undefined; return receipt;
    });
  }
  async remove(source: Readonly<{id: string; generation: number}>, guard: () => boolean): Promise<ImportedSourceReceipt> {
    return this.flight(guard, async () => {
      if (this.pending) fail('request_in_progress');
      const operationId = await this.operationId(); this.admit(guard);
      if (!/^\d{13}-[0-9a-f-]{36}$/i.test(operationId)) fail('invalid_request');
      const pending = Object.freeze({operationId, sourceId: source.id, expectedGeneration: source.generation, remove: true}); this.pending = pending;
      const body = routeBody('remove_storage_source_rclone', {operationId, expectedGeneration: source.generation});
      const raw = await this.request(encodedRoutePath('remove_storage_source_rclone', {id: encodeURIComponent(source.id)}), 'DELETE', body, guard);
      const receipt = this.receipt(raw, pending); this.pending = undefined; return receipt;
    });
  }
  async recover(guard: () => boolean): Promise<ImportedSourceReceipt> {
    return this.flight(guard, async () => {
      const pending = this.pending; if (!pending) fail('not_found');
      const raw = await this.request(encodedRoutePath('get_storage_source_rclone_operation', {id: encodeURIComponent(pending.operationId)}), 'GET', undefined, guard);
      const receipt = this.receipt(raw, pending); this.pending = undefined; return receipt;
    });
  }
  dispose() {this.disposed = true; for (const controller of this.controllers) controller.abort(); this.controllers.clear(); this.pending = undefined;}
}
