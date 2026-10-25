import {useLayoutEffect, useState} from 'react';

export type OwnedResource<T> = Readonly<{value: T; id: number}>;
/** `live` is the one intentionally mutable field: retirement happens in place
 * in the cleanup, so the tuple it publishes keeps its identity. */
type Held<T> = {readonly value: T; readonly deps: readonly unknown[]; readonly id: number; live: boolean};
let nextOwnedId = 1;

const sameDeps = (a: readonly unknown[], b: readonly unknown[]) => a.length === b.length && a.every((value, index) => Object.is(value, b[index]));

/** One resource per dependency set, created in a layout pass, published through
 * state and retired synchronously by that same pass. Only a live resource whose
 * original dependency tuple is still the current one is handed to the render,
 * so a new API/scope never receives the previous reader, and a replayed mount
 * gets a fresh resource rather than a disposed one. The cleanup retires the
 * exact value it created (live first, then dispose) and clears only that one. */
export function useOwnedResource<T>(
  create: () => T, dispose: (value: T) => void, deps: readonly unknown[], onMount?: (value: T) => void,
): OwnedResource<T> | undefined {
  const [held, setHeld] = useState<Held<T> | null>(null);
  useLayoutEffect(() => {
    const entry: Held<T> = {value: create(), deps: [...deps], live: true, id: nextOwnedId++};
    setHeld(entry);
    onMount?.(entry.value);
    return () => {
      entry.live = false;
      dispose(entry.value);
      setHeld(current => current === entry ? null : current);
    };
    // The dependency list is the resource's identity; it is passed through by design.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
  return held && held.live && sameDeps(held.deps, deps) ? {value: held.value, id: held.id} : undefined;
}
