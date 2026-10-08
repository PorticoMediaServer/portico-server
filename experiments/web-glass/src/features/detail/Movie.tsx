import { useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router';
import {
  Check,
  CircleCheck,
  Plus,
  Download,
  Ellipsis,
  Film,
  Pencil,
  Play,
  RefreshCw,
  RotateCcw,
  Share2,
  Star,
  Trash2,
  Wand2,
} from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import { useMovie, useSetWatched, useToggleWatchlist } from '../../services/queries';
import { api } from '../../services/client';
import { useCurrentProfile } from '../../app/ProfileMenu';
import { ActionTile, CastRow, DetailHero, DetailSkeleton, FloatBar, InfoGrid, Section } from '../../components/Detail';
import { MediaCard, Score, Shelf, WatchlistButton } from '../../components/media';
import { Button, ErrorState, IconButton, Progress, cls } from '../../components/ui';
import { Confirm, Menu, toast, useMenu } from '../../components/overlay';
import { DownloadSheet, EditMetadataSheet, FixMatchSheet, RateSheet, ShareSheet } from '../../components/ItemSheets';
import { fmtBytes, fmtClock, fmtDuration, useIsMobile } from '../../lib/hooks';
import { frameStyle } from '../../lib/scrub';
import { VersionsPanel } from './Versions';

export function MoviePage() {
  const { id = '' } = useParams();
  const q = useMovie(id);
  const me = useCurrentProfile();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const toggle = useToggleWatchlist();
  const watched = useSetWatched();
  const mobile = useIsMobile();
  const more = useMenu();
  const [params] = useSearchParams();
  const [sheet, setSheet] = useState<null | 'download' | 'rate' | 'share' | 'edit' | 'fix' | 'delete'>((params.get('sheet') as 'download') ?? null);

  if (q.isError) return <div className="page"><ErrorState error={q.error} retry={() => q.refetch()} /></div>;
  if (q.isLoading || !q.data) return <DetailSkeleton />;
  const { movie: m, state, related, collections } = q.data;
  const admin = me?.role === 'owner' || me?.role === 'admin';
  const v = m.versions[0];
  const resume = state.progress;
  const hdr = v.hdr;
  const atmos = v.audio.some((a) => a.label.includes('Atmos'));
  const director = m.crew.find((c) => c.job === 'Director');
  const writers = m.crew.filter((c) => c.job === 'Writer');
  const trailer = m.extras.find((x) => x.kind === 'Trailer');
  const subLangs = Array.from(new Set(v.subtitles.map((s) => s.label.replace(/ \(.*\)/, '')))).join(', ');
  const toggleList = () => toggle.mutate(m.id, { onSuccess: (on) => toast(on ? `Added to your Watchlist` : 'Removed from your Watchlist', <Check />) });
  const setWatched = (w: boolean) => watched.mutate({ id: m.id, watched: w }, { onSuccess: () => toast(w ? 'Marked as watched' : 'Marked as unwatched', <CircleCheck />) });

  const menuItems = [
    { label: state.watched ? 'Mark as Unwatched' : 'Mark as Watched', icon: <CircleCheck />, onSelect: () => setWatched(!state.watched) },
    ...(resume ? [{ label: 'Play from Beginning', icon: <RotateCcw />, onSelect: () => navigate(`/watch/${m.id}?t=0`) }] : []),
    { label: 'Rate', icon: <Star />, onSelect: () => setSheet('rate') },
    { label: 'Recommend to…', icon: <Share2 />, onSelect: () => setSheet('share') },
    { label: state.downloaded ? 'Downloaded' : 'Download…', icon: <Download />, onSelect: () => (state.downloaded ? navigate('/downloads') : setSheet('download')) },
    ...(admin
      ? [
          { separator: true },
          { heading: 'Manage' },
          { label: 'Edit Metadata…', icon: <Pencil />, onSelect: () => setSheet('edit') },
          { label: 'Fix Match…', icon: <Wand2 />, onSelect: () => setSheet('fix') },
          {
            label: 'Refresh Metadata',
            icon: <RefreshCw />,
            onSelect: () => {
              toast('Refreshing metadata from The Movie Database…', <RefreshCw />);
              setTimeout(() => qc.invalidateQueries({ queryKey: ['movie'] }), 1200);
            },
          },
          { label: 'Delete from Harbor…', icon: <Trash2 />, danger: true, onSelect: () => setSheet('delete') },
        ]
      : []),
  ];

  const badges = (
    <>
      <span className="badge">{v.resolution === '2160p' ? '4K' : v.resolution === '1080p' ? 'HD' : 'SD'}</span>
      {hdr && <span className="badge">{hdr === 'Dolby Vision' ? 'Dolby Vision' : 'HDR10'}</span>}
      {atmos && <span className="badge">Dolby Atmos</span>}
      {!atmos && v.audio[0].channels !== '2.0' && v.audio[0].channels !== '1.0' && <span className="badge">{v.audio[0].channels}</span>}
      <span className="badge">CC</span>
      {v.subtitles.some((s) => s.sdh) && <span className="badge">SDH</span>}
      {m.versions.length > 1 && <span className="badge">{m.versions.length} Versions</span>}
    </>
  );

  return (
    <div className="page flush detail">
      <FloatBar onMore={more.open} />
      <DetailHero
        backdrop={m.art.backdrop}
        logo={m.art.logo}
        title={m.title}
        palette={m.palette}
        eyebrow={m.source.kind === 'blender' ? 'Blender Studio Open Movie' : m.source.kind === 'public-domain' ? 'Public Domain Classic' : m.genres[0]}
        meta={
          <>
            <span>{m.year}</span>
            <span className="rating-box">{m.contentRating}</span>
            <span>{fmtDuration(m.runtime * 60)}</span>
            <span>{m.genres.join(', ')}</span>
            <Score critic={m.ratings.critic} audience={m.ratings.audience} />
          </>
        }
        badges={badges}
      >
        <div className="dhero-actions">
          <Button variant="primary" size="lg" icon={<Play fill="currentColor" />} onClick={() => navigate(`/watch/${m.id}`)} className="play-main">
            {resume ? `Resume · ${fmtClock(resume.duration - resume.position)} left` : 'Play'}
          </Button>
          {!mobile && (
            <>
              <WatchlistButton on={state.inWatchlist} onToggle={toggleList} />
              {trailer && (
                <IconButton label="Play Trailer" size="lg" onClick={() => navigate(`/watch/${m.id}?extra=${trailer.id}`)}>
                  <Film />
                </IconButton>
              )}
              <IconButton label={state.downloaded ? 'Downloaded' : 'Download'} size="lg" active={!!state.downloaded} onClick={() => (state.downloaded ? navigate('/downloads') : setSheet('download'))}>
                <Download />
              </IconButton>
              <IconButton label="More" size="lg" onClick={more.open}>
                <Ellipsis />
              </IconButton>
            </>
          )}
        </div>
        {resume && (
          <div className="dhero-progress">
            <Progress value={resume.position / resume.duration} />
          </div>
        )}
        {mobile && (
          <div className="atiles">
            <ActionTile icon={state.inWatchlist ? <Check /> : <Plus />} label={state.inWatchlist ? 'In Watchlist' : 'Watchlist'} active={state.inWatchlist} onClick={toggleList} />
            {trailer && <ActionTile icon={<Film />} label="Trailer" onClick={() => navigate(`/watch/${m.id}?extra=${trailer.id}`)} />}
            <ActionTile icon={<Download />} label={state.downloaded ? 'Downloaded' : 'Download'} active={!!state.downloaded} onClick={() => setSheet('download')} />
            <ActionTile icon={<Share2 />} label="Share" onClick={() => setSheet('share')} />
          </div>
        )}
        <p className="dhero-summary">
          {m.tagline && <strong className="tagline">{m.tagline} </strong>}
          {m.summary}
        </p>
        <div className="dhero-credits">
          {director && (
            <span>
              <em>Director</em> {director.name}
            </span>
          )}
          <span>
            <em>Starring</em> {m.cast.slice(0, 3).map((c) => c.name).join(', ') || '—'}
          </span>
        </div>
      </DetailHero>
      <Menu anchor={more.anchor} open={more.isOpen} onClose={more.close} title={m.title} items={menuItems} width={260} />

      <div className="detail-body">
        {m.chapters.length > 1 && m.runtime > 20 && (
          <Section title="Chapters" action={<span className="t-sub">{m.chapters.length} chapters</span>}>
            <div className="chapter-row no-scrollbar">
              {m.chapters.map((c, i) => (
                <button key={i} className="chapter" onClick={() => navigate(`/watch/${m.id}?t=${c.start}`)}>
                  <span className="chapter-frame" style={frameStyle(m.streamId, c.start * 1.37 + i * 11, 220)} />
                  <span className="chapter-title ellipsis">
                    {i + 1}. {c.title}
                  </span>
                  <span className="chapter-time tnum">{fmtClock(c.start)}</span>
                </button>
              ))}
            </div>
          </Section>
        )}

        {m.cast.length > 0 && (
          <Section title="Cast & Crew">
            <CastRow cast={[...m.cast, ...Object.values(m.crew.reduce<Record<string, { personId: string; name: string; role: string }>>((acc, c) => { acc[c.personId] = acc[c.personId] ? { ...acc[c.personId], role: `${acc[c.personId].role}, ${c.job}` } : { personId: c.personId, name: c.name, role: c.job }; return acc; }, {}))]} />
          </Section>
        )}

        {m.extras.length > 0 && (
          <Section title="Trailers & Extras">
            <div className="extras-row no-scrollbar">
              {m.extras.map((x) => (
                <MediaCard key={x.id} shape="landscape" item={{ id: x.id, kind: 'movie', title: x.title, subtitle: `${x.kind} · ${fmtClock(x.runtime)}`, image: x.thumb, href: `/watch/${m.id}?extra=${x.id}` }} />
              ))}
            </div>
          </Section>
        )}

        {collections.map((c) => (
          <Shelf key={c.id} shelf={{ id: c.id, title: `Part of ${c.title}`, style: 'poster', href: `/collection/${c.id}`, items: c.items.filter((x) => x.id !== m.id) }} />
        ))}

        {related.length > 0 && <Shelf shelf={{ id: 'related', title: 'More Like This', style: 'poster', items: related }} />}

        <Section title="About">
          <div className="about-grid">
            <InfoGrid
              rows={[
                ['Released', new Date(m.released).toLocaleDateString('en-US', { month: 'long', day: 'numeric', year: 'numeric' })],
                ['Runtime', fmtDuration(m.runtime * 60)],
                ['Rated', m.contentRating],
                ['Studio', m.studio],
                ['Country', m.country],
                ['Original language', ({ en: 'English', no: 'Norwegian', fr: 'French', pt: 'Portuguese', es: 'Spanish', ja: 'Japanese', it: 'Italian', de: 'German' } as Record<string, string>)[m.originalLanguage] ?? m.originalLanguage],
                ['Written by', writers.map((w) => w.name).join(', ')],
                ['Audio', v.audio.map((a) => a.label).join(' · ')],
                ['Subtitles', subLangs],
                ['Added', new Date(m.addedAt).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })],
              ]}
            />
          </div>
        </Section>

        <Section title="Versions & Files" className="versions-sec" action={<span className="t-sub">{fmtBytes(m.versions.reduce((a, x) => a + x.sizeBytes, 0))} on Harbor</span>}>
          <VersionsPanel versions={m.versions} />
        </Section>

        <p className={cls('source-credit pad')}>{m.source.credit} · {m.source.license}{m.source.url ? ` · ${m.source.url}` : ''}</p>
      </div>

      <DownloadSheet open={sheet === 'download'} onClose={() => setSheet(null)} itemId={m.id} title={m.title} sizeHint={m.runtime * 60 * 10_000 * 125} />
      <RateSheet open={sheet === 'rate'} onClose={() => setSheet(null)} title={m.title} value={state.rating} onRate={(n) => api.setRating(m.id, n).then(() => q.refetch())} />
      <ShareSheet open={sheet === 'share'} onClose={() => setSheet(null)} title={m.title} />
      <EditMetadataSheet open={sheet === 'edit'} onClose={() => setSheet(null)} fields={{ title: m.title, sortTitle: m.sortTitle, year: m.year, summary: m.summary, tagline: m.tagline ?? '', rating: m.contentRating, genres: m.genres, studio: m.studio, art: m.art }} />
      <FixMatchSheet
        open={sheet === 'fix'}
        onClose={() => setSheet(null)}
        title={m.title}
        year={m.year}
        candidates={[
          { id: `tmdb-${1000 + m.title.length * 37}`, title: m.title, year: m.year, source: 'The Movie Database', image: m.art.poster, note: 'current match' },
          { id: `tmdb-${2000 + m.title.length * 53}`, title: m.title, year: m.year - 17, source: 'The Movie Database', note: 'different film, same title' },
          { id: `imdb-tt${3100000 + m.title.length * 991}`, title: `${m.title} (Director’s Cut)`, year: m.year, source: 'IMDb via TMDB' },
        ]}
      />
      <Confirm
        open={sheet === 'delete'}
        onClose={() => setSheet(null)}
        danger
        title={`Delete ${m.title}?`}
        body={`This removes ${m.versions.length === 1 ? 'the file' : `all ${m.versions.length} files`} (${fmtBytes(m.versions.reduce((a, x) => a + x.sizeBytes, 0))}) from Harbor’s disks for everyone. Watch history is kept.`}
        confirmLabel="Delete Files"
        onConfirm={() => toast(`Moved ${m.title} to Harbor’s trash for 30 days`, <Trash2 />)}
      />
    </div>
  );
}
