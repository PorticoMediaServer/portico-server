import React, {Suspense, lazy, useEffect, useMemo, useRef, useState} from 'react';
import {Link, Outlet, useLocation, useNavigate} from '@tanstack/react-router';
import {InboxProvider, useUnreadCount} from '../app/inbox';
import {useI18n} from '../app/i18n';
import {currentAccessToken, sessionIdentity, useSession} from '../app/session';
import {LibrariesContext, useLibraries, libraryKindLabel} from '../app/libraries';
import {useViewerScope} from '../app/viewer-scope';
import {NotInterestedNotice} from '../app/not-interested-notice';
import {useSidebarScans} from '../app/library-scan';
import {usePreferences} from '../app/preferences';
import {ArtworkContext, ArtworkStore, Avatar, BrandMark, Button, cx, Dialog, Icon, IconButton, ListRow, Menu, Popover, Text, Tooltip, useCompact, useHoverCapable, useNarrow, Wordmark, type IconName} from '../ui';
import {iconFor} from '@core/presentation/index.ts';
import {PlayerProvider, usePlayer} from '../player/engine';
import {ServerPreferencesProvider, useServerPreferences} from '../app/server-preferences';
import {ConnectionOverlay} from './Connection';
import {CustodyPrompt} from './CustodyPrompt';
import {SelectionProvider, useSelectionActive} from '../app/selection';
import {MetadataEditorProvider, useMetadataEditor} from '../app/metadata-editor';
import {DeleteMediaProvider} from '../app/delete-media';
import {DownloadsProvider, useDownloads} from '../app/downloads';
import {TogetherProvider, useInRoom} from '../app/together';
import {CastProvider} from '../app/cast';
import {ChannelSourcesProvider, channelEntries, readReminders, subscribeReminders, useChannelSources} from '../app/channels';
import {artworkMetricsEnabled, logScreen, useArtworkScreens} from '../app/artwork-metrics';
import s from './Shell.module.css';

// Bundle size (PERF): the player UI, the entry actions sheet, the selection bar
// and the metadata editor load on first use. The player and actions chunks are fetched when the browser is idle after first paint, so
// pressing Play doesn't wait for it.
const loadPlayer = () => import('../player/Player');
const loadEntryActions = () => import('../screens/shared/EntryActions');
const SelectionBar = lazy(() => import('../screens/shared/SelectionBar').then(m => ({default: m.SelectionBar})));
const EntryActionsSheet = lazy(() => loadEntryActions().then(m => ({default: m.EntryActionsSheet})));
const PlayerSurface = lazy(() => loadPlayer().then(m => ({default: m.PlayerSurface})));
const ChannelReminders = lazy(() => import('../screens/channels/Channels').then(m => ({default: m.ChannelReminders})));
const MetadataEditorDialog = lazy(() => import('../screens/shared/MetadataEditor').then(m => ({default: m.MetadataEditorDialog})));

function usePrefetchPlayer() {
  useEffect(() => {
    const w = window as Window & {requestIdleCallback?: (fn: () => void, o?: {timeout: number}) => number; cancelIdleCallback?: (id: number) => void};
    if (w.requestIdleCallback) { const id = w.requestIdleCallback(() => { void loadPlayer(); void loadEntryActions(); }, {timeout: 5000}); return () => w.cancelIdleCallback?.(id); }
    const t = setTimeout(() => { void loadPlayer(); void loadEntryActions(); }, 2000);
    return () => clearTimeout(t);
  }, []);
}

/** Mounts once needed and stays mounted (close animations, the player's reserve cleanup). */
function useOnceTrue(value: boolean) {
  const [seen, setSeen] = useState(value);
  useEffect(() => { if (value) setSeen(true); }, [value]);
  return seen || value;
}

/**
 * WEB-SEARCH-02: "/" anywhere outside a text field opens Search with the field
 * focused (the web convention; Esc still leaves it).
 */
