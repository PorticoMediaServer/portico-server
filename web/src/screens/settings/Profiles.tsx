import React, {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import {AVATAR_MAX_BYTES, IdentityClient, type PINRecoveryMethods, type ProfileAvatar} from '@core/index.ts';
import {ManagedProfilesClient, PROFILE_ART, parseDirectSnapshot, type DirectSnapshot, type ManagedProfile, type ProfileArt, type ProfileDeletion} from '@core/profile-management.ts';
import {profileActions} from '@core/presentation/index.ts';
import {currentAccessToken, useSession} from '../../app/session';
import {ErrorNotice, errorText} from '../../app/errors';
import {Button, ConfirmDialog, Dialog, Input, Loading, Menu, Notice, PasswordInput, SettingsGroup, SettingsRow, Spinner, Text, type MenuItem, PasswordStrength} from '../../ui';
import {Avatar} from '../../ui';
import {passwordStrength} from '@core/password-strength.ts';

const t = defaultI18n.t;

export function useIdentity() {
  const {api, session} = useSession();
  const token = useRef('');
  token.current = session?.accessToken ?? '';
  return useMemo(() => new IdentityClient(api, async (path, body, contentType) => {
    const response = await api.routeFetch(api.baseUrl + path, {method: 'POST', headers: {Authorization: 'Bearer ' + token.current, 'Content-Type': contentType}, body: body as BodyInit});
    const value = await response.json().catch(() => null);
    if (!response.ok) throw Object.assign(new Error('upload failed'), {status: response.status, code: value?.error?.code});
    return value;
  }), [api]);
}

/** A profile picture is personal data and needs the viewer's sign-in to read, which an image
 * tag cannot send; so it is fetched and shown from memory. */
function usePictures(avatars: readonly ProfileAvatar[]) {
  const {api} = useSession();
  const [urls, setUrls] = useState<Record<string, string>>({});
  const made = useRef(new Map<string, string>());
  const key = avatars.map(a => a.profileId + ':' + a.version).join(',');
  useEffect(() => {
    let live = true;
    for (const a of avatars) {
      const id = a.profileId + ':' + a.version;
      if (made.current.has(id)) continue;
      made.current.set(id, '');
      void api.routeFetch(api.baseUrl + a.url + (a.url.includes('?') ? '&' : '?') + 'size=160', {headers: {Authorization: 'Bearer ' + currentAccessToken()}}).then(r => (r.ok ? r.blob() : Promise.reject())).then(blob => {
        const url = URL.createObjectURL(blob); made.current.set(id, url);
        if (live) setUrls(prev => ({...prev, [a.profileId]: url}));
      }, () => { made.current.delete(id); });
    }
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api, key]);
  useEffect(() => () => { made.current.forEach(url => url && URL.revokeObjectURL(url)); }, []);
  return Object.fromEntries(avatars.map(a => [a.profileId, made.current.get(a.profileId + ':' + a.version) || urls[a.profileId] || ''])) as Record<string, string>;
}

function Picture({src, name, art}: {src?: string; name: string; art?: string}) {
  return <Avatar src={src} name={name} art={art} size={32} />;
}

type Editing = {kind: 'add'} | {kind: 'rename'; profile: ManagedProfile} | {kind: 'pin'; profile: ManagedProfile};

/**
 * Profiles on a direct server account (WEB-SET-01, D-ID-4). The main profile can add, rename,
 * reorder, protect with a PIN and delete profiles, and change pictures; every profile can reach
 * its own limits and recover a forgotten PIN. Other profiles see the list and a note.
 */
export function LocalProfiles({onLimits}: {onLimits: (profile: {id: string; name: string}) => void}) {
  const session = useSession();
  const {api} = session;
  const viewer = session.session!.viewer;
  const identity = useIdentity();
  const client = useMemo(() => new ManagedProfilesClient(api, {authority: 'local', accountId: viewer.accountId, serverId: viewer.serverId}), [api, viewer.accountId, viewer.serverId]);
  const [snapshot, setSnapshot] = useState<DirectSnapshot>();
  const [loadError, setLoadError] = useState<unknown>();
  const [avatars, setAvatars] = useState<readonly ProfileAvatar[]>([]);
  const [problem, setProblem] = useState('');
  const [busy, setBusy] = useState('');
  const [editing, setEditing] = useState<Editing>();
  const [removingPin, setRemovingPin] = useState<ManagedProfile>();
  const [deleting, setDeleting] = useState<{profile: ManagedProfile; preview?: ProfileDeletion}>();
  const [recovering, setRecovering] = useState<{id: string; name: string}>();
  const [notice, setNotice] = useState('');
  const picker = useRef<HTMLInputElement>(null);
  const target = useRef('');
  const pictures = usePictures(avatars);
  const loadAvatars = useCallback(() => identity.avatars().then(setAvatars, () => {}), [identity]);
  const load = useCallback(async () => {
    setLoadError(undefined);
    try { setSnapshot(parseDirectSnapshot(await api.request<unknown>('/v1/direct'), {authority: 'local', accountId: viewer.accountId, serverId: viewer.serverId})); } catch (e) { setLoadError(e); }
  }, [api, viewer.accountId, viewer.serverId]);
  useEffect(() => { void load(); void loadAvatars(); }, [load, loadAvatars]);
  /** After any change: this list, and the session's copy used by the profile chooser. */
  const changed = useCallback(async (message?: string) => { await load(); session.direct.reloadProfiles(); if (message) setNotice(message); }, [load, session.direct]);
  const run = async (profileId: string, work: () => Promise<unknown>, message?: string) => {
    setBusy(profileId); setProblem(''); setNotice('');
    try { await work(); await changed(message); return true; } catch (e) { setProblem(errorText(e, 'profiles', 'save')); return false; } finally { setBusy(''); }
  };
  const choosePicture = (profileId: string) => { target.current = profileId; setProblem(''); picker.current?.click(); };
  const picked = async (file?: File) => {
    const profileId = target.current;
    if (!file || !profileId) return;
    if (file.size > AVATAR_MAX_BYTES) { setProblem(t('web.profile.pictureTooLarge')); return; }
    if (!/^image\/(jpeg|png|webp)$/.test(file.type)) { setProblem(t('profile.pictureRejected')); return; }
    setBusy(profileId);
    try { await identity.uploadAvatar(profileId, file, file.type); await loadAvatars(); } catch (e) { setProblem(errorText(e, 'profiles', 'save')); } finally { setBusy(''); }
  };
  const removePicture = async (profileId: string) => {
    setBusy(profileId); setProblem('');
    try { await identity.removeAvatar(profileId); await loadAvatars(); } catch (e) { setProblem(errorText(e, 'profiles', 'save')); } finally { setBusy(''); }
  };
  const move = (profile: ManagedProfile, delta: number) => {
    if (!snapshot) return;
    const list = [...snapshot.profiles].sort((a, b) => a.position - b.position);
    const i = list.findIndex(p => p.id === profile.id), j = i + delta;
    if (i < 0 || j < 0 || j >= list.length) return;
    [list[i], list[j]] = [list[j]!, list[i]!];
    void run(profile.id, () => client.order(list));
  };
  const startDelete = async (profile: ManagedProfile) => {
    setDeleting({profile});
    try { const preview = await client.previewDelete(profile); setDeleting({profile, preview}); } catch (e) { setDeleting(undefined); setProblem(errorText(e, 'profiles', 'action')); }
  };

  if (loadError && !snapshot) return <SettingsGroup title={t('profiles.title')}><div style={{padding: '0 16px 12px'}}><ErrorNotice error={loadError} context="profiles" retry={() => void load()} compact /></div></SettingsGroup>;
  if (!snapshot) return <SettingsGroup title={t('profiles.title')}><div style={{padding: 16}}><Loading label={t('status.loadingThing', {thing: t('profiles.title').toLowerCase()})} /></div></SettingsGroup>;
  const manage = snapshot.canManage;
  const profiles = [...snapshot.profiles].sort((a, b) => a.position - b.position);
  const current = viewer.profileId;
  return (
    <SettingsGroup title={t('profiles.title')} description={manage ? t('web.account.profilesHelp') : t('account.onlyMainProfileManages')}
      action={manage && profiles.length < 8 ? <Button size="sm" variant="secondary" icon="plus" label={t('profile.add')} onClick={() => { setProblem(''); setEditing({kind: 'add'}); }} /> : undefined}>
      {problem ? <div style={{padding: '0 16px 8px'}}><Notice tone="error" compact>{problem}</Notice></div> : null}
      {notice ? <div style={{padding: '0 16px 8px'}}><Notice tone="success" compact>{notice}</Notice></div> : null}
      <input ref={picker} type="file" accept="image/jpeg,image/png,image/webp" hidden onChange={e => { const file = e.target.files?.[0]; e.target.value = ''; void picked(file); }} />
      {profiles.map((p, index) => {
        const has = avatars.some(a => a.profileId === p.id);
        // CON-12: one shared model decides the profile menu order. Forget remembered
        // devices has no handler here, so it is hidden (listed under Questions).
        const model = profileActions({hasPicture: has, pinRequired: p.pinRequired, primary: p.primary}, {canManage: manage, isCurrent: p.id === current});
        const items: MenuItem[] = [];
        for (const action of model) {
          if (action.id === 'forgetDevices') continue;
          if (action.id === 'changePin' || action.id === 'forgotPin') {
            items.push({id: action.id, label: t(action.label), icon: action.icon, ...(action.destructive ? {destructive: true} : {}), separatorBefore: true});
          } else {
            items.push({id: action.id, label: t(action.label), icon: action.icon, ...(action.destructive ? {destructive: true} : {})});
          }
        }
        if (manage && p.pinRequired) items.push({id: 'removePin', label: t('profile.removePin'), icon: 'lock'});
        if (manage && index > 0) items.push({id: 'up', label: t('web.profile.moveUp'), icon: 'chevronUp', separatorBefore: true});
        if (manage && index < profiles.length - 1) items.push({id: 'down', label: t('web.profile.moveDown'), icon: 'chevronDown', separatorBefore: index === 0});
        // Delete is destructive last; ensure separator before it.
        const deleteIdx = items.findIndex(i => i.id === 'delete');
        if (deleteIdx >= 0) items[deleteIdx] = {...items[deleteIdx]!, separatorBefore: true};
        const meta = busy === p.id ? <Spinner /> : p.id === current ? t('profile.current') : p.primary ? t('profile.main') : p.pinRequired ? t('profile.protected') : undefined;
        return (
          <SettingsRow key={p.id} label={<span style={{display: 'inline-flex', alignItems: 'center', gap: 12}}><Picture src={pictures[p.id]} name={p.name} art={p.art} />{p.name}</span>} meta={meta}
            control={<Menu label={t('web.profile.options', {name: p.name})} trigger={<Button size="sm" variant="ghost" icon="more" aria-label={t('web.profile.options', {name: p.name})} />} items={items} onSelect={id => {
              setProblem(''); setNotice('');
              if (id === 'rename') setEditing({kind: 'rename', profile: p});
              else if (id === 'picture') choosePicture(p.id);
              else if (id === 'removePicture') void removePicture(p.id);
              else if (id === 'changePin') setEditing({kind: 'pin', profile: p});
              else if (id === 'removePin') setRemovingPin(p);
              else if (id === 'forgotPin') setRecovering({id: p.id, name: p.name});
              else if (id === 'limits') onLimits({id: p.id, name: p.name});
              else if (id === 'up') move(p, -1);
              else if (id === 'down') move(p, 1);
              else if (id === 'delete') void startDelete(p);
            }} />} />
        );
      })}
      {editing?.kind === 'add' || editing?.kind === 'rename' ? <ProfileForm editing={editing} onClose={() => setEditing(undefined)} save={async (name, art) => {
        if (editing.kind === 'add') await client.create(name, art);
        else await client.edit(editing.profile, {name, art});
        await changed(editing.kind === 'add' ? t('profile.added', {name}) : undefined);
      }} /> : null}
      {editing?.kind === 'pin' ? <PinForm profile={editing.profile} onClose={() => setEditing(undefined)} save={async pin => { await client.edit(editing.profile, {pin}); await changed(t('profile.pinSaved', {name: editing.profile.name})); }} /> : null}
      <ConfirmDialog open={!!removingPin} onOpenChange={o => !o && setRemovingPin(undefined)} title={t('profile.removePinConfirm.title', {name: removingPin?.name ?? ''})} body={t('profile.removePinConfirm.body', {name: removingPin?.name ?? ''})} confirmLabel={t('profile.removePin')} cancelLabel={t('action.cancel')} busy={!!removingPin && busy === removingPin.id}
        onConfirm={async () => { const p = removingPin!; if (await run(p.id, () => client.edit(p, {pin: ''}), t('profile.pinRemoved', {name: p.name}))) setRemovingPin(undefined); }} />
      <ConfirmDialog open={!!deleting} onOpenChange={o => !o && setDeleting(undefined)} title={t('profile.deleteConfirm.title', {name: deleting?.profile.name ?? ''})}
        body={deleting?.preview ? <>{t('profile.deleteConfirm.body')}{deleting.preview.playlists ? ` ${t('profile.deleteConfirm.playlists', {count: deleting.preview.playlists})}` : ''}</> : <Loading label={t('web.profile.checking')} />}
        confirmLabel={t('profile.deleteConfirm.action')} cancelLabel={t('action.cancel')} busy={!!deleting && (busy === deleting.profile.id || !deleting.preview)}
        onConfirm={async () => { const d = deleting!; if (!d.preview) return; if (await run(d.profile.id, () => client.delete(d.preview!), t('profile.deleted', {name: d.profile.name}))) setDeleting(undefined); }} />
      {recovering ? <PINRecoveryDialog profile={recovering} identity={identity} onClose={() => setRecovering(undefined)} /> : null}
    </SettingsGroup>
  );
}

/** Add or rename: a `form` dialog with Cancel and one primary. */
function ProfileForm({editing, onClose, save}: {editing: {kind: 'add'} | {kind: 'rename'; profile: ManagedProfile}; onClose: () => void; save: (name: string, art: ProfileArt) => Promise<void>}) {
  const [name, setName] = useState(editing.kind === 'rename' ? editing.profile.name : '');
  const [art, setArt] = useState<ProfileArt>(editing.kind === 'rename' ? editing.profile.art : 'blue');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const submit = async () => {
    if (!name.trim()) { setError(t('web.profile.nameRequired')); return; }
    setBusy(true); setError('');
    try { await save(name.trim(), art); onClose(); } catch (e) { setError(errorText(e, 'profiles', 'save')); } finally { setBusy(false); }
  };
  const title = editing.kind === 'add' ? t('profile.addTitle') : t('profile.renameTitle', {name: editing.profile.name});
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={title} width={440} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={onClose} /><Button variant="primary" label={t('action.save')} loading={busy} onClick={() => void submit()} /></>}>
      <form onSubmit={e => { e.preventDefault(); void submit(); }} style={{display: 'flex', flexDirection: 'column', gap: 16}}>
        {error ? <Notice tone="error" compact>{error}</Notice> : null}
        <Input label={t('profile.name')} value={name} maxLength={120} autoFocus onChange={e => setName(e.target.value)} />
        <div role="radiogroup" aria-label={t('web.profile.color')}>
          <Text as="div" variant="label" tone="secondary" style={{marginBottom: 8}}>{t('web.profile.color')}</Text>
          <div style={{display: 'flex', gap: 8, flexWrap: 'wrap'}}>
            {PROFILE_ART.map(a => <button key={a} type="button" role="radio" aria-checked={art === a} aria-label={t('web.profile.colorName', {color: a})} onClick={() => setArt(a)} style={{width: 32, height: 32, borderRadius: 16, background: `var(--profile-art-${a})`, border: art === a ? '2px solid var(--color-text)' : '2px solid transparent', cursor: 'pointer'}} />)}
          </div>
        </div>
      </form>
    </Dialog>
  );
}

