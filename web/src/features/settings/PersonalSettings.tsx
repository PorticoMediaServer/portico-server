import { productMessage, viewerPreferenceLimitsV1 } from '@portico/client-core';
import {
  AlertTriangle,
  Camera,
  CheckCircle2,
  ChevronRight,
  ClipboardCopy,
  ExternalLink,
  Film,
  KeyRound,
  Laptop,
  ListVideo,
  LogOut,
  MonitorSmartphone,
  MonitorCog,
  RefreshCw,
  Search,
  ShieldCheck,
  ShieldOff,
  SlidersHorizontal,
  Trash2,
	UserPlus,
} from '#portico-icons';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { PasswordInput, PasswordRequirements, validPorticoPassword } from '../../components/controls/PasswordInput';
import { useAuthSession, useHome, useLibraries, useOptionalAuthSession } from '../../data/DataProvider';
import { ProductMessageIcon, SemanticProductIcon, productProblem, productText, reviewedProductErrorText } from '../../components/ProductLanguage';
import { useWebDisplayPreferences } from '../../preferences/WebDisplayPreferencesProvider';
import type { WebDisplayPreferences } from '../../preferences/webDisplayPreferences';
import { useOptionalRuntime } from '../../runtime/RuntimeContext';
import { HomeCustomizationDialog } from '../home/HomeCustomizationDialog';
import {
  ChoiceControl,
  InlineNotice,
  NumberControl,
  SaveBar,
  SettingRow,
  SettingsError,
  SettingsGroup,
  SettingsLoading,
  TextControl,
  ToggleControl,
} from './SettingsControls';
import { useAbortableMutation, useSettingsQuery } from './settingsHooks';
import type {
  AccountIdentitySnapshot,
  AccountMFAStatus,
  AccountOrigin,
  AccountSignedInDevice,
  SettingsDataSource,
  SettingsOperationalSnapshot,
  SettingsViewer,
} from './settingsTypes';

function relativeTime(value: string): string {
  const elapsed = Date.now() - Date.parse(value);
  if (!Number.isFinite(elapsed) || elapsed < 60_000) return 'just now';
  const minutes = Math.floor(elapsed / 60_000);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

function dateTime(value: string): string {
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf()) ? value : parsed.toLocaleString();
}

function viewerOrigin(viewer: SettingsViewer): AccountOrigin {
  return viewer.authOrigin === 'portico' ? 'portico' : 'local';
}

function SignedInDevices({ viewer, source }: { viewer: SettingsViewer; source: SettingsDataSource }) {
  const origin = viewerOrigin(viewer);
  const load = useCallback((next: SettingsDataSource, signal: AbortSignal) => next.signedInDevices(origin, signal), [origin]);
  const [revision, setRevision] = useState(0);
  const query = useSettingsQuery(load, source, revision);
  const mutation = useAbortableMutation();
  const [confirming, setConfirming] = useState('');
  const [feedback, setFeedback] = useState('');
  const [error, setError] = useState('');
  const revoke = async (device: AccountSignedInDevice) => {
    setError(''); setFeedback('');
    try {
      await mutation.run((signal) => source.revokeSignedInDevice(origin, device.id, signal));
      setConfirming('');
      setFeedback(`${device.name} signed out.`);
      setRevision((current) => current + 1);
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'sign out this session' }));
    }
  };
  const title = origin === 'portico' ? 'Signed-in devices' : 'Active sessions';
  const description = origin === 'portico' ? 'Devices signed into your Portico Account across all connected servers.' : `Applications currently signed in to ${viewer.serverName}.`;
  if (query.status === 'loading') return <SettingsGroup title={title} description={description}><SettingsLoading label={`Loading ${title.toLocaleLowerCase()}`} /></SettingsGroup>;
  if (query.status === 'error') return <SettingsGroup title={title} description={description}><SettingsError title={`${title} are unavailable`} message={reviewedProductErrorText(query.error, 'settings.load-failed', { sectionName: title })} onRetry={() => setRevision((current) => current + 1)} /></SettingsGroup>;
  const devices = query.data;
  return <SettingsGroup title={title} description={description} actions={<SecondaryButton onClick={() => setRevision((current) => current + 1)}><RefreshCw /> Refresh</SecondaryButton>}>
    {(feedback || error) && <InlineNotice tone={error ? 'error' : 'success'}>{error || feedback}</InlineNotice>}
    <div className="portico-account-session-list">{devices.length === 0 ? <div className="portico-settings-state"><Laptop /><strong>No signed-in devices were returned</strong><p>{origin === 'portico' ? 'Devices will appear after they sign into your Portico Account.' : `Signed-in applications for ${viewer.serverName} will appear here.`}</p></div> : devices.map((device) => <article key={device.id}>
      <span className={`portico-session-icon ${device.current ? 'current' : ''}`}><MonitorSmartphone /></span>
      <span><strong>{device.name}</strong><small>{device.app || 'Portico'} · {device.platform || 'Unknown platform'} · {origin === 'portico' ? 'Portico Account' : 'This Server'} · last used {relativeTime(device.lastSeenAt)}</small>{(device.createdAt || device.expiresAt || device.clientIp) && <em>{device.createdAt ? `Signed in ${dateTime(device.createdAt)}` : ''}{device.expiresAt ? `${device.createdAt ? ' · ' : ''}expires ${dateTime(device.expiresAt)}` : ''}{device.clientIp ? ` · ${device.clientIp}` : ''}</em>}</span>
      <div>{device.trusted && <span className="portico-settings-capability configured">Trusted</span>}{device.current ? <span className="portico-current-session">Current session</span> : device.canRevoke ? confirming === device.id ? <div className="portico-inline-confirm"><span>Sign out this device?</span><button type="button" onClick={() => setConfirming('')}>Cancel</button><button type="button" className="danger" disabled={mutation.busy} onClick={() => void revoke(device)}>Sign out</button></div> : <SecondaryButton disabled={mutation.busy} onClick={() => setConfirming(device.id)}><LogOut /> Sign out</SecondaryButton> : <span className="portico-current-session">Managed externally</span>}</div>
    </article>)}</div>
  </SettingsGroup>;
}

