import { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import { Delete, Lock, Pencil, Plus } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import type { Profile } from '../../data/types';
import { useProfiles } from '../../services/queries';
import { api } from '../../services/client';
import { useSession } from '../../state/session';
import { Avatar, Button, ErrorState, Segmented, Skel, cls } from '../../components/ui';
import { Sheet, toast } from '../../components/overlay';

const COLORS = ['#409fd4', '#f0a02a', '#e25aa8', '#35c28a', '#8a5cf0', '#e0513f', '#c9a86a', '#3fae6b'];

export function Profiles() {
  const profiles = useProfiles();
  const choose = useSession((s) => s.chooseProfile);
  const signedIn = useSession((s) => s.signedIn);
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [params] = useSearchParams();
  const [editing, setEditing] = useState(false);
  const [unlock, setUnlock] = useState<Profile | null>(null);
  const [edit, setEdit] = useState<Profile | 'new' | null>(null);

  useEffect(() => {
    if (!signedIn) navigate('/welcome', { replace: true });
  }, [signedIn, navigate]);

  useEffect(() => {
    const id = params.get('unlock');
    const p = profiles.data?.find((x) => x.id === id);
    if (p) setUnlock(p);
  }, [params, profiles.data]);

  const enter = (p: Profile) => {
    if (editing) return setEdit(p);
    if (p.hasPin) return setUnlock(p);
    go(p);
  };
  const go = (p: Profile) => {
    choose(p.id);
    qc.clear();
    navigate('/', { replace: true, viewTransition: true });
  };

  return (
    <div className="auth profiles">
      <div className="auth-center">
        <div className="profiles-inner">
          <img className="profiles-mark" src="/brand/portico-wordmark-mono-white.svg" alt="Portico" />
          <h1 className="profiles-title">{editing ? 'Manage Profiles' : 'Who’s watching?'}</h1>
          {profiles.isError && <ErrorState error={profiles.error} retry={() => profiles.refetch()} />}
          <div className="profile-grid">
            {profiles.isLoading &&
              Array.from({ length: 4 }, (_, i) => (
                <div key={i} className="profile-tile">
                  <Skel w={132} h={132} r={66} />
                  <Skel w={70} h={14} />
                </div>
              ))}
            {profiles.data?.map((p, i) => (
              <button key={p.id} className={cls('profile-tile', editing && 'editing')} onClick={() => enter(p)} style={{ animationDelay: `${i * 60}ms` }}>
                <span className="profile-ring">
                  <Avatar name={p.name} color={p.avatarColor} size={132} />
                  {editing && (
                    <span className="profile-edit">
                      <Pencil />
                    </span>
                  )}
                </span>
                <span className="profile-name">
                  {p.name}
                  {p.hasPin && !editing && <Lock className="profile-lock" aria-label="PIN protected" />}
                </span>
                {p.role === 'kid' && <span className="profile-kid">Kids</span>}
              </button>
            ))}
            {profiles.data && (
              <button className="profile-tile add" onClick={() => setEdit('new')}>
                <span className="profile-ring">
                  <span className="profile-add">
                    <Plus />
                  </span>
                </span>
                <span className="profile-name">Add Profile</span>
              </button>
            )}
          </div>
          <Button variant="glass" size="lg" onClick={() => setEditing((e) => !e)}>
            {editing ? 'Done' : 'Manage Profiles'}
          </Button>
        </div>
      </div>
      {unlock && <PinPad profile={unlock} onClose={() => setUnlock(null)} onUnlocked={() => go(unlock)} />}
      {edit && <ProfileEditor profile={edit === 'new' ? null : edit} onClose={() => setEdit(null)} />}
    </div>
  );
}

function PinPad({ profile, onClose, onUnlocked }: { profile: Profile; onClose: () => void; onUnlocked: () => void }) {
  const [pin, setPin] = useState('');
  const [shake, setShake] = useState(false);
  const [help, setHelp] = useState(false);
  const press = async (d: string) => {
    if (pin.length >= 4) return;
    const next = pin + d;
    setPin(next);
    if (next.length === 4) {
      const ok = await api.unlockProfile(profile.id, next);
      if (ok) onUnlocked();
      else {
        setShake(true);
        setTimeout(() => {
          setShake(false);
          setPin('');
        }, 500);
      }
    }
  };
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (/^\d$/.test(e.key)) press(e.key);
      if (e.key === 'Backspace') setPin((p) => p.slice(0, -1));
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });
  return (
    <Sheet open onClose={onClose} size="sm" compact>
      <div className="pin">
        <Avatar name={profile.name} color={profile.avatarColor} size={72} />
        <div className="pin-title">Enter PIN for {profile.name}</div>
        <div className={cls('pin-dots', shake && 'shake')} aria-live="polite" aria-label={`${pin.length} of 4 digits entered`}>
          {[0, 1, 2, 3].map((i) => (
            <i key={i} className={cls(i < pin.length && 'on')} />
          ))}
        </div>
        <div className="pin-pad">
          {['1', '2', '3', '4', '5', '6', '7', '8', '9'].map((d) => (
            <button key={d} className="pin-key" onClick={() => press(d)}>
              {d}
            </button>
          ))}
          <span />
          <button className="pin-key" onClick={() => press('0')}>
            0
          </button>
          <button className="pin-key fn" onClick={() => setPin((p) => p.slice(0, -1))} aria-label="Delete">
            <Delete />
          </button>
        </div>
        <button className="linkish" onClick={() => setHelp((h) => !h)}>
          Forgot PIN?
        </button>
        {help && <p className="pin-help">An owner can reset it in Settings → Profiles on any signed-in device, or sign in again with the account password. (Fixture household PIN: 1234.)</p>}
      </div>
    </Sheet>
  );
}

