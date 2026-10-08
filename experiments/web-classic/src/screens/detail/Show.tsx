import {useMemo} from 'react';
import {Link, useNavigate} from '@tanstack/react-router';
import {useClient, useQuery} from '@/data/DataProvider';
import type {EpisodeSummary, Season, ShowDetail} from '@/data/types';
import {useActions} from '@/app/actions';
import {usePlayer} from '@/player/PlayerProvider';
import {date, formatDuration, plural, remaining} from '@/app/format';
import {Button, IconButton} from '@/ui/Button';
import {Artwork, Badge, ErrorState, Progress, Skeleton} from '@/ui/Bits';
import {Dialog, Menu, useToast} from '@/ui/Overlay';
import {Select} from '@/ui/Form';
import {Icon} from '@/ui/Icon';
import {Shelf} from '@/ui/Shelf';
import {useTitle} from '@/ui/hooks';
import {DetailLoading, useRouteParams, useRouteSearch} from '../common';
import {ActionRow, CastRow, DetailFrame, Facts, MetaLine, Overview, People, RatingPill, Ratings} from './parts';
import s from './Detail.module.css';

const seasonName = (se: Season) => (se.number === 0 ? 'Specials' : se.title || `Season ${se.number}`);
const code = (e: EpisodeSummary) => (e.season === 0 ? `Special ${e.episode}` : `S${e.season} E${e.episode}`);

/**
 * Show page: next episode first, seasons (Specials last), each season's
 * episodes with stills, progress and watched state. An episode opens as a
 * panel over its show at /show/:id/episode/:id, with previous and next.
 */
export default function ShowScreen() {
  const {showId, episodeId} = useRouteParams<{showId: string; episodeId?: string}>();
  const search = useRouteSearch<{season?: string}>();
  const q = useQuery(c => c.catalog.show(showId), [showId], ['personal']);
  const navigate = useNavigate();
  useTitle(q.data?.entry.title);
  if (q.error && !q.data) return <ErrorState title="This show didn’t load" body={q.error} onRetry={q.refresh} />;
  if (!q.data) return <DetailLoading />;
  const d = q.data;
  const ordered = [...d.seasons.filter(x => x.number > 0), ...d.seasons.filter(x => x.number === 0)];
  const current = ordered.find(x => x.id === search.season) ?? ordered.find(x => x.episodes.some(e => e.id === (episodeId ?? d.nextUp?.id))) ?? ordered[0]!;
  return (
    <>
      <ShowBody d={d} season={current} seasons={ordered} onSeason={id => void navigate({to: `/show/${showId}` as never, search: {season: id} as never, replace: true, resetScroll: false})} />
      {episodeId ? <EpisodePanel showId={showId} episodeId={episodeId} seasonId={current.id} /> : null}
    </>
  );
}

