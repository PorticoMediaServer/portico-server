import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { useNavigate } from 'react-router';
import {
  Activity,
  ArrowRight,
  CornerDownLeft,
  Download,
  Eye,
  Gauge,
  HardDrive,
  History,
  KeyRound,
  LayoutDashboard,
  ListChecks,
  LogOut,
  Network,
  RefreshCw,
  ScrollText,
  Search,
  Server,
  Settings,
  Stethoscope,
  Tags,
  Tv2,
  UserRoundCog,
  Users,
  Wand2,
} from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import { useUi } from '../state/ui';
import { useSession } from '../state/session';
import { useRecentSearches, useSearch } from '../services/queries';
import { api } from '../services/client';
import { MORE, PERSONAL, PRIMARY } from './nav';
import { useCurrentProfile } from './ProfileMenu';
import { useLockBody } from '../lib/hooks';
import { cls } from '../components/ui';
import { toast } from '../components/overlay';

interface Cmd {
  id: string;
  label: string;
  hint?: string;
  icon?: ReactNode;
  image?: string;
  shape?: string;
  group: string;
  keywords?: string;
  run: () => void;
}

export function CommandPalette() {
  const open = useUi((s) => s.palette);
  const initial = useUi((s) => s.paletteQuery);
  const close = useUi((s) => s.closePalette);
  if (!open) return null;
  return createPortal(<PaletteBody initial={initial} onClose={close} />, document.body);
}

