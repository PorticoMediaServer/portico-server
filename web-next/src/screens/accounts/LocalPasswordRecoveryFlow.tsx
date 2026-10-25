import {useCallback, useEffect, useId, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {passwordStrength} from '@core/password-strength.ts';
import type {ResetFileLocation} from '@core/account-reset.ts';
import type {BrowserAccountEntry} from '../../platform/account-entry';
import type {AccountSignInForm} from './sign-in-form';
import {AuthError, AuthLinks, AuthPasswordField, AuthStage, LocalPasswordRecovery} from '../../kit/accounts';
import {ServicesStrength} from '../../kit/accounts/ServicesAuth';
import {Button, Switch, TextField} from '../../kit/controls';
import authStyles from '../../kit/accounts/accounts.module.css';
import styles from './local-password-recovery.module.css';

const t = defaultI18n.t;
export type LocalPasswordRecoveryFlowProps = Readonly<{
  entry: BrowserAccountEntry; form: AccountSignInForm; onBack: () => void;
}>;

/** Local proof stays in the inspected entry. This view never handles a grant,
 * saves a code, constructs an API, or admits an authenticated session itself. */
export function LocalPasswordRecoveryFlow({entry, form, onBack}: LocalPasswordRecoveryFlowProps) {
  const state = useSyncExternalStore(entry.subscribe, entry.getSnapshot);
  const input = useSyncExternalStore(form.subscribe, form.getSnapshot);
  const captured = useRef({entry, origin: state.origin, serverId: state.server?.id, pin: state.pin}).current;
  const [file, setFile] = useState<ResetFileLocation>(), [owner, setOwner] = useState(input.username), [error, setError] = useState<unknown>();
  const alive = useRef(true), pending = useRef(false), ownerPending = useRef(false);
  const sameEntry = useCallback(() => {
    const current = entry.getSnapshot();
    return captured.entry === entry && captured.origin === current.origin && captured.serverId === current.server?.id
      && Boolean(captured.pin) && captured.pin === current.pin && current.server?.signIn === 'local' && !current.invitation;
  }, [captured, entry]);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      if (sameEntry() && (ownerPending.current || entry.getSnapshot().phase === 'owner-reset-password')) entry.cancelLocalOwnerReset();
    };
  }, [entry, sameEntry]);
  const available = () => alive.current && sameEntry() && !pending.current && !entry.getSnapshot().busy && !form.getSnapshot().busy;
  const back = () => {
    if (!alive.current || !sameEntry()) return;
    entry.cancelLocalOwnerReset();
    form.clear(); onBack();
  };
  const requestFile = async (username: string) => {
    if (!available() || entry.getSnapshot().phase !== 'password' || !username.trim()) return;
    pending.current = true; ownerPending.current = true; setError(undefined);
    try {
      const location = await entry.requestLocalResetFile(username);
      if (alive.current && sameEntry() && location) {setOwner(username); setFile(location);}
    } catch (failure) {if (alive.current && sameEntry()) setError(failure);}
    finally {pending.current = false; ownerPending.current = false;}
  };
  const verifyFile = async (code: string) => {
    if (!available() || entry.getSnapshot().phase !== 'password') return;
    pending.current = true; ownerPending.current = true; setError(undefined);
    try {await entry.verifyLocalOwnerReset(code);}
    catch (failure) {if (alive.current && sameEntry()) setError(failure);}
    finally {pending.current = false; ownerPending.current = false;}
  };
  const resetMember = async (username: string, code: string, password: string) => {
    if (!available() || entry.getSnapshot().phase !== 'password') return;
    pending.current = true; setError(undefined);
    try {await form.resetMemberPassword(username, code, password);}
    catch (failure) {if (alive.current && sameEntry()) setError(failure);}
    finally {pending.current = false;}
  };
  const finishOwner = async (password: string, turnOffTwoStep: boolean) => {
    if (!available() || entry.getSnapshot().phase !== 'owner-reset-password' || !entry.getSnapshot().ownerReset) return;
    const details = entry.getSnapshot().ownerReset!;
    pending.current = true; ownerPending.current = true; setError(undefined);
    try {
      const finished = await entry.finishLocalOwnerResetPassword(password, details.twoStepEnabled && turnOffTwoStep);
      if (finished && alive.current && sameEntry()) {
        // Fresh ordinary sign-in owns MFA, required setup and durable admission.
        await form.signIn(details.username, password);
      }
    } catch (failure) {if (alive.current && sameEntry()) setError(failure);}
    finally {pending.current = false; ownerPending.current = false;}
  };
  if (!sameEntry() || state.phase !== 'password' && state.phase !== 'owner-reset-password') return null;
  const busy = state.busy || input.busy, failure = state.error ?? error;
  if (state.phase === 'owner-reset-password' && state.ownerReset) return <OwnerPassword serverName={state.server!.name}
    username={state.ownerReset.username} twoStepEnabled={state.ownerReset.twoStepEnabled} busy={busy} error={failure} onSubmit={finishOwner} onBack={back}/>;
  if (file) return <OwnerFile key={`${file.fileDirectory}:${file.retryAfterSeconds}`} serverName={state.server!.name} location={file}
    busy={busy} error={failure} onVerify={verifyFile} onRewrite={() => {void requestFile(owner);}} onBack={back}/>;
  return <LocalPasswordRecovery serverName={state.server!.name} initialUsername={input.username} busy={busy} error={failure}
    onWriteFile={username => {void requestFile(username);}} onMemberReset={(username, code, password) => {void resetMember(username, code, password);}} onBack={back}/>;
}

