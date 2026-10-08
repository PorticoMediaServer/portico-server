import React, {useState} from 'react';
import {Link, useNavigate, useParams} from '@tanstack/react-router';
import {useDataVersion, useQuery, useServices} from '../../data/hooks';
import type {PreferenceValue} from '../../data/services';
import {useSession} from '../../app/session';
import {usePrefs} from '../../app/prefs';
import {setLatencyFactor, latencyFactor} from '../../data/latency';
import {Page, PageHeader, Panel, Row, KeyValue} from '../../ui/Layout';
import {Button} from '../../ui/Button';
import {Segmented, Select, Slider, Switch} from '../../ui/Controls';
import {Badge, ErrorState, Notice, Skeleton, toast} from '../../ui/Feedback';
import {Icon, type IconName} from '../../ui/Icon';
import {Avatar, BrandSymbol} from '../../ui/Media';
import {ConfirmDialog} from '../../ui/Overlay';
import {ProfilesSection, SecuritySection, DevicesSection} from './Profiles';
import {NotificationsInbox} from './Notifications';

/** Settings: this profile's preferences, account and profiles, servers, notifications and About. */
export function SettingsScreen() {
  const session = useSession();
  const navigate = useNavigate();
  const [signOut, setSignOut] = useState(false);
  const tile = (to: string, icon: IconName, label: string, help: string) => (
    <Link to={to as never} className="set-tile"><span className="set-icon"><Icon name={icon} size={22} /></span><span className="set-text"><strong>{label}</strong><span>{help}</span></span><Icon name="chevronRight" size={18} className="row-chevron" /></Link>
  );
  return (
    <Page title="Settings" className="settings-page">
      <PageHeader title="Settings" />
      <div className="set-who">
        <Link to="/settings/$section" params={{section: 'account'}} className="set-profile">
          <Avatar name={session.profile?.name ?? '?'} color={session.profile?.color} size={64} />
          <span className="set-text"><strong>{session.profile?.name}</strong><span>{session.via === 'account' ? 'Signed in with your Portico Account · justin' : 'Signed in directly to this server'}</span></span>
          <Icon name="chevronRight" size={18} className="row-chevron" />
        </Link>
        <Link to="/settings/$section" params={{section: 'servers'}} className="set-profile">
          <span className="set-server"><BrandSymbol size={30} /></span>
          <span className="set-text"><strong>{session.server?.name}</strong><span>{session.server?.host} · {session.via === 'account' ? 'Through your Portico Account' : 'Direct connection'}</span></span>
          <Icon name="chevronRight" size={18} className="row-chevron" />
        </Link>
      </div>
      <div className="set-grid">
        {tile('/settings/preferences', 'sliders', 'Preferences', 'Playback, quality, subtitles, language and appearance')}
        {tile('/settings/profiles', 'people', 'Profiles', 'Who watches here, PINs and limits')}
        {tile('/settings/notifications', 'bell', 'Notifications', 'Your inbox and what you’re told about')}
        {tile('/downloads?tab=settings', 'download', 'Downloads', 'Quality, storage and Wi-Fi')}
        {tile('/together', 'people', 'Watch Together', 'Start or join a group')}
        {tile('/device', 'tv', 'Link a TV', 'Enter the code a TV shows you')}
        {tile('/account', 'account', 'Portico Account', 'Security, sign-ins, servers and invitations')}
        {tile('/settings/about', 'info', 'About', 'Versions, legal and help')}
      </div>
      {session.manager ? (
        <Panel className="set-console">
          <div className="set-console-row">
            <span className="set-icon is-accent"><Icon name="server" size={22} /></span>
            <span className="set-text"><strong>You manage {session.server?.name}</strong><span>Libraries, people, Live TV, backups and everything else about the server live in the console.</span></span>
            <Button variant="primary" icon="server" label="Open the server console" onClick={() => void navigate({to: '/server/$section', params: {section: 'overview'}})} />
          </div>
        </Panel>
      ) : null}
      <div className="hstack" style={{marginTop: 24}}>
        <Button variant="danger" icon="signOut" label="Sign out" onClick={() => setSignOut(true)} />
      </div>
      <ConfirmDialog open={signOut} onOpenChange={setSignOut} title="Sign out of Portico?" body="You’ll need your Portico Account to sign in again on this browser. Downloads stay until you remove them." confirmLabel="Sign out" onConfirm={() => { session.signOut(); void navigate({to: '/welcome/$screen', params: {screen: 'signin'}}); }} />
    </Page>
  );
}

