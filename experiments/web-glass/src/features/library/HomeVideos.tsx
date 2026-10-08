import { Video } from 'lucide-react';
import { useHomeVideos } from '../../services/queries';
import { PageHeader } from '../../components/PageHeader';
import { MediaCard } from '../../components/media';
import { EmptyState, ErrorState, Skel } from '../../components/ui';
import { fmtClock } from '../../lib/hooks';

export function HomeVideosPage() {
  const q = useHomeVideos();
  const byYear = new Map<number, NonNullable<typeof q.data>>();
  for (const v of q.data ?? []) {
    const y = new Date(v.recordedAt).getFullYear();
    if (!byYear.has(y)) byYear.set(y, []);
    byYear.get(y)!.push(v);
  }
  return (
    <div className="page">
      <PageHeader title="Home Videos" subtitle={q.data ? `${q.data.length} videos · private to your household` : undefined} />
      <div className="page-body pad">
        {q.isError && <ErrorState error={q.error} retry={() => q.refetch()} />}
        {!q.data && (
          <div className="hv-grid">
            {Array.from({ length: 8 }, (_, i) => (
              <Skel key={i} style={{ aspectRatio: '16 / 9' }} />
            ))}
          </div>
        )}
        {q.data?.length === 0 && <EmptyState icon={<Video />} title="No home videos yet" body="Point a Home Videos library at the folder your phone backs up to." />}
        {[...byYear.entries()].map(([year, list]) => (
          <section key={year} className="hv-year">
            <h2 className="month-head">
              <span className="shelf-title">{year}</span>
            </h2>
            <div className="hv-grid">
              {list.map((v) => (
                <MediaCard key={v.id} shape="landscape" item={{ id: v.id, kind: 'homevideo', title: v.title, subtitle: `${new Date(v.recordedAt).toLocaleDateString('en-US', { month: 'long', day: 'numeric' })} · ${v.place} · ${fmtClock(v.duration)}`, image: v.art.thumb!, href: `/home-video/${v.id}`, palette: v.palette }} />
              ))}
            </div>
          </section>
        ))}
      </div>
    </div>
  );
}
