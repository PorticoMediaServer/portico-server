import {Wordmark} from '../../shell/Brand';
import {serverLine} from '../../app/format';
import React, {useState} from 'react';
import {View} from 'react-native';
import QRCode from 'react-native-qrcode-svg';
import {useAction, useClient, useQuery, useTopic} from '../../data/DataProvider';
import type {Notice, Profile} from '../../data/types';
import {Button, IconButton} from '../../ui/Button';
import {Avatar, Badge, EmptyState, Group, ListRow, Loading, Notice as NoticeBox, PinPad, Segmented, Surface, TextField, Toggle} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {Focusable} from '../../ui/Focusable';
import {ActionMenu, ConfirmDialog, Sheet} from '../../ui/Sheet';
import {colors, profileArt} from '../../theme/tokens';
import {useMetrics} from '../../theme/layout';
import {useSession} from '../../app/session';
import {useToast} from '../../app/toast';
import {push, root} from '../../nav/nav';
import {ServerSwitcher, ProfileSwitcher} from '../../shell/ProfileMenu';

const ago = (t: number) => { const mn = Math.round((Date.now() - t) / 60000); return mn < 1 ? 'Just now' : mn < 60 ? `${mn} min ago` : mn < 1440 ? `${Math.round(mn / 60)} h ago` : `${Math.round(mn / 1440)} days ago`; };

export function AccountSection() {
  const m = useMetrics();
  const {profile, server, signOut} = useSession();
  const [switching, setSwitching] = useState(false);
  return (
    <>
      <Surface style={{gap: 4}}>
        <Text variant="label" tone="tertiary">Signed in as</Text>
        <Text variant="heading">Justin Ehler</Text>
        <Text variant="caption" tone="secondary">Profile · {profile?.name} · {server?.name}</Text>
      </Surface>
      <Group>
        <ListRow icon="switch" title="Switch profile or server" onPress={() => setSwitching(true)} />
        <ListRow icon="user" title="Profiles" subtitle="Each profile keeps its own history, Watchlist and preferences." onPress={() => push('SettingsSection', {section: 'profiles'})} />
        <ListRow icon="shield" title="Password and two-step verification" onPress={() => push('SettingsSection', {section: 'security'})} />
        <ListRow icon="phone" title="Devices" onPress={() => push('SettingsSection', {section: 'devices'})} />
        <ListRow icon="network" title="Manage your Portico Account" subtitle="Password, sign-in methods and security" onPress={() => push('Account')} />
      </Group>
      {m.isTV ? <NoticeBox tone="info" title="Manage profiles, PINs and two-step verification on your phone or computer" body="Scan the code in About, or go to getportico.tv/account." /> : null}
      <View><Button label={m.isTV ? 'Sign out of this TV' : 'Sign out'} icon="signOut" variant="danger" onPress={() => void signOut()} /></View>
      <ProfileSwitcher visible={switching} onClose={() => setSwitching(false)} />
    </>
  );
}

