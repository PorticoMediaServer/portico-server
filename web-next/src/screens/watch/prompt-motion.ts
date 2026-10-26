import {useLayoutEffect, useRef, useState, type CSSProperties} from 'react';
import {springs, springTiming} from '@design';
import type {WebPlaybackRuntimeSnapshot} from '../../platform/playback';
import {fenceKey} from '../../shell/playback-model';
export {useReducedMotion as usePromptReducedMotion} from '../../kit/overlays/shared';

const snap = springTiming(springs.snap);

/** The prompt's canonical snap spring, shared by the three watch prompts. */
export const promptSnapStyle: CSSProperties = {
  '--prompt-snap-ms': `${snap.durationMs}ms`,
  '--prompt-spring': snap.css,
} as unknown as CSSProperties;

/** The authority, queue fence and visibility a painted prompt belongs to. A retained
 * paint never crosses these facts: another source or authority, a collapsed player or
 * a quiet dock replaces it at once rather than animating an offer the owner withdrew. */
type PromptScope = Readonly<{authority: WebPlaybackRuntimeSnapshot['authority']; owner: object; key: string}>;
export function promptScope(snapshot: WebPlaybackRuntimeSnapshot, expanded: boolean, owner: object): PromptScope {
  const authority = snapshot.authority;
  return {authority, owner, key: JSON.stringify([authority, snapshot.fence ? fenceKey(snapshot.fence) : null,
    expanded, snapshot.disposed, snapshot.dock.remote?.phase === 'playing'])};
}
const sameScope = (a: PromptScope, b: PromptScope) => a.authority === b.authority && a.owner === b.owner && a.key === b.key;

export type PromptPhase = 'in' | 'out' | 'idle';

/** Only the prompt's presentation lifetime. The caller keeps its admission, expiry,
 * segment state and handlers; an outgoing paint keeps the exact value it was drawn
 * with and is inert from the first render that withdraws it. */
export function usePromptPresence<T>(value: T | undefined, key: string | undefined, scope: PromptScope, reduced: boolean) {
  type Paint = Readonly<{value: T; key: string; scope: PromptScope}>;
  const last = useRef<Paint | undefined>(undefined), [released, setReleased] = useState<Paint>();
  // Only committed paint can outlive an offer. A speculative render never
  // publishes its callbacks or value into the retained record.
  useLayoutEffect(() => {
    if (key !== undefined && value !== undefined) last.current = {value, key, scope};
    else if (last.current && !sameScope(last.current.scope, scope)) last.current = undefined;
  });
  const held = last.current && last.current !== released && sameScope(last.current.scope, scope) ? last.current : undefined;
  const active = key !== undefined && value !== undefined;
  const present = active ? value : held?.value;
  const phase: PromptPhase = active ? 'in' : present !== undefined ? 'out' : 'idle';
  useLayoutEffect(() => {
    if (active || !held) return;
    const original = held;
    const timer = setTimeout(() => setReleased(original), reduced ? 160 : 120);
    return () => clearTimeout(timer);
  }, [active, reduced, held]);
  useLayoutEffect(() => () => {last.current = undefined;}, []);
  return {present, phase, paintKey: active ? `${scope.key}\u0000${key}` : held ? `${held.scope.key}\u0000${held.key}` : undefined};
}