function ProfileIdentity({ viewer, source }: { viewer: SettingsViewer; source: SettingsDataSource }) {
  const origin = viewerOrigin(viewer);
  const readOnly = viewer.authProvider === 'api_key';
  const emailReadOnly = readOnly || origin === 'portico';
  const initialIdentity = (): AccountIdentitySnapshot => ({ displayName: viewer.displayName, email: viewer.email, profileImageUrl: viewer.profileImageUrl });
  const [saved, setSaved] = useState<AccountIdentitySnapshot>(initialIdentity);
  const [displayName, setDisplayName] = useState(saved.displayName);
  const [email, setEmail] = useState(saved.email);
  const [profileImageUrl, setProfileImageUrl] = useState(saved.profileImageUrl ?? '');
  const [removePhoto, setRemovePhoto] = useState(false);
  const [feedback, setFeedback] = useState('');
  const [warning, setWarning] = useState('');
  const [error, setError] = useState('');
  const mutation = useAbortableMutation();
  const fileInput = useRef<HTMLInputElement>(null);
  const dirty = displayName.trim() !== saved.displayName || (!emailReadOnly && email.trim() !== saved.email);
  useEffect(() => {
    const identity = initialIdentity();
    setSaved(identity);
    setDisplayName(identity.displayName);
    setEmail(identity.email);
    setProfileImageUrl(identity.profileImageUrl ?? '');
  }, [viewer.displayName, viewer.email, viewer.profileImageUrl]);
  const acceptIdentity = (identity: AccountIdentitySnapshot, message: string) => {
    setSaved(identity);
    setDisplayName(identity.displayName);
    setEmail(identity.email);
    setProfileImageUrl(identity.profileImageUrl ?? '');
    setFeedback(message);
    setWarning(identity.serverSyncWarning ?? '');
  };
  const save = async () => {
    const nextName = displayName.trim();
    const nextEmail = emailReadOnly ? saved.email : email.trim();
    setFeedback(''); setWarning(''); setError('');
    if (!nextName) {
      setError('Username is required.');
      return;
    }
    if (!nextEmail || !nextEmail.includes('@')) {
      setError('Enter a valid email address.');
      return;
    }
    try {
      const identity = await mutation.run((signal) => source.updateAccountIdentity(origin, { displayName: nextName, email: nextEmail }, signal));
      acceptIdentity(identity, origin === 'portico' ? 'Portico Account username saved.' : 'This Server username saved.');
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'save this profile' }));
    }
  };
  const upload = async (file?: File) => {
    if (!file) return;
    setFeedback(''); setWarning(''); setError('');
    if (file.size > 2 * 1024 * 1024) {
      setError('Profile images must be smaller than 2 MB.');
      return;
    }
    if (file.type && !['image/jpeg', 'image/png', 'image/gif'].includes(file.type)) {
      setError('Choose a JPEG, PNG, or GIF image.');
      return;
    }
    try {
      const identity = await mutation.run((signal) => source.uploadAccountImage(origin, file, signal));
      acceptIdentity(identity, 'Profile image updated.');
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'upload this profile image' }));
    } finally {
      if (fileInput.current) fileInput.current.value = '';
    }
  };
  const remove = async () => {
    setFeedback(''); setWarning(''); setError('');
    try {
      const identity = await mutation.run((signal) => source.deleteAccountImage(origin, signal));
      acceptIdentity(identity, 'Profile image removed.');
      setRemovePhoto(false);
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'remove this profile image' }));
    }
  };
  const avatar = profileImageUrl ? <img src={source.accountImageUrl(profileImageUrl)} alt="" /> : (displayName.trim().slice(0, 1).toLocaleUpperCase() || 'P');
  return <SettingsGroup title="Account identity" description={origin === 'portico' ? 'Your Portico Account username and account image. Profile names are managed separately.' : `Your sign-in identity stored only on ${viewer.serverName}. Profile names are managed separately.`}>
    {origin === 'portico' && <InlineNotice tone="info">Changes here apply to your Portico Account and are shared with connected servers.</InlineNotice>}
    {readOnly && <InlineNotice tone="warn">Profile changes require an interactive account sign-in.</InlineNotice>}
    {warning && <InlineNotice tone="warn">{warning}</InlineNotice>}
    <div className="portico-account-identity">
      <span className="portico-account-avatar">{avatar}</span>
      <span><strong>{saved.displayName}</strong><small>{saved.email} · {origin === 'portico' ? 'Portico Account' : 'This Server account'} · {viewer.role.slice(0, 1).toLocaleUpperCase()}{viewer.role.slice(1)}</small></span>
      <div><input ref={fileInput} className="portico-account-file-input" type="file" accept="image/jpeg,image/png,image/gif" aria-label="Upload profile image" disabled={readOnly || mutation.busy} onChange={(event) => void upload(event.currentTarget.files?.[0])} /><SecondaryButton disabled={readOnly || mutation.busy} onClick={() => fileInput.current?.click()}><Camera /> {profileImageUrl ? 'Replace image' : 'Add image'}</SecondaryButton>{profileImageUrl && (removePhoto ? <div className="portico-inline-confirm"><span>Remove image?</span><button type="button" onClick={() => setRemovePhoto(false)}>Cancel</button><button type="button" className="danger" disabled={mutation.busy} onClick={() => void remove()}>Remove</button></div> : <IconButton label="Remove profile image" disabled={readOnly || mutation.busy} onClick={() => setRemovePhoto(true)}><Trash2 /></IconButton>)}</div>
    </div>
    <SettingRow label="Username" description="Used to sign in and identify this account. Profile names do not affect sign-in."><TextControl label="Username" value={displayName} disabled={readOnly} onChange={setDisplayName} /></SettingRow>
    <SettingRow label="Email" description={origin === 'portico' ? 'Identifies your Portico Account and receives account recovery messages.' : 'Stored only on this server.'}><TextControl label="Email" type="email" value={email} disabled={emailReadOnly} onChange={setEmail} /></SettingRow>
    <SaveBar dirty={!readOnly && dirty} busy={mutation.busy} feedback={feedback} error={error} onSave={save} onReset={() => { setDisplayName(saved.displayName); setEmail(saved.email); setFeedback(''); setWarning(''); setError(''); }} />
  </SettingsGroup>;
}

function PasswordSettings({ viewer, source }: { viewer: SettingsViewer; source: SettingsDataSource }) {
  const origin = viewerOrigin(viewer);
  const readOnly = viewer.authProvider === 'api_key';
  const title = origin === 'portico' ? 'Portico Account password' : 'This Server password';
  const description = origin === 'portico'
    ? 'Change the password used to sign in to your Portico Account.'
    : `Change the password used to sign in to ${viewer.serverName}.`;
  const mutation = useAbortableMutation();
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [feedback, setFeedback] = useState('');
  const [error, setError] = useState('');
  if (origin === 'local' && !viewer.hasLocalPassword) return <SettingsGroup title={title} description="Password for this server profile.">
    <InlineNotice tone="info">This profile does not have a This Server password. The server owner can create one.</InlineNotice>
    <SettingRow label="Local credential" description="The server owner can reset this credential from People & Access."><span className="portico-setting-readonly"><KeyRound /> {viewer.hasLocalPassword ? 'Configured' : 'Not configured'}</span></SettingRow>
  </SettingsGroup>;
  if (readOnly) return <SettingsGroup title={title} description={description}><div className="portico-settings-readonly-note"><KeyRound />Password changes require an interactive account sign-in.</div></SettingsGroup>;
  const change = async () => {
    setFeedback(''); setError('');
    if (!currentPassword) {
      setError('Enter your current password.');
      return;
    }
    if (!validPorticoPassword(newPassword)) {
      setError('New passwords must be at least 8 characters and include uppercase, lowercase, and a number or special character.');
      return;
    }
    if (new TextEncoder().encode(newPassword).length > 72) {
      setError('New passwords must be no more than 72 UTF-8 bytes.');
      return;
    }
    if (newPassword !== confirmPassword) {
      setError('New passwords do not match.');
      return;
    }
    try {
      await mutation.run((signal) => origin === 'portico'
        ? source.changePorticoPassword({ currentPassword, newPassword }, signal)
        : source.changeLocalPassword({ currentPassword, newPassword }, signal));
      setCurrentPassword(''); setNewPassword(''); setConfirmPassword('');
      setFeedback(origin === 'portico' ? 'Portico Account password changed.' : 'This Server password changed.');
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'change this password' }));
    }
  };
  return <SettingsGroup title={title} description={description}>
    {origin === 'portico' && viewer.hasLocalPassword && <InlineNotice tone="info">Changing your Portico Account password does not change the separate recovery password stored by {viewer.serverName}.</InlineNotice>}
    {(feedback || error) && <InlineNotice tone={error ? 'error' : 'success'}>{error || feedback}</InlineNotice>}
    <div className="portico-account-security-form">
      <label><span>Current password</span><PasswordInput aria-label="Current password" autoComplete="current-password" value={currentPassword} disabled={mutation.busy} onChange={(event) => setCurrentPassword(event.target.value)} /></label>
      <label><span>New password</span><PasswordInput aria-label="New password" autoComplete="new-password" minLength={8} maxLength={72} value={newPassword} disabled={mutation.busy} onChange={(event) => setNewPassword(event.target.value)} aria-describedby="change-password-requirements" /><PasswordRequirements id="change-password-requirements" value={newPassword} /></label>
      <label><span>Confirm new password</span><PasswordInput aria-label="Confirm new password" autoComplete="new-password" value={confirmPassword} disabled={mutation.busy} onChange={(event) => setConfirmPassword(event.target.value)} /></label>
      <PrimaryButton disabled={mutation.busy || !currentPassword || !newPassword || !confirmPassword} onClick={() => void change()}>{mutation.busy ? 'Changing…' : 'Change password'}</PrimaryButton>
    </div>
  </SettingsGroup>;
}

