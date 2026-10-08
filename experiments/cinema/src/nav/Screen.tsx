import React from 'react';
import {ScrollView, StyleSheet, TVFocusGuideView, View, type ScrollViewProps, type StyleProp, type ViewStyle} from 'react-native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {useQuery, useServices} from '../data/hooks';
import {IconButton} from '../design/Button';
import {Focusable} from '../design/Focusable';
import {Icon} from '../design/Icon';
import {Text} from '../design/Text';
import {colors, fonts, isTV, u} from '../design/tokens';
import {useLayout} from '../design/useLayout';
import {usePlayer} from '../state/player';
import {useSession} from '../state/session';
import {openProfileMenu} from './menus';
import {back, go, navigationRef} from './routes';
import {TAB_BAR_HEIGHT} from './TabBar';

export const MINI_PLAYER_HEIGHT = 64;

/** Space the fixed chrome takes: rail on the left (TV/iPad), tabs and mini player at the bottom (phone). */
export function useChromeInsets(chromeless = false) {
  const layout = useLayout();
  const insets = useSafeAreaInsets();
  const mini = usePlayer(p => p.active && (p.kind !== 'video' || p.minimized) && p.kind !== 'live');
  const left = chromeless ? 0 : layout.railWidth;
  const bottom = chromeless ? insets.bottom : layout.compact ? TAB_BAR_HEIGHT + insets.bottom + (mini ? MINI_PLAYER_HEIGHT : 0) : insets.bottom + (mini && !isTV ? 80 : 0);
  return {left, bottom, top: insets.top};
}

/** Phone/tablet header: back or server switcher on the left, title, actions and the profile on the right. */
export function Header({title, right, transparent, showBack}: {title?: string; right?: React.ReactNode; transparent?: boolean; showBack?: boolean}) {
  const session = useSession();
  const s = useServices();
  const notes = useQuery('notifications', () => s.account.notifications());
  const unread = (notes.data ?? []).filter(n => !n.read && !n.archived).length;
  const insets = useSafeAreaInsets();
  const layout = useLayout();
  if (isTV) return null;
  const canBack = showBack ?? (navigationRef.isReady() && navigationRef.canGoBack() && (navigationRef.getRootState()?.routes.length ?? 0) > 1);
  return (
    <View style={[styles.header, {paddingTop: insets.top + 6, paddingLeft: layout.gutter - 4, paddingRight: layout.gutter - 4}, !transparent && styles.headerSolid]}>
      {canBack ? (
        <IconButton icon="chevronLeft" label="Back" onPress={back} size={36} variant={transparent ? 'glass' : 'ghost'} />
      ) : (
        <Focusable onPress={() => go('Chooser')} accessibilityRole="button" accessibilityLabel={`Server ${session.server?.name ?? ''}, switch server`} style={styles.serverPill}>
          <View style={[styles.serverDot, session.connection !== 'online' && {backgroundColor: colors.amber}]} />
          <Text variant="captionStrong" numberOfLines={1} style={{maxWidth: 170}}>{session.server?.name ?? 'Portico'}</Text>
          <Icon name="chevronDown" size={13} color={colors.textTertiary} />
        </Focusable>
      )}
      <View style={{flex: 1, paddingHorizontal: 8}}>
        {title && canBack ? <Text variant="subheading" numberOfLines={1}>{title}</Text> : null}
      </View>
      {right}
      <Focusable onPress={() => openProfileMenu(session, unread)} accessibilityRole="button" accessibilityLabel={`${session.profile?.name ?? 'Profile'} menu${unread ? `, ${unread} unread` : ''}`} style={{marginLeft: 8}}>
        <View style={[styles.avatar, {backgroundColor: session.profile?.color ?? colors.accent}]}>
          <Text style={{fontFamily: fonts.bold, fontSize: 14, color: colors.actionInk}}>{session.profile?.initials ?? 'P'}</Text>
          {unread ? <View style={styles.unread} /> : null}
        </View>
      </Focusable>
    </View>
  );
}

