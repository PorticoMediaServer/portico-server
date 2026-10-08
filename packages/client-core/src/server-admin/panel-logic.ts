/**
 * The rules behind the Server pages' panels that are not a settings document: what a broadcast
 * may carry, how a WebDAV dialog maps onto the server's credential rules, when "remove what's
 * due" is safe, how a window's time is typed. One copy, for every client.
 */
import type {DAVConfiguration} from '../remote-sources.ts';
import type {FeedbackStatus} from '../console.ts';
import type {AccessTier, KeyScope} from '../server-administration.ts';
import type {FilesystemPage} from '../administration.ts';
import type {LibraryAdminJob, LibraryManagementService, ManagedLibrary} from '../library-management.ts';
import type {ScanTier, SourceSettings} from '../library-inventory.ts';
import type {MessageId} from '../../../i18n/src/index.ts';

// ── Maintenance windows and schedules ───────────────────────────────────────────────────────

/** Owner background priority from the server (`lower` | `normal`). Background work is de-prioritized, never paused. */
export type BackgroundTaskPriority = 'lower' | 'normal';
export function backgroundPriorityOrDefault(value: unknown): BackgroundTaskPriority {
  return value === 'normal' ? 'normal' : 'lower';
}
export function maintenanceSaveSettings<T extends {backgroundTaskPriority?: unknown}>(draft: T): T & {backgroundTaskPriority: BackgroundTaskPriority} {
  return {...draft, backgroundTaskPriority: backgroundPriorityOrDefault(draft.backgroundTaskPriority)};
}

/** The scheduled tasks' names; a task this build does not know falls back to the job's own label. */
export const TASK_LABELS: Readonly<Record<string, MessageId>> = {'library-scan': 'server.task.libraryScan', 'metadata-refresh': 'server.task.metadataRefresh', analysis: 'server.task.analysis', trickplay: 'server.task.trickplay', cleanup: 'server.task.cleanup', backup: 'server.task.backup', 'database-maintenance': 'server.task.database'};
/** Minutes after midnight as "HH:MM", and back (clamped to the day). */
export const clockText = (minute: number) => `${String(Math.floor(minute / 60)).padStart(2, '0')}:${String(minute % 60).padStart(2, '0')}`;
export const clockMinutes = (value: string) => { const [h, m] = value.split(':').map(Number); return Math.min(1439, Math.max(0, (h || 0) * 60 + (m || 0))); };

// ── A message to everyone ───────────────────────────────────────────────────────────────────

/** CD-11: one operation id per broadcast; retries reuse it and the dedupe key derived from it. */
export function broadcastBody(operationId: string, input: {audience: string; severity: string; title: string; body: string}) {
  if (!operationId) throw new Error('request_failed');
  return {operationId, audience: input.audience, severity: input.severity, dedupeKey: `broadcast:${operationId}`, title: input.title.trim(), body: input.body.trim(), actions: [], expiresInDays: 14};
}
/** CD-46: the server limits UTF-8 bytes (title 200, body 1000), not characters, on the trimmed text. */
export const broadcastTitleBytes = 200;
export const broadcastBodyBytes = 1000;
export function utf8Length(value: string): number {
  let bytes = 0;
  for (const ch of value) { const c = ch.codePointAt(0)!; bytes += c < 0x80 ? 1 : c < 0x800 ? 2 : c < 0x10000 ? 3 : 4; }
  return bytes;
}
export type BroadcastProblem = '' | 'title-too-long' | 'body-too-long';
export function broadcastProblem(title: string, body: string): BroadcastProblem {
  if (utf8Length(title.trim()) > broadcastTitleBytes) return 'title-too-long';
  if (utf8Length(body.trim()) > broadcastBodyBytes) return 'body-too-long';
  return '';
}

// ── Remote sources ──────────────────────────────────────────────────────────────────────────

const randomUuid = (): string => globalThis.crypto?.randomUUID?.() ?? 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, c => { const r = Math.floor(Math.random() * 16); return (c === 'x' ? r : (r & 0x3) | 0x8).toString(16); });
/** CD-09: a 13-digit epoch-milliseconds prefix plus a UUID, fresh within ten minutes. The replay-receipt key. */
export const storageOperationId = () => `${Date.now()}-${randomUuid()}`;
/** One id per logical operation: kept while the outcome is unknown, released once it is definite. */
export function stableStorageOperationId() {
  let current: string | null = null;
  return {
    next: async (): Promise<string> => (current ??= storageOperationId()),
    release: () => { current = null; },
  };
}
/** CD-09: only an accepted receipt is success. */
export function mountCommandSucceeded(mutation: {phase: string}): boolean {
  return mutation.phase === 'accepted';
}
export function davCommandSucceeded(state: {ambiguous: boolean; receipt: unknown}): boolean {
  return !state.ambiguous && !!state.receipt;
}
export type WebDavDraft = {name: string; root: string; username: string; password: string; insecureLocal: boolean};
/**
 * CD-09: the dialog fields onto the server's credential rules. Create always sends fresh
 * credentials. An edit with an untouched address keeps the connection (and the password unless a
 * new one was typed); an edit with a new address never reuses the stored password.
 */
