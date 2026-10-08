import { useState, type ReactNode } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import {
  Bell,
  ChevronRight,
  Clock,
  Download,
  Eye,
  Fingerprint,
  Info,
  KeyRound,
  Laptop,
  Lock,
  LogOut,
  Monitor,
  Moon,
  Plus,
  Shield,
  ShieldCheck,
  Smartphone,
  Speaker,
  Tablet,
  Trash2,
  Tv,
  User,
  Users,
} from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import type { ContentRating, Profile } from '../../data/types';
import { useLibraries, useProfiles, useSecurity } from '../../services/queries';
import { api } from '../../services/client';
import { useSession } from '../../state/session';
import { useCurrentProfile } from '../../app/ProfileMenu';
import { PageHeader } from '../../components/PageHeader';
import { Avatar, Button, Segmented, Skel, Switch, cls } from '../../components/ui';
import { Confirm, Sheet, toast } from '../../components/overlay';
import { ago, useIsMobile } from '../../lib/hooks';

const SECTIONS: { id: string; label: string; icon: ReactNode; color: string; sub: string }[] = [
  { id: 'account', label: 'Account', icon: <User />, color: '#409fd4', sub: 'Name, email, server' },
  { id: 'profiles', label: 'Profiles & Parental Controls', icon: <Users />, color: '#8a5cf0', sub: 'Kids, PINs, ratings, bedtime' },
  { id: 'playback', label: 'Playback & Quality', icon: <Tv />, color: '#2f86bb', sub: 'Subtitles, audio, streaming quality' },
  { id: 'downloads', label: 'Downloads & Offline', icon: <Download />, color: '#3fae6b', sub: 'Quality, smart downloads, storage' },
  { id: 'notifications', label: 'Notifications', icon: <Bell />, color: '#e0513f', sub: 'New episodes, recordings, shares' },
  { id: 'security', label: 'Sign-In & Security', icon: <Shield />, color: '#5a6b7c', sub: 'Passkeys, two-step, devices' },
  { id: 'appearance', label: 'Appearance & Accessibility', icon: <Eye />, color: '#f0a02a', sub: 'Transparency, motion, captions' },
  { id: 'about', label: 'About Portico', icon: <Info />, color: '#6f7bf7', sub: 'Version, licenses, credits' },
];

export function SettingsPage() {
  const { section } = useParams();
  const mobile = useIsMobile();
  const navigate = useNavigate();
  const me = useCurrentProfile();
  const kid = me?.role === 'kid';
  const current = section ?? (mobile ? null : 'account');
  const sec = SECTIONS.find((s) => s.id === current);
  const allowed = kid ? SECTIONS.filter((s) => ['playback', 'appearance', 'about'].includes(s.id)) : SECTIONS;

  if (kid && current && !['playback', 'appearance', 'about'].includes(current))
    return (
      <div className="page">
        <PageHeader title="Settings" />
        <div className="pad">
          <div className="kid-lock">
            <Lock />
            <h2>Ask a grown-up</h2>
            <p>This setting is managed by the people who run Harbor.</p>
            <Button variant="plain" onClick={() => navigate('/settings/playback')}>
              See what you can change
            </Button>
          </div>
        </div>
      </div>
    );

  if (mobile && !current)
    return (
      <div className="page">
        <PageHeader title="Settings" />
        <div className="pad settings-home">
          {me && (
            <div className="group me-card">
              <Link to="/settings/account" className="row">
                <Avatar name={me.name} color={me.avatarColor} size={56} />
                <span className="row-main">
                  <span className="row-title" style={{ fontSize: 19, fontWeight: 700 }}>
                    {me.name}
                  </span>
                  <span className="row-sub">{me.email ?? 'Profile on Harbor'}</span>
                </span>
                <ChevronRight className="row-chev" />
              </Link>
            </div>
          )}
          <div className="group">
            {allowed.map((s) => (
              <Link key={s.id} to={`/settings/${s.id}`} className="row has-icon">
                <span className="row-icon" style={{ background: s.color }}>
                  {s.icon}
                </span>
                <span className="row-main">
                  <span className="row-title">{s.label}</span>
                </span>
                <ChevronRight className="row-chev" />
              </Link>
            ))}
          </div>
        </div>
      </div>
    );

  return (
    <div className="page">
      <PageHeader title={mobile ? sec?.label ?? 'Settings' : 'Settings'} subtitle={mobile ? undefined : sec?.sub} back={mobile ? '/settings' : undefined} />
      {!mobile && (
        <div className="pad">
          <nav className="settings-tabs no-scrollbar" aria-label="Settings sections">
            {allowed.map((s) => (
              <Link key={s.id} to={`/settings/${s.id}`} className={cls('settings-tab', s.id === current && 'on')} aria-current={s.id === current ? 'page' : undefined}>
                <span className="row-icon sm" style={{ background: s.color }}>
                  {s.icon}
                </span>
                {s.label.split(' & ')[0]}
              </Link>
            ))}
          </nav>
        </div>
      )}
      <div className="page-body pad settings-body" key={current}>
        {current === 'account' && <AccountSection />}
        {current === 'profiles' && <ProfilesSection />}
        {current === 'playback' && <PlaybackSection />}
        {current === 'downloads' && <DownloadsSection />}
        {current === 'notifications' && <NotificationsSection />}
        {current === 'security' && <SecuritySection />}
        {current === 'appearance' && <AppearanceSection />}
        {current === 'about' && <AboutSection />}
      </div>
    </div>
  );
}

