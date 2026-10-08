import {useSyncExternalStore} from 'react';
import type {EventsClient} from '@core/playback-v1/events.ts';

/**
 * MU2: this device's one event feed, reachable from anywhere. The player
 * provider publishes it (`player/engine.tsx`); the shell's sidebar reads it
 * through `app/library-scan.ts`. Never create a second `EventsClient`.
 */
let current: EventsClient | null = null;
const listeners = new Set<() => void>();

export function setDeviceEvents(events: EventsClient | null): void {
  if (current === events) return;
  current = events;
  for (const listener of [...listeners]) listener();
}

/** The feed for the cleanup guard in `player/engine.tsx`. */
export function getDeviceEvents(): EventsClient | null {
  return current;
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

function getSnapshot(): EventsClient | null {
  return current;
}

export function useDeviceEvents(): EventsClient | null {
  return useSyncExternalStore(subscribe, getSnapshot);
}