function PinForm({profile, onClose, save}: {profile: ManagedProfile; onClose: () => void; save: (pin: string) => Promise<void>}) {
  const [pin, setPin] = useState('');
  const [again, setAgain] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const ready = /^\d{4}$/.test(pin) && /^\d{4}$/.test(again);
  const submit = async () => {
    if (!ready) return;
    if (pin !== again) { setError(t('profile.pinMismatch')); return; }
    setBusy(true); setError('');
    try { await save(pin); onClose(); } catch (e) { setError(errorText(e, 'profiles', 'save')); } finally { setBusy(false); }
  };
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={t('profile.pinTitle', {name: profile.name})} description={t('profile.pinChoose')} width={400} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={onClose} /><Button variant="primary" label={t('action.save')} loading={busy} disabled={!ready} onClick={() => void submit()} /></>}>
      <form onSubmit={e => { e.preventDefault(); void submit(); }} style={{display: 'flex', flexDirection: 'column', gap: 16}}>
        {error ? <Notice tone="error" compact>{error}</Notice> : null}
        <Input label={t('web.profile.newPin')} help={t('web.profile.pinDigits')} inputMode="numeric" autoComplete="off" value={pin} maxLength={4} autoFocus onChange={e => setPin(e.target.value.replace(/\D/g, ''))} />
        <Input label={t('web.profile.repeatPin')} inputMode="numeric" autoComplete="off" value={again} maxLength={4} onChange={e => setAgain(e.target.value.replace(/\D/g, ''))} />
      </form>
    </Dialog>
  );
}