type ScreenProps = {
  children?: React.ReactNode;
  title?: string;
  /** Large page title drawn in the content (phone and TV). */
  heading?: string;
  eyebrow?: string;
  headerRight?: React.ReactNode;
  /** Full-bleed top content (heroes): no top padding, transparent header over it. */
  bleed?: boolean;
  scroll?: boolean;
  chromeless?: boolean;
  padded?: boolean;
  contentStyle?: StyleProp<ViewStyle>;
  scrollProps?: ScrollViewProps;
  noHeader?: boolean;
};

export function Screen({children, title, heading, eyebrow, headerRight, bleed, scroll = true, chromeless, padded = true, contentStyle, scrollProps, noHeader}: ScreenProps) {
  const guide = React.useRef<View>(null);

  const layout = useLayout();
  const chrome = useChromeInsets(chromeless);
  const insets = useSafeAreaInsets();
  const headerH = isTV || noHeader ? 0 : insets.top + 52;
  const topPad = bleed ? 0 : isTV ? 56 : headerH + 4;
  const body = (
    <>
      {heading ? (
        <View style={{paddingHorizontal: padded ? 0 : layout.gutter, marginBottom: u(14)}}>
          {eyebrow ? <Text variant="eyebrow" tone="accent">{eyebrow}</Text> : null}
          <Text variant="display">{heading}</Text>
        </View>
      ) : null}
      {children}
    </>
  );
  const inner = scroll ? (
    <ScrollView
      style={{flex: 1}}
      contentContainerStyle={[{paddingTop: topPad, paddingBottom: chrome.bottom + u(40), paddingHorizontal: padded ? layout.gutter : 0}, contentStyle]}
      showsVerticalScrollIndicator={!isTV}
      keyboardShouldPersistTaps="handled"
      {...scrollProps}>
      {body}
    </ScrollView>
  ) : (
    <View style={[{flex: 1, paddingTop: topPad, paddingBottom: chrome.bottom, paddingHorizontal: padded ? layout.gutter : 0}, contentStyle]}>{body}</View>
  );
  return (
    <View style={[styles.screen, {paddingLeft: chrome.left}]}>
      {isTV ? (
        <TVFocusGuideView ref={guide as never} autoFocus style={{flex: 1}}>
          {inner}
        </TVFocusGuideView>
      ) : (
        inner
      )}
      {!noHeader && !chromeless ? <View style={[StyleSheet.absoluteFill, {left: chrome.left, bottom: undefined}]} pointerEvents="box-none"><Header title={title ?? heading} right={headerRight} transparent={bleed} /></View> : null}
      {!noHeader && chromeless && !isTV ? <View style={[StyleSheet.absoluteFill, {bottom: undefined}]} pointerEvents="box-none"><ChromelessBack /></View> : null}
    </View>
  );
}

function ChromelessBack() {
  const insets = useSafeAreaInsets();
  if (!(navigationRef.isReady() && navigationRef.canGoBack())) return null;
  return (
    <View style={{paddingTop: insets.top + 6, paddingLeft: 12}}>
      <IconButton icon="chevronLeft" label="Back" onPress={back} size={36} variant="glass" />
    </View>
  );
}

const styles = StyleSheet.create({
  screen: {flex: 1, backgroundColor: 'transparent'},
  header: {flexDirection: 'row', alignItems: 'center', paddingBottom: 8},
  headerSolid: {backgroundColor: 'rgba(4,10,16,0.82)', borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: 'rgba(40,64,79,0.6)'},
  serverPill: {flexDirection: 'row', alignItems: 'center', gap: 6, height: 34, paddingHorizontal: 12, borderRadius: 17, backgroundColor: 'rgba(237,245,250,0.1)'},
  serverDot: {width: 7, height: 7, borderRadius: 4, backgroundColor: colors.healthy},
  avatar: {width: 34, height: 34, borderRadius: 17, alignItems: 'center', justifyContent: 'center'},
  unread: {position: 'absolute', top: -1, right: -1, width: 10, height: 10, borderRadius: 5, backgroundColor: colors.record, borderWidth: 2, borderColor: colors.ink},
});
