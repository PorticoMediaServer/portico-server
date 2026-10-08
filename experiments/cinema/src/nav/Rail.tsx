import React, {useEffect, useRef, useState} from 'react';
import {Animated, Easing, ScrollView, StyleSheet, TVFocusGuideView, View} from 'react-native';
import Svg, {Defs, LinearGradient, Rect, Stop} from 'react-native-svg';
import {useQuery, useServices} from '../data/hooks';
import {BrandMark} from '../design/Brand';
import {Focusable} from '../design/Focusable';
import {Icon, type IconName} from '../design/Icon';
import {Text} from '../design/Text';
import {colors, fonts, isTV} from '../design/tokens';
import {useSession} from '../state/session';
import {usePlayer} from '../state/player';
import {back} from './back';
import {currentRoute, useCurrentRoute} from './current';
import {libraryIcon, openProfileMenu} from './menus';
import {goSection, navigationRef, type RouteName} from './routes';

// The active rail item, so content screens can point a focus guide at it (focus can't always find
// views outside the navigator's screen on its own).
let railTarget: View | null = null;
const targetListeners = new Set<() => void>();
function setRailTarget(v: View | null) {
  railTarget = v;
  targetListeners.forEach(l => l());
}
export function useRailTarget() {
  return React.useSyncExternalStore(l => { targetListeners.add(l); return () => targetListeners.delete(l); }, () => railTarget);
}

type Dest = {key: string; label: string; icon: IconName; route: RouteName; params?: Record<string, unknown>; badge?: string};

export function useDestinations(): Dest[] {
  const s = useServices();
  const session = useSession();
  const libs = useQuery('libraries', () => s.catalog.libraries());
  const groups = useQuery('together:groups', () => s.together.groups());
  const inGroup = (groups.data ?? []).some(g => g.state !== 'ended');
  const manager = session.role === 'owner' || session.role === 'admin';
  return [
    {key: 'search', label: 'Search', icon: 'search', route: 'Search'},
    {key: 'home', label: 'Home', icon: 'home', route: 'Home'},
    ...(libs.data ?? []).map(l => ({key: 'lib:' + l.id, label: l.name, icon: libraryIcon[l.kind], route: 'Library' as RouteName, params: {libraryId: l.id}, badge: l.scanning ? 'scanning' : undefined})),
    {key: 'live', label: 'Live TV', icon: 'live', route: 'Live'},
    {key: 'saved', label: 'Saved', icon: 'bookmark', route: 'Saved'},
    {key: 'together', label: 'Watch Together', icon: 'together', route: 'Together', badge: inGroup ? 'group' : undefined},
    ...(!isTV ? [{key: 'downloads', label: 'Downloads', icon: 'download' as IconName, route: 'Downloads' as RouteName}] : []),
    {key: 'settings', label: 'Settings', icon: 'settings', route: 'Settings'},
    ...(manager ? [{key: 'admin', label: 'Manage server', icon: 'server' as IconName, route: 'Admin' as RouteName, params: {section: 'overview'}}] : []),
  ];
}

export function activeKey(name: RouteName, params?: Record<string, unknown>): string {
  if (name === 'Library' || name === 'Entity') return 'lib:' + String(params?.libraryId ?? '');
  const map: Partial<Record<RouteName, string>> = {Home: 'home', Search: 'search', Live: 'live', Saved: 'saved', Resource: 'saved', Together: 'together', TogetherGroup: 'together', Downloads: 'downloads', Settings: 'settings', SettingsPreferences: 'settings', SettingsProfiles: 'settings', SettingsServers: 'settings', SettingsAbout: 'settings', ScreenIndex: 'settings', Admin: 'admin'};
  return map[name] ?? '';
}

/**
 * The Aisle: Cinema's left rail. On TV it sits as a slim column of icons; when focus enters, it
 * widens into a labelled menu over a dark gradient, and focus always lands on the section you are
 * in. On iPad it is a tappable sidebar that expands the same way.
 */
