import { useNavigate } from 'react-router';
import { Keyboard, LogOut, Server, Settings, Shield, UserRoundCog, Users } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import { useProfiles } from '../services/queries';
import { useSession } from '../state/session';
import { useUi } from '../state/ui';
import { Avatar } from '../components/ui';
import { Popover, Sheet, useMenu } from '../components/overlay';
import { useIsMobile } from '../lib/hooks';
import { usePlayer } from '../state/player';

export function useCurrentProfile() {
  const id = useSession((s) => s.profileId);
  const profiles = useProfiles();
  return profiles.data?.find((p) => p.id === id);
}

export function ProfileButton({ size = 40 }: { size?: number }) {
  const me = useCurrentProfile();
  const menu = useMenu();
  const mobile = useIsMobile();
  if (!me) return <span className="avatar skel" style={{ ['--s' as string]: `${size}px` }} />;
  const body = <ProfileMenuBody onClose={menu.close} />;
  return (
    <>
      <button className="profile-btn" onClick={menu.open} aria-label={`Profile: ${me.name}`} aria-haspopup="dialog" aria-expanded={menu.isOpen}>
        <Avatar name={me.name} color={me.avatarColor} size={size} />
      </button>
      {mobile ? (
        <Sheet open={menu.isOpen} onClose={menu.close} title="Account" compact>
          {body}
        </Sheet>
      ) : (
        <Popover anchor={menu.anchor} open={menu.isOpen} onClose={menu.close} width={300} label="Account">
          {body}
        </Popover>
      )}
    </>
  );
}

function ProfileMenuBody({ onClose }: { onClose: () => void }) {
  const me = useCurrentProfile();
  const profiles = useProfiles();
  const choose = useSession((s) => s.chooseProfile);
  const switchProfile = useSession((s) => s.switchProfile);
  const signOut = useSession((s) => s.signOut);
  const setShortcuts = useUi((s) => s.setShortcuts);
  const stop = usePlayer((s) => s.stop);
  const navigate = useNavigate();
  const qc = useQueryClient();
  if (!me) return null;
  const admin = me.role === 'owner' || me.role === 'admin';
  const go = (to: string) => {
    onClose();
    navigate(to, { viewTransition: true });
  };
  const others = (profiles.data ?? []).filter((p) => p.id !== me.id);
  return (
    <div className="profile-menu">
      <div className="pm-head">
        <Avatar name={me.name} color={me.avatarColor} size={52} />
        <div>
          <div className="pm-name">{me.name}</div>
          <div className="pm-sub">
            {me.role === 'owner' ? 'Owner' : me.role === 'admin' ? 'Admin' : me.role === 'kid' ? 'Kids profile' : 'Member'} · Harbor
          </div>
        </div>
      </div>
      {others.length > 0 && (
        <div className="pm-switch">
          {others.map((p) => (
            <button
              key={p.id}
              className="pm-other"
              onClick={() => {
                onClose();
                if (p.hasPin) {
                  switchProfile();
                  navigate(`/profiles?unlock=${p.id}`);
                  return;
                }
                stop();
                choose(p.id);
                qc.clear();
                navigate('/', { viewTransition: true });
              }}
            >
              <Avatar name={p.name} color={p.avatarColor} size={40} />
              <span>{p.name}</span>
            </button>
          ))}
        </div>
      )}
      <div className="menu-sep" />
      <button className="menu-item" onClick={() => go('/settings')}>
        <Settings />
        Settings
      </button>
      <button className="menu-item" onClick={() => go('/settings/profiles')}>
        <Users />
        Profiles & Parental Controls
      </button>
      <button className="menu-item" onClick={() => go('/settings/security')}>
        <Shield />
        Sign-In & Security
      </button>
      {admin && (
        <button className="menu-item" onClick={() => go('/console')}>
          <Server />
          Server Console
          <span className="hint">Harbor</span>
        </button>
      )}
      <button
        className="menu-item hide-mobile"
        onClick={() => {
          onClose();
          setShortcuts(true);
        }}
      >
        <Keyboard />
        Keyboard Shortcuts
        <span className="hint">?</span>
      </button>
      <div className="menu-sep" />
      <button
        className="menu-item"
        onClick={() => {
          onClose();
          stop();
          switchProfile();
          navigate('/profiles');
        }}
      >
        <UserRoundCog />
        Switch Profile
      </button>
      <button
        className="menu-item danger"
        onClick={() => {
          onClose();
          stop();
          signOut();
          qc.clear();
          navigate('/signin');
        }}
      >
        <LogOut />
        Sign Out
      </button>
    </div>
  );
}
