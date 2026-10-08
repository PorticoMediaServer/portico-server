import React, {useState} from 'react';
import {ago} from '../../design/time';
import {ScrollView, View} from 'react-native';
import QRCode from 'react-native-qrcode-svg';
import {useRoute, type RouteProp} from '@react-navigation/native';
import {useQuery, useServices} from '../../data/hooks';
import type {DeviceSession} from '../../data/types';
import {Button} from '../../design/Button';
import {Badge, Chip, ListRow, Panel, TextField, ToggleRow} from '../../design/Controls';
import {ui} from '../../design/overlays';
import {EmptyState} from '../../design/States';
import {Text} from '../../design/Text';
import {colors, isTV, u} from '../../design/tokens';
import {Screen} from '../../nav/Screen';
import {push, replaceAll, type Routes} from '../../nav/routes';
import {useSession} from '../../state/session';
import {ProfilesSettingsScreen} from './Settings';

type Tab = NonNullable<NonNullable<Routes['Account']>['tab']>;
const TABS: {id: Tab; label: string}[] = [{id: 'profiles', label: 'Profiles'}, {id: 'security', label: 'Security'}, {id: 'sessions', label: 'Where you’re signed in'}, {id: 'devices', label: 'Devices'}, {id: 'notifications', label: 'Notifications'}, {id: 'servers', label: 'Servers'}, {id: 'invitations', label: 'Invitations'}];
const platform = (p: DeviceSession['platform']) => ({web: 'Web browser', ios: 'iPhone or iPad', tvos: 'Apple TV', android: 'Android', androidtv: 'Android TV'}[p]);

/** The Portico Account workspace (web: /account): profiles, security, sessions, devices, notifications, servers, invitations. */
export function AccountScreen() {
  const route = useRoute<RouteProp<Routes, 'Account'>>();
  const [tab, setTab] = useState<Tab>(route.params?.tab ?? 'profiles');
  React.useEffect(() => {
    if (route.params?.tab) setTab(route.params.tab);
  }, [route.params?.tab]);
  const session = useSession();
  return (
    <Screen heading="Portico Account" eyebrow={`${session.profile?.name} · justin@example.com`}>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{gap: u(8), marginBottom: u(16)}}>
        {TABS.map((t, i) => <Chip key={t.id} label={t.label} active={t.id === tab} onPress={() => setTab(t.id)} hasTVPreferredFocus={i === 0} />)}
      </ScrollView>
      {tab === 'profiles' ? <Profiles /> : null}
      {tab === 'security' ? <Security /> : null}
      {tab === 'sessions' ? <Sessions /> : null}
      {tab === 'devices' ? <Devices /> : null}
      {tab === 'notifications' ? <AccountNotifications /> : null}
      {tab === 'servers' ? <Servers /> : null}
      {tab === 'invitations' ? <Invitations /> : null}
    </Screen>
  );
}

function Profiles() {
  return (
    <View>
      <Text variant="body" tone="secondary" style={{marginBottom: 10}}>Each profile keeps its own history, Watchlist and preferences on every server you use. Limits for a profile are set on each server by its owner.</Text>
      <Button label="Manage profiles" icon="users" onPress={() => push('SettingsProfiles')} style={{alignSelf: 'flex-start'}} />
    </View>
  );
}

