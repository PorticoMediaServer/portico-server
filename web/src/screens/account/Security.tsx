import React, {useCallback, useEffect, useMemo, useState} from 'react';
import type {DeletionPreview, IdentityProvider, SecurityIntent, SecurityPurpose, SecurityResult, SecurityState} from '@core/hosted-security';
import {useSession} from '../../app/session';
import {accountSecurityClient} from '../../bridge/account-security-client';
import {accountApi, type AccountScope} from '../../bridge/account-api';
import {defaultI18n} from '@i18n';
import {ErrorNotice, errorText} from '../../app/errors';
import {Button, ConfirmDialog, Dialog, Input, Loading, Notice, SettingsGroup, SettingsPage, SettingsRow, Text, PasswordStrength} from '../../ui';
import {passwordStrength} from '@core/password-strength.ts';

const t = defaultI18n.t;

type Task = {purpose: SecurityPurpose; title: () => string; target?: 'email' | 'password'; confirm?: 'mfa' | 'email'};
const tasks = {
  password: {purpose: 'password_set_or_change', title: () => t('account.changePasswordTitle'), target: 'password'},
  email: {purpose: 'email_change', title: () => t('web.hostedSecurity.changeEmailTitle'), target: 'email', confirm: 'email'},
  enrol: {purpose: 'mfa_enroll', title: () => t('web.twoStep.turnOnTitle'), confirm: 'mfa'},
  disable: {purpose: 'mfa_disable', title: () => t('web.twoStep.turnOffTitle')},
  codes: {purpose: 'recovery_codes', title: () => t('web.hostedSecurity.newCodesTitle')},
} satisfies Record<string, Task>;
const statusOf = (e: unknown) => (e && typeof e === 'object' && typeof (e as {status?: unknown}).status === 'number' ? (e as {status: number}).status : undefined);
/** A proof that failed means the password or code was wrong; anything else goes through the presenter. */
const say = (e: unknown, wrong: string) => (statusOf(e) === 401 || statusOf(e) === 403 ? wrong : errorText(e, 'portico-account', 'save'));
const codeOf = (e: unknown) => (e && typeof e === 'object' && typeof (e as {code?: unknown}).code === 'string' ? (e as {code: string}).code : '');
const providerName = (p: IdentityProvider | string) => (p === 'google' ? 'Google' : 'Apple');
/** Connect and Disconnect: the refusals people can act on get their own words. */
const providerError = (e: unknown) => (codeOf(e) === 'last_method' ? t('web.hostedSecurity.lastMethod') : codeOf(e) === 'provider_unavailable' ? t('web.hostedSecurity.providerInUse') : errorText(e, 'portico-account', 'save'));

/**
 * Account security. Credential changes (password, email, two-step verification, recovery codes)
 * first prove the person at the keyboard holds the account (password, plus a code once two-step
 * verification is on); the proof is used once, for that change, and never kept. Everything else,
 * including deleting the account (confirmed by typing the email), needs only the signed-in
 * session (A80).
 */
