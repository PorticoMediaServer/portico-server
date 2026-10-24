import {useEffect, useId, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import type {ResetFileLocation} from '@core/account-reset.ts';
import {passwordStrength} from '@core/password-strength.ts';
import {Button, Switch, TextField} from '../controls';
import {Icon} from '../Icon';
import {AuthError, AuthPasswordField, AuthStage} from './AuthStage';
import {ServicesStrength} from './ServicesAuth';
import styles from './accounts.module.css';

const t = defaultI18n.t;
/** Only the actual returned system/path chooses the file shown here. */
export function resetFilePath(location: ResetFileLocation): string {
  const separator = location.systemKind === 'windows' ? '\\' : '/';
  return location.fileDirectory + (/[\\/]$/.test(location.fileDirectory) ? '' : separator) + location.fileName;
}

/** Auth20b: the secret is read on the server's computer, never returned here. */
export function OwnerResetFile({serverName, location, writtenAt, busy, error, onVerify, onRewrite, onBack}: {
  serverName: string; location: ResetFileLocation; writtenAt: number; busy?: boolean; error?: unknown;
  onVerify: (code: string) => void; onRewrite: () => void; onBack: () => void;
}) {
  const [code, setCode] = useState(''), [copyError, setCopyError] = useState<unknown>();
  const [remaining, setRemaining] = useState(() => Math.max(0, Math.ceil((writtenAt + location.retryAfterSeconds * 1000 - Date.now()) / 1000)));
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => {input.current?.focus();}, []);
  useEffect(() => {
    const tick = () => setRemaining(Math.max(0, Math.ceil((writtenAt + location.retryAfterSeconds * 1000 - Date.now()) / 1000)));
    tick();if (writtenAt + location.retryAfterSeconds * 1000 <= Date.now()) return;
    const timer = setInterval(() => {tick();if (writtenAt + location.retryAfterSeconds * 1000 <= Date.now()) clearInterval(timer);}, 1000);return () => clearInterval(timer);
  }, [writtenAt, location.retryAfterSeconds]);
  const path = resetFilePath(location);
  return <AuthStage legal={false} phoneShape="top" className={styles.ownerResetStage} contentClassName={styles.resetFileContent}>
    <button className={styles.localRecoveryPhoneBack} type="button" onClick={onBack}><Icon name="i-left" size={22} />{t('accounts.signIn.back')}</button>
    <p className={styles.eyebrow}>{serverName}</p><h1>{t('accounts.recovery.fileTitle')}</h1><p className={styles.lede}>{t('accounts.recovery.fileBody')}</p>
    <AuthError error={error ?? copyError} />
    <div className={styles.resetFilePath}><div><small>{t('accounts.recovery.fileSystem', {system: location.systemKind})}</small><code>{path}</code></div>
      <button type="button" aria-label={t('web.twoStep.copy')} onClick={() => {void navigator.clipboard.writeText(path).catch(setCopyError);}}><Icon name="i-copy" size={20} /></button></div>
    <details className={styles.resetFilePlatforms}><summary><Icon name="i-down" size={18} />{t('accounts.recovery.fileOtherSystems')}</summary>
      <dl><div><dt>{t('accounts.recovery.windows')}</dt><dd><code>{t('accounts.recovery.windowsPath')}</code></dd></div><div><dt>{t('accounts.recovery.macos')}</dt><dd><code>{t('accounts.recovery.macosPath')}</code></dd></div><div><dt>{t('accounts.recovery.docker')}</dt><dd><code>{t('accounts.recovery.dockerPath')}</code> {t('accounts.recovery.inContainer')}</dd></div></dl>
    </details>
    <form className={styles.form} onSubmit={event => {event.preventDefault(); if (code.trim() && !busy) onVerify(code);}}>
      <TextField ref={input} variant="auth" label={t('accounts.recovery.code')} autoComplete="one-time-code" placeholder={t('accounts.recovery.codePlaceholder')} value={code} onChange={event => setCode(event.currentTarget.value)} disabled={busy} required />
      <div className={styles.resetFileActions}><Button variant="primary" type="submit" loading={busy} disabled={!code.trim()}>{t('action.continue')}</Button>
        <Button variant="ghost" disabled={busy || remaining > 0} onClick={onRewrite}>{remaining > 0 ? t('accounts.recovery.writeNewIn', {time: `${Math.floor(remaining / 60)}:${String(remaining % 60).padStart(2, '0')}`}) : t('accounts.recovery.writeNew')}</Button></div>
    </form>
    <p className={styles.ownerResetQuiet}>{t('accounts.recovery.fileHelp')}</p><button type="button" className={styles.localRecoveryBack} onClick={onBack}><Icon name="i-left" size={18} />{t('accounts.recovery.back')}</button>
  </AuthStage>;
}