export function ServersSection() {
  const client = useClient();
  const {server, setAuthStart} = useSession();
  const toast = useToast();
  const servers = useQuery(c => c.session.servers(), []);
  const [switching, setSwitching] = useState<'server' | 'profile'>();
  const [disconnect, setDisconnect] = useState(false);
  return (
    <>
      <Surface style={{gap: 4}}>
        <Text variant="label" tone="tertiary">Connected to</Text>
        <Text variant="heading">{server?.name}</Text>
        <Text variant="caption" tone="secondary">{server?.host} · Server {server?.version} · Connected through your Portico Account</Text>
      </Surface>
      <Group title="Your servers" footer="Servers you own or that are shared with your Portico Account. Checked 4 min ago.">
        {servers.data?.map(s => <ListRow key={s.id} icon="server" title={s.name} subtitle={serverLine(s)} trailing={<Badge label={s.id === server?.id ? 'Current server' : s.status === 'online' ? 'Online' : 'Can’t reach'} tone={s.id === server?.id ? 'accent' : s.status === 'online' ? 'good' : 'bad'} />} onPress={() => setSwitching('server')} />) ?? <Loading />}
      </Group>
      <View style={{flexDirection: 'row', gap: 8, flexWrap: 'wrap'}}>
        <Button label="Switch profile or server" icon="switch" variant="secondary" onPress={() => setSwitching('server')} />
        <Button label="Refresh servers" icon="refresh" variant="ghost" onPress={() => { servers.refresh(); toast({message: 'Your servers are up to date.'}); }} />
        <Button label="Sign in directly to a server" icon="link" variant="ghost" onPress={() => setAuthStart('DirectSignIn')} />
        <Button label="Disconnect from this server" icon="close" variant="danger" onPress={() => setDisconnect(true)} />
      </View>
      <ServerSwitcher visible={switching === 'server'} onClose={() => setSwitching(undefined)} onChosen={() => setTimeout(() => setSwitching('profile'), 250)} />
      <ProfileSwitcher visible={switching === 'profile'} onClose={() => setSwitching(undefined)} />
      <ConfirmDialog visible={disconnect} onClose={() => setDisconnect(false)} title="Disconnect from this server?" body="This device forgets the server session. Your Portico Account stays signed in and you can reconnect any time." confirmLabel="Disconnect" destructive onConfirm={() => { setDisconnect(false); void client.session.signOut(); }} />
    </>
  );
}

export function NotificationsSection() {
  const client = useClient();
  const m = useMetrics();
  const {isAdmin} = useSession();
  const [view, setView] = useState<'unread' | 'all' | 'archived'>('unread');
  const [audience, setAudience] = useState<'all' | 'profile' | 'admin'>('all');
  const [menu, setMenu] = useState<Notice>();
  const q = useQuery(c => c.notifications.list(view), [view], ['notifications']);
  const list = (q.data ?? []).filter(n => audience === 'all' || n.audience === audience);
  return (
    <>
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 8, alignItems: 'center'}}>
        <Segmented value={view} onChange={setView} options={[{value: 'unread', label: 'Unread'}, {value: 'all', label: 'All'}, {value: 'archived', label: 'Archived'}]} />
        {isAdmin ? <Segmented value={audience} onChange={setAudience} options={[{value: 'all', label: 'Everything'}, {value: 'profile', label: 'For you'}, {value: 'admin', label: 'Server'}]} /> : null}
        <View style={{flex: 1}} />
        <Button size="sm" icon="check" label="Mark all read" variant="ghost" onPress={() => void client.notifications.markAll()} />
      </View>
      {q.loading && !q.data ? <Loading label="Loading notifications" /> : list.length ? (
        <Group>
          {list.map(n => (
            <ListRow key={n.id} icon={n.kind === 'feedback' ? 'flag' : n.kind === 'download' ? 'download' : n.kind === 'recording' ? 'recordings' : n.kind === 'security' ? 'shield' : n.kind === 'broadcast' ? 'mail' : 'server'} title={n.title} subtitle={`${n.body}\n${ago(n.at)} · ${n.audience === 'admin' ? 'Server' : 'For you'}`}
              trailing={!n.read ? <View style={{width: 10, height: 10, borderRadius: 5, backgroundColor: colors.accent}} /> : n.importance === 'urgent' ? <Badge label="Urgent" tone="bad" /> : undefined} onPress={() => setMenu(n)} chevron={false} />
          ))}
        </Group>
      ) : <EmptyState icon="bell" title={view === 'unread' ? 'You’re all caught up' : view === 'archived' ? 'Nothing archived' : 'No notifications'} body="Messages from your server, such as replies to a report or a finished download, appear here." />}
      <ActionMenu visible={!!menu} onClose={() => setMenu(undefined)} title={menu?.title} subtitle={menu?.body} items={[
        {id: 'read', label: menu?.read ? 'Mark unread' : 'Mark read', icon: 'check'},
        {id: 'archive', label: menu?.archived ? 'Move back to inbox' : 'Archive', icon: 'archive'},
        {id: 'open', label: menu?.kind === 'download' ? 'View downloads' : menu?.kind === 'recording' ? 'View recordings' : menu?.kind === 'security' ? 'Review devices' : 'Open', icon: 'forward', hidden: !menu || !['download', 'recording', 'security'].includes(menu.kind)},
      ]} onSelect={id => {
        const n = menu!;
        setMenu(undefined);
        if (id === 'read') void client.notifications.mark(n.id, !n.read);
        if (id === 'archive') void client.notifications.archive(n.id, !n.archived);
        if (id === 'open') { void client.notifications.mark(n.id, true); if (n.kind === 'download') push('Downloads'); else if (n.kind === 'recording') root('Live', {view: 'recordings'}); else root('Admin', {section: 'access'}); }
      }} />
      {m.isTV ? null : null}
    </>
  );
}

