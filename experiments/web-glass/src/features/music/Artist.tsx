import { useParams } from 'react-router';
import { Play, Radio, Shuffle } from 'lucide-react';
import { useArtist } from '../../services/queries';
import { usePlayer } from '../../state/player';
import { FloatBar, Section } from '../../components/Detail';
import { MediaCard } from '../../components/media';
import { Button, ErrorState, Skel } from '../../components/ui';
import { toast } from '../../components/overlay';
import { TrackRow } from './common';

export function ArtistPage() {
  const { id = '' } = useParams();
  const q = useArtist(id);
  if (q.isError) return <div className="page"><ErrorState error={q.error} retry={() => q.refetch()} /></div>;
  const d = q.data;
  const p = usePlayer.getState();
  const all = d?.albums.flatMap((a) => a.tracks) ?? [];
  return (
    <div className="page flush artist-page" style={{ ['--tint' as string]: d?.artist.palette.base, ['--glow' as string]: d?.artist.palette.accent }}>
      <FloatBar />
      <header className="artist-hero">
        {d && <img className="artist-bg" src={d.artist.art.square} alt="" />}
        <div className="artist-hero-inner pad">
          {d ? <img className="artist-circle" src={d.artist.art.square} alt="" /> : <Skel w={220} h={220} r={999} />}
          <div className="artist-text">
            <span className="t-eyebrow">{d ? `${d.artist.genres.join(' · ')} · ${d.artist.from}` : ' '}</span>
            <h1 className="artist-name">{d?.artist.title ?? <Skel w={260} h={50} />}</h1>
            {d && <div className="t-sub">{d.artist.monthlyPlays.toLocaleString('en-US')} plays in your household this month</div>}
            <div className="album-actions">
              <Button variant="primary" icon={<Play fill="currentColor" />} disabled={!d} onClick={() => d && p.playTracks(d.topTracks, 0)}>
                Play
              </Button>
              <Button variant="glass" icon={<Shuffle />} disabled={!d} onClick={() => p.playTracks(all, 0, { shuffle: true })}>
                Shuffle
              </Button>
              <Button variant="glass" icon={<Radio />} disabled={!d} onClick={() => toast(`Starting ${d?.artist.title} Radio`, <Radio />)}>
                Station
              </Button>
            </div>
          </div>
        </div>
      </header>
      {d && (
        <>
          <Section title="Top Songs">
            <div className="tlist two-col">
              {d.topTracks.map((t, i) => (
                <TrackRow key={t.id} t={t} index={i} list={d.topTracks} showArt showArtist={false} numbered={false} />
              ))}
            </div>
          </Section>
          <Section title="Albums">
            <div className="square-grid">
              {d.albums.map((a) => (
                <MediaCard key={a.id} shape="square" item={{ id: a.id, kind: 'album', title: a.title, subtitle: `${a.year} · ${a.format}`, image: a.art.square!, href: `/album/${a.id}` }} />
              ))}
            </div>
          </Section>
          <Section title={`About ${d.artist.title}`}>
            <p className="person-bio">{d.artist.bio}</p>
          </Section>
          {d.appearsOn.length > 0 && (
            <Section title="Appears On">
              <div className="square-grid">
                {d.appearsOn.map((pl) => (
                  <MediaCard key={pl.id} shape="square" item={{ id: pl.id, kind: 'playlist', title: pl.title, subtitle: 'Playlist', image: pl.art.square!, href: `/playlist/${pl.id}` }} />
                ))}
              </div>
            </Section>
          )}
          {d.similar.length > 0 && (
            <Section title="Similar Artists">
              <div className="square-grid circles">
                {d.similar.map((s) => (
                  <MediaCard key={s.id} shape="circle" item={{ id: s.id, kind: 'artist', title: s.title, subtitle: s.genres[0], image: s.art.square!, href: `/artist/${s.id}` }} />
                ))}
              </div>
            </Section>
          )}
        </>
      )}
    </div>
  );
}
