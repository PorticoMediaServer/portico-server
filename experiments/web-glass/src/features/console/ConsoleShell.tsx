import { Suspense, lazy, useEffect, useRef } from 'react';
import { Link, useLocation, useNavigate } from 'react-router';
import {
  Activity,
  ArrowLeft,
  ChevronRight,
  DatabaseBackup,
  Gauge,
  HardDrive,
  LayoutDashboard,
  ListChecks,
  MonitorPlay,
  Network,
  ScrollText,
  Search,
  Server,
  Settings,
  Stethoscope,
  Tags,
  Tv2,
  Users,
  type LucideIcon,
} from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import { useCurrentProfile } from '../../app/ProfileMenu';
import { CommandPalette } from '../../app/CommandPalette';
import { useUi } from '../../state/ui';
import { useAdminOverview, useSessions } from '../../services/queries';
import { Button, EmptyState, IconButton, Kbd, cls } from '../../components/ui';
import { Toaster } from '../../components/overlay';
import { useIsMobile } from '../../lib/hooks';
import { useRefraction } from '../../lib/refraction';
import { PageFallback } from '../../components/Fallbacks';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const L = (f: () => Promise<Record<string, any>>, n: string) => lazy(() => f().then((m) => ({ default: m[n] as React.ComponentType<any> })));
const Overview = L(() => import('./sections/Overview'), 'Overview');
const Sessions = L(() => import('./sections/Sessions'), 'Sessions');
const ActivityFeed = L(() => import('./sections/Activity'), 'ActivityFeed');
const Logs = L(() => import('./sections/Logs'), 'Logs');
const Libraries = L(() => import('./sections/Libraries'), 'Libraries');
const Metadata = L(() => import('./sections/Metadata'), 'Metadata');
const LiveTv = L(() => import('./sections/LiveTv'), 'LiveTv');
const UsersSection = L(() => import('./sections/Users'), 'UsersSection');
const Transcoding = L(() => import('./sections/Transcoding'), 'Transcoding');
const NetworkSection = L(() => import('./sections/Network'), 'NetworkSection');
const Storage = L(() => import('./sections/Storage'), 'Storage');
const Backups = L(() => import('./sections/Backups'), 'Backups');
const Tasks = L(() => import('./sections/Tasks'), 'Tasks');
const Diagnostics = L(() => import('./sections/Diagnostics'), 'Diagnostics');
const ServerSettings = L(() => import('./sections/ServerSettings'), 'ServerSettings');

interface Sec {
  id: string;
  label: string;
  icon: LucideIcon;
  group: 'Monitor' | 'Library' | 'People' | 'System';
  el: React.ComponentType;
  desc: string;
}

export const SECTIONS: Sec[] = [
  { id: '', label: 'Overview', icon: LayoutDashboard, group: 'Monitor', el: Overview, desc: 'Health, load and what’s playing' },
  { id: 'sessions', label: 'Now Playing', icon: MonitorPlay, group: 'Monitor', el: Sessions, desc: 'Live streams and transcodes' },
  { id: 'activity', label: 'Activity', icon: Activity, group: 'Monitor', el: ActivityFeed, desc: 'Who did what, when' },
  { id: 'logs', label: 'Logs', icon: ScrollText, group: 'Monitor', el: Logs, desc: 'Live server log with filters' },
  { id: 'libraries', label: 'Libraries', icon: Server, group: 'Library', el: Libraries, desc: 'Folders, scanning and options' },
  { id: 'metadata', label: 'Metadata', icon: Tags, group: 'Library', el: Metadata, desc: 'Agents, unmatched items, duplicates' },
  { id: 'livetv', label: 'Live TV & DVR', icon: Tv2, group: 'Library', el: LiveTv, desc: 'Tuners, guide data, recording' },
  { id: 'users', label: 'Users & Sharing', icon: Users, group: 'People', el: UsersSection, desc: 'Household, friends, access, limits' },
  { id: 'transcoding', label: 'Transcoding', icon: Gauge, group: 'System', el: Transcoding, desc: 'Hardware acceleration and quality' },
  { id: 'network', label: 'Network', icon: Network, group: 'System', el: NetworkSection, desc: 'Remote access, domains, bandwidth' },
  { id: 'storage', label: 'Storage', icon: HardDrive, group: 'System', el: Storage, desc: 'Volumes, drives and caches' },
  { id: 'backups', label: 'Backups', icon: DatabaseBackup, group: 'System', el: Backups, desc: 'Schedule, destinations, restore' },
  { id: 'tasks', label: 'Scheduled Tasks', icon: ListChecks, group: 'System', el: Tasks, desc: 'Maintenance jobs and history' },
  { id: 'diagnostics', label: 'Diagnostics', icon: Stethoscope, group: 'System', el: Diagnostics, desc: 'Health checks and support bundle' },
  { id: 'settings', label: 'Server Settings', icon: Settings, group: 'System', el: ServerSettings, desc: 'Name, updates, security, DLNA' },
];

const GROUPS = ['Monitor', 'Library', 'People', 'System'] as const;

