import {defineWebScreen} from '../../route-registry';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {WatchPlayback} from './WatchPlayback';
import type {WatchBindings} from './types';

/** The persistent host renders the live player across routes; the outlet owns no media. */
export function renderWatch() { return null; }
function RegisteredWatchPlayback({binding}: {binding: WatchBindings | undefined}) {
  const captured = useProtectedWebScreenContext();
  return <WatchPlayback binding={binding} context={captured?.binding.playback?.runtime === binding?.runtime ? captured?.context : undefined} />;
}

export default defineWebScreen({id: 'player', chrome: 'player', component: renderWatch, playbackHost: RegisteredWatchPlayback});
