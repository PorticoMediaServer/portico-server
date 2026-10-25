import {Fragment, useEffect, useMemo, useRef, useState} from 'react';
import type {DeletionPreview, IdentityProvider, SecurityIntent, SecurityPurpose, SecurityResult, SecurityState} from '@core/hosted-security.ts';
import {passwordStrength} from '@core/password-strength.ts';
import {ApiError} from '@core/index.ts';
import {runAccountExport} from '@core/account-notifications.ts';
import {AuthCodeBoxes, AuthPasswordField} from '../kit/accounts';
import {ServicesAuthenticator, ServicesBadge, ServicesDangerAction, ServicesGroup, ServicesRecoveryCodes, ServicesRowNote, ServicesWorkspaceRow} from '../kit/accounts/ServicesWorkspace';
import {HostedError, ServicesLabel, ServicesNotice, ServicesQuiet, ServicesStrength} from '../kit/accounts/ServicesAuth';
import {Button, TextField} from '../kit/controls';
import {Dialog} from '../kit/overlays';
import {currentAccountWork, type AccountIdentity, type ServicesAccount} from './session';
import {servicesI18n} from './i18n';
import type {WorkspaceApi} from './workspace-api';
import styles from './services.module.css';

type Task = 'password_set_or_change' | 'email_change' | 'mfa_enroll' | 'mfa_disable' | 'recovery_codes';
export function SecurityPanel({account, api, identity, username, error: parentError}: {account: ServicesAccount; api: WorkspaceApi; identity: AccountIdentity; username: string; error?: unknown}) {
  const {t} = useMemo(servicesI18n, []);
  const [state, setState] = useState<SecurityState>(), [offered, setOffered] = useState<IdentityProvider[]>([]);
  const [error, setError] = useState<unknown>(), [task, setTask] = useState<Task>(), [deleting, setDeleting] = useState(false);
  const [disconnecting, setDisconnecting] = useState<{id: string; provider: IdentityProvider}>(), [busy, setBusy] = useState(false);
  const [exporting, setExporting] = useState(false), [exportPending, setExportPending] = useState(false);
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
  const download = async () => {
    if (exporting || !life.current) return;
    const signal = life.current.signal; setExporting(true); setError(undefined);
    try {
      const value = await runAccountExport(api.notifications, crypto.randomUUID(), {signal});
      if (signal.aborted) return;
      if (value.state === 'pending') setExportPending(true);
      else downloadJSON(value.data);
    } catch (value) { if (!signal.aborted) setError(value); }
    finally { if (!signal.aborted) setExporting(false); }
  };
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
      <ServicesGroup title={t('services.security.yourData')}><ServicesWorkspaceRow phoneIcon="i-file" title={t('web.account.export.button')} help={t('web.account.export.help')}><Button size="small" icon="i-download" loading={exporting} onClick={() => void download()}>{t('web.account.export.button')}</Button></ServicesWorkspaceRow></ServicesGroup>
      {exportPending && <ServicesQuiet>{t('web.account.export.preparing')}</ServicesQuiet>}
      <ServicesGroup title={t('web.deleteAccount.title')} danger><ServicesWorkspaceRow phoneIcon="i-trash" title={t('web.deleteAccount.row')} help={t('web.deleteAccount.help')}><ServicesDangerAction compact icon="i-trash" onClick={() => setDeleting(true)}>{t('web.deleteAccount.action')}</ServicesDangerAction></ServicesWorkspaceRow></ServicesGroup>
      {task && <SecurityTask key={task} account={account} identity={identity} state={state} task={task} onClose={() => { setTask(undefined); void api.security(life.current?.signal).then(setState, setError); }} />}
      {deleting && <DeleteAccount account={account} identity={identity} target={state.contactAddress || username} onClose={() => setDeleting(false)} />}
    </>}
    <Dialog open={Boolean(disconnecting)} onOpenChange={open => { if (!open && !busy) setDisconnecting(undefined); }} title={t('web.hostedSecurity.disconnectTitle', {provider: disconnecting ? providerName(disconnecting.provider) : ''})}
      description={t('web.hostedSecurity.disconnectBody', {provider: disconnecting ? providerName(disconnecting.provider) : ''})} footer={<><Button size="small" disabled={busy} onClick={() => setDisconnecting(undefined)}>{t('action.cancel')}</Button><ServicesDangerAction loading={busy} onClick={() => void disconnect()}>{t('web.hostedSecurity.disconnect')}</ServicesDangerAction></>}><></></Dialog>
  </>;
}