export function SettingsSectionScreen() {
  const {section} = useParams({from: '/app/settings/$section'});
  switch (section) {
    case 'preferences': return <PreferencesScreen />;
    case 'account': return <Section title="Account" lede="Your profile on this server, and signing in."><AccountSection /></Section>;
    case 'profiles': return <Section title="Profiles" lede="Everyone who watches here gets their own Home, history and recommendations."><ProfilesSection /></Section>;
    case 'servers': return <Section title="Servers" lede="The Portico servers you can use from this browser."><ServersSection /></Section>;
    case 'notifications': return <Section title="Notifications" lede="What’s new on your servers, and replies to your reports."><NotificationsInbox /></Section>;
    case 'about': return <Section title="About"><AboutSection /></Section>;
    default: return <SettingsScreen />;
  }
}

function Section({title, lede, children}: {title: string; lede?: string; children: React.ReactNode}) {
  return (
    <Page title={title} className="settings-page">
      <PageHeader back={{label: 'Settings', to: '/settings'}} title={title} lede={lede} size="title" />
      <div className="settings-col">{children}</div>
    </Page>
  );
}

// ---- Preferences

type Field = {key: string; label: string; help?: string; type: 'switch' | 'select' | 'segmented' | 'slider'; options?: {value: string; label: string}[]; min?: number; max?: number; step?: number; unit?: string};
const secs = (list: number[]) => list.map(v => ({value: String(v), label: `${v} seconds`}));
const skip = [{value: 'ask', label: 'Ask each time'}, {value: 'auto', label: 'Skip automatically'}, {value: 'off', label: 'Never'}];
const quality = [{value: 'original', label: 'Original quality'}, {value: 'automatic', label: 'Automatic'}, {value: 'high', label: 'High'}, {value: 'standard', label: 'Standard'}, {value: 'data-saver', label: 'Data saver'}];
const GROUPS: {id: string; title: string; description?: string; fields: Field[]}[] = [
  {id: 'playback', title: 'Playback', description: 'How video and audio behave while you watch and listen.', fields: [
    {key: 'playback.autoplayNext', label: 'Play next automatically', help: 'Start the next episode when one ends.', type: 'switch'},
    {key: 'playback.upNextCountdownSeconds', label: 'Up Next countdown', help: 'How long the next episode waits before starting.', type: 'select', options: [{value: '0', label: 'Immediately'}, ...secs([5, 10, 15, 30])]},
    {key: 'playback.passoutProtection', label: 'Still watching check', help: 'Pause autoplay after several episodes with no one using the controls.', type: 'switch'},
    {key: 'playback.passoutAfterEpisodes', label: 'Ask after', help: 'Episodes in a row before Portico checks you’re still watching.', type: 'select', options: [2, 3, 4, 5].map(v => ({value: String(v), label: `${v} episodes`}))},
    {key: 'playback.introSkip', label: 'Intros', help: 'Skip detected intros.', type: 'select', options: skip},
    {key: 'playback.recapSkip', label: 'Recaps', help: 'Skip detected recaps.', type: 'select', options: skip},
    {key: 'playback.creditsSkip', label: 'Credits', help: 'Skip detected credits.', type: 'select', options: skip},
    {key: 'playback.skipBackSeconds', label: 'Skip back', help: 'How far the back button rewinds.', type: 'select', options: secs([5, 10, 15, 30])},
    {key: 'playback.skipForwardSeconds', label: 'Skip forward', help: 'How far the forward button jumps.', type: 'select', options: secs([10, 15, 30, 60])},
    {key: 'playback.defaultSpeed', label: 'Default speed', type: 'select', options: [0.75, 1, 1.25, 1.5].map(v => ({value: String(v), label: v === 1 ? 'Normal' : `${v}×`}))},
    {key: 'playback.startedThresholdPercent', label: 'Counts as started', help: 'How much you watch before a title joins Continue Watching.', type: 'slider', min: 1, max: 10, unit: '%'},
    {key: 'playback.playedThresholdPercent', label: 'Counts as watched', help: 'How much you watch before a title is marked watched.', type: 'slider', min: 80, max: 98, unit: '%'},
  ]},
  {id: 'languages', title: 'Audio and subtitles', fields: [
    {key: 'playback.preferredAudioLanguages', label: 'Preferred audio language', help: 'Portico picks this track when a title has it.', type: 'select', options: ['English', 'Original language', 'Spanish', 'French', 'Japanese'].map(v => ({value: v, label: v}))},
    {key: 'playback.preferredSubtitleLanguages', label: 'Preferred subtitle language', type: 'select', options: ['English', 'Spanish', 'French', 'German'].map(v => ({value: v, label: v}))},
    {key: 'playback.subtitlesEnabled', label: 'Subtitles on by default', type: 'switch'},
    {key: 'playback.subtitleSize', label: 'Subtitle size', type: 'segmented', options: [{value: 'small', label: 'Small'}, {value: 'medium', label: 'Medium'}, {value: 'large', label: 'Large'}, {value: 'extra-large', label: 'Extra large'}]},
    {key: 'playback.subtitleBackground', label: 'Subtitle background', type: 'segmented', options: [{value: 'none', label: 'None'}, {value: 'shadow', label: 'Shadow'}, {value: 'box', label: 'Box'}]},
    {key: 'playback.showSyncedLyrics', label: 'Show synced lyrics', type: 'switch'},
  ]},
  {id: 'quality', title: 'Quality', description: 'Limits for each kind of network.', fields: [
    {key: 'quality.local', label: 'Home network', type: 'select', options: quality},
    {key: 'quality.wifi', label: 'Wi-Fi', type: 'select', options: quality},
    {key: 'quality.cellular', label: 'Cellular', type: 'select', options: quality},
    {key: 'quality.unknown', label: 'Other networks', type: 'select', options: quality},
    {key: 'quality.allowHDR', label: 'Allow HDR', help: 'Play HDR when this display supports it.', type: 'switch'},
  ]},
  {id: 'delivery', title: 'Streaming', description: 'How this browser prefers to receive media from your server.', fields: [
    {key: 'delivery.directPlay', label: 'Play original files', help: 'Play the original file when this browser can.', type: 'switch'},
    {key: 'delivery.directStream', label: 'Repackage when needed', help: 'Change only the file’s container, keeping full quality, when that’s enough to play it.', type: 'switch'},
    {key: 'delivery.transcode', label: 'Allow conversion', help: 'Let your server convert video that this browser can’t play.', type: 'switch'},
    {key: 'preferences.receiveFromDevices', label: 'Play from other devices', help: 'Let your phone or another Portico app send playback to this browser.', type: 'switch'},
  ]},
  {id: 'music', title: 'Music', fields: [
    {key: 'music.audioNormalization', label: 'Volume normalization', type: 'segmented', options: [{value: 'off', label: 'Off'}, {value: 'track', label: 'Per song'}, {value: 'album', label: 'Per album'}]},
    {key: 'music.crossfadeSeconds', label: 'Crossfade', help: 'Overlap between songs.', type: 'select', options: [{value: '0', label: 'Off'}, ...secs([2, 4, 6, 8])]},
    {key: 'music.gapless', label: 'Gapless playback', type: 'switch'},
    {key: 'music.shuffleDefault', label: 'Shuffle by default', type: 'switch'},
    {key: 'music.repeatDefault', label: 'Repeat by default', type: 'segmented', options: [{value: 'off', label: 'Off'}, {value: 'one', label: 'One song'}, {value: 'all', label: 'All'}]},
  ]},
  {id: 'privacy', title: 'Privacy', fields: [
    {key: 'privacy.pauseWatchHistory', label: 'Pause watch history', help: 'Nothing new is recorded while paused. Existing progress still resumes.', type: 'switch'},
    {key: 'privacy.showActivityToMembers', label: 'Show my activity to others on this server', type: 'switch'},
    {key: 'privacy.includeInWatchTogether', label: 'Include me in Watch Together activity', type: 'switch'},
    {key: 'search.rememberHistory', label: 'Remember recent searches', type: 'switch'},
  ]},
  {id: 'region', title: 'Language and region', fields: [
    {key: 'region.locale', label: 'Language and region', type: 'select', options: [{value: 'en-US', label: 'English (United States)'}, {value: 'en-CA', label: 'English (Canada)'}]},
    {key: 'region.timeZone', label: 'Time zone', type: 'select', options: ['America/Los_Angeles', 'America/Denver', 'America/Chicago', 'America/New_York', 'Europe/London', 'UTC'].map(v => ({value: v, label: v.replace('_', ' ')}))},
    {key: 'region.hourCycle', label: 'Clock', type: 'segmented', options: [{value: 'auto', label: 'Follow system'}, {value: 'h12', label: '12-hour'}, {value: 'h23', label: '24-hour'}]},
  ]},
];

