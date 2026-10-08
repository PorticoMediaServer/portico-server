import { useState } from 'react';
import { Link, useSearchParams } from 'react-router';
import { CheckCircle2, Clock, ListVideo, Trash2 } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import { useHistory, useWatchlist } from '../../services/queries';
import { api } from '../../services/client';
import { PageHeader } from '../../components/PageHeader';
import { MediaCard } from '../../components/media';
import { Button, EmptyState, ErrorState, Segmented, Skel } from '../../components/ui';
import { Confirm, toast } from '../../components/overlay';
import { ago } from '../../lib/hooks';

export function WatchlistPage() {
  const q = useWatchlist();
  const [params, setParams] = useSearchParams();
  const kind = (params.get('kind') as 'all' | 'movie' | 'show' | null) ?? 'all';
  const setKind = (k: 'all' | 'movie' | 'show') => setParams(k === 'all' ? {} : { kind: k }, { replace: true });
  const items = (q.data ?? []).filter((i) => kind === 'all' || i.kind === kind);
  return (
    <div className="page">
      <PageHeader
        title="Watchlist"
        subtitle={q.data ? `${q.data.length} saved · synced across your devices` : undefined}
        actions={
          <Segmented
            value={kind}
            onChange={setKind}
            options={[
              { value: 'all', label: 'All' },
              { value: 'movie', label: 'Movies' },
              { value: 'show', label: 'Shows' },
            ]}
          />
        }
      />
      <div className="page-body pad">
        {q.isError && <ErrorState error={q.error} retry={() => q.refetch()} />}
        {!q.data && !q.isError && (
          <div className="credit-grid">
            {Array.from({ length: 8 }, (_, i) => (
              <Skel key={i} style={{ aspectRatio: '2 / 3' }} />
            ))}
          </div>
        )}
        {q.data && items.length === 0 && (
          <EmptyState
            icon={<ListVideo />}
            title={kind === 'all' ? 'Your Watchlist is empty' : `No ${kind === 'movie' ? 'movies' : 'shows'} saved`}
            body="Tap + on any movie or show to save it for later. Everyone in your household has their own list."
            actions={
              <Button variant="accent" to="/movies">
                Browse Movies
              </Button>
            }
          />
        )}
        <div className="credit-grid">
          {items.map((c) => (
            <MediaCard key={c.id} shape="poster" item={c} />
          ))}
        </div>
      </div>
    </div>
  );
}

export function HistoryPage() {
  const q = useHistory();
  const qc = useQueryClient();
  const [confirm, setConfirm] = useState(false);
  const groups = new Map<string, NonNullable<typeof q.data>>();
  for (const h of q.data ?? []) {
    const d = new Date(h.at);
    const today = new Date();
    const diff = Math.floor((today.setHours(0, 0, 0, 0) - new Date(h.at).setHours(0, 0, 0, 0)) / 86_400_000);
    const k = diff === 0 ? 'Today' : diff === 1 ? 'Yesterday' : diff < 7 ? d.toLocaleDateString('en-US', { weekday: 'long' }) : d.toLocaleDateString('en-US', { month: 'long', day: 'numeric' });
    if (!groups.has(k)) groups.set(k, []);
    groups.get(k)!.push(h);
  }
  return (
    <div className="page">
      <PageHeader
        title="History"
        subtitle="What you’ve watched and listened to, on every device"
        actions={
          q.data?.length ? (
            <Button variant="plain" icon={<Trash2 />} onClick={() => setConfirm(true)}>
              Clear History
            </Button>
          ) : undefined
        }
      />
      <div className="page-body pad history">
        {q.isError && <ErrorState error={q.error} retry={() => q.refetch()} />}
        {!q.data && !q.isError && <Skel h={300} />}
        {q.data?.length === 0 && <EmptyState icon={<Clock />} title="No history yet" body="Things you play will appear here. History is private to your profile." />}
        {[...groups.entries()].map(([day, list]) => (
          <section key={day}>
            <h2 className="group-title">{day}</h2>
            <div className="group">
              {list.map((h) => (
                <Link key={h.id} to={h.itemKind === 'album' ? `/album/${h.itemId}` : h.itemKind === 'audiobook' ? `/audiobook/${h.itemId}` : h.itemKind === 'episode' ? `/episode/${h.itemId}` : h.itemKind === 'homevideo' ? `/home-video/${h.itemId}` : `/movie/${h.itemId}`} className="row hist-row">
                  <img className={h.itemKind === 'album' || h.itemKind === 'audiobook' ? 'hist-img sq' : 'hist-img'} src={h.image} alt="" loading="lazy" />
                  <span className="row-main">
                    <span className="row-title">{h.title}</span>
                    <span className="row-sub">
                      {h.subtitle} · {h.device}
                    </span>
                  </span>
                  <span className="row-value">
                    {h.completed && <CheckCircle2 className="hist-done" />}
                    {ago(h.at)}
                  </span>
                </Link>
              ))}
            </div>
          </section>
        ))}
      </div>
      <Confirm
        open={confirm}
        onClose={() => setConfirm(false)}
        danger
        title="Clear your history?"
        body="Your watch history and Continue Watching will be emptied on every device. Watched checkmarks stay."
        confirmLabel="Clear History"
        onConfirm={async () => {
          await api.clearHistory();
          qc.invalidateQueries({ queryKey: ['history'] });
          toast('History cleared', <Trash2 />);
        }}
      />
    </div>
  );
}