function useSearchShortcut() {
  const navigate = useNavigate();
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== '/' || e.metaKey || e.ctrlKey || e.altKey || e.defaultPrevented) return;
      const t = e.target as HTMLElement | null;
      if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName) || t.closest('[role=dialog],[role=menu]'))) return;
      e.preventDefault();
      const focus = () => document.querySelector<HTMLInputElement>('input[type=search]')?.focus();
      if (location.pathname === '/search') focus();
      else void navigate({to: '/search'}).then(() => requestAnimationFrame(focus));
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [navigate]);
}

/**
 * While the full player covers the page, the page behind it is inert: Tab, a screen reader and
 * stray clicks stay in the player. The attribute is set from an effect, so the page itself does
 * not re-render with the player.
 */
function PageInertUnderPlayer({frame}: {frame: React.RefObject<HTMLDivElement | null>}) {
  const player = usePlayer();
  const covered = player.active && player.expanded;
  useEffect(() => {
    const el = frame.current;
    if (!el) return;
    el.inert = covered;
    return () => { el.inert = false; };
  }, [covered, frame]);
  return null;
}

function LazyPlayerSurface() {
  usePrefetchPlayer();
  const player = usePlayer();
  const needed = useOnceTrue(player.active);
  return needed ? <Suspense fallback={null}><PlayerSurface /></Suspense> : null;
}

function LazyEntryActions() {
  const player = usePlayer();
  const needed = useOnceTrue(!!player.moreEntry);
  return needed ? <Suspense fallback={null}><EntryActionsSheet /></Suspense> : null;
}

function LazySelectionBar() {
  const needed = useOnceTrue(!!useSelectionActive());
  return needed ? <Suspense fallback={null}><SelectionBar /></Suspense> : null;
}

/** Channel reminders fire as a toast while Portico is open (Spec — Channels and Guide §3.5); the chunk loads only when one is set. */
function LazyReminders() {
  const count = React.useSyncExternalStore(subscribeReminders, () => readReminders().length);
  const needed = useOnceTrue(count > 0);
  return needed ? <Suspense fallback={null}><ChannelReminders /></Suspense> : null;
}

function LazyMetadataEditor() {
  const editor = useMetadataEditor();
  const needed = useOnceTrue(!!editor?.request);
  return needed ? <Suspense fallback={null}><MetadataEditorDialog /></Suspense> : null;
}

/**
 * Authenticated frame. A left rail on wide viewports (collapsible to icons),
 * a bottom tab bar on phones. The rail switches roots; the router owns the
 * stack. Player surfaces mount above the content later in the tree.
 */
export function Shell() {
  const {t} = useI18n();
  const session = useSession();
  const compact = useCompact();
  const narrow = useNarrow();
  const {preferences, update} = usePreferences();
  // Keyed on who is signed in, not the access token, which rotates every ~13 minutes.
  const who = sessionIdentity(session.session);
  const libraries = useLibraries(session.api, who || undefined);
  const metrics = useMemo(artworkMetricsEnabled, []);
  const artwork = useMemo(() => new ArtworkStore(session.api, currentAccessToken, metrics ? {onScreenSettled: logScreen} : {}), [session.api, who, metrics]);
  useArtworkScreens(artwork, metrics);
  useSearchShortcut();
  // A replaced store must release its object URLs, or every poster it ever
  // resolved stays alive for the life of the document.
  useEffect(() => () => artwork.dispose(), [artwork]);
  // WEB-RESP-02: tablet widths start collapsed but can be expanded (the rail pushes the page,
  // it never covers it); a device without hover starts expanded, since tooltips need a pointer.
  const hover = useHoverCapable();
  const [narrowExpanded, setNarrowExpanded] = useState<boolean>();
  const collapsed = narrow ? !(narrowExpanded ?? !hover) : !preferences.railExpanded;
  const frame = useRef<HTMLDivElement>(null);
  return (
    <LibrariesContext.Provider value={libraries}>
    <InboxProvider>
      <ArtworkContext.Provider value={artwork}>
      <ServerPreferencesProvider>
      <AppearanceBridge />
      <PlayerProvider>
      <SelectionProvider>
      <MetadataEditorProvider>
      <DeleteMediaProvider>
      <CastProvider>
      <TogetherProvider>
      <DownloadsProvider>
      <ChannelSourcesProvider>
        <PageInertUnderPlayer frame={frame} />
        <div ref={frame} className={cx(s.frame, collapsed && s.collapsed)}>
          {/* The card keyboard shortcuts, described once for assistive technology (cards reference it). */}
          <span id="card-keyboard-hint" className="visually-hidden">{t('card.keyboardHint')}</span>
          {!compact ? <Rail collapsed={collapsed} onToggle={narrow ? () => setNarrowExpanded(collapsed) : () => update({railExpanded: !preferences.railExpanded})} /> : null}
          <div className={s.main}>
            {compact ? <TopBar /> : null}
            <div className={s.content}>
              <CustodyPrompt />
              <NotInterestedNotice />
              <ConnectionOverlay><Outlet /></ConnectionOverlay>
            </div>
          </div>
          {compact ? <BottomBar /> : null}
        </div>
        <LazyPlayerSurface />
        <LazyEntryActions />
        <LazySelectionBar />
        <LazyMetadataEditor />
        <LazyReminders />
      </ChannelSourcesProvider>
      </DownloadsProvider>
      </TogetherProvider>
      </CastProvider>
      </DeleteMediaProvider>
      </MetadataEditorProvider>
      </SelectionProvider>
      </PlayerProvider>
      </ServerPreferencesProvider>
      </ArtworkContext.Provider>
    </InboxProvider>
    </LibrariesContext.Provider>
  );
}

