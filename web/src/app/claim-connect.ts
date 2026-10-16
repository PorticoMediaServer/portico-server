import {useCallback, useRef, useState} from 'react';
import type {HttpLocalApi, ServerPin} from '@core/index.ts';
import {awaitWebClaim, beginWebClaim, ClaimError, safeApprovalUrl} from '@core/claim-web.ts';

// The claim-code flow is shared with the apps (client-core `claim-web.ts`); this adds the browser's window.
export {awaitWebClaim, beginWebClaim, ClaimError, claimErrorMessage, safeApprovalUrl, type WebClaimStatus} from '@core/claim-web.ts';

type Api = Pick<HttpLocalApi, 'request'>;

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
