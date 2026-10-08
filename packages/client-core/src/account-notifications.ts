/**
 * The Portico Account's in-app notification feed and data export (BE-hosted, `be/hosted` da0a9db):
 * no push and no alert email any more. Parsers are strict about the shapes and lenient about
 * unknown kinds (a newer Hosted can add one; it shows as a generic row).
 *
 * - `GET /v1/account/notifications[?cursor=]` → `{items, unread, nextCursor?}` (newest first, 50 a page)
 * - `GET /v1/account/notifications/{id}`
 * - `POST /v1/account/notifications/read` `{ids}` or `{all: true}`
 * - `GET|PUT /v1/account/notification-preferences` `{serverOffline, storageNearlyFull, invitationAccepted}`
 * - export: `POST /v1/account/export` → `{id, state, expiresAt}`, then `GET /v1/account/exports/{id}`
 *   until `ready` (or the `account_export_ready` item), then `POST /v1/account/exports/{id}/download`.
 */

export type AccountNotificationKind =
  | 'server_offline' | 'storage_nearly_full' | 'invitation_accepted'
  | 'new_device' | 'security_event' | 'account_export_ready' | 'signing_key_expiring';
export const ACCOUNT_NOTIFICATION_KINDS: readonly AccountNotificationKind[] = ['server_offline', 'storage_nearly_full', 'invitation_accepted', 'new_device', 'security_event', 'account_export_ready', 'signing_key_expiring'];
export const ACCOUNT_SECURITY_EVENTS = ['password_changed', 'password_reset', 'recovery_codes_replaced', 'sign_in_method_removed', 'provider_linked', 'two_step_enabled', 'two_step_disabled', 'contact_email_changed', 'contact_email_verified'] as const;
export type AccountSecurityEvent = (typeof ACCOUNT_SECURITY_EVENTS)[number];

export type AccountNotification = Readonly<{
  id: string;
  /** A kind this client doesn't know yet is kept (shown generically), never dropped. */
  kind: AccountNotificationKind | 'other';
  rawKind: string;
  occurredAt: string;
  read: boolean;
  server?: Readonly<{id: string; name: string}>;
  detail: Readonly<Record<string, string>>;
}>;
export type AccountNotificationPage = Readonly<{items: readonly AccountNotification[]; unread: number; nextCursor?: string}>;
export type NotificationPreferences = Readonly<{serverOffline: boolean; storageNearlyFull: boolean; invitationAccepted: boolean}>;
export type AccountExportJob = Readonly<{id: string; state: 'pending' | 'ready'; expiresAt: string}>;

type Request = <T>(path: string, method?: string, body?: unknown, signal?: AbortSignal) => Promise<T>;

function bad(): never { throw Object.assign(new Error('invalid notification response'), {code: 'invalid_response'}); }
const obj = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
const str = (v: unknown, max = 256): v is string => typeof v === 'string' && v.length > 0 && v.length <= max;
const instant = (v: unknown): v is string => typeof v === 'string' && v.length <= 64 && Number.isFinite(Date.parse(v));

export function parseAccountNotification(v: unknown): AccountNotification {
  if (!obj(v) || !str(v.id, 128) || !str(v.kind, 64) || !instant(v.occurredAt) || typeof v.read !== 'boolean') bad();
  let server: {id: string; name: string} | undefined;
  if (v.server !== undefined && v.server !== null) {
    if (!obj(v.server) || !str(v.server.id, 128) || typeof v.server.name !== 'string' || v.server.name.length > 200) bad();
    server = {id: v.server.id, name: v.server.name};
  }
  const detail: Record<string, string> = {};
  if (v.detail !== undefined && v.detail !== null) {
    if (!obj(v.detail)) bad();
    for (const [k, val] of Object.entries(v.detail)) if (typeof val === 'string' && k.length <= 64 && val.length <= 500) detail[k] = val;
  }
  const kind = (ACCOUNT_NOTIFICATION_KINDS as readonly string[]).includes(v.kind) ? v.kind as AccountNotificationKind : 'other';
  return Object.freeze({id: v.id, kind, rawKind: v.kind, occurredAt: v.occurredAt, read: v.read, ...(server ? {server: Object.freeze(server)} : {}), detail: Object.freeze(detail)});
}

