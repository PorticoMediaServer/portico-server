import React from 'react';
import {KeyboardAvoidingView, ScrollView, StyleSheet, TVFocusGuideView, View} from 'react-native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {Wordmark} from '../../design/Brand';
import {IconButton} from '../../design/Button';
import {Text} from '../../design/Text';
import {colors, isTV, radius, u} from '../../design/tokens';
import {useLayout} from '../../design/useLayout';
import {back, navigationRef} from '../../nav/routes';

/** Frame for getting-in screens: the wordmark over the glow, a glass card, a serif headline. */
export function AuthFrame({title, lede, children, wide, footer, hideBack}: {title: string; lede?: string; children?: React.ReactNode; wide?: boolean; footer?: React.ReactNode; hideBack?: boolean}) {
  const insets = useSafeAreaInsets();
  const layout = useLayout();
  const canBack = !hideBack && navigationRef.isReady() && navigationRef.canGoBack();
  const cardW = isTV ? (wide ? 1200 : 900) : Math.min(layout.width - 32, wide ? 720 : 480);
  return (
    <KeyboardAvoidingView behavior={isTV ? undefined : 'padding'} style={{flex: 1}}>
      <ScrollView contentContainerStyle={[styles.scroll, {paddingTop: insets.top + u(20), paddingBottom: insets.bottom + u(30)}]} keyboardShouldPersistTaps="handled">
        <View style={styles.brand}>
          <Wordmark size={isTV ? 44 : 26} />
        </View>
        <TVFocusGuideView autoFocus style={[styles.card, {width: cardW}]}>
          <Text variant="display" center>{title}</Text>
          {lede ? <Text variant="body" tone="secondary" center style={{marginTop: u(8)}}>{lede}</Text> : null}
          <View style={{marginTop: u(20)}}>{children}</View>
        </TVFocusGuideView>
        {footer ? <View style={{marginTop: u(16), width: cardW}}>{footer}</View> : null}
        <Text variant="caption" tone="tertiary" center style={{marginTop: u(24)}}>Terms of Use · Privacy Policy</Text>
      </ScrollView>
      {canBack && !isTV ? (
        <View style={{position: 'absolute', top: insets.top + 6, left: 12}}>
          <IconButton icon="chevronLeft" label="Back" onPress={back} size={36} variant="glass" />
        </View>
      ) : null}
    </KeyboardAvoidingView>
  );
}

export function Divider({label}: {label: string}) {
  return (
    <View style={{flexDirection: 'row', alignItems: 'center', marginVertical: u(14)}}>
      <View style={{flex: 1, height: StyleSheet.hairlineWidth, backgroundColor: colors.line}} />
      <Text variant="caption" tone="tertiary" style={{marginHorizontal: u(10)}}>{label}</Text>
      <View style={{flex: 1, height: StyleSheet.hairlineWidth, backgroundColor: colors.line}} />
    </View>
  );
}

const styles = StyleSheet.create({
  scroll: {flexGrow: 1, alignItems: 'center', justifyContent: 'center', paddingHorizontal: 16},
  brand: {marginBottom: u(24)},
  card: {backgroundColor: 'rgba(16,33,45,0.78)', borderRadius: radius.xl, padding: u(24), borderWidth: StyleSheet.hairlineWidth, borderColor: colors.line},
});