function PreferencesScreen() {
  const services = useServices();
  const version = useDataVersion();
  const q = useQuery(`prefs.${version}`, () => services.preferences.get());
  const {prefs, update} = usePrefs();
  const [saved, setSaved] = useState<string | null>(null);
  const [reset, setReset] = useState(false);
  const [latency, setLat] = useState(latencyFactor());
  const set = (key: string, v: PreferenceValue) => {
    void services.preferences.set({[key]: v}).then(() => { setSaved(key); setTimeout(() => setSaved(s => (s === key ? null : s)), 1600); }, () => toast('That change wasn’t saved. Try again.', {tone: 'bad'}));
  };
  const control = (f: Field) => {
    const raw = q.data?.[f.key];
    const v = Array.isArray(raw) ? raw[0] ?? '' : raw;
    if (f.type === 'switch') return <Switch checked={!!v} onChange={x => set(f.key, x)} label={f.label} />;
    if (f.type === 'segmented') return <Segmented size="sm" label={f.label} value={String(v)} onChange={x => set(f.key, x)} options={f.options!} />;
    if (f.type === 'slider') return <span className="pref-slider"><Slider label={f.label} value={Number(v)} min={f.min} max={f.max} onChange={x => set(f.key, x)} /><span className="tabular">{String(v)}{f.unit}</span></span>;
    return <Select label={f.label} value={String(v)} onChange={x => set(f.key, f.key.includes('Languages') ? [x] : /^\d+(\.\d+)?$/.test(x) ? Number(x) : x)} options={f.options!} />;
  };
  return (
    <Page title="Preferences" className="settings-page">
      <PageHeader back={{label: 'Settings', to: '/settings'}} title="Preferences" eyebrow="Saved to your profile" lede="These follow your profile to every Portico app. Changes save as you make them." size="title" />
      <div className="settings-col pref-layout">
        <nav className="pref-nav" aria-label="Preference groups">
          {[...GROUPS, {id: 'appearance', title: 'Appearance'}, {id: 'recommendations', title: 'Recommendations'}].map(g => <a key={g.id} href={`#pref-${g.id}`}>{g.title}</a>)}
        </nav>
        <div className="pref-groups">
          {q.error ? <ErrorState error={q.error} onRetry={q.reload} what="your preferences" /> : null}
          {GROUPS.map(g => (
            <Panel key={g.id} title={<span id={`pref-${g.id}`}>{g.title}</span>} subtitle={g.description} padded={false}>
              {q.data ? g.fields.map(f => (
                <Row key={f.key} label={f.label} help={f.help} trailing={saved === f.key ? <span className="saved-tick" role="status"><Icon name="check" size={14} />Saved</span> : undefined}>{control(f)}</Row>
              )) : g.fields.map(f => <Row key={f.key} label={f.label}><Skeleton w={120} h={32} /></Row>)}
            </Panel>
          ))}
          <Panel title={<span id="pref-appearance">Appearance</span>} subtitle="Applies to this browser." padded={false}>
            <Row label="Show backdrops" help="Artwork behind title pages and Home."><Switch checked={prefs.showBackdrops} onChange={v => update({showBackdrops: v})} label="Show backdrops" /></Row>
            <Row label="Card size" help="Relative to the standard poster size."><Segmented size="sm" label="Card size" value={prefs.posterSize} onChange={v => update({posterSize: v})} options={[{value: 'compact', label: 'Smaller'}, {value: 'regular', label: 'Standard'}, {value: 'large', label: 'Larger'}]} /></Row>
            <Row label="Text size"><Segmented size="sm" label="Text size" value={prefs.textSize} onChange={v => update({textSize: v})} options={[{value: 'default', label: 'Standard'}, {value: 'large', label: 'Larger'}]} /></Row>
            <Row label="Reduce motion" help="Shorter transitions and no zoom on hover."><Switch checked={prefs.reduceMotion} onChange={v => update({reduceMotion: v})} label="Reduce motion" /></Row>
            <Row label="Hover previews" help="Show a preview when you rest the pointer on a title."><Switch checked={prefs.autoplayPreviews} onChange={v => update({autoplayPreviews: v})} label="Hover previews" /></Row>
            <Row label="Show unread badge" help="On Notifications in the sidebar."><Switch checked={!!q.data?.['notifications.badges']} onChange={v => set('notifications.badges', v)} label="Show unread badge" /></Row>
          </Panel>
          <Panel title={<span id="pref-recommendations">Recommendations</span>} padded={false}>
            <Row label="Reset recommendations" help="Forget likes, dislikes and Not interested for this profile. Watch history stays."><Button variant="secondary" label="Reset…" onClick={() => setReset(true)} /></Row>
          </Panel>
          <Panel title="Demo data" subtitle="This build runs on sample data. Simulated network delay makes loading states visible." padded={false}>
            <Row label="Simulated delay"><Segmented size="sm" label="Simulated delay" value={String(latency)} onChange={v => { setLat(Number(v)); setLatencyFactor(Number(v)); }} options={[{value: '0', label: 'Off'}, {value: '1', label: 'Normal'}, {value: '3', label: 'Slow'}]} /></Row>
          </Panel>
        </div>
      </div>
      <ConfirmDialog open={reset} onOpenChange={setReset} title="Reset your recommendations?" body="Portico forgets what you liked, disliked and marked Not interested, so Picks for you starts fresh. Your watch history and ratings stay." confirmLabel="Reset recommendations" onConfirm={() => services.user.resetRecommendations().then(() => toast('Recommendations reset'))} />
    </Page>
  );
}

