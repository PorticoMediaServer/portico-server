import {useEffect, useLayoutEffect, useRef, useState, type CSSProperties, type PointerEvent} from 'react';
import {springTiming, springs} from '@design';
import {Menu} from '../overlays';
import {Control, Identity, Progress, TerminalActions, Transport, useDockActions} from './parts';
import {remoteTerminal} from './model';
import {dockText} from './text';
import {GestureRelease, PlayerMorph} from './motion';
import {signalDockMotion} from './geometry-event';
import {useDockHandover, useDockReducedMotion} from './use-motion';
import type {DockRendererProps} from './types';
import styles from './dock.module.css';

export function MediaBar(props: DockRendererProps) {
  const {view, binding, t = dockText, className, style} = props, root = useRef<HTMLElement | null>(null), anchor = useRef<HTMLButtonElement | null>(null), menuAnchor = useRef<HTMLSpanElement | null>(null);
  const [held, setHeld] = useState(false), [holding, setHolding] = useState(false), reduced = useDockReducedMotion(root), send = useDockActions({view, binding});
  const holdTimer = useRef<ReturnType<typeof setTimeout> | null>(null), animation = useRef<Animation | null>(null), suppressClick = useRef(false);
  const gesture = useRef<{id: number; x: number; y: number; lastX: number; lastY: number; time: number; vx: number; vy: number; dx: number; dy: number; moved: boolean} | null>(null);
  const movement = useRef(new PlayerMorph(0, springs.settle));
  useDockHandover(root, `${view.playback.fence.sessionId}:${view.playback.fence.generation}:${view.mode}`);
  const clearHold = () => { if (holdTimer.current) clearTimeout(holdTimer.current); holdTimer.current = null; setHolding(false); };
  useEffect(() => () => { if (holdTimer.current) clearTimeout(holdTimer.current); animation.current?.cancel(); }, []);
  useLayoutEffect(() => { setHeld(false); }, [view.playback.fence.sessionId, view.playback.fence.generation]);
  useLayoutEffect(() => {
    if (!root.current) return;
    const element = root.current;
    signalDockMotion(element, {kind: 'hold', raised: held, scales: reduced ? [1] : held ? [.98, 1.02, 1] : holding ? [.98] : [1], duration: held && !reduced ? springTiming(springs.snap).durationMs : 0});
    if (!held || reduced) return;
    const job = root.current.animate([{transform: 'scale(.98)'}, {transform: 'scale(1.02)', offset: .3}, {transform: 'scale(1)'}], {duration: springTiming(springs.snap).durationMs, easing: 'ease-out'});
    return () => job.cancel();
  }, [held, holding, reduced]);
  const down = (event: PointerEvent<HTMLElement>) => {
    if (event.button !== 0 || held || remoteTerminal(view.remote)) return;
    const current = animation.current ? movement.current.sample(performance.now()) : {position: 0, velocity: 0};
    animation.current?.cancel(); animation.current = null; event.currentTarget.style.transform = current.position ? `translateX(${current.position}px)` : ''; suppressClick.current = false;
    gesture.current = {id: event.pointerId, x: event.clientX - current.position, y: event.clientY, lastX: event.clientX, lastY: event.clientY, time: event.timeStamp, vx: current.velocity, vy: 0, dx: current.position, dy: 0, moved: current.position !== 0};
    event.currentTarget.setPointerCapture(event.pointerId);
    signalDockMotion(event.currentTarget, {kind: 'begin', offset: current.position});
    holdTimer.current = setTimeout(() => { suppressClick.current = true; setHolding(false); setHeld(true); }, 500); setHolding(true);
  };
  const move = (event: PointerEvent<HTMLElement>) => {
    const g = gesture.current; if (!g || g.id !== event.pointerId || held) return;
    const dt = Math.max(1, event.timeStamp - g.time); g.vx = (event.clientX - g.lastX) * 1000 / dt; g.vy = (event.clientY - g.lastY) * 1000 / dt;
    g.lastX = event.clientX; g.lastY = event.clientY; g.time = event.timeStamp; g.dx = event.clientX - g.x; g.dy = event.clientY - g.y;
    if (Math.hypot(g.dx, g.dy) < 6 && !g.moved) return;
    g.moved = true; suppressClick.current = true; clearHold();
    if (Math.abs(g.dx) >= Math.abs(g.dy)) { event.currentTarget.setPointerCapture(event.pointerId); event.currentTarget.style.transform = `translateX(${g.dx}px)`; signalDockMotion(event.currentTarget, {kind: 'track', offset: g.dx}); }
  };
  const release = (event: PointerEvent<HTMLElement>, cancelled = false) => {
    clearHold(); const g = gesture.current; gesture.current = null; if (!g || g.id !== event.pointerId) return;
    if (!g.moved) { signalDockMotion(event.currentTarget, {kind: 'reset'}); return; }
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
    if (!cancelled && Math.abs(g.dy) > Math.abs(g.dx) && (g.dy < -35 || g.vy < -600)) { send({kind: 'expand'}); return; }
    const vx = event.timeStamp - g.time > 80 ? 0 : g.vx;
    const decision = cancelled ? {dismiss: false, target: 0, velocity: vx} : GestureRelease.dismiss(g.dx, vx, event.currentTarget.getBoundingClientRect().width);
    const element = event.currentTarget, start = performance.now(); movement.current.track(g.dx, decision.velocity, start); movement.current.retarget(decision.target, start, decision.velocity);
    const duration = decision.dismiss ? 200 : 500;
    const offsets = Array.from({length: 34}, (_, i) => reduced ? g.dx : movement.current.sample(start + duration * i / 33).position), opacity = offsets.map((_, i) => decision.dismiss ? 1 - i / 33 : 1);
    const keyframes = offsets.map((offset, i) => reduced ? {opacity: opacity[i]} : {transform: `translateX(${offset}px)`, opacity: opacity[i]});
    const job = element.animate(keyframes, {duration: reduced ? 160 : duration, easing: 'linear', fill: 'forwards'}); animation.current = job;
    signalDockMotion(element, {kind: 'release', offsets, opacity, duration: reduced ? 160 : duration});
    void job.finished.then(() => { if (animation.current !== job) return; if (decision.dismiss) send({kind: 'stop'}); element.style.transform = ''; signalDockMotion(element, {kind: 'reset'}); job.cancel(); animation.current = null; }).catch(() => {});
  };
  return <>
    <span ref={menuAnchor} className={styles.menuAnchor} style={style} aria-hidden="true" />
    {held && <div className={styles.phoneScrim} onClick={() => setHeld(false)} />}
    <section ref={root} className={[styles.phoneBar, held && styles.held, className].filter(Boolean).join(' ')} data-mode={view.mode} data-native-underlay-branch="dock" data-holding={holding || undefined} aria-label={t('next.dock.nowPlaying')} style={style}
      onPointerDown={down} onPointerMove={move} onPointerUp={event => release(event)} onPointerCancel={event => release(event, true)}
      onClickCapture={event => { if (suppressClick.current) { event.preventDefault(); event.stopPropagation(); suppressClick.current = false; } }}
      onContextMenu={event => { event.preventDefault(); clearHold(); setHeld(true); }}>
      <span className={styles.phoneIdentity} data-dock-identity><Identity {...props} identityRef={anchor} phone /></span>
      {remoteTerminal(view.remote) ? <TerminalActions {...props} phone /> : view.remote && view.remote.phase !== 'playing' ? <Control icon="i-stop" label={t('next.dock.stopConnecting')} onClick={() => send({kind: 'stop'})} /> : <Transport {...props} phone />}
      <Progress {...props} line />
    </section>
    <Menu open={held} onOpenChange={open => setHeld(open)} title={t('next.dock.nowPlaying')} presentation="anchored" anchorRef={menuAnchor} returnFocusRef={anchor} placement="top-start" width={250} sideOffset={8} className={styles.holdMenu} items={[
      {id: 'open', label: t('next.dock.open'), icon: 'i-up', onSelect: () => send({kind: 'expand'})},
      ...(view.presentation.controls.includes('play-on') ? [{id: 'play-on', label: t('next.dock.playOn'), icon: 'i-cast' as const, onSelect: () => send({kind: 'play-on'})}] : []),
      {id: 'stop', label: t('next.dock.stop'), icon: 'i-stop', destructive: true, onSelect: () => send({kind: 'stop'})},
    ]} />
  </>;
}