export function Rail() {
  const dests = useDestinations();
  const session = useSession();
  const s = useServices();
  const route = useCurrentRoute();
  const notes = useQuery('notifications', () => s.account.notifications());
  const unread = (notes.data ?? []).filter(n => !n.read && !n.archived).length;
  const nowPlaying = usePlayer(p => p.active && (p.kind === 'audio' || p.kind === 'book'));
  const [expanded, setExpanded] = useState(false);
  const focusCount = useRef(0);
  const collapseTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const width = useRef(new Animated.Value(0)).current;
  const refs = useRef(new Map<string, View | null>()).current;
  const active = activeKey(route.name, route.params);
  const [destTarget, setDestTarget] = useState<View | null>(null);

  useEffect(() => {
    Animated.timing(width, {toValue: expanded ? 1 : 0, duration: 240, easing: Easing.out(Easing.cubic), useNativeDriver: false}).start();
  }, [expanded, width]);

  useEffect(() => {
    const t = refs.get(active) ?? refs.get('home') ?? null;
    setDestTarget(t);
    setRailTarget(t);
  }, [active, refs, dests.length]);

  // Menu from content on a top-level screen moves focus to the rail; Menu in the rail leaves the app.
  useEffect(
    () =>
      back.register({
        priority: 5,
        active: () => isTV && !expandedRef.current && !(navigationRef.isReady() && navigationRef.canGoBack()) && !['Landing', 'Profiles', 'SignIn'].includes(currentRoute().name),
        handle: () => {
          const target = refs.get(activeKey(currentRoute().name, currentRoute().params)) ?? refs.get('home');
          (target as unknown as {requestTVFocus?: () => void})?.requestTVFocus?.();
        },
      }),
    [refs],
  );
  const expandedRef = useRef(false);
  expandedRef.current = expanded;
  useEffect(() => back.sync(), [expanded]);

  const onItemFocus = () => {
    focusCount.current++;
    if (collapseTimer.current) clearTimeout(collapseTimer.current);
    setExpanded(true);
  };
  const onItemBlur = () => {
    focusCount.current = Math.max(0, focusCount.current - 1);
    if (collapseTimer.current) clearTimeout(collapseTimer.current);
    collapseTimer.current = setTimeout(() => focusCount.current === 0 && setExpanded(false), 80);
  };

  const collapsedW = isTV ? 112 : 76;
  const expandedW = isTV ? 420 : 260;
  const w = width.interpolate({inputRange: [0, 1], outputRange: [collapsedW, expandedW]});
  const labelOpacity = width.interpolate({inputRange: [0, 0.6, 1], outputRange: [0, 0, 1]});
  const item = (d: Dest | {key: string; label: string; icon: IconName; onPress: () => void; badge?: string; avatar?: boolean}) => {
    const on = d.key === active;
    const press = 'onPress' in d ? d.onPress : () => {
      goSection(d.route as never, d.params as never);
      if (!isTV) setExpanded(false);
    };
    return (
      <Focusable
        key={d.key}
        ref={v => {
          refs.set(d.key, v);
        }}
        onPress={press}
        onFocus={onItemFocus}
        onBlur={onItemBlur}
        scale={1}
        accessibilityRole="button"
        accessibilityLabel={d.label + (d.badge === 'scanning' ? ', scanning' : d.badge === 'group' ? ', you’re in a group' : '')}
        accessibilityState={{selected: on}}>
        {({focused}) => {
          const lit = focused && isTV;
          return (
            <View style={[styles.item, lit && styles.itemLit, on && !lit && styles.itemOn]}>
              <View style={styles.iconBox}>
                {'avatar' in d && d.avatar ? (
                  <View style={[styles.avatar, {backgroundColor: session.profile?.color ?? colors.accent}]}>
                    <Text style={{fontFamily: fonts.bold, fontSize: isTV ? 24 : 15, color: colors.actionInk}}>{session.profile?.initials ?? 'P'}</Text>
                  </View>
                ) : (
                  <Icon name={d.icon} size={isTV ? 19 : 20} color={lit ? colors.actionInk : on ? colors.accentStrong : colors.textSecondary} />
                )}
                {d.badge ? <View style={[styles.dot, d.badge === 'scanning' && {backgroundColor: colors.amber}]} /> : null}
              </View>
              <Animated.View style={{opacity: labelOpacity, flex: 1}}>
                <Text variant="bodyStrong" numberOfLines={1} color={lit ? colors.actionInk : on ? colors.text : colors.textSecondary}>{d.label}</Text>
              </Animated.View>
              {on && !lit ? <View style={styles.activeBar} /> : null}
            </View>
          );
        }}
      </Focusable>
    );
  };

  return (
    <Animated.View style={[styles.rail, {width: w}]} pointerEvents="box-none">
      <Animated.View pointerEvents="none" style={{position: 'absolute', left: 0, top: 0, bottom: 0, width: expandedW + (isTV ? 520 : 200), opacity: width}}>
        <Svg width="100%" height="100%">
          <Defs>
            <LinearGradient id="railfade" x1="0" y1="0" x2="1" y2="0">
              <Stop offset="0" stopColor={colors.inkDeep} stopOpacity={0.97} />
              <Stop offset="0.45" stopColor={colors.ink} stopOpacity={0.9} />
              <Stop offset="1" stopColor={colors.ink} stopOpacity={0} />
            </LinearGradient>
          </Defs>
          <Rect x="0" y="0" width="100%" height="100%" fill="url(#railfade)" />
        </Svg>
      </Animated.View>
      <TVFocusGuideView style={styles.inner} autoFocus destinations={destTarget ? [destTarget] : undefined}>
        <View style={styles.brand}>
          <BrandMark size={isTV ? 44 : 30} />
          <Animated.View style={{opacity: labelOpacity, marginLeft: isTV ? 20 : 12}}>
            <Text variant="eyebrow" tone="tertiary">Portico</Text>
            <Text variant="captionStrong" numberOfLines={1}>{session.server?.name ?? 'Home'}</Text>
          </Animated.View>
        </View>
        <ScrollView style={{flex: 1}} contentContainerStyle={{paddingVertical: isTV ? 8 : 4}} showsVerticalScrollIndicator={false}>
          {dests.map(item)}
        </ScrollView>
        <View style={{paddingBottom: isTV ? 20 : 12}}>
          {!isTV && nowPlaying ? null : null}
          {item({key: 'profile', label: session.profile?.name ?? 'Profile', icon: 'user', avatar: true, badge: unread ? 'unread' : undefined, onPress: () => openProfileMenu(session, unread)})}
        </View>
      </TVFocusGuideView>
      {!isTV ? (
        <Focusable onPress={() => setExpanded(e => !e)} accessibilityLabel={expanded ? 'Collapse navigation' : 'Expand navigation'} style={styles.toggle}>
          <Icon name={expanded ? 'chevronLeft' : 'chevronRight'} size={14} color={colors.textTertiary} />
        </Focusable>
      ) : null}
    </Animated.View>
  );
}

