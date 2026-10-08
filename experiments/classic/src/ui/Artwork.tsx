import React, {useRef, useState} from 'react';
import {Animated, Image, StyleSheet, View, type ImageStyle, type StyleProp, type ViewStyle} from 'react-native';
import Svg, {Defs, LinearGradient, Rect, Stop} from 'react-native-svg';
import {colors, profileArt} from '../theme/tokens';
import type {Img} from '../data/types';
import {Text} from './Text';
import {hashColor} from './color';

/**
 * Every picture in the app. Fades in when decoded; with no artwork it draws a
 * designed placeholder (a tinted gradient and the title's initials), never an
 * empty box.
 */
export function Artwork({source, title, width, height, radius = 8, style, round, resizeMode = 'cover', imageStyle, fade = true, blur}: {source?: Img; title?: string; width: number | `${number}%`; height: number | `${number}%`; radius?: number; style?: StyleProp<ViewStyle>; round?: boolean; resizeMode?: 'cover' | 'contain'; imageStyle?: StyleProp<ImageStyle>; fade?: boolean; blur?: number}) {
  const opacity = useRef(new Animated.Value(fade ? 0 : 1)).current;
  const [failed, setFailed] = useState(false);
  const r = round && typeof width === 'number' ? width / 2 : radius;
  const showFallback = !source || failed;
  return (
    <View style={[{width, height, borderRadius: r, overflow: 'hidden', backgroundColor: colors.raised}, style]}>
      {showFallback ? <Placeholder title={title} /> : (
        <Animated.Image
          source={source!}
          resizeMode={resizeMode}
          blurRadius={blur}
          onLoad={() => fade && Animated.timing(opacity, {toValue: 1, duration: 220, useNativeDriver: true}).start()}
          onError={() => setFailed(true)}
          style={[StyleSheet.absoluteFill, {width: '100%', height: '100%', opacity}, imageStyle]}
        />
      )}
    </View>
  );
}

function Placeholder({title}: {title?: string}) {
  const base = hashColor(title ?? 'portico', Object.values(profileArt));
  const initials = (title ?? '').replace(/^(the|a|an)\s+/i, '').split(/\s+/).slice(0, 2).map(w => w[0]).join('').toUpperCase();
  return (
    <View style={StyleSheet.absoluteFill}>
      <Svg width="100%" height="100%" style={StyleSheet.absoluteFill}>
        <Defs>
          <LinearGradient id="ph" x1="0" y1="0" x2="1" y2="1">
            <Stop offset="0" stopColor={base} stopOpacity={0.95} />
            <Stop offset="1" stopColor={colors.raisedSoft} stopOpacity={1} />
          </LinearGradient>
        </Defs>
        <Rect x="0" y="0" width="100%" height="100%" fill="url(#ph)" />
      </Svg>
      <View style={[StyleSheet.absoluteFill, {alignItems: 'center', justifyContent: 'center'}]}>
        <Text variant="title" tone="secondary" style={{opacity: 0.9}}>{initials}</Text>
      </View>
    </View>
  );
}

export const prefetchable = Image;
