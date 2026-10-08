import React, {forwardRef, useCallback, useEffect, useRef, useState} from 'react';
import {Animated, Pressable, StyleSheet, type PressableProps, type StyleProp, type View, type ViewStyle} from 'react-native';
import {focusMemory} from './focusMemory';
import {isTV, motion} from './tokens';

// Placement keys belong on the focusable view itself: tvOS moves focus using that view's frame,
// so a Pressable wrapping an absolutely positioned child would have no size and be unreachable.
const OUTER = new Set(['position', 'top', 'left', 'right', 'bottom', 'width', 'height', 'minWidth', 'maxWidth', 'minHeight', 'maxHeight', 'margin', 'marginTop', 'marginBottom', 'marginLeft', 'marginRight', 'marginHorizontal', 'marginVertical', 'flex', 'flexGrow', 'flexShrink', 'flexBasis', 'alignSelf', 'zIndex']);
function splitStyle(style: StyleProp<ViewStyle>) {
  const flat = (StyleSheet.flatten(style) ?? {}) as Record<string, unknown>;
  const outer: Record<string, unknown> = {};
  const inner: Record<string, unknown> = {};
  for (const k of Object.keys(flat)) (OUTER.has(k) ? outer : inner)[k] = flat[k];
  // A sized outer view: let the visual layer fill it.
  if (outer.height !== undefined || outer.flex !== undefined || (outer.top !== undefined && outer.bottom !== undefined)) inner.flexGrow = 1;
  return {outer: outer as ViewStyle, inner: inner as ViewStyle};
}

export type FocusState = {focused: boolean; pressed: boolean};

type Props = Omit<PressableProps, 'children' | 'style'> & {
  children: React.ReactNode | ((s: FocusState) => React.ReactNode);
  style?: StyleProp<ViewStyle>;
  /** Extra style while focused (TV) or pressed (touch). */
  focusStyle?: StyleProp<ViewStyle>;
  /** Scale on TV focus. 1 disables. */
  scale?: number;
  hasTVPreferredFocus?: boolean;
  nextFocusUp?: number;
  nextFocusDown?: number;
  nextFocusLeft?: number;
  nextFocusRight?: number;
};

/**
 * The one interactive primitive. On TV it tracks focus and lifts the element (scale, ring and glow
 * are drawn by callers through the render function); on touch it gives a pressed state.
 */
export const Focusable = forwardRef<View, Props>(function Focusable(
  {children, style, focusStyle, scale = 1.06, onFocus, onBlur, disabled, hasTVPreferredFocus, ...rest},
  ref,
) {
  const [focused, setFocused] = useState(false);
  // Preferred focus is a one-time claim: when the element mounts (or the flag turns on), not on
  // every render. react-native-tvos re-asserts it whenever the view updates, so two elements that
  // both carried it (a library's active tab and its first card) traded focus on every re-render.
  const [preferred, setPreferred] = useState(!!hasTVPreferredFocus);
  useEffect(() => { if (hasTVPreferredFocus) setPreferred(true); }, [hasTVPreferredFocus]);
  useEffect(() => {
    if (!preferred) return;
    const t = setTimeout(() => setPreferred(false), 1200);
    return () => clearTimeout(t);
  }, [preferred]);
  const anim = useRef(new Animated.Value(1)).current;
  const own = useRef<View | null>(null);
  const setRef = useCallback(
    (v: View | null) => {
      own.current = v;
      if (typeof ref === 'function') ref(v);
      else if (ref) ref.current = v;
    },
    [ref],
  );
  const animate = useCallback(
    (to: number) => {
      Animated.spring(anim, {toValue: to, useNativeDriver: true, speed: 22, bounciness: to > 1 ? 5 : 0}).start();
    },
    [anim],
  );
  const {outer, inner} = splitStyle(style);
  return (
    <Pressable
      ref={setRef}
      style={outer}
      disabled={disabled}
      focusable={!disabled}
      hasTVPreferredFocus={isTV && preferred}
      onFocus={e => {
        setFocused(true);
        setPreferred(false);
        if (isTV) focusMemory.set(own.current);
        if (isTV && scale !== 1) animate(scale);
        onFocus?.(e);
      }}
      onBlur={e => {
        setFocused(false);
        if (isTV && scale !== 1) animate(1);
        onBlur?.(e);
      }}
      onPressIn={() => !isTV && animate(0.97)}
      onPressOut={() => !isTV && animate(1)}
      android_ripple={undefined}
      {...rest}>
      {({pressed}) => (
        <Animated.View style={[inner, {transform: [{scale: anim}]}, (focused || (pressed && !isTV)) && focusStyle, disabled && {opacity: 0.45}]}>
          {typeof children === 'function' ? children({focused, pressed}) : children}
        </Animated.View>
      )}
    </Pressable>
  );
});

export const focusDuration = motion.fast;
