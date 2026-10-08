import {useEffect, useState} from 'react';

/**
 * Link contract (Spec — Link Contract): secrets (tokens, invite codes, claim
 * codes) ride in the URL fragment, never the query. A page reads them once and
 * then removes the fragment from the address bar and history, after mounting
 * (the router listens to history, so not during render).
 */
export function readFragment<K extends string>(names: readonly K[], hash = typeof location === 'undefined' ? '' : location.hash): Partial<Record<K, string>> {
  const params = new URLSearchParams(hash.startsWith('#') ? hash.slice(1) : hash);
  const out: Partial<Record<K, string>> = {};
  for (const name of names) { const v = params.get(name); if (v) out[name] = v; }
  return out;
}

export function scrubFragment() {
  if (typeof location !== 'undefined' && location.hash) history.replaceState(history.state, '', location.pathname + location.search);
}

/** Read once, scrub after mount. */
export function useFragmentSecrets<K extends string>(names: readonly K[]): Partial<Record<K, string>> {
  const [values] = useState(() => readFragment(names));
  useEffect(() => { scrubFragment(); }, []);
  return values;
}

/**
 * `/verify-email#token=…`: base64url of the JSON tuple `["1", kind, resource, code]`
 * (Hosted `mailoutbox.VerificationToken`). `kind` is registration | oidc_contact | email_change.
 */
export type VerificationLink = {kind: 'registration' | 'oidc_contact' | 'email_change'; resource: string; code: string};
export function decodeVerificationToken(token: string | undefined): VerificationLink | null {
  if (!token || token.length > 2048) return null;
  try {
    const b64 = token.replace(/-/g, '+').replace(/_/g, '/');
    const json = typeof atob === 'function' ? atob(b64 + '='.repeat((4 - (b64.length % 4)) % 4)) : Buffer.from(b64, 'base64').toString('utf8');
    const value = JSON.parse(json) as unknown;
    if (!Array.isArray(value) || value.length !== 4 || value[0] !== '1') return null;
    const [, kind, resource, code] = value as string[];
    if (kind !== 'registration' && kind !== 'oidc_contact' && kind !== 'email_change') return null;
    if (typeof resource !== 'string' || !/^[A-Za-z0-9_-]{1,128}$/.test(resource) || typeof code !== 'string' || !code || code.length > 64) return null;
    return {kind, resource, code};
  } catch { return null; }
}

/**
 * `/together/join#code=…`: the code waits for the Together screen, which may be a sign-in (and
 * a Hosted redirect) away, so it is kept in this tab's session storage for 15 minutes and taken once.
 */
const TOGETHER_KEY = 'portico.together.pendingCode';
export function normalizeTogetherCode(raw: string | undefined): string | undefined {
  if (!raw || !/^[A-Za-z0-9 -]{4,16}$/.test(raw)) return undefined;
  return raw.replace(/[\s-]/g, '').toUpperCase();
}
export function setPendingTogetherCode(code: string) {
  try { sessionStorage.setItem(TOGETHER_KEY, JSON.stringify({code, at: Date.now()})); } catch {}
}
export function takePendingTogetherCode(): string | undefined {
  try {
    const raw = sessionStorage.getItem(TOGETHER_KEY);
    sessionStorage.removeItem(TOGETHER_KEY);
    const value = raw ? JSON.parse(raw) as {code?: unknown; at?: unknown} : undefined;
    return value && typeof value.code === 'string' && typeof value.at === 'number' && Date.now() - value.at < 15 * 60_000 ? normalizeTogetherCode(value.code) : undefined;
  } catch { return undefined; }
}
