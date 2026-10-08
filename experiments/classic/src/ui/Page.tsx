import React, {useEffect} from 'react';
import {RefreshControl, ScrollView, TVFocusGuideView, View, type ScrollViewProps, type StyleProp, type ViewStyle} from 'react-native';
import {useNavigation} from '@react-navigation/native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {useMetrics} from '../theme/layout';
import {registerContentNav} from '../nav/nav';
import {Text} from './Text';
import {Button, IconButton} from './Button';
import {Icon} from './Icon';
import {colors} from '../theme/tokens';

/** Space the phone shell reserves at the top (header) and bottom (tabs and mini player). */
export const PHONE_HEADER = 52;
export const PHONE_TABS = 58;

/** Every content screen registers its stack with the global navigator helpers. */
export function useRegisterNav() {
  const navigation = useNavigation();
  useEffect(() => {
    registerContentNav(navigation as never);
    const off = navigation.addListener('focus', () => registerContentNav(navigation as never));
    return off;
  }, [navigation]);
}

export function useChromeInsets() {
  const m = useMetrics();
  const insets = useSafeAreaInsets();
  if (m.isTV) return {top: 48, bottom: 64};
  if (m.isPhone) return {top: insets.top + PHONE_HEADER, bottom: insets.bottom + PHONE_TABS + 16};
  return {top: Math.max(insets.top, 20) + 20, bottom: insets.bottom + 32};
}

/** A scrolling content screen inside the shell. */
export function Screen({children, scroll = true, style, contentStyle, refreshing, onRefresh, bleedTop, ...rest}: {children: React.ReactNode; scroll?: boolean; style?: StyleProp<ViewStyle>; contentStyle?: StyleProp<ViewStyle>; refreshing?: boolean; onRefresh?: () => void; bleedTop?: boolean} & ScrollViewProps) {
  useRegisterNav();
  const m = useMetrics();
  const chrome = useChromeInsets();
  const pad = {paddingTop: bleedTop ? 0 : chrome.top, paddingBottom: chrome.bottom};
  const inner = scroll ? (
    <ScrollView
      style={[{flex: 1}, style]}
      contentContainerStyle={[pad, contentStyle]}
      showsVerticalScrollIndicator={!m.isTV}
      refreshControl={onRefresh && !m.isTV ? <RefreshControl refreshing={!!refreshing} onRefresh={onRefresh} tintColor={colors.accentStrong} progressViewOffset={chrome.top} /> : undefined}
      {...rest}
    >
      {children}
    </ScrollView>
  ) : <View style={[{flex: 1}, pad, style]}>{children}</View>;
  return m.isTV ? <TVFocusGuideView autoFocus style={{flex: 1}}>{inner}</TVFocusGuideView> : inner;
}

/** Page title block with optional back button and trailing actions. */
export function PageHeader({title, eyebrow, subtitle, onBack, actions, style, below}: {title: string; eyebrow?: string; subtitle?: string; onBack?: () => void; actions?: React.ReactNode; style?: StyleProp<ViewStyle>; below?: React.ReactNode}) {
  const m = useMetrics();
  const canBack = useCanGoBack();
  const showBack = !m.isTV && canBack;
  return (
    <View style={[{paddingHorizontal: m.gutter, marginBottom: m.space.lg, gap: m.space.md}, style]}>
      {showBack ? <BackButton onPress={onBack} /> : null}
      <View style={{flexDirection: 'row', alignItems: 'flex-end', gap: m.space.md}}>
        <View style={{flex: 1, gap: 2}}>
          {eyebrow ? <Text variant="label" tone="accent" numberOfLines={1}>{eyebrow}</Text> : null}
          <Text variant="display" numberOfLines={2}>{title}</Text>
          {subtitle ? <Text variant="body" tone="secondary" numberOfLines={2}>{subtitle}</Text> : null}
        </View>
        {actions ? <View style={{flexDirection: 'row', gap: m.space.sm, alignItems: 'center'}}>{actions}</View> : null}
      </View>
      {below}
    </View>
  );
}

/** Whether this screen's stack has somewhere to go back to. */
export function useCanGoBack() {
  const navigation = useNavigation();
  const [can, setCan] = React.useState(() => navigation.canGoBack());
  useEffect(() => {
    const update = () => setCan(navigation.canGoBack());
    update();
    return navigation.addListener('state' as never, update);
  }, [navigation]);
  return can;
}

/** The standard back control on iPhone and iPad (TV uses the Menu button). */
export function BackButton({onPress, label = 'Back'}: {onPress?: () => void; label?: string}) {
  const m = useMetrics();
  const navigation = useNavigation();
  if (m.isTV) return null;
  return (
    <View style={{alignSelf: 'flex-start', marginLeft: -6}}>
      <Button label={label} icon="back" variant="ghost" size="sm" onPress={onPress ?? (() => navigation.goBack())} style={{paddingHorizontal: 6}} />
    </View>
  );
}

export function ErrorView({message, onRetry}: {message: string; onRetry?: () => void}) {
  const m = useMetrics();
  return (
    <View style={{alignItems: 'center', padding: m.space.xl, gap: m.space.sm}}>
      <Icon name="alert" size={m.isTV ? 48 : 32} color={colors.danger} />
      <Text variant="subheading" center>Something didn’t load</Text>
      <Text variant="body" tone="secondary" center style={{maxWidth: 560}}>{message}</Text>
      {onRetry ? <View style={{marginTop: m.space.sm}}><IconButton icon="refresh" label="Try again" onPress={onRetry} variant="secondary" /></View> : null}
    </View>
  );
}
