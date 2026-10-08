import {useNavigate} from '@tanstack/react-router';
import {useQuery, useServices} from '../data/hooks';
import {useSession} from '../app/session';
import {Dialog} from '../ui/Overlay';
import {Badge} from '../ui/Feedback';
import {Icon} from '../ui/Icon';
import {Avatar, BrandSymbol} from '../ui/Media';
import {Button} from '../ui/Button';
import {cx} from '../ui/cx';
import {relativeTime} from '../screens/shared/items';

/** Switch between the Portico Account's servers (and this browser's direct sign-ins). */
export function ServerSwitcher({onClose}: {onClose: () => void}) {
  const services = useServices();
  const session = useSession();
  const navigate = useNavigate();
  const servers = useQuery('auth.servers', () => services.auth.servers());
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title="Switch server" description="Your Portico Account stays signed in." size="sm" footer={<Button variant="ghost" icon="plus" label="Sign in to a server directly" onClick={() => { onClose(); void navigate({to: '/welcome/$screen', params: {screen: 'direct'}}); }} />}>
      <div className="server-list">
        {(servers.data ?? []).map(s => (
          <button key={s.id} type="button" className={cx('server-pick', s.status !== 'online' && 'is-off', s.id === session.serverId && 'is-current')} onClick={() => { if (s.status !== 'online' || s.id === session.serverId) return; session.setServer(s.id); onClose(); void navigate({to: '/welcome/$screen', params: {screen: 'profiles'}}); }} aria-disabled={s.status !== 'online'}>
            <span className="server-glyph"><BrandSymbol size={22} /></span>
            <span className="set-text"><strong>{s.name}</strong><span>{s.status === 'online' ? `${s.role === 'owner' ? 'Yours' : s.role === 'admin' ? 'Admin' : 'Member'} · ${s.via === 'account' ? 'Portico Account' : 'Signed in directly'}` : `Can’t reach it · last seen ${relativeTime(s.lastSeen).toLowerCase()}`}</span></span>
            {s.id === session.serverId ? <Badge tone="accent">Current</Badge> : s.status === 'online' ? <Icon name="chevronRight" size={18} className="row-chevron" /> : <Badge tone="warn">Offline</Badge>}
          </button>
        ))}
      </div>
    </Dialog>
  );
}

/** Switch profile on this server (profiles with a PIN ask for it). */
export function ProfileSwitcher({onClose}: {onClose: () => void}) {
  const session = useSession();
  const navigate = useNavigate();
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title="Switch profile" description={session.server?.name} size="sm">
      <div className="switch-profiles">
        {session.profiles.map(p => (
          <button key={p.id} type="button" className={cx('whos-card', p.id === session.profileId && 'is-current')} onClick={() => { onClose(); if (p.id === session.profileId) return; if (p.pin) { session.setProfile(null); void navigate({to: '/welcome/$screen', params: {screen: 'profiles'}}); } else { session.setProfile(p.id); void navigate({to: '/'}); } }}>
            <Avatar name={p.name} color={p.color} size={72} ring={p.id === session.profileId} />
            <strong>{p.name}</strong>
            <span className="faint small">{p.id === session.profileId ? 'Current' : p.pin ? 'PIN' : p.kids ? 'Kids' : ' '}</span>
          </button>
        ))}
      </div>
    </Dialog>
  );
}
