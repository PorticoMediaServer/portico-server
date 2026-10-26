import {Fragment, useLayoutEffect, useEffect, useMemo, useRef, useState} from 'react';
import type {DeletionPreview, IdentityProvider, SecurityIntent, SecurityPurpose, SecurityResult, SecurityState} from '@core/hosted-security.ts';
import {passwordStrength} from '@core/password-strength.ts';
import {ApiError} from '@core/index.ts';
import {AuthCodeBoxes, AuthPasswordField, AuthLinks} from '../kit/accounts';
import {ServicesAuthenticator, ServicesBadge, ServicesDangerAction, ServicesGroup, ServicesRecoveryCodes, ServicesRowNote, ServicesWorkspaceRow} from '../kit/accounts/ServicesWorkspace';
import {HostedError, ServicesLabel, ServicesNotice, ServicesQuiet, ServicesRegistrationLead, ServicesStrength, ServicesAuthStage, ServicesHeading, ServicesActions} from '../kit/accounts/ServicesAuth';
import {Button, TextField} from '../kit/controls';
import {Dialog} from '../kit/overlays';
import {accountIdentity, sameAccount, currentAccountWork, type AccountIdentity, type ServicesAccount} from './session';
import {servicesI18n} from './i18n';
import type {WorkspaceApi} from './workspace-api';
import styles from './services.module.css';
import {AccountExportRow} from './AccountExportRow';