function ShowBody({d, season, seasons, onSeason}: {d: ShowDetail; season: Season; seasons: Season[]; onSeason: (id: string) => void}) {
  const player = usePlayer();
  const client = useClient();
  const toast = useToast();
  const e = d.entry;
  const all = seasons.filter(x => x.number > 0).flatMap(x => x.episodes);
  const next = d.nextUp ?? all[0];
  const playFrom = (ep: EpisodeSummary, fromStart?: boolean) => player.play(all, Math.max(0, all.findIndex(x => x.id === ep.id)), {fromStart});
  const seasonWatched = season.episodes.every(x => x.watched);
  return (
    <DetailFrame entry={e}
      eyebrow={<><Link to={`/library/${e.libraryId}` as never}>{e.libraryId === 'lib-anime' ? 'Anime' : 'TV Shows'}</Link>{d.network ? <><span aria-hidden>·</span><Link to={`/library/${e.libraryId}/network/${encodeURIComponent(d.network)}` as never}>{d.network}</Link></> : null}</>}
      sections={<>
        <section aria-label="Episodes">
          <div className={s.seasonBar}>
            <h2>Episodes</h2>
            {seasons.length > 1 ? <Select compact ariaLabel="Season" value={season.id} onChange={onSeason} options={seasons.map(x => ({value: x.id, label: `${seasonName(x)} · ${plural(x.episodes.length, 'episode')}`}))} /> : <span className="c-3 t-caption">{seasonName(season)}</span>}
            <span style={{flex: 1}} />
            <span className="t-caption c-3">{season.watchedCount} of {season.episodes.length} watched</span>
            <Menu label="Season options" items={[
              {id: 'play', label: `Play ${seasonName(season)}`, icon: 'play', onSelect: () => player.play(season.episodes, 0)},
              {id: 'shuffle', label: 'Shuffle this season', icon: 'shuffle', onSelect: () => player.play([...season.episodes].sort(() => Math.random() - 0.5), 0)},
              {id: 'watched', label: seasonWatched ? 'Mark season as unwatched' : 'Mark season as watched', icon: 'watched', separatorBefore: true, onSelect: async () => { await client.personal.setWatched(season.episodes.map(x => x.id), !seasonWatched); toast.show(seasonWatched ? `${seasonName(season)} marked as unwatched` : `${seasonName(season)} marked as watched`, {icon: 'check', action: {label: 'Undo', run: () => void client.personal.setWatched(season.episodes.map(x => x.id), seasonWatched)}}); }},
              {id: 'download', label: 'Download season…', icon: 'download', onSelect: () => toast.show(`Downloading ${seasonName(season)} of “${e.title}”`, {icon: 'download'})},
            ]} trigger={<Button variant="secondary" size="sm" icon="more" label="Season" />} />
          </div>
          <div className={s.episodes} role="list">
            {season.episodes.map(ep => <EpisodeRow key={ep.id} ep={ep} showId={e.id} onPlay={fromStart => playFrom(ep, fromStart)} />)}
          </div>
        </section>
        <CastRow people={[...d.creators, ...d.cast]} />
        {d.related.length ? <Shelf title="More like this" entries={d.related} origin="recommendation" /> : null}
      </>}>
      <MetaLine wrapLast items={[e.subtitle, e.rating ? <RatingPill>{e.rating}</RatingPill> : null, plural(seasons.filter(x => x.number > 0).length, 'season'), plural(d.episodeCount, 'episode'), e.genres?.join(', ')]} />
      <Ratings audience={d.audienceRating} mine={d.personal.rating} />
      <ActionRow entry={e} personal={d.personal} onPlay={() => next && playFrom(next)} playLabel={next ? `${next.progress ? 'Resume' : d.watchedCount ? 'Play' : 'Play'} ${code(next)}` : 'Play'} extra={<Button variant="secondary" size="lg" icon="shuffle" label="Shuffle" onClick={() => player.play([...all].sort(() => Math.random() - 0.5), 0)} />} />
      {next && d.watchedCount ? <p className="t-caption c-3">Up next: {code(next)} · {next.title}{next.progress && next.duration ? ` · ${remaining(next.duration, next.progress)}` : ''}</p> : null}
      <Overview text={e.overview} />
      <Facts rows={[['Created by', d.creators.length ? <People list={d.creators} /> : undefined], ['Network', d.network], ['Starring', d.cast.length ? <People list={d.cast.slice(0, 3)} /> : undefined], ['Your progress', `${d.watchedCount} of ${d.episodeCount} episodes watched`]]} />
    </DetailFrame>
  );
}

function EpisodeRow({ep, showId, onPlay}: {ep: EpisodeSummary; showId: string; onPlay: (fromStart?: boolean) => void}) {
  const actions = useActions();
  const href = `/show/${showId}/episode/${ep.id}`;
  return (
    <div className={s.episode} role="listitem">
      <span className={s.epNum}>{ep.episode}</span>
      <div className={s.epStill}>
        <Artwork src={ep.image} title={ep.title} shape="wide" tint={ep.tint} />
        {ep.progress ? <Progress value={ep.progress} className={s.epProgress} /> : null}
        <button type="button" className={s.epPlay} onClick={() => onPlay()} aria-label={`Play ${code(ep)}, ${ep.title}`}><span><Icon name="play" size={20} /></span></button>
      </div>
      <div className={s.epText}>
        <div className={s.epTitle}><Link to={href as never} resetScroll={false}>{ep.title}</Link>{ep.watched ? <span className={s.watchedTick} title="Watched"><Icon name="check" size={12} strokeWidth={3} /></span> : null}</div>
        <div className={s.epMeta}>{code(ep)} · {formatDuration(ep.duration)}{ep.airDate ? ` · ${date(new Date(ep.airDate).getTime())}` : ''}{ep.progress && ep.duration ? ` · ${remaining(ep.duration, ep.progress)}` : ''}</div>
        {ep.overview ? <p className={`${s.epOverview} clamp-2`}>{ep.overview}</p> : null}
      </div>
      <div className={s.epActions}>
        <IconButton variant="ghost" icon="watched" label={ep.watched ? 'Mark as unwatched' : 'Mark as watched'} pressed={!!ep.watched} onClick={() => void actions.toggleWatched(ep)} />
        <Menu label={`More for ${ep.title}`} items={actions.items(ep).filter(i => i.id !== 'show')} trigger={<IconButton variant="ghost" icon="more" label={`More for ${ep.title}`} />} />
      </div>
    </div>
  );
}

