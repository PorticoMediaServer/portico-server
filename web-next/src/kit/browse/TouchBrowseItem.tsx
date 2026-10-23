import type {HTMLAttributes} from 'react';
import {SelectionHoldFeedback} from '../selection/SelectionHoldFeedback';
import {touchMenuHaptic} from './touch-haptics';
import {useTouchHold} from './useTouchHold';

export type BrowseTouchSelection = Readonly<{active: boolean; onToggle: (id: string) => void}>;

/** Row/track counterparts of CardFrame, with caller-owned identity and selection. */
export function TouchBrowseItem({itemId, onMore, selectionState, moreExpanded = false, ...props}: HTMLAttributes<HTMLDivElement> & {
  itemId: string; onMore?: (id: string, anchor: HTMLElement) => void; selectionState?: BrowseTouchSelection; moreExpanded?: boolean;
}) {
  const touch = useTouchHold(anchor => {
    if (selectionState?.active) selectionState.onToggle(itemId);
    else if (onMore) {anchor.dataset.touchMenu = 'true'; touchMenuHaptic(); onMore(itemId, anchor);}
  }, !!onMore || !!selectionState?.active);
  return <SelectionHoldFeedback active={touch.phase === 'pending' || !selectionState?.active && (touch.phase === 'fired' || moreExpanded)} progress={touch.phase === 'fired' || moreExpanded ? 1 : 0}><div {...props} data-track-item={itemId}
    onPointerDown={event => {
      props.onPointerDown?.(event);
      const control = event.target instanceof Element ? event.target.closest('button,a,input,select,textarea') : null;
      if (!event.defaultPrevented && (!control || control.hasAttribute('data-track-primary'))) touch.handlers.onPointerDown(event);
    }}
    onPointerMove={event => {props.onPointerMove?.(event); touch.handlers.onPointerMove(event);}}
    onPointerUp={event => {props.onPointerUp?.(event); touch.handlers.onPointerUp();}}
    onPointerCancel={event => {props.onPointerCancel?.(event); touch.handlers.onPointerCancel();}}
    onPointerLeave={event => {props.onPointerLeave?.(event); touch.handlers.onPointerLeave();}}
    onContextMenu={event => {props.onContextMenu?.(event); if (touch.phase !== 'idle') event.preventDefault();}}
    onClickCapture={event => {
      props.onClickCapture?.(event);
      if (event.defaultPrevented) return;
      if (touch.consumeClick() && event.detail !== 0) {event.preventDefault(); event.stopPropagation(); return;}
      if (selectionState?.active && event.target instanceof Element && event.target.closest('[data-track-primary]')) {
        event.preventDefault(); event.stopPropagation(); selectionState.onToggle(itemId);
      }
    }}/></SelectionHoldFeedback>;
}