/** Auth20c/phone33. The switch is offered only by the proved account's real
 * two-step state; native browser inputs own selection, autofill and carets. */
export function OwnerResetPassword({serverName, username, twoStepEnabled, busy, error, onSubmit}: {
  serverName: string; username: string; twoStepEnabled: boolean; busy?: boolean; error?: unknown; onSubmit: (password: string, turnOffTwoStep: boolean) => void;
}) {
  const [password, setPassword] = useState(''), [confirm, setConfirm] = useState(''), [turnOff, setTurnOff] = useState(false);
  const [phone, setPhone] = useState(() => typeof matchMedia === 'function' && matchMedia('(max-width: 700px)').matches);
  const input = useRef<HTMLInputElement>(null), strength = useId();
  useEffect(() => {const media = matchMedia('(max-width: 700px)'), change = () => setPhone(media.matches);change();media.addEventListener('change', change);return () => media.removeEventListener('change', change);}, []);
  useEffect(() => {input.current?.focus();}, []);
  const ready = passwordStrength(password).acceptable && (phone || password === confirm);
  return <AuthStage legal={false} phoneShape="top" className={styles.ownerResetStage} contentClassName={styles.ownerResetContent}>
    <p className={styles.eyebrow}>{serverName}</p><h1>{t('web.reset.title')}</h1><p className={styles.lede}>{t('accounts.recovery.ownerPasswordBody', {username})}</p><AuthError error={error} />
    <form className={styles.form} onSubmit={event => {event.preventDefault(); if (ready && !busy) {onSubmit(password, twoStepEnabled && turnOff);setPassword('');setConfirm('');setTurnOff(false);}}}>
      <div className={styles.ownerResetPasswordField}><AuthPasswordField ref={input} revealLabel={false} label={t('accounts.recovery.newPassword')} name="new-password" autoComplete="new-password" value={password} onChange={event => setPassword(event.currentTarget.value)} aria-describedby={strength} disabled={busy} required /><ServicesStrength password={password} id={strength} /></div>
      {!phone && <TextField variant="auth" type="password" label={t('web.reset.confirm')} name="confirm-password" autoComplete="new-password" value={confirm} onChange={event => setConfirm(event.currentTarget.value)} disabled={busy} required />}
      {twoStepEnabled && <label className={styles.ownerResetFactor}><span>{t('web.twoStep.turnOffTitle')}<small className={styles.desktopRecoveryCopy}>{t('accounts.recovery.ownerFactorHelp')}</small><small className={styles.phoneRecoveryCopy}>{t('accounts.recovery.phoneOwnerFactorHelp')}</small></span><Switch size="compact" checked={turnOff} onCheckedChange={setTurnOff} disabled={busy} /></label>}
      <Button className={styles.ownerResetSubmit} variant="primary" type="submit" loading={busy} disabled={!ready}>{t('accounts.recovery.setPasswordSignIn')}</Button>
    </form><p className={styles.ownerResetQuiet}>{t('accounts.recovery.ownerOtherDevices')}</p>
  </AuthStage>;
}
