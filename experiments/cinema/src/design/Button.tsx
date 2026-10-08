import React from 'react';
import {ActivityIndicator, StyleSheet, View, type StyleProp, type ViewStyle} from 'react-native';
import {Focusable} from './Focusable';
import {Icon, type IconName} from './Icon';
import {Text} from './Text';
import {colors, isTV, radius, u} from './tokens';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'glass';

type Props = {
  label?: string;
  icon?: IconName;
  iconRight?: IconName;
  onPress?: () => void;
  onLongPress?: () => void;
  variant?: ButtonVariant;
  size?: 'sm' | 'md' | 'lg';
  disabled?: boolean;
  busy?: boolean;
  active?: boolean;
  hasTVPreferredFocus?: boolean;
  style?: StyleProp<ViewStyle>;
  accessibilityLabel?: string;
  onFocus?: () => void;
  grow?: boolean;
};

const bg: Record<ButtonVariant, string> = {
  primary: colors.accent,
  secondary: 'rgba(237,245,250,0.12)',
  glass: 'rgba(7,16,24,0.55)',
  ghost: 'transparent',
  danger: 'rgba(243,139,136,0.16)',
};

/**
 * Pill buttons. Focus on TV turns any button bright white with ink text and lifts it: the
 * clearest possible focus signal at ten feet. On touch, pressing dims slightly.
 */
export function Button({label, icon, iconRight, onPress, onLongPress, variant = 'secondary', size = 'md', disabled, busy, active, hasTVPreferredFocus, style, accessibilityLabel, onFocus, grow}: Props) {
  const h = size === 'lg' ? u(50) : size === 'sm' ? u(34) : u(42);
  const padH = label ? (size === 'sm' ? u(12) : u(18)) : 0;
  return (
    <Focusable
      onPress={onPress}
      onLongPress={onLongPress}
      disabled={disabled || busy}
      hasTVPreferredFocus={hasTVPreferredFocus}
      onFocus={onFocus}
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel ?? label}
      accessibilityState={{disabled, selected: active}}
      scale={1.07}
      style={[grow && {flexGrow: 1}, style]}>
      {({focused, pressed}) => {
        const lit = focused && isTV;
        const fore = lit ? colors.actionInk : variant === 'primary' ? colors.actionInk : variant === 'danger' ? colors.danger : colors.text;
        return (
          <View
            style={[
              styles.base,
              {height: h, minWidth: h, paddingHorizontal: padH, borderRadius: radius.pill, backgroundColor: lit ? colors.white : active ? colors.raisedBright : bg[variant]},
              variant === 'ghost' && !lit && active && {backgroundColor: 'rgba(237,245,250,0.1)'},
              variant === 'secondary' && !lit && styles.hairline,
              lit && styles.lit,
              pressed && !isTV && {opacity: 0.8},
            ]}>
            {busy ? <ActivityIndicator color={fore} /> : icon ? <Icon name={icon} size={size === 'sm' ? 15 : 18} color={fore} fill={icon === 'play' || icon === 'pause' ? fore : undefined} /> : null}
            {label ? <Text variant="button" color={fore} numberOfLines={1} style={[{marginLeft: icon ? u(8) : 0}, size === 'sm' && {fontSize: isTV ? 22 : 13}]}>{label}</Text> : null}
            {iconRight ? <View style={{marginLeft: u(6)}}><Icon name={iconRight} size={15} color={fore} /></View> : null}
          </View>
        );
      }}
    </Focusable>
  );
}

/** Round icon-only button. */
export function IconButton({icon, onPress, label, size = 40, variant = 'secondary', active, hasTVPreferredFocus, fill, color, disabled, style}: {icon: IconName; onPress?: () => void; label: string; size?: number; variant?: ButtonVariant; active?: boolean; hasTVPreferredFocus?: boolean; fill?: boolean; color?: string; disabled?: boolean; style?: StyleProp<ViewStyle>}) {
  const d = u(size);
  return (
    <Focusable onPress={onPress} disabled={disabled} accessibilityRole="button" accessibilityLabel={label} hasTVPreferredFocus={hasTVPreferredFocus} scale={1.12} style={style}>
      {({focused}) => {
        const lit = focused && isTV;
        const fore = lit ? colors.actionInk : color ?? (active ? colors.accentStrong : colors.text);
        return (
          <View style={[styles.base, {width: d, height: d, borderRadius: d / 2, backgroundColor: lit ? colors.white : variant === 'ghost' ? 'transparent' : bg[variant]}, lit && styles.lit]}>
            <Icon name={icon} size={size * 0.46} color={fore} fill={fill ? fore : undefined} />
          </View>
        );
      }}
    </Focusable>
  );
}

const styles = StyleSheet.create({
  base: {flexDirection: 'row', alignItems: 'center', justifyContent: 'center'},
  hairline: {borderWidth: StyleSheet.hairlineWidth, borderColor: 'rgba(237,245,250,0.18)'},
  lit: {shadowColor: colors.accentStrong, shadowOpacity: 0.7, shadowRadius: 22, shadowOffset: {width: 0, height: 6}},
});
