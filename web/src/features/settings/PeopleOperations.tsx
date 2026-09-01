import type { APIKey, Device, Permissions, PorticoInvite, User, UserCreateRequest, UserPatchRequest } from '@porticomediaserver/client-core';
import { StatusWarningIcon, ActionConfirmIcon, ViewListIcon, AccountSecurityIcon, DeviceClientIcon, ActionEditIcon, ActionAddIcon, StatusSecureIcon, ActionDeleteIcon, ActionCloseIcon } from '#portico-icons';
import { useEffect, useMemo, useRef, useState } from 'react';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { StableImage } from '../../components/media/StableImage';
import { ModalOverlay } from '../../components/overlay/OverlayPortal';
import { reviewedProductErrorText } from '../../components/ProductLanguage';
import { InlineNotice, SettingsGroup, TextControl, ToggleControl } from './SettingsControls';
import { useAbortableMutation } from './settingsHooks';
import { setSettingsNavigationDirty } from './settingsNavigationGuard';
import type { SettingsDataSource, SettingsOperationalSnapshot } from './settingsTypes';

const permissionPresentations: Record<string, { label: string; description: string }> = {
  read: { label: 'Read library data', description: 'Browse libraries, media details, and safe server projections.' },
  playMedia: { label: 'Play media', description: 'Play available library media.' },
  downloadMedia: { label: 'Download media', description: 'Download available media files.' },
  editMetadata: { label: 'Edit metadata', description: 'Change titles, artwork, and media details.' },
  manageLyrics: { label: 'Manage lyrics', description: 'Add, edit, and remove lyrics.' },
  manageSubtitles: { label: 'Manage subtitles', description: 'Add, edit, and remove subtitle tracks.' },
  watchWithFriends: { label: 'Use Watch With Friends', description: 'Create and join synchronized watch sessions.' },
  viewLiveTV: { label: 'View Live TV', description: 'Browse channels and the program guide.' },
  playLiveTV: { label: 'Play Live TV', description: 'Watch available live channels.' },
  viewDVR: { label: 'View DVR', description: 'Browse recording rules and recorded programs.' },
  scheduleDVR: { label: 'Schedule DVR recordings', description: 'Create and update recording schedules.' },
  manageDVR: { label: 'Manage DVR', description: 'Manage DVR rules and recordings. Live TV source administration remains interactive-owner only.' },
  deleteDVRRecordings: { label: 'Delete DVR recordings', description: 'Permanently remove recorded programs.' },
  deleteMedia: { label: 'Delete media', description: 'Permanently remove media from this server.' },
  transcode: { label: 'Transcode media', description: 'Create compatible playback versions.' },
};

// API keys are intentionally narrower than the account permission catalog.
// Keep this list aligned with the server's validated API-key scope vocabulary;
// never derive credential authority from capabilities intended for human users.
export const apiKeyScopeCatalog = [
  'read', 'playMedia', 'downloadMedia', 'editMetadata', 'manageLyrics',
  'manageSubtitles', 'watchWithFriends', 'viewLiveTV', 'playLiveTV', 'viewDVR',
  'scheduleDVR', 'manageDVR', 'deleteDVRRecordings', 'deleteMedia', 'transcode',
] as const;

export function canonicalPermissionID(value: string): string | undefined {
  const trimmed = value.trim();
  if (Object.hasOwn(permissionPresentations, trimmed)) return trimmed;
  const lowerCamel = trimmed ? `${trimmed[0]!.toLocaleLowerCase()}${trimmed.slice(1)}` : '';
  return Object.hasOwn(permissionPresentations, lowerCamel) ? lowerCamel : undefined;
}

function canonicalPermissionCatalog(values: string[]): string[] {
  return [...new Set(values.map(canonicalPermissionID).filter((value): value is string => Boolean(value)))];
}

export function permissionPresentation(value: string): { label: string; description: string } {
  return permissionPresentations[value] ?? {
    label: value.replace(/([a-z0-9])([A-Z])/g, '$1 $2').replaceAll(/[:._-]+/g, ' ').replace(/\b\w/g, (letter) => letter.toUpperCase()),
    description: 'Use this server capability.',
  };
}