function MFASettings({ viewer, source }: { viewer: SettingsViewer; source: SettingsDataSource }) {
  const origin = viewerOrigin(viewer);
  const available = origin === 'portico' && viewer.authProvider !== 'api_key';
  const [revision, setRevision] = useState(0);
  const load = useCallback((next: SettingsDataSource, signal: AbortSignal) => available ? next.porticoMFAStatus(signal) : Promise.resolve(null), [available]);
  const query = useSettingsQuery<AccountMFAStatus | null>(load, source, revision);
  const mutation = useAbortableMutation();
  const [setup, setSetup] = useState<{ enrollmentToken: string; secret: string; otpauthUrl: string }>();
  const [setupPassword, setSetupPassword] = useState('');
  const [verificationCode, setVerificationCode] = useState('');
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([]);
  const [disabling, setDisabling] = useState(false);
  const [disablePassword, setDisablePassword] = useState('');
  const [disableCode, setDisableCode] = useState('');
  const [statusOverride, setStatusOverride] = useState<AccountMFAStatus>();
  const [feedback, setFeedback] = useState('');
  const [error, setError] = useState('');
  const refresh = (preserveEnrollment = false) => {
    if (!preserveEnrollment) {
      setSetup(undefined);
      setVerificationCode('');
    }
    setStatusOverride(undefined);
    setRevision((current) => current + 1);
  };
  if (origin === 'local') return null;
  if (!available) return <SettingsGroup title="Two-factor authentication" description="Additional Portico Account sign-in protection."><div className="portico-settings-readonly-note"><ShieldOff />Two-factor authentication changes require an interactive Portico Account sign-in.</div></SettingsGroup>;
  const start = async () => {
    setFeedback(''); setError(''); setRecoveryCodes([]);
    try {
      const nextSetup = await mutation.run((signal) => source.startPorticoMFA(setupPassword, signal));
      setSetup(nextSetup);
      setSetupPassword('');
      setFeedback('Authenticator setup started. Verify a fresh code to finish enrollment.');
      refresh(true);
    } catch (reason) {
      setSetup(undefined);
      setSetupPassword('');
      setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'start authenticator setup' }));
    }
  };
  const enable = async () => {
    setFeedback(''); setError('');
    if (!verificationCode.trim() || !setup) return;
    try {
      const result = await mutation.run((signal) => source.enablePorticoMFA({ code: verificationCode.trim(), enrollmentToken: setup.enrollmentToken }, signal));
      setRecoveryCodes(result.recoveryCodes);
      setSetup(undefined);
      setVerificationCode('');
      setStatusOverride({
        enabled: result.enabled,
        setupStarted: false,
        recoveryCodesSupported: true,
        recoveryCodesRemaining: result.recoveryCodes.length,
      });
      setFeedback('Two-factor authentication enabled. Save the new recovery codes before leaving this page.');
    } catch (reason) {
      setSetup(undefined);
      setVerificationCode('');
      setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'verify this authenticator code' }));
    }
  };
  const disable = async () => {
    setFeedback(''); setError('');
    try {
      await mutation.run((signal) => source.disablePorticoMFA({ password: disablePassword, code: disableCode.trim() }, signal));
      setDisablePassword(''); setDisableCode(''); setDisabling(false); setRecoveryCodes([]); setSetup(undefined);
      setStatusOverride({ enabled: false, setupStarted: false, recoveryCodesSupported: true, recoveryCodesRemaining: 0 });
      setFeedback('Two-factor authentication disabled.');
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'disable two-factor authentication' }));
    }
  };
  const copyRecoveryCodes = async () => {
    if (!navigator.clipboard?.writeText) {
      setError('Clipboard access is unavailable. Select and copy the recovery codes manually.');
      return;
    }
    try {
      await navigator.clipboard.writeText(recoveryCodes.join('\n'));
      setFeedback('Recovery codes copied.');
    } catch {
      setError('The browser blocked clipboard access. Select and copy the recovery codes manually.');
    }
  };
  if (query.status === 'loading') return <SettingsGroup title="Two-factor authentication" description="Additional Portico Account sign-in protection."><SettingsLoading label="Checking two-factor authentication" /></SettingsGroup>;
  if (query.status === 'error') return <SettingsGroup title="Two-factor authentication" description="Additional Portico Account sign-in protection."><SettingsError title="Portico Account security is unavailable" message={reviewedProductErrorText(query.error, 'settings.load-failed', { sectionName: 'Portico Account security' })} onRetry={refresh} /></SettingsGroup>;
  const status = statusOverride ?? query.data;
  if (!status) return null;
  return <SettingsGroup title="Two-factor authentication" description="Protect your Portico Account with time-based codes from an authenticator app." actions={<SecondaryButton disabled={mutation.busy} onClick={() => refresh()}><RefreshCw /> Refresh</SecondaryButton>}>
    {(feedback || error) && <InlineNotice tone={error ? 'error' : recoveryCodes.length ? 'warn' : 'success'}>{error || feedback}</InlineNotice>}
    <div className="portico-account-mfa-summary"><span className={status.enabled ? 'enabled' : status.setupStarted ? 'pending' : ''}>{status.enabled ? <ShieldCheck /> : <ShieldOff />}</span><span><small>Status</small><strong>{status.enabled ? 'Enabled' : status.setupStarted ? 'Setup started' : 'Not enabled'}</strong><em>{status.enabled ? 'Authenticator code required at sign-in' : 'Portico Account password only'}</em></span>{status.enabled && typeof status.recoveryCodesRemaining === 'number' && <b className={status.recoveryCodesRemaining > 0 ? '' : 'warn'}>{status.recoveryCodesRemaining} recovery codes remaining</b>}</div>
    {!status.enabled && <div className="portico-account-mfa-enrollment">
      <div><span><strong>Authenticator app</strong><small>{status.setupStarted ? 'Resume setup with your password to display the current enrollment key.' : 'Confirm your password to create a private setup key.'}</small></span>{!setup && <><PasswordInput aria-label="Password to start two-factor setup" autoComplete="current-password" value={setupPassword} disabled={mutation.busy} onChange={(event) => setSetupPassword(event.target.value)} /><PrimaryButton disabled={mutation.busy || !setupPassword} onClick={() => void start()}>{mutation.busy ? 'Starting…' : status.setupStarted ? 'Resume setup' : 'Start setup'}</PrimaryButton></>}</div>
      {setup && <><label><span>Manual setup key</span><input aria-label="Manual authenticator setup key" readOnly value={setup.secret} /></label><label><span>Authenticator URI</span><input aria-label="Authenticator setup URI" readOnly value={setup.otpauthUrl} /></label><a className="portico-settings-inline-link" href={setup.otpauthUrl}><ExternalLink /> Open in authenticator app</a><div className="portico-account-mfa-verify"><label><span>Verification code</span><input aria-label="Two-factor verification code" autoComplete="one-time-code" inputMode="numeric" value={verificationCode} disabled={mutation.busy} onChange={(event) => setVerificationCode(event.target.value.replace(/\D/g, '').slice(0, 8))} /></label><SecondaryButton disabled={mutation.busy} onClick={() => { setSetup(undefined); setVerificationCode(''); setFeedback(''); }}>Cancel</SecondaryButton><PrimaryButton disabled={mutation.busy || !verificationCode.trim()} onClick={() => void enable()}>{mutation.busy ? 'Verifying…' : 'Enable two-factor'}</PrimaryButton></div></>}
    </div>}
    {status.enabled && <div className="portico-account-mfa-disable">
      {status.recoveryCodesRemaining === 0 && <InlineNotice tone="warn">No unused recovery codes remain. New recovery codes can only be created by re-enrolling two-factor authentication.</InlineNotice>}
      {!disabling ? <SettingRow label="Disable two-factor" description="Removing this protection requires your password and a fresh authenticator code."><SecondaryButton onClick={() => setDisabling(true)}><ShieldOff /> Disable two-factor</SecondaryButton></SettingRow> : <div className="portico-account-security-form danger"><label><span>Portico Account password</span><PasswordInput aria-label="Password to disable two-factor authentication" autoComplete="current-password" value={disablePassword} disabled={mutation.busy} onChange={(event) => setDisablePassword(event.target.value)} /></label><label><span>Authenticator code</span><input aria-label="Two-factor code to disable" autoComplete="one-time-code" inputMode="numeric" value={disableCode} disabled={mutation.busy} onChange={(event) => setDisableCode(event.target.value.replace(/\D/g, '').slice(0, 8))} /></label><div><SecondaryButton disabled={mutation.busy} onClick={() => { setDisabling(false); setDisablePassword(''); setDisableCode(''); }}>Cancel</SecondaryButton><button type="button" className="button secondary portico-destructive-button" disabled={mutation.busy || !disablePassword || !disableCode.trim()} onClick={() => void disable()}>{mutation.busy ? 'Disabling…' : 'Confirm disable'}</button></div></div>}
    </div>}
    {recoveryCodes.length > 0 && <div className="portico-account-recovery-codes"><header><span><strong>New recovery codes</strong><small>Each code works once. They cannot be shown again after this page is left.</small></span><SecondaryButton onClick={() => void copyRecoveryCodes()}><ClipboardCopy /> Copy codes</SecondaryButton></header><textarea aria-label="New two-factor recovery codes" readOnly value={recoveryCodes.join('\n')} rows={Math.min(10, recoveryCodes.length)} /></div>}
  </SettingsGroup>;
}