// ---------- building blocks ----------
function Group({ title, foot, children }: { title?: string; foot?: ReactNode; children: ReactNode }) {
  return (
    <section className="sgroup">
      {title && <h2 className="group-title">{title}</h2>}
      <div className="group">{children}</div>
      {foot && <p className="group-foot">{foot}</p>}
    </section>
  );
}
function ToggleRow({ title, sub, value, onChange, icon, color }: { title: string; sub?: string; value: boolean; onChange: (v: boolean) => void; icon?: ReactNode; color?: string }) {
  return (
    <div className={cls('row', icon && 'has-icon')}>
      {icon && (
        <span className="row-icon" style={{ background: color }}>
          {icon}
        </span>
      )}
      <span className="row-main">
        <span className="row-title">{title}</span>
        {sub && <span className="row-sub">{sub}</span>}
      </span>
      <Switch checked={value} onChange={(v) => (onChange(v), toast('Saved'))} label={title} />
    </div>
  );
}
function SelectRow<T extends string>({ title, sub, value, options, onChange }: { title: string; sub?: string; value: T; options: { value: T; label: string }[]; onChange: (v: T) => void }) {
  return (
    <label className="row">
      <span className="row-main">
        <span className="row-title">{title}</span>
        {sub && <span className="row-sub">{sub}</span>}
      </span>
      <select className="input sm row-select" value={value} onChange={(e) => (onChange(e.target.value as T), toast('Saved'))}>
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </label>
  );
}
function ValueRow({ title, value, onClick, danger }: { title: string; value?: ReactNode; onClick?: () => void; danger?: boolean }) {
  return (
    <button className={cls('row', danger && 'danger-row')} onClick={onClick}>
      <span className="row-main">
        <span className="row-title">{title}</span>
      </span>
      {value !== undefined && <span className="row-value">{value}</span>}
      {onClick && !danger && <ChevronRight className="row-chev" />}
    </button>
  );
}

function useProfilePatch() {
  const me = useCurrentProfile();
  const qc = useQueryClient();
  return async (patch: Partial<Profile>) => {
    if (!me) return;
    await api.updateProfile(me.id, patch);
    qc.invalidateQueries({ queryKey: ['profiles'] });
  };
}