function PermissionCopy({ value }: { value: string }) {
  const presentation = permissionPresentation(value);
  return <span><strong>{presentation.label}</strong><small>{presentation.description}</small></span>;
}

function permissionLabel(value: string) {
  return <PermissionCopy value={value} />;
}

function accountUsername(user: User): string {
  return user.username || user.displayName;
}

function invitationProjection(invite: PorticoInvite): { label: string; problem: boolean } {
  if (invite.status === 'accepted' || invite.acceptedAt) return { label: 'Accepted', problem: false };
  if (invite.status === 'revoked' || invite.revokedAt) return { label: 'Revoked', problem: false };
  if (invite.status === 'expired' || Date.parse(invite.expiresAt) <= Date.now()) return { label: 'Expired', problem: false };
  if (invite.deliveryMode === 'email' && ['dead-letter', 'failed', 'invalid'].includes(invite.emailDeliveryStatus ?? '')) return { label: 'Delivery problem', problem: true };
  return { label: invite.deliveryMode === 'email' ? 'Sent' : 'Link created', problem: false };
}

function invitationCanBeRevoked(invite: PorticoInvite): boolean {
  if (invite.status !== 'pending' || invite.acceptedAt || invite.revokedAt) return false;
  const expiresAt = Date.parse(invite.expiresAt);
  return !Number.isFinite(expiresAt) || expiresAt > Date.now();
}

function invitationRecipientLabel(invite: PorticoInvite): string {
  if (invite.invitedUsername) return `@${invite.invitedUsername}`;
  return invite.invitedEmail || 'Portico account';
}

function UserEditor({ user, operations, source, onDismiss, onSaved }: { user?: User; operations: SettingsOperationalSnapshot; source: SettingsDataSource; onDismiss: () => void; onSaved: () => void }) {
  const [username, setUsername] = useState(user?.username || user?.displayName || '');
  const [email, setEmail] = useState(user?.email ?? '');
  const [password, setPassword] = useState('');
  const [libraryIds, setLibraryIds] = useState<string[]>(user?.libraryIds ?? operations.libraries.map((library) => library.id));
  const [permissions, setPermissions] = useState<Permissions>(user?.permissions ?? {});
  const [error, setError] = useState('');
  const mutation = useAbortableMutation();
  const porticoAccount = user?.authOrigin === 'portico';
  const submit = async () => {
    if (!username.trim()) { setError('Enter a username.'); return; }
    if (!user && password.length < 8) { setError('Use a password with at least 8 characters.'); return; }
    const common: UserPatchRequest = {
      displayName: username.trim(), username: username.trim(), email: email.trim(), libraryIds, permissions,
      ...(password ? { password } : {}),
      ...(user?.accessSchedule ? { accessSchedule: user.accessSchedule } : {}),
      ...(user?.tagPolicy ? { tagPolicy: user.tagPolicy } : {}),
      ...(user?.devicePolicy ? { devicePolicy: user.devicePolicy } : {}),
      ...(user?.channelPolicy ? { channelPolicy: user.channelPolicy } : {}),
      ...(user?.maxContentRating !== undefined ? { maxContentRating: user.maxContentRating } : {}),
      ...(user?.maxActiveSessions !== undefined ? { maxActiveSessions: user.maxActiveSessions } : {}),
      ...(user?.remoteBitrateLimitMbps !== undefined ? { remoteBitrateLimitMbps: user.remoteBitrateLimitMbps } : {}),
    };
    setError('');
    try {
      await mutation.run((signal) => user
        ? source.updateUser(user, common, signal)
        : source.createUser({ ...common, password, displayName: username.trim(), username: username.trim(), email: email.trim(), libraryIds, permissions } as UserCreateRequest, signal));
      onSaved();
    }
    catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'save this account' })); }
  };
  return <ModalOverlay labelledBy="portico-user-editor-title" className="portico-settings-dialog portico-user-dialog" onDismiss={onDismiss}><header><div><h2 id="portico-user-editor-title">{user ? `Edit ${user.username || user.displayName}` : 'New server account'}</h2><p>{porticoAccount ? 'Portico Account server access' : 'This Server identity and access'}</p></div><IconButton label="Close" onClick={onDismiss}><ActionCloseIcon /></IconButton></header><div className="portico-settings-dialog-fields">{porticoAccount && <InlineNotice tone="info">The username and credentials belong to this member’s Portico Account. This server controls their media access. Profiles remain separate and do not sign in.</InlineNotice>}<label><span>Username</span><TextControl label="Username" value={username} onChange={setUsername} disabled={porticoAccount} /></label><label><span>Email</span><TextControl label="Email" type="email" value={email} onChange={setEmail} disabled={porticoAccount} /></label>{!porticoAccount && <label><span>{user ? 'New password' : 'Password'} {user && <small>Leave blank to keep the current password</small>}</span><TextControl label={user ? 'New password' : 'Password'} type="password" value={password} onChange={setPassword} /></label>}<fieldset><legend>Libraries</legend><div className="portico-settings-checkbox-grid">{operations.libraries.map((library) => <label key={library.id}><input type="checkbox" checked={libraryIds.includes(library.id)} onChange={(event) => setLibraryIds((current) => event.target.checked ? [...current, library.id] : current.filter((id) => id !== library.id))} /><span>{library.name}</span></label>)}</div></fieldset><fieldset><legend>Permissions</legend><div className="portico-settings-checkbox-grid">{operations.capabilities.permissionCatalog.map((permission) => <label key={permission}><input type="checkbox" checked={permissions[permission] === true} onChange={(event) => setPermissions((current) => ({ ...current, [permission]: event.target.checked }))} /><span>{permissionLabel(permission)}</span></label>)}</div></fieldset>{error && <p className="portico-settings-dialog-error" role="alert"><StatusWarningIcon />{error}</p>}</div><footer><SecondaryButton disabled={mutation.busy} onClick={onDismiss}>Cancel</SecondaryButton><PrimaryButton disabled={mutation.busy} onClick={() => void submit()}>{mutation.busy ? 'Saving…' : user ? 'Save access' : 'Create account'}</PrimaryButton></footer></ModalOverlay>;
}