function DeletePorticoAccount({ viewer, source }: { viewer: SettingsViewer; source: SettingsDataSource }) {
  const auth = useOptionalAuthSession();
  const runtime = useOptionalRuntime();
  const description = productMessage('account.delete-description');
  const confirmation = productMessage('account.delete-ready');
  const deleted = productMessage('account.deleted');
  const [open, setOpen] = useState(false);
  const [password, setPassword] = useState('');
  const [secondFactor, setSecondFactor] = useState('');
  const [confirmationText, setConfirmationText] = useState('');
  const [complete, setComplete] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<ReturnType<typeof productProblem>>();

  if (viewerOrigin(viewer) !== 'portico' || viewer.authProvider === 'api_key') return null;

  const remove = async () => {
    if (!password || confirmationText !== 'DELETE') return;
    const controller = new AbortController();
    setBusy(true);
    setError(undefined);
    try {
      const verification = secondFactor.trim();
      await source.deletePorticoAccount({
        password,
        ...(verification ? /^\d{6}$/.test(verification) ? { mfaCode: verification } : { recoveryCode: verification } : {}),
      }, controller.signal);
      setPassword('');
      setSecondFactor('');
      setConfirmationText('');
      setComplete(true);
      try {
        if (runtime?.config.mode === 'hosted') await runtime.hostedLogout();
        else if (auth) await auth.signOutAllBrowserAccounts();
        else await source.signOut(controller.signal);
      } catch {
        // The account is already gone. Fail closed locally even if one cleanup
        // transport is unavailable, and never turn its raw detail into UI copy.
        if (runtime?.config.mode === 'hosted') await runtime.hostedLogout().catch(() => undefined);
        else if (auth) await auth.logout().catch(() => undefined);
        else await source.signOut(controller.signal).catch(() => undefined);
      }
    } catch (reason) {
      setError(productProblem(reason));
    } finally {
      setBusy(false);
    }
  };

  return <SettingsGroup title={description.title ?? productText('action.delete-account')} description={description.body ?? ''}>
    {complete ? <div className="portico-settings-compact-state" role="status"><ProductMessageIcon presentation={deleted} /><span><strong>{deleted.title}</strong><small>{deleted.body}</small></span></div> : !open ? <SettingRow label={productText('action.delete-account')} description={confirmation.body ?? ''}><button type="button" className="button secondary portico-destructive-button" onClick={() => setOpen(true)}><SemanticProductIcon id="action.delete" /> {productText('action.delete-account')}</button></SettingRow> : <div className="portico-account-security-form danger">
      <div className="portico-settings-compact-state"><ProductMessageIcon presentation={confirmation} /><span><strong>{confirmation.title}</strong><small>{confirmation.body}</small></span></div>
      {error && <InlineNotice tone={error.tone === 'warning' ? 'warn' : error.tone === 'neutral' ? 'info' : error.tone}><strong>{error.title}</strong>{error.body && <> {error.body}</>}</InlineNotice>}
      <label><span>{productText('account.delete-password-label')}</span><PasswordInput aria-label={productText('account.delete-password-label')} autoComplete="current-password" value={password} disabled={busy} onChange={(event) => setPassword(event.target.value)} /></label>
      <label><span>{productText('account.delete-mfa-label')}</span><input aria-label={productText('account.delete-mfa-label')} autoComplete="one-time-code" value={secondFactor} disabled={busy} onChange={(event) => setSecondFactor(event.target.value.slice(0, 32))} /><small>{productText('account.delete-mfa-description')}</small></label>
      <label><span>{productText('account.delete-confirmation')}</span><input aria-label={productText('account.delete-confirmation')} autoComplete="off" value={confirmationText} disabled={busy} onChange={(event) => setConfirmationText(event.target.value)} /></label>
      <div><SecondaryButton disabled={busy} onClick={() => { setOpen(false); setPassword(''); setSecondFactor(''); setConfirmationText(''); setError(undefined); }}>{productText('action.cancel')}</SecondaryButton><button type="button" className="button secondary portico-destructive-button" disabled={busy || !password || confirmationText !== 'DELETE'} onClick={() => void remove()}><SemanticProductIcon id="action.delete" /> {busy ? productText('action.deleting-account') : productText('action.delete-account')}</button></div>
    </div>}
  </SettingsGroup>;
}

export function AccountSettings({ viewer, source }: { viewer: SettingsViewer; source: SettingsDataSource }) {
  return <div className="portico-settings-form">
    <ProfileIdentity viewer={viewer} source={source} />
    <PasswordSettings viewer={viewer} source={source} />
    <MFASettings viewer={viewer} source={source} />
    <SignedInDevices viewer={viewer} source={source} />
    <DeletePorticoAccount viewer={viewer} source={source} />
  </div>;
}