/** The episode panel over its show: still, facts, play, previous and next episode. */
function EpisodePanel({showId, episodeId, seasonId}: {showId: string; episodeId: string; seasonId: string}) {
  const q = useQuery(c => c.catalog.episode(episodeId), [episodeId], ['personal']);
  const navigate = useNavigate();
  const player = usePlayer();
  const actions = useActions();
  const close = () => void navigate({to: `/show/${showId}` as never, search: {season: seasonId} as never, resetScroll: false});
  const go = (id: string) => void navigate({to: `/show/${showId}/episode/${id}` as never, resetScroll: false, replace: true});
  const d = q.data;
  const title = useMemo(() => (d ? `${code(d.entry)} · ${d.entry.title}` : 'Episode'), [d]);
  return (
    <Dialog open onOpenChange={o => !o && close()} side="right" title={d?.entry.title ?? 'Loading episode'} description={d ? `${d.show.title} · ${code(d.entry)} · ${d.seasonTitle}` : undefined}>
      {!d ? <><Skeleton height={220} /><Skeleton height={14} width="60%" style={{marginTop: 16}} /><Skeleton height={12} style={{marginTop: 12}} /></> : (
        <div aria-label={title}>
          <div className={s.panelStill}><Artwork src={d.entry.image} title={d.entry.title} shape="wide" eager />{d.entry.progress ? <Progress value={d.entry.progress} className={s.epProgress} /> : null}</div>
          <MetaLine items={[formatDuration(d.entry.duration), d.entry.airDate ? `Aired ${date(new Date(d.entry.airDate).getTime())}` : null, d.personal.watched ? <Badge tone="accent">Watched</Badge> : null]} />
          <div className={s.buttons} style={{margin: '16px 0'}}>
            <Button variant="primary" icon="play" label={d.entry.progress ? 'Resume' : 'Play'} onClick={() => { const list = [d.previous, d.entry, d.next].filter(Boolean) as EpisodeSummary[]; player.play(list, list.indexOf(d.entry)); }} />
            <IconButton variant="secondary" icon="watched" label={d.personal.watched ? 'Mark as unwatched' : 'Mark as watched'} pressed={d.personal.watched} onClick={() => void actions.toggleWatched({...d.entry, watched: d.personal.watched})} />
            <Menu label="More" items={actions.items(d.entry).filter(i => !['play', 'show', 'watched'].includes(i.id))} trigger={<IconButton variant="secondary" icon="more" label="More actions" />} />
          </div>
          <Overview text={d.entry.overview} />
          <div style={{marginTop: 16}}><Facts rows={[['Directed by', <People list={d.directors} />], ['Written by', <People list={d.writers} />], ['Guest cast', <People list={d.cast.slice(0, 4)} />], ['File', d.versions[0] ? `${d.versions[0].label} · ${d.versions[0].size}` : undefined]]} /></div>
          <div className={s.panelNav}>
            {d.previous ? <Button variant="ghost" icon="back" label={`Previous: ${code(d.previous)}`} onClick={() => go(d.previous!.id)} /> : <span />}
            {d.next ? <Button variant="ghost" trailingIcon="forward" label={`Next: ${code(d.next)}`} onClick={() => go(d.next!.id)} /> : null}
          </div>
          <div style={{marginTop: 12}}><Button variant="ghost" size="sm" icon="tv" label={`Go to ${d.show.title}`} onClick={close} /></div>
        </div>
      )}
    </Dialog>
  );
}
