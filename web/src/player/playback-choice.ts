import {useCallback, useSyncExternalStore} from 'react';
import {playerSessionChoices} from '@core/player-session-choices.ts';

/** A choice made in the player, for this playback only (`playbackRun`): the next episode keeps
 * it, a new playback starts from `settings` again. Nothing is stored on the device. */
export function usePlaybackChoice<T>(name: string, run: number, settings: T): readonly [T, (next: (now: T) => T) => void] {
  useSyncExternalStore(playerSessionChoices.subscribe, playerSessionChoices.getVersion);
  const value = playerSessionChoices.get(name, run, settings);
  const set = useCallback((next: (now: T) => T) => {
    playerSessionChoices.set(name, run, next(playerSessionChoices.get(name, run, settings)));
  }, [name, run, settings]);
  return [value, set] as const;
}
