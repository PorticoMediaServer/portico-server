import {useEffect, useRef, useState, type CSSProperties} from 'react';
import {springs, springTiming} from '@design';
import type {WebPlaybackRuntimeSnapshot} from '../../platform/playback';
import {fenceKey} from '../../shell/playback-model';

const snap = springTiming(springs.snap);

/** The prompt's canonical snap spring, shared by the three watch prompts. */
export const promptSnapStyle: CSSProperties = {
  '--prompt-snap-ms': `${snap.durationMs}ms`,
  '--prompt-spring': snap.css,
} as unknown as CSSProperties;

/** The authority, queue fence and visibility a painted prompt belongs to. A retained
 * paint never crosses these facts: another source or authority, a collapsed player or
 * a quiet dock replaces it at once rather than animating an offer the owner withdrew. */
export function promptScope(snapshot: WebPlaybackRuntimeSnapshot, expanded: boolean) {
  const authority = snapshot.authority;
  return [authority.serverId, authority.authority, authority.accountId, authority.profileId, String(authority.epoch), authority.viewerFence,
    snapshot.fence ? fenceKey(snapshot.fence) : '', expanded ? 'expanded' : 'collapsed',
    snapshot.dock.remote?.phase === 'playing' ? 'quiet' : 'drawn'].join('|');
}

export type PromptPhase = 'in' | 'out' | 'idle';

/** Only the prompt's presentation lifetime. The caller keeps its admission, expiry,
 * segment state and handlers; an outgoing paint keeps the exact value it was drawn
 * with and is inert from the first render that withdraws it. */
export function usePromptPresence<T>(value: T | undefined, key: string | undefined, scope: string, reduced: boolean) {
  const last = useRef<Readonly<{value: T; key: string; scope: string}> | undefined>(undefined);
  const released = useRef(true), [, rerender] = useState(0);
  if (key !== undefined && value !== undefined) {last.current = {value, key, scope}; released.current = false;}
  else if (last.current && last.current.scope !== scope) {last.current = undefined; released.current = true;}
  const held = !released.current ? last.current : undefined;
  const present = key !== undefined ? value : held?.value;
  const phase: PromptPhase = key !== undefined ? 'in' : present !== undefined ? 'out' : 'idle';
  useEffect(() => {
    if (key !== undefined || !held) return;
    const timer = setTimeout(() => {released.current = true; rerender(count => count + 1);}, reduced ? 160 : 120);
    return () => clearTimeout(timer);
  }, [key, scope, reduced, held]);
  return {present, phase, paintKey: key !== undefined ? `${scope}\u0000${key}` : held ? `${held.scope}\u0000${held.key}` : undefined};
}

/** The app's existing reduce-motion signals, for the exit timer's length. */
export function usePromptReducedMotion() {
  const [reduced, setReduced] = useState(false);
  useEffect(() => {
    const media = window.matchMedia('(prefers-reduced-motion: reduce)');
    const update = () => setReduced(media.matches || document.documentElement.dataset.reduceMotion === 'true' || document.body.dataset.reduceMotion === 'true');
    const observer = new MutationObserver(update);
    observer.observe(document.documentElement, {subtree: true, attributes: true, attributeFilter: ['data-reduce-motion']});
    media.addEventListener('change', update); update();
    return () => {observer.disconnect(); media.removeEventListener('change', update);};
  }, []);
  return reduced;
}
