import {useMemo} from 'react';
import {useWindowDimensions} from 'react-native';
import {computeLayout, type Layout} from './tokens';

export function useLayout(): Layout {
  const {width, height} = useWindowDimensions();
  return useMemo(() => computeLayout(width, height), [width, height]);
}
