import React, {Suspense, lazy} from 'react';
import {Link, useNavigate, useParams} from '@tanstack/react-router';
import {useSession} from '../../app/session';
import {Page} from '../../ui/Layout';
import {Select} from '../../ui/Controls';
import {StateView} from '../../ui/Feedback';
import {Icon, type IconName} from '../../ui/Icon';
import {cx} from '../../ui/cx';
import {useAdmin, AdminSkeleton} from './common';

type Sec = {id: string; label: string; icon: IconName; group: 'Console' | 'Media' | 'Live TV' | 'Server'; lede: string};
export const SECTIONS: Sec[] = [
  {id: 'overview', label: 'Overview', icon: 'activity', group: 'Console', lede: 'How the server is doing right now, and anything that needs you.'},
  {id: 'now-playing', label: 'Now Playing', icon: 'play', group: 'Console', lede: 'Who is watching or listening, and how it reaches them.'},
  {id: 'activity', label: 'Activity', icon: 'history', group: 'Console', lede: 'Background work: scans, matching, conversions and backups.'},
  {id: 'logs', label: 'Logs', icon: 'logs', group: 'Console', lede: 'What the server has been doing, message by message.'},
  {id: 'diagnostics', label: 'Diagnostics', icon: 'gauge', group: 'Console', lede: 'What this server can do, and checks that it can do it.'},
  {id: 'feedback', label: 'Feedback', icon: 'message', group: 'Console', lede: 'Problems people reported from their apps.'},
  {id: 'alerts', label: 'Alerts', icon: 'bell', group: 'Console', lede: 'When the server tells you something needs attention.'},
  {id: 'libraries', label: 'Libraries', icon: 'library', group: 'Media', lede: 'The folders Portico watches, and how they’re doing.'},
  {id: 'library-settings', label: 'Library settings', icon: 'sliders', group: 'Media', lede: 'Matching, scanning, pictures and deleting, per library.'},
  {id: 'metadata', label: 'Metadata', icon: 'tags', group: 'Media', lede: 'Where details, artwork and lyrics come from.'},
  {id: 'storage', label: 'Storage', icon: 'cloud', group: 'Media', lede: 'Network shares and cloud storage the server can read.'},
  {id: 'live-tv', label: 'Live TV', icon: 'live', group: 'Live TV', lede: 'Tuners, playlists and guide sources for Live TV.'},
  {id: 'dvr', label: 'DVR', icon: 'record', group: 'Live TV', lede: 'How recordings are made, kept and named.'},
  {id: 'channels', label: 'Library Channels', icon: 'channels', group: 'Live TV', lede: 'Always-on channels made from your own libraries.'},
  {id: 'channel-builder', label: 'Channel Builder', icon: 'wand', group: 'Live TV', lede: 'Make a channel from rules: libraries, genres, order.'},
  {id: 'transcoding', label: 'Transcoding', icon: 'cpu', group: 'Server', lede: 'Converting media for devices that can’t play the original.'},
  {id: 'network', label: 'Network', icon: 'network', group: 'Server', lede: 'Reaching the server from home and from outside.'},
  {id: 'people', label: 'People', icon: 'people', group: 'Server', lede: 'Accounts on this server, their profiles and what they can see.'},
  {id: 'access', label: 'Access & API keys', icon: 'key', group: 'Server', lede: 'Invitations, device approval and keys for other apps.'},
  {id: 'schedules', label: 'Schedules', icon: 'calendar', group: 'Server', lede: 'When routine jobs run.'},
  {id: 'maintenance', label: 'Maintenance', icon: 'wrench', group: 'Server', lede: 'Quiet hours, cleanup, deleted titles, updates and messages.'},
  {id: 'backups', label: 'Backups', icon: 'database', group: 'Server', lede: 'Copies of the database and settings, and restoring them.'},
  {id: 'general', label: 'General', icon: 'settings', group: 'Server', lede: 'The server’s name, version and Portico Account connection.'},
];
const GROUPS = ['Console', 'Media', 'Live TV', 'Server'] as const;

const Console = lazy(() => import('./Console').then(m => ({default: m.ConsoleSection})));
const MediaAdmin = lazy(() => import('./MediaAdmin').then(m => ({default: m.MediaSection})));
const LiveAdmin = lazy(() => import('./LiveAdmin').then(m => ({default: m.LiveSection})));
const SystemAdmin = lazy(() => import('./SystemAdmin').then(m => ({default: m.SystemSection})));

/** Server administration: a stable section list on the left, one question answered per page. */
export function ServerScreen() {
  const {section} = useParams({from: '/app/server/$section'});
  const session = useSession();
  const navigate = useNavigate();
  const current = SECTIONS.find(s => s.id === section) ?? SECTIONS[0]!;
  const overview = useAdmin('overview');
  const attention = (overview.data?.attention ?? []).filter(a => !a.acknowledged).length;
  if (!session.manager) return <Page title="Server"><StateView icon="lock" title="Only the owner and admins manage this server" body="Ask the server owner if something needs changing." action={{label: 'Go home', to: '/', icon: 'home'}} /></Page>;
  const group = current.group;
  const Body = group === 'Console' ? Console : group === 'Media' ? MediaAdmin : group === 'Live TV' ? LiveAdmin : SystemAdmin;
  return (
    <Page title={`${current.label} · Server`} className="admin-page">
      <div className="adm">
        <nav className="adm-nav" aria-label="Server administration">
          <div className="adm-server"><span className="eyebrow">Server</span><strong>{session.server?.name}</strong></div>
          {GROUPS.map(g => (
            <React.Fragment key={g}>
              <div className="adm-group">{g}</div>
              {SECTIONS.filter(s => s.group === g).map(s => (
                <Link key={s.id} to="/server/$section" params={{section: s.id}} search={{}} className={cx('adm-link', s.id === current.id && 'is-on')} aria-current={s.id === current.id ? 'page' : undefined}>
                  <Icon name={s.icon} size={17} />{s.label}{s.id === 'overview' && attention ? <span className="rail-count">{attention}</span> : null}
                </Link>
              ))}
            </React.Fragment>
          ))}
        </nav>
        <div className="adm-body">
          <div className="adm-picker">
            <Select label="Section" value={current.id} onChange={v => void navigate({to: '/server/$section', params: {section: v}, search: {}})} options={SECTIONS.map(s => ({value: s.id, label: `${s.group} · ${s.label}`}))} />
          </div>
          <Suspense fallback={<AdminSkeleton />}><Body id={current.id} sec={current} /></Suspense>
        </div>
      </div>
    </Page>
  );
}

export type {Sec};
