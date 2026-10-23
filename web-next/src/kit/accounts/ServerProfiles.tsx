import {useState, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import {profileArtColors} from '@design';
import type {ProfilePickerProfile} from '../artwork/ProfilePicker';
import {ProfileFace, ProfilePicker} from '../artwork/ProfilePicker';
import {Button} from '../controls/Button';
import {AuthCodeBoxes, AuthError, AuthStage} from './AuthStage';
import styles from './accounts.module.css';

const t = defaultI18n.t;
// Auth's approved blue face is brighter than the compact account avatar.
const profileTint = (profile: ProfilePickerProfile) => profile.art === 'blue' ? '#2f86bb' : undefined;
export function ServerProfiles({serverName, profiles, onSelect, onSwitchServer, error, autoFocusId}: {
  serverName: string; profiles: readonly ProfilePickerProfile[]; onSelect: (profile: ProfilePickerProfile) => void; onSwitchServer: () => void; error?: unknown; autoFocusId?: string;
}) {
  // A server-local picker never contains profiles owned elsewhere. Restrictions
  // remain visible and disabled; a legacy not-shared row cannot become a choice.
  const visible = profiles.filter(profile => profile.unavailableReason !== 'membership_required' && profile.unavailableReason !== 'membership_revoked');
  return <AuthStage shape="round" layout="center" legal={false} topRight={<Button icon="i-server" size="small" onClick={onSwitchServer}>{t('settings.switchServer')}</Button>}>
    <h1 className={styles.pickerTitle}>{t('accounts.profiles.onServer', {server: serverName})}</h1>
    <AuthError error={error} /><ProfilePicker profiles={visible} onSelect={onSelect} autoFocusId={autoFocusId} getProfileTint={profileTint} layout="auto" />
  </AuthStage>;
}
export function ServerProfilePIN({serverName, profile, onComplete, onOtherProfiles, onForgotPIN, onSwitchServer, remember, onRememberChange, error, errorText, failure, busy, initialPIN = '', autoFocus = true}: {
  serverName: string; profile: ProfilePickerProfile; onComplete: (pin: string) => void; onOtherProfiles: () => void; onForgotPIN: () => void; onSwitchServer: () => void;
  remember: boolean; onRememberChange: (value: boolean) => void; error?: boolean; errorText?: ReactNode; failure?: unknown; busy?: boolean; initialPIN?: string; autoFocus?: boolean;
}) {
  const [pin, setPIN] = useState(initialPIN);
  const context = t('accounts.profiles.serverContext', {server: '\uFFFC'}).split('\uFFFC');
  return <AuthStage shape="round" layout="center" legal={false} topRight={<>
    <span className={styles.serverContext}>{context[0]}<b>{serverName}</b>{context[1]}</span><Button icon="i-server" size="small" onClick={onSwitchServer}>{t('settings.switchServer')}</Button>
  </>}><div className={styles.pin}>
    <ProfileFace initial={Array.from(profile.name.trim())[0] ?? ''} art={profile.art && Object.hasOwn(profileArtColors, profile.art) ? profile.art as keyof typeof profileArtColors : 'blue'} tint={profileTint(profile)} locked size={148} />
    <h2>{profile.name}</h2><p>{t('accounts.profiles.pinForProfile', {profile: profile.name})}</p>
    <AuthError error={failure} />
    <AuthCodeBoxes length={4} value={pin} label={t('accounts.profiles.pinLabel')} onChange={setPIN} autoFocus={autoFocus} disabled={busy}
      error={errorText ?? (error && t('accounts.profiles.pinFailed'))} onComplete={value => {onComplete(value); setPIN('');}} />
    <label className={styles.remember}><input type="checkbox" checked={remember} onChange={event => onRememberChange(event.currentTarget.checked)} />{t('web.direct.rememberProfile')}</label>
    <div className={styles.pinActions}><Button size="small" icon="i-left" onClick={onOtherProfiles}>{t('web.direct.otherProfiles')}</Button><Button variant="ghost" size="small" onClick={onForgotPIN}>{t('pin.forgot')}</Button></div>
  </div></AuthStage>;
}
