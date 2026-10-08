import { useRef } from 'react';
import { Cast, ChevronUp, FastForward, ListMusic, MicVocal, Pause, Play, Rewind, SkipBack, SkipForward, Volume2, VolumeX } from 'lucide-react';
import { usePlayer } from '../../state/player';
import { useUi } from '../../state/ui';
import { useIsMobile } from '../../lib/hooks';
import { useRefraction, useSheen } from '../../lib/refraction';
import { IconButton, cls } from '../../components/ui';

export function MiniPlayer() {
  const mode = usePlayer((s) => s.mode);
  const item = usePlayer((s) => s.queue[s.index]);
  const playing = usePlayer((s) => s.playing);
  const buffering = usePlayer((s) => s.buffering);
  const position = usePlayer((s) => s.position);
  const volume = usePlayer((s) => s.volume);
  const muted = usePlayer((s) => s.muted);
  const castTo = usePlayer((s) => s.castTo);
  const p = usePlayer.getState();
  const open = usePlayer((s) => s.openNowPlaying);
  const setPanel = usePlayer((s) => s.setPanel);
  const setCast = useUi((s) => s.setCast);
  const mobile = useIsMobile();
  const ref = useRef<HTMLDivElement>(null);
  useRefraction(ref, { scale: 60, radius: mobile ? 24 : 32 });
  useSheen(ref);
  if (!mode || !item) return null;
  const book = mode === 'audiobook';
  const pct = item.duration ? (position / item.duration) * 100 : 0;
  const PlayIcon = playing ? Pause : Play;

  return (
    <div ref={ref} className={cls('mini glass glass-chrome refract', mobile && 'is-mobile')} role="region" aria-label="Now playing">
      <button className="mini-main" onClick={() => open(true)} aria-label="Open Now Playing">
        <img className={cls('mini-art', book && 'book')} src={item.image} alt="" />
        <span className="mini-text">
          <span className="mini-title ellipsis">{item.title}</span>
          <span className="mini-sub ellipsis">
            {castTo ? (
              <>
                <Cast className="inline-ic" /> Playing on {castLabel(castTo)}
              </>
            ) : book ? (
              `${item.album} · ${item.subtitle}`
            ) : (
              item.subtitle
            )}
          </span>
        </span>
      </button>
      <div className="mini-controls">
        {!mobile && (book ? (
          <IconButton label="Back 15 seconds" variant="plain" onClick={() => p.skip(-15)}>
            <Rewind />
          </IconButton>
        ) : (
          <IconButton label="Previous" variant="plain" onClick={() => p.prev()}>
            <SkipBack fill="currentColor" />
          </IconButton>
        ))}
        <IconButton label={playing ? 'Pause' : 'Play'} variant="plain" className="mini-play" onClick={() => p.toggle()}>
          {buffering && playing ? <span className="spinner" /> : <PlayIcon fill="currentColor" />}
        </IconButton>
        {book ? (
          <IconButton label="Forward 30 seconds" variant="plain" onClick={() => p.skip(30)}>
            <FastForward />
          </IconButton>
        ) : (
          <IconButton label="Next" variant="plain" onClick={() => p.next()}>
            <SkipForward fill="currentColor" />
          </IconButton>
        )}
      </div>
      {!mobile && (
        <div className="mini-extra">
          {!book && (
            <IconButton
              label="Lyrics"
              variant="plain"
              size="sm"
              disabled={!item.lyrics}
              onClick={() => {
                setPanel('lyrics');
                open(true);
              }}
            >
              <MicVocal />
            </IconButton>
          )}
          <IconButton
            label={book ? 'Chapters' : 'Up Next'}
            variant="plain"
            size="sm"
            onClick={() => {
              setPanel('queue');
              open(true);
            }}
          >
            <ListMusic />
          </IconButton>
          <div className="mini-vol">
            <IconButton label={muted ? 'Unmute' : 'Mute'} variant="plain" size="sm" onClick={() => p.toggleMute()}>
              {muted || volume === 0 ? <VolumeX /> : <Volume2 />}
            </IconButton>
            <input
              type="range"
              className="range"
              min={0}
              max={1}
              step={0.01}
              value={muted ? 0 : volume}
              style={{ ['--pct' as string]: `${(muted ? 0 : volume) * 100}%` }}
              onChange={(e) => p.setVolume(Number(e.target.value))}
              aria-label="Volume"
            />
          </div>
          <IconButton label="Play on another device" variant="plain" size="sm" active={!!castTo} onClick={() => setCast(true)}>
            <Cast />
          </IconButton>
          <IconButton label="Expand" variant="plain" size="sm" onClick={() => open(true)}>
            <ChevronUp />
          </IconButton>
        </div>
      )}
      <div className="mini-progress" aria-hidden="true">
        <i style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}

export const castLabel = (id: string) =>
  ({ 'cast-atv': 'Living Room', 'cast-bedroom': 'Bedroom TV', 'cast-kitchen': 'Kitchen', 'cast-downstairs': 'Downstairs', 'cast-projector': 'Garage Projector' })[id] ?? 'another device';
