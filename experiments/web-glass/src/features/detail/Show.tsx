import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router';
import { CalendarClock, Check, CircleCheck, Download, Ellipsis, Pencil, Play, Plus, RefreshCw, Share2, Star, Wand2 } from 'lucide-react';
import type { Episode } from '../../data/types';
import { useSetWatched, useShow, useToggleWatchlist } from '../../services/queries';
import { api } from '../../services/client';
import { useUi } from '../../state/ui';
import { useCurrentProfile } from '../../app/ProfileMenu';
import { ActionTile, CastRow, DetailHero, DetailSkeleton, FloatBar, InfoGrid, Section } from '../../components/Detail';
import { Artwork, Score, Shelf, WatchlistButton } from '../../components/media';
import { Button, ErrorState, IconButton, Progress, Segmented, cls } from '../../components/ui';
import { Menu, toast, useMenu } from '../../components/overlay';
import { DownloadSheet, EditMetadataSheet, FixMatchSheet, RateSheet, ShareSheet } from '../../components/ItemSheets';
import { ago, useIsMobile } from '../../lib/hooks';

export function ShowPage() {
  const { id = '' } = useParams();
  const [params, setParams] = useSearchParams();
  const q = useShow(id);
  const me = useCurrentProfile();
  const navigate = useNavigate();
  const toggle = useToggleWatchlist();
  const watched = useSetWatched();
  const setAnime = useUi((s) => s.setAnimeHint);
  const mobile = useIsMobile();
  const more = useMenu();
  const seasonMenu = useMenu();
  const [sheet, setSheet] = useState<null | 'download' | 'rate' | 'share' | 'edit' | 'fix'>(null);
  const show = q.data?.show;
  useEffect(() => {
    setAnime(!!show?.isAnime);
  }, [show?.isAnime, setAnime]);

  if (q.isError) return <div className="page"><ErrorState error={q.error} retry={() => q.refetch()} /></div>;
  if (q.isLoading || !q.data || !show) return <DetailSkeleton />;
  const { state, nextUp, related, watched: w, progress } = q.data;
  const admin = me?.role === 'owner' || me?.role === 'admin';
  const defaultSeason = nextUp?.episode.season ?? 1;
  const seasonNo = Number(params.get('season') ?? defaultSeason);
  const season = show.seasons.find((s) => s.number === seasonNo) ?? show.seasons[0];
  const total = show.seasons.reduce((a, s) => a + s.episodes.length, 0);
  const seen = Object.values(w).filter(Boolean).length;
  const toggleList = () => toggle.mutate(show.id, { onSuccess: (on) => toast(on ? 'Added to your Watchlist' : 'Removed from your Watchlist', <Check />) });
  const nuProgress = nextUp ? progress[nextUp.episode.id] : undefined;
  const years = `${show.year}${show.status === 'Ended' ? `–${show.seasons[show.seasons.length - 1].year}` : show.status === 'Continuing' ? '–' : ''}`;
  const seasonWatched = season.episodes.every((e) => w[e.id]);

  const menuItems = [
    { label: seen === total ? 'Mark Show as Unwatched' : 'Mark Show as Watched', icon: <CircleCheck />, onSelect: () => watched.mutate({ id: show.id, watched: seen !== total }, { onSuccess: () => toast(seen === total ? 'Marked as unwatched' : `Marked all ${total} episodes as watched`, <CircleCheck />) }) },
    { label: 'Rate', icon: <Star />, onSelect: () => setSheet('rate') },
    { label: 'Recommend to…', icon: <Share2 />, onSelect: () => setSheet('share') },
    { label: 'Download Season…', icon: <Download />, onSelect: () => setSheet('download') },
    ...(admin
      ? [
          { separator: true },
          { heading: 'Manage' },
          { label: 'Edit Metadata…', icon: <Pencil />, onSelect: () => setSheet('edit') },
          { label: 'Fix Match…', icon: <Wand2 />, onSelect: () => setSheet('fix') },
          { label: 'Refresh Metadata', icon: <RefreshCw />, onSelect: () => toast(`Refreshing ${show.title} from ${show.isAnime ? 'AniDB' : 'TheTVDB'}…`, <RefreshCw />) },
        ]
      : []),
  ];

  const pickSeason = (n: number) => {
    const next = new URLSearchParams(params);
    next.set('season', String(n));
    setParams(next, { replace: true });
  };

  return (
    <div className="page flush detail">
      <FloatBar onMore={more.open} />
      <DetailHero
        backdrop={show.art.backdrop}
        logo={show.art.logo}
        title={show.title}
        palette={show.palette}
        eyebrow={show.isAnime ? `Anime · ${show.originalTitle}` : show.network}
        meta={
          <>
            <span>{years}</span>
            <span className="rating-box">{show.contentRating}</span>
            <span>
              {show.seasons.length} Season{show.seasons.length > 1 ? 's' : ''}
            </span>
            <span>{show.genres.filter((g) => g !== 'Anime').join(', ')}</span>
            <Score critic={show.ratings.critic} audience={show.ratings.audience} />
          </>
        }
        badges={
          <>
            {show.airs && (
              <span className="badge new">
                <CalendarClock className="inline-ic" style={{ width: 12, height: 12 }} />
                {show.airs}
              </span>
            )}
            <span className="badge">{show.status}</span>
            {show.isAnime ? <span className="badge">Japanese · English subtitles</span> : <span className="badge">4K</span>}
            <span className="badge">CC</span>
          </>
        }
      >
        <div className="dhero-actions">
          {nextUp && (
            <Button variant="primary" size="lg" icon={<Play fill="currentColor" />} className="play-main" onClick={() => navigate(`/watch/${nextUp.episode.id}`)}>
              {nextUp.label}
            </Button>
          )}
          {!mobile && (
            <>
              <WatchlistButton on={state.inWatchlist} onToggle={toggleList} />
              <IconButton label="Download" size="lg" onClick={() => setSheet('download')}>
                <Download />
              </IconButton>
              <IconButton label="More" size="lg" onClick={more.open}>
                <Ellipsis />
              </IconButton>
            </>
          )}
        </div>
        {nuProgress && (
          <div className="dhero-progress">
            <Progress value={nuProgress.position / nuProgress.duration} />
          </div>
        )}
        {mobile && (
          <div className="atiles">
            <ActionTile icon={state.inWatchlist ? <Check /> : <Plus />} label={state.inWatchlist ? 'In Watchlist' : 'Watchlist'} active={state.inWatchlist} onClick={toggleList} />
            <ActionTile icon={<Download />} label="Download" onClick={() => setSheet('download')} />
            <ActionTile icon={<Share2 />} label="Share" onClick={() => setSheet('share')} />
          </div>
        )}
        {nextUp && (
          <p className="dhero-next">
            <strong>
              S{nextUp.episode.season} · E{nextUp.episode.number} “{nextUp.episode.title}”
            </strong>{' '}
            {nextUp.episode.summary}
          </p>
        )}
        <p className="dhero-summary clamp-3">{show.summary}</p>
        <div className="dhero-credits">
          <span>
            <em>Created by</em> {show.crew.map((c) => c.name).join(', ')}
          </span>
          <span>
            <em>Starring</em> {show.cast.slice(0, 3).map((c) => c.name).join(', ')}
          </span>
        </div>
      </DetailHero>
      <Menu anchor={more.anchor} open={more.isOpen} onClose={more.close} title={show.title} items={menuItems} />

      <div className="detail-body">
        <Section
          title={
            show.seasons.length > 1 && mobile ? (
              <button className="season-pick" onClick={seasonMenu.open}>
                {season.title} <span className="caret">▾</span>
              </button>
            ) : show.seasons.length > 1 ? (
              <Segmented value={String(season.number)} onChange={(v) => pickSeason(Number(v))} options={show.seasons.map((s) => ({ value: String(s.number), label: s.title }))} label="Season" />
            ) : (
              season.title
            )
          }
          action={
            <span className="season-meta">
              <span className="t-sub">
                {season.episodes.length} episodes · {season.year} · {seen}/{total} watched
              </span>
              <Button size="sm" variant="plain" icon={<CircleCheck />} onClick={() => watched.mutate({ id: season.id, watched: !seasonWatched })}>
                {seasonWatched ? 'Mark Unwatched' : 'Mark Season Watched'}
              </Button>
            </span>
          }
        >
          <Menu anchor={seasonMenu.anchor} open={seasonMenu.isOpen} onClose={seasonMenu.close} title="Season" items={show.seasons.map((s) => ({ label: `${s.title} · ${s.episodes.length} episodes`, checked: s.number === season.number, onSelect: () => pickSeason(s.number) }))} />
          <div className="episodes">
            {season.episodes.map((e) => (
              <EpisodeRow key={e.id} e={e} watched={!!w[e.id]} progress={progress[e.id]} isNext={nextUp?.episode.id === e.id} onToggle={() => watched.mutate({ id: e.id, watched: !w[e.id] })} />
            ))}
          </div>
        </Section>

        <Section title="Cast & Crew">
          <CastRow cast={[...show.cast, ...show.crew.map((c) => ({ ...c, role: c.job }))]} />
        </Section>

        {related.length > 0 && <Shelf shelf={{ id: 'related', title: 'More Like This', style: 'poster', items: related }} />}

        <Section title="About">
          <InfoGrid
            rows={[
              ['Network', show.network],
              ['Status', show.status],
              ['First aired', String(show.year)],
              ['Rated', show.contentRating],
              ['Genres', show.genres.join(', ')],
              ['Episodes', `${total} in ${show.seasons.length} season${show.seasons.length > 1 ? 's' : ''}`],
              ['Original title', show.originalTitle],
              ['Metadata', show.isAnime ? 'AniDB + AniList' : 'TheTVDB'],
            ]}
          />
        </Section>
        <p className="source-credit pad">{show.source.credit}</p>
      </div>

      <DownloadSheet open={sheet === 'download'} onClose={() => setSheet(null)} itemId={(nextUp?.episode ?? season.episodes[0]).id} title={`${show.title} · ${season.title}`} sizeHint={season.episodes.length * 2.2e9} />
      <RateSheet open={sheet === 'rate'} onClose={() => setSheet(null)} title={show.title} value={state.rating} onRate={(n) => api.setRating(show.id, n)} />
      <ShareSheet open={sheet === 'share'} onClose={() => setSheet(null)} title={show.title} />
      <EditMetadataSheet open={sheet === 'edit'} onClose={() => setSheet(null)} fields={{ title: show.title, sortTitle: show.sortTitle, year: show.year, summary: show.summary, rating: show.contentRating, genres: show.genres, studio: show.network, art: show.art }} />
      <FixMatchSheet
        open={sheet === 'fix'}
        onClose={() => setSheet(null)}
        title={show.title}
        year={show.year}
        candidates={[
          { id: `tvdb-${300000 + show.title.length * 71}`, title: show.title, year: show.year, source: show.isAnime ? 'AniDB' : 'TheTVDB', image: show.art.poster, note: 'current match' },
          { id: `tmdb-${80000 + show.title.length * 13}`, title: show.title, year: show.year - 9, source: 'The Movie Database', note: 'different series' },
        ]}
      />
    </div>
  );
}

