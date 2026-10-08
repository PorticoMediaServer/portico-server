import React, {useState} from 'react';
import {Platform, View} from 'react-native';
import {useRoute, type RouteProp} from '@react-navigation/native';
import {useQuery, useServices} from '../../data/hooks';
import type {Profile} from '../../data/types';
import {Wordmark} from '../../design/Brand';
import {Button, IconButton} from '../../design/Button';
import {Badge, ListRow, Panel, Segmented, TextField, ToggleRow} from '../../design/Controls';
import {ui} from '../../design/overlays';
import {Text} from '../../design/Text';
import {colors, fonts, isTV, u} from '../../design/tokens';
import {useLayout} from '../../design/useLayout';
import {Screen} from '../../nav/Screen';
import {back, go, goSection, push, replaceAll, type Routes} from '../../nav/routes';
import {isManager, useSession} from '../../state/session';
import {plural} from '../../design/format';

export function SettingsScreen() {
  const s = useServices();
  const session = useSession();
  const notes = useQuery('notifications', () => s.account.notifications());
  const unread = (notes.data ?? []).filter(n => !n.read && !n.archived).length;
  return (
    <Screen heading="Settings">
      <Panel>
        <ListRow icon="user" label={session.profile?.name ?? 'Profile'} help={session.via === 'account' ? 'Portico Account · profiles, security and devices' : 'Server account'} onPress={() => push('Account')} hasTVPreferredFocus />
        <ListRow icon="server" label={session.server?.name ?? 'Server'} help={`${session.server?.host ?? ''} · ${session.via === 'account' ? 'Connected through your Portico Account' : 'Connected directly'}`} onPress={() => push('SettingsServers')} />
      </Panel>
      <Panel>
        <ListRow icon="settings" label="Preferences" help="Playback, subtitles and appearance" onPress={() => push('SettingsPreferences')} />
        <ListRow icon="users" label="Profiles" help="Names, pictures, PINs and limits" onPress={() => push('SettingsProfiles')} />
        <ListRow icon="bell" label="Notifications" help="Messages from your server" value={unread ? `${unread} new` : undefined} onPress={() => goSection('Notifications')} />
        {!isTV ? <ListRow icon="download" label="Downloads" help="Saved on this device to watch offline" onPress={() => goSection('Downloads')} /> : null}
        <ListRow icon="together" label="Watch Together" help="Watch in step with others on this server" onPress={() => goSection('Together')} />
        {isTV ? <ListRow icon="qr" label="Sign in another device" help="Show a code to approve from your phone" onPress={() => go('LinkTV')} /> : <ListRow icon="tv" label="Link a TV" help="Sign a TV in with the code it shows" onPress={() => go('DeviceApprove')} />}
      </Panel>
      <Panel>
        <ListRow icon="info" label="About Portico" onPress={() => push('SettingsAbout')} />
        <ListRow icon="list" label="Screen index" help="Every screen and state, for review" onPress={() => push('ScreenIndex')} />
      </Panel>
      {isManager(session.role) ? (
        <Panel title="You manage this server" lede="Libraries, sharing, remote access and everything else about the server live under Server.">
          <Button label="Open server console" icon="terminal" onPress={() => goSection('Admin', {section: 'overview'})} style={{alignSelf: 'flex-start'}} />
        </Panel>
      ) : null}
      <Button label="Sign out" icon="logout" variant="danger" style={{alignSelf: 'flex-start'}} onPress={() => ui.confirm({title: `Sign out of ${session.server?.name} on this ${isTV ? 'TV' : 'device'}?`, body: isTV ? 'You’ll need to sign this TV in again.' : 'Downloads stay on this device.', confirmLabel: 'Sign out', destructive: true, onConfirm: () => { session.signOut(); replaceAll('SignIn'); }})} />
    </Screen>
  );
}

// ---------------------------------------------------------------- Preferences