export function BrowserAccountsSettings({ viewer }: { viewer: SettingsViewer; source: SettingsDataSource }) {
	const auth = useAuthSession();
	const navigate = useNavigate();
	const [confirming, setConfirming] = useState('');
	const [feedback, setFeedback] = useState('');
	const [error, setError] = useState('');
	const accounts = auth.browserAccounts.data.accounts;
	const setAutomaticSignIn = async (value: boolean) => {
		setFeedback(''); setError('');
		try {
			await auth.updateAutomaticSignIn(value);
			setFeedback(value ? 'Automatic sign in is on.' : 'The account chooser will open after signing out.');
		} catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'update automatic sign in' })); }
	};
	const remove = async (accountId: string) => {
		setFeedback(''); setError('');
		try {
			await auth.removeBrowserAccount(accountId);
			setConfirming('');
			setFeedback('Account removed from this browser.');
		} catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'remove this account from the browser' })); }
	};
	const switchAccount = async (accountId: string) => {
		setFeedback(''); setError('');
		try {
			await auth.switchBrowserAccount(accountId);
			navigate('/home', { replace: true });
		} catch (reason) { setError(reviewedProductErrorText(reason, 'auth.account-switch-failed')); }
	};
	const signOutAll = async () => {
		setFeedback(''); setError('');
		try { await auth.signOutAllBrowserAccounts(); }
		catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'sign out these browser accounts' })); }
	};
	return <div className="portico-settings-form browser-accounts-settings">
		<SettingsGroup title="Sign-in behavior" description="Choose what this server does when Portico opens in this browser.">
			{auth.browserAccounts.status === 'error' && <InlineNotice tone="error" action={<SecondaryButton onClick={auth.retryBrowserAccounts}>Retry</SecondaryButton>}>{reviewedProductErrorText(auth.browserAccounts.error, 'auth.browser-accounts-load-failed')}</InlineNotice>}
			{error && <InlineNotice tone="error">{error}</InlineNotice>}
			{feedback && <InlineNotice tone="success">{feedback}</InlineNotice>}
			<SettingRow label="Automatically sign in" description="Open the most recently used remembered account. Turn this off to choose an account whenever no session is active."><ToggleControl label="Automatically sign in" value={auth.browserAccounts.data.automaticSignIn} disabled={auth.busy} onChange={(value) => void setAutomaticSignIn(value)} /></SettingRow>
		</SettingsGroup>
		<SettingsGroup title="Accounts on this browser" description={`${accounts.length} ${accounts.length === 1 ? 'account is' : 'accounts are'} remembered for ${viewer.serverName}.`} actions={auth.browserAccounts.data.canAddAccount ? <PrimaryButton disabled={auth.busy} onClick={auth.beginAddAccount}><UserPlus /> Add account</PrimaryButton> : undefined}>
			{auth.browserAccounts.status === 'loading' && accounts.length === 0 && <SettingsLoading label="Loading browser accounts" />}
			{auth.browserAccounts.status !== 'loading' && accounts.length === 0 && <div className="browser-account-empty"><MonitorSmartphone /><strong>No accounts are remembered</strong><p>Sign in and choose to remember the account to make switching available here.</p></div>}
			<div className="browser-account-settings-list">{accounts.map((account) => {
				const active = account.id === viewer.id;
				const initial = account.displayName.trim().slice(0, 1).toLocaleUpperCase() || 'P';
				return <div key={account.id} className={active ? 'active' : ''}>
					<span className="browser-account-avatar">{account.profileImageUrl ? <img src={account.profileImageUrl} alt="" /> : initial}</span>
					<span className="browser-account-copy"><strong>{account.displayName}</strong><small>{account.authProvider === 'portico' ? 'Portico Account' : 'This Server'} · Used {relativeTime(account.lastUsedAt)}</small></span>
					{active && <span className="browser-account-active"><CheckCircle2 /> Current</span>}
					{!active && <SecondaryButton disabled={auth.busy} onClick={() => void switchAccount(account.id)}>Switch</SecondaryButton>}
					{confirming === account.id ? <div className="browser-account-confirm"><span>Remove this account from the browser?</span><button type="button" onClick={() => setConfirming('')}>Cancel</button><button type="button" className="danger" disabled={auth.busy} onClick={() => void remove(account.id)}>Remove</button></div> : <IconButton label={`Remove ${account.displayName} from this browser`} disabled={auth.busy} onClick={() => setConfirming(account.id)}><Trash2 /></IconButton>}
				</div>;
			})}</div>
		</SettingsGroup>
		<SettingsGroup title="Browser sign out" description="These actions affect this browser only. They do not delete server accounts.">
			<SettingRow label="Sign out current session" description={`End the current ${viewer.displayName} session but keep remembered accounts available.`}><SecondaryButton disabled={auth.busy} onClick={() => void auth.logout()}><LogOut /> Sign out</SecondaryButton></SettingRow>
			<SettingRow label="Sign out all accounts" description="Remove every remembered account for this server and clear the current session.">{confirming === 'all' ? <div className="browser-signout-confirm"><SecondaryButton onClick={() => setConfirming('')}>Cancel</SecondaryButton><button type="button" className="button secondary portico-destructive-button" disabled={auth.busy} onClick={() => void signOutAll()}>{auth.busy ? 'Signing out…' : 'Confirm sign out all'}</button></div> : <button type="button" className="button secondary portico-destructive-button" disabled={auth.busy || accounts.length === 0} onClick={() => setConfirming('all')}><LogOut /> Sign out all</button>}</SettingRow>
		</SettingsGroup>
	</div>;
}

function webPreferenceError(reason: unknown): string {
  return reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'save these web application preferences' });
}

function WebAppearanceSettings() {
  const display = useWebDisplayPreferences();
  const libraries = useLibraries();
  const home = useHome();
  const [draft, setDraft] = useState<WebDisplayPreferences>(display.preferences);
  const [customizingHome, setCustomizingHome] = useState(false);
  const [feedback, setFeedback] = useState('');
  const [error, setError] = useState('');
  useEffect(() => setDraft(display.preferences), [display.preferences]);
  const dirty = JSON.stringify(draft) !== JSON.stringify(display.preferences);
  const save = async () => {
    setFeedback(''); setError('');
    try {
      await display.update(draft);
      setFeedback(productText('preferences.saved'));
    } catch (reason) {
      setError(webPreferenceError(reason));
    }
  };
  const toggleLibrary = (id: string) => setDraft((current) => ({
    ...current,
    pinnedLibraryIds: current.pinnedLibraryIds.includes(id)
      ? current.pinnedLibraryIds.filter((candidate) => candidate !== id)
      : [...current.pinnedLibraryIds, id],
  }));

  if (display.status === 'loading') return <SettingsGroup title={productText('preferences.web-appearance-title')} description={productText('preferences.web-appearance-loading-description')}><SettingsLoading label={productText('preferences.loading-web-appearance')} /></SettingsGroup>;
  return <div className="portico-settings-form">
    <SettingsGroup title={productText('preferences.web-appearance-title')} description={productText('preferences.web-appearance-description')}>
      {display.status === 'error' && <InlineNotice tone="error" action={<SecondaryButton onClick={display.retry}>{productText('action.retry')}</SecondaryButton>}>{reviewedProductErrorText(display.error, 'preferences.load-failed')}</InlineNotice>}
      <SettingRow label={productText('preferences.web-backdrop-label')} description={productText('preferences.web-backdrop-description')}><ToggleControl label={productText('preferences.web-backdrop-label')} value={draft.showBackdrops} onChange={(showBackdrops) => setDraft({ ...draft, showBackdrops })} /></SettingRow>
      <SettingRow label={productText('preferences.card-size-label')} description={productText('preferences.card-size-description')}><NumberControl label={productText('preferences.card-size-label')} value={draft.cardSizePercent} min={viewerPreferenceLimitsV1.cardSizePercent.minimum} max={viewerPreferenceLimitsV1.cardSizePercent.maximum} step={5} unit="%" onChange={(cardSizePercent) => cardSizePercent !== undefined && setDraft({ ...draft, cardSizePercent })} /></SettingRow>
      <SettingRow label={productText('preferences.reduce-motion-label')} description={productText('preferences.reduce-motion-description')}><ToggleControl label={productText('preferences.reduce-motion-label')} value={draft.reduceMotion} onChange={(reduceMotion) => setDraft({ ...draft, reduceMotion })} /></SettingRow>
    </SettingsGroup>
    <SettingsGroup title={productText('preferences.pinned-libraries-title')} description={productText('preferences.pinned-libraries-description')}>
      {libraries.status === 'loading' && <div className="portico-settings-compact-state"><span className="portico-settings-spinner"><SlidersHorizontal /></span><span><strong>{productText('preferences.libraries-loading')}</strong><small>{productMessage('preferences.libraries-loading').body}</small></span></div>}
      {libraries.status === 'error' && <InlineNotice tone="error">{reviewedProductErrorText(libraries.error, 'preferences.libraries-load-failed')}</InlineNotice>}
      {libraries.status === 'success' && libraries.data.length === 0 && <div className="portico-settings-state"><Film /><strong>{productText('preferences.libraries-empty')}</strong><p>{productMessage('preferences.libraries-empty').body}</p></div>}
      {libraries.status === 'success' && libraries.data.length > 0 && <div className="portico-pinned-library-list">{libraries.data.map((library) => {
        const selected = draft.pinnedLibraryIds.includes(library.id);
        return <label key={library.id}><input type="checkbox" checked={selected} onChange={() => toggleLibrary(library.id)} /><span><strong>{library.name}</strong><small>{library.kind.replace('-', ' ')} · {productText('preferences.library-item-count', { count: library.itemCount, unit: library.itemCount === 1 ? 'item' : 'items' })}</small></span><b>{productText(selected ? 'preferences.library-status-pinned' : 'preferences.library-status-available')}</b></label>;
      })}</div>}
    </SettingsGroup>
    <SettingsGroup title={productText('preferences.home-layout-title')} description={productText('preferences.home-layout-description')}>
      {home.status === 'error' && <InlineNotice tone="error">{reviewedProductErrorText(home.error, 'preferences.load-failed')}</InlineNotice>}
      <div className="portico-home-layout-entry"><ListVideo /><span><strong>{home.status === 'success' ? productText('preferences.home-rows-count', { count: home.data.rows.length }) : productText('preferences.home-rows-loading')}</strong><small>{productText('preferences.hidden-count', { count: draft.hiddenHomeRows.length })} · {productText(draft.homeRowOrder.length ? 'preferences.home-order-custom' : 'preferences.home-order-server')}</small></span><SecondaryButton disabled={home.status !== 'success' || home.data.rows.length === 0} onClick={() => setCustomizingHome(true)}>{productText('action.customize-rows')} <ChevronRight /></SecondaryButton></div>
    </SettingsGroup>
    <SaveBar dirty={dirty} busy={display.busy} feedback={feedback} error={error} onSave={save} onReset={() => { setDraft(display.preferences); setFeedback(''); setError(''); }} />
    {customizingHome && home.status === 'success' && <HomeCustomizationDialog rows={home.data.rows} preferences={draft} busy={display.busy} onDismiss={() => setCustomizingHome(false)} onSave={async (next) => { await display.update(next); setDraft(next); setFeedback(productText('preferences.home-layout-saved')); }} />}
  </div>;
}

