/**
 * Portico Classic design tokens. Colors follow Portico's shared palette
 * (navy canvas, blue projection glow, Portico blue actions); the type scale has
 * a handheld column and a ten-foot column designed at 1920×1080.
 */
export const colors = {
  ink: '#071018',
  inkDeep: '#050b11',
  raisedSoft: '#0c1a25',
  raised: '#10212d',
  raisedHigh: '#162c3a',
  raisedBright: '#1d3848',
  overlay: '#132634',
  ambient: '#153749',
  accent: '#409fd4',
  accentStrong: '#5cb3e2',
  accentDeep: '#2f86bb',
  accentInk: '#03121c',
  text: '#edf5fa',
  textSecondary: '#bdccd6',
  textTertiary: '#96a9b5',
  placeholder: '#8298a6',
  lineSoft: '#1c3341',
  line: '#28404f',
  lineStrong: '#3b5668',
  lineControl: '#637580',
  danger: '#f38b88',
  dangerDeep: '#c65a57',
  focus: '#d5efff',
  amber: '#e8bc6a',
  record: '#f17979',
  healthy: '#86cfb3',
  track: '#35505f',
  scrim: 'rgba(7,16,24,0.86)',
  scrimSoft: 'rgba(7,16,24,0.55)',
} as const;

export type ColorName = keyof typeof colors;

export const profileArt = {
  blue: '#28587a',
  violet: '#685294',
  mint: '#28665d',
  coral: '#985243',
  gold: '#796120',
  slate: '#4a596a',
  rose: '#854d73',
  sky: '#376f8c',
} as const;
export type ProfileArt = keyof typeof profileArt;

export const fonts = {
  regular: 'Manrope-Regular',
  medium: 'Manrope-Medium',
  semibold: 'Manrope-SemiBold',
  bold: 'Manrope-Bold',
} as const;

export type TypeRole = 'display' | 'title' | 'heading' | 'subheading' | 'body' | 'bodyStrong' | 'caption' | 'captionStrong' | 'micro' | 'label';
type TypeStyle = {size: number; lineHeight: number; weight: 400 | 500 | 600 | 700; tracking: number; uppercase?: boolean};

export const typeScale: Record<'handheld' | 'tenFoot', Record<TypeRole, TypeStyle>> = {
  handheld: {
    display: {size: 34, lineHeight: 39, weight: 700, tracking: -0.026},
    title: {size: 26, lineHeight: 31, weight: 700, tracking: -0.023},
    heading: {size: 20, lineHeight: 25, weight: 600, tracking: -0.0175},
    subheading: {size: 17, lineHeight: 22, weight: 600, tracking: -0.012},
    body: {size: 15, lineHeight: 21, weight: 400, tracking: 0},
    bodyStrong: {size: 15, lineHeight: 21, weight: 600, tracking: 0},
    caption: {size: 13, lineHeight: 18, weight: 400, tracking: 0},
    captionStrong: {size: 13, lineHeight: 18, weight: 600, tracking: 0},
    micro: {size: 11, lineHeight: 14, weight: 500, tracking: 0.018},
    label: {size: 12, lineHeight: 16, weight: 600, tracking: 0.05, uppercase: true},
  },
  tenFoot: {
    display: {size: 60, lineHeight: 66, weight: 700, tracking: -0.029},
    title: {size: 42, lineHeight: 50, weight: 700, tracking: -0.027},
    heading: {size: 30, lineHeight: 38, weight: 600, tracking: -0.019},
    subheading: {size: 26, lineHeight: 33, weight: 600, tracking: -0.011},
    body: {size: 23, lineHeight: 32, weight: 400, tracking: 0},
    bodyStrong: {size: 23, lineHeight: 32, weight: 600, tracking: 0},
    caption: {size: 20, lineHeight: 27, weight: 400, tracking: 0},
    captionStrong: {size: 20, lineHeight: 27, weight: 600, tracking: 0},
    micro: {size: 17, lineHeight: 22, weight: 500, tracking: 0.018},
    label: {size: 18, lineHeight: 24, weight: 600, tracking: 0.055, uppercase: true},
  },
};

export const motion = {quick: 120, standard: 200, slow: 320} as const;