// ---- Account (this server)

function AccountSection() {
  const session = useSession();
  const navigate = useNavigate();
  return (
    <div className="stack">
      <Panel>
        <div className="acct-card">
          <Avatar name={session.profile?.name ?? '?'} color={session.profile?.color} size={72} />
          <div className="set-text"><strong className="acct-name">{session.profile?.name}</strong><span>{session.profile?.main ? 'Main profile' : session.profile?.kids ? 'Kids profile' : 'Profile'} on {session.server?.name}</span><span>{session.via === 'account' ? 'Signed in with your Portico Account (justin)' : 'Signed in with a server account'}</span></div>
          <div className="hstack">
            <Button variant="secondary" icon="people" label="Switch profile" onClick={() => void navigate({to: '/welcome/$screen', params: {screen: 'profiles'}})} />
            <Button variant="ghost" icon="server" label="Switch server" onClick={() => void navigate({to: '/welcome/$screen', params: {screen: 'servers'}})} />
          </div>
        </div>
      </Panel>
      {session.via === 'account' ? <Notice title="Your Portico Account manages sign-in" action={<Button size="sm" variant="secondary" label="Open Portico Account" to="/account" />}>Password, two-step verification and where you’re signed in are managed there.</Notice> : null}
      <SecuritySection />
      <DevicesSection />
    </div>
  );
}

