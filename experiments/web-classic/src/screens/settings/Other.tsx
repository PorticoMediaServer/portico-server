import {useState} from 'react';
import {Link, useNavigate} from '@tanstack/react-router';
import {useClient, useQuery} from '@/data/DataProvider';
import type {Notice} from '@/data/types';
import {useSession} from '@/app/session';
import {ago} from '@/app/format';
import {Button, IconButton} from '@/ui/Button';
import {Badge, EmptyState, Skeleton} from '@/ui/Bits';
import {Segmented} from '@/ui/Form';
import {ConfirmDialog, Menu, useToast} from '@/ui/Overlay';
import {Icon, type IconName} from '@/ui/Icon';
import {Group, Row} from '@/ui/Page';
import {ProfileGrid, serverLine} from '@/shell/Switchers';
import {Dialog} from '@/ui/Overlay';
import {cx} from '@/ui/cx';
import s from './Settings.module.css';

const KIND: Record<Notice['kind'], {icon: IconName; label: string}> = {feedback: {icon: 'mail', label: 'Replies'}, download: {icon: 'download', label: 'Downloads'}, server: {icon: 'server', label: 'Server'}, security: {icon: 'shield', label: 'Security'}, recording: {icon: 'recordings', label: 'Recordings'}, broadcast: {icon: 'bell', label: 'From the owner'}};

