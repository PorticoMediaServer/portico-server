import {useState} from 'react';
import {defaultI18n} from '@i18n';
import {passwordStrength} from '@core/password-strength.ts';
import {Button} from '../controls/Button';
import {TextField} from '../controls/Fields';
import {AuthError, AuthLinks, AuthStage} from './AuthStage';
import styles from './accounts.module.css';

const t = defaultI18n.t;
export type LocalInvitationFormProps = Readonly<{
  serverName: string; inviter?: string; libraries: readonly string[]; busy?: boolean; error?: unknown;
  initialValues?: Readonly<{name?: string; username?: string; password?: string; confirmation?: string}>;
  onJoin: (username: string, password: string, name: string) => void; onBack: () => void;
}>;
/** Auth 12. The plain password fields are deliberate: this frame has no reveal action. */
export function LocalInvitationForm({serverName, inviter, libraries, busy, error, initialValues, onJoin, onBack}: LocalInvitationFormProps) {
  const [name, setName] = useState(initialValues?.name ?? ''), [username, setUsername] = useState(initialValues?.username ?? '');
  const [password, setPassword] = useState(initialValues?.password ?? ''), [confirmation, setConfirmation] = useState(initialValues?.confirmation ?? '');
  const [confirmed, setConfirmed] = useState(false);
  const [changedUsername, setChangedUsername] = useState(false);
  const usernameTaken = Boolean(error && typeof error === 'object' && 'code' in error && error.code === 'username_taken');
  const strength = passwordStrength(password), trimmed = username.trim();
  const valid = trimmed.length >= 3 && trimmed.length <= 64 && !/[\s@]/.test(trimmed) && strength.acceptable && password === confirmation;
  const mismatch = confirmed && confirmation && password !== confirmation ? t('web.directJoin.mismatch') : undefined;
  // These three labels are drawn. The remaining computed levels await exact copy;
  // never relabel a weak password as Good or Strong, and never gate on advice.
  const strengthLabel = strength.label === 'Good' ? t('accounts.password.good') : strength.label === 'Strong' ? t('services.password.strong') : strength.label === 'Too short' ? t('services.password.tooShort') : undefined;
  return <AuthStage contentClassName={styles.invitationContent}>
    <p className={styles.eyebrow}>{t('accounts.invitation.to')}</p><h1>{serverName}</h1>
    <p className={styles.lede}>{t('accounts.invitation.localBody')}</p>
    {inviter && <div className={styles.invitedBy}><span className={styles.inviterAvatar} aria-hidden="true">{Array.from(inviter)[0]}</span>
      <span><b>{inviter}</b>{' '}{t('accounts.invitation.invitedYou')}{libraries.length > 0 && <><i className={styles.invitationGap} aria-hidden="true" />{defaultI18n.list(libraries)}</>}</span>
    </div>}
    <AuthError error={usernameTaken ? undefined : error} />
    <form className={[styles.form, styles.invitationForm].join(' ')} onSubmit={event => {event.preventDefault(); setConfirmed(true); setChangedUsername(false); if (valid && !busy) onJoin(trimmed, password, name.trim());}}>
      <TextField variant="auth" label={<>{t('web.directJoin.name')}{' '}<small>{t('field.optional')}</small></>} name="name" autoComplete="name" description={t('web.directJoin.nameHelp')} value={name} onChange={event => setName(event.currentTarget.value)} disabled={busy} />
      <TextField variant="auth" label={t('web.directJoin.username')} name="username" autoComplete="username" autoCapitalize="none" spellCheck={false} description={usernameTaken && !changedUsername ? undefined : t('web.directJoin.usernameHelp')} error={usernameTaken && !changedUsername ? t('web.directJoin.error.taken') : undefined} value={username} onChange={event => {setUsername(event.currentTarget.value); setChangedUsername(true);}} disabled={busy} required minLength={3} maxLength={64} pattern={'[^\\s@]{3,64}'} />
      <div className={styles.invitationPair}>
        <div className={styles.invitationPassword}>
          <TextField variant="auth" label={t('web.directJoin.password')} name="password" type="password" autoComplete="new-password" value={password} onChange={event => setPassword(event.currentTarget.value)} disabled={busy} required />
          {password && <div className={styles.passwordStrength} data-level={strength.level}>{Array.from({length: 4}, (_, index) => <i key={index} data-on={index < strength.level || undefined} aria-hidden="true" />)}{strengthLabel && <span>{strengthLabel}</span>}</div>}
        </div>
        <TextField variant="auth" label={t('web.directJoin.confirm')} name="confirmation" type="password" autoComplete="new-password" value={confirmation} onBlur={() => setConfirmed(true)} onChange={event => {setConfirmation(event.currentTarget.value); if (confirmed) setConfirmed(false);}} error={mismatch} disabled={busy} required />
      </div>
      <Button className={styles.invitationSubmit} variant="primary" type="submit" loading={busy} disabled={!valid}>{t('web.directJoin.submit')}</Button>
    </form>
    <AuthLinks><button type="button" data-quiet onClick={onBack}>{t('web.hostedAccount.back')}</button></AuthLinks>
  </AuthStage>;
}
