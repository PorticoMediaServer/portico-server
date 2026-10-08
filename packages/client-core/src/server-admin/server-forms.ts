import type {ConsoleClient, RuntimeSettings, SettingsDocument} from '../console.ts';
import {parseConnectivityStatus, parseLogSettings, type ConnectivityPolicy, type ConnectivityStatus, type LogSettings} from '../server-administration.ts';
import {parseDVRDocument, parseEnvelope, parseLiveDefaultsDocument, folderTemplateProblems, type DVRDefaults, type DVRDocument, type LiveDefaults} from '../administration.ts';
import {loadRemoteAccess, saveRemoteAccess, type RemoteAccess, type RemoteConfiguration} from '../remote-access.ts';
import {loadCertificate, saveCertificate, type CertificateApi, type CertificateConfig, type CertificateStatus} from '../certificates.ts';
import type {HttpLocalApi} from '../index.ts';
import {DraftForm, type Loaded} from './draft-form.ts';

/**
 * The server's settings documents as draft forms (`draft-form.ts`), one per document, for the
 * Server pages of Settings. Every read, write, revision fence and operation id is here, so the
 * web and the iPhone app share them and a page has one Save.
 *
 * A form's value is what the owner edits; `status` is what arrives with it and is only shown
 * (effective values, choices the server offers, the revision to save against).
 */

export type ServerFormId = 'runtime' | 'connectivity' | 'remote' | 'certificate' | 'alerts' | 'liveDefaults' | 'recording' | 'maintenance' | 'logs';

export type ServerFormDeps = Readonly<{
  api: Pick<HttpLocalApi, 'request'>;
  console: ConsoleClient;
  /** The server every administration answer must name. */
  serverId: string;
  /** 48 lowercase hex characters (every write route accepts that shape). */
  operationId: () => string;
}>;

/** BE-API-10: one operation id per logical write. A retry of the same payload reuses it, so the server replays its receipt; a changed payload gets a new one. */
export function createOperationIds(make: () => string): {forPayload: (key: string) => string; release: () => void} {
  let current: {key: string; id: string} | null = null;
  return {
    forPayload: key => {
      if (current && current.key === key) return current.id;
      current = {key, id: make()};
      return current.id;
    },
    release: () => { current = null; },
  };
}

/** A read that answers 404 is a feature this server has not set up: the form is unavailable, not failed. */
async function orAbsent<T>(read: Promise<T>): Promise<T | null> {
  try { return await read; } catch (error) { if ((error as {status?: number} | null)?.status === 404) return null; throw error; }
}

// ── Runtime settings: name, converting video, capacity, how long records are kept ───────────

export type RuntimeStatus = Readonly<{revision: number; effective: RuntimeSettings; restartFields: readonly string[]}>;
const runtimeLoaded = (doc: SettingsDocument): Loaded<RuntimeSettings, RuntimeStatus> => ({value: doc.requested, status: {revision: doc.revision, effective: doc.effective, restartFields: doc.restartFields}});

/** The retention the server accepts: diagnostics and finished tasks 1–30 days, notifications 1–180. */
export const RETENTION_LIMITS: Readonly<Record<'diagnosticDays' | 'notificationDays' | 'jobDays', number>> = {diagnosticDays: 30, notificationDays: 180, jobDays: 30};

export function runtimeForm(deps: ServerFormDeps): DraftForm<RuntimeSettings, RuntimeStatus> {
  return new DraftForm<RuntimeSettings, RuntimeStatus>({
    load: async () => runtimeLoaded(await deps.console.settings()),
    save: async (draft, _saved, status) => runtimeLoaded(await deps.console.applySettings(status?.revision ?? 0, draft)),
    validate: draft => (draft.name.trim() ? undefined : 'server.form.nameRequired'),
  });
}

// ── Connectivity: who may sign in from outside, streaming limits, home networks ──────────────