function SecurityTask({account, identity, state, task, onClose}: {account: ServicesAccount; identity: AccountIdentity; state: SecurityState; task: Task; onClose: () => void}) {
  const {t} = useMemo(servicesI18n, []), [operationId] = useState(() => crypto.randomUUID());
  const [password, setPassword] = useState(''), [factor, setFactor] = useState(''), [value, setValue] = useState(''), [code, setCode] = useState('');
  const [result, setResult] = useState<SecurityResult>(), [error, setError] = useState<unknown>(), [busy, setBusy] = useState(false);
  const life = useRef<AbortController | null>(null);
  useEffect(() => { life.current = new AbortController(); return () => { life.current?.abort(); }; }, []);
  const codes = result?.recoveryCodes, confirm = result?.challengeId;
  const title = t(codes ? 'web.hostedSecurity.newCodesTitle' : confirm ? task === 'mfa_enroll' ? 'web.hostedSecurity.addToAuthenticator' : 'web.hostedSecurity.checkEmail' : task === 'password_set_or_change' ? 'account.changePasswordTitle' : task === 'email_change' ? 'web.hostedSecurity.changeEmailTitle' : task === 'mfa_disable' ? 'web.twoStep.turnOffConfirmTitle' : task === 'mfa_enroll' ? 'web.twoStep.turnOnTitle' : 'web.hostedSecurity.newCodesTitle');
  const submit = async () => {
    if (busy || !life.current) return;
    const signal = life.current.signal; setBusy(true); setError(undefined);
    try {
      const intent: SecurityIntent = {purpose: task as SecurityPurpose, target: task === 'email_change' ? value.trim() : '', operationId};
      const proof = confirm ? undefined : await currentAccountWork(account, session =>
        account.client.passwordProof(intent, password, factor, identity, session.accessToken, signal), identity);
      // A proof response may have arrived after another tab changed the account. Acquire and
      // check the current family again before the separate mutation is allowed to leave.
      const output = await currentAccountWork(account, session => confirm ?
        account.client.confirm(task === 'mfa_enroll' ? 'mfa' : 'email', confirm, code, operationId, identity, session.accessToken, signal) :
        account.client.action(intent, proof!.proof, task === 'password_set_or_change' ? value : '', identity, session.accessToken, signal), identity);
      if (signal.aborted) return;
      setPassword(''); setFactor('');
      if (output.challengeId || output.recoveryCodes) setResult(output); else onClose();
    } catch (value) { if (!signal.aborted) setError(value); }
    finally { if (!signal.aborted) setBusy(false); }
  };
  const ready = confirm ? code.length === 6 : Boolean(password) && (!state.mfaEnabled || Boolean(factor.trim())) && (task === 'password_set_or_change' ? passwordStrength(value).acceptable : task !== 'email_change' || /^\S+@\S+\.\S+$/.test(value.trim()));
  const commitLabel = t(confirm ? 'web.hostedSecurity.confirm' : task === 'password_set_or_change' ? 'account.changePasswordTitle' : task === 'email_change' ? 'services.security.sendCode' : task === 'mfa_disable' ? 'web.hostedSecurity.turnOff' : 'action.continue');
  const commitAction = task === 'mfa_disable' && !confirm ? <ServicesDangerAction disabled={!ready} loading={busy} onClick={() => void submit()}>{commitLabel}</ServicesDangerAction>
    : <Button size="small" variant="primary" disabled={!ready} loading={busy} onClick={() => void submit()}>{commitLabel}</Button>;
  return <Dialog open onOpenChange={open => { if (!open && !busy) onClose(); }} title={title} showCloseButton={false}
    footer={codes ? <Button size="small" variant="primary" onClick={onClose}>{t('action.done')}</Button> : <><Button size="small" disabled={busy} onClick={onClose}>{t('action.cancel')}</Button>{commitAction}</>}>
    {error ? <HostedError error={error} /> : null}
    {codes ? <><ServicesQuiet>{t('services.security.codesBody')}</ServicesQuiet><ServicesRecoveryCodes codes={codes}
      onCopy={() => void navigator.clipboard.writeText(codes.join('\n')).catch(setError)} onDownload={() => downloadText(codes.join('\n'), 'portico-recovery-codes.txt')} /></> :
      confirm ? <div className={styles.form}>{task === 'mfa_enroll' ? <><ServicesQuiet>{t('web.hostedSecurity.enterKey')}</ServicesQuiet>{result?.secret && <ServicesAuthenticator secret={result.secret} uri={result.otpAuthUri} onCopy={() => void navigator.clipboard.writeText(result.secret!).catch(setError)} onError={setError} />}</> : <ServicesQuiet>{t('web.hostedSecurity.sentCode', {address: value.trim()})}</ServicesQuiet>}<AuthCodeBoxes length={6} value={code} onChange={setCode} onComplete={() => {}} label={t('web.twoStep.code')} disabled={busy} /></div> :
      <form className={styles.form} onSubmit={event => { event.preventDefault(); if (ready) void submit(); }}>
        {task === 'password_set_or_change' && <ServicesQuiet>{t('web.hostedSecurity.othersSignedOut')}</ServicesQuiet>}
        {task === 'mfa_disable' && <ServicesQuiet>{t('services.security.disableBody')}</ServicesQuiet>}
        {task === 'email_change' && <ServicesQuiet>{t('services.security.emailPending')}</ServicesQuiet>}
        <AuthPasswordField label={t(task === 'password_set_or_change' ? 'account.currentPassword' : 'web.password')} autoComplete="current-password" value={password} onChange={event => setPassword(event.currentTarget.value)} disabled={busy} />
        {task === 'password_set_or_change' && <><AuthPasswordField label={<ServicesLabel detail={t('services.password.minimum')}>{t('auth.newPassword')}</ServicesLabel>} autoComplete="new-password" value={value} onChange={event => setValue(event.currentTarget.value)} disabled={busy} /><ServicesStrength password={value} id="security-strength" /></>}
        {task === 'email_change' && <TextField label={t('web.hostedSecurity.newEmail')} type="email" autoComplete="email" value={value} onChange={event => setValue(event.currentTarget.value)} description={t('services.security.emailCode')} disabled={busy} />}
        {state.mfaEnabled && <TextField label={t('services.security.authenticatorCode')} autoComplete="one-time-code" value={factor} onChange={event => setFactor(event.currentTarget.value)} disabled={busy} />}
      </form>}
  </Dialog>;
}

