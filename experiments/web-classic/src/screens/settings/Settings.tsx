import {Link, useNavigate} from '@tanstack/react-router';
import {useSession} from '@/app/session';
import {useClient, useTopic} from '@/data/DataProvider';
import {Avatar, Badge, EmptyState} from '@/ui/Bits';
import {Button} from '@/ui/Button';
import {Icon, type IconName} from '@/ui/Icon';
import {BackLink, Group, PageHeader, Row} from '@/ui/Page';
import {useMediaQuery, useTitle} from '@/ui/hooks';
import {useRouteParams} from '../common';
import {PreferencesSection} from './Preferences';
import {AccountSection, DevicesSection, ProfilesSection, SecuritySection} from './Account';
import {AboutSection, NotificationsSection, ServersSection} from './Other';
import s from './Settings.module.css';

type Sec = {id: string; label: string; icon: IconName; group: 'You' | 'This profile' | 'Portico'; help: string};
export const SECTIONS: Sec[] = [
  {id: 'account', label: 'Account', icon: 'user', group: 'You', help: 'Name, picture and PIN for this profile'},
  {id: 'profiles', label: 'Profiles', icon: 'together', group: 'You', help: 'Everyone in your household, limits and PINs'},
  {id: 'security', label: 'Security', icon: 'shield', group: 'You', help: 'Password, two-step sign-in and signing out everywhere'},
  {id: 'devices', label: 'Devices', icon: 'laptop', group: 'You', help: 'Where you’re signed in'},
  {id: 'preferences', label: 'Preferences', icon: 'sliders', group: 'This profile', help: 'Playback, subtitles, languages, quality and appearance'},
  {id: 'notifications', label: 'Notifications', icon: 'bell', group: 'This profile', help: 'Messages from the server and its owner'},
  {id: 'servers', label: 'Servers', icon: 'server', group: 'Portico', help: 'Servers you can use, and switching between them'},
  {id: 'about', label: 'About', icon: 'info', group: 'Portico', help: 'Version, licenses and help'},
];

/** Settings: a section list beside the section on wide screens; the list, then each section, on narrow ones. */
export default function SettingsScreen() {
  const {section} = useRouteParams<{section?: string}>();
  const wide = !useMediaQuery('(max-width: 1023px)');
  const current = SECTIONS.find(x => x.id === section);
  useTitle(current?.label ?? 'Settings');
  if (section && !current) return <EmptyState icon="settings" title="There’s no such setting" body="It may have moved. Here’s the list of settings." actions={[{label: 'All settings', to: '/settings'}]} />;
  const id = current?.id;
  return (
    <div style={{padding: '28px 0 72px'}}>
      {!wide && current ? <div style={{padding: '0 var(--gutter)'}}><BackLink fallback="/settings" label="Settings" /></div> : null}
      <PageHeader title={current && !wide ? current.label : 'Settings'} subtitle={current && !wide ? current.help : undefined} />
      <div className={s.layout}>
        <SettingsNav current={id} />
        <div className={s.content}>{id && wide ? <div><h2 className="t-heading">{current!.label}</h2><p className="c-3 t-caption" style={{marginTop: 4}}>{current!.help}</p></div> : null}{id ? <SectionBody id={id} /> : <Hub />}</div>
      </div>
    </div>
  );
}

function SettingsNav({current}: {current?: string}) {
  const session = useSession();
  const client = useClient();
  useTopic(['notifications']);
  const unread = client.notifications.unread();
  const groups = ['You', 'This profile', 'Portico'] as const;
  return (
    <nav className={s.nav} aria-label="Settings">
      <Link to="/settings" className={s.navItem} aria-current={!current ? 'page' : undefined} activeOptions={{exact: true}} activeProps={{}}><Icon name="settings" size={18} />Overview</Link>
      {groups.map(g => (
        <div key={g} style={{display: 'contents'}}>
          <div className={s.navGroup}>{g}</div>
          {SECTIONS.filter(x => x.group === g && (x.id !== 'profiles' || session.profile?.main || session.isAdmin)).map(x => (
            <Link key={x.id} to={`/settings/${x.id}` as never} className={s.navItem} aria-current={current === x.id ? 'page' : undefined} activeProps={{}}>
              <Icon name={x.icon} size={18} /><span style={{flex: 1}}>{x.label}</span>{x.id === 'notifications' && unread ? <Badge tone="accent">{unread}</Badge> : null}
            </Link>
          ))}
        </div>
      ))}
      {session.isAdmin ? <><div className={s.navGroup}>Server</div><Link to="/server/overview" className={s.navItem}><Icon name="console" size={18} />Manage server</Link></> : null}
    </nav>
  );
}