export type ConnectivityDraft = Omit<ConnectivityPolicy, 'revision'>;
const connectivityLoaded = (status: ConnectivityStatus): Loaded<ConnectivityDraft, ConnectivityStatus> => {
  const {revision: _revision, ...draft} = status.policy;
  return {value: draft, status};
};

export function connectivityForm(deps: ServerFormDeps): DraftForm<ConnectivityDraft, ConnectivityStatus> {
  const ids = createOperationIds(deps.operationId);
  return new DraftForm<ConnectivityDraft, ConnectivityStatus>({
    load: async signal => connectivityLoaded(parseConnectivityStatus(await deps.api.request('/v1/admin/connectivity/status', 'GET', undefined, signal))),
    save: async (draft, saved, status) => {
      // Only what changed is sent: the policy is a merge patch.
      const change: Record<string, unknown> = {};
      for (const key of Object.keys(draft) as (keyof ConnectivityDraft)[]) if (JSON.stringify(draft[key]) !== JSON.stringify(saved[key])) change[key] = draft[key];
      const revision = status?.policy.revision ?? 0;
      await deps.api.request('/v1/admin/connectivity/policy', 'PATCH', {expectedRevision: revision, operationId: ids.forPayload(JSON.stringify({revision, change})), ...change});
      ids.release();
    },
  });
}

// ── Remote access (the router and the public address) and the HTTPS certificate ─────────────

export function remoteAccessForm(deps: ServerFormDeps): DraftForm<RemoteConfiguration, RemoteAccess> {
  const api = deps.api as HttpLocalApi;
  return new DraftForm<RemoteConfiguration, RemoteAccess>({
    load: async signal => { const status = await orAbsent(loadRemoteAccess(api, signal)); return status && {value: status.config, status}; },
    save: async (draft, _saved, status) => { const next = await saveRemoteAccess(api, status!, draft); return {value: next.config, status: next}; },
  });
}

export function certificateForm(deps: ServerFormDeps): DraftForm<CertificateConfig, CertificateStatus> {
  const api: CertificateApi = {requestCertificate: (action, body, signal) => (action === 'status' ? deps.api.request('/v1/networking/certificate', 'GET', undefined, signal) : deps.api.request(`/v1/networking/certificate/${action}`, 'POST', body, signal))};
  const bounded = () => { const c = new AbortController(); setTimeout(() => c.abort(), 15_000); return c.signal; };
  return new DraftForm<CertificateConfig, CertificateStatus>({
    load: async signal => { const status = await orAbsent(loadCertificate(api, signal)); return status && {value: status.config, status}; },
    save: async (draft, _saved, status) => { const next = await saveCertificate(api, status!, draft, bounded()); return {value: next.config, status: next}; },
  });
}

// ── Alert thresholds ─────────────────────────────────────────────────────────────────────────

export type AlertThresholds = Readonly<{storageWarningPercent: number; storageCriticalPercent: number; certificateWarningDays: number}>;

export function alertsForm(deps: ServerFormDeps): DraftForm<AlertThresholds, {revision: number}> {
  const read = (raw: unknown): Loaded<AlertThresholds, {revision: number}> => {
    const v = ((raw as {data?: unknown})?.data ?? raw) as AlertThresholds & {revision: number};
    if (!v || typeof v.revision !== 'number' || typeof v.storageWarningPercent !== 'number') throw new Error('The server returned alert settings this version cannot read.');
    return {value: {storageWarningPercent: v.storageWarningPercent, storageCriticalPercent: v.storageCriticalPercent, certificateWarningDays: v.certificateWarningDays}, status: {revision: v.revision}};
  };
  return new DraftForm<AlertThresholds, {revision: number}>({
    load: async signal => read(await deps.api.request('/v1/admin/notifications/settings', 'GET', undefined, signal)),
    save: async (draft, _saved, status) => { await deps.api.request('/v1/admin/notifications/settings', 'PATCH', {expectedRevision: status?.revision ?? 0, ...draft}); },
    validate: draft => (draft.storageCriticalPercent >= draft.storageWarningPercent ? 'server.form.alertOrder' : undefined),
  });
}