function deliveryRequestChoice(request: WebDisplayPreferences['deliveryRequest']): string {
  if (request.transcode === 'require') return 'transcode';
  if (request.directStream === 'prefer' && request.directPlay !== 'prefer') return 'direct-stream';
  return 'automatic';
}

function deliveryRequestForChoice(value: string): WebDisplayPreferences['deliveryRequest'] {
  if (value === 'direct-stream') return { directPlay: 'allow', directStream: 'prefer', transcode: 'allow' };
  if (value === 'transcode') return { directPlay: 'allow', directStream: 'allow', transcode: 'require' };
  return { directPlay: 'prefer', directStream: 'allow', transcode: 'allow' };
}

function WebPlaybackSettings() {
  const display = useWebDisplayPreferences();
  const [draft, setDraft] = useState<WebDisplayPreferences>(display.preferences);
  const [feedback, setFeedback] = useState('');
  const [error, setError] = useState('');
  useEffect(() => setDraft(display.preferences), [display.preferences]);
  const dirty = JSON.stringify(draft) !== JSON.stringify(display.preferences);
  const save = async () => {
    setFeedback(''); setError('');
    try {
      await display.update(draft);
      setFeedback(productText('preferences.saved'));
    } catch (reason) {
      setError(webPreferenceError(reason));
    }
  };
  if (display.status === 'loading') return <SettingsGroup title={productText('preferences.web-player-title')} description={productText('preferences.web-player-description')}><SettingsLoading label={productText('preferences.loading-web-player')} /></SettingsGroup>;
  return <div className="portico-settings-form">
    <SettingsGroup title="Web player controls" description="Skip intervals and automatic playback behavior in this browser.">
      {display.status === 'error' && <InlineNotice tone="error" action={<SecondaryButton onClick={display.retry}>{productText('action.retry')}</SecondaryButton>}>{reviewedProductErrorText(display.error, 'preferences.load-failed')}</InlineNotice>}
      <SettingRow label="Skip backward" description="Seconds moved by the player’s backward skip control."><ChoiceControl label="Skip backward" value={String(draft.skipBackSeconds)} options={[5, 10, 15].map((value) => ({ value: String(value), label: `${value} seconds` }))} onChange={(value) => setDraft({ ...draft, skipBackSeconds: Number(value) as WebDisplayPreferences['skipBackSeconds'] })} /></SettingRow>
      <SettingRow label="Skip forward" description="Seconds moved by the player’s forward skip control."><ChoiceControl label="Skip forward" value={String(draft.skipForwardSeconds)} options={[15, 30, 45].map((value) => ({ value: String(value), label: `${value} seconds` }))} onChange={(value) => setDraft({ ...draft, skipForwardSeconds: Number(value) as WebDisplayPreferences['skipForwardSeconds'] })} /></SettingRow>
      <SettingRow label="Autoplay next item" description="Begin the next episode or queue item when the current item finishes."><ToggleControl label="Autoplay next item" value={draft.autoplayNext} onChange={(autoplayNext) => setDraft({ ...draft, autoplayNext })} /></SettingRow>
      <SettingRow label="Up Next countdown" description="Delay before autoplay begins. Choose Off to wait for an explicit selection."><ChoiceControl label="Up Next countdown" value={String(draft.upNextCountdownSeconds)} options={[{ value: '0', label: 'Off' }, { value: '5', label: '5 seconds' }, { value: '10', label: '10 seconds' }, { value: '15', label: '15 seconds' }]} onChange={(value) => setDraft({ ...draft, upNextCountdownSeconds: Number(value) as WebDisplayPreferences['upNextCountdownSeconds'] })} /></SettingRow>
      <SettingRow label="Default playback speed" description="Speed used when a new video or audiobook starts."><ChoiceControl label="Default playback speed" value={draft.defaultPlaybackSpeed} options={[{ value: '1', label: 'Normal' }, { value: '1.25', label: '1.25×' }, { value: '1.5', label: '1.5×' }]} onChange={(defaultPlaybackSpeed) => setDraft({ ...draft, defaultPlaybackSpeed: defaultPlaybackSpeed as WebDisplayPreferences['defaultPlaybackSpeed'] })} /></SettingRow>
      <SettingRow label="Skip intros" description="Ask first, skip automatically only when the server marks the intro safe, or turn the action off."><ChoiceControl label="Skip intros" value={draft.introSkip} options={[{ value: 'ask', label: 'Ask first' }, { value: 'automatic', label: 'Automatic when safe' }, { value: 'off', label: 'Off' }]} onChange={(introSkip) => setDraft({ ...draft, introSkip: introSkip as WebDisplayPreferences['introSkip'] })} /></SettingRow>
      <SettingRow label="Skip credits" description="Choose how verified credits markers behave. Uncertain markers always require confirmation."><ChoiceControl label="Skip credits" value={draft.creditsSkip} options={[{ value: 'ask', label: 'Ask first' }, { value: 'automatic', label: 'Automatic when safe' }, { value: 'off', label: 'Off' }]} onChange={(creditsSkip) => setDraft({ ...draft, creditsSkip: creditsSkip as WebDisplayPreferences['creditsSkip'] })} /></SettingRow>
      <SettingRow label="Passout protection" description="Pause automatic episode advances and ask whether you are still watching."><ToggleControl label="Passout protection" value={draft.passoutProtection} onChange={(passoutProtection) => setDraft({ ...draft, passoutProtection })} /></SettingRow>
      {draft.passoutProtection && <SettingRow label="Still-watching check" description="Automatic episode advances allowed before Portico pauses the queue."><ChoiceControl label="Still-watching check" value={String(draft.passoutAfterEpisodes)} options={[2, 3, 4, 5].map((value) => ({ value: String(value), label: `After ${value} episodes` }))} onChange={(value) => setDraft({ ...draft, passoutAfterEpisodes: Number(value) as WebDisplayPreferences['passoutAfterEpisodes'] })} /></SettingRow>}
    </SettingsGroup>
    <SettingsGroup title="Playback delivery" description="Requests sent to the server for this browser. Server limits and browser compatibility always take priority.">
      <SettingRow label="Delivery preference" description="Recommended prefers Direct Play, permits remux, and falls back to transcoding. The server explains the final delivery mode."><ChoiceControl label="Delivery preference" value={deliveryRequestChoice(draft.deliveryRequest)} options={[{ value: 'automatic', label: 'Recommended' }, { value: 'direct-stream', label: 'Prefer Direct Stream' }, { value: 'transcode', label: 'Require Transcode' }]} onChange={(value) => setDraft({ ...draft, deliveryRequest: deliveryRequestForChoice(value) })} /></SettingRow>
      {(['local', 'wifi', 'cellular', 'unknown'] as const).map((network) => <SettingRow key={network} label={`${network === 'local' ? 'Local / LAN' : network[0].toUpperCase() + network.slice(1)} quality`} description={`Preferred quality on ${network === 'local' ? 'this server’s local network' : network === 'unknown' ? 'remote or unclassified connections; the server applies its own remote clamp' : `${network} connections`}.`}><ChoiceControl label={`${network} quality`} value={draft.playbackQuality[network]} options={[{ value: 'off', label: 'Off' }, { value: 'automatic', label: 'Automatic' }, { value: 'original', label: 'Original' }, { value: 'high', label: 'High' }, { value: 'standard', label: 'Standard' }, { value: 'data-saver', label: 'Data Saver' }]} onChange={(value) => setDraft({ ...draft, playbackQuality: { ...draft.playbackQuality, [network]: value as WebDisplayPreferences['playbackQuality'][typeof network] } })} /></SettingRow>)}
    </SettingsGroup>
    <SettingsGroup title={productText('preferences.subtitles-lyrics-title')} description={productText('preferences.subtitles-lyrics-description')}>
      <SettingRow label={productText('preferences.subtitle-size-label')} description={productText('preferences.subtitle-size-description')}><ChoiceControl label={productText('preferences.subtitle-size-label')} value={draft.subtitleSize} options={(['small', 'medium', 'large'] as const).map((value) => ({ value, label: productText(`preferences.option-${value}`) }))} onChange={(subtitleSize) => setDraft({ ...draft, subtitleSize: subtitleSize as WebDisplayPreferences['subtitleSize'] })} /></SettingRow>
      <SettingRow label={productText('preferences.subtitle-background-label')} description={productText('preferences.subtitle-background-description')}><ChoiceControl label={productText('preferences.subtitle-background-label')} value={draft.subtitleBackground} options={(['none', 'subtle', 'solid'] as const).map((value) => ({ value, label: productText(`preferences.option-${value}`) }))} onChange={(subtitleBackground) => setDraft({ ...draft, subtitleBackground: subtitleBackground as WebDisplayPreferences['subtitleBackground'] })} /></SettingRow>
      <SettingRow label={productText('preferences.synced-lyrics-label')} description={productText('preferences.synced-lyrics-description')}><ToggleControl label={productText('preferences.synced-lyrics-label')} value={draft.showSyncedLyrics} onChange={(showSyncedLyrics) => setDraft({ ...draft, showSyncedLyrics })} /></SettingRow>
      <SettingRow label={productText('preferences.diagnostics-label')} description={productText('preferences.diagnostics-description')}><ToggleControl label={productText('preferences.diagnostics-label')} value={draft.playbackDiagnostics} onChange={(playbackDiagnostics) => setDraft({ ...draft, playbackDiagnostics })} /></SettingRow>
    </SettingsGroup>
    <SaveBar dirty={dirty} busy={display.busy} feedback={feedback} error={error} onSave={save} onReset={() => { setDraft(display.preferences); setFeedback(''); setError(''); }} />
  </div>;
}