function Security() {
  const s = useServices();
  const acct = useQuery('account', () => s.account.account());
  const a = acct.data;
  if (!a) return null;
  const twoStepOn = () => ui.sheet({title: 'Turn on two-step verification', render: close => (
    <View style={{alignItems: 'center'}}>
      <Text variant="heading">Scan with your authenticator app</Text>
      <Text variant="caption" tone="secondary" style={{marginBottom: 12}}>Then enter the 6-digit code it shows.</Text>
      <View style={{padding: 12, backgroundColor: '#fff', borderRadius: 12}}><QRCode value="otpauth://totp/Portico:justin?secret=JBSWY3DPEHPK3PXP&issuer=Portico" size={isTV ? 260 : 170} /></View>
      <Text variant="caption" tone="tertiary" style={{marginTop: 10}}>Can’t scan? Enter this key: JBSW Y3DP EHPK 3PXP</Text>
      <TextField label="Code" value="" onChangeText={() => {}} keyboardType="number-pad" style={{alignSelf: 'stretch', marginTop: 10}} />
      <Button label="Turn on" variant="primary" hasTVPreferredFocus onPress={() => { s.account.updateAccount({twoStep: true}); ui.toast('Two-step verification is on'); close(); }} />
    </View>
  )});
  return (
    <View>
      <Panel title="Signing in">
        <ListRow icon="key" label="Password" help="Used with your username" value="Change" onPress={() => ui.prompt({title: 'Change password', fields: [{key: 'c', label: 'Current password', secure: true}, {key: 'a', label: 'New password', secure: true, help: 'At least 8 characters'}], submitLabel: 'Change', onSubmit: v => (v.a.length < 8 ? 'At least 8 characters' : (ui.toast('Password changed. Other devices signed in to this account are signed out.'), null))})} hasTVPreferredFocus />
        <ListRow icon="mail" label="Email address" help="Where sign-in codes and recovery messages go" value={a.email} onPress={() => ui.prompt({title: 'Change email address', fields: [{key: 'e', label: 'New email address'}, {key: 'p', label: 'Password', secure: true}], submitLabel: 'Continue', onSubmit: v => { ui.toast(`We sent a code to ${v.e || 'your new address'}. Enter it to finish changing your address.`); return null; }})} />
        <ListRow icon="phone" label="Sign in with Apple" value={a.providers.apple ? 'Connected' : 'Connect'} help={a.providers.apple ? undefined : 'Sign in with your Apple account as well as your password'} onPress={() => (a.providers.apple ? ui.confirm({title: 'Disconnect Apple?', body: 'You won’t be able to sign in with Apple anymore. Your other devices will be signed out; this one stays signed in.', confirmLabel: 'Disconnect', destructive: true, onConfirm: () => s.account.updateAccount({providers: {...a.providers, apple: false}})}) : s.account.updateAccount({providers: {...a.providers, apple: true}}).then(() => ui.toast('Apple connected')))} />
        <ListRow icon="globe" label="Sign in with Google" value={a.providers.google ? 'Connected' : 'Connect'} onPress={() => s.account.updateAccount({providers: {...a.providers, google: !a.providers.google}}).then(() => ui.toast(a.providers.google ? 'Google disconnected' : 'Google connected'))} />
      </Panel>
      <Panel title="Two-step verification" lede="A code from an authenticator app is asked for after your password. Recovery codes get you in if you lose the app." right={<Badge label={a.twoStep ? 'On' : 'Off'} tone={a.twoStep ? 'healthy' : 'neutral'} />}>
        {a.twoStep ? (
          <>
            <ListRow icon="shieldCheck" label="Recovery codes" help={`${a.recoveryCodesLeft} recovery codes left · making new codes stops the old ones working`} value="Make new codes" onPress={() => ui.confirm({title: 'Make new recovery codes?', body: 'Your old codes stop working. Save the new ones somewhere safe.', confirmLabel: 'Make new codes', onConfirm: () => ui.sheet({title: 'New recovery codes', render: () => <Text variant="mono">{['7K2P-QX4M', 'H9DL-3WZR', 'BF6T-N8QC', 'M2XV-R7JH', 'Q4ZN-6LPD', 'T8WC-2KFB', 'V3HJ-9MXS', 'Y6RP-4GDN'].join('\n')}</Text>})})} />
            <ListRow icon="unlock" label="Turn off two-step verification" destructive onPress={() => ui.confirm({title: 'Turn off two-step verification?', body: 'Your password alone will be enough to sign in.', confirmLabel: 'Turn off', destructive: true, onConfirm: () => s.account.updateAccount({twoStep: false}).then(() => ui.toast('Two-step verification is off'))})} />
          </>
        ) : <ListRow icon="shieldCheck" label="Turn on two-step verification" help="Recommended if other people know your password" onPress={twoStepOn} />}
      </Panel>
      <Panel title="Your data">
        <ListRow icon="download" label="Download your data" help="A copy of your Portico Account data, as a JSON file." onPress={() => ui.toast('Your account data is still being prepared. It will be in Notifications when it’s ready.')} />
        <ListRow icon="trash" label="Delete account…" help="Removes your account, its profiles and your access to every server on it." destructive onPress={() => ui.confirm({title: 'Delete your Portico Account?', body: 'Your 4 profiles are deleted. You own Ehler Home Theater. It leaves your account, and its personal data is deleted. You lose access to 2 servers you were invited to.', confirmLabel: 'Delete account', destructive: true, typeToConfirm: 'justin', onConfirm: () => replaceAll('SignIn')})} />
      </Panel>
    </View>
  );
}