export type HostedSecurityTaskKind = Task;
type Task = 'password_set_or_change' | 'email_change' | 'mfa_enroll' | 'mfa_disable' | 'recovery_codes';
export function SecurityPanel({account, api, identity, username, onDeleted, onSecurityEnded, error: parentError}: {account: ServicesAccount; api: WorkspaceApi; identity: AccountIdentity; username: string; onDeleted?: () => void; onSecurityEnded?: (codes?: readonly string[]) => void; error?: unknown}) {
  const {t} = useMemo(servicesI18n, []);
  const [state, setState] = useState<SecurityState>(), [offered, setOffered] = useState<IdentityProvider[]>([]);
  const [error, setError] = useState<unknown>(), [task, setTask] = useState<Task>(), [deleting, setDeleting] = useState(false);
  const [disconnecting, setDisconnecting] = useState<{id: string; provider: IdentityProvider}>(), [busy, setBusy] = useState(false);
  const [connected, setConnected] = useState<IdentityProvider>();
  const life = useRef<AbortController | null>(null);
  const scope = identity.accountId + '/' + identity.familyId;
  useEffect(() => {
    const controller = new AbortController(); life.current = controller;
    void (async () => {
      const returning = new URLSearchParams(location.search).get('identityTransaction');
      if (returning && /^[A-Za-z0-9_-]{1,160}$/.test(returning)) {
        // This is the actual Hosted browser callback query, never a native completion header.
        const tx = await currentAccountWork(account, () => account.client.transaction(returning, controller.signal), identity);
        if (tx?.mode === 'link') {
          if (tx.status !== 'verified') throw new ApiError(409, 'provider_link_failed', 'provider_link_failed', false);
          await currentAccountWork(account, session => account.client.completeSecurity(tx.transactionId, '', identity, session.accessToken, controller.signal), identity);
          if (!controller.signal.aborted) { setConnected(tx.provider); history.replaceState(history.state, '', '/account?tab=security'); }
        }
      }
      const value = await api.security(controller.signal);
      if (!controller.signal.aborted) setState(value);
    })().catch(value => { if (!controller.signal.aborted) setError(value); });
    void account.client.options(controller.signal).then(value => { if (!controller.signal.aborted) setOffered(value.providers.filter(item => item.enabled).map(item => item.id)); }, () => {});
    return () => controller.abort();
  }, [api, account, scope, identity]);
  const work = async (operation: (token: string, signal: AbortSignal) => Promise<void>) => {
    if (busy || !life.current) return;
    const signal = life.current.signal; setBusy(true); setError(undefined);
    try { await currentAccountWork(account, session => operation(session.accessToken, signal), identity); }
    catch (value) { if (!signal.aborted) setError(value); }
    finally { if (!signal.aborted) setBusy(false); }
  };
  const providerName = (provider: IdentityProvider) => t(provider === 'apple' ? 'accounts.provider.apple' : 'accounts.provider.google');
  const connect = (provider: IdentityProvider) => work(async (token, signal) => {
    const started = await account.client.start(provider, {mode: 'link', requestId: crypto.randomUUID(), intent: {purpose: 'provider_link', target: provider, operationId: crypto.randomUUID()}}, signal, token);
    // The canonical client validates the provider URL. Recheck the initiating account before leaving.
    if (started.authorizationUrl) {
      await currentAccountWork(account, async () => undefined, identity);
      if (!signal.aborted) location.assign(started.authorizationUrl);
    }
  });
  const disconnect = () => work(async (token, signal) => {
    if (!disconnecting) return;
    await account.client.action({purpose: 'provider_unlink', target: disconnecting.id, operationId: crypto.randomUUID()}, undefined, '', identity, token, signal);
    const next = await api.security(signal);
    if (!signal.aborted) { setState(next); setDisconnecting(undefined); }
  });
  const onlyProvider = state && !state.hasPassword && state.providers.length === 1;
  return <>{parentError || error ? <HostedError error={parentError ?? error} /> : null}
    {connected && <ServicesNotice tone="success">{t('web.hostedSecurity.providerConnected', {provider: providerName(connected)})}</ServicesNotice>}
    {state && <>
      <ServicesGroup title={t('web.hostedSecurity.signingIn')}>
        <ServicesWorkspaceRow phoneIcon="i-key" title={t('web.password')} help={state.hasPassword ? t('web.hostedSecurity.passwordHelp') : state.providers.length ? t('web.hostedSecurity.noPassword', {providers: state.providers.map(value => providerName(value.provider)).join(', ')}) : t('web.hostedSecurity.noPasswordOther')}>
          {state.hasPassword && <Button size="small" onClick={() => setTask('password_set_or_change')}>{t('web.hostedSecurity.change')}</Button>}
        </ServicesWorkspaceRow>
        <ServicesWorkspaceRow phoneIcon="i-mail" title={t('web.hostedSecurity.email')} help={<>{state.contactAddress}{state.contactAddress && <br />}{t('services.security.emailHelp')}</>} helpPhone={state.contactAddress}>
          {state.hasPassword && <Button size="small" onClick={() => setTask('email_change')}>{t('web.hostedSecurity.change')}</Button>}
        </ServicesWorkspaceRow>
        {[...state.providers].sort((a, b) => a.provider.localeCompare(b.provider)).map(provider => <Fragment key={provider.id}><ServicesWorkspaceRow icon={provider.provider === 'apple' ? 'i-apple' : 'i-google'} title={t('web.hostedSecurity.signInWith', {provider: providerName(provider.provider)})}
          help={t('web.hostedSecurity.connected')}>
          <Button size="small" disabled={onlyProvider || busy} onClick={() => setDisconnecting(provider)}>{t('web.hostedSecurity.disconnect')}</Button>
        </ServicesWorkspaceRow>{onlyProvider && <ServicesRowNote>{t('web.hostedSecurity.lastMethod')}</ServicesRowNote>}</Fragment>)}
        {offered.filter(provider => !state.providers.some(item => item.provider === provider)).sort((a, b) => a.localeCompare(b)).map(provider => <ServicesWorkspaceRow key={provider} icon={provider === 'apple' ? 'i-apple' : 'i-google'} title={t('web.hostedSecurity.signInWith', {provider: providerName(provider)})}
          help={t('web.hostedSecurity.connectHelp', {provider: providerName(provider)})}><Button size="small" disabled={busy} onClick={() => void connect(provider)}>{t('web.hostedSecurity.connect')}</Button></ServicesWorkspaceRow>)}
      </ServicesGroup>
      {!state.hasPassword && <ServicesNotice tone="info">{t('web.hostedSecurity.providerOnly')}</ServicesNotice>}
      <ServicesGroup title={t('web.twoStep.title')} description={t('web.hostedSecurity.twoStepHelp')}>
        <ServicesWorkspaceRow phoneIcon={state.mfaEnabled ? 'i-shieldok' : 'i-shield'} phoneTone={state.mfaEnabled ? 'green' : undefined} title={t('web.twoStep.title')} badge={<ServicesBadge tone={state.mfaEnabled ? 'healthy' : 'default'}>{t(state.mfaEnabled ? 'web.hostedSecurity.on' : 'web.hostedSecurity.off')}</ServicesBadge>}
          help={state.mfaEnabled ? t('web.hostedSecurity.codesLeft', {count: state.recoveryCodesRemaining}) : t('web.hostedSecurity.recommended')}>
          {state.hasPassword && <Button size="small" onClick={() => setTask(state.mfaEnabled ? 'mfa_disable' : 'mfa_enroll')}>{t(state.mfaEnabled ? 'web.hostedSecurity.turnOff' : 'web.twoStep.turnOn')}</Button>}
        </ServicesWorkspaceRow>
        {state.mfaEnabled && state.hasPassword && <ServicesWorkspaceRow phoneIcon="i-key" title={t('web.hostedSecurity.recoveryCodes')} help={t('web.hostedSecurity.recoveryCodesHelp')}><Button size="small" onClick={() => setTask('recovery_codes')}>{t('web.hostedSecurity.makeCodes')}</Button></ServicesWorkspaceRow>}
      </ServicesGroup>
      <AccountExportRow account={account} api={api} identity={identity}/>
      <ServicesGroup title={t('web.deleteAccount.title')} danger><ServicesWorkspaceRow phoneIcon="i-trash" title={t('web.deleteAccount.row')} help={t('web.deleteAccount.help')}><ServicesDangerAction compact icon="i-trash" onClick={() => setDeleting(true)}>{t('web.deleteAccount.action')}</ServicesDangerAction></ServicesWorkspaceRow></ServicesGroup>
      {task && <SecurityTask key={task} account={account} identity={identity} state={state} task={task} onSecurityEnded={onSecurityEnded} onClose={() => { setTask(undefined); void api.security(life.current?.signal).then(setState, setError); }} />}
      {deleting && <DeleteAccount account={account} identity={identity} target={state.contactAddress || username} onDeleted={onDeleted} onClose={() => setDeleting(false)} />}
    </>}
    <Dialog open={Boolean(disconnecting)} onOpenChange={open => { if (!open && !busy) setDisconnecting(undefined); }} stackFooter className={styles.overlaySpacing} title={t('web.hostedSecurity.disconnectTitle', {provider: disconnecting ? providerName(disconnecting.provider) : ''})}
      description={t('web.hostedSecurity.disconnectBody', {provider: disconnecting ? providerName(disconnecting.provider) : ''})} footer={<><Button size="small" disabled={busy} onClick={() => setDisconnecting(undefined)}>{t('action.cancel')}</Button><ServicesDangerAction loading={busy} onClick={() => void disconnect()}>{t('web.hostedSecurity.disconnect')}</ServicesDangerAction></>}><></></Dialog>
  </>;
}