/** Mirrors the server's appearance preferences into the browser-local ones the
 * shell reads synchronously, so a profile's choices follow it between browsers.
 * X-01: `appearance.showBackdrops` is published on the document root as
 * `data-backdrops="off"`; `ui/Backdrop.tsx` (M2 owner; see Questions) renders
 * nothing but the atmosphere when it is off. */
function AppearanceBridge() {
  const server = useServerPreferences();
  const {update} = usePreferences();
  const reduce = server.value('appearance.reduceMotion', false);
  const size = server.value('appearance.cardSizePercent', 100);
  const backdrops = server.value('appearance.showBackdrops', true);
  useEffect(() => {
    if (!server.snapshot) return;
    update({reduceMotion: reduce, posterSize: size < 90 ? 'compact' : size > 115 ? 'large' : 'regular'});
  }, [server.snapshot, reduce, size, update]);
  useEffect(() => {
    if (!server.snapshot) return;
    if (backdrops) delete document.documentElement.dataset.backdrops;
    else document.documentElement.dataset.backdrops = 'off';
  }, [server.snapshot, backdrops]);
  return null;
}

function RailLink({to, params, icon, label, exact, collapsed, count, live, activeWhen, badge, scanning, scanCount}: {to: string; params?: Record<string, string>; icon: IconName; label: string; exact?: boolean; collapsed: boolean; count?: number; live?: boolean; /** Replaces the router's prefix match (WEB-SET-04: one selected item per route). */ activeWhen?: (pathname: string) => boolean; /** Library Channels carry their own mark (§2.0a). */ badge?: 'library'; /** A library scan is running: the count pill carries the found files. */ scanning?: boolean; scanCount?: string}) {
  const {t} = useI18n();
  const pathname = useLocation().pathname;
  const forced = activeWhen ? activeWhen(pathname) : undefined;
  const link = (
    <Link to={to} params={params} className={s.railItem} activeOptions={{exact: forced !== undefined ? true : exact}} {...(forced ? {'data-status': 'active', 'aria-current': 'page' as const} : {})} aria-label={collapsed || count || live || scanning ? (count ? t('web.shell.unread', {label, count}) : live ? t('web.shell.inGroup', {label}) : scanning ? t('web.shell.scanning', {label}) : label) : undefined}>
      <Icon name={icon} size={20} />
      <span className={s.railLabel}>{label}</span>
      {badge && !collapsed ? <span className={s.railBadge} aria-hidden><Icon name="library" size={11} /></span> : null}
      {count ? <span className={s.railCount} aria-hidden>{count > 99 ? '99+' : count}</span> : live ? <span className={s.railCount} aria-hidden>{t('web.shell.groupOn')}</span> : scanning && scanCount ? <span className={s.railCount} aria-hidden>{scanCount}</span> : null}
    </Link>
  );
  return collapsed ? <Tooltip label={scanning ? t('web.shell.scanning', {label}) : label}>{link}</Tooltip> : link;
}

