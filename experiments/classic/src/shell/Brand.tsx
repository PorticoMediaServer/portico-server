import React from 'react';
import {Animated, View} from 'react-native';
import Svg, {G, Path} from 'react-native-svg';
import {LETTER_PATHS, SYMBOL_PATHS} from './brandPaths';

// The Portico brand set: white artwork on dark surfaces, never recoloured,
// stretched or re-spaced. Proportions come from the masters in assets/brand.
const SYMBOL_W = 304;
const LETTERS_X = 510;
const WORD_W = 1774;
const H = 405;
const WHITE = '#FFFFFF';

/** The Play-P symbol, `size` points tall. */
export function BrandMark({size = 32}: {size?: number}) {
  return (
    <Svg width={(size * SYMBOL_W) / H} height={size} viewBox={`206 184 ${SYMBOL_W} ${H}`} accessibilityLabel="Portico">
      {SYMBOL_PATHS.map((d, i) => <Path key={i} d={d} fill={WHITE} />)}
    </Svg>
  );
}

function Letters({height}: {height: number}) {
  const w = WORD_W - (LETTERS_X - 206);
  return (
    <Svg width={(height * w) / H} height={height} viewBox={`${LETTERS_X} 184 ${w} ${H}`}>
      {LETTER_PATHS.map((l, i) => (
        <G key={i} transform={`translate(${l.x},${l.y}) scale(${l.sx},${l.sy})`}><Path d={l.d} fill={WHITE} /></G>
      ))}
    </Svg>
  );
}

/**
 * The full PORTICO wordmark, `height` points tall. With `reveal` (0 to 1) the
 * letters fade while the symbol stays put, so a collapsed rail can show the
 * symbol alone and grow into the wordmark.
 */
export function Wordmark({height = 24, reveal}: {height?: number; reveal?: Animated.AnimatedInterpolation<number> | Animated.Value | number}) {
  return (
    <View style={{flexDirection: 'row', alignItems: 'flex-start'}} accessible accessibilityLabel="Portico">
      <BrandMark size={height} />
      <Animated.View style={{opacity: reveal ?? 1}}><Letters height={height} /></Animated.View>
    </View>
  );
}

/** Width of the full wordmark at a given height. */
export const wordmarkWidth = (height: number) => (height * WORD_W) / H;