function Sessions() {
  const s = useServices();
  const q = useQuery('sessions', () => s.account.sessions());
  return (
    <Panel title="Where you’re signed in" lede="Signing a device out ends its Portico Account sign-in. Servers it was using stop accepting it the next time they check.">
      {(q.data ?? []).map((d, i) => (
        <ListRow key={d.id} icon={d.platform === 'tvos' || d.platform === 'androidtv' ? 'tv' : d.platform === 'web' ? 'laptop' : 'phone'} label={d.name} hasTVPreferredFocus={i === 0} help={`${d.app} on ${platform(d.platform)} · signed in ${new Date(d.signedInAt).toLocaleDateString('en-US', {month: 'short', day: 'numeric'})} · last used ${ago(d.lastUsed)} · ${d.location}`} right={d.current ? <Badge label="This device" /> : undefined}
          onPress={d.current ? undefined : () => ui.confirm({title: `Sign out ${d.name}?`, body: 'It will need to sign in to your Portico Account again.', confirmLabel: 'Sign out', destructive: true, onConfirm: () => s.account.signOutSession(d.id).then(() => ui.toast('Device signed out'))})} />
      ))}
      <Button label="Sign out all other devices" icon="logout" variant="danger" size="sm" style={{margin: u(10), alignSelf: 'flex-start'}} onPress={() => ui.confirm({title: 'Sign out all other devices?', body: 'Every other device is signed out of your Portico Account. This device stays signed in.', confirmLabel: 'Sign out others', destructive: true, onConfirm: () => s.account.signOutOthers().then(() => ui.toast('All other devices signed out'))})} />
    </Panel>
  );
}

function Devices() {
  const s = useServices();
  const q = useQuery('sessions', () => s.account.sessions());
  const waiting = (q.data ?? []).filter(d => d.waiting);
  return (
    <View>
      {waiting.length ? (
        <Panel title="Waiting for your approval" lede="This device signed in but can’t browse or play until you approve it.">
          {waiting.map(d => (
            <View key={d.id} style={{flexDirection: 'row', alignItems: 'center', gap: 10, padding: u(8)}}>
              <Text variant="bodyStrong" style={{flex: 1}}>{d.name} · {platform(d.platform)}</Text>
              <Button label="Deny" size="sm" onPress={() => s.account.signOutSession(d.id)} />
              <Button label="Approve" size="sm" variant="primary" hasTVPreferredFocus onPress={() => s.account.updateSession(d.id, {waiting: false}).then(() => ui.toast(`${d.name} approved`))} />
            </View>
          ))}
        </Panel>
      ) : null}
      <Panel title="Devices on this server" lede="Every device that has signed in to Ehler Home Theater as you.">
        {(q.data ?? []).filter(d => !d.waiting).map(d => (
          <ListRow key={d.id} icon="monitor" label={d.name} help={`${platform(d.platform)} · first seen ${new Date(d.signedInAt).toLocaleDateString('en-US', {month: 'short', day: 'numeric'})} · last seen ${ago(d.lastUsed)}${d.trusted ? ' · Trusted device' : ''}`}
            onPress={() => ui.menu({title: d.name, actions: [
              {label: 'Rename…', icon: 'edit', onPress: () => ui.prompt({title: 'Device name', fields: [{key: 'n', label: 'Name', initial: d.name}], submitLabel: 'Save name', onSubmit: v => { s.account.updateSession(d.id, {name: v.n}); return null; }})},
              {label: d.trusted ? 'Stop trusting' : 'Trust this device', icon: 'shieldCheck', detail: 'Skips approval the next time this device signs in.', onPress: () => s.account.updateSession(d.id, {trusted: !d.trusted})},
              {label: 'Remove device', icon: 'trash', destructive: true, onPress: () => ui.confirm({title: `Remove ${d.name}?`, body: 'It’s signed out and must be approved again.', confirmLabel: 'Remove device', destructive: true, onConfirm: () => s.account.signOutSession(d.id)})},
            ]})} />
        ))}
        <ListRow icon="logout" label="Sign out everywhere" destructive help="Every device, including this one, is signed out of this server. Downloads already on a device stay until it next connects." onPress={() => ui.confirm({title: 'Sign out everywhere?', confirmLabel: 'Sign out everywhere', destructive: true, onConfirm: () => replaceAll('SignIn')})} />
      </Panel>
    </View>
  );
}

