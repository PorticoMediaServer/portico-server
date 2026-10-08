import React, {forwardRef, useCallback, useContext, useEffect, useRef, useState} from 'react';
import {Animated, Pressable, StyleSheet, type PressableProps, type StyleProp, type View, type ViewStyle} from 'react-native';

const OUTER = ['flex', 'flexGrow', 'flexShrink', 'flexBasis', 'alignSelf', 'position', 'top', 'left', 'right', 'bottom', 'margin', 'marginTop', 'marginBottom', 'marginLeft', 'marginRight', 'marginHorizontal', 'marginVertical', 'zIndex'] as const;

/** Layout props belong on the pressable itself; visual props on the animated inner view. */
function split(style: StyleProp<ViewStyle>): [ViewStyle, ViewStyle] {
  const flat = (StyleSheet.flatten(style) ?? {}) as Record<string, unknown>;
  const outer: Record<string, unknown> = {};
  const inner: Record<string, unknown> = {...flat};
  for (const k of OUTER) if (k in flat) { outer[k] = flat[k]; delete inner[k]; }
  if (outer.flex !== undefined || outer.flexGrow !== undefined || outer.alignSelf === 'stretch') { inner.flexGrow = 1; }
  if (outer.position === 'absolute') { inner.width = inner.width ?? '100%'; inner.height = inner.height ?? '100%'; }
  return [outer as ViewStyle, inner as ViewStyle];
}
import {isTV} from '../theme/layout';
import {offerContentFocus, lastContentFocus} from './focusMemory';
import {FocusGateContext} from './focusGate';
import {colors} from '../theme/tokens';

export type FocusState = {focused: boolean; pressed: boolean};

type Props = Omit<PressableProps, 'style' | 'children'> & {
  style?: StyleProp<ViewStyle> | ((s: FocusState) => StyleProp<ViewStyle>);
  children?: React.ReactNode | ((s: FocusState) => React.ReactNode);
  /** Scale on TV focus (1 = none). */
  focusScale?: number;
  /** Draw Portico's focus ring around the element on TV. */
  ring?: boolean;
  ringRadius?: number;
  hasTVPreferredFocus?: boolean;
  onFocusChange?: (focused: boolean) => void;
  /** Remember this element as the place to return to from the rail (TV). */
  rememberFocus?: boolean;
  /** When a screen opens, start here rather than at the top-left control (e.g. the selected tab). */
  pageFocus?: boolean;
};

/**
 * Every interactive element goes through here. On TV it animates a gentle
 * scale and draws the focus ring with a blue glow; on touch it dims on press.
 */
export const Focusable = forwardRef<View, Props>(function Focusable({style, children, focusScale = 1, ring = false, ringRadius = 12, onFocus, onBlur, onFocusChange, disabled, hasTVPreferredFocus, rememberFocus = true, pageFocus, ...rest}, ref) {
  const inner = useRef<View | null>(null);
  const setRef = useCallback((v: View | null) => {
    inner.current = v;
    if (typeof ref === 'function') ref(v); else if (ref) ref.current = v;
  }, [ref]);
  useEffect(() => () => { if (lastContentFocus.current && lastContentFocus.current === inner.current) lastContentFocus.current = null; }, []);
  const [focused, setFocused] = useState(false);
  const gate = useContext(FocusGateContext);
  const blocked = !!gate?.gated && !hasTVPreferredFocus;
  // Preferred focus is a one-shot request. Left on, tvOS re-requests it on every
  // layout pass and pulls focus back from wherever the viewer moved it.
  const [prefer, setPrefer] = useState(!!hasTVPreferredFocus && isTV);
  useEffect(() => {
    if (!isTV || !hasTVPreferredFocus) { setPrefer(false); return; }
    setPrefer(true);
    const t = setTimeout(() => setPrefer(false), 900);
    return () => clearTimeout(t);
  }, [hasTVPreferredFocus]);
  const alive = useRef(true);
  useEffect(() => {
    if (hasTVPreferredFocus && !disabled) gate?.claim();
    // Only on mount: a modal's preferred control announces itself as the modal opens.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  useEffect(() => {
    alive.current = true;
    if (isTV && rememberFocus && !disabled && !gate) {
      offerContentFocus({preferred: !!hasTVPreferredFocus || !!pageFocus, view: () => inner.current, take: () => { if (alive.current) setPrefer(true); }});
    }
    return () => { alive.current = false; };
    // Only on mount: a control of a newly opened screen.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  useEffect(() => {
    if (!prefer || hasTVPreferredFocus) return;
    const t = setTimeout(() => setPrefer(false), 900);
    return () => clearTimeout(t);
  }, [prefer, hasTVPreferredFocus]);
  const scale = useRef(new Animated.Value(1)).current;
  const animate = useCallback((to: number) => {
    Animated.spring(scale, {toValue: to, useNativeDriver: true, speed: 22, bounciness: 3}).start();
  }, [scale]);
  return (
    <Pressable
      ref={setRef}
      disabled={disabled}
      hasTVPreferredFocus={prefer}
      onFocus={e => {
        setFocused(true);
        if (prefer) setPrefer(false);
        if (gate?.gated) gate.release();
        if (isTV && rememberFocus) lastContentFocus.current = inner.current;
        if (isTV && focusScale !== 1) animate(focusScale);
        onFocusChange?.(true);
        onFocus?.(e);
      }}
      onBlur={e => {
        setFocused(false);
        if (isTV && focusScale !== 1) animate(1);
        onFocusChange?.(false);
        onBlur?.(e);
      }}
      {...rest}
      {...(blocked ? {focusable: false} : null)}
      style={({pressed}) => split(typeof style === 'function' ? style({focused: focused && isTV, pressed}) : style)[0]}
    >
      {({pressed}) => {
        const state = {focused: focused && isTV, pressed};
        const resolved = split(typeof style === 'function' ? style(state) : style)[1];
        return (
          <Animated.View style={[resolved, {transform: [{scale}]}, !isTV && pressed ? {opacity: 0.72} : null, disabled ? {opacity: 0.45} : null]}>
            {typeof children === 'function' ? children(state) : children}
            {ring && state.focused ? <Animated.View pointerEvents="none" style={{position: 'absolute', left: -5, right: -5, top: -5, bottom: -5, borderRadius: ringRadius + 5, borderWidth: 3, borderColor: colors.focus, shadowColor: colors.accent, shadowOpacity: 0.85, shadowRadius: 18, shadowOffset: {width: 0, height: 0}}} /> : null}
          </Animated.View>
        );
      }}
    </Pressable>
  );
});