export function AccountSecurity({scope, returning, onReturned}: {scope: AccountScope; returning?: string; onReturned?: () => void}) {
  const client = useMemo(() => accountSecurityClient(), []);
  const api = useMemo(() => accountApi(scope), [scope.accountId, scope.familyId]);
  const [state, setState] = useState<SecurityState>();
  const [error, setError] = useState<unknown>();
  const [notice, setNotice] = useState('');
  const [task, setTask] = useState<Task>();
  const [deleting, setDeleting] = useState(false);
  // Google and Apple (A80, NEW-13): connecting and disconnecting need the signed-in session only.
  const [offered, setOffered] = useState<IdentityProvider[]>([]);
  const [providerBusy, setProviderBusy] = useState('');
  const [providerFailure, setProviderFailure] = useState('');
  const [disconnecting, setDisconnecting] = useState<{id: string; provider: IdentityProvider}>();
  const load = useCallback(async () => {
    setError(undefined);
    try { setState(await client.state(scope, await api.token())); } catch (e) { setError(e ?? new Error()); }
  }, [client, api, scope.accountId, scope.familyId]);
  useEffect(() => { void load(); }, [load]);
  useEffect(() => {
    let live = true;
    client.options().then(o => { if (live) setOffered(o.providers.filter(p => p.enabled).map(p => p.id)); }, () => {});
    return () => { live = false; };
  }, [client]);
  // Back from the provider after Connect: finish the link this session started, once.
  useEffect(() => {
    if (!returning) return;
    let live = true;
    void (async () => {
      try {
        const tx = await client.transaction(returning);
        if (!tx || tx.mode !== 'link') return;
        if (tx.status === 'failed') throw Object.assign(new Error(), {code: 'provider_link_failed'});
        await client.completeSecurity(returning, '', scope, await api.token());
        if (!live) return;
        setNotice(t('web.hostedSecurity.providerConnected', {provider: providerName(tx.provider)}));
        void load();
      } catch (e) {
        if (live) setProviderFailure(codeOf(e) === 'provider_link_failed' ? t('web.hostedSecurity.connectFailed') : providerError(e));
      } finally {
        // Drop the transaction from the address only now: changing it earlier would end this effect.
        if (live) onReturned?.();
      }
    })();
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [returning]);
  const connect = async (provider: IdentityProvider) => {
    if (providerBusy) return;
    setProviderBusy(provider); setProviderFailure('');
    try {
      const started = await client.start(provider, {mode: 'link', requestId: crypto.randomUUID(), intent: {purpose: 'provider_link', target: provider, operationId: crypto.randomUUID()}}, undefined, await api.token());
      if (!started.authorizationUrl) throw Object.assign(new Error(), {code: 'provider_unavailable_now'});
      location.assign(started.authorizationUrl);
    } catch (e) {
      setProviderFailure(codeOf(e) === 'provider_unavailable_now' ? t('web.hostedSecurity.connectUnavailable') : providerError(e));
      setProviderBusy('');
    }
  };
  const disconnect = async () => {
    if (!disconnecting || providerBusy) return;
    setProviderBusy(disconnecting.id); setProviderFailure('');
    try {
      await client.action({purpose: 'provider_unlink', target: disconnecting.id, operationId: crypto.randomUUID()}, undefined, '', scope, await api.token());
      setNotice(t('web.hostedSecurity.providerDisconnected', {provider: providerName(disconnecting.provider)}));
      setDisconnecting(undefined);
      void load();
    } catch (e) {
      setProviderFailure(providerError(e));
      setDisconnecting(undefined);
    } finally { setProviderBusy(''); }
  };
  if (!state) return error ? <ErrorNotice error={error} context="portico-account" retry={() => void load()} /> : <Loading label={t('status.loadingThing', {thing: t('web.hostedSecurity.thing')})} />;
  const providers = new Intl.ListFormat(undefined, {type: 'disjunction'}).format(state.providers.map(p => providerName(p.provider)));
  // The server refuses removing the last way in; say so up front instead of offering it.
  const onlyWayIn = !state.hasPassword && state.providers.length === 1;
  const connectable = offered.filter(p => !state.providers.some(linked => linked.provider === p));
  return (
    <SettingsPage>
      {notice ? <Notice tone="success" compact>{notice}</Notice> : null}
      <SettingsGroup title={t('web.hostedSecurity.signingIn')}>
        <SettingsRow icon="lock" label={t('web.password')} help={state.hasPassword ? t('web.hostedSecurity.passwordHelp') : state.providers.length ? t('web.hostedSecurity.noPassword', {providers}) : t('web.hostedSecurity.noPasswordOther')} control={state.hasPassword ? <Button size="sm" variant="secondary" label={t('web.hostedSecurity.change')} onClick={() => setTask(tasks.password)} /> : undefined} />
        <SettingsRow icon="mail" label={t('web.hostedSecurity.email')} help={state.privateEmail ? t('web.hostedSecurity.privateEmail') : t('web.hostedSecurity.emailHelp')} meta={state.contactAddress || t('web.hostedSecurity.notSet')} control={state.hasPassword ? <Button size="sm" variant="ghost" label={t('web.hostedSecurity.change')} onClick={() => setTask(tasks.email)} /> : undefined} />
        {state.providers.map(p => <SettingsRow key={p.id} icon="link" label={t('web.hostedSecurity.signInWith', {provider: providerName(p.provider)})} help={onlyWayIn ? t('web.hostedSecurity.onlyWayIn') : undefined} meta={t('web.hostedSecurity.connected')} control={onlyWayIn ? undefined : <Button size="sm" variant="ghost" label={t('web.hostedSecurity.disconnect')} disabled={!!providerBusy} onClick={() => setDisconnecting({id: p.id, provider: p.provider})} />} />)}
        {connectable.map(p => <SettingsRow key={p} icon="link" label={t('web.hostedSecurity.signInWith', {provider: providerName(p)})} help={t('web.hostedSecurity.connectHelp', {provider: providerName(p)})} control={<Button size="sm" variant="secondary" label={t('web.hostedSecurity.connect')} loading={providerBusy === p} disabled={!!providerBusy && providerBusy !== p} onClick={() => void connect(p)} />} />)}
        {providerFailure ? <div style={{padding: 12}}><Notice tone="error" compact>{providerFailure}</Notice></div> : null}
      </SettingsGroup>
      <ConfirmDialog open={!!disconnecting} onOpenChange={o => { if (!o) setDisconnecting(undefined); }} title={t('web.hostedSecurity.disconnectTitle', {provider: disconnecting ? providerName(disconnecting.provider) : ''})} body={t('web.hostedSecurity.disconnectBody', {provider: disconnecting ? providerName(disconnecting.provider) : ''})} confirmLabel={t('web.hostedSecurity.disconnect')} cancelLabel={t('action.cancel')} busy={!!providerBusy} onConfirm={disconnect} />
      <SettingsGroup title={t('web.twoStep.title')} description={t('web.hostedSecurity.twoStepHelp')}>
        <SettingsRow icon="shield" label={state.mfaEnabled ? t('web.hostedSecurity.on') : t('web.hostedSecurity.off')} help={state.mfaEnabled ? t('web.hostedSecurity.codesLeft', {count: state.recoveryCodesRemaining}) : t('web.hostedSecurity.recommended')} control={state.hasPassword ? <Button size="sm" variant={state.mfaEnabled ? 'ghost' : 'secondary'} label={state.mfaEnabled ? t('web.hostedSecurity.turnOff') : t('web.twoStep.turnOn')} onClick={() => setTask(state.mfaEnabled ? tasks.disable : tasks.enrol)} /> : undefined} />
        {state.mfaEnabled ? <SettingsRow icon="refresh" label={t('web.hostedSecurity.recoveryCodes')} help={t('web.hostedSecurity.recoveryCodesHelp')} control={<Button size="sm" variant="ghost" label={t('web.hostedSecurity.makeCodes')} onClick={() => setTask(tasks.codes)} />} /> : null}
      </SettingsGroup>
      {!state.hasPassword ? <Notice tone="info">{t('web.hostedSecurity.providerOnly')}</Notice> : null}
      <SettingsGroup title={t('web.deleteAccount.title')} description={t('web.deleteAccount.help')}>
        <SettingsRow icon="trash" label={t('web.deleteAccount.row')} control={<Button size="sm" variant="danger" label={t('web.deleteAccount.action')} onClick={() => setDeleting(true)} />} />
      </SettingsGroup>
      {deleting ? <DeleteAccount state={state} scope={scope} onClose={() => setDeleting(false)} /> : null}
      {task ? <SecurityTask task={task} state={state} scope={scope} onClose={(message, signedOut) => { setTask(undefined); if (message) setNotice(message); if (!signedOut) void load(); }} /> : null}
    </SettingsPage>
  );
}

function SecurityTask({task, state, scope, onClose}: {task: Task; state: SecurityState; scope: AccountScope; onClose: (message?: string, signedOut?: boolean) => void}) {
  const client = useMemo(() => accountSecurityClient(), []);
  const api = useMemo(() => accountApi(scope), [scope.accountId, scope.familyId]);
  const operationId = useMemo(() => crypto.randomUUID(), []);
  const [password, setPassword] = useState('');
  const [factor, setFactor] = useState('');
  const [value, setValue] = useState('');
  const [code, setCode] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [result, setResult] = useState<SecurityResult>();
  const done = (purpose: SecurityPurpose) => purpose === 'password_set_or_change' ? t('auth.passwordChanged') : purpose === 'email_change' ? t('web.hostedSecurity.emailChanged') : purpose === 'mfa_enroll' ? t('web.hostedSecurity.twoStepOn') : purpose === 'mfa_disable' ? t('web.hostedSecurity.twoStepOff') : purpose === 'recovery_codes' ? t('web.hostedSecurity.codesMade') : '';
  const start = async () => {
    setBusy(true); setError('');
    try {
      const token = await api.token();
      const intent: SecurityIntent = {purpose: task.purpose, target: task.target === 'email' ? value.trim() : '', operationId};
      const proof = await client.passwordProof(intent, password, factor.trim(), scope, token);
      const out = await client.action(intent, proof.proof, task.target === 'password' ? value : '', scope, token);
      if (out.challengeId || out.recoveryCodes) setResult(out); else onClose(done(task.purpose), out.signOut);
    } catch (e) { setError(say(e, state.mfaEnabled ? t('web.hostedSecurity.wrongPasswordOrCode') : t('account.wrongPassword'))); } finally { setBusy(false); }
  };
  const confirm = async () => {
    if (!result?.challengeId || !task.confirm) return;
    setBusy(true); setError('');
    try {
      const out = await client.confirm(task.confirm, result.challengeId, code.trim(), operationId, scope, await api.token());
      if (out.recoveryCodes) setResult({...out, challengeId: undefined}); else onClose(done(task.purpose), out.signOut);
    } catch (e) { setError(say(e, t('web.hostedSecurity.wrongCode'))); } finally { setBusy(false); }
  };
  const codes = result?.recoveryCodes && !result.challengeId ? result.recoveryCodes : undefined;
  if (codes) {
    return (
      <Dialog open onOpenChange={o => !o && onClose(done(task.purpose))} title={t('confirm.recoveryCodes.title')} width={460} actions={<><Button variant="ghost" icon="copy" label={t('web.twoStep.copy')} onClick={() => void navigator.clipboard?.writeText(codes.join('\n'))} /><Button variant="primary" label={t('confirm.recoveryCodes.action')} onClick={() => onClose(done(task.purpose))} /></>}>
        <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
          <Text variant="body" tone="secondary">{t('confirm.recoveryCodes.body')}</Text>
          <div style={{display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8}}>{codes.map(c => <Text key={c} variant="mono">{c}</Text>)}</div>
        </div>
      </Dialog>
    );
  }
  if (result?.challengeId) {
    return (
      <Dialog open onOpenChange={o => !o && onClose()} title={task.confirm === 'mfa' ? t('web.hostedSecurity.addToAuthenticator') : t('web.hostedSecurity.checkEmail')} width={460} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={() => onClose()} /><Button variant="primary" label={t('web.hostedSecurity.confirm')} loading={busy} disabled={code.trim().length < 6} onClick={() => void confirm()} /></>}>
        <form onSubmit={e => { e.preventDefault(); void confirm(); }} style={{display: 'flex', flexDirection: 'column', gap: 12}}>
          {error ? <Notice tone="error" compact>{error}</Notice> : null}
          {task.confirm === 'mfa' ? <><Text variant="body" tone="secondary">{t('web.hostedSecurity.enterKey')}</Text><Text variant="mono" style={{wordBreak: 'break-all'}}>{result.secret}</Text>{result.otpAuthUri ? <div><Button size="sm" variant="ghost" icon="external" label={t('web.hostedSecurity.openAuthenticator')} href={result.otpAuthUri} /></div> : null}</> : <Text variant="body" tone="secondary">{t('web.hostedSecurity.sentCode', {address: value.trim()})}</Text>}
          <Input label={t('web.twoStep.code')} value={code} inputMode="numeric" autoComplete="one-time-code" maxLength={12} autoFocus onChange={e => setCode(e.target.value)} />
        </form>
      </Dialog>
    );
  }
  const ready = password.length > 0 && (!state.mfaEnabled || factor.trim().length >= 6) && (!task.target || (task.target === 'password' ? passwordStrength(value).acceptable : /^\S+@\S+\.\S+$/.test(value.trim())));
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={task.title()} width={460} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={() => onClose()} /><Button variant="primary" label={t('action.continue')} loading={busy} disabled={!ready} onClick={() => void start()} /></>}>
      <form onSubmit={e => { e.preventDefault(); if (ready) void start(); }} style={{display: 'flex', flexDirection: 'column', gap: 12}}>
        {error ? <Notice tone="error" compact>{error}</Notice> : null}
        {task.purpose === 'mfa_disable' ? <Text variant="body" tone="secondary">{t('web.hostedSecurity.passwordAlone')}</Text> : null}
        {task.purpose === 'password_set_or_change' ? <Text variant="body" tone="secondary">{t('web.hostedSecurity.othersSignedOut')}</Text> : null}
        <Input label={task.target === 'password' ? t('account.currentPassword') : t('web.password')} type="password" autoComplete="current-password" value={password} autoFocus onChange={e => setPassword(e.target.value)} />
        {state.mfaEnabled ? <Input label={t('profile.pinRecoveryCode')} value={factor} autoComplete="one-time-code" maxLength={40} onChange={e => setFactor(e.target.value)} /> : null}
        {task.target === 'password' ? <><Input label={t('auth.newPassword')} type="password" autoComplete="new-password" aria-describedby="hosted-password-strength" value={value} onChange={e => setValue(e.target.value)} /><PasswordStrength id="hosted-password-strength" password={value} /></> : null}
        {task.target === 'email' ? <Input label={t('web.hostedSecurity.newEmail')} type="email" autoComplete="email" value={value} onChange={e => setValue(e.target.value)} /> : null}
      </form>
    </Dialog>
  );
}

