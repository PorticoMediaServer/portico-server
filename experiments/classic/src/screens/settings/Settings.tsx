import React from 'react';
import {View} from 'react-native';
import {useRoute} from '@react-navigation/native';
import {useClient, useTopic} from '../../data/DataProvider';
import {Screen, PageHeader} from '../../ui/Page';
import {Button} from '../../ui/Button';
import {Avatar, Group, ListRow, Surface} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {useMetrics} from '../../theme/layout';
import {useSession} from '../../app/session';
import {push, root, back} from '../../nav/nav';
import {PreferencesSection} from './Preferences';
import {AboutSection, DevicesSection, LinkTvSection, NotificationsSection, ProfilesSection, SecuritySection, ServersSection, AccountSection} from './Sections';
import {ScreenIndex} from './ScreenIndex';

/** Settings hub, in the order shared with the web: account and server, then Preferences, Notifications, Downloads, Watch Together, Link a TV, About. */
export function SettingsScreen() {
  const m = useMetrics();
  const client = useClient();
  useTopic(['notifications']);
  const {profile, server, isAdmin, signOut} = useSession();
  const unread = client.notifications.unread();
  return (
    <Screen contentStyle={{gap: m.space.lg}}>
      <PageHeader title="Settings" />
      <View style={{paddingHorizontal: m.gutter, gap: m.space.lg, maxWidth: m.isTV ? 1200 : 820, width: '100%'}}>
        <Group>
          <ListRow left={<Avatar name={profile?.name ?? '?'} art={profile?.art} size={m.isTV ? 52 : 36} />} title={profile?.name ?? ''} subtitle="Portico Account · justin" onPress={() => push('SettingsSection', {section: 'account'})} hasTVPreferredFocus />
          <ListRow icon="server" title={server?.name ?? 'Server'} subtitle={`${server?.host} · Connected through your Portico Account`} onPress={() => push('SettingsSection', {section: 'servers'})} />
        </Group>
        <Group>
          <ListRow icon="sliders" title="Preferences" subtitle="Playback, subtitles and appearance" onPress={() => push('SettingsSection', {section: 'preferences'})} />
          <ListRow icon="bell" title="Notifications" subtitle="Messages from your server" badge={unread || undefined} onPress={() => push('SettingsSection', {section: 'notifications'})} />
          {!m.isTV && profile?.limits?.downloads !== false ? <ListRow icon="download" title="Downloads" subtitle="Saved on this device to watch offline" onPress={() => push('Downloads')} /> : null}
          {profile?.limits?.together !== false ? <ListRow icon="together" title="Watch Together" subtitle="Watch in step with others on this server" onPress={() => push('Together')} /> : null}
          {!m.isTV ? <ListRow icon="tvDevice" title="Link a TV" subtitle="Sign a TV in with the code it shows" onPress={() => push('SettingsSection', {section: 'linkTv'})} /> : null}
        </Group>
        <Group title="Account and security">
          <ListRow icon="user" title="Profiles" subtitle="Add, rename, PINs, colors and limits" onPress={() => push('SettingsSection', {section: 'profiles'})} />
          <ListRow icon="shield" title="Security" subtitle="Password and two-step verification" onPress={() => push('SettingsSection', {section: 'security'})} />
          <ListRow icon="phone" title="Devices" subtitle="Everywhere this account is signed in to this server" onPress={() => push('SettingsSection', {section: 'devices'})} />
          <ListRow icon="network" title="Portico Account" subtitle="Sign-in methods, servers, invitations and your data" onPress={() => push('Account')} />
        </Group>
        <Group>
          <ListRow icon="info" title="About Portico" onPress={() => push('SettingsSection', {section: 'about'})} />
          <ListRow icon="gridView" title="Screen index" subtitle="Every screen and state, for review" onPress={() => push('SettingsSection', {section: 'screens'})} />
        </Group>
        {isAdmin ? (
          <Surface tone="soft" style={{gap: 8}}>
            <Text variant="bodyStrong">You manage this server</Text>
            <Text variant="caption" tone="secondary">Libraries, sharing, remote access and everything else about the server live under Manage server.</Text>
            <View><Button label="Open server console" icon="console" variant="secondary" onPress={() => root('Admin')} /></View>
          </Surface>
        ) : null}
        <View><Button label="Sign out" icon="signOut" variant="danger" onPress={() => void signOut()} /></View>
      </View>
    </Screen>
  );
}

const TITLES: Record<string, string> = {preferences: 'Preferences', account: 'Account', servers: 'Servers', notifications: 'Notifications', about: 'About Portico', profiles: 'Profiles', security: 'Security', devices: 'Devices', linkTv: 'Link a TV', screens: 'Screen index'};

export function SettingsSectionScreen() {
  const {params} = useRoute() as {params: {section: string}};
  const m = useMetrics();
  const s = params.section;
  return (
    <Screen contentStyle={{gap: m.space.lg}}>
      <PageHeader eyebrow="Settings" title={TITLES[s] ?? 'Settings'} onBack={back} />
      <View style={{paddingHorizontal: m.gutter, gap: m.space.lg, maxWidth: m.isTV ? 1300 : 860, width: '100%'}}>
        {s === 'preferences' ? <PreferencesSection /> : null}
        {s === 'account' ? <AccountSection /> : null}
        {s === 'servers' ? <ServersSection /> : null}
        {s === 'notifications' ? <NotificationsSection /> : null}
        {s === 'about' ? <AboutSection /> : null}
        {s === 'profiles' ? <ProfilesSection /> : null}
        {s === 'security' ? <SecuritySection /> : null}
        {s === 'devices' ? <DevicesSection /> : null}
        {s === 'linkTv' ? <LinkTvSection /> : null}
        {s === 'screens' ? <ScreenIndex /> : null}
      </View>
    </Screen>
  );
}