function PorticoInviteEditor({ operations, source, onDismiss, onSaved }: { operations: SettingsOperationalSnapshot; source: SettingsDataSource; onDismiss: () => void; onSaved: () => void }) {
  const emailRef = useRef<HTMLInputElement>(null);
  const [permissions, setPermissions] = useState<Permissions>({});
  const [deliveryMode, setDeliveryMode] = useState<'email' | 'link'>('email');
  const [createdLink, setCreatedLink] = useState('');
  const [error, setError] = useState('');
  const mutation = useAbortableMutation();
  const permissionCatalog = useMemo(() => canonicalPermissionCatalog(operations.capabilities.permissionCatalog), [operations.capabilities.permissionCatalog]);
  const submit = async () => {
    const recipient = emailRef.current?.value.trim() ?? '';
    if (!recipient || !recipient.includes('@')) { setError('Enter the email address for the Portico Account you want to invite.'); return; }
    setError('');
    try {
      const created = await mutation.run((signal) => source.createPorticoMemberInvite({
        recipient,
        email: recipient,
        permissionTemplate: { permissions },
        deliveryMode,
      }, signal));
      if (deliveryMode === 'link' && created?.inviteUrl) {
        setCreatedLink(created.inviteUrl);
        return;
      }
      if (deliveryMode === 'link') throw new Error('Portico Hosted Services did not return an invitation link.');
      onSaved();
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'create this invitation' }));
    }
  };
  return <ModalOverlay labelledBy="portico-invite-editor-title" className="portico-settings-dialog portico-user-dialog" onDismiss={onDismiss}>
    <header><div><h2 id="portico-invite-editor-title">Invite a Portico Account</h2><p>Hosted membership and server access</p></div><IconButton label="Close" onClick={onDismiss}><ActionCloseIcon /></IconButton></header>
    <div className="portico-settings-dialog-fields">
      <InlineNotice tone="info">Portico Hosted Services stores generic membership grants only. Assign this member’s server libraries locally after the invitation is accepted.</InlineNotice>
      <label><span>Email</span><input ref={emailRef} aria-label="Portico Account email" type="email" /></label>
      <fieldset><legend>Delivery</legend><div className="portico-settings-checkbox-grid">
        <label><input type="radio" name="invite-delivery" checked={deliveryMode === 'email'} onChange={() => setDeliveryMode('email')} /><span>Email the invitation</span></label>
        <label><input type="radio" name="invite-delivery" checked={deliveryMode === 'link'} onChange={() => setDeliveryMode('link')} /><span>Create a link to share yourself</span></label>
      </div></fieldset>
      {createdLink && <InlineNotice tone="success"><span>Invitation link created.</span> <code>{createdLink}</code> <SecondaryButton onClick={() => { void navigator.clipboard.writeText(createdLink).catch(() => setError('The link could not be copied automatically. Select and copy it above.')); }}>Copy link</SecondaryButton></InlineNotice>}
      <fieldset><legend>Permissions</legend><div className="portico-settings-checkbox-grid">{permissionCatalog.map((permission) => <label key={permission}><input type="checkbox" checked={permissions[permission] === true} onChange={(event) => setPermissions((current) => ({ ...current, [permission]: event.target.checked }))} /><span>{permissionLabel(permission)}</span></label>)}</div></fieldset>
      {error && <p className="portico-settings-dialog-error" role="alert"><StatusWarningIcon />{error}</p>}
    </div>
    <footer><SecondaryButton disabled={mutation.busy} onClick={createdLink ? onSaved : onDismiss}>{createdLink ? 'Done' : 'Cancel'}</SecondaryButton>{!createdLink && <PrimaryButton disabled={mutation.busy} onClick={() => void submit()}>{mutation.busy ? 'Creating…' : deliveryMode === 'email' ? 'Send invitation' : 'Create link'}</PrimaryButton>}</footer>
  </ModalOverlay>;
}

