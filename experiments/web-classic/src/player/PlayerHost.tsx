import {usePlayerState} from './PlayerProvider';
import {VideoPlayer} from './VideoPlayer';
import {MiniPlayer, NowPlaying} from './AudioPlayer';

/** Chooses the surface: the video/live player, or music and audiobooks as a mini bar that expands to Now playing. */
export default function PlayerHost() {
  const st = usePlayerState();
  if (st.mode === 'video' || st.mode === 'live') return st.expanded ? <VideoPlayer /> : null;
  if (st.mode === 'music' || st.mode === 'book') return st.expanded ? <NowPlaying /> : <MiniPlayer />;
  return null;
}