function DeleteAccount({account, identity, target, onClose}: {account: ServicesAccount; identity: AccountIdentity; target: string; onClose: () => void}) {
  const {t} = useMemo(servicesI18n, []), [operationId] = useState(() => crypto.randomUUID());
  const [preview, setPreview] = useState<DeletionPreview>(), [typed, setTyped] = useState(''), [error, setError] = useState<unknown>(), [busy, setBusy] = useState(false);
  const life = useRef<AbortController | null>(null);
  useEffect(() => {
    const controller = new AbortController(); life.current = controller;
    void currentAccountWork(account, session => account.client.deletionPreview(operationId, session.accessToken, controller.signal), identity).then(value => { if (!controller.signal.aborted) setPreview(value); }, value => { if (!controller.signal.aborted) setError(value); });
    return () => controller.abort();
  }, [account, identity, operationId]);
  const remove = async () => {
    if (!preview?.canDelete || !target || typed.trim().toLowerCase() !== target.toLowerCase() || busy || !life.current) return;
    const signal = life.current.signal; setBusy(true);
    try { await currentAccountWork(account, session => account.client.acceptDeletion(preview, typed, session.accessToken, signal), identity); if (!signal.aborted) await account.service.logout(); }
    catch (value) { if (!signal.aborted) setError(value); }
    finally { if (!signal.aborted) setBusy(false); }
  };
  return <Dialog open onOpenChange={open => { if (!open && !busy) onClose(); }} title={t('web.deleteAccount.confirmTitle')} description={t('web.deleteAccount.help')} showCloseButton={false}
    footer={<><Button size="small" disabled={busy} onClick={onClose}>{t('action.cancel')}</Button>{preview?.ownedServers.length ? <Button size="small" variant="primary" onClick={() => location.assign('/account?tab=servers')}>{t('account.delete.manageServers')}</Button> : null}{preview?.canDelete && <ServicesDangerAction icon="i-trash" loading={busy} disabled={!target || typed.trim().toLowerCase() !== target.toLowerCase()} onClick={() => void remove()}>{t('web.deleteAccount.confirmAction')}</ServicesDangerAction>}</>}>
    {error ? <HostedError error={error} /> : null}
    {!preview && !error && <ServicesQuiet>{t('web.deleteAccount.checking')}</ServicesQuiet>}
    {preview?.ownedServers.length ? <ServicesNotice tone="info">{t('services.security.stillOwn', {servers: preview.ownedServers.map(server => server.name).join(', ')})}</ServicesNotice> : preview?.canDelete ? <><ServicesQuiet>{t('web.deleteAccount.member', {count: preview.affectedServers.length})}</ServicesQuiet><TextField label={t('web.deleteAccount.typeLabel', {value: target})} value={typed} onChange={event => setTyped(event.currentTarget.value)} disabled={busy} /></> : preview ? <ServicesNotice>{t('web.deleteAccount.cannot')}</ServicesNotice> : null}
  </Dialog>;
}
function downloadText(value: string, filename: string, type = 'text/plain') {
  const url = URL.createObjectURL(new Blob([value], {type}));
  const link = document.createElement('a'); link.href = url; link.download = filename; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
}
function downloadJSON(value: unknown) { downloadText(JSON.stringify(value, null, 2), 'portico-account.json', 'application/json'); }