function SecurityTask({account, identity, state, task, onClose, onSecurityEnded, isCurrent}: {account: ServicesAccount; identity: AccountIdentity; state: SecurityState; task: Task; onClose: () => void; onSecurityEnded?: (codes?: readonly string[]) => void; isCurrent?: () => boolean}) {
  const {t} = useMemo(servicesI18n, []), [operationId] = useState(() => crypto.randomUUID());
  const [password, setPassword] = useState(''), [factor, setFactor] = useState(''), [value, setValue] = useState(''), [code, setCode] = useState('');
  const [result, setResult] = useState<SecurityResult>(), [error, setError] = useState<unknown>(), [busy, setBusy] = useState(false);
  type Flight = {controller: AbortController; pending: boolean};
  const life = useRef<Flight | null>(null), [flight, setFlight] = useState<Flight | null>(null);
  const resendOperation = useRef<string | undefined>(undefined);
  useLayoutEffect(() => {
    const active: Flight = {controller: new AbortController(), pending: false};
    life.current = active; setFlight(active); setBusy(false);
    return () => {if (life.current === active) life.current = null; active.controller.abort();};
  }, [account, identity.accountId, identity.familyId, isCurrent]);
  const parentCurrent = () => {try {return isCurrent?.() !== false;} catch {return false;}};
  const current = () => Boolean(flight && life.current === flight && !flight.controller.signal.aborted && parentCurrent());
  const assertCurrent = () => {if (!current()) throw new DOMException('Account security opening ended', 'AbortError');};
  const codes = result?.recoveryCodes, confirm = result?.challengeId;
  const edit = (write:(value:string)=>void,value:string) => {if(current()&&!flight?.pending)write(value);};
  const close = () => {if(!current()||flight?.pending)return;setPassword('');setFactor('');setValue('');setCode('');setResult(undefined);onClose();};
  const title = t(codes ? 'web.hostedSecurity.newCodesTitle' : confirm ? task === 'mfa_enroll' ? 'web.hostedSecurity.addToAuthenticator' : 'web.hostedSecurity.checkEmail' : task === 'password_set_or_change' ? 'account.changePasswordTitle' : task === 'email_change' ? 'web.hostedSecurity.changeEmailTitle' : task === 'mfa_disable' ? 'web.twoStep.turnOffConfirmTitle' : task === 'mfa_enroll' ? 'web.twoStep.turnOnTitle' : 'web.hostedSecurity.newCodesTitle');
  const submit = async () => {
    if (!flight || !current() || flight.pending) return;
    const signal = flight.controller.signal, proofPassword = password, proofFactor = factor, submittedValue = value, submittedCode = code;
    flight.pending = true; setBusy(true); setError(undefined);setPassword('');setFactor('');setCode('');
    if(task === 'password_set_or_change')setValue('');
    try {
      const intent: SecurityIntent = {purpose: task as SecurityPurpose, target: task === 'email_change' ? submittedValue.trim() : '', operationId};
      const proof = confirm ? undefined : await currentAccountWork(account, session =>
        {assertCurrent();return account.client.passwordProof(intent, proofPassword, proofFactor, identity, session.accessToken, signal);}, identity);
      // A proof response may have arrived after another tab changed the account. Acquire and
      // check the current family again before the separate mutation is allowed to leave.
      assertCurrent();
      const session = await currentAccountWork(account, async session => {assertCurrent();return session;}, identity);
      assertCurrent();
      signal.throwIfAborted();
      const output = confirm ?
        await account.client.confirm(task === 'mfa_enroll' ? 'mfa' : 'email', confirm, submittedCode, operationId, identity, session.accessToken, signal) :
        await account.client.action(intent, proof!.proof, task === 'password_set_or_change' ? submittedValue : '', identity, session.accessToken, signal);
      signal.throwIfAborted();
      if (output.signOut) {
        const current = account.service.getSnapshot().session;
        const original = current && sameAccount(accountIdentity(current), identity);
        const cleanup = account.service.logoutIfCurrent(identity);
        if (original && parentCurrent()) onSecurityEnded?.(output.recoveryCodes);
        await cleanup;
        return;
      }
      assertCurrent();
      await currentAccountWork(account, async () => {assertCurrent();}, identity);
      assertCurrent();
      if (signal.aborted) return;
      setPassword(''); setFactor('');
      if (output.challengeId || output.recoveryCodes) setResult(output); else onClose();
    } catch (value) { if (current()) setError(value); }
    finally {flight.pending = false; if (current()) setBusy(false);}
  };
  const resend = async () => {
    // This rendered handler owns this challenge. It never discovers another one
    // from a mutable service or successor dialog after an await.
    if (task !== 'email_change' || !confirm || !result?.expiresAt || Date.parse(result.expiresAt) <= Date.now() || result.resendsRemaining === 0 || !flight || !current() || flight.pending) return;
    const signal = flight.controller.signal, originalExpiry = result.expiresAt;
    const requestId = resendOperation.current ??= crypto.randomUUID();
    flight.pending = true; setBusy(true); setError(undefined);
    try {
      const output = await currentAccountWork(account, session => {
        signal.throwIfAborted();assertCurrent();
        return account.client.resendEmailChange(confirm, requestId, identity, session.accessToken, signal);
      }, identity);
      signal.throwIfAborted();assertCurrent();
      if (output.expiresAt !== originalExpiry) throw new Error('The account service returned a different email challenge.');
      if (resendOperation.current === requestId) resendOperation.current = undefined;
      setResult(output); setCode('');
    } catch (value) { if (current()) setError(value); }
    finally { flight.pending = false; if (current()) setBusy(false); }
  };
  const ready = confirm ? code.length === 6 : Boolean(password) && (!state.mfaEnabled || Boolean(factor.trim())) && (task === 'password_set_or_change' ? passwordStrength(value).acceptable : task !== 'email_change' || /^\S+@\S+\.\S+$/.test(value.trim()));
  const commitLabel = t(confirm ? 'web.hostedSecurity.confirm' : task === 'password_set_or_change' ? 'account.changePasswordTitle' : task === 'email_change' ? 'services.security.sendCode' : task === 'mfa_disable' ? 'web.hostedSecurity.turnOff' : 'action.continue');
  const commitAction = task === 'mfa_disable' && !confirm ? <ServicesDangerAction disabled={!current() || !ready} loading={busy} onClick={() => void submit()}>{commitLabel}</ServicesDangerAction>
    : <Button size="small" variant="primary" disabled={!current() || !ready} loading={busy} onClick={() => void submit()}>{commitLabel}</Button>;
  return <Dialog open onOpenChange={open => { if (!open) close(); }} className={styles.overlaySpacing} title={title} showCloseButton={false}
    footer={codes ? <Button size="small" variant="primary" onClick={close}>{t('action.done')}</Button> : <><Button size="small" disabled={busy} onClick={close}>{t('action.cancel')}</Button>{commitAction}</>}>
    {error ? <HostedError error={error} /> : null}
    {codes ? <><ServicesQuiet>{t('services.security.codesBody')}</ServicesQuiet><ServicesRecoveryCodes codes={codes}
      onCopy={() => {if(current())void navigator.clipboard.writeText(codes.join('\n')).catch(error => {if(current())setError(error);});}} onDownload={() => {if(current())downloadText(codes.join('\n'), 'portico-recovery-codes.txt');}} /></> :
      confirm ? <div className={styles.form}>{task === 'mfa_enroll' ? <><ServicesQuiet>{t('web.hostedSecurity.enterKey')}</ServicesQuiet>{result?.secret && <ServicesAuthenticator secret={result.secret} uri={result.otpAuthUri} onCopy={() => {if(current())void navigator.clipboard.writeText(result.secret!).catch(error => {if(current())setError(error);});}} onError={error=>{if(current())setError(error);}} />}</> : <ServicesRegistrationLead address={result?.maskedAddress ?? maskedEmail(value.trim())} message="web.hostedSecurity.sentCode" />}<AuthCodeBoxes length={6} value={code} onChange={value=>edit(setCode,value)} onComplete={() => {}} label={t('web.twoStep.code')} disabled={busy} />{task === 'email_change' && <AuthLinks><button type="button" disabled={!current() || busy || result?.resendsRemaining === 0} onClick={() => void resend()}>{t('auth.sendCodeAgain')}</button></AuthLinks>}</div> :
      <form className={styles.form} onSubmit={event => { event.preventDefault(); if (ready) void submit(); }}>
        {task === 'password_set_or_change' && <ServicesQuiet>{t('web.hostedSecurity.othersSignedOut')}</ServicesQuiet>}
        {task === 'mfa_disable' && <ServicesQuiet>{t('services.security.disableBody')}</ServicesQuiet>}
        {task === 'email_change' && <ServicesQuiet>{t('services.security.emailPending')}</ServicesQuiet>}
        <AuthPasswordField label={t(task === 'password_set_or_change' ? 'account.currentPassword' : 'web.password')} autoComplete="current-password" value={password} onChange={event => edit(setPassword,event.currentTarget.value)} disabled={busy} />
        {task === 'password_set_or_change' && <><AuthPasswordField label={<ServicesLabel detail={t('services.password.minimum')}>{t('auth.newPassword')}</ServicesLabel>} autoComplete="new-password" value={value} onChange={event => edit(setValue,event.currentTarget.value)} disabled={busy} /><ServicesStrength password={value} id="security-strength" /></>}
        {task === 'email_change' && <TextField label={t('web.hostedSecurity.newEmail')} type="email" autoComplete="email" value={value} onChange={event => edit(setValue,event.currentTarget.value)} description={t('services.security.emailCode')} disabled={busy} />}
        {state.mfaEnabled && <TextField label={task === 'password_set_or_change' ? <ServicesLabel detail={t('services.security.twoStepOnHint')}>{t('services.security.authenticatorCode')}</ServicesLabel> : t('services.security.authenticatorCode')} autoComplete="one-time-code" value={factor} onChange={event => edit(setFactor,event.currentTarget.value)} disabled={busy} />}
      </form>}
  </Dialog>;
}

