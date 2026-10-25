import {routePath} from '../../contracts/src/routes.ts';

/** One device a person has signed in on, as the owner's device list shows it.
 * This is the same record sessions are bound to, filtered to one account — not an
 * account-wide list relabelled as a person's. */
export type PersonDevice = Readonly<{id: string; name: string; platform: string; app: string; appVersion: string;
  lastSeenAt: string; firstSeenAt: string; liveState: string; profileName?: string; sessionCount?: number}>;
export type PersonDevicePage = Readonly<{items: readonly PersonDevice[]; nextCursor?: string}>;

export type PersonDevicesApi = Readonly<{request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T>}>;

const obj = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v);
const text = (v: unknown, max = 512): v is string => typeof v === 'string' && v.length > 0 && v.length <= max && !/[\u0000-\u001f\u007f]/.test(v);

/** One page of one person's devices, newest activity first. The producer owns
 * authorization, the target-person fence, filtering and the cursor; this keeps
 * only the facts the drawn row shows and leaves out a device it cannot read. */
export async function listPersonDevices(api: PersonDevicesApi, input: {account: string; cursor?: string; limit?: number}, signal?: AbortSignal): Promise<PersonDevicePage> {
  const params = new URLSearchParams({account: input.account, sort: 'last_seen'});
  if (input.cursor) params.set('cursor', input.cursor);
  if (input.limit) params.set('limit', String(input.limit));
  const raw = await api.request<unknown>(routePath('get_admin_access_devices') + '?' + params, 'GET', undefined, signal);
  if (!obj(raw) || !Array.isArray(raw.items) || raw.items.length > 200) throw new Error('The server returned an unreadable device list.');
  if (raw.nextCursor !== undefined && !text(raw.nextCursor, 256)) throw new Error('The server returned an unreadable device cursor.');
  const items: PersonDevice[] = [];
  for (const entry of raw.items) {
    if (!obj(entry) || !text(entry.id, 256) || !text(entry.name) || !text(entry.platform, 64) || !text(entry.app, 64) || !text(entry.appVersion, 64) ||
      !text(entry.lastSeenAt, 64) || !text(entry.firstSeenAt, 64) || !text(entry.liveState, 32)) continue;
    items.push(Object.freeze({id: entry.id, name: entry.name, platform: entry.platform, app: entry.app, appVersion: entry.appVersion,
      lastSeenAt: entry.lastSeenAt, firstSeenAt: entry.firstSeenAt, liveState: entry.liveState,
      ...(text(entry.profileName) ? {profileName: entry.profileName} : {}),
      ...(typeof entry.sessionCount === 'number' && Number.isSafeInteger(entry.sessionCount) && entry.sessionCount >= 0 ? {sessionCount: entry.sessionCount} : {})}));
  }
  return Object.freeze({items: Object.freeze(items), ...(raw.nextCursor ? {nextCursor: raw.nextCursor} : {})});
}
