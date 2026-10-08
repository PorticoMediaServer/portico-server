import React, {useState} from 'react';
import {View} from 'react-native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import Svg, {Defs, LinearGradient, Rect, Stop} from 'react-native-svg';
import {colors} from '../theme/tokens';
import {useClient, useQuery, useTopic} from '../data/DataProvider';
import {useSession} from '../app/session';
import {usePrefs} from '../app/prefs';
import {Focusable} from '../ui/Focusable';
import {Icon, type IconName} from '../ui/Icon';
import {Text} from '../ui/Text';
import {Avatar, ListRow} from '../ui/Controls';
import {Sheet} from '../ui/Sheet';
import {openFromChrome, root} from '../nav/nav';
import {BrandMark} from './Brand';
import {libraryIcon} from './Rail';
import {destinationKey, useDestination} from './useDestination';
import {PHONE_HEADER, PHONE_TABS} from '../ui/Page';
import {ServerSwitcher, ProfileSwitcher} from './ProfileMenu';

/** iPhone header: server switcher on the left; downloads, notifications and the profile on the right. */
export function PhoneHeader({onProfile}: {onProfile: () => void}) {
  const insets = useSafeAreaInsets();
  const client = useClient();
  const {server, profile} = useSession();
  const prefs = usePrefs();
  useTopic(['notifications']);
  const unread = prefs.badges ? client.notifications.unread() : 0;
  const [switching, setSwitching] = useState<'server' | 'profile'>();
  return (
    <View pointerEvents="box-none" style={{position: 'absolute', left: 0, right: 0, top: 0, height: insets.top + PHONE_HEADER}}>
      <Svg width="100%" height={insets.top + PHONE_HEADER + 24} style={{position: 'absolute'}} pointerEvents="none">
        <Defs><LinearGradient id="hd" x1="0" y1="0" x2="0" y2="1"><Stop offset="0" stopColor={colors.ink} stopOpacity={0.96} /><Stop offset="0.7" stopColor={colors.ink} stopOpacity={0.7} /><Stop offset="1" stopColor={colors.ink} stopOpacity={0} /></LinearGradient></Defs>
        <Rect x="0" y="0" width="100%" height="100%" fill="url(#hd)" />
      </Svg>
      <View style={{marginTop: insets.top, height: PHONE_HEADER, flexDirection: 'row', alignItems: 'center', paddingHorizontal: 12, gap: 6}}>
        <Focusable onPress={() => setSwitching('server')} accessibilityLabel={`${server?.name}, switch server`} style={{flexDirection: 'row', alignItems: 'center', gap: 8, paddingVertical: 6, paddingRight: 8}}>
          <BrandMark size={24} />
          <Text variant="subheading">{server?.name}</Text>
          <Icon name="chevronDown" size={16} color={colors.textTertiary} />
        </Focusable>
        <View style={{flex: 1}} />
        {profile?.limits?.downloads === false ? null : <HeaderIcon icon="download" label="Downloads" onPress={() => openFromChrome('Downloads', undefined, true)} />}
        <HeaderIcon icon="bell" label={unread ? `Notifications, ${unread} unread` : 'Notifications'} onPress={() => openFromChrome('SettingsSection', {section: 'notifications'}, true)} dot={unread > 0} />
        <Focusable onPress={onProfile} accessibilityLabel={`${profile?.name}, profile and settings`} style={{padding: 4}}>
          <Avatar name={profile?.name ?? '?'} art={profile?.art} size={30} />
        </Focusable>
      </View>
      <ServerSwitcher visible={switching === 'server'} onClose={() => setSwitching(undefined)} onChosen={() => setTimeout(() => setSwitching('profile'), 250)} />
      <ProfileSwitcher visible={switching === 'profile'} onClose={() => setSwitching(undefined)} />
    </View>
  );
}

function HeaderIcon({icon, label, onPress, dot}: {icon: IconName; label: string; onPress: () => void; dot?: boolean}) {
  return (
    <Focusable onPress={onPress} accessibilityLabel={label} style={{width: 40, height: 40, alignItems: 'center', justifyContent: 'center'}}>
      <Icon name={icon} size={22} color={colors.text} />
      {dot ? <View style={{position: 'absolute', top: 8, right: 8, width: 9, height: 9, borderRadius: 5, backgroundColor: colors.accent, borderWidth: 1.5, borderColor: colors.ink}} /> : null}
    </Focusable>
  );
}

/** iPhone tab bar: Home, Libraries, Live TV, Saved, Search. */
export function TabBar() {
  const insets = useSafeAreaInsets();
  const {profile} = useSession();
  const dest = useDestination();
  const key = destinationKey(dest.root);
  const [picker, setPicker] = useState(false);
  const libs = useQuery(c => c.libraries.list(), [profile?.id], ['libraries', 'session']);
  const tabs: {key: string; label: string; icon: IconName; onPress: () => void; active: boolean}[] = [
    {key: 'Home', label: 'Home', icon: 'home', onPress: () => root('Home'), active: key === 'Home'},
    {key: 'lib', label: 'Libraries', icon: 'gridView', onPress: () => setPicker(true), active: key.startsWith('lib:')},
    ...(profile?.limits?.liveTv === false ? [] : [{key: 'live', label: 'Live TV', icon: 'live' as IconName, onPress: () => root('Live', {view: 'guide'}), active: key.startsWith('live:')}]),
    {key: 'Saved', label: 'Saved', icon: 'saved', onPress: () => root('Saved'), active: key === 'Saved'},
    {key: 'Search', label: 'Search', icon: 'search', onPress: () => root('Search'), active: key === 'Search'},
  ];
  return (
    <View style={{position: 'absolute', left: 0, right: 0, bottom: 0, paddingBottom: insets.bottom, height: insets.bottom + PHONE_TABS, backgroundColor: colors.raisedSoft, borderTopWidth: 1, borderColor: colors.lineSoft, flexDirection: 'row'}}>
      {tabs.map(t => (
        <Focusable key={t.key} onPress={t.onPress} accessibilityRole="tab" accessibilityState={{selected: t.active}} accessibilityLabel={t.label} style={{flex: 1, alignItems: 'center', justifyContent: 'center', gap: 3, paddingTop: 6}}>
          <Icon name={t.icon} size={23} color={t.active ? colors.accentStrong : colors.textTertiary} />
          <Text variant="micro" color={t.active ? colors.accentStrong : colors.textTertiary}>{t.label}</Text>
        </Focusable>
      ))}
      <Sheet visible={picker} onClose={() => setPicker(false)} title="Libraries">
        {(libs.data ?? []).map(l => (
          <ListRow key={l.id} icon={libraryIcon(l.kind)} title={l.name} subtitle={`${l.count} ${l.kind === 'music' ? 'artists' : l.kind === 'photo' ? 'albums' : 'titles'}`} selected={key === `lib:${l.id}`} onPress={() => { setPicker(false); root('Library', {libraryId: l.id}); }} />
        ))}
      </Sheet>
    </View>
  );
}
