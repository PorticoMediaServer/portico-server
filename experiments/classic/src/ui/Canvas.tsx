import React, {useEffect, useRef} from 'react';
import {Animated, Easing, StyleSheet, View, useWindowDimensions, type StyleProp, type ViewStyle} from 'react-native';
import Svg, {Defs, LinearGradient, RadialGradient, Rect, Stop} from 'react-native-svg';
import {colors} from '../theme/tokens';
import {usePrefs} from '../app/prefs';

/**
 * Portico's signature projection glow: a navy canvas lit by a blue beam
 * falling from the top left. In Classic it breathes very slowly, and a second,
 * softer layer tints toward the title in focus (see `tint`).
 */
export const Atmosphere = React.memo(function Atmosphere({tint, intensity = 1}: {tint?: string; intensity?: number}) {
  const {width, height} = useWindowDimensions();
  const reduce = usePrefs().reduceMotion;
  const breathe = useRef(new Animated.Value(0)).current;
  const tintOpacity = useRef(new Animated.Value(0)).current;
  const lastTint = useRef<string | undefined>(tint);
  if (tint) lastTint.current = tint;
  useEffect(() => {
    if (reduce) return;
    const loop = Animated.loop(Animated.sequence([
      Animated.timing(breathe, {toValue: 1, duration: 9000, easing: Easing.inOut(Easing.sin), useNativeDriver: true}),
      Animated.timing(breathe, {toValue: 0, duration: 9000, easing: Easing.inOut(Easing.sin), useNativeDriver: true}),
    ]));
    loop.start();
    return () => loop.stop();
  }, [breathe, reduce]);
  useEffect(() => {
    Animated.timing(tintOpacity, {toValue: tint ? 1 : 0, duration: 700, useNativeDriver: true}).start();
  }, [tint, tintOpacity]);
  const glowOpacity = breathe.interpolate({inputRange: [0, 1], outputRange: [0.86 * intensity, 1 * intensity]});
  return (
    <View pointerEvents="none" style={[StyleSheet.absoluteFill, {backgroundColor: colors.ink}]}>
      <Animated.View style={[StyleSheet.absoluteFill, {opacity: glowOpacity}]}>
        <Svg width={width} height={height}>
          <Defs>
            <RadialGradient id="beam" cx="14%" cy="-4%" rx="82%" ry="74%" fx="14%" fy="-4%" gradientUnits="objectBoundingBox">
              <Stop offset="0" stopColor={colors.raisedBright} stopOpacity={1} />
              <Stop offset="0.38" stopColor={colors.ambient} stopOpacity={0.72} />
              <Stop offset="1" stopColor={colors.ink} stopOpacity={0} />
            </RadialGradient>
            <RadialGradient id="spill" cx="100%" cy="100%" rx="60%" ry="48%" gradientUnits="objectBoundingBox">
              <Stop offset="0" stopColor={colors.ambient} stopOpacity={0.3} />
              <Stop offset="1" stopColor={colors.ink} stopOpacity={0} />
            </RadialGradient>
          </Defs>
          <Rect x={0} y={0} width={width} height={height} fill="url(#beam)" />
          <Rect x={0} y={0} width={width} height={height} fill="url(#spill)" />
        </Svg>
      </Animated.View>
      {lastTint.current ? (
        <Animated.View style={[StyleSheet.absoluteFill, {opacity: tintOpacity}]}>
          <Svg width={width} height={height}>
            <Defs>
              <RadialGradient id="tint" cx="22%" cy="0%" rx="70%" ry="62%" gradientUnits="objectBoundingBox">
                <Stop offset="0" stopColor={lastTint.current} stopOpacity={0.34} />
                <Stop offset="1" stopColor={lastTint.current} stopOpacity={0} />
              </RadialGradient>
            </Defs>
            <Rect x={0} y={0} width={width} height={height} fill="url(#tint)" />
          </Svg>
        </Animated.View>
      ) : null}
    </View>
  );
});

/** Scrims for artwork: a bottom fade and (on wide layouts) a left fade into the canvas. */
export function Scrim({height, width, left = true, bottom = true, top = false, strength = 1, style}: {height: number; width: number; left?: boolean; bottom?: boolean; top?: boolean; strength?: number; style?: StyleProp<ViewStyle>}) {
  return (
    <Svg width={width} height={height} style={[StyleSheet.absoluteFill, style]} pointerEvents="none">
      <Defs>
        <LinearGradient id="sb" x1="0" y1="0" x2="0" y2="1">
          <Stop offset="0.35" stopColor={colors.ink} stopOpacity={0} />
          <Stop offset="0.78" stopColor={colors.ink} stopOpacity={0.7 * strength} />
          <Stop offset="1" stopColor={colors.ink} stopOpacity={1} />
        </LinearGradient>
        <LinearGradient id="sl" x1="0" y1="0" x2="1" y2="0">
          <Stop offset="0" stopColor={colors.ink} stopOpacity={0.92 * strength} />
          <Stop offset="0.45" stopColor={colors.ink} stopOpacity={0.45 * strength} />
          <Stop offset="0.75" stopColor={colors.ink} stopOpacity={0} />
        </LinearGradient>
        <LinearGradient id="st" x1="0" y1="0" x2="0" y2="1">
          <Stop offset="0" stopColor={colors.ink} stopOpacity={0.7 * strength} />
          <Stop offset="0.3" stopColor={colors.ink} stopOpacity={0} />
        </LinearGradient>
      </Defs>
      {left ? <Rect x={0} y={0} width={width} height={height} fill="url(#sl)" /> : null}
      {bottom ? <Rect x={0} y={0} width={width} height={height} fill="url(#sb)" /> : null}
      {top ? <Rect x={0} y={0} width={width} height={height} fill="url(#st)" /> : null}
    </Svg>
  );
}