function UsersPanel({ operations, source, onChanged }: { operations: SettingsOperationalSnapshot; source: SettingsDataSource; onChanged: () => void }) {
  const [editor, setEditor] = useState<User | 'new' | 'invite' | null>(null);
  const [confirmDelete, setConfirmDelete] = useState('');
  const [confirmRevokeInvite, setConfirmRevokeInvite] = useState('');
  const [error, setError] = useState('');
  const mutation = useAbortableMutation();
  const porticoMode = operations.users.some((user) => user.role === 'owner' && user.authOrigin === 'portico');
  const remove = async (user: User) => {
    setError('');
    try { await mutation.run((signal) => source.deleteUser(user, signal)); setConfirmDelete(''); onChanged(); }
    catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: `remove ${accountUsername(user)}` })); }
  };
  const resend = async (invite: PorticoInvite) => {
    setError('');
    try { await mutation.run((signal) => source.resendPorticoMemberInvite(invite.id, signal)); onChanged(); }
    catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: `retry the invitation to ${invitationRecipientLabel(invite)}` })); }
  };
  const revokeInvite = async (invite: PorticoInvite) => {
    setError('');
    try {
      await mutation.run((signal) => source.revokePorticoMemberInvite(invite.id, signal));
      setConfirmRevokeInvite('');
      onChanged();
    }
    catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: `cancel the invitation to ${invitationRecipientLabel(invite)}` })); }
  };
  return <SettingsGroup title="Members" description="Server-local profiles, linked Portico identities, and library access." actions={<PrimaryButton onClick={() => setEditor(porticoMode ? 'invite' : 'new')}><ActionAddIcon /> {porticoMode ? 'Invite account' : 'New account'}</PrimaryButton>}>
    {error && <InlineNotice tone="error">{error}</InlineNotice>}
    <div className="portico-member-list">
      {operations.users.map((user) => <article key={user.id}><span className="portico-member-avatar"><StableImage src={user.profileImageUrl} alt="" fallback={accountUsername(user).slice(0, 1).toLocaleUpperCase()} /></span><span><strong>{accountUsername(user)}</strong><small>{user.email} · {user.role} · {user.authOrigin === 'portico' ? 'Portico account' : 'This Server'} · {user.libraryIds.length} {user.libraryIds.length === 1 ? 'library' : 'libraries'}</small></span><div>{user.authOrigin === 'portico' && <span className="portico-settings-capability configured"><StatusSecureIcon /> Linked</span>}{user.role !== 'owner' && <IconButton label={`Edit ${accountUsername(user)}`} onClick={() => setEditor(user)}><ActionEditIcon /></IconButton>}{user.role !== 'owner' && (confirmDelete === user.id ? <div className="portico-inline-confirm"><span>Remove {accountUsername(user)} from this server? Their server access and profile data will be permanently deleted; media files are retained.</span><button type="button" onClick={() => setConfirmDelete('')}>Cancel</button><button type="button" className="danger" disabled={mutation.busy} onClick={() => void remove(user)}>Remove</button></div> : <IconButton label={`Remove ${accountUsername(user)}`} onClick={() => setConfirmDelete(user.id)}><ActionDeleteIcon /></IconButton>)}</div></article>)}
      {(operations.porticoInvites ?? []).filter((invite) => invite.status !== 'accepted' && !invite.acceptedAt).map((invite) => {
        const projection = invitationProjection(invite);
        const recipient = invitationRecipientLabel(invite);
        const revocable = invitationCanBeRevoked(invite);
        return <article key={invite.id}><span className="portico-member-avatar">{recipient.replace(/^@/, '').slice(0, 1).toLocaleUpperCase()}</span><span><strong>{recipient}</strong><small>Portico Account invitation · expires {new Date(invite.expiresAt).toLocaleDateString()}</small></span><div><span className={`portico-settings-capability ${projection.problem ? 'unavailable' : 'configured'}`}>{projection.problem ? <StatusWarningIcon /> : <ActionConfirmIcon />}{projection.label}</span>{projection.problem && <SecondaryButton disabled={mutation.busy} onClick={() => void resend(invite)}>Retry email</SecondaryButton>}{confirmRevokeInvite === invite.id ? <div className="portico-inline-confirm"><span>Cancel the invitation to {recipient}? Its code will stop granting access immediately.</span><button type="button" onClick={() => setConfirmRevokeInvite('')}>Keep invitation</button><button type="button" className="danger" disabled={mutation.busy} onClick={() => void revokeInvite(invite)}>Cancel invitation</button></div> : revocable && <SecondaryButton disabled={mutation.busy} onClick={() => setConfirmRevokeInvite(invite.id)}>Cancel invitation</SecondaryButton>}</div></article>;
      })}
    </div>
    {editor === 'invite' && <PorticoInviteEditor operations={operations} source={source} onDismiss={() => setEditor(null)} onSaved={() => { setEditor(null); onChanged(); }} />}
    {editor && editor !== 'invite' && <UserEditor user={editor === 'new' ? undefined : editor} operations={operations} source={source} onDismiss={() => setEditor(null)} onSaved={() => { setEditor(null); onChanged(); }} />}
  </SettingsGroup>;
}

