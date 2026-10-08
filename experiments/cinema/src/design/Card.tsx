import React from 'react';
import {StyleSheet, View, type StyleProp, type ViewStyle} from 'react-native';
import type {ArtRef, Shape} from '../data/types';
import {Artwork, Scrim} from './Artwork';
import {Focusable} from './Focusable';
import {glow} from './Glow';
import {Icon} from './Icon';
import {Text} from './Text';
import {colors, isTV, radius, u} from './tokens';

export type CardModel = {
  id: string;
  title: string;
  subtitle?: string;
  art?: ArtRef | null;
  tint: string;
  shape: Shape;
  progress?: number;
  watched?: boolean;
  badge?: string;
  /** Shown over the image bottom-left (episode code, duration). */
  overline?: string;
};

export function aspectFor(shape: Shape) {
  return shape === 'poster' ? 1.5 : shape === 'landscape' ? 0.5625 : 1;
}

type Props = {
  item: CardModel;
  width: number;
  onPress?: () => void;
  onLongPress?: () => void;
  onFocus?: () => void;
  selected?: boolean;
  selecting?: boolean;
  hasTVPreferredFocus?: boolean;
  /** TV shows titles only under the focused card, like the platform apps; touch always shows them. */
  showTitle?: 'always' | 'focus' | 'never';
  style?: StyleProp<ViewStyle>;
  nextFocusLeft?: number;
};

/**
 * A focusable artwork card. Focus lifts the card, draws a bright ring and throws a glow in the
 * artwork's own colour, and tints the canvas glow to match.
 */
export const Card = React.memo(function Card({item, width, onPress, onLongPress, onFocus, selected, selecting, hasTVPreferredFocus, showTitle, style, nextFocusLeft}: Props) {
  const circle = item.shape === 'circle';
  const h = Math.round(width * aspectFor(item.shape));
  const titleMode = showTitle ?? (isTV ? (circle ? 'always' : 'focus') : 'always');
  return (
    <Focusable
      onPress={onPress}
      onLongPress={onLongPress}
      hasTVPreferredFocus={hasTVPreferredFocus}
      nextFocusLeft={nextFocusLeft}
      accessibilityRole="button"
      accessibilityLabel={[item.title, item.subtitle].filter(Boolean).join(', ')}
      onFocus={() => {
        glow.setTint(item.tint);
        onFocus?.();
      }}
      scale={isTV ? 1.08 : 1}
      style={[{width}, style]}>
      {({focused}) => (
        <View>
          <View
            style={[
              {width, height: h, borderRadius: circle ? width / 2 : radius.md},
              styles.frame,
              focused && isTV && [styles.focused, {shadowColor: item.tint, borderRadius: circle ? width / 2 : radius.md}],
            ]}>
            <Artwork art={item.art} title={item.title} tint={item.tint} fallbackVariant={width > 200 ? 'title' : 'heading'} style={{width, height: h, borderRadius: circle ? width / 2 : radius.md}}>
              {item.progress ? (
                <>
                  <Scrim bottom={0.8} />
                  <View style={styles.progressTrack}>
                    <View style={[styles.progressFill, {width: `${Math.max(4, Math.min(100, item.progress * 100))}%`}]} />
                  </View>
                </>
              ) : null}
              {item.overline ? (
                <View style={styles.overline}>
                  <Text variant="micro" tone="white">{item.overline}</Text>
                </View>
              ) : null}
              {item.watched ? (
                <View style={styles.watched}>
                  <Icon name="check" size={11} color={colors.actionInk} strokeWidth={3} />
                </View>
              ) : null}
              {item.badge ? (
                <View style={styles.badge}>
                  <Text variant="micro" tone="ink">{item.badge}</Text>
                </View>
              ) : null}
              {selecting ? (
                <View style={[styles.select, selected && styles.selectOn]}>
                  {selected ? <Icon name="check" size={13} color={colors.actionInk} strokeWidth={3} /> : null}
                </View>
              ) : null}
            </Artwork>
            {focused && isTV ? <View pointerEvents="none" style={[StyleSheet.absoluteFill, styles.ring, {borderRadius: circle ? width / 2 : radius.md}]} /> : null}
          </View>
          {titleMode !== 'never' ? (
            <View style={[styles.caption, circle && {alignItems: 'center'}, titleMode === 'focus' && !focused && {opacity: 0}]}>
              <Text variant={isTV ? 'captionStrong' : 'captionStrong'} numberOfLines={1} center={circle}>{item.title}</Text>
              {item.subtitle ? <Text variant="caption" tone="tertiary" numberOfLines={1} center={circle}>{item.subtitle}</Text> : null}
            </View>
          ) : null}
        </View>
      )}
    </Focusable>
  );
});

const styles = StyleSheet.create({
  frame: {backgroundColor: colors.raised},
  focused: {shadowOpacity: 0.85, shadowRadius: 34, shadowOffset: {width: 0, height: 14}},
  ring: {borderWidth: 4, borderColor: colors.white},
  caption: {marginTop: u(8), minHeight: isTV ? 64 : 36},
  progressTrack: {position: 'absolute', left: u(8), right: u(8), bottom: u(8), height: u(4), borderRadius: 4, backgroundColor: 'rgba(255,255,255,0.28)', overflow: 'hidden'},
  progressFill: {height: '100%', backgroundColor: colors.accentStrong, borderRadius: 4},
  overline: {position: 'absolute', left: u(8), bottom: u(16)},
  watched: {position: 'absolute', top: u(6), right: u(6), width: u(18), height: u(18), borderRadius: u(9), backgroundColor: colors.accentStrong, alignItems: 'center', justifyContent: 'center'},
  badge: {position: 'absolute', top: u(6), left: u(6), paddingHorizontal: u(6), paddingVertical: u(2), borderRadius: u(4), backgroundColor: colors.amber},
  select: {position: 'absolute', top: u(6), right: u(6), width: u(22), height: u(22), borderRadius: u(11), borderWidth: 2, borderColor: colors.white, backgroundColor: 'rgba(7,16,24,0.5)', alignItems: 'center', justifyContent: 'center'},
  selectOn: {backgroundColor: colors.accentStrong, borderColor: colors.accentStrong},
});
