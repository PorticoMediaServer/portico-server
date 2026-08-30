import { ActionConfirmIcon, StatusLockedIcon, AccountSignOutIcon, ActionCloseIcon } from '#portico-icons';
import { type ServerManagedProfileDirectory } from '@porticomediaserver/client-core';
import { useCallback, useEffect, useRef, useState } from 'react';
import { IconButton } from '../../components/controls/Buttons';
import { PasswordInput } from '../../components/controls/PasswordInput';
import { ModalOverlay } from '../../components/overlay/OverlayPortal';
import { productProblemText, productText } from '../../components/ProductLanguage';
import { useAuthSession, usePorticoDataSource } from '../../data/DataProvider';
import './profile-switcher.css';

function initials(name: string) {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  return (parts.length > 1 ? `${parts[0]?.[0] ?? ''}${parts.at(-1)?.[0] ?? ''}` : parts[0]?.slice(0, 2) || 'P').toUpperCase();
}

function avatarSource(profile: ServerManagedProfileDirectory['profiles'][number]) {
  const reference = profile.avatar?.reference?.trim();
  if (!reference || profile.avatar?.kind !== 'custom') return undefined;
  return reference.startsWith('https://') || reference.startsWith('/api/') ? reference : undefined;
}

export function ProfileSwitcherDialog({ onDismiss, required = false, onSignOut }: { onDismiss: () => void; required?: boolean; onSignOut?: () => void }) {
  const auth = useAuthSession();
  const source = usePorticoDataSource();
  const [directory, setDirectory] = useState<ServerManagedProfileDirectory>();
  const [selectedId, setSelectedId] = useState<string>();
  const [pin, setPin] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const loadController = useRef<AbortController | undefined>(undefined);

  const load = useCallback(async () => {
    loadController.current?.abort();
    const controller = new AbortController();
    loadController.current = controller;
    setError('');
    for (let attempt = 0; attempt < 2; attempt += 1) {
      try {
        const next = await source.accountProfiles(controller.signal);
        if (!controller.signal.aborted) setDirectory(next);
        return;
      } catch (reason) {
        if (controller.signal.aborted) return;
        if (attempt === 0) {
          await new Promise((resolve) => window.setTimeout(resolve, 250));
          continue;
        }
        // A retained directory remains usable during a transient refresh
        // failure. Only an initial, repeatedly failed load becomes visible.
        setDirectory((current) => {
          if (!current) setError(productProblemText(reason));
          return current;
        });
      }
    }
  }, [source]);

  useEffect(() => {
    void load();
    return () => loadController.current?.abort();
  }, [load]);
  const selected = directory?.profiles.find((profile) => profile.id === selectedId);

  const choose = async (profileId: string, profilePin?: string) => {
    const profile = directory?.profiles.find((candidate) => candidate.id === profileId);
    if (!profile) return;
    if (profile.hasPIN && profilePin === undefined) {
      setSelectedId(profile.id);
      setPin('');
      setError('');
      return;
    }
    setBusy(true);
    setError('');
    try {
      if (directory?.authority === 'hosted') {
        await auth.switchHostedProfile({ profileId, pin: profilePin });
      } else {
		await auth.switchAuthenticatedLocalProfile(profileId, profilePin);
      }
      onDismiss();
    } catch (reason) {
      setError(productProblemText(reason));
    } finally {
      setBusy(false);
    }
  };

  return <ModalOverlay labelledBy="profile-switcher-title" className={`profile-switcher-dialog ${required ? 'required' : ''}`} onDismiss={busy || required ? () => undefined : onDismiss}>
    <header>
      <div><p>Profiles</p><h2 id="profile-switcher-title">Who’s watching?</h2></div>
      {required
        ? <IconButton label="Sign out" disabled={busy} onClick={onSignOut}><AccountSignOutIcon /></IconButton>
        : <IconButton label={productText('action.close')} disabled={busy} onClick={onDismiss}><ActionCloseIcon /></IconButton>}
    </header>
    {!directory && !error ? <div className="profile-switcher-loading" aria-busy="true">Loading profiles…</div> : null}
    {directory ? <div className="profile-switcher-grid" role="list" aria-label="Available profiles">
      {directory.profiles.map((profile) => {
        const current = profile.id === auth.viewer?.viewerScope?.profileId;
        const image = avatarSource(profile);
        return <button key={profile.id} type="button" role="listitem" disabled={busy} className={`${selectedId === profile.id ? 'selected' : ''} ${current ? 'current' : ''}`} onClick={() => void choose(profile.id)}>
          <span className="profile-switcher-avatar">{image ? <img src={image} alt="" /> : initials(profile.name)}{profile.hasPIN ? <span className="profile-switcher-lock"><StatusLockedIcon /></span> : null}{current ? <span className="profile-switcher-current"><ActionConfirmIcon /></span> : null}</span>
          <strong>{profile.name}</strong>
          <small>{current ? 'Current profile' : profile.hasPIN ? 'PIN required' : 'Open profile'}</small>
        </button>;
      })}
    </div> : null}
    {selected?.hasPIN ? <form className="profile-switcher-pin" onSubmit={(event) => { event.preventDefault(); if (pin.length === 4) void choose(selected.id, pin); }}>
      <div><strong>Enter PIN for {selected.name}</strong><small>Use this profile’s four-digit PIN.</small></div>
      <PasswordInput autoFocus aria-label={`PIN for ${selected.name}`} autoComplete="one-time-code" inputMode="numeric" pattern="[0-9]{4}" maxLength={4} value={pin} onChange={(event) => setPin(event.target.value.replace(/\D/g, '').slice(0, 4))} />
      <button className="button primary" type="submit" disabled={busy || pin.length !== 4}>{busy ? 'Opening…' : 'Open profile'}</button>
    </form> : null}
    {error ? <div className="profile-switcher-error" role="alert"><span>{error}</span><button type="button" className="button secondary" onClick={() => void load()}>Try again</button></div> : null}
  </ModalOverlay>;
}