export function AboutSection() {
  const m = useMetrics();
  const {server} = useSession();
  return (
    <>
      <View style={{gap: m.space.sm, paddingVertical: m.space.sm}}>
        <Wordmark height={m.isTV ? 56 : 36} />
        <Text variant="caption" tone="tertiary">Portico Classic for {m.isTV ? 'Apple TV' : m.isTablet ? 'iPad' : 'iPhone'} · Version 0.1.0 (1)</Text>
      </View>
      <Group>
        <ListRow title="App version" value="0.1.0 (1)" />
        <ListRow title="Server" value={server?.name} />
        <ListRow title="Server version" value={server?.version} />
      </Group>
      <Surface style={{gap: 10}}>
        <Text variant="caption" tone="secondary">Terms of Use and Privacy Policy: getportico.tv/terms and getportico.tv/privacy. Metadata and artwork are provided by TMDB and other providers configured on your server. This product uses the TMDB API but is not endorsed or certified by TMDB.</Text>
        <Text variant="caption" tone="secondary">Recommendation tags and similar titles in part from the Portico Title Dataset (CC BY-SA 4.0), built from Wikipedia and Wikidata.</Text>
        <Text variant="caption" tone="secondary">Fixture artwork: public-domain film posters and stills from Wikimedia Commons; Blender Foundation open movies (CC BY, blender.org); photographs from Unsplash via picsum.photos; other covers made for this sample library.</Text>
      </Surface>
      <View style={{flexDirection: 'row', gap: m.space.lg, alignItems: 'center'}}>
        <View style={{padding: 12, backgroundColor: '#fff', borderRadius: 12}}><QRCode value="https://getportico.tv/help" size={m.isTV ? 160 : 110} /></View>
        <View style={{flex: 1, gap: 4}}><Text variant="bodyStrong">Help and support</Text><Text variant="caption" tone="secondary">Scan the code with your phone, or go to getportico.tv/help on a computer.</Text></View>
      </View>
    </>
  );
}