const styles = StyleSheet.create({
  rail: {position: 'absolute', left: 0, top: 0, bottom: 0, zIndex: 20, overflow: 'visible'},
  inner: {flex: 1, paddingTop: isTV ? 56 : 58, paddingHorizontal: isTV ? 18 : 10},
  brand: {flexDirection: 'row', alignItems: 'center', height: isTV ? 90 : 56, paddingLeft: isTV ? 20 : 14, marginBottom: isTV ? 16 : 6},
  item: {flexDirection: 'row', alignItems: 'center', height: isTV ? 66 : 46, borderRadius: isTV ? 33 : 23, paddingRight: 16, marginVertical: isTV ? 3 : 2},
  itemLit: {backgroundColor: colors.white, shadowColor: colors.accentStrong, shadowOpacity: 0.6, shadowRadius: 20},
  itemOn: {backgroundColor: 'rgba(92,179,226,0.12)'},
  iconBox: {width: isTV ? 76 : 56, alignItems: 'center', justifyContent: 'center'},
  avatar: {width: isTV ? 46 : 30, height: isTV ? 46 : 30, borderRadius: 99, alignItems: 'center', justifyContent: 'center'},
  dot: {position: 'absolute', top: isTV ? -4 : -2, right: isTV ? 14 : 12, width: isTV ? 12 : 8, height: isTV ? 12 : 8, borderRadius: 6, backgroundColor: colors.accentStrong, borderWidth: 2, borderColor: colors.ink},
  activeBar: {position: 'absolute', left: 0, top: isTV ? 18 : 12, bottom: isTV ? 18 : 12, width: 3, borderRadius: 2, backgroundColor: colors.accentStrong},
  toggle: {position: 'absolute', right: -10, top: 70, width: 22, height: 44, borderRadius: 11, backgroundColor: colors.raised, alignItems: 'center', justifyContent: 'center', borderWidth: StyleSheet.hairlineWidth, borderColor: colors.line},
});