function maskedEmail(address: string) {
  const split = address.lastIndexOf('@');
  return split > 0 ? Array.from(address.slice(0, split))[0] + '***' + address.slice(split) : '';
}

function DeleteAccount({account, identity, target, onClose, onDeleted}: {account: ServicesAccount; identity: AccountIdentity; target: string; onClose: () => void; onDeleted?: () => void}) {
  const {t} = useMemo(servicesI18n, []), [operationId] = useState(() => crypto.randomUUID());
  const [preview, setPreview] = useState<DeletionPreview>(), [typed, setTyped] = useState(''), [error, setError] = useState<unknown>(), [busy, setBusy] = useState(false);
  const life = useRef<AbortController | null>(null);
  useEffect(() => {
    const controller = new AbortController(); life.current = controller;
    void currentAccountWork(account, session => account.client.deletionPreview(operationId, session.accessToken, controller.signal), identity).then(value => { if (!controller.signal.aborted) setPreview(value); }, value => { if (!controller.signal.aborted) setError(value); });
    return () => controller.abort();
  }, [account, identity, operationId]);
  const pending = useRef(false);
  const remove = async () => {
    const flight = life.current, reviewed = preview, confirmation = typed;
    if (!reviewed?.canDelete || !target || confirmation.trim().toLowerCase() !== target.toLowerCase() || pending.current || !flight || flight.signal.aborted) return;
    const signal = flight.signal; pending.current = true; setBusy(true);
    try {
      // Obtain the exact family before the terminal request. A successful deletion
      // itself ends that family, so a post-request session refresh is not proof of failure.
      const session = await currentAccountWork(account, async session => session, identity);
      signal.throwIfAborted();
      const receipt = await account.client.acceptDeletion(reviewed, confirmation, session.accessToken, signal);
      signal.throwIfAborted();
      if (!receipt.accountDeleted || !receipt.deletionId || (receipt.status !== 'complete' && receipt.status !== 'pending-offline-servers')) throw new ApiError(400, 'invalid_response', 'invalid_response', false);
      const current = account.service.getSnapshot().session;
      if (current && sameAccount(accountIdentity(current), identity)) {
        const cleanup = account.service.logoutIfCurrent(identity);
        // logoutIfCurrent clears only this family synchronously before its secure
        // persistence awaits. The receipt-backed done view survives that unmount.
        onDeleted?.();
        await cleanup;
      }
    } catch (value) { if (!signal.aborted) setError(value); }
    finally { pending.current = false; if (!signal.aborted) setBusy(false); }
  };
  return <Dialog open onOpenChange={open => { if (!open && !busy) onClose(); }} className={styles.overlaySpacing} title={t('web.deleteAccount.confirmTitle')} description={t('web.deleteAccount.help')} showCloseButton={false} stackFooter
    footer={<><Button size="small" disabled={busy} onClick={onClose}>{t('action.cancel')}</Button>{preview?.ownedServers.length ? <Button size="small" variant="primary" onClick={() => location.assign('/account?tab=servers')}>{t('account.delete.manageServers')}</Button> : null}{preview?.canDelete && <ServicesDangerAction icon="i-trash" loading={busy} disabled={!target || typed.trim().toLowerCase() !== target.toLowerCase()} onClick={() => void remove()}>{t('web.deleteAccount.confirmAction')}</ServicesDangerAction>}</>}>
    {error ? <HostedError error={error} /> : null}
    {!preview && !error && <ServicesQuiet>{t('web.deleteAccount.checking')}</ServicesQuiet>}
    {preview?.ownedServers.length ? <ServicesNotice tone="info">{t('services.security.stillOwn', {servers: preview.ownedServers.map(server => server.name).join(', ')})}</ServicesNotice> : preview?.canDelete ? <><ServicesQuiet>{t('web.deleteAccount.member', {count: preview.affectedServers.length})}</ServicesQuiet><TextField label={t('web.deleteAccount.typeLabel', {value: target})} value={typed} onChange={event => setTyped(event.currentTarget.value)} disabled={busy} /></> : preview ? <ServicesNotice>{t('web.deleteAccount.cannot')}</ServicesNotice> : null}
  </Dialog>;
}
/** One receipt-backed, memory-only view remains after MFA ends the old family. */
export function SignedOutRecoveryCodes({codes, onDone}: {codes: readonly string[]; onDone: () => void}) {
  const {t} = servicesI18n(), [error, setError] = useState<unknown>();
  return <ServicesAuthStage variant="state"><ServicesHeading>{t('web.hostedSecurity.newCodesTitle')}</ServicesHeading>
    <ServicesQuiet>{t('services.security.codesBody')}</ServicesQuiet>{error ? <HostedError error={error}/> : null}
    <ServicesRecoveryCodes codes={codes} onCopy={() => void navigator.clipboard.writeText(codes.join('\n')).catch(setError)}
      onDownload={() => downloadText(codes.join('\n'), 'portico-recovery-codes.txt')}/>
    <ServicesActions><Button variant="primary" onClick={onDone}>{t('action.done')}</Button></ServicesActions>
  </ServicesAuthStage>;
}

function downloadText(value: string, filename: string, type = 'text/plain') {
  const url = URL.createObjectURL(new Blob([value], {type}));
  const link = document.createElement('a'); link.href = url; link.download = filename; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
}

/** Shared in-place task; a server Settings caller supplies its original opening guard. */
export {SecurityTask as HostedSecurityTask};