function ProfileEditor({ profile, onClose }: { profile: Profile | null; onClose: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState(profile?.name ?? '');
  const [color, setColor] = useState(profile?.avatarColor ?? COLORS[4]);
  const [kind, setKind] = useState<'member' | 'kid'>(profile?.role === 'kid' ? 'kid' : 'member');
  const [busy, setBusy] = useState(false);
  const save = async () => {
    setBusy(true);
    if (profile) await api.updateProfile(profile.id, { name, avatarColor: color, initials: name[0] });
    else await api.createProfile({ name, avatarColor: color, role: kind });
    await qc.invalidateQueries({ queryKey: ['profiles'] });
    setBusy(false);
    toast(profile ? `Saved ${name}` : `Created ${name}’s profile`);
    onClose();
  };
  return (
    <Sheet
      open
      onClose={onClose}
      title={profile ? `Edit ${profile.name}` : 'New Profile'}
      size="sm"
      footer={
        <>
          <Button variant="plain" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="accent" onClick={save} disabled={!name.trim()} loading={busy}>
            {profile ? 'Save' : 'Create Profile'}
          </Button>
        </>
      }
    >
      <div className="editor">
        <div className="editor-avatar">
          <Avatar name={name || '?'} color={color} size={96} />
        </div>
        <div className="swatches" role="radiogroup" aria-label="Color">
          {COLORS.map((c) => (
            <button key={c} role="radio" aria-checked={c === color} className={cls('swatch', c === color && 'on')} style={{ background: c }} onClick={() => setColor(c)} aria-label={c} />
          ))}
        </div>
        <div className="field">
          <label htmlFor="pname">Name</label>
          <input id="pname" className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Name" autoFocus />
        </div>
        {!profile && (
          <div className="field">
            <span className="field-label">Profile type</span>
            <Segmented
              block
              value={kind}
              onChange={setKind}
              options={[
                { value: 'member', label: 'Standard' },
                { value: 'kid', label: 'Kids' },
              ]}
            />
            <span className="field-hint">{kind === 'kid' ? 'Kids profiles only see G and TV-Y7 titles, have a bedtime, and can’t reach settings.' : 'Standard profiles see everything the server owner has shared with them.'}</span>
          </div>
        )}
        {profile && (
          <p className="field-hint">
            Ratings limits, PIN and bedtime live in <strong>Settings → Profiles & Parental Controls</strong>.
          </p>
        )}
      </div>
    </Sheet>
  );
}
