import {Platform, useWindowDimensions} from 'react-native';
import {typeScale} from './tokens';

export type Form = 'phone' | 'tablet' | 'tv';
export const isTV = Platform.isTV;

export type Metrics = ReturnType<typeof metricsFor>;

/** Every size in the app comes from here, per form factor. */
export function metricsFor(width: number, height: number) {
  const form: Form = isTV ? 'tv' : Math.min(width, height) >= 600 ? 'tablet' : 'phone';
  const tv = form === 'tv';
  const tablet = form === 'tablet';
  const landscape = width > height;
  return {
    form,
    isTV: tv,
    isPhone: form === 'phone',
    isTablet: tablet,
    width,
    height,
    landscape,
    type: tv ? typeScale.tenFoot : typeScale.handheld,
    gutter: tv ? 56 : tablet ? 28 : 16,
    gap: tv ? 28 : tablet ? 16 : 12,
    rowGap: tv ? 52 : tablet ? 34 : 26,
    space: tv ? {xs: 8, sm: 14, md: 22, lg: 34, xl: 52} : {xs: 4, sm: 8, md: 14, lg: 22, xl: 32},
    poster: tv ? {w: 206, h: 309} : tablet ? {w: 146, h: 219} : {w: 112, h: 168},
    wide: tv ? {w: 372, h: 209} : tablet ? {w: 284, h: 160} : {w: 232, h: 131},
    square: tv ? 232 : tablet ? 164 : 132,
    circle: tv ? 176 : tablet ? 118 : 92,
    radius: {small: tv ? 8 : 6, control: tv ? 14 : 9, card: tv ? 14 : 8, panel: tv ? 22 : 12, sheet: tv ? 28 : 16},
    control: tv ? 64 : 44,
    controlSmall: tv ? 52 : 34,
    icon: tv ? 30 : 20,
    iconSmall: tv ? 24 : 16,
    focusScale: tv ? 1.07 : 1,
    rail: tv ? {collapsed: 112, expanded: 388} : tablet ? {collapsed: 80, expanded: 252} : {collapsed: 0, expanded: 300},
    hero: tv ? Math.round(height * 0.74) : tablet ? Math.round(Math.min(height * 0.62, 640)) : Math.round(Math.min(width * 1.18, height * 0.66)),
  };
}

export function useMetrics(): Metrics {
  const {width, height} = useWindowDimensions();
  return metricsFor(width, height);
}

/** Columns and exact card width for a grid in `contentWidth`. */
export function gridFor(contentWidth: number, targetWidth: number, gap: number, min = 2) {
  const columns = Math.max(min, Math.floor((contentWidth + gap) / (targetWidth + gap)));
  const itemWidth = Math.floor((contentWidth - gap * (columns - 1)) / columns);
  return {columns, itemWidth};
}
