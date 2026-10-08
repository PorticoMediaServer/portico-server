import { useNavigate, useParams } from 'react-router';
import { Play, Shuffle } from 'lucide-react';
import { useCollection } from '../../services/queries';
import { DetailHero, DetailSkeleton, FloatBar } from '../../components/Detail';
import { MediaCard } from '../../components/media';
import { Button, ErrorState } from '../../components/ui';

export function CollectionPage() {
  const { id = '' } = useParams();
  const q = useCollection(id);
  const navigate = useNavigate();
  if (q.isError) return <div className="page"><ErrorState error={q.error} retry={() => q.refetch()} /></div>;
  if (q.isLoading || !q.data) return <DetailSkeleton />;
  const c = q.data;
  const unwatched = c.items.find((i) => !i.watched) ?? c.items[0];
  const runtime = c.items.length;
  return (
    <div className="page flush detail">
      <FloatBar />
      <DetailHero short backdrop={c.art.backdrop} title={c.title} palette={c.palette} eyebrow="Collection" meta={<span>{runtime === 1 ? '1 film' : `${runtime} films`} · {c.items.filter((i) => i.watched).length} watched</span>}>
        <div className="dhero-actions">
          {unwatched && (
            <Button variant="primary" size="lg" className="play-main" icon={<Play fill="currentColor" />} onClick={() => navigate(`/watch/${unwatched.id}`)}>
              Play {unwatched.title}
            </Button>
          )}
          <Button variant="glass" size="lg" icon={<Shuffle />} onClick={() => navigate(`/watch/${c.items[Math.floor(Math.random() * c.items.length)].id}`)}>
            Shuffle
          </Button>
        </div>
        <p className="dhero-summary">{c.summary}</p>
      </DetailHero>
      <div className="detail-body pad">
        <div className="credit-grid">
          {c.items.map((m) => (
            <MediaCard key={m.id} shape="poster" item={m} />
          ))}
        </div>
      </div>
    </div>
  );
}
