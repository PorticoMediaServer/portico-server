import {Dimensions, Platform} from 'react-native';

/** One codebase serves phone, tablet and TV; layout decisions go through these flags. */
export const isTV = Platform.isTV;

/** Portico's palette. The navy canvas and the blue projection glow are the brand. */
export const colors = {
  ink: '#071018',
  inkDeep: '#040a10',
  raised: '#10212d',
  raisedHigh: '#162c3a',
  raisedBright: '#1d3848',
  raisedSoft: '#0c1a25',
  overlay: '#132634',
  ambient: '#153749',
  accent: '#409fd4',
  accentStrong: '#5cb3e2',
  accentDeep: '#2f86bb',
  text: '#edf5fa',
  textSecondary: '#bdccd6',
  textTertiary: '#96a9b5',
  placeholder: '#8298a6',
  lineSoft: '#1c3341',
  line: '#28404f',
  lineStrong: '#3b5668',
  lineControl: '#637580',
  danger: '#f38b88',
  focus: '#d5efff',
  amber: '#e8bc6a',
  record: '#f17979',
  healthy: '#86cfb3',
  trackRest: '#35505f',
  actionInk: '#03121c',
  white: '#ffffff',
  black: '#000000',
  scrim: 'rgba(4,10,16,0.72)',
} as const;

export type ColorName = keyof typeof colors;

/**
 * Scale factor: TV layouts are designed on a 1920-point canvas viewed from across the room,
 * phones on a ~390-point canvas held close. `u(n)` converts a phone-sized value.
 */
const tvScale = 1.9;
export const u = (n: number) => (isTV ? Math.round(n * tvScale) : n);

export const radius = {sm: u(6), md: u(10), lg: u(16), xl: u(24), pill: 999};
export const space = {xxs: u(2), xs: u(4), sm: u(8), md: u(12), lg: u(16), xl: u(24), xxl: u(32), xxxl: u(48)};

export const fonts = {
  // Display: Inter Tight for heroes, titles and shelf headings; Manrope (Portico Sans) for the UI.
  display: 'InterTight-Bold',
  displayHeavy: 'InterTight-ExtraBold',
  displaySemibold: 'InterTight-SemiBold',
  regular: 'Manrope-Regular',
  medium: 'Manrope-Medium',
  semibold: 'Manrope-SemiBold',
  bold: 'Manrope-Bold',
};

export type Layout = {
  width: number;
  height: number;
  /** phone portrait / small; tablet and TV are "wide". */
  compact: boolean;
  tablet: boolean;
  tv: boolean;
  /** Horizontal page gutter. */
  gutter: number;
  /** Width reserved on the left for the collapsed rail. */
  railWidth: number;
  posterWidth: number;
  landscapeWidth: number;
  squareWidth: number;
};

export function computeLayout(width = Dimensions.get('window').width, height = Dimensions.get('window').height): Layout {
  const tv = isTV;
  const compact = !tv && width < 600;
  const tablet = !tv && !compact;
  const railWidth = tv ? 112 : tablet ? 76 : 0;
  const gutter = tv ? 64 : tablet ? 32 : 18;
  const content = width - railWidth - gutter * 2;
  const posterWidth = tv ? 232 : tablet ? Math.min(168, content / 5.2) : Math.round((content - 24) / 2.7);
  const landscapeWidth = tv ? 420 : tablet ? 300 : Math.round(content * 0.78);
  const squareWidth = tv ? 250 : tablet ? 180 : Math.round((content - 24) / 2.3);
  return {width, height, compact, tablet, tv, gutter, railWidth, posterWidth, landscapeWidth, squareWidth};
}

export const motion = {fast: 140, base: 220, slow: 420, hero: 700};

/** Palette names used by fixtures, mapped to a representative artwork tint. */
export const paletteTint: Record<string, string> = {
  teal: '#2fa4a0', amber: '#d99a3a', crimson: '#c2424f', violet: '#8a5fd0', emerald: '#3fae78',
  cobalt: '#3f6fe0', rose: '#d4668f', gold: '#d8b44a', slate: '#6f8796', orange: '#e0763a',
};