/** What the person types to confirm: the account email, or the username when there is none (A80). */
export function deletionConfirmationTarget(contactAddress: string | undefined, username: string | undefined): string {
  return (contactAddress || username || '').trim();
}
/** What deleting removes, one sentence per kind of thing. */
function consequences(preview: DeletionPreview): string[] {
  const lines = [t('web.deleteAccount.profiles', {count: preview.profileCount})];
  const owned = preview.ownedServers.length, member = preview.affectedServers.length - owned;
  if (owned) lines.push(t('web.deleteAccount.owned', {count: owned, servers: preview.ownedServers.map(x => x.name).join(', ')}));
  if (member > 0) lines.push(t('web.deleteAccount.member', {count: member}));
  return lines;
}
export const confirmationMatches = (typed: string, target: string) => !!target && typed.trim().toLowerCase() === target.toLowerCase();

/**
 * A80: deleting the Portico Account needs the signed-in session only. The preview says what goes
 * (profiles, servers you own, servers you're a member of); the person types their email (or
 * username) to confirm. A mismatch is 400 `deletion_confirmation_mismatch`.
 */
function DeleteAccount({state, scope, onClose}: {state: SecurityState; scope: AccountScope; onClose: () => void}) {
  const client = useMemo(() => accountSecurityClient(), []);
  const api = useMemo(() => accountApi(scope), [scope.accountId, scope.familyId]);
  const session = useSession();
  const operationId = useMemo(() => crypto.randomUUID(), []);
  const [preview, setPreview] = useState<DeletionPreview>();
  const [typed, setTyped] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [done, setDone] = useState(false);
  const target = deletionConfirmationTarget(state.contactAddress, session.hosted?.account.username);
  useEffect(() => {
    let live = true;
    void (async () => {
      try { const p = await client.deletionPreview(operationId, await api.token()); if (live) setPreview(p); } catch (e) { if (live) setError(errorText(e, 'portico-account', 'load')); }
    })();
    return () => { live = false; };
  }, [client, api, operationId]);
  const remove = async () => {
    if (!preview || !confirmationMatches(typed, target)) return;
    setBusy(true); setError('');
    try {
      await client.acceptDeletion(preview, typed, await api.token());
      setDone(true);
    } catch (e) {
      setError((e as {code?: string})?.code === 'deletion_confirmation_mismatch' ? t('web.deleteAccount.mismatch', {value: target}) : errorText(e, 'portico-account', 'save'));
    } finally { setBusy(false); }
  };
  if (done) {
    return (
      <Dialog open onOpenChange={() => void session.signOutAccount()} title={t('web.deleteAccount.doneTitle')} width={460} actions={<Button variant="primary" label={t('action.close')} onClick={() => void session.signOutAccount()} />}>
        <Text variant="body">{t('web.deleteAccount.doneBody')}</Text>
      </Dialog>
    );
  }
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={t('web.deleteAccount.confirmTitle')} width={480} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={onClose} /><Button variant="danger" label={t('web.deleteAccount.confirmAction')} loading={busy} disabled={!preview || !preview.canDelete || !confirmationMatches(typed, target)} onClick={() => void remove()} /></>}>
      <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
        {!preview && !error ? <Loading label={t('web.deleteAccount.checking')} /> : null}
        {preview ? (
          <>
            {consequences(preview).map(line => <Text key={line} variant="body">{line}</Text>)}
            {!preview.canDelete ? <Notice tone="warning" compact>{t('web.deleteAccount.cannot')}</Notice> : null}
            <Input label={t('web.deleteAccount.typeLabel', {value: target})} value={typed} onChange={e => { setTyped(e.target.value); setError(''); }} autoComplete="off" autoCapitalize="off" spellCheck={false} />
          </>
        ) : null}
        {error ? <Notice tone="error" compact>{error}</Notice> : null}
      </div>
    </Dialog>
  );
}
