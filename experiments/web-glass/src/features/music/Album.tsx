import { useEffect } from 'react';
import { Link, useParams, useSearchParams } from 'react-router';
import { Download, Heart, Play, Share2, Shuffle } from 'lucide-react';
import { useAlbum } from '../../services/queries';
import { usePlayer } from '../../state/player';
import { FloatBar } from '../../components/Detail';
import { MediaCard } from '../../components/media';
import { Button, ErrorState, IconButton, Skel } from '../../components/ui';
import { toast } from '../../components/overlay';
import { fmtDuration } from '../../lib/hooks';
import { TrackRow } from './common';

export function AlbumPage() {
  const { id = '' } = useParams();
  const [params] = useSearchParams();
  const q = useAlbum(id);
  const hl = params.get('track');
  useEffect(() => {
    if (hl && q.data) document.querySelector(`[data-track="${hl}"]`)?.scrollIntoView({ block: 'center' });
  }, [hl, q.data]);
  if (q.isError) return <div className="page"><ErrorState error={q.error} retry={() => q.refetch()} /></div>;
  const a = q.data?.album;
  const total = a?.tracks.reduce((s, t) => s + t.duration, 0) ?? 0;
  const p = usePlayer.getState();
  return (
    <div className="page album-page" style={{ ['--tint' as string]: a?.palette.base, ['--glow' as string]: a?.palette.accent }}>
      <FloatBar />
      <div className="album-glow" aria-hidden="true" />
      <header className="album-head pad">
        {a ? <img className="album-art" src={a.art.square} alt="" /> : <Skel className="album-art" />}
        <div className="album-info">
          <span className="t-eyebrow">{a ? `${a.format} · ${a.genre}` : ' '}</span>
          <h1 className="album-title">{a?.title ?? <Skel w={300} h={40} />}</h1>
          {a && (
            <Link to={`/artist/${a.artistId}`} className="album-artist" viewTransition>
              {a.artistName}
            </Link>
          )}
          {a && (
            <div className="album-meta tnum">
              {a.year} · {a.tracks.length} songs · {fmtDuration(total)} · {a.tracks[0].codec}
              {a.tracks[0].bitDepth ? ` ${a.tracks[0].bitDepth}-bit/${(a.tracks[0].sampleRate ?? 44100) / 1000} kHz` : ''}
            </div>
          )}
          {a?.review && <p className="album-review clamp-3">{a.review}</p>}
          <div className="album-actions">
            <Button variant="primary" icon={<Play fill="currentColor" />} disabled={!a} onClick={() => a && p.playTracks(a.tracks, 0)}>
              Play
            </Button>
            <Button variant="glass" icon={<Shuffle />} disabled={!a} onClick={() => a && p.playTracks(a.tracks, 0, { shuffle: true })}>
              Shuffle
            </Button>
            <IconButton label="Love album" onClick={() => toast(`Loved ${a?.title}`, <Heart />)}>
              <Heart />
            </IconButton>
            <IconButton label="Download album" onClick={() => toast(`Downloading ${a?.title} (lossless)`, <Download />)}>
              <Download />
            </IconButton>
            <IconButton label="Share" className="hide-mobile" onClick={() => toast('Link copied')}>
              <Share2 />
            </IconButton>
          </div>
        </div>
      </header>
      <div className="pad tlist">
        {a?.tracks.map((t, i) => (
          <div key={t.id} data-track={t.id} className={hl === t.id ? 'hl' : undefined}>
            <TrackRow t={t} index={i} list={a.tracks} showArtist={false} />
          </div>
        ))}
        {a && (
          <p className="album-foot">
            Released {a.year} · ℗ {a.year} {a.label}
          </p>
        )}
      </div>
      {q.data && q.data.more.length > 0 && (
        <section className="dsec">
          <header className="dsec-head">
            <h2>More by {a?.artistName}</h2>
          </header>
          <div className="square-grid">
            {q.data.more.map((x) => (
              <MediaCard key={x.id} shape="square" item={{ id: x.id, kind: 'album', title: x.title, subtitle: String(x.year), image: x.art.square!, href: `/album/${x.id}` }} />
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