function DevicesPanel({ devices, source, onChanged }: { devices: Device[]; source: SettingsDataSource; onChanged: () => void }) {
  const [error, setError] = useState('');
  const [confirmRevoke, setConfirmRevoke] = useState('');
  const mutation = useAbortableMutation();
  const trust = async (device: Device, trusted: boolean) => {
    setError('');
    try { await mutation.run((signal) => source.updateDevice(device.id, { trusted }, signal)); onChanged(); }
    catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: `update ${device.name}` })); }
  };
  const revoke = async (device: Device) => {
    setError('');
    try { await mutation.run((signal) => source.revokeDevice(device.id, signal)); setConfirmRevoke(''); onChanged(); }
    catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: `revoke ${device.name}` })); }
  };
  return <SettingsGroup title="Devices" description="Known application installations and trusted-device enforcement.">
    {error && <InlineNotice tone="error">{error}</InlineNotice>}
    {devices.length === 0 ? <div className="portico-settings-state"><DeviceClientIcon /><strong>No known devices</strong><p>Devices appear after an application signs in.</p></div> : <div className="portico-device-list">{devices.map((device) => <article key={device.id}><span className="portico-device-icon"><DeviceClientIcon /></span><span><strong>{device.name || device.autoName}</strong><small>{device.user} · {device.app} on {device.platform} · seen {new Date(device.lastSeenAt).toLocaleString()}</small></span><div><label className="portico-inline-toggle"><span>Trusted</span><ToggleControl label={`Trust ${device.name}`} value={device.trusted} disabled={mutation.busy} onChange={(trusted) => void trust(device, trusted)} /></label>{confirmRevoke === device.id ? <div className="portico-inline-confirm"><span>Sign out {device.name || device.autoName}? This installation will need to sign in again.</span><button type="button" onClick={() => setConfirmRevoke('')}>Cancel</button><button type="button" className="danger" disabled={mutation.busy} onClick={() => void revoke(device)}>Sign out device</button></div> : <IconButton label={`Revoke ${device.name}`} onClick={() => setConfirmRevoke(device.id)}><ActionDeleteIcon /></IconButton>}</div></article>)}</div>}
  </SettingsGroup>;
}