export function PreferencesScreen() {
  const s = useServices();
  const [home, setHome] = useState('Automatic');
  const [away, setAway] = useState('Automatic');
  const [res, setRes] = useState({home: '4K', away: '1080p', cell: '720p'});
  const [hdr, setHdr] = useState({home: true, away: true, cell: false});
  const [audioLangs, setAudioLangs] = useState(['English', 'Original language']);
  const [subLangs, setSubLangs] = useState(['English', 'Spanish']);
  const [subMode, setSubMode] = useState<'off' | 'forced' | 'always'>('forced');
  const [subSize, setSubSize] = useState<'small' | 'medium' | 'large'>('medium');
  const [view, setView] = useState<'grid' | 'list'>('grid');
  const [motion, setMotion] = useState(false);
  const [textSize, setTextSize] = useState<'standard' | 'large'>('standard');
  const [receive, setReceive] = useState(true);
  const [autoplay, setAutoplay] = useState(true);
  const [skip, setSkip] = useState(false);
  const pick = (title: string, options: string[], cur: string, set: (v: string) => void) => ui.menu({title, actions: options.map(o => ({label: o, checked: o === cur, onPress: () => set(o)}))});
  const langs = ['English', 'Original language', 'Spanish', 'French', 'German', 'Japanese', 'Korean', 'Italian', 'Portuguese'];
  const editLangs = (title: string, cur: string[], set: (v: string[]) => void) => ui.sheet({title, subtitle: 'In order of preference', render: close => <LangEditor cur={cur} all={langs} onDone={v => { set(v); close(); }} />});
  return (
    <Screen heading="Preferences" eyebrow="Settings">
      <View style={{flexDirection: 'row', gap: 8, marginBottom: u(12)}}><Badge label="Saved to your profile" tone="healthy" /><Badge label="This device" tone="neutral" /></View>
      <Panel title="Playback quality" lede="Mode, resolution ceiling and HDR for each network.">
        <ListRow label="Home network quality" value={home} onPress={() => pick('Home network', ['Automatic', 'Original', 'Convert'], home, setHome)} hasTVPreferredFocus />
        <ListRow label="Home network: highest resolution" value={res.home} onPress={() => pick('Highest resolution', ['4K', '1080p', '720p', '480p'], res.home, v => setRes({...res, home: v}))} />
        <ToggleRow label="Allow HDR on home network" value={hdr.home} onChange={v => setHdr({...hdr, home: v})} />
        <ListRow label="Away from home quality" value={away} onPress={() => pick('Away from home', ['Automatic', 'Original', 'Convert'], away, setAway)} />
        <ListRow label="Away from home: highest resolution" value={res.away} onPress={() => pick('Highest resolution', ['4K', '1080p', '720p', '480p'], res.away, v => setRes({...res, away: v}))} />
        <ToggleRow label="Allow HDR away from home" value={hdr.away} onChange={v => setHdr({...hdr, away: v})} />
        {!isTV ? <ListRow label="Cellular: highest resolution" value={res.cell} onPress={() => pick('Cellular', ['1080p', '720p', '480p'], res.cell, v => setRes({...res, cell: v}))} /> : null}
        <ToggleRow label="Play the next episode automatically" value={autoplay} onChange={setAutoplay} />
        <ToggleRow label="Skip intros automatically" help="Otherwise a Skip intro button appears" value={skip} onChange={setSkip} />
      </Panel>
      <Panel title="Languages and subtitles">
        <ListRow label="Audio languages" value={audioLangs.join(', ')} onPress={() => editLangs('Audio languages', audioLangs, setAudioLangs)} />
        <ListRow label="Subtitle languages" value={subLangs.join(', ')} onPress={() => editLangs('Subtitle languages', subLangs, setSubLangs)} />
        <Text variant="captionStrong" tone="secondary" style={{marginLeft: u(14), marginTop: u(8)}}>Show subtitles</Text>
        <View style={{padding: u(10)}}><Segmented value={subMode} onChange={setSubMode} options={[{value: 'off', label: 'Off'}, {value: 'forced', label: 'Only forced'}, {value: 'always', label: 'Always'}]} /></View>
        <Text variant="captionStrong" tone="secondary" style={{marginLeft: u(14)}}>Subtitle size</Text>
        <View style={{padding: u(10)}}><Segmented value={subSize} onChange={setSubSize} options={[{value: 'small', label: 'Small'}, {value: 'medium', label: 'Medium'}, {value: 'large', label: 'Large'}]} /></View>
      </Panel>
      <Panel title="On this device" lede="Conveniences kept on this device only.">
        <Text variant="captionStrong" tone="secondary" style={{marginLeft: u(14)}}>Library view</Text>
        <View style={{padding: u(10)}}><Segmented value={view} onChange={setView} options={[{value: 'grid', label: 'Grid', icon: 'grid'}, {value: 'list', label: 'List', icon: 'list'}]} /></View>
        <ToggleRow label="Reduce motion" help={isTV ? 'Shorter transitions and no zoom on focus. Also on whenever Reduce Motion is on in Apple TV Settings.' : 'Shorter transitions. Also on whenever Reduce Motion is on in iOS Settings.'} value={motion} onChange={setMotion} />
        {isTV ? <ToggleRow label="Large text" help="Makes text larger across Portico on this TV." value={textSize === 'large'} onChange={v => setTextSize(v ? 'large' : 'standard')} /> : null}
        {isTV ? <ToggleRow label="Play from your other devices" help="Phones on this profile can send what they’re watching to this TV" value={receive} onChange={setReceive} /> : null}
        <ListRow label="Language and time zone" value="Same as this device" help="Change the language and time zone in Portico on the web." />
      </Panel>
      <Panel title="Recommendations">
        <ListRow icon="refresh" label="Reset recommendations" help="Start over with what Portico recommends to you." onPress={() => ui.confirm({title: 'Reset recommendations?', body: 'Portico will forget what it has learned about your taste. Your watch history, ratings and lists stay.', confirmLabel: 'Reset', destructive: true, onConfirm: () => s.user.resetRecommendations().then(() => ui.toast('Recommendations reset.'))})} />
      </Panel>
    </Screen>
  );
}

