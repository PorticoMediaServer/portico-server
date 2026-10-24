import {useEffect, useMemo, useRef, useState, useSyncExternalStore, type RefObject} from 'react';
import {defaultI18n} from '@i18n';
import type {ServerShortInvitation} from '@core/direct-account.ts';
import {Button} from '../controls/Button';
import {Dialog} from '../overlays/Dialog';
import {AuthError} from './AuthStage';
import {ShortCodeIssuer, type ShortCodeRun} from './invitation-short-code';
import styles from './invitation-short-code.module.css';

const t = defaultI18n.t;
export type ShortCodeInvitation = Readonly<{id: string; revision: number; state: 'pending' | 'accepted' | 'revoked' | 'expired'}>;
export type ShortCodeRequest = (operationId: string, signal: AbortSignal, isCurrent: () => boolean) => Promise<ServerShortInvitation>;
export type InvitationShortCodeDialogProps = Readonly<{
  open: boolean;
  onOpenChange: (open: boolean) => void;
  invitation: ShortCodeInvitation;
  request: ShortCodeRequest;
  /** True only for the explicit creation that opened this card. */
  issueOnOpen?: boolean;
  createOperationId?: () => string;
  returnFocusRef?: RefObject<HTMLElement | null>;
}>;

/**
 * The one explicit short-code issuance surface. A code is shown only from the
 * receipt that carries it; a replay without a code stays an absence and never
 * triggers a second operation. New code is the only thing that rotates it.
 */
export function InvitationShortCodeDialog(props: InvitationShortCodeDialogProps) {
  const identity = `${props.invitation.id}/${props.invitation.revision}`;
  return <IssuerDialog key={identity} {...props} identity={identity} />;
}

type IssuerDialogProps = InvitationShortCodeDialogProps & Readonly<{identity: string}>;
function IssuerDialog({open, onOpenChange, request, issueOnOpen = false, createOperationId, returnFocusRef, identity}: IssuerDialogProps) {
  const createRef = useRef(createOperationId);
  createRef.current = createOperationId;
  const issuer = useMemo(() => new ShortCodeIssuer(createRef.current), []);
  const state = useSyncExternalStore(issuer.subscribe, issuer.getSnapshot, issuer.getSnapshot);
  const [copied, setCopied] = useState(false);
  const alive = useRef(true), openRef = useRef(open), requestRef = useRef(request), runRef = useRef<ShortCodeRun>(() => Promise.reject(new Error('invitation_operation_unavailable')));
  const autoIssued = useRef<string | undefined>(undefined);
  openRef.current = open; requestRef.current = request;
  runRef.current = (operationId, signal) => requestRef.current(operationId, signal, () => alive.current && openRef.current);

  useEffect(() => {
    alive.current = true;
    return () => {alive.current = false;issuer.dispose();};
  }, [issuer]);
  useEffect(() => {
    if (!open) {issuer.reset();setCopied(false);return;}
    if (!issueOnOpen || autoIssued.current === identity) return;
    autoIssued.current = identity;
    void issuer.issue(true, runRef.current);
  }, [open, identity, issueOnOpen, issuer]);
  useEffect(() => {if (state.phase !== 'ready') setCopied(false);}, [state.phase]);

  const busy = state.phase === 'issuing';
  const copy = () => {
    if (state.phase !== 'ready') return;
    if (!navigator.clipboard) {setCopied(false);return;}
    void navigator.clipboard.writeText(state.code).then(() => {if (alive.current && openRef.current) setCopied(true);});
  };
  const expiry = state.phase === 'ready' || state.phase === 'unavailable'
    ? new Intl.DateTimeFormat(undefined, {dateStyle: 'medium'}).format(new Date(state.expiresAt)) : undefined;
  return <Dialog open={open} onOpenChange={value => {if (!busy) onOpenChange(value);}} title={t('invite.readyTitle')}
    description={t('invite.readyBody')} size="sm" width={540} returnFocusRef={returnFocusRef}
    footer={<Button variant="primary" onClick={() => onOpenChange(false)}>{t('action.done')}</Button>}>
    <div className={styles.body} data-phase={state.phase}>
      <div className={styles.codeArea} aria-live="polite">
        {state.phase === 'ready' ? <><p className={styles.code}>{state.code}</p>
          <p className={styles.help}>{t('web.access.shortCodeShownOnce')}</p></> :
          state.phase === 'issuing' ? <p className={styles.pending} role="status">{t('web.directJoin.checking')}</p> :
          state.phase === 'unavailable' ? <p className={styles.help}>{t('web.access.shortCodeUnavailable')}</p> :
          state.phase === 'failed' ? <AuthError error={state.error} /> :
          <p className={styles.help}>{t('web.access.shortCodeHelp')}</p>}
      </div>
      {expiry && <p className={styles.expiry}>{t('api.credential.expires', {when: expiry})}</p>}
      <div className={styles.actions}>
        {state.phase === 'ready' && <Button variant="glass" icon={copied ? 'i-check' : 'i-copy'} onClick={copy} disabled={busy}>{t(copied ? 'web.together.copied' : 'web.access.copyShortCode')}</Button>}
        {state.phase === 'failed' && <Button variant="glass" onClick={() => void issuer.retry(runRef.current)} disabled={busy}>{t('action.tryAgain')}</Button>}
        {!busy && <Button variant="glass" icon="i-plus" onClick={() => void issuer.issue(true, runRef.current)}>{t('web.access.newShortCode')}</Button>}
      </div>
    </div>
  </Dialog>;
}
