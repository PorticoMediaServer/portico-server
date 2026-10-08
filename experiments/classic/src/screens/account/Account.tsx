import {serverLine} from '../../app/format';
import React, {useEffect, useState} from 'react';
import {View} from 'react-native';
import {useRoute} from '@react-navigation/native';
import {useAction, useClient, useQuery, useTopic} from '../../data/DataProvider';
import {Screen, PageHeader} from '../../ui/Page';
import {Button} from '../../ui/Button';
import {Badge, EmptyState, Group, ListRow, Notice, Surface, Tabs, TextField, Toggle} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {ConfirmDialog, Sheet} from '../../ui/Sheet';
import {useMetrics} from '../../theme/layout';
import {useToast} from '../../app/toast';
import {back} from '../../nav/nav';
import {ProfilesSection, SecuritySection} from '../settings/Sections';

type Tab = 'profiles' | 'sessions' | 'security' | 'servers' | 'invitations' | 'notifications';

/** The Portico Account workspace: profiles, where you're signed in, security, servers, invitations, notifications, your data. */
export function AccountScreen() {
  const route = useRoute() as {params?: {tab?: Tab}};
  const m = useMetrics();
  const client = useClient();
  useTopic(['account']);
  const toast = useToast();
  const [tab, setTab] = useState<Tab>(route.params?.tab ?? 'profiles');
  useEffect(() => { if (route.params?.tab) setTab(route.params.tab); }, [route.params?.tab]);
  const acct = client.session.current()?.account;
  const devices = useQuery(c => c.account.devices(), [], ['account']);
  const servers = useQuery(c => c.session.servers(), []);
  const invites = useQuery(c => c.account.invitations(), [], ['account']);
  const [sheet, setSheet] = useState<'email' | 'delete' | 'others' | 'paste' | 'leave' | 'export'>();
  const [value, setValue] = useState('');
  const [leaveId, setLeaveId] = useState<string>();
  const {busy, run, error, clearError} = useAction();
  const open = (s: typeof sheet) => { clearError(); setValue(''); setSheet(s); };
  const waiting = (invites.data ?? []).filter(i => i.state === 'waiting');
  return (
    <Screen contentStyle={{gap: m.space.lg}}>
      <PageHeader eyebrow="Portico Account" title={acct?.displayName ?? 'Portico Account'} subtitle={`${acct?.username} · ${acct?.email}`} onBack={back} />
      <View style={{paddingHorizontal: m.gutter, gap: m.space.lg, maxWidth: m.isTV ? 1300 : 900, width: '100%'}}>
        <Tabs tabs={[{value: 'profiles', label: 'Profiles'}, {value: 'sessions', label: 'Where you’re signed in'}, {value: 'security', label: 'Security'}, {value: 'servers', label: 'Servers'}, {value: 'invitations', label: 'Invitations', count: waiting.length || undefined}, {value: 'notifications', label: 'Notifications'}]} value={tab} onChange={setTab} />
        {tab === 'profiles' ? <><Text variant="caption" tone="tertiary">Each profile keeps its own history, Watchlist and preferences on every server you use. Limits for a profile are set on each server by its owner.</Text><ProfilesSection /></> : null}
        {tab === 'sessions' ? (
          <>
            <Group title="Where you’re signed in" footer="Signing a device out ends its Portico Account sign-in. Servers it was using stop accepting it the next time they check.">
              {devices.data?.map(d => <ListRow key={d.id} icon={d.platform === 'web' ? 'laptop' : d.platform === 'tvos' || d.platform === 'androidtv' ? 'tvDevice' : 'phone'} title={d.current ? `${d.app} on ${d.name} (this device)` : `${d.app} on ${d.name}`} subtitle={`Signed in ${d.firstSeen} · last used ${d.lastSeen.toLowerCase()} · ${d.location}`} trailing={d.current ? <Badge label="This device" tone="accent" /> : <Button size="sm" label="Sign out" variant="ghost" onPress={() => void run(async () => { await client.account.signOutDevice(d.id); toast({message: 'Device signed out'}); })} />} />)}
            </Group>
            <View><Button label="Sign out all other devices" icon="signOut" variant="danger" onPress={() => open('others')} /></View>
          </>
        ) : null}
        {tab === 'security' ? (
          <>
            <Group title="Email address" footer="Where sign-in codes and recovery messages go.">
              <ListRow icon="mail" title={acct?.email ?? 'Not set'} onPress={() => open('email')} value="Change" />
            </Group>
            <Group title="Sign-in methods" footer="Changes here are confirmed with your password.">
              {(['apple', 'google'] as const).map(p => {
                const on = acct?.providers.includes(p);
                return <ListRow key={p} icon={p === 'apple' ? 'sparkle' : 'network'} title={`Sign in with ${p === 'apple' ? 'Apple' : 'Google'}`} subtitle={on ? 'Connected' : `Sign in with your ${p === 'apple' ? 'Apple' : 'Google'} account as well as your password`} trailing={<Button size="sm" label={on ? 'Disconnect' : 'Connect'} variant={on ? 'ghost' : 'secondary'} onPress={() => void run(async () => { await client.account.connectProvider(p, !on); toast({message: `${p === 'apple' ? 'Apple' : 'Google'} ${on ? 'disconnected' : 'connected'}`}); })} />} />;
              })}
            </Group>
            <SecuritySection />
          </>
        ) : null}
        {tab === 'servers' ? (
          <Group title="Servers" footer="The servers this account belongs to, from the server list.">
            {servers.data?.map(s => <ListRow key={s.id} icon="server" title={s.name} subtitle={serverLine(s)} trailing={s.yours ? <Badge label="Owner" tone="accent" /> : <Button size="sm" label="Leave" variant="ghost" onPress={() => { setLeaveId(s.name); setSheet('leave'); }} />} />)}
          </Group>
        ) : null}
        {tab === 'invitations' ? (
          <>
            {waiting.length ? (
              <Group title="Waiting for you" footer="Accepting lets one of your profiles use that server. Its owner decides which libraries you see.">
                {waiting.map(i => (
                  <View key={i.id} style={{padding: 14, gap: 8}}>
                    <Text variant="bodyStrong">{i.from} invited you to {i.server}</Text>
                    <Text variant="caption" tone="tertiary">{i.libraries} libraries shared with you · Expires {i.expires} · As Justin</Text>
                    <View style={{flexDirection: 'row', gap: 8}}>
                      <Button size="sm" label="Accept" variant="primary" onPress={() => void run(async () => { await client.account.answerInvitation(i.id, true); toast({message: 'Invitation accepted. The server appears in your server list once it confirms your access.'}); })} />
                      <Button size="sm" label="Decline" variant="ghost" onPress={() => void run(async () => { await client.account.answerInvitation(i.id, false); toast({message: 'Invitation declined'}); })} />
                    </View>
                  </View>
                ))}
              </Group>
            ) : <EmptyState icon="mail" title="No invitations" body="When someone shares their server with your Portico Account, the invitation appears here." compact />}
            <Group title="Earlier">
              {(invites.data ?? []).filter(i => i.state !== 'waiting').map(i => <ListRow key={i.id} icon="mail" title={i.server} subtitle={`From ${i.from}`} trailing={<Badge label={i.state === 'accepted' ? 'Accepted' : i.state === 'declined' ? 'Declined or withdrawn' : 'Expired'} tone={i.state === 'accepted' ? 'good' : 'neutral'} />} />)}
            </Group>
            <View><Button label="Have an invitation link?" icon="link" variant="secondary" onPress={() => open('paste')} /></View>
          </>
        ) : null}
        {tab === 'notifications' ? (
          <>
            <Group title="Notify me about" footer="Security changes and new sign-ins are always shown.">
              <View style={{paddingHorizontal: 14}}><Toggle label="A server that has gone quiet" value onChange={() => {}} /></View>
              <View style={{paddingHorizontal: 14}}><Toggle label="Server storage running low" value onChange={() => {}} /></View>
              <View style={{paddingHorizontal: 14}}><Toggle label="Accepted invitations" value={false} onChange={() => {}} /></View>
            </Group>
            <Group title="Recent">
              <ListRow icon="shield" title="New sign-in: Chrome on Windows" subtitle="If this wasn’t you, sign it out and change your password. · 2 h ago" />
              <ListRow icon="server" title="Studio Annex has been quiet" subtitle="Portico hasn’t heard from this server for a while. · 3 days ago" />
              <ListRow icon="key" title="New recovery codes were made" subtitle="2 weeks ago" />
            </Group>
          </>
        ) : null}
        <Surface style={{gap: 10}}>
          <Text variant="subheading">Your data</Text>
          <Text variant="caption" tone="secondary">A copy of your Portico Account data, as a JSON file.</Text>
          <View style={{flexDirection: 'row', gap: 8, flexWrap: 'wrap'}}>
            <Button label="Download your data" icon="download" variant="secondary" loading={busy && sheet === 'export'} onPress={() => { setSheet('export'); void run(async () => { await client.account.exportData(); setSheet(undefined); toast({message: 'Your account data is ready. It’s in Notifications.'}); }); }} />
            <Button label="Delete account…" icon="trash" variant="danger" onPress={() => open('delete')} />
          </View>
        </Surface>
      </View>
      <Sheet visible={sheet === 'email'} onClose={() => setSheet(undefined)} title="Change email address" size="sm" footer={<Button label="Confirm" variant="primary" loading={busy} disabled={!value.includes('@')} onPress={() => void run(async () => { await client.account.changeEmail(value); setSheet(undefined); toast({message: `We sent a code to ${value}. Email address changed.`}); })} />}>
        <TextField label="New email address" keyboardType="email-address" autoCapitalize="none" value={value} onChangeText={setValue} />
        <TextField label="Password" secureTextEntry />
      </Sheet>
      <Sheet visible={sheet === 'paste'} onClose={() => setSheet(undefined)} title="Have an invitation link?" subtitle="Paste a link or code someone sent you to see who invited you and to which server." size="sm" footer={<Button label="Review" variant="primary" disabled={!value} onPress={() => { setSheet(undefined); toast({message: 'That isn’t a complete invitation. Paste the whole link, including everything after #.', tone: 'error'}); }} />}>
        <TextField label="Invitation link or code" value={value} onChangeText={setValue} autoCapitalize="none" />
      </Sheet>
      <ConfirmDialog visible={sheet === 'others'} onClose={() => setSheet(undefined)} title="Sign out all other devices?" body="Every other device is signed out of your Portico Account. This device stays signed in." confirmLabel="Sign out" destructive onConfirm={() => void run(async () => { await client.account.signOutEverywhere(); setSheet(undefined); toast({message: 'All other devices signed out'}); })} />
      <ConfirmDialog visible={sheet === 'leave'} onClose={() => setSheet(undefined)} title={`Leave ${leaveId}?`} body={`You lose access to ${leaveId}. Its owner can invite you again.`} confirmLabel="Leave" destructive onConfirm={() => void run(async () => { await client.account.leaveServer(leaveId!); setSheet(undefined); toast({message: `Left ${leaveId}`}); })} />
      <ConfirmDialog visible={sheet === 'delete'} onClose={() => setSheet(undefined)} title="Delete your Portico Account?" body={`This deletes your account and all its profiles for good. It can’t be undone. You still own Lighthouse and Studio Annex: they leave your account and their personal data is deleted. You lose access to 1 server you were invited to.`} confirmLabel="Delete account" destructive typeToConfirm={acct?.email} error={error} busy={busy} onConfirm={() => void run(async () => { await client.account.deleteAccount(acct!.email); setSheet(undefined); })} />
      {error && sheet !== 'delete' ? <Notice tone="error" body={error} style={{marginHorizontal: m.gutter}} /> : null}
    </Screen>
  );
}
