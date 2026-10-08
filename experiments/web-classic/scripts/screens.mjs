/** Every reviewable screen and state (src/app/screens.json), with extra waits for timed states. */
import {readFileSync} from 'node:fs';

const WAITS = {
  'player-upnext': 5200, 'player-finished': 3400, 'player-intro': 2200, 'player-movie': 2000, 'player-options': 2000, 'player-queue': 2200, 'player-playon': 2600,
  'player-live': 2200, 'player-miniguide': 2400, 'player-channel-problem': 2400, 'now-playing': 1800, 'mini-player': 1600, 'audiobook-player': 1800,
  'activate': 1500, 'restore': 1200, 'together-room': 1800, 'selection': 1500, 'state-kids': 1600, 'photo-viewer': 1400,
};
export const SCREENS = JSON.parse(readFileSync(new URL('../src/app/screens.json', import.meta.url), 'utf8')).map(s => ({...s, wait: WAITS[s.id]}));