function APIKeyEditor({ scopes, source, initialKey, onDismiss, onSaved }: { scopes: string[]; source: SettingsDataSource; initialKey?: APIKey; onDismiss: () => void; onSaved: (token: string) => void }) {
  const [name, setName] = useState(initialKey?.name ?? '');
  const [selected, setSelected] = useState<string[]>(initialKey?.scopes.filter((scope) => scope !== 'read') ?? []);
  const [error, setError] = useState('');
  const mutation = useAbortableMutation();
  const submit = async () => {
    if (!name.trim()) { setError('Enter a key name.'); return; }
    if (selected.length === 0) { setError('Choose at least one scope.'); return; }
    setError('');
    try { const response = await mutation.run((signal) => source.createAPIKey({ name: name.trim(), scopes: selected }, signal)); onSaved(response.token); }
    catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'create this API key' })); }
  };
  return <ModalOverlay labelledBy="portico-api-key-title" className="portico-settings-dialog portico-key-dialog" onDismiss={onDismiss}><header><div><h2 id="portico-api-key-title">New API key</h2><p>Least-privilege access for an integration</p></div><IconButton label="Close" onClick={onDismiss}><ActionCloseIcon /></IconButton></header><div className="portico-settings-dialog-fields"><label><span>Name</span><TextControl label="API key name" value={name} onChange={setName} placeholder="Home automation" /></label><fieldset><legend>Scopes</legend><div className="portico-settings-checkbox-grid">{scopes.map((scope) => <label key={scope}><input type="checkbox" checked={selected.includes(scope)} onChange={(event) => setSelected((current) => event.target.checked ? [...current, scope] : current.filter((item) => item !== scope))} /><span>{permissionLabel(scope)}</span></label>)}</div></fieldset>{error && <p className="portico-settings-dialog-error" role="alert"><StatusWarningIcon />{error}</p>}</div><footer><SecondaryButton disabled={mutation.busy} onClick={onDismiss}>Cancel</SecondaryButton><PrimaryButton disabled={mutation.busy} onClick={() => void submit()}>{mutation.busy ? 'Creating…' : 'Create API key'}</PrimaryButton></footer></ModalOverlay>;
}

