import React, {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {AccessibilityInfo, Animated, Easing, StyleSheet, View, useWindowDimensions} from 'react-native';
import Svg, {Defs, RadialGradient, Rect, Stop} from 'react-native-svg';
import {colors} from './tokens';

/**
 * Portico's projection glow, evolved: the navy canvas with blue light falling from the top left,
 * breathing slowly, and picking up the colour of whatever artwork has focus. The blue base never
 * goes away, so the glow always reads as Portico.
 */

// --- tint store (focused artwork colour) ---
let currentTint: string | null = null;
const listeners = new Set<() => void>();
let pending: ReturnType<typeof setTimeout> | null = null;
export const glow = {
  setTint(tint: string | null | undefined) {
    const next = tint ?? null;
    if (pending) clearTimeout(pending);
    // Debounce so fast scrolling through a row doesn't strobe the canvas.
    pending = setTimeout(() => {
      if (next === currentTint) return;
      currentTint = next;
      listeners.forEach(l => l());
    }, 120);
  },
  subscribe(l: () => void) {
    listeners.add(l);
    return () => listeners.delete(l);
  },
  get: () => currentTint,
};
export function useGlowTint() {
  return useSyncExternalStore(glow.subscribe, glow.get, glow.get);
}

function mix(a: string, b: string, t: number) {
  const pa = parseInt(a.slice(1), 16), pb = parseInt(b.slice(1), 16);
  const ch = (p: number, s: number) => (p >> s) & 255;
  const m = (s: number) => Math.round(ch(pa, s) * (1 - t) + ch(pb, s) * t);
  return `#${((1 << 24) + (m(16) << 16) + (m(8) << 8) + m(0)).toString(16).slice(1)}`;
}

function GlowLayer({width, height, color, id, strength = 1}: {width: number; height: number; color: string; id: string; strength?: number}) {
  return (
    <Svg width={width} height={height} style={StyleSheet.absoluteFill}>
      <Defs>
        <RadialGradient id={id} cx="14%" cy="-6%" rx="85%" ry="78%" fx="14%" fy="-6%" gradientUnits="objectBoundingBox">
          <Stop offset="0" stopColor={mix(color, '#ffffff', 0.08)} stopOpacity={0.95 * strength} />
          <Stop offset="0.35" stopColor={color} stopOpacity={0.55 * strength} />
          <Stop offset="1" stopColor={colors.ink} stopOpacity={0} />
        </RadialGradient>
        <RadialGradient id={id + 'b'} cx="100%" cy="100%" rx="60%" ry="50%" gradientUnits="objectBoundingBox">
          <Stop offset="0" stopColor={color} stopOpacity={0.22 * strength} />
          <Stop offset="1" stopColor={colors.ink} stopOpacity={0} />
        </RadialGradient>
      </Defs>
      <Rect x={0} y={0} width={width} height={height} fill={`url(#${id})`} />
      <Rect x={0} y={0} width={width} height={height} fill={`url(#${id}b)`} />
    </Svg>
  );
}

/** Full-screen canvas. Place once behind everything. `quiet` dims it (player, sheets). */
export function Glow({tinted = true, intensity = 1}: {tinted?: boolean; intensity?: number}) {
  const {width, height} = useWindowDimensions();
  const tint = useGlowTint();
  const breathe = useRef(new Animated.Value(0)).current;
  const [layers, setLayers] = useState<{key: number; color: string}[]>([]);
  const fades = useRef(new Map<number, Animated.Value>()).current;
  const [reduce, setReduce] = useState(false);

  useEffect(() => {
    AccessibilityInfo.isReduceMotionEnabled().then(setReduce).catch(() => {});
  }, []);

  useEffect(() => {
    if (reduce) return;
    const loop = Animated.loop(
      Animated.sequence([
        Animated.timing(breathe, {toValue: 1, duration: 9000, easing: Easing.inOut(Easing.sin), useNativeDriver: true}),
        Animated.timing(breathe, {toValue: 0, duration: 9000, easing: Easing.inOut(Easing.sin), useNativeDriver: true}),
      ]),
    );
    loop.start();
    return () => loop.stop();
  }, [breathe, reduce]);

  useEffect(() => {
    if (!tinted || !tint) {
      layers.forEach(l => {
        const f = fades.get(l.key);
        if (f) Animated.timing(f, {toValue: 0, duration: 700, useNativeDriver: true}).start();
      });
      return;
    }
    const key = Date.now();
    const v = new Animated.Value(0);
    fades.set(key, v);
    // Blend the artwork colour into Portico blue so the light stays recognisably ours.
    const color = mix(colors.accentDeep, tint, 0.55);
    setLayers(prev => [...prev.slice(-1), {key, color}]);
    Animated.timing(v, {toValue: 1, duration: 900, easing: Easing.out(Easing.quad), useNativeDriver: true}).start();
    layers.forEach(l => {
      const f = fades.get(l.key);
      if (f) {
        Animated.timing(f, {toValue: 0, duration: 900, useNativeDriver: true}).start(({finished}) => {
          if (!finished) return;
          fades.delete(l.key);
          setLayers(prev => prev.filter(x => x.key !== l.key));
        });
      }
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tint, tinted]);

  const drift = breathe.interpolate({inputRange: [0, 1], outputRange: [-width * 0.03, width * 0.03]});
  const breatheOpacity = breathe.interpolate({inputRange: [0, 1], outputRange: [0.86, 1]});

  return (
    <View pointerEvents="none" style={[StyleSheet.absoluteFill, {backgroundColor: colors.ink}]}>
      <Animated.View style={[styles.over, {opacity: Animated.multiply(breatheOpacity, intensity), transform: [{translateX: drift}]}]}>
        <GlowLayer width={width * 1.1} height={height} color={colors.ambient} id="base" strength={1} />
        <GlowLayer width={width * 1.1} height={height} color={colors.accentDeep} id="blue" strength={0.55} />
      </Animated.View>
      {layers.filter(l => fades.has(l.key)).map(l => (
        <Animated.View key={l.key} style={[styles.over, {opacity: Animated.multiply(fades.get(l.key)!, 0.75 * intensity), transform: [{translateX: drift}]}]}>
          <GlowLayer width={width * 1.1} height={height} color={l.color} id={`t${l.key}`} strength={0.9} />
        </Animated.View>
      ))}
    </View>
  );
}

const styles = StyleSheet.create({over: {position: 'absolute', top: 0, bottom: 0, left: '-5%', width: '110%'}});
