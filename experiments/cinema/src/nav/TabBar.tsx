import React from 'react';
import {Pressable, StyleSheet, View} from 'react-native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import Svg, {Defs, LinearGradient, Rect, Stop} from 'react-native-svg';
import {Icon, type IconName} from '../design/Icon';
import {Text} from '../design/Text';
import {colors} from '../design/tokens';
import {useCurrentRoute} from './current';
import {goSection, type RouteName} from './routes';

const tabs: {key: string; label: string; icon: IconName; route: RouteName; match: RouteName[]}[] = [
  {key: 'home', label: 'Home', icon: 'home', route: 'Home', match: ['Home', 'HomeRow', 'HomeCustomise']},
  {key: 'libraries', label: 'Library', icon: 'library', route: 'Libraries', match: ['Libraries', 'Library', 'Entity']},
  {key: 'live', label: 'Live', icon: 'live', route: 'Live', match: ['Live']},
  {key: 'saved', label: 'Saved', icon: 'bookmark', route: 'Saved', match: ['Saved', 'Resource', 'Downloads']},
  {key: 'search', label: 'Search', icon: 'search', route: 'Search', match: ['Search']},
];

export const TAB_BAR_HEIGHT = 58;

/** iPhone navigation: five fixed destinations at the bottom, the mini player docks above. */
export function TabBar() {
  const insets = useSafeAreaInsets();
  const route = useCurrentRoute();
  return (
    <View style={[styles.bar, {height: TAB_BAR_HEIGHT + insets.bottom, paddingBottom: insets.bottom}]}>
      <Svg style={StyleSheet.absoluteFill} width="100%" height="100%">
        <Defs>
          <LinearGradient id="tabbg" x1="0" y1="0" x2="0" y2="1">
            <Stop offset="0" stopColor={colors.inkDeep} stopOpacity={0.9} />
            <Stop offset="1" stopColor={colors.inkDeep} stopOpacity={1} />
          </LinearGradient>
        </Defs>
        <Rect x="0" y="0" width="100%" height="100%" fill="url(#tabbg)" />
      </Svg>
      <View style={styles.hairline} />
      {tabs.map(t => {
        const on = t.match.includes(route.name);
        return (
          <Pressable key={t.key} style={styles.tab} onPress={() => (goSection as (n: RouteName) => void)(t.route)} accessibilityRole="tab" accessibilityState={{selected: on}} accessibilityLabel={t.label}>
            <Icon name={t.icon} size={22} color={on ? colors.accentStrong : colors.textTertiary} strokeWidth={on ? 2.4 : 2} raw />
            <Text variant="micro" color={on ? colors.accentStrong : colors.textTertiary} style={{marginTop: 3}}>{t.label}</Text>
          </Pressable>
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  bar: {position: 'absolute', left: 0, right: 0, bottom: 0, flexDirection: 'row', zIndex: 20},
  hairline: {position: 'absolute', top: 0, left: 0, right: 0, height: StyleSheet.hairlineWidth, backgroundColor: colors.line},
  tab: {flex: 1, alignItems: 'center', justifyContent: 'center', paddingTop: 6},
});
