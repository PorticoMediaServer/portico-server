import React, {useCallback, useEffect, useRef, useState} from 'react';
import {Animated, Easing, ScrollView, TVFocusGuideView, View, findNodeHandle} from 'react-native';
import Svg, {Defs, LinearGradient, Rect, Stop} from 'react-native-svg';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {colors} from '../theme/tokens';
import {useMetrics} from '../theme/layout';
import {useClient, useQuery, useTopic} from '../data/DataProvider';
import {Focusable} from '../ui/Focusable';
import {lastContentFocus, requestContentFocus} from '../ui/focusMemory';
import {Icon, type IconName} from '../ui/Icon';
import {Text} from '../ui/Text';
import {Avatar} from '../ui/Controls';
import {Wordmark} from './Brand';
import {root} from '../nav/nav';
import {useSession} from '../app/session';
import {usePrefs} from '../app/prefs';
import {destinationKey, useDestination} from './useDestination';
import type {LibraryKind} from '../data/types';

export const libraryIcon = (k: LibraryKind): IconName => ({movie: 'movie', tv: 'tv', anime: 'anime', music: 'music', audiobook: 'audiobook', photo: 'photo', homevideo: 'homeVideo'} as const)[k];

type Item = {key: string; label: string; icon: IconName; onPress: () => void; badge?: string | number; live?: boolean; scanning?: string};

/**
 * The left rail. On TV it rests as an icon strip and expands over the page
 * when focus enters it; on iPad it is a persistent sidebar that can collapse
 * to icons. It lists every library, Live TV, and the account footer.
 */
