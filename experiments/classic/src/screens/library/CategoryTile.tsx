import React from 'react';
import {View} from 'react-native';
import Svg, {Defs, LinearGradient, Rect, Stop} from 'react-native-svg';
import type {Category} from '../../data/types';
import {Focusable} from '../../ui/Focusable';
import {Artwork} from '../../ui/Artwork';
import {Text} from '../../ui/Text';
import {hashColor} from '../../ui/color';
import {colors} from '../../theme/tokens';
import {useMetrics} from '../../theme/layout';

// Deep, even-weight colours so white text always reads and no tile shouts.
const TILE = ['#1f4e79', '#553070', '#1d5a52', '#7a3d2b', '#6a5419', '#2c3f78', '#6d2c4b', '#23607c', '#3c5a2b', '#5d3b20', '#40306e', '#1c5566'];

/** A category tile: a colour field with the name, fading into a picture from the category. */
export function CategoryTile({category, width, onPress}: {category: Category; width: number; onPress: () => void}) {
  const m = useMetrics();
  const h = Math.round(width * 0.5);
  const base = hashColor(category.kind + category.title, TILE);
  const iw = Math.round(width * 0.64);
  const id = `cat-${category.id.replace(/[^a-z0-9]/gi, '')}`;
  return (
    <Focusable onPress={onPress} focusScale={m.isTV ? 1.05 : 1} accessibilityLabel={`${category.title}, ${category.count} ${category.count === 1 ? 'title' : 'titles'}`} style={{width}}>
      {({focused}) => (
        <View style={{width, height: h}}>
          <View style={{position: 'absolute', left: 0, right: 0, top: 0, bottom: 0, borderRadius: m.radius.card, overflow: 'hidden', backgroundColor: base}}>
            <Artwork source={category.images[0]} title={category.title} width={iw} height={h} radius={0} style={{position: 'absolute', right: 0, top: 0, opacity: 0.85}} />
            <Svg width={iw} height={h} style={{position: 'absolute', right: 0, top: 0}} pointerEvents="none">
              <Defs>
                <LinearGradient id={id} x1="0" y1="0" x2="1" y2="0">
                  <Stop offset="0" stopColor={base} stopOpacity={1} />
                  <Stop offset="0.55" stopColor={base} stopOpacity={0.35} />
                  <Stop offset="1" stopColor={base} stopOpacity={0.1} />
                </LinearGradient>
              </Defs>
              <Rect x="0" y="0" width={iw} height={h} fill={`url(#${id})`} />
            </Svg>
            <View style={{flex: 1, justifyContent: 'flex-end', padding: m.isTV ? 22 : 12, paddingRight: width * 0.3}}>
              <Text variant={m.isPhone ? 'subheading' : 'heading'} numberOfLines={2}>{category.title}</Text>
              <Text variant="caption" color="rgba(237,245,250,0.78)">{category.count} {category.count === 1 ? 'title' : 'titles'}</Text>
            </View>
          </View>
          {focused ? <View pointerEvents="none" style={{position: 'absolute', left: -5, right: -5, top: -5, bottom: -5, borderRadius: m.radius.card + 5, borderWidth: 3, borderColor: colors.focus, shadowColor: colors.accent, shadowOpacity: 0.9, shadowRadius: 22, shadowOffset: {width: 0, height: 0}}} /> : null}
        </View>
      )}
    </Focusable>
  );
}