/**
 * Notifications are a bell in the shell, not a destination: the panel opens over the page, so
 * reading a message never leaves where you were. Its own component so only the badge re-renders
 * when the count changes. X-01: the badge hides when the viewer turned `notifications.badges` off.
 */
function NotificationsBell({collapsed}: {collapsed: boolean}) {
  const {t} = useI18n();
  const unread = useUnreadCount();
  const badges = useServerPreferences().value('notifications.badges', true);
  const count = badges ? unread : 0;
  const [open, setOpen] = useState(false);
  const label = t('web.shell.notifications');
  const trigger = (
    <button type="button" className={s.railItem} aria-label={count ? t('web.shell.unread', {label, count}) : label} aria-expanded={open}>
      <Icon name="bell" size={20} />
      <span className={s.railLabel}>{label}</span>
      {count ? <span className={s.railCount} aria-hidden>{count > 99 ? '99+' : count}</span> : null}
    </button>
  );
  return (
    <Popover open={open} onOpenChange={setOpen} side="right" align="end" trigger={collapsed ? <Tooltip label={label}>{trigger}</Tooltip> : trigger}>
      <div className={s.bellPanel}>{open ? <Suspense fallback={null}><NotificationsPanel /></Suspense> : null}</div>
    </Popover>
  );
}
const NotificationsPanel = lazy(() => import('../screens/settings/Notifications').then(m => ({default: m.NotificationsSection})));

/** A Watch Together group in progress shows in the rail; starting or joining one happens from a title or the player. */
function TogetherLink({collapsed}: {collapsed: boolean}) {
  const {t} = useI18n();
  const inRoom = useInRoom();
  if (!inRoom) return null;
  return <RailLink to="/together" icon="people" label={t('web.shell.together')} collapsed={collapsed} live />;
}

/**
 * Channels in the rail (Spec — Channels and Guide §2.0, §7): a heading under the libraries that
 * opens Channels, then All channels (2+ sources), each source by its owner's name, and Recordings.
 * Hidden while there are no channel sources.
 */
function ChannelsRail({collapsed}: {collapsed: boolean}) {
  const {t} = useI18n();
  const {sources} = useChannelSources();
  const entries = channelEntries(sources);
  if (!entries.length) return null;
  return (
    <>
      <Link to="/channels" className={s.railHeading} style={{textDecoration: 'none'}}>{t('channels.title')}</Link>
      <div className={s.railGroup}>
        {entries.map(e => <RailLink key={e.id} to="/channels/$entry" params={{entry: e.id}} icon={e.icon} label={e.name} collapsed={collapsed} badge={e.library ? 'library' : undefined} />)}
      </div>
    </>
  );
}