export function Rail({variant, expanded: expandedProp, onToggle, onProfile, onFocusChange, railRef, focusRequest = 0}: {variant: 'tv' | 'tablet'; expanded?: boolean; onToggle?: () => void; onProfile: () => void; onFocusChange?: (f: boolean) => void; railRef?: React.RefObject<View | null>; focusRequest?: number}) {
  const m = useMetrics();
  const client = useClient();
  const {isAdmin, profile, server, session} = useSession();
  const insets = useSafeAreaInsets();
  const prefs = usePrefs();
  useTopic(['notifications', 'together']);
  const libs = useQuery(c => c.libraries.list(), [profile?.id], ['libraries', 'session']);
  const dest = useDestination();
  const active = destinationKey(dest.root);
  const [focused, setFocused] = useState(false);
  const [wantFocus, setWantFocus] = useState(false);
  useEffect(() => {
    if (!focusRequest) return;
    setWantFocus(true);
    const t = setTimeout(() => setWantFocus(false), 400);
    return () => clearTimeout(t);
  }, [focusRequest]);
  const [returnTo, setReturnTo] = useState<View | null>(null);
  const [scrollBox, setScrollBox] = useState({y: 0, h: 0, content: 0});
  const focusCount = useRef(0);
  const blurTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const expanded = variant === 'tv' ? focused : expandedProp ?? true;
  const width = useRef(new Animated.Value(expanded ? m.rail.expanded : m.rail.collapsed)).current;
  useEffect(() => {
    Animated.timing(width, {toValue: expanded ? m.rail.expanded : m.rail.collapsed, duration: prefs.reduceMotion ? 0 : 220, easing: Easing.out(Easing.cubic), useNativeDriver: false}).start();
  }, [expanded, width, m.rail, prefs.reduceMotion]);
  const onItemFocus = useCallback(() => {
    focusCount.current++;
    if (blurTimer.current) clearTimeout(blurTimer.current);
    if (!focused) { setReturnTo(lastContentFocus.current); setFocused(true); onFocusChange?.(true); }
  }, [focused, onFocusChange]);
  const onItemBlur = useCallback(() => {
    focusCount.current--;
    if (blurTimer.current) clearTimeout(blurTimer.current);
    blurTimer.current = setTimeout(() => { if (focusCount.current <= 0) { focusCount.current = 0; setFocused(false); onFocusChange?.(false); } }, 60);
  }, [onFocusChange]);

  const unread = prefs.badges ? client.notifications.unread() : 0;
  const inGroup = !!client.together.current();
  const top: Item[] = [
    {key: 'Home', label: 'Home', icon: 'home', onPress: () => root('Home')},
    {key: 'Search', label: 'Search', icon: 'search', onPress: () => root('Search')},
    {key: 'Saved', label: 'Saved', icon: 'saved', onPress: () => root('Saved')},
    ...(profile?.limits?.liveTv === false ? [] : [{key: 'live', label: 'Live TV', icon: 'live' as IconName, onPress: () => root('Live', {view: 'guide'})}]),
    ...(variant === 'tablet' && profile?.limits?.downloads !== false ? [{key: 'Downloads', label: 'Downloads', icon: 'download' as IconName, onPress: () => root('Downloads')}] : []),
    ...(profile?.limits?.together === false ? [] : [{key: 'Together', label: 'Watch Together', icon: 'together' as IconName, onPress: () => root('Together'), live: inGroup}]),
  ];
  const libraries: Item[] = (libs.data ?? []).map(l => ({key: `lib:${l.id}`, label: l.name, icon: libraryIcon(l.kind), onPress: () => root('Library', {libraryId: l.id}), scanning: l.scanning ? String(l.scanning.found) : undefined}));
  const foot: Item[] = [
    ...(isAdmin ? [{key: 'Admin', label: 'Manage server', icon: 'console' as IconName, onPress: () => root('Admin')}] : []),
    {key: 'notifications', label: 'Notifications', icon: 'bell', onPress: () => root('SettingsSection', {section: 'notifications'}), badge: unread || undefined},
    {key: 'Settings', label: 'Settings', icon: 'settings', onPress: () => root('Settings')},
  ];
  const tv = variant === 'tv';
  const edgeColor = tv ? (expanded ? colors.overlay : colors.ink) : colors.raisedSoft;
  const rowH = tv ? 64 : 42;
  const iconBox = tv ? m.rail.collapsed : m.rail.collapsed;
  const labelOpacity = width.interpolate({inputRange: [m.rail.collapsed, m.rail.expanded], outputRange: [0, 1]});

  const renderItem = (it: Item) => {
    const isActive = active === it.key || (it.key === 'live' && active.startsWith('live:'));
    return (
      <Focusable key={it.key} onPress={() => { if (variant === 'tv' && !isActive) requestContentFocus(); it.onPress(); }} onFocus={onItemFocus} onBlur={onItemBlur} rememberFocus={false} accessibilityRole="button" accessibilityLabel={it.label} accessibilityState={{selected: isActive}}
        ref={isActive ? (railRef as React.RefObject<View>) : undefined}
        hasTVPreferredFocus={isActive && wantFocus}
        style={({focused: f}) => ({height: rowH, marginHorizontal: tv ? 16 : 10, borderRadius: tv ? 16 : 10, flexDirection: 'row', alignItems: 'center', backgroundColor: f ? colors.focus : isActive ? 'rgba(64,159,212,0.16)' : 'transparent'})}>
        {({focused: f}) => (
          <>
            <View style={{width: iconBox - (tv ? 32 : 20), alignItems: 'center'}}>
              <Icon name={it.icon} size={tv ? 30 : 20} color={f ? colors.accentInk : isActive ? colors.accentStrong : colors.textSecondary} />
              {!expanded && (it.badge || it.live || it.scanning) ? <View style={{position: 'absolute', top: -4, right: tv ? 10 : 8, width: 10, height: 10, borderRadius: 5, backgroundColor: it.live ? colors.healthy : colors.accent}} /> : null}
            </View>
            <Animated.View style={{flex: 1, opacity: labelOpacity, flexDirection: 'row', alignItems: 'center', gap: 8, paddingRight: 12}}>
              <Text variant={tv ? 'bodyStrong' : 'body'} color={f ? colors.accentInk : isActive ? colors.text : colors.textSecondary} numberOfLines={1} style={{flex: 1}}>{it.label}</Text>
              {it.badge ? <View style={{minWidth: tv ? 30 : 20, height: tv ? 30 : 20, borderRadius: 15, paddingHorizontal: 6, backgroundColor: f ? colors.accentDeep : colors.accent, alignItems: 'center', justifyContent: 'center'}}><Text variant="micro" color={colors.accentInk}>{it.badge}</Text></View> : null}
              {it.live ? <Text variant="micro" color={f ? colors.accentInk : colors.healthy}>On</Text> : null}
              {it.scanning ? <Text variant="micro" color={f ? colors.accentInk : colors.accentStrong}>{it.scanning}</Text> : null}
            </Animated.View>
          </>
        )}
      </Focusable>
    );
  };
  const heading = (label: string) => (
    <Animated.View style={{height: tv ? 44 : 30, justifyContent: 'flex-end', paddingLeft: tv ? 32 : 22, opacity: labelOpacity, marginTop: tv ? 10 : 6}}>
      <Text variant="label" tone="tertiary" numberOfLines={1}>{label}</Text>
    </Animated.View>
  );
  const content = (
    <Animated.View style={{width, height: '100%', paddingTop: tv ? 44 : insets.top + 6, paddingBottom: tv ? 36 : Math.max(insets.bottom, 14)}}>
      <View style={{flexDirection: 'row', alignItems: 'center', height: tv ? 72 : 52, paddingLeft: Math.round(m.rail.collapsed / 2 - ((tv ? 40 : 26) * 304) / 405 / 2), marginBottom: tv ? 12 : 4}}>
        <Wordmark height={tv ? 40 : 26} reveal={labelOpacity} />
      </View>
      <View style={{flex: 1}}>
        <ScrollView style={{flex: 1}} showsVerticalScrollIndicator={false} contentContainerStyle={{gap: tv ? 4 : 2, paddingVertical: 4}} scrollEventThrottle={32}
          onLayout={e => { const h = e.nativeEvent.layout.height; setScrollBox(b => ({...b, h})); }}
          onContentSizeChange={(_, h) => setScrollBox(b => ({...b, content: h}))}
          onScroll={e => { const y = e.nativeEvent.contentOffset.y; setScrollBox(b => (Math.abs(b.y - y) > 2 ? {...b, y} : b)); }}>
          {top.map(renderItem)}
          {heading('Libraries')}
          {libraries.map(renderItem)}
        </ScrollView>
        {/* Soft edges where the list continues, so a clipped row reads as "more". */}
        {scrollBox.y > 4 ? <EdgeFade edge="top" color={edgeColor} /> : null}
        {scrollBox.content - scrollBox.y - scrollBox.h > 4 ? <EdgeFade edge="bottom" color={edgeColor} /> : null}
      </View>
      <View style={{gap: tv ? 4 : 2, paddingTop: 8, borderTopWidth: 1, borderColor: expanded ? colors.lineSoft : 'transparent', marginHorizontal: 0}}>
        {foot.map(renderItem)}
        <Focusable onPress={onProfile} onFocus={onItemFocus} onBlur={onItemBlur} rememberFocus={false} accessibilityLabel={`${profile?.name}, profile menu`}
          style={({focused: f}) => ({height: tv ? 76 : 52, marginHorizontal: tv ? 16 : 10, borderRadius: tv ? 16 : 10, flexDirection: 'row', alignItems: 'center', backgroundColor: f ? colors.focus : 'transparent'})}>
          {({focused: f}) => (
            <>
              <View style={{width: iconBox - (tv ? 32 : 20), alignItems: 'center'}}><Avatar name={profile?.name ?? '?'} art={profile?.art} size={tv ? 44 : 30} /></View>
              <Animated.View style={{flex: 1, opacity: labelOpacity}}>
                <Text variant="bodyStrong" color={f ? colors.accentInk : colors.text} numberOfLines={1}>{profile?.name}</Text>
                <Text variant="micro" color={f ? colors.accentInk : colors.textTertiary} numberOfLines={1}>{server?.name} · {session?.account.kind === 'local' ? 'Signed in directly' : 'Portico Account'}</Text>
              </Animated.View>
            </>
          )}
        </Focusable>
        {variant === 'tablet' && onToggle ? (
          <Focusable onPress={onToggle} accessibilityLabel={expanded ? 'Collapse navigation' : 'Expand navigation'} style={({focused: f}) => ({height: 40, marginHorizontal: 10, borderRadius: 10, flexDirection: 'row', alignItems: 'center', backgroundColor: f ? colors.focus : 'transparent'})}>
            <View style={{width: iconBox - 20, alignItems: 'center'}}><Icon name={expanded ? 'back' : 'forward'} size={18} color={colors.textTertiary} /></View>
            <Animated.View style={{opacity: labelOpacity}}><Text variant="caption" tone="tertiary">Collapse</Text></Animated.View>
          </Focusable>
        ) : null}
      </View>
    </Animated.View>
  );
  if (tv) {
    return (
      <View pointerEvents="box-none" style={{position: 'absolute', left: 0, top: 0, bottom: 0, flexDirection: 'row'}}>
        <Animated.View style={{height: '100%', backgroundColor: expanded ? colors.overlay : 'rgba(7,16,24,0.55)', borderRightWidth: 1, borderColor: expanded ? colors.line : 'rgba(40,64,79,0.5)'}}>
          <TVFocusGuideView autoFocus style={{flex: 1}}>{content}</TVFocusGuideView>
        </Animated.View>
        {expanded && returnTo ? <TVFocusGuideView destinations={[returnTo]} style={{width: 2, height: '100%'}} /> : null}
        {expanded ? (
          <Svg width={260} height="100%" pointerEvents="none">
            <Defs><LinearGradient id="rs" x1="0" y1="0" x2="1" y2="0"><Stop offset="0" stopColor={colors.ink} stopOpacity={0.85} /><Stop offset="1" stopColor={colors.ink} stopOpacity={0} /></LinearGradient></Defs>
            <Rect x="0" y="0" width="260" height="100%" fill="url(#rs)" />
          </Svg>
        ) : null}
      </View>
    );
  }
  return <View style={{height: '100%', backgroundColor: 'rgba(12,26,37,0.72)', borderRightWidth: 1, borderColor: colors.lineSoft}}>{content}</View>;
}

function EdgeFade({edge, color}: {edge: 'top' | 'bottom'; color: string}) {
  const id = `rail-fade-${edge}`;
  return (
    <Svg pointerEvents="none" width="100%" height={36} style={{position: 'absolute', left: 0, right: 0, [edge]: 0}}>
      <Defs><LinearGradient id={id} x1="0" y1={edge === 'top' ? '1' : '0'} x2="0" y2={edge === 'top' ? '0' : '1'}><Stop offset="0" stopColor={color} stopOpacity={0} /><Stop offset="1" stopColor={color} stopOpacity={0.85} /></LinearGradient></Defs>
      <Rect x="0" y="0" width="100%" height="36" fill={`url(#${id})`} />
    </Svg>
  );
}

export const nodeOf = findNodeHandle;
