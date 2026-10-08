import {useCallback} from 'react';
import type {Entry} from '../data/types';
import {useActions} from './actions';
import {useSelection} from './selection';
import {useGlow} from './glow';
import {isTV} from '../theme/layout';

/** Press opens (or toggles while selecting); long press opens the actions menu. */
export function useCardHandlers(origin?: 'continue' | 'recommendation') {
  const actions = useActions();
  const sel = useSelection();
  const glow = useGlow();
  const onPress = useCallback((e: Entry) => (sel.active ? sel.toggle(e) : actions.open(e)), [sel, actions]);
  const onLongPress = useCallback((e: Entry) => (sel.active ? sel.toggle(e) : actions.more(e, origin ?? (e.recommendation ? 'recommendation' : undefined))), [sel, actions, origin]);
  const onFocus = useCallback((e: Entry) => { if (isTV) glow(e.parentId && e.kind === 'episode' ? e.parentId : e.id); }, [glow]);
  return {onPress, onLongPress, onFocus};
}