function LangEditor({cur, all, onDone}: {cur: string[]; all: string[]; onDone: (v: string[]) => void}) {
  const [list, setList] = useState(cur);
  const move = (i: number, d: number) => setList(l => {
    const n = [...l];
    const j = i + d;
    if (j < 0 || j >= n.length) return l;
    [n[i], n[j]] = [n[j], n[i]];
    return n;
  });
  return (
    <View>
      {list.map((l, i) => (
        <View key={l} style={{flexDirection: 'row', alignItems: 'center'}}>
          <Text variant="bodyStrong" style={{flex: 1, marginLeft: u(14)}}>{i + 1}. {l}</Text>
          <IconButton icon="arrowUp" label={`Move ${l} up`} size={32} variant="ghost" onPress={() => move(i, -1)} />
          <IconButton icon="arrowDown" label={`Move ${l} down`} size={32} variant="ghost" onPress={() => move(i, 1)} />
          <IconButton icon="x" label={`Remove ${l}`} size={32} variant="ghost" onPress={() => setList(list.filter(x => x !== l))} />
        </View>
      ))}
      {!list.length ? <Text variant="caption" tone="tertiary">No languages chosen</Text> : null}
      <Button label="Add language" icon="plus" size="sm" style={{marginTop: 10, alignSelf: 'flex-start'}} onPress={() => ui.menu({title: 'Add language', actions: all.filter(a => !list.includes(a)).map(a => ({label: a, onPress: () => setList([...list, a])}))})} />
      <Button label="Done" variant="primary" style={{marginTop: 14}} onPress={() => onDone(list)} hasTVPreferredFocus />
    </View>
  );
}

// ---------------------------------------------------------------- Profiles and limits

const COLORS = ['#409fd4', '#d4668f', '#3fae78', '#e8bc6a', '#8a5fd0', '#e0763a', '#2fa4a0', '#c2424f'];