// ── Live TV and recording defaults ──────────────────────────────────────────────────────────

function documentForm<T extends object, D extends {revision: number; settings: T}>(deps: ServerFormDeps, path: string, parse: (raw: unknown) => D, validate?: (draft: T, doc: D | undefined) => string | undefined): DraftForm<T, D> {
  const ids = createOperationIds(deps.operationId);
  return new DraftForm<T, D>({
    load: async signal => { const doc = await orAbsent(deps.api.request<unknown>(path, 'GET', undefined, signal).then(raw => parseEnvelope(raw, deps.serverId, parse).result)); return doc && {value: doc.settings, status: doc}; },
    save: async (draft, _saved, doc) => {
      const revision = doc?.revision ?? 0;
      await deps.api.request(path, 'PUT', {expectedRevision: revision, operationId: ids.forPayload(JSON.stringify({revision, settings: draft})), settings: draft});
      ids.release();
    },
    validate,
  });
}

export const liveDefaultsForm = (deps: ServerFormDeps) => documentForm<LiveDefaults, ReturnType<typeof parseLiveDefaultsDocument>>(deps, '/v1/admin/live/settings', parseLiveDefaultsDocument);

export const recordingForm = (deps: ServerFormDeps) => documentForm<DVRDefaults, DVRDocument>(deps, '/v1/admin/dvr/settings', parseDVRDocument, (draft, doc) => {
  if (!doc?.folderTokens.length) return undefined;
  return [...folderTemplateProblems(draft.folderTemplate, doc.folderTokens), ...folderTemplateProblems(draft.movieFolderTemplate, doc.folderTokens)][0];
});

// ── Maintenance: windows, background priority, what is kept and for how long ────────────────

export type MaintenanceWindow = Readonly<{id: string; name: string; enabled: boolean; cadence: string; days: readonly string[]; startMinute: number; durationMinutes: number; timezone: string; tasks: readonly string[]}>;
export type MaintenanceSettings = Readonly<{windows: readonly MaintenanceWindow[]; retention: Readonly<Record<string, number>>; backupKeepCount: number; backgroundTaskPriority: 'lower' | 'normal'}>;
export type MaintenanceDocument = Readonly<{revision: number; settings: MaintenanceSettings; cadences: readonly {id: string; name: string; days: readonly string[]; description: string}[]; tasks: readonly string[]; days: readonly string[]; retentionMaxima: Readonly<Record<string, number>>; storageCategories: readonly {id: string; name: string; retentionKey: string}[]}>;

/** The document is owner configuration the server validates in full on save, so it is read structurally. */
export function parseMaintenanceDocument(raw: unknown): MaintenanceDocument {
  const v = raw as MaintenanceDocument;
  if (!v || typeof v.revision !== 'number' || !v.settings || !Array.isArray(v.settings.windows) || !Array.isArray(v.cadences) || !Array.isArray(v.tasks)) throw new Error('The server returned maintenance settings this version cannot read.');
  // Background work is de-prioritized, never paused: anything else reads as "lower".
  return {...v, settings: {...v.settings, backgroundTaskPriority: v.settings.backgroundTaskPriority === 'normal' ? 'normal' : 'lower'}};
}

export const maintenanceForm = (deps: ServerFormDeps) => documentForm<MaintenanceSettings, MaintenanceDocument>(deps, '/v1/admin/maintenance/settings', parseMaintenanceDocument);

export type ScheduledJob = Readonly<{task: string; /** Names of the enabled windows that run it. */ windows: readonly string[]; /** The start of the next window that runs it, in the server window's own clock, or undefined when no enabled window does. */ nextAt?: Date}>;

const DAY_INDEX: Readonly<Record<string, number>> = {sunday: 0, monday: 1, tuesday: 2, wednesday: 3, thursday: 4, friday: 5, saturday: 6, sun: 0, mon: 1, tue: 2, wed: 3, thu: 4, fri: 5, sat: 6};

