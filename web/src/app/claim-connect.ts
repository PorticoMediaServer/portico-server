import {useCallback, useRef, useState} from 'react';
import {parseClaimStatus, type ClaimExpected, type ClaimStatus} from '@core/claim-onboarding.ts';
import type {HttpLocalApi, ServerPin} from '@core/index.ts';

/**
 * Connecting a server to a Portico Account with a claim code. One flow for every client:
 *
 *   1. The server prepares an unbound claim (`/v1/networking/claim/prepare`, no account:
 *      BE-hosted d37e441, Plex-style; whoever approves the code becomes the owner), then registers a signed pending claim
 *      with Hosted (`/v1/networking/claim/web-approval`), which answers with a
 *      49-character claim code and `approvalUrl`
 *      (`https://web.getportico.tv/claim#code=…&server=…&return=…`).
 *   2. This page opens `approvalUrl` (it returns to this browser's own origin); the person
 *      signs in there if needed and approves or denies. That site never talks to this page.
 *   3. Meanwhile this page holds one request open
 *      (`/v1/networking/claim/await-approval`): Hosted pushes the decision to
 *      the server, and the server completes the claim before answering. No
 *      polling, no credentials or envelopes cross origins.
 */
export type WebClaimStatus = ClaimStatus & {approvalUrl?: string; claimCode?: string};
type Api = Pick<HttpLocalApi, 'request'>;

const post = async (api: Api, pin: ServerPin, action: string, body: unknown, signal?: AbortSignal): Promise<WebClaimStatus> => {
  const raw = await api.request<Record<string, unknown>>(`/v1/networking/claim/${action}`, 'POST', body, signal);
  const status = parseClaimStatus(raw, pin) as WebClaimStatus;
  const approvalUrl = typeof raw?.approvalUrl === 'string' ? raw.approvalUrl : undefined;
  const claimCode = typeof raw?.claimCode === 'string' ? raw.claimCode : undefined;
  return {...status, ...(approvalUrl ? {approvalUrl} : {}), ...(claimCode ? {claimCode} : {})};
};

/** Only an approval page on the Portico Account site may be opened. */
export function safeApprovalUrl(url: string | undefined, hostedOrigin: string): string | undefined {
  if (!url) return undefined;
  try {
    const u = new URL(url);
    const hosted = new URL(hostedOrigin);
    return u.origin === hosted.origin && u.pathname === '/claim' && /(^|&)code=/.test(u.hash.slice(1)) ? u.href : undefined;
  } catch { return undefined; }
}

/** Steps 1 and 2: prepare (when needed) and register the claim; returns the approval URL and the expected operation. */
export async function beginWebClaim(api: Api, pin: ServerPin, signal?: AbortSignal): Promise<{status: WebClaimStatus; expected: ClaimExpected}> {
  let status = parseClaimStatus(await api.request('/v1/networking/claim', 'GET', undefined, signal), pin) as WebClaimStatus;
  // No account here: the approver on web.getportico.tv becomes the owner.
  if (status.actions.includes('prepare')) status = await post(api, pin, 'prepare', {expected: status.identity}, signal);
  if (!status.operation) throw new ClaimError('claim_unavailable');
  const expected = status.operation;
  status = await post(api, pin, 'web-approval', {expected}, signal);
  return {status, expected: status.operation ?? expected};
}

/** Step 3: wait for the decision (the server completes the claim), then take any remaining step. */
export async function awaitWebClaim(api: Api, pin: ServerPin, expected: ClaimExpected, signal?: AbortSignal): Promise<WebClaimStatus> {
  // The server answers with the final claim status, or `{state: 'approval_closed' | 'approval_changed'}`
  // when the person denied, the code expired, or the claim changed underneath.
  const raw = await api.request<Record<string, unknown>>('/v1/networking/claim/await-approval', 'POST', {expected}, signal);
  if (raw?.state === 'approval_closed' || raw?.state === 'approval_changed') throw new ClaimError(raw.state);
  let status = parseClaimStatus(raw, pin) as WebClaimStatus;
  if (status.operation && status.actions.includes('continue') && !status.approvalRequired) status = await post(api, pin, 'continue', {expected: status.operation}, signal);
  return status;
}

/**
 * The catalogue message for a failed connection (BE-hosted d37e441): 400 `invalid_request`,
 * 429 when more than 3 claims are pending, 409 `claim_stale` (already decided or expired), and the
 * await's `approval_closed` / `approval_changed`. `undefined` means "use the shared presenter".
 */
export function claimErrorMessage(e: unknown): 'web.claim.errorInvalid' | 'web.claim.errorTooMany' | 'web.claim.errorStale' | 'web.claim.closed' | 'web.claim.errorUnavailable' | undefined {
  const code = (e as {code?: string} | undefined)?.code, status = (e as {status?: number} | undefined)?.status;
  if (code === 'approval_closed' || code === 'approval_changed') return 'web.claim.closed';
  if (code === 'claim_stale' || status === 409) return 'web.claim.errorStale';
  if (status === 429) return 'web.claim.errorTooMany';
  if (code === 'invalid_request' || status === 400) return 'web.claim.errorInvalid';
  if (code === 'claim_unavailable') return 'web.claim.errorUnavailable';
  return undefined;
}

export class ClaimError extends Error {
  code: string;
  constructor(code: string) { super(code); this.code = code; }
}

export type ClaimConnectPhase = 'idle' | 'starting' | 'waiting' | 'done';

/**
 * React binding for the server's Connect button. No Portico Account is needed here: the
 * person signs in on the approval page, and their account becomes the owner.
 */
export function useClaimConnect(api: Api | undefined, pin: ServerPin | undefined, hostedOrigin: string, onConnected?: () => void) {
  const [phase, setPhase] = useState<ClaimConnectPhase>('idle');
  const [error, setError] = useState<unknown>();
  const [approvalUrl, setApprovalUrl] = useState<string>();
  const run = useRef<AbortController | null>(null);
  const connect = useCallback(async () => {
    if (!api || !pin || run.current) return;
    const controller = new AbortController();
    run.current = controller;
    setError(undefined);
    setPhase('starting');
    // Open the window during the click, so pop-up blockers allow it; point it at the approval page once known.
    const win = typeof window !== 'undefined' ? window.open('', 'portico-claim') : null;
    try {
      const {status, expected} = await beginWebClaim(api, pin, controller.signal);
      const url = safeApprovalUrl(status.approvalUrl, hostedOrigin);
      if (!url) throw new ClaimError('claim_unavailable');
      setApprovalUrl(url);
      if (win && !win.closed) { win.opener = null; win.location.href = url; }
      setPhase('waiting');
      const final = await awaitWebClaim(api, pin, expected, controller.signal);
      setPhase(final.installationAcknowledged || final.state === 'claimed' || final.state === 'installed' ? 'done' : 'idle');
      onConnected?.();
    } catch (e) {
      if (!controller.signal.aborted) { setError(e); setPhase('idle'); }
      if (win && !win.closed && !approvalUrlSet(win)) win.close();
    } finally {
      run.current = null;
    }
  }, [api, pin, hostedOrigin, onConnected]);
  const cancel = useCallback(() => { run.current?.abort(); run.current = null; setPhase('idle'); }, []);
  return {connect, cancel, phase, busy: phase === 'starting' || phase === 'waiting', error, approvalUrl};
}

function approvalUrlSet(win: Window): boolean {
  try { return win.location.href !== 'about:blank'; } catch { return true; }
}