function PaletteBody({ initial, onClose }: { initial: string; onClose: () => void }) {
  const [q, setQ] = useState(initial);
  const [debounced, setDebounced] = useState(initial);
  const [sel, setSel] = useState(0);
  const navigate = useNavigate();
  const qc = useQueryClient();
  const me = useCurrentProfile();
  const admin = me?.role === 'owner' || me?.role === 'admin';
  const setTransparency = useSession((s) => s.setTransparency);
  const transparency = useSession((s) => s.transparency);
  const switchProfile = useSession((s) => s.switchProfile);
  const signOut = useSession((s) => s.signOut);
  const recent = useRecentSearches();
  const results = useSearch(debounced);
  const list = useRef<HTMLDivElement>(null);
  useLockBody(true);

  useEffect(() => {
    const t = setTimeout(() => setDebounced(q), 110);
    return () => clearTimeout(t);
  }, [q]);

  const go = (to: string) => () => {
    onClose();
    navigate(to, { viewTransition: true });
  };

  const commands: Cmd[] = useMemo(() => {
    const nav: Cmd[] = [...PRIMARY, ...MORE, ...PERSONAL].map((n) => ({ id: `nav-${n.id}`, label: n.label, icon: <n.icon />, group: 'Go to', run: go(n.to), hint: 'Page' }));
    nav.push({ id: 'nav-search', label: 'Search', icon: <Search />, group: 'Go to', run: go('/search'), hint: 'Page' });
    const settings: Cmd[] = [
      { id: 'set-account', label: 'Account', keywords: 'email name password', icon: <Settings />, group: 'Settings', run: go('/settings') },
      { id: 'set-profiles', label: 'Profiles & Parental Controls', keywords: 'kids pin rating restrictions', icon: <UserRoundCog />, group: 'Settings', run: go('/settings/profiles') },
      { id: 'set-playback', label: 'Playback & Quality', keywords: 'subtitles audio language autoplay quality', icon: <Tv2 />, group: 'Settings', run: go('/settings/playback') },
      { id: 'set-security', label: 'Sign-In & Security', keywords: 'passkey 2fa mfa password devices', icon: <KeyRound />, group: 'Settings', run: go('/settings/security') },
      { id: 'set-downloads', label: 'Downloads & Offline', icon: <Download />, group: 'Settings', run: go('/settings/downloads') },
      { id: 'set-appearance', label: 'Appearance & Accessibility', keywords: 'transparency motion glass', icon: <Eye />, group: 'Settings', run: go('/settings/appearance') },
      {
        id: 'toggle-transparency',
        label: transparency === 'reduced' ? 'Turn glass effects back on' : 'Reduce transparency',
        icon: <Eye />,
        group: 'Actions',
        run: () => {
          setTransparency(transparency === 'reduced' ? 'system' : 'reduced');
          onClose();
        },
      },
      {
        id: 'switch-profile',
        label: 'Switch profile',
        icon: <UserRoundCog />,
        group: 'Actions',
        run: () => {
          onClose();
          switchProfile();
          navigate('/profiles');
        },
      },
      {
        id: 'sign-out',
        label: 'Sign out',
        icon: <LogOut />,
        group: 'Actions',
        run: () => {
          onClose();
          signOut();
          qc.clear();
          navigate('/signin');
        },
      },
    ];
    const consoleCmds: Cmd[] = admin
      ? [
          ['overview', 'Server overview', <LayoutDashboard />, 'dashboard cpu status'],
          ['sessions', 'Now playing', <Activity />, 'streams sessions transcode'],
          ['libraries', 'Libraries & scanning', <Server />, 'scan folders'],
          ['users', 'Users & access', <Users />, 'invite share friends'],
          ['transcoding', 'Transcoding', <Gauge />, 'hardware quick sync'],
          ['network', 'Network & remote access', <Network />, 'port domain certificate'],
          ['storage', 'Storage', <HardDrive />, 'disks smart volumes'],
          ['backups', 'Backups', <History />, 'restore'],
          ['tasks', 'Scheduled tasks', <ListChecks />, 'cron jobs'],
          ['livetv', 'Live TV sources', <Tv2 />, 'tuner hdhomerun epg xmltv'],
          ['metadata', 'Metadata tools', <Tags />, 'fix match unmatched duplicates'],
          ['logs', 'Logs', <ScrollText />, 'errors debug'],
          ['diagnostics', 'Diagnostics', <Stethoscope />, 'health check speed test'],
        ].map(([id, label, icon, kw]) => ({ id: `con-${id}`, label: label as string, icon: icon as ReactNode, keywords: kw as string, group: 'Server Console', hint: 'Console', run: go(id === 'overview' ? '/console' : `/console/${id}`) }))
      : [];
    if (admin) {
      consoleCmds.push({
        id: 'scan-all',
        label: 'Scan all libraries now',
        icon: <RefreshCw />,
        group: 'Actions',
        run: () => {
          api.scanLibrary('all');
          onClose();
          toast('Scanning all libraries on Harbor', <RefreshCw />);
        },
      });
      consoleCmds.push({ id: 'optimize', label: 'Run a scheduled task…', icon: <Wand2 />, group: 'Actions', run: go('/console/tasks') });
    }
    return [...nav, ...settings, ...consoleCmds];
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [admin, transparency]);

  const needle = q.trim().toLowerCase();
  const filtered = needle ? commands.filter((c) => `${c.label} ${c.keywords ?? ''} ${c.group}`.toLowerCase().includes(needle)) : commands.filter((c) => c.group === 'Go to' || c.group === 'Actions');

  const content: Cmd[] = needle
    ? (results.data?.groups ?? []).flatMap((g) =>
        g.hits.slice(0, g.kind === 'movie' || g.kind === 'show' ? 4 : 3).map((h) => ({
          id: `hit-${h.kind}-${h.id}`,
          label: h.title,
          hint: h.subtitle,
          image: h.image,
          shape: h.shape,
          group: g.label,
          run: go(h.href),
        })),
      )
    : [];
  const recentCmds: Cmd[] = !needle
    ? (recent.data ?? []).slice(0, 4).map((r) => ({ id: `recent-${r}`, label: r, icon: <History />, group: 'Recent searches', run: () => setQ(r) }))
    : [];
  const all = [...recentCmds, ...content, ...filtered];
  const groups = all.reduce<Record<string, Cmd[]>>((acc, c) => ((acc[c.group] ??= []).push(c), acc), {});
  const flat = Object.values(groups).flat();

  useEffect(() => setSel(0), [debounced, needle]);
  useEffect(() => {
    list.current?.querySelector(`[data-i="${sel}"]`)?.scrollIntoView({ block: 'nearest' });
  }, [sel]);

  const onKey = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.preventDefault();
      onClose();
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSel((s) => Math.min(flat.length - 1, s + 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSel((s) => Math.max(0, s - 1));
    } else if (e.key === 'Enter') {
      e.preventDefault();
      if (flat[sel]) flat[sel].run();
      else if (needle) go(`/search?q=${encodeURIComponent(q)}`)();
    }
  };

  let i = -1;
  return (
    <div className="palette-root" onKeyDown={onKey}>
      <div className="palette-scrim" onClick={onClose} />
      <div className="palette glass glass-thick" role="dialog" aria-label="Search and commands">
        <div className="palette-input">
          <Search />
          <input
            autoFocus
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Search Harbor, jump to a page, or run a command"
            aria-label="Search"
            spellCheck={false}
            autoComplete="off"
          />
          {results.isFetching && needle && <span className="spinner" style={{ width: 16, height: 16 }} />}
          <kbd>esc</kbd>
        </div>
        <div className="palette-list" ref={list}>
          {needle && !results.isFetching && flat.length === 0 && (
            <div className="palette-empty">
              No results for “{q}”. Press <kbd>↵</kbd> to search everything.
            </div>
          )}
          {Object.entries(groups).map(([g, items]) => (
            <div key={g} className="palette-group">
              <div className="palette-group-label">{g}</div>
              {items.map((c) => {
                i++;
                const idx = i;
                return (
                  <button key={c.id} data-i={idx} className={cls('palette-item', idx === sel && 'sel')} onMouseMove={() => setSel(idx)} onClick={c.run}>
                    {c.image ? <img src={c.image} alt="" className={cls('palette-thumb', c.shape)} loading="lazy" /> : <span className="palette-icon">{c.icon ?? <ArrowRight />}</span>}
                    <span className="palette-label ellipsis">{c.label}</span>
                    {c.hint && <span className="palette-hint ellipsis">{c.hint}</span>}
                    {idx === sel && <CornerDownLeft className="palette-enter" />}
                  </button>
                );
              })}
            </div>
          ))}
        </div>
        <div className="palette-foot">
          <span>
            <kbd>↑</kbd>
            <kbd>↓</kbd> to move
          </span>
          <span>
            <kbd>↵</kbd> to open
          </span>
          <span>
            <kbd>g</kbd> then <kbd>m</kbd> jumps to Movies
          </span>
          {results.data && needle && <span className="palette-took">{results.data.total} results · {results.data.tookMs} ms</span>}
        </div>
      </div>
    </div>
  );
}