function Rail({collapsed, onToggle}: {collapsed: boolean; onToggle?: () => void}) {
  const i18n = useI18n();
  const {t} = i18n;
  const session = useSession();
  const libraries = React.useContext(LibrariesContext);
  const scope = useViewerScope();
  // ONB-04: a found-files pill on each library that is scanning (viewer-safe status, polled).
  const scans = useSidebarScans(session.api, scope.serverId, libraries.items.map(l => l.id));
  return (
    <nav className={s.rail} aria-label={t('web.shell.primary')}>
      <Link to="/" className={s.brand} aria-label={t('web.shell.homeLabel')}>
        <BrandMark size={26} />
        <span><Wordmark height={16} /></span>
      </Link>
      <div className={s.railGroup}>
        <RailLink to="/" icon="home" label={t('web.shell.home')} exact collapsed={collapsed} />
        <RailLink to="/search" icon="search" label={t('web.shell.search')} collapsed={collapsed} />
        <RailLink to="/saved" icon="saved" label={t('web.shell.saved')} collapsed={collapsed} />
        <TogetherLink collapsed={collapsed} />
      </div>
      <div className={s.railScroll}>
        <div className={s.railHeading}>{t('web.shell.libraries')}</div>
        <div className={s.railGroup}>
          {libraries.items.map(l => (
            <RailLink key={l.id} to="/library/$libraryId" params={{libraryId: l.id}} icon={iconFor(l.kind)} label={l.name} collapsed={collapsed} scanning={scans.has(l.id)} scanCount={scans.has(l.id) ? i18n.number(scans.get(l.id) ?? 0) : undefined} />
          ))}
          {!libraries.items.length && !libraries.loading ? (session.owner
            ? <RailLink to="/settings/$section" params={{section: 'server-libraries'}} icon="plus" label={t('web.shell.addLibrary')} collapsed={collapsed} />
            : <Text as="p" variant="caption" tone="tertiary" style={{padding: '4px 12px'}}>{collapsed ? '' : t('web.shell.noLibraries')}</Text>) : null}
        </div>
        <ChannelsRail collapsed={collapsed} />
      </div>
      <div className={s.railFoot}>
        <NotificationsBell collapsed={collapsed} />
        <RailLink to="/settings" icon="settings" label={t('web.shell.settings')} collapsed={collapsed} activeWhen={p => p.startsWith('/settings')} />
        <ProfileMenu collapsed={collapsed} />
        {onToggle ? <IconButton name={collapsed ? 'forward' : 'back'} label={collapsed ? t('web.shell.expand') : t('web.shell.collapse')} variant="ghost" size="sm" className={s.collapseToggle} onClick={onToggle} /> : null}
      </div>
    </nav>
  );
}

export function useProfileName(): string {
  const session = useSession();
  const {t} = useI18n();
  const id = session.session?.viewer.profileId;
  return session.hosted?.profiles.find(p => p.id === id)?.name ?? session.local?.profiles.find(p => p.id === id)?.name ?? session.hosted?.account.displayName ?? session.local?.username ?? t('web.shell.profileFallback');
}

/** WEB-SYS-10: one colour system for profile avatars, the profile's own art colour (`--profile-art-*`).
 * Follow-up: the rail uses the shared `ui/Avatar` primitive (size 24) like
 * Settings › Account and Server › People; the ad-hoc span is gone. */
const ART = ['blue', 'violet', 'mint', 'coral', 'gold', 'slate', 'rose', 'sky'];
/** The signed-in profile's art colour: its own when the server says, else stable from its id. */
export function useProfileArt(): string {
  const session = useSession();
  const id = session.session?.viewer.profileId ?? '';
  const own = session.local?.profiles.find(p => p.id === id)?.art;
  if (own && ART.includes(own)) return own;
  let h = 0;
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) | 0;
  return ART[Math.abs(h) % ART.length]!;
}

const HostedChooser = lazy(() => import('../screens/auth/HostedChooser').then(m => ({default: m.HostedChooser})));
const DirectProfiles = lazy(() => import('../screens/auth/DirectProfiles').then(m => ({default: m.DirectProfiles})));
const ServerSwitcher = lazy(() => import('./ServerSwitcher').then(m => ({default: m.ServerSwitcher})));
const FeedbackDialog = lazy(() => import('../screens/shared/FeedbackDialog').then(m => ({default: m.FeedbackDialog})));

/**
 * WEB-SHELL-02: the profile menu says who and where you are (profile, server, account), and
 * Switch profile opens the chooser here instead of sending you to Settings. Settings is
 * already in the rail directly above, so it isn't repeated.
 */
