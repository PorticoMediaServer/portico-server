import React, {useEffect, useRef} from 'react';
import {Animated, Easing, StyleSheet, View, type StyleProp, type ViewStyle} from 'react-native';
import {Button} from './Button';
import {Icon, type IconName} from './Icon';
import {Text} from './Text';
import {colors, isTV, radius, u} from './tokens';
import {useLayout} from './useLayout';

/** A shimmering placeholder block. */
export function Skeleton({width, height, radius: r = radius.md, style}: {width: number | `${number}%`; height: number; radius?: number; style?: StyleProp<ViewStyle>}) {
  const v = useRef(new Animated.Value(0)).current;
  useEffect(() => {
    const loop = Animated.loop(Animated.sequence([
      Animated.timing(v, {toValue: 1, duration: 800, easing: Easing.inOut(Easing.quad), useNativeDriver: true}),
      Animated.timing(v, {toValue: 0, duration: 800, easing: Easing.inOut(Easing.quad), useNativeDriver: true}),
    ]));
    loop.start();
    return () => loop.stop();
  }, [v]);
  return <Animated.View style={[{width, height, borderRadius: r, backgroundColor: colors.raisedHigh, opacity: v.interpolate({inputRange: [0, 1], outputRange: [0.45, 0.9]})}, style]} />;
}

/** Skeleton rows of cards, the loading state of Home, Discover and search. */
export function RowsSkeleton({rows = 3, shape = 'poster', hero}: {rows?: number; shape?: 'poster' | 'landscape' | 'square'; hero?: boolean}) {
  const layout = useLayout();
  const w = shape === 'landscape' ? layout.landscapeWidth : shape === 'square' ? layout.squareWidth : layout.posterWidth;
  const h = shape === 'landscape' ? w * 0.5625 : shape === 'square' ? w : w * 1.5;
  return (
    <View accessibilityLabel="Loading" style={{paddingTop: hero ? 0 : u(10)}}>
      {hero ? <Skeleton width="100%" height={isTV ? 640 : layout.width * 1.1} radius={0} style={{marginBottom: u(20)}} /> : null}
      {Array.from({length: rows}).map((_, r) => (
        <View key={r} style={{paddingLeft: layout.gutter, marginBottom: u(26)}}>
          <Skeleton width={u(180)} height={u(20)} style={{marginBottom: u(12)}} />
          <View style={{flexDirection: 'row', gap: isTV ? 36 : 12}}>
            {Array.from({length: 7}).map((__, i) => <Skeleton key={i} width={w} height={h} />)}
          </View>
        </View>
      ))}
    </View>
  );
}

export function GridSkeleton({columns, width, aspect = 1.5, count = 12}: {columns: number; width: number; aspect?: number; count?: number}) {
  return (
    <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: isTV ? 36 : 12}}>
      {Array.from({length: count}).map((_, i) => <Skeleton key={i} width={width} height={width * aspect} />)}
      <View style={{width: columns}} />
    </View>
  );
}

/** Empty state: icon medallion, a serif headline, a line of help and up to two actions. */
export function EmptyState({icon = 'sparkles', title, body, action, secondary, compact}: {icon?: IconName; title: string; body?: string; action?: {label: string; onPress: () => void; icon?: IconName}; secondary?: {label: string; onPress: () => void}; compact?: boolean}) {
  return (
    <View style={[styles.empty, compact && {paddingVertical: u(24)}]}>
      <View style={styles.medallion}>
        <Icon name={icon} size={26} color={colors.accentStrong} />
      </View>
      <Text variant="title" center style={{marginTop: u(16)}}>{title}</Text>
      {body ? <Text variant="body" tone="secondary" center style={{marginTop: u(8), maxWidth: u(420)}}>{body}</Text> : null}
      {action || secondary ? (
        <View style={{flexDirection: 'row', gap: u(10), marginTop: u(20), flexWrap: 'wrap', justifyContent: 'center'}}>
          {action ? <Button label={action.label} icon={action.icon} variant="primary" onPress={action.onPress} hasTVPreferredFocus /> : null}
          {secondary ? <Button label={secondary.label} onPress={secondary.onPress} /> : null}
        </View>
      ) : null}
    </View>
  );
}

export function ErrorState({title = 'This couldn’t load', body = 'Check that your server is reachable, then try again.', onRetry}: {title?: string; body?: string; onRetry?: () => void}) {
  return <EmptyState icon="cloudOff" title={title} body={body} action={onRetry ? {label: 'Try again', onPress: onRetry, icon: 'refresh'} : undefined} />;
}

/** Inline notice strip (reconnecting, scanning, server-wide problems). */
export function Banner({icon = 'info', tone = 'info', title, body, action}: {icon?: IconName; tone?: 'info' | 'warn' | 'danger' | 'success'; title: string; body?: string; action?: {label: string; onPress: () => void}}) {
  const c = {info: colors.accentStrong, warn: colors.amber, danger: colors.danger, success: colors.healthy}[tone];
  return (
    <View style={[styles.banner, {borderColor: c + '66', backgroundColor: c + '1f'}]}>
      <Icon name={icon} size={18} color={c} />
      <View style={{flex: 1, marginLeft: u(10)}}>
        <Text variant="bodyStrong">{title}</Text>
        {body ? <Text variant="caption" tone="secondary">{body}</Text> : null}
      </View>
      {action ? <Button label={action.label} size="sm" onPress={action.onPress} /> : null}
    </View>
  );
}

const styles = StyleSheet.create({
  empty: {alignItems: 'center', justifyContent: 'center', paddingVertical: u(56), paddingHorizontal: u(24)},
  medallion: {width: u(64), height: u(64), borderRadius: u(32), alignItems: 'center', justifyContent: 'center', backgroundColor: 'rgba(92,179,226,0.12)', borderWidth: 1, borderColor: 'rgba(92,179,226,0.3)'},
  banner: {flexDirection: 'row', alignItems: 'center', borderRadius: radius.md, borderWidth: 1, padding: u(12), marginBottom: u(14)},
});