/** Notifications: unread, all and archived, by who they're for, with mark all read. */
export function NotificationsSection() {
  const client = useClient();
  const session = useSession();
  const navigate = useNavigate();
  const [view, setView] = useState<'unread' | 'all' | 'archived'>('all');
  const [audience, setAudience] = useState<'all' | 'profile' | 'admin'>('all');
  const q = useQuery(c => c.notifications.list(view), [view], ['notifications']);
  const list = (q.data ?? []).filter(n => audience === 'all' || n.audience === audience);
  return (
    <>
      <div style={{display: 'flex', gap: 10, flexWrap: 'wrap', alignItems: 'center'}}>
        <Segmented label="Show" value={view} onChange={setView} options={[{value: 'unread', label: 'Unread'}, {value: 'all', label: 'All'}, {value: 'archived', label: 'Archived'}]} />
        {session.isAdmin ? <Segmented label="For" value={audience} onChange={setAudience} options={[{value: 'all', label: 'Everything'}, {value: 'profile', label: 'For me'}, {value: 'admin', label: 'Server alerts'}]} /> : null}
        <span className="grow" />
        <Button variant="ghost" icon="check" label="Mark all as read" onClick={() => void client.notifications.markAll()} disabled={!q.data?.some(n => !n.read)} />
      </div>
      {!q.data ? <Skeleton height={240} /> : !list.length ? (
        <EmptyState icon="bell" title={view === 'unread' ? 'You’re all caught up' : view === 'archived' ? 'Nothing archived' : 'No notifications'} body={view === 'unread' ? 'New messages from the server and its owner show up here.' : undefined} />
      ) : (
        <Group>
          {list.map(n => (
            <div key={n.id} className={cx(s.notice, !n.read && s.noticeUnread)}>
              <span className={s.noticeIcon}><Icon name={KIND[n.kind].icon} size={18} /></span>
              <div style={{minWidth: 0}}>
                <div className="t-strong">{!n.read ? <span className={s.unreadDot} aria-label="Unread" /> : null}{n.title}</div>
                <p className="c-2" style={{fontSize: 14, marginTop: 2}}>{n.body}</p>
                <div className="t-caption c-3" style={{marginTop: 6, display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap'}}>{ago(n.at)} · {KIND[n.kind].label}{n.audience === 'admin' ? <Badge tone={n.importance === 'urgent' ? 'bad' : 'neutral'}>Server alert</Badge> : null}
                  {n.kind === 'security' && n.audience === 'admin' ? <Link to="/server/access" className="c-accent" style={{fontWeight: 650}}>Review in Access</Link> : null}
                  {n.kind === 'download' ? <Link to="/downloads" className="c-accent" style={{fontWeight: 650}}>Open Downloads</Link> : null}
                  {n.kind === 'recording' ? <Link to="/channels/recordings?tab=recorded" className="c-accent" style={{fontWeight: 650}}>Open Recordings</Link> : null}
                </div>
              </div>
              <Menu label="Notification options" items={[
                {id: 'read', label: n.read ? 'Mark as unread' : 'Mark as read', icon: 'check', onSelect: () => void client.notifications.mark(n.id, !n.read)},
                {id: 'archive', label: n.archived ? 'Move to inbox' : 'Archive', icon: 'archive', onSelect: () => void client.notifications.archive(n.id, !n.archived)},
                ...(n.kind === 'server' && session.isAdmin ? [{id: 'server', label: 'Open Manage server', icon: 'console' as const, onSelect: () => void navigate({to: '/server/overview' as never})}] : []),
              ]} trigger={<IconButton variant="ghost" icon="more" label="Notification options" />} />
            </div>
          ))}
        </Group>
      )}
    </>
  );
}

/** Servers you can use, which one this is, switching, refreshing and signing in directly. */
export function ServersSection() {
  const session = useSession();
  const client = useClient();
  const toast = useToast();
  const navigate = useNavigate();
  const servers = useQuery(c => c.session.servers(), []);
  const [switchTo, setSwitchTo] = useState<string>();
  const [leave, setLeave] = useState<string>();
  const [busy, setBusy] = useState(false);
  return (
    <>
      <Group title="Your servers" lede="Servers on your Portico Account, including ones others share with you.">
        {!servers.data ? <div style={{padding: 16}}><Skeleton height={140} /></div> : servers.data.map(sv => (
          <Row key={sv.id} icon="server" label={<>{sv.name} {session.server?.id === sv.id ? <Badge tone="accent">Using now</Badge> : sv.status === 'unreachable' ? <Badge tone="warn" dot>Not reachable</Badge> : null}</>} help={`${serverLine(sv)} · Portico ${sv.version} · ${sv.host}`}
            trailing={<>
              {session.server?.id !== sv.id && sv.status === 'online' ? <Button size="sm" variant="secondary" label="Switch" onClick={async () => { await client.session.chooseServer(sv.id); setSwitchTo(sv.id); }} /> : null}
              <Menu label={`${sv.name} options`} items={[
                {id: 'direct', label: 'Sign in directly…', icon: 'key', onSelect: () => void navigate({to: '/sign-in/direct' as never, search: {host: sv.host} as never})},
                ...(sv.yours ? [] : [{id: 'leave', label: 'Leave this server…', icon: 'signOut' as const, danger: true, separatorBefore: true, onSelect: () => setLeave(sv.id)}]),
              ]} trigger={<IconButton variant="ghost" icon="more" label={`${sv.name} options`} />} />
            </>} />
        ))}
        <Row icon="refresh" label="Refresh the list" help="Check for servers shared with you since you signed in." trailing={<Button size="sm" variant="ghost" label="Refresh" busy={busy} onClick={async () => { setBusy(true); servers.refresh(); await new Promise(r => setTimeout(r, 600)); setBusy(false); toast.show('Server list is up to date', {icon: 'check'}); }} />} />
      </Group>
      <Group title="Add a server">
        <Row icon="plus" label="Join with an invitation" help="Paste the invitation link or code someone sent you." to="/join" />
        <Row icon="key" label="Sign in to a server directly" help="Use a server’s own address and account, without a Portico Account." to="/sign-in/direct" />
        <Row icon="server" label="Set up a new server" help="Install Portico Server on a computer or NAS, then claim it here." to="/setup" />
      </Group>
      <Dialog open={!!switchTo} onOpenChange={o => !o && setSwitchTo(undefined)} title="Who’s watching?" width={620}>
        {switchTo ? <ProfileGrid serverId={switchTo} onChosen={() => { setSwitchTo(undefined); void navigate({to: '/' as never}); }} /> : null}
      </Dialog>
      {leave ? <ConfirmDialog open onOpenChange={o => !o && setLeave(undefined)} title="Leave this server?" body="You lose access to its libraries. Your history there is kept in case the owner invites you again." action="Leave server" tone="danger" onConfirm={async () => { await client.account.leaveServer(leave); setLeave(undefined); toast.show('You left the server', {icon: 'check'}); }} /> : null}
    </>
  );
}

export function AboutSection() {
  const session = useSession();
  return (
    <>
      <Group title="Portico">
        <div style={{padding: '22px 16px', display: 'flex', alignItems: 'center', gap: 16}}>
          <img src="/brand/portico-symbol-dark.svg" alt="" style={{height: 44}} />
          <div><div className="t-sub">Portico for the web</div><div className="t-caption c-3">Version 1.4.2 · Connected to {session.server?.name}, Portico Server {session.server?.version}</div></div>
        </div>
        <Row icon="help" label="Help and support" help="Guides, troubleshooting and the community forum" onClick={() => window.open('https://getportico.tv/help', '_blank', 'noopener')} trailing={<Icon name="external" size={16} />} />
        <Row icon="keyboard" label="Keyboard shortcuts" help="Press ? anywhere to see them" />
        <Row icon="info" label="Open-source licenses" help="Manrope (SIL Open Font License), React, TanStack Router, Radix UI (MIT)" />
        <Row icon="photo" label="Artwork in this library" help="Public-domain posters and stills from Wikimedia Commons, Blender Foundation open movies (CC BY), and photographs from Unsplash" />
        <Row icon="gridView" label="Screen index" help="Every page and state, for review" to="/dev/screens" />
      </Group>
      <p className="t-caption c-3">© 2026 Portico. Made for people who keep their own media.</p>
    </>
  );
}