// ---------- sections ----------
function AccountSection() {
  const me = useCurrentProfile();
  const signOut = useSession((s) => s.signOut);
  const navigate = useNavigate();
  const [edit, setEdit] = useState(false);
  const [name, setName] = useState(me?.name ?? '');
  const patch = useProfilePatch();
  const [confirm, setConfirm] = useState(false);
  if (!me) return <Skel h={300} />;
  return (
    <>
      <div className="account-card">
        <Avatar name={me.name} color={me.avatarColor} size={76} />
        <div>
          <div className="account-name">{me.name}</div>
          <div className="t-sub">
            {me.email ?? 'No email on this profile'} · {me.role === 'owner' ? 'Owner of Harbor' : me.role === 'admin' ? 'Admin' : 'Member'} · since {new Date(me.createdAt).getFullYear()}
          </div>
        </div>
        <Button variant="plain" onClick={() => setEdit(true)}>
          Edit
        </Button>
      </div>
      <Group title="Server">
        <ValueRow title="Connected to" value="Harbor · 192.168.1.20 (local)" />
        <ValueRow title="Remote address" value="harbor.lindqvist.family" />
        <ValueRow title="Switch server" value="3 available" onClick={() => navigate('/servers')} />
      </Group>
      <Group title="Language & Region">
        <SelectRow title="Language" value="en-US" options={[{ value: 'en-US', label: 'English (US)' }, { value: 'sv', label: 'Svenska' }, { value: 'es', label: 'Español' }]} onChange={() => undefined} />
        <SelectRow title="Time format" value="12" options={[{ value: '12', label: '12-hour' }, { value: '24', label: '24-hour' }]} onChange={() => undefined} />
      </Group>
      <Group title="Your data" foot="Portico keeps your watch history on your server, not in the cloud. Exports are plain JSON and CSV.">
        <ValueRow title="Export watch history" value="JSON · CSV" onClick={() => toast('Preparing export… you’ll get a notification when it’s ready', <Download />)} />
        <ValueRow title="Sign out of this browser" onClick={() => setConfirm(true)} />
      </Group>
      <Sheet
        open={edit}
        onClose={() => setEdit(false)}
        title="Edit Account"
        size="sm"
        footer={
          <Button variant="accent" onClick={() => patch({ name }).then(() => (setEdit(false), toast('Saved')))}>
            Save
          </Button>
        }
      >
        <div className="form-stack">
          <div className="field">
            <label htmlFor="acc-name">Display name</label>
            <input id="acc-name" className="input" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="field">
            <label htmlFor="acc-email">Email</label>
            <input id="acc-email" className="input" defaultValue={me.email} type="email" />
            <span className="field-hint">We’ll send a confirmation link before changing it.</span>
          </div>
        </div>
      </Sheet>
      <Confirm
        open={confirm}
        onClose={() => setConfirm(false)}
        title="Sign out?"
        body="Downloads on this browser will be kept but won’t play until you sign in again."
        confirmLabel="Sign Out"
        danger
        onConfirm={() => {
          signOut();
          navigate('/signin');
        }}
      />
    </>
  );
}

const MOVIE_R: ContentRating[] = ['G', 'PG', 'PG-13', 'R', 'NR'];
const TV_R: ContentRating[] = ['TV-Y', 'TV-Y7', 'TV-G', 'TV-PG', 'TV-14', 'TV-MA'];

