import React, {forwardRef} from 'react';
import {ActivityIndicator, View, type StyleProp, type ViewStyle} from 'react-native';
import {colors} from '../theme/tokens';
import {useMetrics} from '../theme/layout';
import {Focusable} from './Focusable';
import {Icon, type IconName} from './Icon';
import {Text} from './Text';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'outline';

type Props = {
  label?: string;
  icon?: IconName;
  iconRight?: IconName;
  onPress?: () => void;
  onLongPress?: () => void;
  variant?: ButtonVariant;
  size?: 'lg' | 'md' | 'sm';
  selected?: boolean;
  disabled?: boolean;
  loading?: boolean;
  style?: StyleProp<ViewStyle>;
  hasTVPreferredFocus?: boolean;
  accessibilityLabel?: string;
  onFocus?: () => void;
  grow?: boolean;
  badge?: string | number;
  nextFocusUp?: number;
  nextFocusDown?: number;
  nextFocusLeft?: number;
  nextFocusRight?: number;
};

export const Button = forwardRef<View, Props>(function Button({label, icon, iconRight, onPress, onLongPress, variant = 'secondary', size = 'md', selected, disabled, loading, style, hasTVPreferredFocus, accessibilityLabel, onFocus, grow, badge, ...focusProps}, ref) {
  const m = useMetrics();
  const height = size === 'lg' ? m.control + (m.isTV ? 4 : 4) : size === 'sm' ? m.controlSmall : m.control;
  const iconOnly = !label;
  const pad = iconOnly ? 0 : size === 'sm' ? (m.isTV ? 20 : 12) : m.isTV ? 30 : 18;
  const iconSize = size === 'sm' ? m.iconSmall : m.icon;
  const base = variant === 'primary' ? colors.accent : variant === 'danger' ? 'rgba(243,139,136,0.14)' : variant === 'ghost' ? 'transparent' : variant === 'outline' ? 'transparent' : colors.raisedBright;
  const fg = variant === 'primary' ? colors.accentInk : variant === 'danger' ? colors.danger : colors.text;
  return (
    <Focusable
      ref={ref}
      onPress={onPress}
      onLongPress={onLongPress}
      disabled={disabled || loading}
      // tvOS never focuses a control that can't be used (a busy one keeps its focus).
      {...(m.isTV && disabled && !loading ? {focusable: false} : null)}
      focusScale={m.isTV ? 1.06 : 1}
      hasTVPreferredFocus={hasTVPreferredFocus}
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel ?? label}
      accessibilityState={{selected, disabled}}
      onFocus={onFocus}
      {...focusProps}
      style={({focused, pressed}) => [
        {
          height,
          minWidth: height,
          paddingHorizontal: pad,
          borderRadius: size === 'sm' && m.isTV ? m.radius.control * 0.85 : m.radius.control,
          backgroundColor: focused ? colors.focus : selected && variant !== 'primary' ? colors.accentDeep : pressed && variant === 'primary' ? colors.accentDeep : base,
          borderWidth: variant === 'outline' && !focused ? 1.5 : 0,
          borderColor: colors.lineStrong,
          flexDirection: 'row',
          alignItems: 'center',
          justifyContent: 'center',
          gap: m.isTV ? 12 : 8,
          alignSelf: grow ? 'stretch' : 'flex-start',
          shadowColor: colors.accent,
          shadowOpacity: focused ? 0.6 : 0,
          shadowRadius: 16,
          shadowOffset: {width: 0, height: 0},
        },
        style,
      ]}
    >
      {({focused}) => {
        const color = focused ? colors.accentInk : fg;
        return (
          <>
            {loading ? <ActivityIndicator color={color} size="small" /> : icon ? <Icon name={icon} size={iconSize} color={color} /> : null}
            {label ? <Text variant={size === 'sm' ? 'captionStrong' : 'bodyStrong'} color={color} numberOfLines={1}>{label}</Text> : null}
            {iconRight ? <Icon name={iconRight} size={iconSize} color={color} /> : null}
            {badge !== undefined ? (
              <View style={{minWidth: m.isTV ? 30 : 20, height: m.isTV ? 30 : 20, borderRadius: 15, paddingHorizontal: 6, backgroundColor: focused ? colors.accentDeep : colors.accent, alignItems: 'center', justifyContent: 'center'}}>
                <Text variant="micro" color={colors.accentInk}>{badge}</Text>
              </View>
            ) : null}
          </>
        );
      }}
    </Focusable>
  );
});

/** Round icon-only control used in transport bars, headers and cards. */
export const IconButton = forwardRef<View, {icon: IconName; onPress?: () => void; label: string; size?: number; variant?: 'ghost' | 'secondary' | 'primary'; selected?: boolean; disabled?: boolean; style?: StyleProp<ViewStyle>; hasTVPreferredFocus?: boolean; filled?: boolean}>(function IconButton({icon, onPress, label, size, variant = 'ghost', selected, disabled, style, hasTVPreferredFocus, filled}, ref) {
  const m = useMetrics();
  const box = size ?? m.control;
  const bg = variant === 'primary' || filled ? colors.accent : variant === 'secondary' ? colors.raisedBright : 'transparent';
  return (
    <Focusable
      ref={ref}
      onPress={onPress}
      disabled={disabled}
      focusScale={m.isTV ? 1.1 : 1}
      accessibilityRole="button"
      accessibilityLabel={label}
      accessibilityState={{selected}}
      hasTVPreferredFocus={hasTVPreferredFocus}
      style={({focused}) => [{width: box, height: box, borderRadius: box / 2, alignItems: 'center', justifyContent: 'center', backgroundColor: focused ? colors.focus : selected ? colors.accentDeep : bg}, style]}
    >
      {({focused}) => <Icon name={icon} size={Math.round(box * 0.46)} color={focused ? colors.accentInk : variant === 'primary' || filled ? colors.accentInk : selected ? colors.text : colors.text} />}
    </Focusable>
  );
});
