import {useCallback, useLayoutEffect, useMemo, useRef, useState, type RefObject} from 'react';
import {Artwork} from '../../../kit/artwork';
import type {ArtworkSource} from '../../../kit/artwork';
import {useDockReducedMotion} from '../../../kit/dock/use-motion';
import type {WebPlaybackRuntimeSnapshot} from '../../../platform/playback';
import {fenceKey} from '../../../shell/playback-model';
import type {WatchBindings} from '../types';
import {MusicExpansionPaint} from './expansion-paint';
import styles from './music-expansion-owner.module.css';

let lifetime = 0;
type Scope = Readonly<{id: number; binding: WatchBindings; artwork?: ArtworkSource; current: () => boolean}>;
/** Retains only current music paint. No snapshot, media, queue, command or loading deadline is retained. */
export function useMusicExpansion(binding: WatchBindings | undefined, snapshot: WebPlaybackRuntimeSnapshot | undefined, localCurrent: () => boolean,
  dock: RefObject<HTMLSpanElement | null>, bar: RefObject<HTMLDivElement | null>, remotePlaying: boolean) {
  const fence = snapshot?.fence, presentation = snapshot?.presentation;
  const music = !!binding && !!fence && !snapshot?.disposed && presentation?.kind === 'music' && presentation.subject.kind === 'music'
    && fenceKey(presentation.fence) === fenceKey(fence) && !remotePlaying;
  const key = music ? JSON.stringify([fenceKey(fence!), presentation!.subject.kind === 'music' ? presentation!.subject.item.id : null,
    snapshot!.playback.intentId, snapshot!.playback.playbackRun, presentation!.posterUrl, presentation!.backdropUrl]) : undefined;
  const authority = snapshot?.authority;
  const scope = useMemo<Scope | undefined>(() => {
    if (!music || !binding || !key || !fence || !localCurrent()) return;
    const original = key, runtime = binding.runtime;
    return {id: ++lifetime, binding, artwork: binding.artwork.foreground(presentation!.posterUrl ?? presentation!.backdropUrl, 560, 1, {group: 'playback', priority: 'high'}) ?? undefined, current: () => {
      try {
        if (!localCurrent()) return false;
        const live = runtime.getSnapshot(), value = live.presentation;
        return !live.disposed && live.authority === authority && !!live.fence && value?.kind === 'music' && value.subject.kind === 'music' && fenceKey(value.fence) === fenceKey(live.fence)
          && live.dock.remote?.phase !== 'playing' && JSON.stringify([fenceKey(live.fence), value.subject.item.id, live.playback.intentId, live.playback.playbackRun, value.posterUrl, value.backdropUrl]) === original;
      } catch {return false;}
    }};
  }, [binding, key, authority, localCurrent, music]);
  const [held, setHeld] = useState<Scope>();
  const admitted = scope?.current() ? scope : undefined;
  const open = !!admitted && !!snapshot?.dock.local?.expanded;
  useLayoutEffect(() => {if (open && scope) setHeld(scope);else if (held && held !== scope) setHeld(undefined);}, [open, scope, held]);
  const frame = useRef<HTMLElement | null>(null), cover = useRef<HTMLDivElement | null>(null);
  const settled = useCallback((original: Scope) => setHeld(value => value === original ? undefined : value), []);
  return {scope: admitted, open, present: !!admitted && (open || held === admitted), frame, cover, dock, bar, settled};
}
export type MusicExpansion = ReturnType<typeof useMusicExpansion>;

/** One persistent original Artwork instance moves between the two real committed slots.
 * The parent's sole real media element and all action bindings remain with their existing owner. */
export function MusicExpansionSurface({motion}: {motion: MusicExpansion}) {
  const scope = motion.scope;
  return scope ? <OwnedMusicCover key={scope.id} scope={scope} motion={motion}/> : null;
}
function OwnedMusicCover({scope, motion}: {scope: Scope; motion: MusicExpansion}) {
  const surface = useRef<HTMLDivElement | null>(null), reduced = useDockReducedMotion(surface);
  const owner = useRef<MusicExpansionPaint | null>(null), refresh = useRef<() => void>(() => {});
  const source = scope.artwork;
  useLayoutEffect(() => {
    const original = new MusicExpansionPaint(scope.current, () => motion.settled(scope));owner.current = original;
    let expanded = scope.binding.runtime.getSnapshot().dock.local?.expanded;
    const off = scope.binding.runtime.subscribe(() => {
      if (!scope.current()) {original.dispose();return;}
      const next = scope.binding.runtime.getSnapshot().dock.local?.expanded;
      if (next !== expanded) {expanded = next;original.freeze();}
    });
    return () => {off();if (owner.current === original) owner.current = null;original.dispose();};
  }, [scope, motion.settled]);
  useLayoutEffect(() => {
    const original = owner.current, element = surface.current, dock = motion.dock.current;
    if (!original || !element || !dock || !scope.current()) return;
    const paint = () => {
      if (owner.current !== original || !scope.current() || surface.current !== element || motion.dock.current !== dock) return;
      const frame = motion.frame.current;
      original.update({surface: element, dock, frame, cover: motion.cover.current,
        copies: [...(motion.bar.current?.querySelectorAll<HTMLElement>('[data-dock-identity],[data-dock-expansion-copy]') ?? [])],
        groups: [...(frame?.querySelectorAll<HTMLElement>('[data-music-content-group]') ?? [])]}, motion.open,
        reduced || window.matchMedia('(prefers-reduced-motion: reduce)').matches || !!element.closest('[data-reduce-motion="true"]')
        || document.documentElement.dataset.reduceMotion === 'true' || document.body.dataset.reduceMotion === 'true');
    };
    refresh.current = paint;paint();
    const observer = new ResizeObserver(paint);observer.observe(dock);if (motion.cover.current) observer.observe(motion.cover.current);
    window.addEventListener('resize', paint);window.addEventListener('scroll', paint, true);
    return () => {if (refresh.current === paint) refresh.current = () => {};observer.disconnect();window.removeEventListener('resize', paint);window.removeEventListener('scroll', paint, true);};
  }, [scope, motion.open, motion.present, reduced, motion.dock, motion.cover, motion.frame, motion.bar]);
  return <div ref={surface} className={styles.cover} aria-hidden="true" data-music-cover-surface>
    <Artwork source={source} alt="" onReady={() => refresh.current()}/>
  </div>;
}
