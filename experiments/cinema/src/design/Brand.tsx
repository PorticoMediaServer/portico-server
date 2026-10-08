import React from 'react';
import {Image} from 'react-native';

// Portico's official brand artwork (assets/brand/portico, from the Portico brand set). These are
// the "dark" variants: white artwork for dark surfaces. Per the brand rules they are never
// recoloured, stretched or given effects, so these components only take a height.
const SYMBOL = require('../../assets/brand/portico/png/portico-symbol-dark-512.png');
const WORDMARK = require('../../assets/brand/portico/png/portico-wordmark-dark-2048w.png');
const SYMBOL_RATIO = 384 / 512;
const WORDMARK_RATIO = 2048 / 468;

/** The Play-P symbol, `size` tall. */
export function BrandMark({size = 28}: {size?: number}) {
  return <Image source={SYMBOL} style={{width: Math.round(size * SYMBOL_RATIO), height: size}} resizeMode="contain" accessibilityLabel="Portico" />;
}

/** The full Portico wordmark, `size` tall. */
export function Wordmark({size = 28}: {size?: number}) {
  return <Image source={WORDMARK} style={{width: Math.round(size * WORDMARK_RATIO), height: size}} resizeMode="contain" accessibilityLabel="Portico" />;
}
