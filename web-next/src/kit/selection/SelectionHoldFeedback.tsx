import {cloneElement, useLayoutEffect, useRef, type CSSProperties, type ReactElement, type Ref} from 'react';
import {motion, easings, springs} from '@design';
import {springFrames} from '../overlays/model';
import {useReducedMotion} from '../overlays/shared';
import styles from './selection.module.css';

export type SelectionHoldFeedbackProps = Readonly<{
  active: boolean;
  progress: number | null;
  className?: string;
  style?: CSSProperties;
  children?: ReactElement<{className?: string; style?: CSSProperties; ref?: Ref<HTMLElement>}>;
}>;

/** A renderer for the card's actual hold state; it owns no gesture, timer, haptic or selection. */
export function SelectionHoldFeedback({active, progress, className, style, children}: SelectionHoldFeedbackProps) {
  // Q6/tablet9 and motion2.16: only the item sinks/lifts; there is no selection hint.
  const element = useRef<HTMLElement | null>(null), previous = useRef(1);
  const reduce = useReducedMotion(element), target = active ? progress === 1 ? 1.05 : .98 : 1;
  useLayoutEffect(() => {
    const node = element.current;
    if (!node || reduce || !node.animate) {previous.current = target; return;}
    const frames = target === 1.05 ? springFrames(previous.current, target, 0, springs.snap) : [previous.current, target];
    const animation = node.animate(frames.map(scale => ({transform: `scale(${scale})`})), {duration: target === 1.05 ? 500 : motion.quick, easing: target === 1.05 ? 'linear' : easings.out});
    previous.current = target;
    return () => {previous.current = new DOMMatrixReadOnly(getComputedStyle(node).transform).a; animation.cancel();};
  }, [target, reduce]);
  if (!children) return null;
  return cloneElement(children, {className: [children.props.className, styles.holdFeedback, className].filter(Boolean).join(' '),
    ref: node => {element.current = node; const ref = children.props.ref; if (typeof ref === 'function') return ref(node); if (ref) ref.current = node;},
    style: {...children.props.style, ...style, '--hold-scale': target} as CSSProperties});
}
