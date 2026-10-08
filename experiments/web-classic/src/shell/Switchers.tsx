import {useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useClient, useQuery} from '@/data/DataProvider';
import type {Profile, Server} from '@/data/types';
import {useSession} from '@/app/session';
import {Avatar, Badge, Skeleton, Spinner} from '@/ui/Bits';
import {Button} from '@/ui/Button';
import {Dialog} from '@/ui/Overlay';
import {PinInput} from '@/ui/Form';
import {Icon} from '@/ui/Icon';
import {cx} from '@/ui/cx';
import s from './Switchers.module.css';

/** "Who's watching?" with PIN entry. Used by sign-in and by Switch profile. */
export function ProfileGrid({serverId, onChosen}: {serverId?: string; onChosen?: () => void}) {
  const session = useSession();
  const profiles = useQuery(c => c.session.profiles(serverId), [serverId], ['account']);
  const [pinFor, setPinFor] = useState<Profile>();
  const [pin, setPin] = useState('');
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState<string>();
  const choose = async (p: Profile, code?: string) => {
    setBusy(p.id);
    setError(undefined);
    try {
      await session.enter(p.id, code);
      setPinFor(undefined);
      onChosen?.();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'That PIN isn’t right. Try again.');
      setPin('');
    } finally { setBusy(undefined); }
  };
  if (profiles.loading && !profiles.data) return <div className={s.grid}>{[0, 1, 2, 3].map(i => <div key={i} className={s.profile}><Skeleton width={96} height={96} radius={48} /><Skeleton width={64} height={14} /></div>)}</div>;
  if (pinFor) {
    return (
      <div className={s.pin}>
        <Avatar name={pinFor.name} art={pinFor.art} size={80} />
        <div><div className="t-sub">Enter {pinFor.name}’s PIN</div><div className="t-caption c-3" style={{marginTop: 4}}>This profile is protected with a four-digit PIN.</div></div>
        <PinInput label={`${pinFor.name}’s PIN`} value={pin} onChange={v => { setPin(v); setError(undefined); }} autoFocus onComplete={v => void choose(pinFor, v)} />
        {error ? <div className={s.error} role="alert">{error}</div> : busy ? <Spinner size={22} /> : <div style={{height: 20}} />}
        <div className="t-caption c-3">Forgot it? The main profile can reset PINs in Settings › Account › Profiles.</div>
        <Button variant="ghost" icon="back" label="Choose another profile" onClick={() => { setPinFor(undefined); setError(undefined); setPin(''); }} />
      </div>
    );
  }
  return (
    <div className={s.grid} role="group" aria-label="Profiles">
      {(profiles.data ?? []).map((p, i) => (
        <button key={p.id} type="button" className={cx(s.profile, session.profile?.id === p.id && s.current)} onClick={() => (p.pin ? setPinFor(p) : void choose(p))} disabled={!!busy} aria-label={`${p.name}${p.pin ? ', protected by a PIN' : ''}${p.kids ? ', kids profile' : ''}`} autoFocus={i === 0}>
          <span className={s.ring}>
            <Avatar name={p.name} art={p.art} size={88} />
            {p.pin ? <span className={s.lock}><Icon name="lock" size={14} /></span> : null}
          </span>
          <span className={s.name}>{p.name}</span>
          <span className={s.role}>{p.kids ? <Badge tone="warn">Kids</Badge> : p.role === 'owner' ? 'Owner' : p.role === 'admin' ? 'Administrator' : ''}</span>
          {busy === p.id ? <Spinner size={18} /> : null}
        </button>
      ))}
    </div>
  );
}

export function ProfileSwitcher({open, onClose}: {open: boolean; onClose: () => void}) {
  const navigate = useNavigate();
  return (
    <Dialog open={open} onOpenChange={o => !o && onClose()} title="Switch profile" description="Each profile keeps its own history, Watchlist, recommendations and settings." width={620}>
      <ProfileGrid onChosen={() => { onClose(); void navigate({to: '/' as never}); }} />
    </Dialog>
  );
}

export function serverLine(sv: Server): string {
  const state = sv.status === 'unreachable' ? `Last seen ${sv.lastSeen.toLowerCase()}` : sv.yours ? 'Yours' : `Shared by ${sv.owner}`;
  return `${state} · ${sv.libraries} libraries`;
}

/** The Portico Account's servers, including shared ones and one that can't be reached. */
export function ServerList({onChosen, current}: {onChosen: (sv: Server) => void; current?: string}) {
  const client = useClient();
  const servers = useQuery(c => c.session.servers(), []);
  const [busy, setBusy] = useState<string>();
  const [error, setError] = useState<string>();
  if (!servers.data) return <div className={s.servers}>{[0, 1, 2].map(i => <Skeleton key={i} height={66} radius={12} />)}</div>;
  return (
    <div className={s.servers}>
      {servers.data.map(sv => (
        <button key={sv.id} type="button" className={cx(s.server, current === sv.id && s.serverCurrent)} disabled={!!busy} onClick={async () => {
          setBusy(sv.id); setError(undefined);
          const r = await client.session.chooseServer(sv.id);
          setBusy(undefined);
          if (r.status === 'online') onChosen(sv);
          else setError(`${sv.name} isn’t reachable right now. It was last seen ${sv.lastSeen.toLowerCase()}. Check that it’s turned on and connected, then try again.`);
        }}>
          <span className={s.serverIcon}><Icon name="server" size={20} /></span>
          <span className={s.serverText}><span className={s.serverName}>{sv.name}</span><span className={s.serverSub}>{serverLine(sv)}</span></span>
          {busy === sv.id ? <Spinner size={20} /> : current === sv.id ? <Badge tone="accent">Current</Badge> : sv.status === 'unreachable' ? <Badge tone="warn" dot>Offline</Badge> : <Icon name="forward" size={16} className="c-3" />}
        </button>
      ))}
      {error ? <p className={s.error} role="alert">{error}</p> : null}
    </div>
  );
}

export function ServerSwitcher({open, onClose}: {open: boolean; onClose: () => void}) {
  const session = useSession();
  const [chosen, setChosen] = useState<Server>();
  const navigate = useNavigate();
  return (
    <Dialog open={open} onOpenChange={o => { if (!o) { setChosen(undefined); onClose(); } }} title={chosen ? `Who’s watching on ${chosen.name}?` : 'Switch server'} description={chosen ? undefined : 'Servers on your Portico Account, including ones shared with you.'} width={chosen ? 620 : 520}>
      {chosen ? <ProfileGrid serverId={chosen.id} onChosen={() => { setChosen(undefined); onClose(); void navigate({to: '/' as never}); }} /> : <ServerList current={session.server?.id} onChosen={setChosen} />}
    </Dialog>
  );
}