function WebPrivacySettings() {
  const display = useWebDisplayPreferences();
  const [draft, setDraft] = useState<WebDisplayPreferences>(display.preferences);
  const [feedback, setFeedback] = useState('');
  const [error, setError] = useState('');
  useEffect(() => setDraft(display.preferences), [display.preferences]);
  const dirty = JSON.stringify(draft) !== JSON.stringify(display.preferences);
  const save = async () => {
    setFeedback(''); setError('');
    try {
      await display.update(draft);
      setFeedback(productText('preferences.saved'));
    } catch (reason) {
      setError(webPreferenceError(reason));
    }
  };
  if (display.status === 'loading') return <SettingsGroup title={productText('preferences.search-history-title')} description={productText('preferences.search-history-loading-description')}><SettingsLoading label={productText('preferences.loading-search')} /></SettingsGroup>;
  return <div className="portico-settings-form">
    <SettingsGroup title={productText('preferences.search-history-title')} description={productText('preferences.search-history-description')}>
      {display.status === 'error' && <InlineNotice tone="error" action={<SecondaryButton onClick={display.retry}>{productText('action.retry')}</SecondaryButton>}>{reviewedProductErrorText(display.error, 'preferences.load-failed')}</InlineNotice>}
      <SettingRow label={productText('preferences.search-remember-label')} description={productText('preferences.search-remember-description')}><ToggleControl label={productText('preferences.search-remember-label')} value={draft.rememberSearchHistory} onChange={(rememberSearchHistory) => setDraft({ ...draft, rememberSearchHistory })} /></SettingRow>
      <div className="portico-search-history-list">
        <header><span><strong>{productText('preferences.recent-searches-title')}</strong><small>{draft.recentSearches.length ? productText('preferences.recent-search-saved-count', { count: draft.recentSearches.length }) : productText('preferences.recent-searches-empty')}</small></span>{draft.recentSearches.length > 0 && <SecondaryButton onClick={() => setDraft({ ...draft, recentSearches: [] })}><Trash2 /> {productText('action.clear-all')}</SecondaryButton>}</header>
        {draft.recentSearches.map((query) => <div key={query}><Search /><span>{query}</span><IconButton label={productText('preferences.recent-search-remove', { query })} onClick={() => setDraft({ ...draft, recentSearches: draft.recentSearches.filter((candidate) => candidate !== query) })}><Trash2 /></IconButton></div>)}
      </div>
    </SettingsGroup>
    <SaveBar dirty={dirty} busy={display.busy} feedback={feedback} error={error} onSave={save} onReset={() => { setDraft(display.preferences); setFeedback(''); setError(''); }} />
  </div>;
}

