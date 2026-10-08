import React from 'react';
import {Text as RNText, type TextProps, type TextStyle, StyleSheet} from 'react-native';
import {colors, fonts, isTV, u} from './tokens';

export type Variant = 'hero' | 'display' | 'title' | 'heading' | 'subheading' | 'body' | 'bodyStrong' | 'caption' | 'captionStrong' | 'eyebrow' | 'micro' | 'button' | 'mono';
export type Tone = 'primary' | 'secondary' | 'tertiary' | 'accent' | 'danger' | 'amber' | 'record' | 'healthy' | 'ink' | 'white';

const toneColor: Record<Tone, string> = {
  primary: colors.text, secondary: colors.textSecondary, tertiary: colors.textTertiary, accent: colors.accentStrong,
  danger: colors.danger, amber: colors.amber, record: colors.record, healthy: colors.healthy, ink: colors.actionInk, white: colors.white,
};

const styles = StyleSheet.create({
  hero: {fontFamily: fonts.displayHeavy, fontSize: isTV ? 84 : 38, lineHeight: isTV ? 92 : 43, letterSpacing: isTV ? -1.6 : -0.9},
  display: {fontFamily: fonts.display, fontSize: u(29), lineHeight: u(34), letterSpacing: isTV ? -0.9 : -0.5},
  title: {fontFamily: fonts.display, fontSize: u(20), lineHeight: u(26), letterSpacing: isTV ? -0.4 : -0.2},
  heading: {fontFamily: fonts.bold, fontSize: u(19), lineHeight: u(25), letterSpacing: -0.2},
  subheading: {fontFamily: fonts.semibold, fontSize: u(16), lineHeight: u(22)},
  body: {fontFamily: fonts.regular, fontSize: isTV ? 26 : 15, lineHeight: isTV ? 38 : 22},
  bodyStrong: {fontFamily: fonts.semibold, fontSize: isTV ? 26 : 15, lineHeight: isTV ? 38 : 22},
  caption: {fontFamily: fonts.medium, fontSize: isTV ? 23 : 13, lineHeight: isTV ? 32 : 18},
  captionStrong: {fontFamily: fonts.bold, fontSize: isTV ? 23 : 13, lineHeight: isTV ? 32 : 18},
  eyebrow: {fontFamily: fonts.bold, fontSize: isTV ? 20 : 11, lineHeight: isTV ? 28 : 15, letterSpacing: isTV ? 2.4 : 1.4, textTransform: 'uppercase'},
  micro: {fontFamily: fonts.bold, fontSize: isTV ? 18 : 10, lineHeight: isTV ? 24 : 13, letterSpacing: 0.4},
  button: {fontFamily: fonts.bold, fontSize: isTV ? 26 : 15, lineHeight: isTV ? 32 : 20},
  mono: {fontFamily: 'Menlo', fontSize: isTV ? 22 : 12, lineHeight: isTV ? 30 : 17},
});

export function Text({variant = 'body', tone = 'primary', color, center, style, ...rest}: TextProps & {variant?: Variant; tone?: Tone; color?: string; center?: boolean}) {
  const s: TextStyle[] = [styles[variant], {color: color ?? toneColor[tone]}];
  if (center) s.push({textAlign: 'center'});
  const flat = StyleSheet.flatten(style) as TextStyle | undefined;
  if (flat?.fontSize && !flat.lineHeight) s.push({lineHeight: Math.round(flat.fontSize * 1.18)});
  return <RNText allowFontScaling={!isTV} maxFontSizeMultiplier={1.4} {...rest} style={[s, style]} />;
}

/** An editorial heading: "Because you watched" + the subject in Portico blue. */
export function EditorialTitle({lead, subject, variant = 'title', style}: {lead: string; subject?: string; variant?: Variant; style?: TextStyle}) {
  return (
    <Text variant={variant} numberOfLines={1} style={style}>
      {lead}
      {subject ? <Text variant={variant} tone="accent">{' '}{subject}</Text> : null}
    </Text>
  );
}
