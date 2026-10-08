import React from 'react';
import {Text as RNText, type TextProps, type TextStyle, type StyleProp} from 'react-native';
import {colors, fonts, type TypeRole} from '../theme/tokens';
import {useMetrics} from '../theme/layout';

export type Tone = 'primary' | 'secondary' | 'tertiary' | 'accent' | 'danger' | 'amber' | 'healthy' | 'ink' | 'record';
const toneColor: Record<Tone, string> = {
  primary: colors.text,
  secondary: colors.textSecondary,
  tertiary: colors.textTertiary,
  accent: colors.accentStrong,
  danger: colors.danger,
  amber: colors.amber,
  healthy: colors.healthy,
  ink: colors.accentInk,
  record: colors.record,
};
const weightFont = {400: fonts.regular, 500: fonts.medium, 600: fonts.semibold, 700: fonts.bold} as const;

export function useType(role: TypeRole): TextStyle {
  const m = useMetrics();
  const t = m.type[role];
  return {fontFamily: weightFont[t.weight], fontSize: t.size, lineHeight: t.lineHeight, letterSpacing: t.tracking * t.size, textTransform: t.uppercase ? 'uppercase' : 'none'};
}

export function Text({variant = 'body', tone = 'primary', color, style, center, children, ...rest}: TextProps & {variant?: TypeRole; tone?: Tone; color?: string; center?: boolean; style?: StyleProp<TextStyle>}) {
  const type = useType(variant);
  return (
    <RNText allowFontScaling={false} {...rest} style={[type, {color: color ?? toneColor[tone]}, center ? {textAlign: 'center'} : null, style]}>
      {children}
    </RNText>
  );
}
