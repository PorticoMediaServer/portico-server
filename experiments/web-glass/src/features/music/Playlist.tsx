import { useParams } from 'react-router';
import { Pencil, Play, Shuffle, Sparkles } from 'lucide-react';
import { usePlaylist } from '../../services/queries';
import { usePlayer } from '../../state/player';
import { FloatBar } from '../../components/Detail';
import { Button, EmptyState, ErrorState, IconButton, Skel } from '../../components/ui';
import { toast } from '../../components/overlay';
import { ago, fmtDuration, plural } from '../../lib/hooks';
import { Mosaic, TrackRow } from './common';

export function PlaylistPage() {
  const { id = '' } = useParams();
  const q = usePlaylist(id);
  if (q.isError) return <div className="page"><ErrorState error={q.error} retry={() => q.refetch()} /></div>;
  const pl = q.data?.playlist;
  const tracks = q.data?.tracks ?? [];
  const p = usePlayer.getState();
  const total = tracks.reduce((s, t) => s + t.duration, 0);
  return (
    <div className="page album-page" style={{ ['--tint' as string]: pl?.palette.base, ['--glow' as string]: pl?.palette.accent }}>
      <FloatBar />
      <div className="album-glow" aria-hidden="true" />
      <header className="album-head pad">
        <span className="album-art mosaic-wrap">{pl ? <Mosaic images={tracks.map((t) => t.art.square!)} /> : <Skel style={{ width: '100%', height: '100%' }} />}</span>
        <div className="album-info">
          <span className="t-eyebrow">{pl?.smart ? 'Smart Playlist' : 'Playlist'}</span>
          <h1 className="album-title">{pl?.title ?? <Skel w={260} h={40} />}</h1>
          {pl && <p className="album-review">{pl.description}</p>}
          {pl?.smart && (
            <div className="smart-rule">
              <Sparkles /> {pl.smart.rule}
            </div>
          )}
          {pl && (
            <div className="album-meta tnum">
              {plural(tracks.length, 'song')} · {fmtDuration(total)} · Updated {ago(pl.updatedAt)}
            </div>
          )}
          <div className="album-actions">
            <Button variant="primary" icon={<Play fill="currentColor" />} disabled={!tracks.length} onClick={() => p.playTracks(tracks, 0)}>
              Play
            </Button>
            <Button variant="glass" icon={<Shuffle />} disabled={!tracks.length} onClick={() => p.playTracks(tracks, 0, { shuffle: true })}>
              Shuffle
            </Button>
            {!pl?.smart && (
              <IconButton label="Edit playlist" onClick={() => toast('Drag songs to reorder. Changes save automatically.', <Pencil />)}>
                <Pencil />
              </IconButton>
            )}
          </div>
        </div>
      </header>
      <div className="pad tlist">
        {q.data && tracks.length === 0 && <EmptyState icon={<Sparkles />} title="No songs match yet" body="As your library grows, songs matching this rule will appear here automatically." />}
        {tracks.map((t, i) => (
          <TrackRow key={t.id + i} t={t} index={i} list={tracks} showArt showAlbum numbered={false} />
        ))}
      </div>
    </div>
  );
}
