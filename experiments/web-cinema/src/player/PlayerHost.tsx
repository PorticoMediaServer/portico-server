import {usePlayer} from '../app/player';
import {VideoPlayer} from './VideoPlayer';
import {NowPlaying} from './NowPlaying';

/** Mounts the full-screen player for what's playing (the mini player lives in the shell). */
export function PlayerHost() {
  const s = usePlayer(x => ({active: x.active, kind: x.kind, minimized: x.minimized}), true);
  if (!s.active || s.minimized) return null;
  return s.kind === 'video' || s.kind === 'live' ? <VideoPlayer /> : <NowPlaying />;
}
