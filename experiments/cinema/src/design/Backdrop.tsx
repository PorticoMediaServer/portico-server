import React, {useEffect, useRef, useState} from 'react';
import {Animated, Image, StyleSheet, View, type StyleProp, type ViewStyle} from 'react-native';
import Svg, {Defs, LinearGradient, Rect, Stop} from 'react-native-svg';
import {resolveArt} from '../data/art';
import type {ArtRef} from '../data/types';
import {colors} from './tokens';

type Layer = {key: number; art: ArtRef};

/**
 * Full-bleed artwork that crossfades as the focused title changes, with Portico's ink scrims
 * (bottom, plus left on wide layouts) so type stays readable. A slow push-in adds depth.
 */
export function Backdrop({art, style, left = true, bottom = 1, top = 0.25, kenBurns = true, dim = 0}: {art?: ArtRef | null; style?: StyleProp<ViewStyle>; left?: boolean; bottom?: number; top?: number; kenBurns?: boolean; dim?: number}) {
  const [layers, setLayers] = useState<Layer[]>([]);
  useEffect(() => {
    if (!art) return;
    setLayers(prev => (prev.length && prev[prev.length - 1].art === art ? prev : [...prev.slice(-1), {key: Date.now(), art}]));
  }, [art]);
  return (
    <View pointerEvents="none" style={[StyleSheet.absoluteFill, {overflow: 'hidden'}, style]}>
      {layers.map((l, i) => (
        <FadeLayer key={l.key} art={l.art} last={i === layers.length - 1} kenBurns={kenBurns} onGone={() => setLayers(p => p.filter(x => x.key !== l.key))} />
      ))}
      <Svg width="100%" height="100%" style={StyleSheet.absoluteFill} preserveAspectRatio="none">
        <Defs>
          <LinearGradient id="bdb" x1="0" y1="0" x2="0" y2="1">
            <Stop offset="0" stopColor={colors.ink} stopOpacity={top} />
            <Stop offset="0.35" stopColor={colors.ink} stopOpacity={0} />
            <Stop offset="0.72" stopColor={colors.ink} stopOpacity={bottom * 0.55} />
            <Stop offset="1" stopColor={colors.ink} stopOpacity={bottom} />
          </LinearGradient>
          <LinearGradient id="bdl" x1="0" y1="0" x2="1" y2="0">
            <Stop offset="0" stopColor={colors.ink} stopOpacity={0.92} />
            <Stop offset="0.45" stopColor={colors.ink} stopOpacity={0.45} />
            <Stop offset="0.75" stopColor={colors.ink} stopOpacity={0} />
          </LinearGradient>
        </Defs>
        {left ? <Rect x="0" y="0" width="100%" height="100%" fill="url(#bdl)" /> : null}
        <Rect x="0" y="0" width="100%" height="100%" fill="url(#bdb)" />
        {dim ? <Rect x="0" y="0" width="100%" height="100%" fill={colors.ink} opacity={dim} /> : null}
      </Svg>
    </View>
  );
}

function FadeLayer({art, last, kenBurns, onGone}: {art: ArtRef; last: boolean; kenBurns: boolean; onGone: () => void}) {
  const opacity = useRef(new Animated.Value(0)).current;
  const scale = useRef(new Animated.Value(1.02)).current;
  const gone = useRef(onGone);
  gone.current = onGone;
  useEffect(() => {
    Animated.timing(opacity, {toValue: last ? 1 : 0, duration: 650, useNativeDriver: true}).start(({finished}) => finished && !last && gone.current());
    if (last && kenBurns) Animated.timing(scale, {toValue: 1.1, duration: 22000, useNativeDriver: true}).start();
  }, [last, opacity, scale, kenBurns]);
  const src = resolveArt(art);
  if (!src) return null;
  return <Animated.Image source={src} resizeMode="cover" style={[StyleSheet.absoluteFill, {width: '100%', height: '100%', opacity, transform: [{scale}]}]} />;
}

export {Image};
