import { useState } from 'react';
import { useNavigate, useParams } from 'react-router';
import { Download, MapPin, Play, Share2, Smartphone, Users } from 'lucide-react';
import { useHomeVideo } from '../../services/queries';
import { DetailHero, DetailSkeleton, FloatBar, Section } from '../../components/Detail';
import { MediaCard } from '../../components/media';
import { Button, ErrorState, IconButton, Progress } from '../../components/ui';
import { DownloadSheet, ShareSheet } from '../../components/ItemSheets';
import { fmtClock } from '../../lib/hooks';

export function HomeVideoPage() {
  const { id = '' } = useParams();
  const q = useHomeVideo(id);
  const navigate = useNavigate();
  const [sheet, setSheet] = useState<null | 'dl' | 'share'>(null);
  if (q.isError) return <div className="page"><ErrorState error={q.error} retry={() => q.refetch()} /></div>;
  if (q.isLoading || !q.data) return <DetailSkeleton />;
  const { video: v, state, more } = q.data;
  const p = state.progress;
  return (
    <div className="page flush detail">
      <FloatBar />
      <DetailHero
        short
        backdrop={v.art.backdrop}
        title={v.title}
        palette={v.palette}
        eyebrow="Home Video"
        meta={
          <>
            <span>{new Date(v.recordedAt).toLocaleDateString('en-US', { weekday: 'long', month: 'long', day: 'numeric', year: 'numeric' })}</span>
            <span>{fmtClock(v.duration)}</span>
          </>
        }
      >
        <div className="dhero-actions">
          <Button variant="primary" size="lg" className="play-main" icon={<Play fill="currentColor" />} onClick={() => navigate(`/watch/${v.id}`)}>
            {p ? `Resume · ${fmtClock(p.duration - p.position)} left` : 'Play'}
          </Button>
          <IconButton label="Download" size="lg" onClick={() => setSheet('dl')}>
            <Download />
          </IconButton>
          <IconButton label="Share with family" size="lg" onClick={() => setSheet('share')}>
            <Share2 />
          </IconButton>
        </div>
        {p && (
          <div className="dhero-progress">
            <Progress value={p.position / p.duration} />
          </div>
        )}
        <p className="dhero-summary">{v.summary}</p>
        <div className="hv-facts">
          {v.place && (
            <span>
              <MapPin /> {v.place}
            </span>
          )}
          <span>
            <Smartphone /> {v.device}
          </span>
          {v.people.length > 0 && (
            <span>
              <Users /> {v.people.join(', ')}
            </span>
          )}
        </div>
      </DetailHero>
      <div className="detail-body">
        <Section title="More Home Videos">
          <div className="extras-row no-scrollbar">
            {more.map((x) => (
              <MediaCard key={x.id} shape="landscape" item={{ id: x.id, kind: 'homevideo', title: x.title, subtitle: new Date(x.recordedAt).toLocaleDateString('en-US', { month: 'short', year: 'numeric' }), image: x.art.thumb!, href: `/home-video/${x.id}`, palette: x.palette }} />
            ))}
          </div>
        </Section>
      </div>
      <DownloadSheet open={sheet === 'dl'} onClose={() => setSheet(null)} itemId={v.id} title={v.title} sizeHint={v.duration * 3e6} />
      <ShareSheet open={sheet === 'share'} onClose={() => setSheet(null)} title={v.title} />
    </div>
  );
}