/** Change password on a direct account (WEB-SET-01). */
export function PasswordGroup() {
  const identity = useIdentity();
  const {owner} = useSession();
  const [open, setOpen] = useState(false);
  const [done, setDone] = useState(false);
  return (
    <SettingsGroup title={t('account.security')} description={owner ? undefined : t('web.account.ownerResetsPassword')}>
      <SettingsRow icon="lock" label={t('web.account.password')} help={t('web.account.passwordHelp')} control={<Button size="sm" variant="secondary" label={t('account.changePassword')} onClick={() => { setDone(false); setOpen(true); }} />} />
      {done ? <div style={{padding: '0 16px 12px'}}><Notice tone="success" compact title={t('auth.passwordChanged')}>{t('web.account.passwordChangedBody')}</Notice></div> : null}
      {open ? <PasswordDialog identity={identity} onClose={() => setOpen(false)} onDone={() => { setOpen(false); setDone(true); }} /> : null}
    </SettingsGroup>
  );
}

const strong = (value: string) => passwordStrength(value).acceptable;

function PasswordDialog({identity, onClose, onDone}: {identity: IdentityClient; onClose: () => void; onDone: () => void}) {
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [again, setAgain] = useState('');
  const [code, setCode] = useState('');
  const [twoStep, setTwoStep] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  // C45: with two-step verification on, the change carries a current code in the same request.
  useEffect(() => { identity.twoFactor().then(s => setTwoStep(s.enabled), () => {}); }, [identity]);
  const submit = async () => {
    if (!current || !next || (twoStep && code.trim().length < 6)) return;
    if (!strong(next)) { setError(passwordStrength(next).hint ?? t('web.password.hint')); return; }
    if (next !== again) { setError(t('account.passwordMismatch')); return; }
    setBusy(true); setError('');
    try { await identity.changePassword(current, next, twoStep ? code.trim() : undefined); onDone(); } catch (e) {
      const status = (e as {status?: number})?.status;
      setError(status === 401 || status === 403 ? t('account.wrongPassword') : errorText(e, 'account', 'save'));
    } finally { setBusy(false); }
  };
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={t('account.changePasswordTitle')} width={440} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={onClose} /><Button variant="primary" label={t('action.save')} loading={busy} disabled={!current || !next || !again || (twoStep && code.trim().length < 6)} onClick={() => void submit()} /></>}>
      <form onSubmit={e => { e.preventDefault(); void submit(); }} style={{display: 'flex', flexDirection: 'column', gap: 16}}>
        {error ? <Notice tone="error" compact>{error}</Notice> : null}
        <PasswordInput label={t('account.currentPassword')} autoComplete="current-password" value={current} autoFocus onChange={e => setCurrent(e.target.value)} />
        <PasswordInput label={t('auth.newPassword')} autoComplete="new-password" aria-describedby="change-password-strength" value={next} onChange={e => setNext(e.target.value)} />
        <PasswordStrength id="change-password-strength" password={next} />
        <PasswordInput label={t('account.confirmPassword')} autoComplete="new-password" value={again} onChange={e => setAgain(e.target.value)} />
        {twoStep ? <Input label={t('security.authenticatorCode')} inputMode="numeric" autoComplete="one-time-code" value={code} maxLength={40} onChange={e => setCode(e.target.value)} /> : null}
      </form>
    </Dialog>
  );
}

