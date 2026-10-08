import {useCallback, useState} from 'react';
import type {LayoutChangeEvent} from 'react-native';
import {useMetrics} from '../theme/layout';

/** Width of the content area, measured, with a sensible first guess. */
export function useContentWidth(): [number, (e: LayoutChangeEvent) => void] {
  const m = useMetrics();
  const guess = m.isTV ? m.width - m.rail.collapsed : m.isTablet ? m.width - m.rail.expanded : m.width;
  const [w, setW] = useState(guess);
  const onLayout = useCallback((e: LayoutChangeEvent) => setW(e.nativeEvent.layout.width), []);
  return [w, onLayout];
}