function ProfileMenu({collapsed}: {collapsed: boolean}) {
  const {t} = useI18n();
  const session = useSession();
  const navigate = useNavigate();
  const name = useProfileName();
  const art = useProfileArt();
  const hostedAuthority = session.session?.viewer.authority === 'hosted';
  const [switching, setSwitching] = useState(false);
  const [feedback, setFeedback] = useState(false);
  const [switchingServer, setSwitchingServer] = useState(false);
  const server = session.attachedServer?.name ?? session.system?.name ?? '';
  const account = session.hosted?.account.username ?? session.local?.username ?? '';
  const where = [server, hostedAuthority ? account : t('web.shell.localAccount')].filter(Boolean).join(' · ');
  const downloads = useDownloads();
  const items = [
    {id: 'switch', label: t('web.shell.switchProfile'), icon: 'switch' as IconName},
    // The Portico Account's servers and this browser's direct sign-ins (the account stays signed in).
    {id: 'switchServer', label: t('web.shell.switchServer'), icon: 'server' as IconName},
    {id: 'account', label: t('web.shell.account'), icon: 'account' as IconName},
    // Files the server prepared for this browser: a list you visit now and then, not a place in the rail.
    ...(downloads.state.unavailable ? [] : [{id: 'downloads', label: t('web.shell.downloads'), icon: 'download' as IconName, separatorBefore: true}]),
    {id: 'feedback', label: t('web.shell.feedback'), icon: 'flag' as IconName, separatorBefore: downloads.state.unavailable},
    {id: 'help', label: t('web.shell.help'), icon: 'external' as IconName},
    {id: 'signOut', label: t('action.signOut'), icon: 'signOut' as IconName, separatorBefore: true},
  ];
  return (
    <>
      <Menu
        label={t('web.shell.profile')}
        align="start"
        header={<><Avatar name={name} art={art} size={40} /><span style={{display: 'flex', flexDirection: 'column', minWidth: 0}}><Text as="span" variant="bodyStrong">{name}</Text>{where ? <Text as="span" variant="caption" tone="tertiary">{where}</Text> : null}</span></>}
        trigger={
          <button type="button" className={cx(s.railItem, s.avatarItem)} aria-label={collapsed ? name : undefined}>
            <Avatar name={name} art={art} size={24} />
            <span className={s.railLabel}>{name}</span>
          </button>
        }
        items={items}
        onSelect={id => {
          // Settings › Account has its own chooser dialogs; there, reuse them rather than stacking a second.
          if (id === 'switch') { if (hostedAuthority) setSwitching(true); else { session.direct.openProfileChooser(); setSwitching(true); } }
          else if (id === 'switchServer') setSwitchingServer(true);
          else if (id === 'account') void navigate({to: '/settings/$section', params: {section: 'profile'}, search: {}});
          else if (id === 'downloads') void navigate({to: '/downloads'});
          else if (id === 'feedback') setFeedback(true);
          else if (id === 'help') window.open('https://getportico.tv/help', '_blank', 'noopener');
          else if (id === 'signOut') session.signOut();
        }}
      />
      {switching && hostedAuthority ? (
        <Dialog open onOpenChange={setSwitching} title={t('web.account.switchProfileOrServer')} width={560}>
          <Suspense fallback={null}><HostedChooser onDone={() => setSwitching(false)} preferredServerId={session.session?.viewer.serverId} /></Suspense>
        </Dialog>
      ) : null}
      {switching && !hostedAuthority && session.direct.pending ? (
        <Dialog open onOpenChange={o => { if (!o) { session.direct.cancelPending(); setSwitching(false); } }} title={t('settings.switchProfile')} width={560}>
          <Suspense fallback={null}><DirectProfiles onDone={() => { session.direct.cancelPending(); setSwitching(false); }} /></Suspense>
        </Dialog>
      ) : null}
      {feedback ? <Suspense fallback={null}><FeedbackDialog open onOpenChange={setFeedback} /></Suspense> : null}
      {switchingServer ? <Suspense fallback={null}><ServerSwitcher onClose={() => setSwitchingServer(false)} /></Suspense> : null}
    </>
  );
}