/** When a window next opens, by this device's clock (the document's window times are the server's local times; the two agree when owner and server share a time zone, which the row says). */
export function nextWindowStart(window: MaintenanceWindow, now: Date): Date | undefined {
  if (!window.enabled) return undefined;
  const days = new Set(window.days.map(d => DAY_INDEX[d.toLowerCase()]).filter(d => d !== undefined));
  for (let offset = 0; offset < 8; offset++) {
    const at = new Date(now.getFullYear(), now.getMonth(), now.getDate() + offset, Math.floor(window.startMinute / 60), window.startMinute % 60);
    if ((days.size === 0 || days.has(at.getDay())) && at.getTime() > now.getTime()) return at;
  }
  return undefined;
}

/** Every job the server runs on a schedule, with the windows it runs in and when the next one opens. */
export function scheduledJobs(doc: Pick<MaintenanceDocument, 'tasks'>, settings: MaintenanceSettings, now: Date): readonly ScheduledJob[] {
  return doc.tasks.map(task => {
    const running = settings.windows.filter(w => w.enabled && w.tasks.includes(task));
    const starts = running.map(w => nextWindowStart(w, now)).filter((d): d is Date => !!d).sort((a, b) => a.getTime() - b.getTime());
    return {task, windows: running.map(w => w.name), ...(starts[0] ? {nextAt: starts[0]} : {})};
  });
}

// ── Message log detail ──────────────────────────────────────────────────────────────────────

export type LogsDraft = Readonly<{retention: LogSettings['retention']}>;

export function logsForm(deps: ServerFormDeps): DraftForm<LogsDraft, LogSettings> {
  const ids = createOperationIds(deps.operationId);
  return new DraftForm<LogsDraft, LogSettings>({
    load: async signal => { const settings = parseLogSettings(await deps.api.request('/v1/admin/logs/settings', 'GET', undefined, signal)); return {value: {retention: settings.retention}, status: settings}; },
    save: async (draft, _saved, status) => {
      const payload = {logLevel: status?.logLevel ?? 'info', retention: draft.retention};
      await deps.api.request('/v1/admin/logs/settings', 'PATCH', {expectedRevision: status?.revision ?? 0, operationId: ids.forPayload(JSON.stringify({revision: status?.revision, ...payload})), ...payload});
      ids.release();
    },
  });
}

/** "Record more detail for 30 minutes": on, or off again. The one control for extra detail. */
export async function setDetailWindow(deps: ServerFormDeps, on: boolean): Promise<void> {
  await deps.api.request('/v1/admin/logs/debug-window', 'POST', {operationId: deps.operationId(), minutes: on ? 30 : 0});
}

// ── The forms of a page ─────────────────────────────────────────────────────────────────────

const FACTORIES: Readonly<Record<ServerFormId, (deps: ServerFormDeps) => DraftForm<any, any>>> = {
  runtime: runtimeForm, connectivity: connectivityForm, remote: remoteAccessForm, certificate: certificateForm, alerts: alertsForm,
  liveDefaults: liveDefaultsForm, recording: recordingForm, maintenance: maintenanceForm, logs: logsForm,
};

export function createServerForm(id: ServerFormId, deps: ServerFormDeps): DraftForm<any, any> {
  return FACTORIES[id](deps);
}

/** Reads a dotted path from a draft (`keepPolicy.mode`). */
export function formValue(draft: unknown, path: string): unknown {
  let at: unknown = draft;
  for (const part of path.split('.')) {
    if (at === null || typeof at !== 'object') return undefined;
    at = (at as Record<string, unknown>)[part];
  }
  return at;
}

/** The draft with a dotted path set, copying only the objects on the way. */
export function withFormValue<T extends object>(draft: T, path: string, value: unknown): T {
  const [head, ...rest] = path.split('.');
  const current = (draft as Record<string, unknown>)[head!];
  return {...draft, [head!]: rest.length ? withFormValue((current ?? {}) as object, rest.join('.'), value) : value};
}