function ServersSection() {
  const services = useServices();
  const session = useSession();
  const navigate = useNavigate();
  const servers = useQuery('account.servers', () => services.account.servers());
  const [leave, setLeave] = useState<string | null>(null);
  const [checking, setChecking] = useState(false);
  return (
    <div className="stack">
      <Panel title="Connected to" padded={false}>
        <Row icon="server" label={session.server?.name} help={`${session.server?.host} · Portico ${session.server?.version} · ${session.role === 'owner' ? 'You own it' : session.role === 'admin' ? 'You’re an admin' : 'Member'}`}><Badge tone="good" icon="check">Online</Badge></Row>
      </Panel>
      <Panel title="Your servers" actions={<Button size="sm" variant="ghost" icon="refresh" label="Check now" busy={checking} onClick={() => { setChecking(true); setTimeout(() => { setChecking(false); toast('Server list refreshed'); }, 900); }} />} padded={false}>
        {!servers.data ? <Row label={<Skeleton w={200} h={16} />} /> : servers.data.map(s => (
          <Row key={s.id} icon="server" label={s.name} help={`${s.host} · ${s.status === 'online' ? 'Online' : `Can’t reach it · last seen ${new Date(s.lastSeen).toLocaleDateString('en-US', {month: 'short', day: 'numeric'})}`} · ${s.role === 'owner' ? 'Owner' : s.role === 'admin' ? 'Admin' : 'Member'}`}>
            {s.id === session.serverId ? <Badge tone="accent">Current</Badge> : s.status === 'online' ? <Button size="sm" variant="secondary" label="Switch" onClick={() => { session.setServer(s.id); void navigate({to: '/welcome/$screen', params: {screen: 'profiles'}}); }} /> : <Badge tone="warn">Unreachable</Badge>}
            {s.role !== 'owner' ? <Button size="sm" variant="ghost" label="Leave" onClick={() => setLeave(s.id)} /> : null}
          </Row>
        ))}
      </Panel>
      <Panel title="Sign in to another server" subtitle="Use a server’s own account, without a Portico Account."><Button variant="secondary" icon="plus" label="Sign in directly" onClick={() => void navigate({to: '/welcome/$screen', params: {screen: 'direct'}})} /></Panel>
      <ConfirmDialog open={!!leave} onOpenChange={o => !o && setLeave(null)} danger title={`Leave ${servers.data?.find(s => s.id === leave)?.name}?`} body="You’ll lose access to its libraries. The owner can invite you again." confirmLabel="Leave server" onConfirm={async () => { if (leave) await services.account.leaveServer(leave); toast('You left the server'); }} />
    </div>
  );
}