export function ProfilesSection() {
  const client = useClient();
  const m = useMetrics();
  const toast = useToast();
  const {profile: current} = useSession();
  const q = useQuery(c => c.session.profiles(), [], ['account', 'session']);
  const [edit, setEdit] = useState<Profile>();
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState('');
  const [kids, setKids] = useState(false);
  const {busy, run, error} = useAction();
  const list = q.data ?? [];
  return (
    <>
      {m.isTV ? <NoticeBox tone="info" title="Manage profiles on your phone or computer" body="Profiles, PINs and limits are easier to change in Portico on iPhone, iPad or the web. You can still review them here." /> : null}
      <Group title="Profiles" footer="An account can have up to 8 profiles. Profiles, PINs and limits are managed from the main profile.">
        {list.map((p, i) => (
          <ListRow key={p.id} left={<Avatar name={p.name} art={p.art} size={m.isTV ? 52 : 38} />} title={p.name} subtitle={[p.main ? 'Main profile' : undefined, p.kids ? 'Kids profile' : undefined, p.pin ? 'Protected by a PIN' : undefined, p.id === current?.id ? 'Current' : undefined].filter(Boolean).join(' · ') || 'No PIN'} onPress={() => setEdit(p)}
            trailing={!m.isTV && i > 0 ? <View style={{flexDirection: 'row'}}><IconButton icon="chevronUp" label="Move up" size={34} disabled={i <= 1} onPress={() => void client.account.moveProfile(p.id, -1)} /><IconButton icon="chevronDown" label="Move down" size={34} disabled={i === list.length - 1} onPress={() => void client.account.moveProfile(p.id, 1)} /></View> : undefined} />
        ))}
      </Group>
      <View><Button label="Add profile" icon="plus" variant="secondary" disabled={list.length >= 8} onPress={() => setAdding(true)} /></View>
      <Sheet visible={adding} onClose={() => setAdding(false)} title="Add a profile" size="sm" footer={<><Button label="Cancel" variant="ghost" onPress={() => setAdding(false)} /><Button label="Add profile" variant="primary" disabled={!name.trim()} loading={busy} onPress={() => void run(async () => { await client.account.addProfile(name.trim(), 'sky', kids); setAdding(false); toast({message: `${name.trim()} added`}); setName(''); })} /></>}>
        {error ? <NoticeBox tone="error" body={error} /> : null}
        <TextField label="Name" value={name} onChangeText={setName} autoFocus />
        <Toggle label="Kids profile" help="Shows only titles rated for children, and hides Live TV channels that aren’t for kids." value={kids} onChange={setKids} />
      </Sheet>
      {edit ? <ProfileEditor p={edit} onClose={() => setEdit(undefined)} /> : null}
    </>
  );
}