function EpisodeRow({ e, watched, progress, isNext, onToggle }: { e: Episode; watched: boolean; progress?: { position: number; duration: number }; isNext: boolean; onToggle: () => void }) {
  const upcoming = Date.parse(e.airDate) > Date.now();
  const fresh = !upcoming && Date.now() - Date.parse(e.airDate) < 10 * 86_400_000;
  return (
    <div className={cls('ep', isNext && 'next')}>
      <Link to={`/watch/${e.id}`} className="ep-art" aria-label={`Play ${e.title}`}>
        <Artwork src={e.art.thumb} ratio="16 / 9" palette={e.palette}>
          <span className="ep-play glass glass-clear" aria-hidden="true">
            <Play fill="currentColor" />
          </span>
          {progress && <Progress value={progress.position / progress.duration} className="card-progress" />}
        </Artwork>
      </Link>
      <Link to={`/episode/${e.id}`} className="ep-text" viewTransition>
        <span className="ep-kicker tnum">
          Episode {e.number} · {e.runtime} min · {upcoming ? `Airs ${new Date(e.airDate).toLocaleDateString('en-US', { weekday: 'long' })}` : fresh ? `New · ${ago(e.airDate)}` : new Date(e.airDate).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
        </span>
        <span className="ep-title">{e.title}</span>
        <span className="ep-summary clamp-2">{e.summary}</span>
      </Link>
      <button className={cls('ep-check', watched && 'on')} onClick={onToggle} aria-label={watched ? 'Mark as unwatched' : 'Mark as watched'} aria-pressed={watched} title={watched ? 'Watched' : 'Mark as watched'}>
        <Check />
      </button>
    </div>
  );
}