function PreferencesEditor({ section, source }: { section: 'appearance' | 'personal-playback' | 'privacy'; source: SettingsDataSource }) {
  const display = useWebDisplayPreferences();
  const original = display.bundle?.profileServer.values;
  const [draft, setDraft] = useState<typeof original>();
  const [feedback, setFeedback] = useState('');
  const [error, setError] = useState('');
  useEffect(() => { if (original) setDraft(structuredClone(original)); }, [original]);
  if (!draft || !original) return <SettingsLoading label={productText('preferences.loading-personal')} />;
  const save = async () => {
    setFeedback(''); setError('');
    const changes = section === 'appearance' ? { localization: draft.localization } : section === 'personal-playback' ? { playback: draft.playback, music: draft.music } : { privacy: draft.privacy };
    try { await display.patchScope('profile-server', changes); setFeedback(productText('preferences.saved')); }
    catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'save these preferences' })); }
  };
  const saveBar = <SaveBar dirty={JSON.stringify(draft) !== JSON.stringify(original)} busy={display.busy} feedback={feedback} error={error} onSave={save} onReset={() => setDraft(structuredClone(original))} />;
  if (section === 'appearance') return <div className="portico-settings-form"><SettingsGroup title={productText('preferences.region-format-title')} description={productText('preferences.region-format-description')}>
    <SettingRow label={productText('preferences.locale-label')} description={productText('preferences.locale-description')}><TextControl label={productText('preferences.locale-label')} value={draft.localization.locale} placeholder="en-CA" onChange={(locale) => setDraft({ ...draft, localization: { ...draft.localization, locale } })} /></SettingRow>
    <SettingRow label={productText('preferences.time-zone-label')} description={productText('preferences.time-zone-description')}><TextControl label={productText('preferences.time-zone-label')} value={draft.localization.timeZone} placeholder="America/Halifax" onChange={(timeZone) => setDraft({ ...draft, localization: { ...draft.localization, timeZone } })} /></SettingRow>
    <SettingRow label={productText('preferences.clock-label')} description={productText('preferences.clock-description')}><ChoiceControl label={productText('preferences.clock-label')} value={draft.localization.hourCycle} options={[{ value: 'auto', label: productText('preferences.clock-option-system') }, { value: 'h12', label: productText('preferences.clock-option-12-hour') }, { value: 'h23', label: productText('preferences.clock-option-24-hour') }]} onChange={(hourCycle) => setDraft({ ...draft, localization: { ...draft.localization, hourCycle: hourCycle as typeof draft.localization.hourCycle } })} /></SettingRow>
  </SettingsGroup>{saveBar}</div>;
  if (section === 'personal-playback') return <div className="portico-settings-form"><SettingsGroup title={productText('preferences.language-progress-title')} description={productText('preferences.language-progress-description')}>
    <SettingRow label={productText('preferences.personal-playback-audio-label')} description={productText('preferences.personal-playback-audio-description')}><TextControl label={productText('preferences.personal-playback-audio-label')} value={draft.playback.preferredAudioLanguages[0] ?? ''} placeholder="en" onChange={(value) => setDraft({ ...draft, playback: { ...draft.playback, preferredAudioLanguages: value.trim() ? [value.trim()] : [] } })} /></SettingRow>
    <SettingRow label={productText('preferences.personal-playback-subtitles-label')} description={productText('preferences.personal-playback-subtitles-description')}><TextControl label={productText('preferences.personal-playback-subtitles-label')} value={draft.playback.preferredSubtitleLanguages[0] ?? ''} placeholder="en" onChange={(value) => setDraft({ ...draft, playback: { ...draft.playback, preferredSubtitleLanguages: value.trim() ? [value.trim()] : [] } })} /></SettingRow>
    <SettingRow label={productText('preferences.started-threshold-label')} description={productText('preferences.started-threshold-description')}><NumberControl label={productText('preferences.started-threshold-label')} value={draft.playback.startedThresholdPercent} min={viewerPreferenceLimitsV1.startedThresholdPercent.minimum} max={viewerPreferenceLimitsV1.startedThresholdPercent.maximum} unit="%" onChange={(value) => value !== undefined && setDraft({ ...draft, playback: { ...draft.playback, startedThresholdPercent: value } })} /></SettingRow>
    <SettingRow label={productText('preferences.watched-threshold-label')} description={productText('preferences.watched-threshold-description')}><NumberControl label={productText('preferences.watched-threshold-label')} value={draft.playback.playedThresholdPercent} min={viewerPreferenceLimitsV1.playedThresholdPercent.minimum} max={viewerPreferenceLimitsV1.playedThresholdPercent.maximum} unit="%" onChange={(value) => value !== undefined && setDraft({ ...draft, playback: { ...draft.playback, playedThresholdPercent: value } })} /></SettingRow>
  </SettingsGroup><SettingsGroup title={productText('preferences.music-title')} description={productText('preferences.music-description')}>
    <SettingRow label={productText('preferences.music-autoplay-label')} description={productText('preferences.music-autoplay-description')}><ToggleControl label={productText('preferences.music-autoplay-label')} value={draft.music.autoplayDefault} onChange={(autoplayDefault) => setDraft({ ...draft, music: { ...draft.music, autoplayDefault } })} /></SettingRow>
    <SettingRow label={productText('preferences.gapless-label')} description={productText('preferences.gapless-description')}><ToggleControl label={productText('preferences.gapless-label')} value={draft.music.gapless} onChange={(gapless) => setDraft({ ...draft, music: { ...draft.music, gapless } })} /></SettingRow>
    <SettingRow label={productText('preferences.crossfade-label')} description={productText('preferences.crossfade-description')}><NumberControl label={productText('preferences.crossfade-label')} value={draft.music.crossfadeSeconds} min={0} max={12} unit="seconds" onChange={(value) => value !== undefined && setDraft({ ...draft, music: { ...draft.music, crossfadeSeconds: value } })} /></SettingRow>
  </SettingsGroup>{saveBar}</div>;
  return <div className="portico-settings-form"><SettingsGroup title={productText('preferences.visibility-title')} description={productText('preferences.visibility-description')}>
    <SettingRow label={productText('preferences.pause-history-label')} description={productText('preferences.pause-history-description')}><ToggleControl label={productText('preferences.pause-history-label')} value={draft.privacy.pauseWatchHistory} onChange={(pauseWatchHistory) => setDraft({ ...draft, privacy: { ...draft.privacy, pauseWatchHistory } })} /></SettingRow>
    <SettingRow label={productText('preferences.show-activity-label')} description={productText('preferences.show-activity-description')}><ToggleControl label={productText('preferences.show-activity-label')} value={draft.privacy.showActivityToMembers} onChange={(showActivityToMembers) => setDraft({ ...draft, privacy: { ...draft.privacy, showActivityToMembers } })} /></SettingRow>
    <SettingRow label={productText('preferences.include-watch-with-friends-label')} description={productText('preferences.include-watch-with-friends-description')}><ToggleControl label={productText('preferences.include-watch-with-friends-label')} value={draft.privacy.includeInWatchWithFriends} onChange={(includeWatchWithFriends) => setDraft({ ...draft, privacy: { ...draft.privacy, includeInWatchWithFriends: includeWatchWithFriends } })} /></SettingRow>
  </SettingsGroup><ClearHistory source={source} />{saveBar}</div>;
}

function ClearHistory({ source }: { source: SettingsDataSource }) {
  const mutation = useAbortableMutation();
  const [confirming, setConfirming] = useState(false);
  const [feedback, setFeedback] = useState('');
  const [error, setError] = useState('');
  const clear = async () => {
    setError(''); setFeedback('');
    try { await mutation.run((signal) => source.clearWatchHistory(signal)); setConfirming(false); setFeedback(productText('preferences.history-cleared')); }
    catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'clear watch history' })); }
  };
  return <SettingsGroup title={productText('preferences.history-title')} description={productText('preferences.history-description')}>
    {(feedback || error) && <InlineNotice tone={error ? 'error' : 'success'}>{error || feedback}</InlineNotice>}
    <SettingRow label={productText('preferences.history-clear-label')} description={productText('preferences.history-clear-confirmation')}>{confirming ? <div className="portico-confirm-actions"><SecondaryButton disabled={mutation.busy} onClick={() => setConfirming(false)}>{productText('action.cancel')}</SecondaryButton><button type="button" className="button secondary portico-destructive-button" disabled={mutation.busy} onClick={() => void clear()}><Trash2 />{productText(mutation.busy ? 'action.clearing-history' : 'action.clear-history')}</button></div> : <SecondaryButton onClick={() => setConfirming(true)}><Trash2 /> {productText('action.clear-history')}</SecondaryButton>}</SettingRow>
  </SettingsGroup>;
}

export function PersonalPreferencesSettings({ section, source }: { section: 'appearance' | 'personal-playback' | 'privacy'; source: SettingsDataSource }) {
  return <div className="portico-settings-content">
    {section === 'appearance' && <WebAppearanceSettings />}
    {section === 'personal-playback' && <WebPlaybackSettings />}
    <PreferencesEditor section={section} source={source} />
    {section === 'privacy' && <WebPrivacySettings />}
  </div>;
}

export function HelpSettings({ operations }: { operations: SettingsOperationalSnapshot }) {
  const { release } = operations;
  return <div className="portico-settings-form">
    <SettingsGroup title="Portico" description="Installed server and API details.">
      <div className="portico-about-product"><span className="portico-about-mark">P</span><span><strong>Portico {release.version}</strong><small>API {release.apiVersion} · {release.goos}/{release.goarch} · {release.installMethod}</small></span></div>
      <SettingRow label="Updates" description="Server update availability."><span className="portico-setting-readonly">This feature is not yet available.</span></SettingRow>
      <SettingRow label="Database" description="Database readiness and migration status."><span className={`portico-setting-readonly ${release.databaseReady ? 'healthy' : 'danger'}`}>{release.databaseReady ? <CheckCircle2 /> : <AlertTriangle />}{release.migrationStatus}</span></SettingRow>
      <SettingRow label="Web application" description="Packaged Portico web application status."><span className={`portico-setting-readonly ${release.webDistReady ? 'healthy' : 'danger'}`}>{release.webDistReady ? <CheckCircle2 /> : <AlertTriangle />}{release.webDistReady ? 'Ready' : 'Unavailable'}</span></SettingRow>
    </SettingsGroup>
    <SettingsGroup title="Support" description="Useful destinations and local diagnostic status.">
      <div className="portico-support-links"><a href="https://getportico.tv/docs" target="_blank" rel="noreferrer"><MonitorCog /><span><strong>Documentation</strong><small>Setup, playback, remote access, and administration</small></span><ExternalLink /></a><a href="https://getportico.tv/support" target="_blank" rel="noreferrer"><ShieldCheck /><span><strong>Support</strong><small>Help with this Portico installation</small></span><ExternalLink /></a></div>
      <InlineNotice tone="info">Server diagnostics are available under This Server when you need them.</InlineNotice>
    </SettingsGroup>
  </div>;
}