function AboutSection() {
  const session = useSession();
  return (
    <div className="stack">
      <Panel>
        <div className="about-head"><BrandSymbol size={44} /><div><strong>Portico on the web</strong><span className="faint">Version 1.4.2 · Stable</span></div></div>
        <KeyValue items={[['Server', `${session.server?.name} · Portico ${session.server?.version}`], ['Web app', '1.4.2 · built September 27, 2026'], ['Browser', navigator.userAgent.includes('Chrome') ? 'Chrome' : navigator.userAgent.includes('Safari') ? 'Safari' : 'Browser']]} />
      </Panel>
      <Panel padded={false}>
        <Row icon="help" label="Help" help="Guides and answers at getportico.tv" onClick={() => window.open('https://getportico.tv/help', '_blank', 'noopener')} />
        <Row icon="file" label="Terms of service" onClick={() => window.open('https://getportico.tv/terms', '_blank', 'noopener')} />
        <Row icon="shield" label="Privacy policy" onClick={() => window.open('https://getportico.tv/privacy', '_blank', 'noopener')} />
        <Row icon="about" label="Open-source licenses" help="Manrope and Archivo (SIL Open Font License), React, TanStack Router, Radix UI, Lucide" />
        <Row icon="grid" label="Screen index" help="Every screen and state of this build, for review" to="/dev/screens" />
      </Panel>
      <p className="faint small">Metadata and images come from TMDB and the Wikimedia Commons public domain. This product uses the TMDB API but is not endorsed or certified by TMDB. Sample artwork: Wikimedia Commons (public domain), the Blender Foundation (CC BY) and Unsplash.</p>
    </div>
  );
}