export function OwnerFile({serverName, location, busy, error, onVerify, onRewrite, onBack}: {
  serverName: string; location: ResetFileLocation; busy: boolean; error: unknown;
  onVerify: (code: string) => Promise<void>; onRewrite: () => void; onBack: () => void;
}) {
  const [code, setCode] = useState(''), [remaining, setRemaining] = useState(location.retryAfterSeconds);
  useEffect(() => {
    setRemaining(location.retryAfterSeconds);
    const timer = setTimeout(() => setRemaining(0), location.retryAfterSeconds * 1000);
    return () => clearTimeout(timer);
  }, [location]);
  const separator = location.systemKind === 'windows' ? '\\' : '/';
  const path = `${location.fileDirectory}${/[\\/]$/.test(location.fileDirectory) ? '' : separator}${location.fileName}`;
  return <AuthStage shape="round" legal={false}>
    <p className={authStyles.eyebrow}>{serverName}</p><h1>{t('accounts.recovery.fileTitle')}</h1>
    <p className={authStyles.lede}>{t('accounts.recovery.fileBody')}</p><AuthError error={error}/>
    <div className={styles.path}><small>{t('accounts.recovery.fileLocation')}</small><code>{path}</code></div>
    <form className={authStyles.form} onSubmit={event => {event.preventDefault(); if (!busy && code.trim()) {const proof = code; setCode(''); void onVerify(proof);}}}>
      <TextField variant="auth" label={t('accounts.recovery.code')} autoComplete="one-time-code" value={code} onChange={event => setCode(event.currentTarget.value)} disabled={busy} autoFocus/>
      <div className={styles.actions}><Button variant="primary" type="submit" disabled={!code.trim()} loading={busy}>{t('action.continue')}</Button>
        <Button variant="ghost" disabled={busy || remaining > 0} onClick={() => {setCode(''); onRewrite();}}>{t('accounts.recovery.rewrite')}</Button></div>
    </form>
    <p className={styles.quiet}>{t('accounts.recovery.fileHelp')}</p><AuthLinks><button type="button" onClick={onBack}>{t('accounts.recovery.back')}</button></AuthLinks>
  </AuthStage>;
}

function OwnerPassword({serverName, username, twoStepEnabled, busy, error, onSubmit, onBack}: {
  serverName: string; username: string; twoStepEnabled: boolean; busy: boolean; error: unknown;
  onSubmit: (password: string, turnOffTwoStep: boolean) => Promise<void>; onBack: () => void;
}) {
  const strengthId = useId();
  const [password, setPassword] = useState(''), [confirm, setConfirm] = useState(''), [turnOff, setTurnOff] = useState(false);
  const ready = passwordStrength(password).acceptable && password === confirm;
  return <AuthStage shape="round" legal={false}>
    <p className={authStyles.eyebrow}>{serverName}</p><h1>{t('web.direct.newPasswordTitle')}</h1>
    <p className={authStyles.lede}>{t('accounts.recovery.ownerPasswordBody', {username})}</p><AuthError error={error}/>
    <form className={authStyles.form} onSubmit={event => {event.preventDefault(); if (!busy && ready) {const next = password; setPassword(''); setConfirm(''); void onSubmit(next, turnOff);}}}>
      <AuthPasswordField label={t('accounts.recovery.newPassword')} autoComplete="new-password" value={password} onChange={event => setPassword(event.currentTarget.value)} disabled={busy} autoFocus/>
      <ServicesStrength password={password} id={strengthId}/>
      <AuthPasswordField label={t('web.reset.confirm')} autoComplete="new-password" value={confirm} onChange={event => setConfirm(event.currentTarget.value)} disabled={busy}/>
      {twoStepEnabled && <label className={styles.choice}><span>{t('security.twoStep.turnOff')}<small>{t('accounts.recovery.turnOffHelp')}</small></span><Switch checked={turnOff} onCheckedChange={setTurnOff} disabled={busy}/></label>}
      <Button variant="primary" type="submit" disabled={!ready} loading={busy}>{t('accounts.recovery.ownerSubmit')}</Button>
    </form><p className={styles.quiet}>{t('accounts.recovery.ownerSignedOut')}</p><AuthLinks><button type="button" onClick={onBack}>{t('accounts.recovery.back')}</button></AuthLinks>
  </AuthStage>;
}
