import React from 'react';
import {Image, StyleSheet, View, type ImageStyle, type StyleProp, type ViewStyle} from 'react-native';
import Svg, {Defs, LinearGradient, Rect, Stop} from 'react-native-svg';
import {resolveArt} from '../data/art';
import type {ArtRef} from '../data/types';
import {Text} from './Text';
import {colors} from './tokens';

/**
 * An artwork image with a designed fallback: when a key has no image, a tinted gradient with the
 * title set in the display serif stands in (the same treatment a real server's missing art gets).
 */
export function Artwork({art, title, tint = colors.raisedBright, style, imageStyle, resizeMode = 'cover', fallbackVariant = 'title', children}: {art?: ArtRef | null; title?: string; tint?: string; style?: StyleProp<ViewStyle>; imageStyle?: StyleProp<ImageStyle>; resizeMode?: 'cover' | 'contain'; fallbackVariant?: 'display' | 'title' | 'heading' | 'caption'; children?: React.ReactNode}) {
  const src = resolveArt(art);
  return (
    <View style={[styles.box, style]}>
      {src ? (
        <Image source={src} style={[StyleSheet.absoluteFill, {width: '100%', height: '100%'}, imageStyle]} resizeMode={resizeMode} fadeDuration={150} />
      ) : (
        <View style={StyleSheet.absoluteFill}>
          <Svg width="100%" height="100%" style={StyleSheet.absoluteFill}>
            <Defs>
              <LinearGradient id="fb" x1="0" y1="0" x2="1" y2="1">
                <Stop offset="0" stopColor={tint} stopOpacity={0.9} />
                <Stop offset="1" stopColor={colors.ink} stopOpacity={1} />
              </LinearGradient>
            </Defs>
            <Rect x="0" y="0" width="100%" height="100%" fill="url(#fb)" />
          </Svg>
          {title ? (
            <View style={styles.fallbackText}>
              <Text variant={fallbackVariant} numberOfLines={3} center>{title}</Text>
            </View>
          ) : null}
        </View>
      )}
      {children}
    </View>
  );
}

/** A bottom (and optional left) ink gradient over artwork so text stays readable. */
export function Scrim({bottom = 0.95, left = 0, top = 0, style}: {bottom?: number; left?: number; top?: number; style?: StyleProp<ViewStyle>}) {
  return (
    <Svg width="100%" height="100%" style={[StyleSheet.absoluteFill, style]} preserveAspectRatio="none">
      <Defs>
        <LinearGradient id="sb" x1="0" y1="0" x2="0" y2="1">
          <Stop offset="0" stopColor={colors.ink} stopOpacity={top} />
          <Stop offset="0.45" stopColor={colors.ink} stopOpacity={0} />
          <Stop offset="1" stopColor={colors.ink} stopOpacity={bottom} />
        </LinearGradient>
        <LinearGradient id="sl" x1="0" y1="0" x2="1" y2="0">
          <Stop offset="0" stopColor={colors.ink} stopOpacity={left} />
          <Stop offset="0.6" stopColor={colors.ink} stopOpacity={0} />
        </LinearGradient>
      </Defs>
      <Rect x="0" y="0" width="100%" height="100%" fill="url(#sb)" />
      {left > 0 ? <Rect x="0" y="0" width="100%" height="100%" fill="url(#sl)" /> : null}
    </Svg>
  );
}

const styles = StyleSheet.create({
  box: {overflow: 'hidden', backgroundColor: colors.raised},
  fallbackText: {...StyleSheet.absoluteFillObject, alignItems: 'center', justifyContent: 'center', padding: 10},
});
