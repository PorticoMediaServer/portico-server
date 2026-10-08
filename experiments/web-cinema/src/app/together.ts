import {useSyncExternalStore} from 'react';

/** Which Watch Together group this viewer is in (drives the rail badge and the player's group pill). */
let groupId: string | null = null;
const listeners = new Set<() => void>();
const emit = () => listeners.forEach(l => l());

export const together = {
  join(id: string) { groupId = id; emit(); },
  leave() { groupId = null; emit(); },
  get: () => groupId,
};

export function useTogether() {
  const id = useSyncExternalStore(l => { listeners.add(l); return () => listeners.delete(l); }, () => groupId);
  return {groupId: id, inGroup: !!id};
}
