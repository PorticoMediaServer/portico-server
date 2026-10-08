import { memo } from 'react';
import { Link, useNavigate } from 'react-router';
import { Disc3, Ellipsis, ListEnd, ListStart, Radio, User } from 'lucide-react';
import type { Track } from '../../data/types';
import { usePlayer } from '../../state/player';
import { Menu, toast, useMenu } from '../../components/overlay';
import { cls } from '../../components/ui';
import { fmtClock } from '../../lib/hooks';

export function Equalizer({ playing }: { playing: boolean }) {
  return (
    <span className={cls('eq', playing && 'on')} aria-label="Now playing">
      <i />
      <i />
      <i />
    </span>
  );
}

export const TrackRow = memo(function TrackRow({
  t,
  index,
  list,
  showArt,
  showAlbum,
  showArtist = true,
  numbered = true,
}: {
  t: Track;
  index: number;
  list: Track[];
  showArt?: boolean;
  showAlbum?: boolean;
  showArtist?: boolean;
  numbered?: boolean;
}) {
  const current = usePlayer((s) => s.queue[s.index]?.id === t.id);
  const playing = usePlayer((s) => s.playing);
  const menu = useMenu();
  const navigate = useNavigate();
  const p = usePlayer.getState();
  const play = () => {
    if (current) p.toggle();
    else p.playTracks(list, index);
  };
  return (
    <div className={cls('trow', current && 'current', showArt && 'with-art', showAlbum && 'with-album')} onDoubleClick={play}>
      <button className="trow-num tnum" onClick={play} aria-label={current && playing ? `Pause ${t.title}` : `Play ${t.title}`}>
        {current ? <Equalizer playing={playing} /> : showArt ? <img src={t.art.square} alt="" loading="lazy" /> : numbered ? t.number : ''}
        {!current && <span className="trow-play">▶</span>}
      </button>
      <div className="trow-main" onClick={play}>
        <span className="trow-title ellipsis">
          {t.title}
          {t.explicit && <span className="explicit sm">E</span>}
        </span>
        {showArtist && (
          <Link className="trow-artist ellipsis" to={`/artist/${t.artistId}`} onClick={(e) => e.stopPropagation()} viewTransition>
            {t.artistName}
          </Link>
        )}
      </div>
      {showAlbum && (
        <Link className="trow-album ellipsis" to={`/album/${t.albumId}`} viewTransition>
          {t.albumTitle}
        </Link>
      )}
      <span className="trow-dur tnum">{fmtClock(t.duration)}</span>
      <button className="icon-btn sm plain trow-more" aria-label={`More for ${t.title}`} onClick={menu.open}>
        <Ellipsis />
      </button>
      <Menu
        anchor={menu.anchor}
        open={menu.isOpen}
        onClose={menu.close}
        title={t.title}
        items={[
          { label: 'Play Next', icon: <ListStart />, onSelect: () => (p.enqueue([t], true), toast(`“${t.title}” will play next`, <ListStart />)) },
          { label: 'Add to Queue', icon: <ListEnd />, onSelect: () => (p.enqueue([t]), toast(`Added “${t.title}” to the queue`, <ListEnd />)) },
          { label: 'Start Station', icon: <Radio />, onSelect: () => toast(`Starting a station from “${t.title}”`, <Radio />) },
          { separator: true },
          { label: 'Go to Album', icon: <Disc3 />, onSelect: () => navigate(`/album/${t.albumId}`) },
          { label: 'Go to Artist', icon: <User />, onSelect: () => navigate(`/artist/${t.artistId}`) },
        ]}
      />
    </div>
  );
});

export function Mosaic({ images }: { images: string[] }) {
  const uniq = Array.from(new Set(images)).slice(0, 4);
  if (uniq.length < 4) return <img className="mosaic-one" src={uniq[0]} alt="" />;
  return (
    <span className="mosaic">
      {uniq.map((s) => (
        <img key={s} src={s} alt="" loading="lazy" />
      ))}
    </span>
  );
}
