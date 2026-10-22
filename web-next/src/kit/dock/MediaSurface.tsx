import {useLayoutEffect, useRef} from 'react';
import {springTiming, springs} from '@design';
import {Icon} from '../Icon';
import {PlayerMorph} from './motion';
import {dockMotionEvent, type DockMotionDetail} from './geometry-event';
import {useDockReducedMotion} from './use-motion';
import type {MediaSurfaceProps} from './types';
import styles from './dock.module.css';

type Frame = {left: number; top: number; width: number; height: number};
const dimensions = ['left', 'top', 'width', 'height'] as const;

/** Mount the owner's one real video element here for its entire life. Never portals or clones. */
export function MediaSurface({children, expanded, dockSlot, expandedSlot, hidden = false, className}: MediaSurfaceProps) {
  const ref = useRef<HTMLDivElement | null>(null), reduced = useDockReducedMotion(ref);
  const animation = useRef<Animation | null>(null), first = useRef(true), epoch = useRef(0);
  const physics = useRef<Partial<Record<keyof Frame, PlayerMorph>>>({});
  useLayoutEffect(() => {
    const element = ref.current; if (!element) return;
    const apply = (frame: Frame) => { element.style.width = `${frame.width}px`; element.style.height = `${frame.height}px`; element.style.transform = `translate(${frame.left}px, ${frame.top}px)`; };
    const measure = () => {
      const slot = (expanded ? expandedSlot : dockSlot).current;
      if (!slot) { element.style.visibility = 'hidden'; return; }
      const target = slot.getBoundingClientRect(); if (!target.width || !target.height) { element.style.visibility = 'hidden'; return; }
      const token = ++epoch.current, now = performance.now(), current = element.getBoundingClientRect();
      const velocity = Object.fromEntries(dimensions.map(key => [key, physics.current[key]?.sample(now).velocity ?? 0])) as Record<keyof Frame, number>;
      animation.current?.cancel(); animation.current = null; element.style.visibility = hidden ? 'hidden' : 'visible';
      if (first.current || hidden) { first.current = false; apply(target); dimensions.forEach(key => { physics.current[key] = new PlayerMorph(target[key]); }); return; }
      const origin: Frame = {left: current.left, top: current.top, width: current.width, height: current.height};
      dimensions.forEach(key => { const spring = physics.current[key] ?? new PlayerMorph(origin[key]); spring.track(origin[key], velocity[key], now); spring.retarget(target[key], now, velocity[key]); physics.current[key] = spring; });
      apply(reduced ? target : origin);
      const duration = reduced ? 160 : springTiming(springs.player).durationMs;
      const keyframes: Keyframe[] = reduced ? [{opacity: 0}, {opacity: 1}] : Array.from({length: 49}, (_, i) => {
        const time = now + duration * i / 48, frame = Object.fromEntries(dimensions.map(key => [key, i === 48 ? target[key] : physics.current[key]!.sample(time).position])) as Frame;
        return {transform: `translate(${frame.left}px, ${frame.top}px) scale(${frame.width / Math.max(1, origin.width)}, ${frame.height / Math.max(1, origin.height)})`, opacity: 1};
      });
      const job = element.animate(keyframes, {duration, easing: 'linear', fill: 'forwards'}); animation.current = job;
      void job.finished.then(() => { if (token !== epoch.current) return; apply(target); job.cancel(); animation.current = null; }).catch(() => {});
    };
    measure();
    let drag: {frame: Frame; offset: number} | null = null;
    const follow = (event: Event) => {
      const slot = dockSlot.current;
      if (expanded || hidden || !slot || !(event.target instanceof HTMLElement) || !event.target.contains(slot)) return;
      const {detail} = event as CustomEvent<DockMotionDetail>;
      if (detail.kind === 'begin') {
        const frame = element.getBoundingClientRect(); animation.current?.cancel(); animation.current = null; epoch.current++;
        drag = {frame, offset: detail.offset}; apply(frame);
      } else if (detail.kind === 'track' && drag) element.style.transform = `translate(${drag.frame.left + detail.offset - drag.offset}px, ${drag.frame.top}px)`;
      else if (detail.kind === 'release' && drag) {
        const origin = drag.frame, initial = drag.offset;
        animation.current?.cancel();
        const frames = detail.offsets.map((offset, i) => ({transform: `translate(${origin.left + offset - initial}px, ${origin.top}px)`, opacity: detail.opacity[i]}));
        const job = element.animate(frames, {duration: detail.duration, easing: 'linear', fill: 'forwards'}); animation.current = job;
        void job.finished.then(() => { if (!dockSlot.current?.isConnected) element.style.visibility = 'hidden'; }).catch(() => {});
      } else if (detail.kind === 'reset') { drag = null; measure(); }
    };
    document.addEventListener(dockMotionEvent, follow);
    const identity = dockSlot.current?.parentElement;
    const enter = () => { element.dataset.hover = 'true'; }, leave = () => { delete element.dataset.hover; };
    identity?.addEventListener('pointerenter', enter); identity?.addEventListener('pointerleave', leave); identity?.addEventListener('focusin', enter); identity?.addEventListener('focusout', leave);
    const observer = new ResizeObserver(measure); if (expandedSlot.current) observer.observe(expandedSlot.current); if (dockSlot.current) observer.observe(dockSlot.current);
    window.addEventListener('resize', measure); window.addEventListener('scroll', measure, true);
    return () => { observer.disconnect(); document.removeEventListener(dockMotionEvent, follow); window.removeEventListener('resize', measure); window.removeEventListener('scroll', measure, true); identity?.removeEventListener('pointerenter', enter); identity?.removeEventListener('pointerleave', leave); identity?.removeEventListener('focusin', enter); identity?.removeEventListener('focusout', leave); };
  }, [expanded, reduced, hidden, dockSlot, expandedSlot]);
  useLayoutEffect(() => () => { epoch.current++; animation.current?.cancel(); }, []);
  return <div ref={ref} className={[styles.mediaSurface, className].filter(Boolean).join(' ')} data-docked={!expanded || undefined} aria-hidden={!expanded || hidden || undefined}><div className={styles.surfacePicture}>{children}{!expanded && <span className={styles.surfaceExpand}><Icon name="i-zoom" size={22} /></span>}</div></div>;
}