export function ProfilesSettingsScreen() {
  const s = useServices();
  const session = useSession();
  const q = useQuery('profiles', () => s.account.profiles());
  const save = (p: Profile) => s.account.saveProfile(p);
  const options = (p: Profile, i: number) => ui.menu({title: p.name, subtitle: p.main ? 'Main profile' : p.kids ? 'Kids profile' : undefined, actions: [
    {label: 'Rename…', icon: 'edit', onPress: () => ui.prompt({title: `Rename ${p.name}`, fields: [{key: 'n', label: 'Name', initial: p.name}], submitLabel: 'Save', onSubmit: v => { if (!v.n.trim()) return 'Enter a name.'; save({...p, name: v.n.trim(), initials: v.n.trim()[0].toUpperCase()}); return null; }})},
    {label: 'Color', icon: 'sparkles', onPress: () => ui.menu({title: 'Color', actions: COLORS.map((c, k) => ({label: ['Blue', 'Rose', 'Green', 'Gold', 'Violet', 'Orange', 'Teal', 'Red'][k], checked: p.color === c, onPress: () => save({...p, color: c})}))})},
    {label: p.picture ? 'Change picture…' : 'Add a picture…', icon: 'photo', onPress: () => ui.toast('Choose a JPEG, PNG or WebP picture smaller than 4 MB.')},
    {label: p.pin ? 'Change PIN…' : 'Set a PIN…', icon: 'lock', onPress: () => ui.prompt({title: `PIN for ${p.name}`, body: 'Four digits. Asked for when switching to this profile.', fields: [{key: 'a', label: 'New PIN', secure: true}, {key: 'b', label: 'Repeat the PIN', secure: true}], submitLabel: 'Save', onSubmit: v => { if (!/^\d{4}$/.test(v.a)) return 'A profile PIN is four digits.'; if (v.a !== v.b) return 'Those PINs don’t match. Try again.'; save({...p, pin: v.a}); ui.toast(`${p.name} now opens with the new PIN`); return null; }})},
    ...(p.pin ? [{label: 'Remove PIN', icon: 'unlock' as const, onPress: () => ui.confirm({title: `Remove the PIN from ${p.name}?`, body: `Anyone using this account will be able to open ${p.name}.`, confirmLabel: 'Remove PIN', onConfirm: () => save({...p, pin: null})})}] : []),
    {label: 'Limits and permissions…', icon: 'shield', detail: 'What this profile may watch and do', onPress: () => push('ProfileLimits', {profileId: p.id})},
    {label: 'Move up', icon: 'arrowUp', disabled: i === 0, onPress: () => s.account.moveProfile(p.id, -1)},
    {label: 'Move down', icon: 'arrowDown', onPress: () => s.account.moveProfile(p.id, 1)},
    ...(!p.main ? [{label: 'Delete profile…', icon: 'trash' as const, destructive: true, onPress: () => ui.confirm({title: `Delete ${p.name}?`, body: 'Its watch history, Watchlist and saved items are deleted. This can’t be undone. 1 playlist will also be deleted.', confirmLabel: 'Delete profile', destructive: true, onConfirm: () => s.account.deleteProfile(p.id).then(() => ui.toast(`${p.name} deleted`))})}] : []),
  ]});
  return (
    <Screen heading="Profiles" eyebrow="Settings">
      <Text variant="body" tone="secondary" style={{marginBottom: u(12)}}>Each profile keeps its own history, Watchlist and preferences.</Text>
      <Panel>
        {(q.data ?? []).map((p, i) => (
          <ListRow key={p.id} label={p.name} hasTVPreferredFocus={i === 0} help={[p.main ? 'Main profile' : null, p.kids ? 'Kids' : null, p.pin ? 'Protected by a PIN' : null, p.id === session.profile?.id ? 'Current' : null].filter(Boolean).join(' · ') || 'Profile'} onPress={() => options(p, i)}
            right={<View style={{width: u(32), height: u(32), borderRadius: u(16), backgroundColor: p.color, alignItems: 'center', justifyContent: 'center'}}><Text style={{fontFamily: fonts.bold, color: colors.actionInk, fontSize: isTV ? 22 : 13}}>{p.initials}</Text></View>} />
        ))}
      </Panel>
      <View style={{flexDirection: 'row', gap: 10, flexWrap: 'wrap'}}>
        <Button label="Add profile" icon="userPlus" onPress={() => (q.data?.length ?? 0) >= 8 ? ui.toast('An account can have up to 8 profiles.') : ui.prompt({title: 'Add a profile', fields: [{key: 'n', label: 'Name'}], submitLabel: 'Add', onSubmit: v => { if (!v.n.trim()) return 'Enter a name.'; s.account.addProfile(v.n.trim(), COLORS[(q.data?.length ?? 0) % COLORS.length], false).then(p => ui.toast(`${p.name} added`)); return null; }})} />
        <Button label="Change password" icon="key" onPress={() => ui.prompt({title: 'Change password', fields: [{key: 'c', label: 'Current password', secure: true}, {key: 'a', label: 'New password', secure: true, help: 'At least 8 characters. Longer is stronger.'}, {key: 'b', label: 'Confirm password', secure: true}], submitLabel: 'Change password', onSubmit: v => { if (v.a.length < 8) return 'Choose a longer password.'; if (v.a !== v.b) return 'The passwords don’t match.'; ui.toast('Password changed. Devices that are already signed in stay signed in.'); return null; }})} />
        <Button label="Forgot PIN…" variant="ghost" onPress={() => ui.prompt({title: 'New PIN for Ava', body: 'Confirm it’s you with your password and a code, then choose a new PIN.', fields: [{key: 'p', label: 'Account password', secure: true}, {key: 'c', label: 'Authenticator or recovery code'}, {key: 'n', label: 'New PIN', secure: true}], submitLabel: 'Set new PIN', onSubmit: () => { ui.toast('PIN changed'); return null; }})} />
      </View>
    </Screen>
  );
}

