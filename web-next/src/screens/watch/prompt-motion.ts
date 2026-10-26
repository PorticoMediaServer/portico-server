import {useEffect, useRef, useState, type CSSProperties} from 'react';
import {springs, springTiming} from '@design';

const snap = springTiming(springs.snap);

/** The prompt's canonical snap spring, shared by the three watch prompts. */
export const promptSnapStyle: CSSProperties = {
  '--prompt-snap-ms': `${snap.durationMs}ms`,
  '--prompt-spring': snap.css,
} as unknown as CSSProperties;

/** Only the prompt's presentation lifetime. The caller keeps its offer, expiry,
 * segment state and handlers; an outgoing paint retains its original value. */
export function usePromptPresence<T>(value: T | undefined, key: string | undefined, reduced: boolean) {
  const [present, setPresent] = useState<T>(), [phase, setPhase] = useState<'in' | 'out' | 'idle'>('idle');
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined), held = useRef(present), latest = useRef(value);
  held.current = present; latest.current = value;
  useEffect(() => {
    if (timer.current !== undefined) {clearTimeout(timer.current); timer.current = undefined;}
    if (key) {setPresent(latest.current); setPhase('in'); return;}
    if (!held.current) return;
    setPhase('out');
    timer.current = setTimeout(() => {setPresent(undefined); setPhase('idle'); timer.current = undefined;}, reduced ? 160 : 120);
    return () => {if (timer.current !== undefined) {clearTimeout(timer.current); timer.current = undefined;}};
  }, [key, reduced]);
  useEffect(() => () => {if (timer.current !== undefined) clearTimeout(timer.current);}, []);
  return {present: key ? value : present, phase};
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
