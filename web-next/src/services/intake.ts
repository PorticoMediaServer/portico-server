import {normalizeSetupCode} from '@core/setup-code.ts';
import {BrowserProtectedStore} from '../platform/server-connections';

export type ServicesLinkKind = 'device' | 'reset' | 'verify-email';
export type ServicesLink = Readonly<{version: 1; kind: ServicesLinkKind; value: string; createdAt: number}>;
export type LinkCapture = Readonly<{kind: ServicesLinkKind; link?: ServicesLink}>;
export type ServicesLinkStore = Pick<BrowserProtectedStore<ServicesLink>, 'read' | 'change'>;
const lifetimes: Record<ServicesLinkKind, number> = {device: 600_000, reset: 900_000, 'verify-email': 600_000};

/** This runs synchronously in the entry, before a store read, render or network request. */
export function captureServicesLink(page: Pick<Location, 'pathname' | 'search' | 'hash'>,
  history: Pick<History, 'state' | 'replaceState'>, now = Date.now()): LinkCapture | undefined {
  const kind = page.pathname.slice(1);
  if (kind !== 'device' && kind !== 'reset' && kind !== 'verify-email') return;
  if (!page.hash) return;
  const fragment = page.hash;
  history.replaceState(history.state, '', page.pathname + page.search);
  const invalid = {kind} as const;
  if (page.search || fragment.length > 4200) return invalid;
  const params = new URLSearchParams(fragment.slice(1)), key = kind === 'device' ? 'code' : 'token';
  if (params.size !== 1 || params.getAll(key).length !== 1) return invalid;
  let value = params.get(key) ?? '';
  if (kind === 'device') {
    try { value = normalizeSetupCode(value); } catch { return invalid; }
  } else if (!/^[A-Za-z0-9_-]{1,4096}$/.test(value)) return invalid;
  return {kind, link: {version: 1, kind, value, createdAt: now}};
}

function validLink(value: ServicesLink | undefined, now: number): value is ServicesLink {
  if (!value || value.version !== 1 || !Object.hasOwn(lifetimes, value.kind) ||
    !Number.isSafeInteger(value.createdAt) || value.createdAt > now || now - value.createdAt >= lifetimes[value.kind]) return false;
  if (typeof value.value !== 'string') return false;
  if (value.kind !== 'device') return /^[A-Za-z0-9_-]{1,4096}$/.test(value.value);
  try { return normalizeSetupCode(value.value) === value.value; } catch { return false; }
}

/** A malformed newly opened link replaces an older one; it must never revive stale authority. */
export async function restoreServicesLink(store: ServicesLinkStore, capture?: LinkCapture, now = Date.now()) {
  if (capture) await store.change(() => capture.link);
  const value = await store.read();
  if (validLink(value, now)) return value;
  if (value) await store.change(() => undefined);
}

export function servicesLinkStore(): ServicesLinkStore {
  // Services has no plain-HTTP vault fallback. The existing account API allows only HTTPS or
  // a secure loopback context, and this continuation also requires authenticated encryption.
  if (!globalThis.isSecureContext || !globalThis.crypto?.subtle) throw Object.assign(new Error('secure_context_required'), {code: 'secure_context_required'});
  return new BrowserProtectedStore<ServicesLink>('portico.services-link.v1', 8192);
}

export async function forgetServicesLink(store: ServicesLinkStore, expected: ServicesLink) {
  await store.change(current => current?.kind === expected.kind && current.value === expected.value && current.createdAt === expected.createdAt ? undefined : current);
}