export function davConfiguration(source: {generation: number} | null, draft: WebDavDraft): DAVConfiguration {
  const name = draft.name.trim();
  const root = draft.root.trim();
  if (!source) return {name, root, username: draft.username, password: draft.password, keepPassword: false, keepConnection: false, insecureLocal: draft.insecureLocal};
  if (root === '') return {name, root: '', username: '', password: draft.password, keepPassword: draft.password === '', keepConnection: true, insecureLocal: draft.insecureLocal, expectedGeneration: source.generation};
  return {name, root, username: draft.username, password: draft.password, keepPassword: false, keepConnection: false, insecureLocal: draft.insecureLocal, expectedGeneration: source.generation};
}

// ── Deleted titles ──────────────────────────────────────────────────────────────────────────

/** CD-42: "Remove what's due" may run only when the loaded page is the whole held listing. */
export function isFullTrashList(page: {items: readonly unknown[]; heldCount: number} | undefined, canPrevious: boolean, canNext: boolean): boolean {
  return !!page && !canPrevious && !canNext && page.items.length === page.heldCount;
}

// ── Viewer feedback ─────────────────────────────────────────────────────────────────────────

export const feedbackStatusLabel: Readonly<Record<FeedbackStatus, string>> = {open: 'Open', 'in-progress': 'In progress', resolved: 'Resolved', closed: 'Closed'};
const feedbackCategories: Readonly<Record<string, string>> = {playback: 'Playback', metadata: 'Metadata', subtitles: 'Subtitles', other: 'Other'};
/** CD-38: an unknown category shows as it is, never blank. */
export const feedbackCategoryLabel = (category: string): string => feedbackCategories[category] ?? category;

// ── Live TV sources and Library Channels ────────────────────────────────────────────────────

/** CD-49: only a remote, enabled source may refresh (it is in the loaded refresh-state list). */
export function canRefreshSource(src: {id: string; state: string}, refresh: readonly {sourceId: string}[] | undefined): boolean {
  if (!refresh || src.state === 'disabled') return false;
  return refresh.some(r => r.sourceId === src.id);
}
/** CD-49: a bare tuner host or IP becomes an explicit http:// URL before preview; other schemes are refused. */
export function normalizeHdHomerunLocator(input: string): string {
  const trimmed = input.trim();
  if (!trimmed) return trimmed;
  const scheme = /^([a-zA-Z][a-zA-Z0-9+.-]*):\/\//.exec(trimmed);
  if (scheme) {
    const lower = scheme[1]!.toLowerCase();
    if (lower !== 'http' && lower !== 'https') throw new Error('invalid_request');
    return trimmed;
  }
  return 'http://' + trimmed;
}
/** CD-49: a guide-only source has nothing to stream: its guide pointer and streaming override are cleared. */
export function guideOnlyPatch(kind: string): {kind: string; guideSourceId?: string; streamBufferSeconds?: null} {
  if (kind === 'xmltv-guide') return {kind, guideSourceId: '', streamBufferSeconds: null};
  return {kind};
}
/** CD-15: regeneration sends the published boundary unchanged, fenced on the current revision. */
export function regenerationBody(channel: {revision: number; replacementBoundary: string}, requestId: string): {requestId: string; expectedRevision: number; boundary: string} {
  if (!requestId) throw new Error('request_failed');
  if (!channel.replacementBoundary || !Number.isInteger(channel.revision) || channel.revision < 1) throw new Error('request_failed');
  return {requestId, expectedRevision: channel.revision, boundary: channel.replacementBoundary};
}

// ── People ──────────────────────────────────────────────────────────────────────────────────

/** CD-07: the server allows at most 100 rows a page; every list stays on that limit and follows nextCursor. */
export const accessLists = {members: '/v1/admin/access/members', invitations: '/v1/admin/access/invitations', devices: '/v1/admin/access/devices', apiKeys: '/v1/admin/access/api-keys'} as const;
export const accessListPath = (list: keyof typeof accessLists, cursor = '') => `${accessLists[list]}?limit=100${cursor ? '&cursor=' + encodeURIComponent(cursor) : ''}`;
export const tierLabels: Readonly<Record<AccessTier, string>> = {owner: 'Owner', admin: 'Administrator', member: 'Member'};
export const scopeLabels: Readonly<Record<KeyScope, string>> = {'read-only': 'Read only', playback: 'Playback', 'library-management': 'Library management', full: 'Full access'};
export const scopeHelp: Readonly<Record<KeyScope, string>> = {'read-only': 'Browse and read, nothing else.', playback: 'Browse and start playback.', 'library-management': 'Also scan, match and edit libraries.', full: 'Everything this account can do.'};
/** An account made on this server. `hostedAccountId`: a Portico Account member, whose password lives at the Portico Account. */
export type DirectMember = {id: string; username: string; primaryProfileId: string; role: AccessTier; revision: number; disabled: boolean; allowedLibraries: string[]; hostedAccountId?: string};

// ── Libraries ───────────────────────────────────────────────────────────────────────────────

