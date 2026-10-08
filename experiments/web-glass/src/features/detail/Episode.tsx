import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import { Check, ChevronLeft, ChevronRight, CircleCheck, Download, Play } from 'lucide-react';
import { useEpisode, useSetWatched } from '../../services/queries';
import { useUi } from '../../state/ui';
import { DetailHero, DetailSkeleton, FloatBar, InfoGrid, Section } from '../../components/Detail';
import { Button, ErrorState, IconButton, Progress } from '../../components/ui';
import { toast } from '../../components/overlay';
import { DownloadSheet } from '../../components/ItemSheets';
import { fmtClock } from '../../lib/hooks';
import { frameStyle } from '../../lib/scrub';
import { VersionsPanel } from './Versions';

export function EpisodePage() {
  const { id = '' } = useParams();
  const q = useEpisode(id);
  const navigate = useNavigate();
  const watched = useSetWatched();
  const setAnime = useUi((s) => s.setAnimeHint);
  const [dl, setDl] = useState(false);
  useEffect(() => {
    if (q.data) setAnime(!!q.data.show.isAnime);
  }, [q.data, setAnime]);
  if (q.isError) return <div className="page"><ErrorState error={q.error} retry={() => q.refetch()} /></div>;
  if (q.isLoading || !q.data) return <DetailSkeleton />;
  const { episode: e, show, state, prev, next } = q.data;
  const p = state.progress;
  return (
    <div className="page flush detail">
      <FloatBar />
      <DetailHero
        short
        backdrop={e.art.thumb}
        title={e.title}
        palette={e.palette}
        eyebrow={
          <Link to={`/show/${show.id}?season=${e.season}`} className="eyebrow-link">
            {show.title} · Season {e.season}, Episode {e.number}
          </Link>
        }
        meta={
          <>
            <span>{new Date(e.airDate).toLocaleDateString('en-US', { month: 'long', day: 'numeric', year: 'numeric' })}</span>
            <span className="rating-box">{e.contentRating}</span>
            <span>{e.runtime} min</span>
            {state.watched && (
              <span className="pill-status ok">
                <Check className="inline-ic" />
                Watched
              </span>
            )}
          </>
        }
      >
        <div className="dhero-actions">
          <Button variant="primary" size="lg" className="play-main" icon={<Play fill="currentColor" />} onClick={() => navigate(`/watch/${e.id}`)}>
            {p ? `Resume · ${fmtClock(p.duration - p.position)} left` : 'Play Episode'}
          </Button>
          <IconButton label={state.watched ? 'Mark as unwatched' : 'Mark as watched'} size="lg" active={state.watched} onClick={() => watched.mutate({ id: e.id, watched: !state.watched }, { onSuccess: () => toast(state.watched ? 'Marked as unwatched' : 'Marked as watched', <CircleCheck />) })}>
            <CircleCheck />
          </IconButton>
          <IconButton label="Download" size="lg" active={!!state.downloaded} onClick={() => setDl(true)}>
            <Download />
          </IconButton>
        </div>
        {p && (
          <div className="dhero-progress">
            <Progress value={p.position / p.duration} />
          </div>
        )}
        <p className="dhero-summary">{e.summary}</p>
        <div className="ep-nav">
          {prev ? (
            <Link to={`/episode/${prev.id}`} className="ep-nav-link" viewTransition>
              <ChevronLeft /> <span>
                <em>Previous</em> S{prev.season} · E{prev.number} {prev.title}
              </span>
            </Link>
          ) : (
            <span />
          )}
          {next && (
            <Link to={`/episode/${next.id}`} className="ep-nav-link end" viewTransition>
              <span>
                <em>Next</em> S{next.season} · E{next.number} {next.title}
              </span>
              <ChevronRight />
            </Link>
          )}
        </div>
      </DetailHero>
      <div className="detail-body">
        <Section title="Chapters">
          <div className="chapter-row no-scrollbar">
            {e.chapters.map((c, i) => (
              <button key={i} className="chapter" onClick={() => navigate(`/watch/${e.id}?t=${c.start}`)}>
                <span className="chapter-frame" style={frameStyle('coffee-run', c.start * 0.9 + i * 13, 220)} />
                <span className="chapter-title ellipsis">{c.title}</span>
                <span className="chapter-time tnum">{fmtClock(c.start)}</span>
              </button>
            ))}
          </div>
        </Section>
        <Section title="Details">
          <InfoGrid
            rows={[
              ['Show', show.title],
              ['Season', String(e.season)],
              ['Episode', String(e.number)],
              ['Aired', new Date(e.airDate).toLocaleDateString('en-US', { weekday: 'long', month: 'long', day: 'numeric', year: 'numeric' })],
              ['Skip Intro', e.introEnd ? `Detected · ends at ${fmtClock(e.introEnd)}` : '—'],
              ['Credits', e.creditsStart ? `Start at ${fmtClock(e.creditsStart)}` : '—'],
            ]}
          />
        </Section>
        <Section title="Versions & Files">
          <VersionsPanel versions={e.versions} />
        </Section>
      </div>
      <DownloadSheet open={dl} onClose={() => setDl(false)} itemId={e.id} title={`${show.title} ${e.season}×${e.number}`} sizeHint={e.runtime * 60 * 1.25e6} />
    </div>
  );
}