function ProfileEditor({p, onClose}: {p: Profile; onClose: () => void}) {
  const client = useClient();
  const m = useMetrics();
  const toast = useToast();
  const [mode, setMode] = useState<'menu' | 'rename' | 'color' | 'pin' | 'pin2' | 'removePin' | 'limits' | 'confirmPw' | 'delete'>('menu');
  const [name, setName] = useState(p.name);
  const [pin, setPin] = useState('');
  const [pinError, setPinError] = useState<string>();
  const [pw, setPw] = useState('');
  const [limits, setLimits] = useState(p.limits ?? {rating: 'none', unrated: true, downloads: true, liveTv: true, dvr: true, together: true});
  const {busy, run} = useAction();
  const close = onClose;
  if (mode === 'menu') {
    return <ActionMenu visible onClose={close} title={p.name} subtitle={p.kids ? 'Kids profile' : p.main ? 'Main profile' : 'Profile'} items={[
      {id: 'rename', label: 'Rename…', icon: 'edit'},
      {id: 'color', label: 'Color…', icon: 'sparkle'},
      {id: 'pin', label: p.pin ? 'Change PIN…' : 'Set a PIN…', icon: 'lock'},
      {id: 'removePin', label: 'Remove PIN', icon: 'unlock', hidden: !p.pin},
      {id: 'limits', label: 'Limits and permissions…', icon: 'shield', subtitle: 'What this profile may watch and do', hidden: !!p.main},
      {id: 'forget', label: 'Forget remembered devices', icon: 'phone'},
      {id: 'delete', label: 'Delete profile…', icon: 'trash', destructive: true, hidden: !!p.main},
    ]} onSelect={id => { if (id === 'forget') { close(); toast({message: 'Remembered devices forgotten.'}); return; } setMode(id === 'limits' ? 'confirmPw' : (id as typeof mode)); }} />;
  }
  if (mode === 'rename') return <Sheet visible onClose={close} title={`Rename ${p.name}`} size="sm" footer={<><Button label="Cancel" variant="ghost" onPress={close} /><Button label="Save" variant="primary" loading={busy} onPress={() => void run(async () => { await client.account.updateProfile(p.id, {name}); toast({message: 'Profile name saved.'}); close(); })} /></>}><TextField label="Name" value={name} onChangeText={setName} autoFocus /></Sheet>;
  if (mode === 'color') return (
    <Sheet visible onClose={close} title="Color" subtitle={p.name} size="sm">
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: m.space.md, justifyContent: 'center'}}>
        {Object.keys(profileArt).map(c => <Focusable key={c} onPress={() => void run(async () => { await client.account.updateProfile(p.id, {art: c}); close(); })} focusScale={1.12} accessibilityLabel={`${c} color`}>{({focused}) => <Avatar name={p.name} art={c} size={m.isTV ? 88 : 60} focused={focused || p.art === c} />}</Focusable>)}
      </View>
    </Sheet>
  );
  if (mode === 'pin' || mode === 'pin2') return (
    <Sheet visible onClose={close} title={`PIN for ${p.name}`} subtitle={mode === 'pin' ? 'Choose a four-digit PIN' : 'Enter it again'} size="sm">
      <PinPad key={mode} error={pinError} onComplete={v => { if (mode === 'pin') { setPin(v); setPinError(undefined); setMode('pin2'); } else if (v !== pin) { setPinError('Those PINs don’t match. Try again.'); setMode('pin'); } else void run(async () => { await client.account.updateProfile(p.id, {pinValue: v}); toast({message: `${p.name} now opens with the new PIN`}); close(); }); }} />
    </Sheet>
  );
  if (mode === 'removePin') return <ConfirmDialog visible onClose={close} title={`Remove the PIN from ${p.name}?`} body={`Anyone using this account will be able to open ${p.name}.`} confirmLabel="Remove PIN" onConfirm={() => void run(async () => { await client.account.updateProfile(p.id, {pinValue: null}); toast({message: `PIN removed from ${p.name}`}); close(); })} />;
  if (mode === 'confirmPw') return <Sheet visible onClose={close} title="Confirm it’s you" subtitle="Enter this account’s password to change a profile’s limits." size="sm" footer={<><Button label="Cancel" variant="ghost" onPress={close} /><Button label="Continue" variant="primary" disabled={pw.length < 4} onPress={() => setMode('limits')} /></>}><TextField label="Password" secureTextEntry value={pw} onChangeText={setPw} autoFocus /><TextField label="Code" help="Only if two-step verification is on." keyboardType="number-pad" /></Sheet>;
  if (mode === 'limits') return (
    <Sheet visible onClose={close} title={`Limits for ${p.name}`} subtitle="What this profile may watch and do on this server." size="md" footer={<><Button label="Cancel" variant="ghost" onPress={close} /><Button label="Save limits" variant="primary" loading={busy} onPress={() => void run(async () => { await client.account.updateProfile(p.id, {limits: limits as Profile['limits']}); toast({message: `Limits saved for ${p.name}`}); close(); })} /></>}>
      <Text variant="captionStrong" tone="secondary">Highest rating allowed</Text>
      <Segmented value={limits.rating} onChange={v => setLimits({...limits, rating: v})} options={[{value: 'none', label: 'No limit'}, {value: 'PG-13', label: 'PG-13'}, {value: 'PG', label: 'PG'}, {value: 'G', label: 'G'}]} />
      <Text variant="caption" tone="tertiary">Titles rated above this are hidden everywhere, including search. Rating system: MPA (US).</Text>
      <Toggle label="Allow unrated titles" help="Home videos and anything without a rating." value={limits.unrated} onChange={v => setLimits({...limits, unrated: v})} />
      <Text variant="label" tone="tertiary">Features</Text>
      <Toggle label="Downloads" help="Save titles to a device for offline viewing." value={limits.downloads} onChange={v => setLimits({...limits, downloads: v})} />
      <Toggle label="Live TV" help="Watch live channels." value={limits.liveTv} onChange={v => setLimits({...limits, liveTv: v})} />
      <Toggle label="Recordings" help="Schedule and watch recordings." value={limits.dvr} onChange={v => setLimits({...limits, dvr: v})} />
      <Toggle label="Watch Together" help="Join and start shared viewing." value={limits.together} onChange={v => setLimits({...limits, together: v})} />
      <TextField label="Hidden labels" help="Titles carrying any of these labels are hidden. Separate with commas." placeholder="horror, mature" />
    </Sheet>
  );
  return <ConfirmDialog visible onClose={close} title={`Delete ${p.name}?`} body="Its watch history, Watchlist and saved items are deleted. This can’t be undone. 2 playlists will also be deleted." confirmLabel="Delete profile" destructive busy={busy} onConfirm={() => void run(async () => { await client.account.deleteProfile(p.id); toast({message: `${p.name} deleted`}); close(); })} />;
}