/** C45: the signed-in account session resets a profile PIN (no password or code prompt; those
 * are asked only in the email, password and two-step forms). */
function PINRecoveryDialog({profile, identity, onClose}: {profile: {id: string; name: string}; identity: IdentityClient; onClose: () => void}) {
  const [methods, setMethods] = useState<PINRecoveryMethods>();
  const [pin, setPin] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [done, setDone] = useState(false);
  useEffect(() => { identity.pinRecovery().then(setMethods, e => setError(errorText(e, 'profiles', 'load'))); }, [identity]);
  const possible = !!methods;
  const ready = /^\d{4}$/.test(pin);
  const reset = async () => {
    setBusy(true); setError('');
    try {
      await identity.resetPIN(profile.id, pin);
      setDone(true);
    } catch (e) {
      const status = (e as {status?: number})?.status;
      setError(status === 401 || status === 403 ? t('account.wrongPassword') : errorText(e, 'profiles', 'save'));
    } finally { setBusy(false); }
  };
  if (done) return <Dialog open onOpenChange={o => !o && onClose()} title={t('profile.pinChanged')} width={420} actions={<Button variant="primary" label={t('action.done')} onClick={onClose} />}><Text variant="body" tone="secondary">{t('profile.pinSavedBody', {name: profile.name})}</Text></Dialog>;
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={t('profile.pinRecoveryTitle', {name: profile.name})} width={440} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={onClose} />{possible ? <Button variant="primary" label={t('profile.pinRecoveryAction')} loading={busy} disabled={!ready} onClick={() => void reset()} /> : null}</>}>
      <form onSubmit={e => { e.preventDefault(); if (ready && possible) void reset(); }} style={{display: 'flex', flexDirection: 'column', gap: 12}}>
        {error ? <Notice tone="error" compact>{error}</Notice> : null}
        {!methods && !error ? <Loading label={t('web.profile.checking')} /> : null}
        {methods && !possible ? <Notice tone="info">{t('web.profile.pinRecoveryUnavailableWeb')}</Notice> : null}
        {possible ? (
          <>
            <Input label={t('web.profile.newPin')} autoFocus help={t('web.profile.pinDigits')} inputMode="numeric" autoComplete="off" value={pin} maxLength={4} onChange={e => setPin(e.target.value.replace(/\D/g, ''))} />
          </>
        ) : null}
      </form>
    </Dialog>
  );
}
