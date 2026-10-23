import {defineWebScreen} from '../../route-registry';
import {WatchPlayback} from './WatchPlayback';

/** The persistent host renders the live player across routes; the outlet owns no media. */
export function renderWatch() { return null; }

export default defineWebScreen({id: 'player', chrome: 'player', component: renderWatch, playbackHost: WatchPlayback});