function AccountNotifications() {
  const [prefs, setPrefs] = useState({offline: true, storage: true, joined: false});
  return (
    <View>
      <Panel title="Notify me about" lede="Security changes and new sign-ins are always shown.">
        <ToggleRow label="A server that has gone quiet" value={prefs.offline} onChange={v => setPrefs({...prefs, offline: v})} />
        <ToggleRow label="Server storage running low" value={prefs.storage} onChange={v => setPrefs({...prefs, storage: v})} />
        <ToggleRow label="Accepted invitations" value={prefs.joined} onChange={v => setPrefs({...prefs, joined: v})} />
      </Panel>
      <Button label="Open notifications" icon="bell" onPress={() => push('Notifications')} style={{alignSelf: 'flex-start'}} />
    </View>
  );
}

function Servers() {
  const s = useServices();
  const q = useQuery('servers', () => s.account.servers());
  return (
    <Panel title="Servers" lede="The servers this account belongs to, from the server list.">
      {(q.data ?? []).map(sv => (
        <ListRow key={sv.id} icon="server" label={sv.name} help={`${sv.owner ? 'Owner · ' : ''}${sv.status === 'online' ? 'Online' : `Last seen ${ago(sv.lastSeen)}`}`}
          onPress={() => (sv.owner ? ui.toast('You own this server, so it stays on your account.') : ui.confirm({title: `Leave ${sv.name}?`, body: `You lose access to ${sv.name}. Its owner can invite you again.`, confirmLabel: 'Leave', destructive: true, onConfirm: () => s.account.leaveServer(sv.id).then(() => ui.toast(`Left ${sv.name}`))}))} />
      ))}
    </Panel>
  );
}

function Invitations() {
  const s = useServices();
  const q = useQuery('invitations', () => s.account.invitations());
  const [link, setLink] = useState('');
  const waiting = (q.data ?? []).filter(i => i.state === 'waiting');
  const earlier = (q.data ?? []).filter(i => i.state !== 'waiting');
  return (
    <View>
      <Panel title="Waiting for you" lede="Accepting lets one of your profiles use that server. Its owner decides which libraries you see.">
        {!waiting.length ? <EmptyState compact icon="mail" title="No invitations" body="When someone shares their server with your Portico Account, the invitation appears here." /> : null}
        {waiting.map(i => (
          <View key={i.id} style={{padding: u(8)}}>
            <Text variant="bodyStrong">{i.server}</Text>
            <Text variant="caption" tone="tertiary">From {i.inviter} · {i.libraries} libraries · expires {new Date(i.expires).toLocaleDateString('en-US', {month: 'short', day: 'numeric'})}</Text>
            <View style={{flexDirection: 'row', gap: 8, marginTop: 8}}>
              <Button label="Decline" size="sm" onPress={() => s.account.answerInvitation(i.id, false).then(() => ui.toast('Invitation declined'))} />
              <Button label="Accept" size="sm" variant="primary" hasTVPreferredFocus onPress={() => ui.menu({title: 'Which profile should use this server?', actions: ['Justin', 'Ava', 'Leo', 'Kids'].map(n => ({label: n, onPress: () => s.account.answerInvitation(i.id, true).then(() => ui.toast('Invitation accepted. The server appears in your server list once it confirms your access.'))}))})} />
            </View>
          </View>
        ))}
      </Panel>
      <Panel title="Have an invitation link?" lede="Paste a link or code someone sent you to see who invited you and to which server.">
        <TextField label="Invitation link or code" value={link} onChangeText={setLink} />
        <Button label="Review" size="sm" onPress={() => (link.includes('#') || link.length >= 8 ? push('Join') : ui.toast('That isn’t a complete invitation. Paste the whole link, including everything after #.', {tone: 'error'}))} style={{alignSelf: 'flex-start'}} />
      </Panel>
      <Panel title="Earlier">
        {earlier.map(i => <ListRow key={i.id} icon="mail" label={i.server} help={`From ${i.inviter} · ${{accepted: 'Accepted', declined: 'Declined or withdrawn', expired: 'Expired', revoked: 'Access was later removed by the owner', waiting: ''}[i.state]}`} />)}
      </Panel>
    </View>
  );
}

export {ProfilesSettingsScreen, colors};