export const scanJobLabels: Readonly<Record<LibraryAdminJob['status'], string>> = {queued: 'Queued', running: 'Scanning', paused: 'Paused', complete: 'Up to date', complete_with_warnings: 'Finished with warnings', failed: 'Failed', cancelled: 'Canceled'};
export const scanTierLabels: Readonly<Record<ScanTier, string>> = {file_list_only: 'File list only', basic: 'Basic', complete: 'Complete', custom: 'Custom'};
export const scanTierHelp: Readonly<Record<ScanTier, string>> = {file_list_only: 'Just find files. No file details or artwork.', basic: 'Read file details (length, resolution, audio) and local artwork.', complete: 'Everything, including previews, loudness and fingerprints.', custom: 'Choose exactly which steps run.'};

/** CD-08: a new folder starts from the loaded configuration revision; zero is never submittable. */
export function addSourceSettings(revision: number): SourceSettings {
  return {name: '', path: '', classification: 'local', followSymlinks: false, missingGraceSeconds: 86400, intervalSeconds: 3600, expectedRevision: revision, acceptReplacement: false};
}
export function canAddSource(revision: number | undefined): revision is number {
  return typeof revision === 'number' && revision > 0;
}

/** `GET /v1/admin/filesystem` for one page (at most 200 entries), with the cursor of the previous page when loading more. */
export function filesystemPath(path: string, cursor = ''): string {
  const parts = ['limit=200'];
  if (path) parts.push('path=' + encodeURIComponent(path));
  if (cursor) parts.push('cursor=' + encodeURIComponent(cursor));
  return '/v1/admin/filesystem?' + parts.join('&');
}
/** Appends the next page of a folder, keeping the server's order and never listing an entry twice. */
export function mergeFilesystemPages(current: FilesystemPage, next: FilesystemPage): FilesystemPage {
  const seen = new Set(current.entries.map(e => e.path));
  return {...next, entries: [...current.entries, ...next.entries.filter(e => !seen.has(e.path))]};
}

/** A create publishes its outcome in the service's mutation without rejecting: a refusal is read here. */
export function libraryMutationError(service: LibraryManagementService): {code: string; message: string; retryable?: boolean} | null {
  const mutation = service.getSnapshot().mutation;
  if (mutation.phase === 'error' || mutation.phase === 'ambiguous') {
    const err = mutation.error;
    if (err) return {code: err.code, message: err.message, retryable: err.retryable};
    return {code: 'request_failed', message: 'request_failed'};
  }
  return null;
}
/** Creates the library and, when the server has not started one, its first scan (ONB-04). Resolves the new library's id. */
export async function createLibraryAndScan(service: LibraryManagementService, input: {name: string; kind: ManagedLibrary['kind']; path: string; metadataAgent: 'online' | 'local'; metadataLanguage?: string}): Promise<string | undefined> {
  if (service.getSnapshot().directory.phase !== 'ready') await service.loadLibraries();
  const before = new Set((service.getSnapshot().directory.data?.items ?? []).map(l => l.id));
  await service.createLibrary(input);
  const failed = libraryMutationError(service);
  if (failed) throw Object.assign(new Error(failed.message), {code: failed.code, retryable: failed.retryable});
  await service.loadLibraries();
  const created = (service.getSnapshot().directory.data?.items ?? []).find(l => !before.has(l.id));
  if (!created) {
    const late = libraryMutationError(service);
    if (late) throw Object.assign(new Error(late.message), {code: late.code, retryable: late.retryable});
    throw new Error('request_failed');
  }
  try {
    if (!created.lastScan) {
      await service.selectLibrary(created.id);
      const selected = service.getSnapshot().selected.data?.library;
      if (selected && !selected.lastScan && selected.actions.includes('scan')) await service.scan();
    }
  } catch { /* The library exists; Scan now on its page still works. */ }
  return created.id;
}

/**
 * The "keep for" rows of Storage & backups: what each of the server's retention keys is called
 * and the order they are read in. Authored, so a key never shows as itself ("trickplay") and the
 * list is not alphabetical by an internal name. A key this list does not know yet goes last under
 * the name the server gives its storage category.
 */
const RETENTION_ROWS: readonly (readonly [string, MessageId])[] = [
  ['logs', 'settings.server.keep.logs'], ['conversions', 'settings.server.keep.conversions'], ['prepared', 'settings.server.keep.prepared'],
  ['downloads', 'settings.server.keep.downloads'], ['subtitles', 'settings.server.keep.subtitles'], ['trickplay', 'settings.server.keep.trickplay'], ['trash', 'settings.server.keep.trash'],
];

export function retentionRows(keys: readonly string[], categoryName: (key: string) => string | undefined, t: (id: MessageId, values?: Record<string, string | number>) => string): readonly Readonly<{key: string; label: string}>[] {
  const known = RETENTION_ROWS.filter(([key]) => keys.includes(key)).map(([key, label]) => ({key, label: t(label)}));
  const rest = keys.filter(key => !RETENTION_ROWS.some(([k]) => k === key)).sort().map(key => ({key, label: t('web.maintenance.keepCategory', {name: (categoryName(key) ?? key.replace(/[_-]+/g, ' ')).toLowerCase()})}));
  return [...known, ...rest];
}