export function SecuritySection() {
  const client = useClient();
  useTopic(['account']);
  const m = useMetrics();
  const toast = useToast();
  const acct = client.session.current()?.account;
  const [sheet, setSheet] = useState<'password' | 'twoStep' | 'off' | 'codes'>();
  const [step, setStep] = useState<'password' | 'scan' | 'codes'>('password');
  const [codes, setCodes] = useState<string[]>([]);
  const [cur, setCur] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [code, setCode] = useState('');
  const {busy, run, error, clearError} = useAction();
  const open = (s: typeof sheet) => { clearError(); setCur(''); setNext(''); setConfirm(''); setCode(''); setStep('password'); setSheet(s); };
  return (
    <>
      <Group title="Signing in">
        <ListRow icon="key" title="Password" subtitle="Used with your username" value="Changed 3 months ago" onPress={() => open('password')} />
        <ListRow icon="shield" title="Two-step verification" subtitle={acct?.twoStep ? `On · ${acct.recoveryCodesLeft} recovery codes left` : 'Ask for a code from an authenticator app as well as the password.'} value={acct?.twoStep ? 'On' : 'Off'} onPress={() => open(acct?.twoStep ? 'off' : 'twoStep')} />
        {acct?.twoStep ? <ListRow icon="list" title="Recovery codes" subtitle="Making new codes stops the old ones working" onPress={() => void run(async () => { setCodes(await client.account.recoveryCodes()); setStep('codes'); setSheet('codes'); })} /> : null}
      </Group>
      <Group title="Everywhere">
        <ListRow icon="signOut" title="Sign out everywhere…" subtitle="Every device signed in to this account, including this one, will need to sign in again." danger onPress={() => push('SettingsSection', {section: 'devices'})} />
      </Group>
      <Sheet visible={sheet === 'password'} onClose={() => setSheet(undefined)} title="Change password" size="sm" footer={<><Button label="Cancel" variant="ghost" onPress={() => setSheet(undefined)} /><Button label="Change password" variant="primary" loading={busy} disabled={!cur || next.length < 8 || next !== confirm} onPress={() => void run(async () => { await client.account.changePassword(cur, next); setSheet(undefined); toast({message: 'Password changed. Devices that are already signed in stay signed in.'}); })} /></>}>
        {error ? <NoticeBox tone="error" body={error} /> : null}
        <TextField label="Current password" secureTextEntry value={cur} onChangeText={setCur} />
        <TextField label="New password" secureTextEntry value={next} onChangeText={setNext} help="At least 8 characters. Longer is stronger." />
        <TextField label="Confirm new password" secureTextEntry value={confirm} onChangeText={setConfirm} error={confirm && confirm !== next ? 'The new passwords don’t match.' : undefined} />
        <PasswordStrength value={next} />
      </Sheet>
      <Sheet visible={sheet === 'twoStep' || sheet === 'codes'} onClose={() => setSheet(undefined)} title={step === 'codes' ? 'Save your recovery codes' : 'Turn on two-step verification'} subtitle={step === 'password' ? 'First, confirm this account’s password.' : step === 'scan' ? 'Scan with your authenticator app, then enter the 6-digit code it shows.' : 'Each code works once if you lose your authenticator. They aren’t shown again.'} size="sm"
        footer={step === 'password' ? <Button label="Continue" variant="primary" disabled={cur.length < 4} onPress={() => setStep('scan')} /> : step === 'scan' ? <Button label="Verify" variant="primary" loading={busy} disabled={code.length !== 6} onPress={() => void run(async () => { const r = await client.account.setTwoStep(true, code); setCodes(r.recoveryCodes ?? []); setStep('codes'); })} /> : <Button label="I’ve saved them" variant="primary" onPress={() => { setSheet(undefined); toast({message: 'Two-step verification is on'}); }} />}>
        {error ? <NoticeBox tone="error" body={error} /> : null}
        {step === 'password' ? <TextField label="Password" secureTextEntry value={cur} onChangeText={setCur} autoFocus /> : null}
        {step === 'scan' ? (
          <View style={{alignItems: 'center', gap: m.space.md}}>
            <View style={{padding: 14, backgroundColor: '#fff', borderRadius: 14}}><QRCode value="otpauth://totp/Portico:justin?secret=JBSWY3DPEHPK3PXP&issuer=Portico" size={m.isTV ? 220 : 170} /></View>
            <Text variant="caption" tone="tertiary">Can’t scan? Enter this key:</Text>
            <Text variant="bodyStrong" style={{letterSpacing: 2}}>JBSW Y3DP EHPK 3PXP</Text>
            <TextField label="Code" keyboardType="number-pad" value={code} onChangeText={setCode} maxLength={6} style={{alignSelf: 'stretch'}} />
          </View>
        ) : null}
        {step === 'codes' ? <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 10}}>{codes.map(c => <View key={c} style={{paddingVertical: 8, paddingHorizontal: 14, borderRadius: 8, backgroundColor: colors.raisedSoft}}><Text variant="bodyStrong" style={{fontFamily: 'Menlo'}}>{c}</Text></View>)}</View> : null}
      </Sheet>
      <ConfirmDialog visible={sheet === 'off'} onClose={() => setSheet(undefined)} title="Turn off two-step verification?" body="Anyone with the password will be able to sign in. Existing recovery codes stop working." confirmLabel="Turn off" destructive busy={busy} onConfirm={() => void run(async () => { await client.account.setTwoStep(false); setSheet(undefined); toast({message: 'Two-step verification is off'}); })}>
        <TextField label="Password" secureTextEntry value={cur} onChangeText={setCur} />
      </ConfirmDialog>
    </>
  );
}