function ProfilesSection() {
  const profiles = useProfiles();
  const libs = useLibraries();
  const qc = useQueryClient();
  const [sel, setSel] = useState<string | null>(null);
  const p = profiles.data?.find((x) => x.id === (sel ?? profiles.data?.find((y) => y.role === 'kid')?.id));
  const update = async (patch: Partial<Profile>) => {
    if (!p) return;
    await api.updateProfile(p.id, patch);
    qc.invalidateQueries({ queryKey: ['profiles'] });
    toast(`Updated ${p.name}`);
  };
  if (!profiles.data) return <Skel h={300} />;
  return (
    <>
      <div className="profile-strip">
        {profiles.data.map((x) => (
          <button key={x.id} className={cls('pstrip', x.id === p?.id && 'on')} onClick={() => setSel(x.id)}>
            <Avatar name={x.name} color={x.avatarColor} size={60} />
            <span>{x.name}</span>
            <span className="t-caption">{x.role === 'kid' ? 'Kids' : x.role === 'owner' ? 'Owner' : x.role === 'admin' ? 'Admin' : 'Member'}</span>
          </button>
        ))}
        <Link to="/profiles" className="pstrip add">
          <span className="pstrip-add">
            <Plus />
          </span>
          <span>Add</span>
        </Link>
      </div>
      {p && (
        <>
          <Group title={`Content for ${p.name}`} foot={p.role === 'owner' || p.role === 'admin' ? 'Owners and admins can always see everything.' : 'Titles above these ratings are hidden everywhere — search, Home, Live TV and casting.'}>
            <div className="row">
              <span className="row-main">
                <span className="row-title">Movies up to</span>
              </span>
              <Segmented size="sm" value={p.maxRating} onChange={(v) => update({ maxRating: v })} options={MOVIE_R.slice(0, 4).map((r) => ({ value: r, label: r }))} />
            </div>
            <div className="row">
              <span className="row-main">
                <span className="row-title">TV up to</span>
              </span>
              <Segmented size="sm" value={p.maxTvRating} onChange={(v) => update({ maxTvRating: v })} options={TV_R.map((r) => ({ value: r, label: r.replace('TV-', '') }))} />
            </div>
            <ToggleRow title="Allow unrated titles" sub="Silent films, home videos and imports without a rating" value={p.maxRating === 'R'} onChange={() => undefined} />
          </Group>
          <Group title="Libraries">
            {(libs.data ?? []).map((l) => {
              const on = p.libraries === 'all' || p.libraries.includes(l.id);
              return (
                <ToggleRow
                  key={l.id}
                  title={l.name}
                  sub={l.countLabel}
                  value={on}
                  onChange={(v) => {
                    const cur = p.libraries === 'all' ? (libs.data ?? []).map((x) => x.id) : p.libraries;
                    update({ libraries: v ? [...cur, l.id] : cur.filter((x) => x !== l.id) });
                  }}
                />
              );
            })}
          </Group>
          <Group title="Screen time" foot="At bedtime, playback finishes the current episode and then pauses with a gentle goodnight screen.">
            <SelectRow
              title="Bedtime"
              value={p.restrictions?.bedtime ?? 'off'}
              options={[{ value: 'off', label: 'Off' }, ...['19:00', '19:30', '20:00', '20:30', '21:00', '22:00', '22:30'].map((t) => ({ value: t, label: new Date(`2000-01-01T${t}`).toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' }) }))]}
              onChange={(v) => update({ restrictions: { ...p.restrictions, bedtime: v === 'off' ? undefined : v } })}
            />
            <SelectRow title="Daily limit" value={String(p.restrictions?.dailyLimitMin ?? 'off')} options={[{ value: 'off', label: 'No limit' }, ...[30, 60, 90, 120, 180].map((m) => ({ value: String(m), label: `${m >= 60 ? `${m / 60} hr` : `${m} min`}` }))]} onChange={(v) => update({ restrictions: { ...p.restrictions, dailyLimitMin: v === 'off' ? undefined : Number(v) } })} />
          </Group>
          <Group title="Access">
            <ToggleRow title="Require PIN" sub={p.hasPin ? 'A 4-digit PIN is set' : 'Anyone can open this profile'} value={p.hasPin} onChange={(v) => update({ hasPin: v })} icon={<KeyRound />} color="#5a6b7c" />
            <ToggleRow title="Kids profile" sub="Simplified Home, no settings, no Live TV" value={p.role === 'kid'} onChange={(v) => update({ role: v ? 'kid' : 'member' })} icon={<Moon />} color="#35c28a" />
          </Group>
        </>
      )}
    </>
  );
}

function PlaybackSection() {
  const me = useCurrentProfile();
  const patch = useProfilePatch();
  if (!me) return <Skel h={300} />;
  const langs = [
    { value: 'original', label: 'Original language' },
    { value: 'en', label: 'English' },
    { value: 'ja', label: 'Japanese' },
    { value: 'es', label: 'Spanish' },
    { value: 'fr', label: 'French' },
    { value: 'sv', label: 'Swedish' },
  ];
  return (
    <>
      <Group title="Quality" foot="Original plays the file exactly as stored when your device and connection can handle it; Portico converts only what it must.">
        <SelectRow title="On Wi-Fi and wired" value={me.streamingQuality} options={[{ value: 'original', label: 'Original' }, { value: 'auto', label: 'Automatic' }, { value: '1080p', label: '1080p · 10 Mbps' }, { value: '720p', label: '720p · 4 Mbps' }, { value: '480p', label: '480p · 2 Mbps' }]} onChange={(v) => patch({ streamingQuality: v })} />
        <SelectRow title="On cellular" value={me.cellularQuality} options={[{ value: 'auto', label: 'Automatic' }, { value: '720p', label: '720p · 4 Mbps' }, { value: '480p', label: 'Data saver · 2 Mbps' }]} onChange={(v) => patch({ cellularQuality: v })} />
      </Group>
      <Group title="Audio & Subtitles">
        <SelectRow title="Preferred audio" value={me.audioLanguage} options={langs} onChange={(v) => patch({ audioLanguage: v })} />
        <SelectRow title="Subtitles" value={me.subtitleMode} options={[{ value: 'auto', label: 'When audio isn’t my language' }, { value: 'always', label: 'Always on' }, { value: 'forced', label: 'Forced only (signs, foreign dialogue)' }, { value: 'off', label: 'Off' }]} onChange={(v) => patch({ subtitleMode: v })} />
        <SelectRow title="Subtitle language" value={me.subtitleLanguage} options={langs.slice(1)} onChange={(v) => patch({ subtitleLanguage: v })} />
      </Group>
      <Group title="Autoplay">
        <ToggleRow title="Play next episode automatically" sub="Up Next appears when the credits start" value={me.autoplayNext} onChange={(v) => patch({ autoplayNext: v })} />
        <ToggleRow title="Skip intros automatically" sub="Uses intro detection from Harbor" value={false} onChange={() => undefined} />
        <ToggleRow title="Play previews on Home" value={me.autoplayPreviews} onChange={(v) => patch({ autoplayPreviews: v })} />
      </Group>
    </>
  );
}

function DownloadsSection() {
  const [smart, setSmart] = useState(true);
  const [wifi, setWifi] = useState(true);
  const [q, setQ] = useState('High (1080p)');
  return (
    <>
      <Group title="Downloads">
        <SelectRow title="Default quality" value={q} options={['Original', 'High (1080p)', 'Medium (720p)', 'Low (480p)'].map((v) => ({ value: v, label: v }))} onChange={setQ} />
        <ToggleRow title="Download on Wi-Fi only" value={wifi} onChange={setWifi} />
        <ToggleRow title="Smart downloads" sub="Keep the next 3 episodes of shows you’re watching, remove watched ones" value={smart} onChange={setSmart} />
      </Group>
      <Group title="Storage" foot="Offline licenses renew automatically whenever this device reconnects to Harbor, and expire after 30 days offline.">
        <ValueRow title="Used by downloads" value="15.4 GB" />
        <ValueRow title="Available" value="41.2 GB" />
        <ValueRow title="Remove all downloads" danger onClick={() => toast('Removed all downloads from this browser', <Trash2 />)} />
      </Group>
    </>
  );
}

function NotificationsSection() {
  const [s, setS] = useState({ eps: true, rec: true, dl: true, share: true, server: true, security: true, digest: false });
  const t = (k: keyof typeof s) => (v: boolean) => setS({ ...s, [k]: v });
  return (
    <>
      <Group title="Notify me about">
        <ToggleRow title="New episodes" sub="For shows you’ve watched in the last 3 months" value={s.eps} onChange={t('eps')} />
        <ToggleRow title="Finished recordings" value={s.rec} onChange={t('rec')} />
        <ToggleRow title="Completed downloads" value={s.dl} onChange={t('dl')} />
        <ToggleRow title="Recommendations from family" value={s.share} onChange={t('share')} />
      </Group>
      <Group title="Server" foot="Only owners and admins get server alerts.">
        <ToggleRow title="Disk, backup and update alerts" value={s.server} onChange={t('server')} />
        <ToggleRow title="New sign-ins to my account" value={s.security} onChange={t('security')} />
        <ToggleRow title="Weekly email digest" value={s.digest} onChange={t('digest')} />
      </Group>
    </>
  );
}

const DEVICE_ICON: Record<string, ReactNode> = { tv: <Tv />, phone: <Smartphone />, tablet: <Tablet />, desktop: <Monitor />, browser: <Laptop />, speaker: <Speaker />, console: <Monitor /> };

function SecuritySection() {
  const q = useSecurity();
  const qc = useQueryClient();
  const [revoke, setRevoke] = useState<string | null>(null);
  const [pw, setPw] = useState(false);
  if (!q.data) return <Skel h={400} />;
  const s = q.data;
  return (
    <>
      <div className="sec-score">
        <ShieldCheck />
        <div>
          <div className="t-headline">Your account is well protected</div>
          <div className="t-sub">Passkeys on 2 devices · two-step verification on · strong password changed {ago(s.password.lastChanged)}</div>
        </div>
      </div>
      <Group title="Sign-in methods">
        {s.mfa.passkeys.map((k) => (
          <div key={k.id} className="row has-icon">
            <span className="row-icon" style={{ background: '#2f86bb' }}>
              <Fingerprint />
            </span>
            <span className="row-main">
              <span className="row-title">Passkey · {k.name}</span>
              <span className="row-sub">
                Added {ago(k.created)} · last used {ago(k.lastUsed)}
              </span>
            </span>
            <Button size="sm" variant="ghost" onClick={() => toast(`Removed passkey ${k.name}`)}>
              Remove
            </Button>
          </div>
        ))}
        <button className="row has-icon" onClick={() => toast('Follow the prompt from your browser or security key', <Fingerprint />)}>
          <span className="row-icon" style={{ background: '#3fae6b' }}>
            <Plus />
          </span>
          <span className="row-main">
            <span className="row-title linkish">Add a passkey</span>
          </span>
        </button>
        <ToggleRow title="Two-step verification" sub={`Authenticator app · ${s.mfa.recoveryCodesLeft} recovery codes left`} value={s.mfa.totp} onChange={() => undefined} icon={<KeyRound />} color="#5a6b7c" />
        <ValueRow title="Password" value={`Changed ${ago(s.password.lastChanged)}`} onClick={() => setPw(true)} />
      </Group>
      <Group title="Signed-in devices" foot="Signing out a device ends its session immediately and removes its downloads’ offline licenses.">
        {s.devices.map((d) => (
          <div key={d.id} className="row has-icon">
            <span className="row-icon" style={{ background: '#1c3a57' }}>
              {DEVICE_ICON[d.kind]}
            </span>
            <span className="row-main">
              <span className="row-title">
                {d.name} {d.current && <span className="badge green">This browser</span>}
              </span>
              <span className="row-sub">
                {d.platform} · {d.app} · {d.location} · {d.current ? 'Active now' : ago(d.lastSeen)}
              </span>
            </span>
            {!d.current && (
              <Button size="sm" variant="ghost" icon={<LogOut />} onClick={() => setRevoke(d.id)}>
                Sign Out
              </Button>
            )}
          </div>
        ))}
      </Group>
      <Group title="Recent sign-in activity">
        {s.signInHistory.map((h, i) => (
          <div key={i} className="row">
            <span className={cls('pill-status', h.ok ? 'ok' : 'err')} />
            <span className="row-main">
              <span className="row-title">
                {h.device} · {h.location}
              </span>
              <span className="row-sub tnum">
                {h.method} · {h.ip}
              </span>
            </span>
            <span className="row-value">
              <Clock className="inline-ic" />
              {ago(h.at)}
            </span>
          </div>
        ))}
      </Group>
      <Confirm
        open={!!revoke}
        onClose={() => setRevoke(null)}
        danger
        title="Sign out this device?"
        confirmLabel="Sign Out Device"
        onConfirm={async () => {
          await api.revokeDevice(revoke!);
          qc.invalidateQueries({ queryKey: ['security'] });
          toast('Device signed out', <LogOut />);
        }}
      />
      <Sheet
        open={pw}
        onClose={() => setPw(false)}
        title="Change Password"
        size="sm"
        footer={
          <Button variant="accent" onClick={() => (setPw(false), toast('Password changed. Other devices were signed out.', <Lock />))}>
            Change Password
          </Button>
        }
      >
        <div className="form-stack">
          <div className="field">
            <label htmlFor="pw0">Current password</label>
            <input id="pw0" className="input" type="password" autoComplete="current-password" />
          </div>
          <div className="field">
            <label htmlFor="pw1">New password</label>
            <input id="pw1" className="input" type="password" autoComplete="new-password" />
          </div>
          <label className="check-line">
            <input type="checkbox" defaultChecked /> Sign out all other devices
          </label>
        </div>
      </Sheet>
    </>
  );
}

function AppearanceSection() {
  const transparency = useSession((s) => s.transparency);
  const motion = useSession((s) => s.motion);
  const setT = useSession((s) => s.setTransparency);
  const setM = useSession((s) => s.setMotion);
  return (
    <>
      <Group title="Glass" foot="Reduced transparency swaps glass for solid surfaces with the same depth. “System” follows your device’s accessibility setting.">
        <div className="row">
          <span className="row-main">
            <span className="row-title">Transparency</span>
          </span>
          <Segmented size="sm" value={transparency} onChange={setT} options={[{ value: 'system', label: 'System' }, { value: 'full', label: 'Full' }, { value: 'reduced', label: 'Reduced' }]} />
        </div>
      </Group>
      <Group title="Motion" foot="Reduced motion turns off parallax, Ken Burns pans and animated transitions.">
        <div className="row">
          <span className="row-main">
            <span className="row-title">Animations</span>
          </span>
          <Segmented size="sm" value={motion} onChange={setM} options={[{ value: 'system', label: 'System' }, { value: 'full', label: 'Full' }, { value: 'reduced', label: 'Reduced' }]} />
        </div>
      </Group>
      <Group title="Captions">
        <SelectRow title="Caption style" value="classic" options={[{ value: 'classic', label: 'Classic (white on dark box)' }, { value: 'outline', label: 'Outline' }, { value: 'large', label: 'Large text' }]} onChange={() => undefined} />
        <ToggleRow title="Prefer SDH subtitles" sub="Include sound descriptions when available" value={false} onChange={() => undefined} />
        <ToggleRow title="Audio descriptions" sub="Pick described audio tracks automatically" value={false} onChange={() => undefined} />
      </Group>
    </>
  );
}

function AboutSection() {
  return (
    <>
      <div className="about-card">
        <img src="/brand/portico-appicon-dark.svg" alt="" />
        <div>
          <div className="account-name">Portico</div>
          <div className="t-sub">Web 3.2.0 (build 3204) · Server 3.2.1 on Harbor</div>
        </div>
      </div>
      <Group title="Credits" foot="Portico shows only what’s on your server. No ads, no tracking, no third-party streaming catalogs mixed into your library.">
        <ValueRow title="Blender Studio open movies" value="CC BY — studio.blender.org" />
        <ValueRow title="Classic film posters and stills" value="Public domain, Wikimedia Commons" />
        <ValueRow title="Fixture photography" value="Unsplash via picsum.photos" />
        <ValueRow title="Audiobook texts" value="Public domain" />
      </Group>
      <Group title="Legal">
        <ValueRow title="Open-source licenses" onClick={() => toast('React, TanStack Query, TanStack Virtual, Zustand, Lucide, Inter — MIT / ISC / OFL')} />
        <ValueRow title="Privacy" value="Local-first · telemetry off" />
      </Group>
    </>
  );
}