export function ProfileLimitsScreen() {
  const route = useRoute<RouteProp<Routes, 'ProfileLimits'>>();
  const s = useServices();
  const q = useQuery('profiles', () => s.account.profiles());
  const p = q.data?.find(x => x.id === route.params.profileId);
  const [lim, setLim] = useState(p?.limits);
  React.useEffect(() => {
    if (p && !lim) setLim(p.limits);
  }, [p, lim]);
  if (!p || !lim) return <Screen heading="Limits" />;
  return (
    <Screen heading={`Limits for ${p.name}`} eyebrow="Profiles">
      <Panel title="What they can watch">
        <ListRow label="Highest rating" help="Titles rated higher are hidden" value={lim.maxRating ?? 'No limit'} onPress={() => ui.menu({title: 'Highest rating', actions: ['No limit', 'G', 'PG', 'PG-13', 'R', 'TV-Y7', 'TV-PG', 'TV-14'].map(r => ({label: r === 'No limit' ? r : `${r} (US)`, checked: (lim.maxRating ?? 'No limit') === r, onPress: () => setLim({...lim, maxRating: r === 'No limit' ? null : r})}))})} hasTVPreferredFocus />
        <ToggleRow label="Unrated titles" help="Allow titles without a rating" value={lim.allowUnrated} onChange={v => setLim({...lim, allowUnrated: v})} />
        <TextField label="Hidden labels" value={lim.hiddenLabels.join(', ')} onChangeText={v => setLim({...lim, hiddenLabels: v.split(',').map(x => x.trim()).filter(Boolean)})} help="Titles carrying any of these labels are hidden. Separate with commas." style={{marginHorizontal: u(14), marginTop: 8}} />
      </Panel>
      <Panel title="What they can do">
        <ToggleRow label="Downloads" help="Save titles to a device for offline viewing." value={lim.downloads} onChange={v => setLim({...lim, downloads: v})} />
        <ToggleRow label="Live TV" help="Watch live channels." value={lim.liveTv} onChange={v => setLim({...lim, liveTv: v})} />
        <ToggleRow label="DVR" help="Schedule and watch recordings." value={lim.dvr} onChange={v => setLim({...lim, dvr: v})} />
        <ToggleRow label="Watch Together" help="Join and start shared viewing." value={lim.together} onChange={v => setLim({...lim, together: v})} />
      </Panel>
      <Button label="Save" variant="primary" style={{alignSelf: 'flex-start'}} onPress={() => ui.prompt({title: 'Confirm it’s you', body: 'Enter this account’s password to change a profile’s limits.', fields: [{key: 'p', label: 'Password', secure: true}, {key: 'c', label: 'Code', help: 'Only if two-step verification is on.'}], submitLabel: 'Continue', onSubmit: () => { s.account.saveProfile({...p, limits: lim}).then(() => { ui.toast('Limits saved.'); back(); }); return null; }})} />
    </Screen>
  );
}