function Hub() {
  const session = useSession();
  const navigate = useNavigate();
  const p = session.profile;
  const narrow = useMediaQuery('(max-width: 1023px)');
  return (
    <>
      <Link to="/settings/account" className={s.hubCard}>
        <Avatar name={p?.name ?? '?'} art={p?.art} size={56} kids={p?.kids} />
        <span style={{flex: 1, minWidth: 0}}><span className="t-sub" style={{display: 'block'}}>{p?.name}</span><span className="t-caption c-3">{session.server?.name} · Portico Account ({session.session?.account.username}){p?.role === 'owner' ? ' · Owner' : p?.role === 'admin' ? ' · Administrator' : ''}</span></span>
        <Icon name="forward" size={18} className="c-3" />
      </Link>
      {narrow ? (
        ['You', 'This profile', 'Portico'].map(g => (
          <Group key={g} title={g}>
            {SECTIONS.filter(x => x.group === g && (x.id !== 'profiles' || p?.main || session.isAdmin)).map(x => <Row key={x.id} icon={x.icon} label={x.label} help={x.help} to={`/settings/${x.id}`} />)}
          </Group>
        ))
      ) : (
        <AtAGlance />
      )}
      <Group title="Also here">
        <Row icon="download" label="Downloads" help="Titles saved in this browser for offline viewing" to="/downloads" />
        <Row icon="together" label="Watch Together" help="Watch in step with others" to="/together" />
        <Row icon="tvDevice" label="Link a TV" help="Enter the code a TV shows to sign it in" to="/device" />
        <Row icon="user" label="Portico Account" help="Sessions, security, servers and notifications for your account" to="/account" />
      </Group>
      {session.isAdmin ? (
        <Group title="Your server">
          <Row icon="console" label={`You manage ${session.server?.name}`} help="Libraries, people, live TV, transcoding, backups and everything else about the server." trailing={<Button size="sm" variant="secondary" label="Manage server" onClick={() => void navigate({to: '/server/overview' as never})} />} />
        </Group>
      ) : null}
      <div><Button variant="danger" icon="signOut" label="Sign out" onClick={() => void session.signOut().then(() => navigate({to: '/welcome' as never}))} /></div>
    </>
  );
}

function SectionBody({id}: {id: string}) {
  switch (id) {
    case 'preferences': return <PreferencesSection />;
    case 'account': return <AccountSection />;
    case 'profiles': return <ProfilesSection />;
    case 'security': return <SecuritySection />;
    case 'devices': return <DevicesSection />;
    case 'notifications': return <NotificationsSection />;
    case 'servers': return <ServersSection />;
    default: return <AboutSection />;
  }
}

/** Wide screens list the sections beside this, so the overview shows state instead: what's on and what needs a look. */
function AtAGlance() {
  const session = useSession();
  const client = useClient();
  useTopic(['notifications', 'account']);
  const unread = client.notifications.unread();
  const acct = session.session!.account;
  const p = session.profile!;
  return (
    <Group title="At a glance">
      <Row icon="bell" label={unread ? `${unread} unread notification${unread === 1 ? '' : 's'}` : 'No unread notifications'} help="Replies to your reports, finished recordings and messages from the owner" to="/settings/notifications" />
      <Row icon="shield" label={acct.twoStep ? 'Two-step sign-in is on' : 'Two-step sign-in is off'} help={acct.twoStep ? `${acct.recoveryCodesLeft} recovery codes left` : 'Add a code from an authenticator app when you sign in'} to="/settings/security" />
      <Row icon="lock" label={p.pin ? 'This profile has a PIN' : 'This profile has no PIN'} help={p.pin ? 'A four-digit PIN is needed to open it' : 'Anyone using this account can open it'} to="/settings/account" />
      <Row icon="server" label={`Watching from ${session.server?.name}`} help={`Portico Server ${session.server?.version} · ${session.server?.host}`} to="/settings/servers" />
      <Row icon="sliders" label="Playback and subtitles" help={`Skip back ${client.prefs.get('playback.skipBackSeconds')} s, forward ${client.prefs.get('playback.skipForwardSeconds')} s · Subtitles ${client.prefs.get('playback.subtitlesEnabled') ? 'on' : 'off'} by default`} to="/settings/preferences" />
    </Group>
  );
}