export function parseAccountNotificationPage(v: unknown): AccountNotificationPage {
  if (!obj(v) || !Array.isArray(v.items) || v.items.length > 200 || typeof v.unread !== 'number' || !Number.isInteger(v.unread) || v.unread < 0) bad();
  if (v.nextCursor !== undefined && v.nextCursor !== '' && !str(v.nextCursor, 2048)) bad();
  const items = v.items.map(parseAccountNotification);
  return Object.freeze({items: Object.freeze(items), unread: v.unread, ...(v.nextCursor ? {nextCursor: v.nextCursor as string} : {})});
}

export function parseNotificationPreferences(v: unknown): NotificationPreferences {
  if (!obj(v) || typeof v.serverOffline !== 'boolean' || typeof v.storageNearlyFull !== 'boolean' || typeof v.invitationAccepted !== 'boolean') bad();
  return Object.freeze({serverOffline: v.serverOffline, storageNearlyFull: v.storageNearlyFull, invitationAccepted: v.invitationAccepted});
}

export function parseAccountExportJob(v: unknown): AccountExportJob {
  if (!obj(v) || !str(v.id, 160) || (v.state !== 'pending' && v.state !== 'ready') || !instant(v.expiresAt)) bad();
  return Object.freeze({id: v.id, state: v.state, expiresAt: v.expiresAt});
}

export class AccountNotificationsClient {
  private readonly request: Request;
  constructor(request: Request) { this.request = request; }
  async page(cursor?: string, signal?: AbortSignal): Promise<AccountNotificationPage> {
    return parseAccountNotificationPage(await this.request('/v1/account/notifications' + (cursor ? '?cursor=' + encodeURIComponent(cursor) : ''), 'GET', undefined, signal));
  }
  async one(id: string, signal?: AbortSignal): Promise<AccountNotification> {
    return parseAccountNotification(await this.request('/v1/account/notifications/' + encodeURIComponent(id), 'GET', undefined, signal));
  }
  async markRead(ids: readonly string[] | 'all'): Promise<void> {
    if (ids !== 'all' && !ids.length) return;
    await this.request('/v1/account/notifications/read', 'POST', ids === 'all' ? {all: true} : {ids: [...ids]});
  }
  async preferences(signal?: AbortSignal): Promise<NotificationPreferences> {
    return parseNotificationPreferences(await this.request('/v1/account/notification-preferences', 'GET', undefined, signal));
  }
  async savePreferences(p: NotificationPreferences): Promise<NotificationPreferences> {
    return parseNotificationPreferences(await this.request('/v1/account/notification-preferences', 'PUT', {serverOffline: p.serverOffline, storageNearlyFull: p.storageNearlyFull, invitationAccepted: p.invitationAccepted}));
  }
  /** Starts (or rejoins) the account's export with the signed-in session (BE-hosted cc222bf: `{operationId}`, 16–128 characters, no proof). Idempotent per operation id. */
  async requestExport(operationId: string): Promise<AccountExportJob> {
    if (operationId.length < 16 || operationId.length > 128) throw new Error('An export operation id is 16 to 128 characters.');
    return parseAccountExportJob(await this.request('/v1/account/export', 'POST', {operationId}));
  }
  async exportStatus(id: string, signal?: AbortSignal): Promise<AccountExportJob> {
    return parseAccountExportJob(await this.request('/v1/account/exports/' + encodeURIComponent(id), 'GET', undefined, signal));
  }
  async downloadExport(id: string): Promise<unknown> {
    return this.request('/v1/account/exports/' + encodeURIComponent(id) + '/download', 'POST', {});
  }
}

/**
 * The whole export: request, wait until ready (polling with a growing interval, up to `maxWaitMs`),
 * download. Returns the document, or `pending` with the job when it's still being prepared (the
 * `account_export_ready` notification finishes it later).
 */
export async function runAccountExport(client: AccountNotificationsClient, operationId: string, o: {maxWaitMs?: number; sleep?: (ms: number) => Promise<void>; signal?: AbortSignal} = {}): Promise<{state: 'ready'; data: unknown; job: AccountExportJob} | {state: 'pending'; job: AccountExportJob}> {
  const sleep = o.sleep ?? (ms => new Promise<void>(r => setTimeout(r, ms)));
  let job = await client.requestExport(operationId);
  let waited = 0, step = 1500;
  while (job.state !== 'ready' && waited < (o.maxWaitMs ?? 60_000)) {
    if (o.signal?.aborted) return {state: 'pending', job};
    await sleep(step);
    waited += step;
    step = Math.min(step * 2, 10_000);
    job = await client.exportStatus(job.id, o.signal);
  }
  if (job.state !== 'ready') return {state: 'pending', job};
  return {state: 'ready', data: await client.downloadExport(job.id), job};
}