// ---------------------------------------------------------------- Servers and About

export function ServersSettingsScreen() {
  const s = useServices();
  const session = useSession();
  const q = useQuery('servers', () => s.account.servers());
  const [checked, setChecked] = useState(Date.now());
  return (
    <Screen heading="Servers" eyebrow="Settings">
      <Panel title="Connected to">
        <ListRow icon="server" label={session.server?.name ?? ''} help={`${session.server?.host} · Server ${session.server?.version} · ${session.via === 'account' ? 'Connected through your Portico Account' : 'Connected directly'}`} />
        <Button label="Disconnect from this server" icon="logout" variant="danger" size="sm" style={{margin: u(10), alignSelf: 'flex-start'}} onPress={() => ui.confirm({title: 'Disconnect from this server?', body: 'This device forgets the server session. Your Portico Account stays signed in and you can reconnect any time.', confirmLabel: 'Disconnect', destructive: true, onConfirm: () => replaceAll('Chooser')})} />
      </Panel>
      <Panel title="Your servers" lede="Servers you own or that are shared with your Portico Account." right={<Button label="Refresh" icon="refresh" size="sm" onPress={() => { q.reload(); setChecked(Date.now()); }} />}>
        {(q.data ?? []).map((sv, i) => (
          <ListRow key={sv.id} icon="server" label={sv.name} hasTVPreferredFocus={i === 0} help={`${sv.status === 'online' ? 'Online' : 'Can’t reach'} · ${sv.role === 'owner' ? 'Owner' : sv.role === 'admin' ? 'Administrator' : 'Member'} · ${plural(sv.libraries, 'library', 'libraries')}`} right={sv.id === session.server?.id ? <Badge label="Current" /> : <Badge label={sv.status === 'online' ? 'Online' : 'Offline'} tone={sv.status === 'online' ? 'healthy' : 'danger'} />} onPress={() => (sv.status === 'online' ? (session.setServer(sv), replaceAll('Profiles')) : go('ServerUnavailable'))} />
        ))}
        <Text variant="caption" tone="tertiary" style={{margin: u(10)}}>Checked {new Date(checked).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'})}</Text>
      </Panel>
      <ListRow icon="login" label="Sign in directly to another server" onPress={() => go('SignIn', {mode: 'direct'})} />
    </Screen>
  );
}

export function AboutScreen() {
  const session = useSession();
  return (
    <Screen heading="About Portico" eyebrow="Settings">
      <View style={{marginVertical: u(10)}}><Wordmark size={isTV ? 48 : 30} /></View>
      <Panel>
        <ListRow label="App" value={`Portico Cinema 0.1.0 · ${Platform.isTV ? 'tvOS' : Platform.OS === 'ios' ? 'iOS' : Platform.OS}`} />
        <ListRow label="Server" value={session.server?.name} />
        <ListRow label="Server version" value={session.server?.version} />
      </Panel>
      <Text variant="caption" tone="tertiary" style={{marginBottom: u(10)}}>Metadata and artwork are provided by TMDB and other providers configured on your server. This product uses the TMDB API but is not endorsed or certified by TMDB. Classic films shown here are in the public domain; the Blender open movies are © Blender Foundation under Creative Commons Attribution; photographs via Unsplash.</Text>
      <Panel>
        <ListRow icon="fileText" label="Terms of Use" onPress={() => ui.toast('Opens getportico.tv/terms')} />
        <ListRow icon="shield" label="Privacy Policy" onPress={() => ui.toast('Opens getportico.tv/privacy')} />
        <ListRow icon="help" label="Help and support" onPress={() => ui.toast('Opens getportico.tv/help')} />
        <ListRow icon="list" label="Screen index" onPress={() => push('ScreenIndex')} />
        <ListRow icon="layers" label="Loading, empty and error states" onPress={() => push('States')} />
      </Panel>
    </Screen>
  );
}

export {useLayout, Segmented};