function TopBell() {
  const {t} = useI18n();
  const unread = useUnreadCount();
  // X-01: no dot when the viewer turned `notifications.badges` off.
  const badges = useServerPreferences().value('notifications.badges', true);
  const shown = badges ? unread : 0;
  const [open, setOpen] = useState(false);
  const label = t('web.shell.notifications');
  return (
    <Popover open={open} onOpenChange={setOpen} align="end" trigger={
      <button type="button" className={s.topBell} aria-label={shown ? t('web.shell.unread', {label, count: shown}) : label} aria-expanded={open}>
        <Icon name="bell" size={20} />
        {shown ? <span className={s.topBellDot} aria-hidden /> : null}
      </button>
    }>
      <div className={s.bellPanel}>{open ? <Suspense fallback={null}><NotificationsPanel /></Suspense> : null}</div>
    </Popover>
  );
}

/** Phones: the top bar carries Search on every page (the standard top rail), the bell and the profile. */
function TopBar() {
  const {t} = useI18n();
  const name = useProfileName();
  const art = useProfileArt();
  const navigate = useNavigate();
  return (
    <header className={s.topBar}>
      <Link to="/" aria-label={t('web.shell.homeLabel')}><Wordmark height={18} /></Link>
      <div className={s.topActions}>
        <Button icon="search" aria-label={t('web.shell.search')} variant="ghost" to="/search" />
        <TopBell />
        <button type="button" onClick={() => void navigate({to: '/settings'})} aria-label={t('web.shell.profileSettings', {name})} style={{padding: 4}}><Avatar name={name} art={art} size={24} /></button>
      </div>
    </header>
  );
}

/** Phones: Home · Library · Channels · Saved · Downloads, the same set as the iPhone app. Channels and Downloads appear when the server has them. */
function BottomBar() {
  const {t} = useI18n();
  const libraries = React.useContext(LibrariesContext);
  const location = useLocation();
  const navigate = useNavigate();
  const [picker, setPicker] = useState(false);
  const inLibrary = location.pathname.startsWith('/library/');
  const single = libraries.items.length === 1 ? libraries.items[0] : undefined;
  const hasChannels = useChannelSources().sources.length > 0;
  const hasDownloads = !useDownloads().state.unavailable;
  const tabs = 3 + (hasChannels ? 1 : 0) + (hasDownloads ? 1 : 0);
  return (
    <>
      <nav className={s.bottomBar} aria-label={t('web.shell.primary')} style={{gridTemplateColumns: `repeat(${tabs}, 1fr)`}}>
        <Link to="/" className={s.tab} activeOptions={{exact: true}}><Icon name="home" size={22} />{t('web.shell.home')}</Link>
        <button type="button" className={s.tab} data-status={inLibrary ? 'active' : undefined} onClick={() => (single ? void navigate({to: '/library/$libraryId', params: {libraryId: single.id}}) : setPicker(true))}><Icon name="library" size={22} />{t('web.shell.library')}</button>
        {hasChannels ? <Link to="/channels" className={s.tab}><Icon name="channels" size={22} />{t('channels.title')}</Link> : null}
        <Link to="/saved" className={s.tab}><Icon name="saved" size={22} />{t('web.shell.saved')}</Link>
        {hasDownloads ? <Link to="/downloads" className={s.tab}><Icon name="download" size={22} />{t('web.shell.downloads')}</Link> : null}
      </nav>
      <Dialog open={picker} onOpenChange={setPicker} title={t('web.shell.libraries')}>
        <div className={s.libraryPicker}>
          {libraries.items.map(l => {
            // WEB-RESP-01: the kind only when it adds something ("Kids" · Movies), not "Movies / Movies".
            const kind = libraryKindLabel(l.kind);
            return <ListRow key={l.id} icon={iconFor(l.kind)} title={l.name} subtitle={kind.toLocaleLowerCase() === l.name.toLocaleLowerCase() ? undefined : kind} selected={location.pathname === `/library/${l.id}`} trailingIcon="forward" onClick={() => { setPicker(false); void navigate({to: '/library/$libraryId', params: {libraryId: l.id}}); }} />;
          })}
          {!libraries.items.length ? <Text as="p" variant="body" tone="secondary">{t('web.shell.noLibrariesShared')}</Text> : null}
        </div>
        <Button variant="ghost" label={t('web.shell.close')} onClick={() => setPicker(false)} />
      </Dialog>
    </>
  );
}