export function PasswordStrength({value}: {value: string}) {
  const score = Math.min(4, Math.floor(value.length / 4) + (/[^a-z]/i.test(value) ? 1 : 0));
  const label = ['Too short', 'Weak', 'Fair', 'Good', 'Strong'][score];
  return (
    <View style={{gap: 4}}>
      <View style={{flexDirection: 'row', gap: 4}}>{[0, 1, 2, 3].map(i => <View key={i} style={{flex: 1, height: 4, borderRadius: 2, backgroundColor: i < score ? (score >= 3 ? colors.healthy : colors.amber) : colors.track}} />)}</View>
      <Text variant="micro" tone="tertiary">{value ? label : 'At least 8 characters. Longer is stronger.'}</Text>
    </View>
  );
}

export function DevicesSection() {
  const client = useClient();
  const toast = useToast();
  const m = useMetrics();
  const q = useQuery(c => c.account.devices(), [], ['account']);
  const [menu, setMenu] = useState<(typeof q.data & object)[number]>();
  const [everywhere, setEverywhere] = useState(false);
  const [rename, setRename] = useState<string>();
  const [name, setName] = useState('');
  const platform = (p: string) => ({web: 'Web browser', ios: 'iPhone or iPad', tvos: 'Apple TV', android: 'Android', androidtv: 'Android TV'} as Record<string, string>)[p];
  return (
    <>
      <Group title="Devices" footer="Everywhere this account is signed in to this server.">
        {q.data?.map(d => <ListRow key={d.id} icon={d.platform === 'tvos' || d.platform === 'androidtv' ? 'tvDevice' : d.platform === 'web' ? 'laptop' : 'phone'} title={d.current ? `${d.name} (this device)` : d.name} subtitle={`${platform(d.platform)} · ${d.app} · Last seen: ${d.lastSeen}\nFirst seen ${d.firstSeen} · ${d.location}`} trailing={d.state === 'waiting' ? <Badge label="Waiting for approval" tone="warn" /> : d.trusted ? <Badge label="Trusted" tone="good" /> : undefined} onPress={() => setMenu(d)} />) ?? <Loading />}
      </Group>
      <View><Button label="Sign out everywhere…" icon="signOut" variant="danger" onPress={() => setEverywhere(true)} /></View>
      <ActionMenu visible={!!menu} onClose={() => setMenu(undefined)} title={menu?.name} subtitle={menu ? platform(menu.platform) : undefined} items={[
        {id: 'rename', label: 'Rename…', icon: 'edit'},
        {id: 'trust', label: menu?.trusted ? 'Stop trusting this device' : 'Trust this device', icon: 'shield', subtitle: 'Skips approval the next time this device signs in.'},
        {id: 'out', label: menu?.current ? 'Sign out this device' : 'Sign this device out…', icon: 'signOut', destructive: true},
        {id: 'remove', label: 'Remove device…', icon: 'trash', destructive: true, hidden: menu?.current},
      ]} onSelect={async id => {
        const d = menu!;
        setMenu(undefined);
        if (id === 'rename') { setName(d.name); setRename(d.id); }
        if (id === 'trust') await client.account.trustDevice(d.id, !d.trusted);
        if (id === 'out' || id === 'remove') { await client.account.signOutDevice(d.id); toast({message: `${d.name} signed out.`}); }
      }} />
      <Sheet visible={!!rename} onClose={() => setRename(undefined)} title="Rename device" size="sm" footer={<Button label="Save name" variant="primary" onPress={async () => { await client.account.renameDevice(rename!, name); setRename(undefined); }} />}><TextField label="Device" value={name} onChangeText={setName} /></Sheet>
      <ConfirmDialog visible={everywhere} onClose={() => setEverywhere(false)} title="Sign out everywhere?" body="Every device, including this one, is signed out of this server. Downloads already on a device stay until it next connects." confirmLabel="Sign out everywhere" destructive onConfirm={async () => { await client.account.signOutEverywhere(); setEverywhere(false); toast({message: 'Other devices signed out.'}); }} />
      {m.isTV ? null : null}
    </>
  );
}