function APIKeysPanel({ keys, scopes, source, onChanged }: { keys: APIKey[]; scopes: string[]; source: SettingsDataSource; onChanged: () => void }) {
  const [editor, setEditor] = useState(false);
  const [token, setToken] = useState('');
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState('');
  const [confirmRevoke, setConfirmRevoke] = useState('');
  const [confirmReplace, setConfirmReplace] = useState('');
  const [replacement, setReplacement] = useState<APIKey>();
  const mutation = useAbortableMutation();
  useEffect(() => {
    setSettingsNavigationDirty(Boolean(token), 'api-key-token');
    return () => setSettingsNavigationDirty(false, 'api-key-token');
  }, [token]);
  useEffect(() => {
    if (!token) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  }, [token]);
  const revoke = async (key: APIKey) => {
    setError('');
    try { await mutation.run((signal) => source.revokeAPIKey(key.id, signal)); setConfirmRevoke(''); onChanged(); }
    catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: `revoke ${key.name}` })); }
  };
  const replace = async (key: APIKey) => {
    setError('');
    try {
      await mutation.run((signal) => source.revokeAPIKey(key.id, signal));
      setConfirmReplace('');
      setReplacement(key);
      setEditor(true);
      onChanged();
    }
    catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: `replace ${key.name}` })); }
  };
  const acknowledgeToken = () => { setToken(''); setCopied(false); onChanged(); };
  const copy = async () => {
    try { await navigator.clipboard.writeText(token); setCopied(true); acknowledgeToken(); }
    catch { setError('Clipboard access was denied. Copy the token manually.'); }
  };
  return <SettingsGroup title="API keys" description="Long-lived credentials for integrations. New tokens are shown once." actions={<PrimaryButton disabled={Boolean(token)} onClick={() => setEditor(true)}><ActionAddIcon /> New key</PrimaryButton>}>
    {error && <InlineNotice tone="error">{error}</InlineNotice>}
    {token && <InlineNotice tone="warn" action={<><SecondaryButton onClick={() => void copy()}>{copied ? <ActionConfirmIcon /> : <ViewListIcon />}{copied ? 'Copied' : 'Copy token'}</SecondaryButton><SecondaryButton onClick={acknowledgeToken}>I saved it</SecondaryButton></>}><span className="portico-api-token"><strong>Store this token now.</strong><code>{token}</code></span></InlineNotice>}
    {keys.length === 0 ? <div className="portico-settings-state"><AccountSecurityIcon /><strong>No API keys</strong><p>Create a scoped key when an integration needs server access.</p></div> : <div className="portico-api-key-list">{keys.map((key) => <article key={key.id}><AccountSecurityIcon /><span><strong>{key.name}</strong><small>•••• {key.lastFour} · created {new Date(key.createdAt).toLocaleDateString()}{key.lastUsedAt ? ` · used ${new Date(key.lastUsedAt).toLocaleDateString()}` : ' · never used'}</small></span>{confirmReplace === key.id ? <div className="portico-inline-confirm"><span>Lost the token for {key.name}? Revoke it and create a replacement with the same scopes.</span><button type="button" onClick={() => setConfirmReplace('')}>Cancel</button><button type="button" className="danger" disabled={mutation.busy} onClick={() => void replace(key)}>Revoke and replace</button></div> : confirmRevoke === key.id ? <div className="portico-inline-confirm"><span>Revoke {key.name}? Integrations using this key will stop working immediately.</span><button type="button" onClick={() => setConfirmRevoke('')}>Cancel</button><button type="button" className="danger" disabled={mutation.busy} onClick={() => void revoke(key)}>Revoke key</button></div> : <div><SecondaryButton disabled={mutation.busy} onClick={() => { setConfirmRevoke(''); setConfirmReplace(key.id); }}>Replace</SecondaryButton><IconButton label={`Revoke ${key.name}`} onClick={() => { setConfirmReplace(''); setConfirmRevoke(key.id); }}><ActionDeleteIcon /></IconButton></div>}</article>)}</div>}
    {editor && <APIKeyEditor scopes={scopes} source={source} initialKey={replacement} onDismiss={() => { setEditor(false); setReplacement(undefined); }} onSaved={(value) => { setToken(value); setCopied(false); setEditor(false); setReplacement(undefined); }} />}
  </SettingsGroup>;
}

export function PeopleOperations({ operations, source, onChanged }: { operations: SettingsOperationalSnapshot; source: SettingsDataSource; onChanged: () => void }) {
  const failures = operations.failures ?? {};
  const scopes = apiKeyScopeCatalog.slice();
  const unavailable = (title: string) => <SettingsGroup title={title} description="This panel could not be refreshed independently."><div className="portico-settings-state error"><StatusWarningIcon /><strong>{title} are unavailable</strong><p>Retry the failed panel before making changes. No empty result is being inferred.</p></div></SettingsGroup>;
  const usersUnavailable = Boolean(failures.users || failures.libraries || failures.capabilities);
  return <div className="portico-settings-form">
    {usersUnavailable ? unavailable('People') : <UsersPanel operations={operations} source={source} onChanged={onChanged} />}
    {failures.devices ? unavailable('Devices') : <DevicesPanel devices={operations.devices} source={source} onChanged={onChanged} />}
    {failures.apiKeys ? unavailable('API keys') : <APIKeysPanel keys={operations.apiKeys} scopes={scopes} source={source} onChanged={onChanged} />}
  </div>;
}
