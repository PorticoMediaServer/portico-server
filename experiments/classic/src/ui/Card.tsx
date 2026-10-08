import React, {memo} from 'react';
import {View} from 'react-native';
import {colors} from '../theme/tokens';
import {useMetrics} from '../theme/layout';
import type {Entry, Shape} from '../data/types';
import {Artwork} from './Artwork';
import {Focusable} from './Focusable';
import {Icon} from './Icon';
import {Text} from './Text';
import {ProgressBar} from './Controls';
import {usePrefs} from '../app/prefs';
import {useSelection} from '../app/selection';

export function cardSize(m: ReturnType<typeof useMetrics>, shape: Shape, scale = 1) {
  switch (shape) {
    case 'wide': return {w: Math.round(m.wide.w * scale), h: Math.round(m.wide.h * scale)};
    case 'square': return {w: Math.round(m.square * scale), h: Math.round(m.square * scale)};
    case 'circle': return {w: Math.round(m.circle * scale), h: Math.round(m.circle * scale)};
    default: return {w: Math.round(m.poster.w * scale), h: Math.round(m.poster.h * scale)};
  }
}

type CardProps = {
  entry: Entry;
  shape?: Shape;
  width?: number;
  onPress?: (e: Entry) => void;
  onLongPress?: (e: Entry) => void;
  onFocus?: (e: Entry) => void;
  selected?: boolean;
  selecting?: boolean;
  hideText?: boolean;
  hasTVPreferredFocus?: boolean;
  subtitleOverride?: string;
};

/**
 * The one media card. Poster, wide (episodes, Continue Watching), square
 * (albums, photos) and circle (people, artists). Watched ticks, unwatched
 * counts and progress bars sit on the artwork, Plex-style; title and subtitle
 * sit underneath.
 */
export const MediaCard = memo(function MediaCard({entry, shape, width, onPress, onLongPress, onFocus, selected: selectedProp, selecting: selectingProp, hideText, hasTVPreferredFocus, subtitleOverride}: CardProps) {
  const m = useMetrics();
  const sel = useSelection();
  const selecting = selectingProp ?? sel.active;
  const selected = selectedProp ?? sel.isSelected(entry.id);
  const {cardScale} = usePrefs();
  const s = shape ?? entry.shape;
  const base = cardSize(m, s, cardScale);
  const w = width ?? base.w;
  const h = Math.round((w * base.h) / base.w);
  const round = s === 'circle';
  const radius = round ? w / 2 : m.radius.card;
  const center = round;
  const live = entry.badge === 'LIVE';
  return (
    <Focusable
      onPress={() => onPress?.(entry)}
      onLongPress={() => onLongPress?.(entry)}
      onFocus={() => onFocus?.(entry)}
      focusScale={m.isTV ? m.focusScale : 1}
      hasTVPreferredFocus={hasTVPreferredFocus}
      accessibilityRole="button"
      accessibilityLabel={`${entry.title}${entry.subtitle ? `, ${entry.subtitle}` : ''}`}
      style={{width: w}}
    >
      {({focused}) => (
        <View>
          <View style={{width: w, height: h}}>
            <Artwork source={s === 'poster' && entry.poster ? entry.poster : entry.image} title={entry.kind === 'episode' ? entry.subtitle ?? entry.title : entry.title} width={w} height={h} radius={radius} round={round} />
            {focused ? <View pointerEvents="none" style={{position: 'absolute', left: -5, right: -5, top: -5, bottom: -5, borderRadius: radius + 5, borderWidth: 3, borderColor: colors.focus, shadowColor: colors.accent, shadowOpacity: 0.9, shadowRadius: 22, shadowOffset: {width: 0, height: 0}}} /> : null}
            {entry.watched && !entry.progress && entry.kind !== 'person' ? (
              <View style={{position: 'absolute', top: m.isTV ? 10 : 6, right: m.isTV ? 10 : 6, width: m.isTV ? 34 : 22, height: m.isTV ? 34 : 22, borderRadius: 20, backgroundColor: colors.accent, alignItems: 'center', justifyContent: 'center'}}>
                <Icon name="check" size={m.isTV ? 22 : 14} color={colors.accentInk} strokeWidth={2.6} />
              </View>
            ) : null}
            {entry.unwatched && !entry.watched && entry.kind === 'show' ? (
              <View style={{position: 'absolute', top: m.isTV ? 10 : 6, right: m.isTV ? 10 : 6, minWidth: m.isTV ? 36 : 24, height: m.isTV ? 36 : 24, paddingHorizontal: 6, borderRadius: 18, backgroundColor: colors.accent, alignItems: 'center', justifyContent: 'center'}}>
                <Text variant="micro" color={colors.accentInk} style={{fontFamily: 'Manrope-Bold'}}>{entry.unwatched}</Text>
              </View>
            ) : null}
            {live ? (
              <View style={{position: 'absolute', top: m.isTV ? 12 : 8, left: m.isTV ? 12 : 8, paddingHorizontal: 8, paddingVertical: 2, borderRadius: 6, backgroundColor: colors.record}}>
                <Text variant="micro" color="#2a0707" style={{fontFamily: 'Manrope-Bold'}}>LIVE</Text>
              </View>
            ) : null}
            {entry.progress && entry.progress > 0 && entry.progress < 1 ? (
              <View style={{position: 'absolute', left: m.isTV ? 12 : 8, right: m.isTV ? 12 : 8, bottom: m.isTV ? 12 : 8}}>
                <ProgressBar value={entry.progress} height={m.isTV ? 6 : 4} />
              </View>
            ) : null}
            {selecting ? (
              <View style={{position: 'absolute', top: 8, left: 8, width: m.isTV ? 34 : 24, height: m.isTV ? 34 : 24, borderRadius: 17, borderWidth: 2, borderColor: colors.text, backgroundColor: selected ? colors.accent : 'rgba(7,16,24,0.5)', alignItems: 'center', justifyContent: 'center'}}>
                {selected ? <Icon name="check" size={m.isTV ? 20 : 14} color={colors.accentInk} strokeWidth={2.6} /> : null}
              </View>
            ) : null}
            {selecting && selected ? <View pointerEvents="none" style={{position: 'absolute', left: 0, right: 0, top: 0, bottom: 0, borderRadius: radius, borderWidth: 3, borderColor: colors.accent}} /> : null}
          </View>
          {hideText ? null : (
            <View style={{marginTop: m.isTV ? 14 : 8, alignItems: center ? 'center' : 'flex-start', paddingHorizontal: 2}}>
              <Text variant="captionStrong" numberOfLines={1} center={center}>{subtitleOverride !== undefined ? entry.title : cardLines(entry, s)[0]}</Text>
              <Text variant="caption" tone="tertiary" numberOfLines={1} center={center}>{subtitleOverride ?? cardLines(entry, s)[1]}</Text>
            </View>
          )}
        </View>
      )}
    </Focusable>
  );
});

export function cardLines(e: Entry, _shape?: Shape): [string, string] {
  if (e.kind === 'episode') {
    const code = (e.caption ?? '').split(' · ').filter(p => /^(S\d+|E\d+|Special)/.test(p)).join(' · ');
    return [e.subtitle ?? e.title, [code, e.title].filter(Boolean).join(' · ')];
  }
  if ((e.kind === 'movie' || e.kind === 'homevideo') && e.progress) return [e.title, `${Math.max(1, Math.round(((e.duration ?? 0) * (1 - e.progress)) / 60))} min left`];
  return [e.title, e.subtitle ?? ''];
}