export function ConsoleShell() {
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const me = useCurrentProfile();
  const mobile = useIsMobile();
  const qc = useQueryClient();
  const openPalette = useUi((s) => s.openPalette);
  const overview = useAdminOverview();
  const sessions = useSessions();
  const bar = useRef<HTMLDivElement>(null);
  useRefraction(bar, { scale: 52, radius: 22 });
  const id = pathname.replace(/^\/console\/?/, '').split('/')[0];
  const sec = SECTIONS.find((s) => s.id === id);
  const group = sec?.group ?? 'Monitor';

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || e.metaKey || e.ctrlKey || e.altKey) {
        if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
          e.preventDefault();
          openPalette();
        }
        return;
      }
      const inGroup = SECTIONS.filter((s) => s.group === group);
      if (/^[1-9]$/.test(e.key) && inGroup[Number(e.key) - 1]) navigate(`/console/${inGroup[Number(e.key) - 1].id}`);
      if (e.key === 'r') qc.invalidateQueries({ queryKey: ['admin'] });
      if (e.key === '/' ) {
        const f = document.querySelector<HTMLInputElement>('.cfilter input');
        if (f) {
          e.preventDefault();
          f.focus();
        } else {
          e.preventDefault();
          openPalette();
        }
      }
      if (e.key === 'Escape' && !document.querySelector('.sheet-root, .palette-root')) navigate('/');
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [group, navigate, qc, openPalette]);

  if (me && me.role !== 'owner' && me.role !== 'admin')
    return (
      <div className="console">
        <div className="canvas" aria-hidden="true" />
        <EmptyState icon={<Server />} title="Only owners and admins can open the Server Console" body={`Ask ${'Maya'} if you need something changed on Harbor.`} actions={<Button to="/" variant="accent">Back to Portico</Button>} />
      </div>
    );

  const warn = overview.data?.health.warn ?? 0;
  const streams = sessions.data?.length ?? overview.data?.sessions.length ?? 0;
  const Section = sec?.el;

  return (
    <div className="console">
      <div className="canvas" aria-hidden="true" />
      <header className="cbar-wrap">
        <div ref={bar} className="cbar glass glass-chrome refract">
          <Link to="/" className="cbar-back" aria-label="Back to Portico">
            <ArrowLeft />
            <img src="/brand/portico-symbol-mono-white.svg" alt="" />
          </Link>
          <div className="cbar-server">
            <span className="cbar-name">Harbor</span>
            <span className="cbar-meta">
              <i className="cdot ok" /> v3.2.1 · {streams} streams{warn ? ` · ${warn} warnings` : ''}
            </span>
          </div>
          {!mobile && (
            <nav className="cgroups" aria-label="Console groups">
              {GROUPS.map((g) => {
                const first = SECTIONS.find((s) => s.group === g)!;
                return (
                  <Link key={g} to={`/console/${first.id}`} className={cls('cgroup', g === group && 'on')}>
                    {g}
                  </Link>
                );
              })}
            </nav>
          )}
          <span style={{ flex: 1 }} />
          {!mobile && (
            <button className="cbar-search" onClick={() => openPalette()}>
              <Search />
              Jump to…
              <Kbd>⌘K</Kbd>
            </button>
          )}
          <IconButton label="Server settings" variant="plain" to="/console/settings">
            <Settings />
          </IconButton>
        </div>
        {!mobile && (
          <nav className="csections no-scrollbar" aria-label={`${group} sections`}>
            {SECTIONS.filter((s) => s.group === group).map((s, i) => (
              <Link key={s.id} to={`/console/${s.id}`} className={cls('csection', s.id === id && 'on')} aria-current={s.id === id ? 'page' : undefined}>
                <s.icon />
                {s.label}
                <span className="csection-key">{i + 1}</span>
              </Link>
            ))}
          </nav>
        )}
      </header>
      <main className="cmain">
        {mobile && id === '' && pathname === '/console' && <MobileConsoleIndex />}
        {sec && (!mobile || id !== '') && (
          <>
            <div className="ctitle">
              {mobile && (
                <Link to="/console" className="m-back">
                  <ArrowLeft /> Console
                </Link>
              )}
              <h1>{sec.label}</h1>
              <p>{sec.desc}</p>
            </div>
            <Suspense fallback={<PageFallback bare />}>{Section && <Section />}</Suspense>
          </>
        )}
        {!sec && (
          <EmptyState icon={<Server />} title="No such console section" actions={<Button to="/console" variant="accent">Overview</Button>} />
        )}
      </main>
      <CommandPalette />
      <Toaster />
    </div>
  );
}

function MobileConsoleIndex() {
  return (
    <div className="cm-index">
      <Suspense fallback={null}>
        <Overview compact />
      </Suspense>
      {GROUPS.map((g) => (
        <section key={g}>
          <h2 className="group-title">{g}</h2>
          <div className="group">
            {SECTIONS.filter((s) => s.group === g && s.id !== '').map((s) => (
              <Link key={s.id} to={`/console/${s.id}`} className="row has-icon">
                <span className="row-icon" style={{ background: '#1f4a6e' }}>
                  <s.icon />
                </span>
                <span className="row-main">
                  <span className="row-title">{s.label}</span>
                  <span className="row-sub">{s.desc}</span>
                </span>
                <ChevronRight className="row-chev" />
              </Link>
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}