export function LinkTvSection() {
  const client = useClient();
  const m = useMetrics();
  const toast = useToast();
  const [code, setCode] = useState('');
  const [device, setDevice] = useState<{device: string; platform: string; location: string}>();
  const [done, setDone] = useState<string>();
  const {busy, run, error} = useAction();
  if (done) return <EmptyState icon="check" title={`${done} is signing in now.`} body="You can put your phone down; the TV finishes by itself." action={{label: 'Enter another code', onPress: () => { setDone(undefined); setDevice(undefined); setCode(''); }}} />;
  return (
    <>
      <Text variant="body" tone="secondary">Type the code shown on the TV to sign it in. On the TV, choose Sign in, then Use a code from another device.</Text>
      {error ? <NoticeBox tone="error" body={error} /> : null}
      {!device ? (
        <>
          <TextField label="Enter the code shown on the TV" placeholder="K7QD-2MXF" value={code} onChangeText={t => setCode(t.toUpperCase())} autoCapitalize="characters" autoCorrect={false} maxLength={9} autoFocus />
          <View><Button label="Review device" variant="primary" disabled={code.replace(/[^A-Z0-9]/g, '').length < 8} loading={busy} onPress={() => void run(async () => setDevice(await client.session.approveDevice(code)))} /></View>
        </>
      ) : (
        <Surface style={{gap: m.space.md}}>
          <Text variant="heading">Is this your TV?</Text>
          <ListRow icon="tvDevice" title={device.device} subtitle={`${device.platform} · ${device.location}`} />
          <Text variant="caption" tone="tertiary">Only approve a TV you can see showing this code. It will be signed in to your Portico Account. Approving as Justin.</Text>
          <View style={{flexDirection: 'row', gap: 8}}>
            <Button label="Sign it in" icon="check" variant="primary" loading={busy} onPress={() => void run(async () => { await client.session.confirmDevice(code, true); setDone(device.device); })} />
            <Button label="Not mine" variant="ghost" onPress={() => void run(async () => { await client.session.confirmDevice(code, false); setDevice(undefined); setCode(''); toast({message: 'Request declined.'}); })} />
          </View>
        </Surface>
      )}
    </>
  );
}
