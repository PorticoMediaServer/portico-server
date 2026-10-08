/**
 * A33 (Hosted): a verification link confirms by its token alone, from any browser.
 * `POST /v1/email-links/confirm {token}` → `{kind, status}`. The bound flows (the same browser
 * still holding the registration, sign-in or account session) are tried first by the page.
 */
export type EmailLinkKind = 'registration' | 'oidc_contact' | 'email_change';
export type EmailLinkStatus = 'confirmed' | 'email_changed' | 'already_confirmed';
export type EmailLinkResult = Readonly<{kind: EmailLinkKind; status: EmailLinkStatus}>;
export const EMAIL_LINK_CONFIRM_PATH = '/v1/email-links/confirm';

export function parseEmailLinkResult(v: unknown): EmailLinkResult {
  const o = v as {kind?: unknown; status?: unknown} | null;
  if (!o || typeof o !== 'object') throw new Error('invalid email link response');
  const kind = o.kind, status = o.status;
  if (kind !== 'registration' && kind !== 'oidc_contact' && kind !== 'email_change') throw new Error('invalid email link kind');
  if (status !== 'confirmed' && status !== 'email_changed' && status !== 'already_confirmed') throw new Error('invalid email link status');
  return Object.freeze({kind, status});
}

export async function confirmEmailLink(request: (path: string, method: string, body: unknown, signal?: AbortSignal) => Promise<unknown>, token: string, signal?: AbortSignal): Promise<EmailLinkResult> {
  if (!token || token.length > 4096) throw new Error('invalid email link token');
  return parseEmailLinkResult(await request(EMAIL_LINK_CONFIRM_PATH, 'POST', {token}, signal));
}

/** What the page says: an email change means signing in again; anything else is confirmed. */
export const emailLinkOutcome = (r: EmailLinkResult): 'changed' | 'confirmed' => (r.status === 'email_changed' ? 'changed' : 'confirmed');
