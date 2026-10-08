import {useState} from 'react';
import {Link, useNavigate, useParams, useSearch} from '@tanstack/react-router';
import {useDataVersion, useQuery, useServices} from '../../data/hooks';
import type {Episode, Season, Show} from '../../data/types';
import {Page, useTitle} from '../../ui/Layout';
import {Art} from '../../ui/Media';
import {Button, IconButton} from '../../ui/Button';
import {Badge, ErrorState, Progress, Skeleton, StateView, toast} from '../../ui/Feedback';
import {Icon} from '../../ui/Icon';
import {Dialog, Menu} from '../../ui/Overlay';
import {Input} from '../../ui/Controls';
import {cx} from '../../ui/cx';
import {ActionBar, CastShelf, Credits, DetailsSection, Facts, PlaybackChoices, ReadMore, RelatedShelf, Stage, TitleSkeleton} from './parts';
import {episodeCode, formatRemaining, plural} from '../shared/items';
import {startPlayback} from '../shared/playback';
import {actions, useEntryMenu} from '../shared/actions';
import {player} from '../../app/player';

/**
 * A show: its marquee with Next up, the season picker and the episodes (landscape stills live
 * here, and only here). An episode opens as a panel over the show and has its own URL.
 */
export function ShowScreen() {
  const params = useParams({strict: false}) as {showId: string; episodeId?: string};
  const search = useSearch({strict: false}) as {season?: string};
  const services = useServices();
  useDataVersion();
  const navigate = useNavigate();
  const q = useQuery(`item.${params.showId}`, () => services.catalog.item(params.showId));
  const show = q.data?.kind === 'show' ? (q.data as Show) : undefined;
  const seasons = useQuery(show ? `seasons.${show.id}` : null, () => services.catalog.items(show!.seasonIds));
  const next = useQuery(show ? `nextup.${show.id}.${services.version()}` : null, async () => {
    const id = await services.catalog.nextUp(show!.id);
    return id ? ((await services.catalog.item(id)) as Episode) : undefined;
  });
  const [jump, setJump] = useState(false);
  useTitle(show?.title);
  const seasonList = (seasons.data ?? []) as Season[];
  const defaultSeason = next.data?.seasonNumber ?? seasonList.find(s => s.number > 0)?.number ?? seasonList[0]?.number;
  const seasonNumber = search.season !== undefined && search.season !== '' ? Number(search.season) : defaultSeason;
  const season = seasonList.find(s => s.number === seasonNumber) ?? seasonList[0];
  if (q.error) return <Page><ErrorState error={q.error} onRetry={q.reload} what="this show" /></Page>;
  if (!show) return q.loading ? <Page flush><TitleSkeleton /></Page> : <Page><StateView icon="search" title="This show isn’t on the server any more" action={{label: 'Go home', to: '/', icon: 'home'}} /></Page>;
  const nextEp = next.data;
  const nst = nextEp ? services.user.state(nextEp.id) : null;
  const resuming = !!nst && nst.progress > 0 && !nst.watched;
  const allWatched = seasonList.length > 0 && next.data === undefined;
  const regular = show.seasonIds.filter(s => !s.endsWith('-s0')).length;
  return (
    <Page flush className="title-page">
      <Stage art={show.art.backdrop ?? show.art.poster} tint={show.tint}>
        <div className="eyebrow stage-eyebrow">{show.anime ? 'Anime' : 'Series'} · {show.network}</div>
        <h1 className="display stage-title">{show.title}</h1>
        <Facts items={[show.yearEnd ? `${show.year}–${show.yearEnd}` : `${show.year}–present`, show.contentRating, plural(regular, 'season'), plural(show.episodeCount, 'episode'), show.genres.slice(0, 2).join(', ')]} badges={<Badge>{show.status === 'Ended' ? 'Ended' : 'Returning'}</Badge>} />
        <ReadMore text={show.synopsis} className="stage-syn" />
        <ActionBar
          item={show}
          playLabel={nextEp ? `${resuming ? 'Resume' : 'Play'} ${episodeCode(nextEp)}` : 'Play'}
          onPlay={() => nextEp && player.play(nextEp.id, {kind: 'video', from: resuming ? nst!.position : 0})}
          extra={<Button variant="glass" icon="hash" label="Jump to" onClick={() => setJump(true)} className="jump-btn" />}
        />
        {nextEp ? (
          <p className="next-up">
            <span className="eyebrow">{resuming ? 'Continue' : allWatched ? 'Watch again' : 'Next up'}</span>
            <Link to="/show/$showId/episode/$episodeId" params={{showId: show.id, episodeId: nextEp.id}} className="credit-link">{episodeCode(nextEp)} · {nextEp.title}</Link>
            {resuming ? <span className="faint">{formatRemaining(nextEp.runtime * 60 - nst!.position)}</span> : null}
          </p>
        ) : null}
        <Credits credits={show.credits} />
      </Stage>
      <div className="title-sections">
        <div className="page-pad">
          <SeasonBar show={show} seasons={seasonList} current={season} onPick={n => void navigate({to: '/show/$showId', params: {showId: show.id}, search: {season: String(n)}, replace: true, resetScroll: false})} />
          {season ? <EpisodeList show={show} season={season} /> : <EpisodeSkeleton />}
        </div>
        <CastShelf credits={show.credits} />
        <RelatedShelf id={show.id} />
        <div className="page-pad">
          <DetailsSection item={show as never} about={[['Network', show.network], ['Genres', show.genres.join(', ')], ['Status', show.status === 'Ended' ? `Ended in ${show.yearEnd ?? show.year}` : 'Returning series'], ['Rating', show.contentRating], ['Added', new Date(show.addedAt).toLocaleDateString('en-US', {month: 'long', day: 'numeric', year: 'numeric'})]]} />
        </div>
      </div>
      {params.episodeId ? <EpisodePanel show={show} episodeId={params.episodeId} /> : null}
      <JumpDialog show={show} seasons={seasonList} open={jump} setOpen={setJump} />
    </Page>
  );
}

function SeasonBar({show, seasons, current, onPick}: {show: Show; seasons: Season[]; current?: Season; onPick: (n: number) => void}) {
  const services = useServices();
  if (!seasons.length) return <div className="season-bar"><Skeleton w={320} h={40} r={20} /></div>;
  const watched = current ? current.episodeIds.filter(e => services.user.state(e).watched).length : 0;
  const menu = current ? [
    {id: 'play', label: `Play ${current.title}`, icon: 'play' as const, onSelect: () => void startPlayback(services, current)},
    {id: 'watched', label: watched === current.episodeIds.length ? 'Mark season unwatched' : 'Mark season watched', icon: 'check' as const, onSelect: () => { const on = watched !== current.episodeIds.length; void services.user.markContainerWatched(current.id, on).then(n => toast(`${current.title}: ${plural(n, 'episode')} marked ${on ? 'watched' : 'unwatched'}`, {action: {label: 'Undo', onClick: () => void services.user.markContainerWatched(current.id, !on)}})); }},
    {id: 'download', label: 'Download season…', icon: 'download' as const, onSelect: () => actions.download(current.episodeIds)},
    {id: 'playlist', label: 'Add season to playlist…', icon: 'playlist' as const, onSelect: () => actions.addToPlaylist(current.episodeIds)},
  ] : [];
  return (
    <div className="season-bar">
      <div className="season-tabs" role="tablist" aria-label="Seasons">
        {seasons.map(s => (
          <button key={s.id} type="button" role="tab" aria-selected={s.id === current?.id} className={cx('season-tab', s.id === current?.id && 'is-on')} onClick={() => onPick(s.number)}>
            {s.number === 0 ? 'Specials' : seasons.length > 6 ? `S${s.number}` : s.title}
          </button>
        ))}
      </div>
      {current ? <span className="season-count faint">{watched ? `${watched} of ${plural(current.episodeIds.length, 'episode')} watched` : plural(current.episodeIds.length, 'episode')}</span> : null}
      <Menu label="Season options" items={menu} trigger={<button type="button" className="btn btn-secondary btn-sm"><Icon name="more" size={16} /><span className="btn-label">Season</span></button>} />
    </div>
  );
}

function EpisodeList({show, season}: {show: Show; season: Season}) {
  const services = useServices();
  useDataVersion();
  const eps = useQuery(`episodes.${season.id}`, () => services.catalog.items(season.episodeIds));
  const list = (eps.data ?? []) as Episode[];
  if (!list.length && eps.loading) return <EpisodeSkeleton />;
  if (!list.length) return <StateView compact icon="tv" title="No episodes in this season yet" body="When episodes are added to the server they appear here." />;
  return (
    <ol className="episodes">
      {list.map(e => {
        const st = services.user.state(e.id);
        const inProgress = st.progress > 0 && !st.watched;
        return (
          <li key={e.id} className={cx('episode', st.watched && 'is-watched')}>
            <Link to="/show/$showId/episode/$episodeId" params={{showId: show.id, episodeId: e.id}} search={{season: String(season.number)}} resetScroll={false} className="episode-link" aria-label={`${episodeCode(e)}: ${e.title}${st.watched ? ', watched' : inProgress ? ', in progress' : ''}`}>
              <span className="episode-still">
                <Art src={e.art.still} tint={e.tint} ratio="16/9" />
                <span className="episode-play" aria-hidden><Icon name="play" size={22} fill="currentColor" /></span>
                {inProgress ? <span className="card-progress"><Progress value={st.progress} thin tone="white" /></span> : null}
                {st.watched ? <span className="card-watched"><Icon name="check" size={13} stroke={3} /></span> : null}
              </span>
              <span className="episode-text">
                <span className="episode-top"><span className="episode-code">{e.number}</span><span className="episode-title">{e.title}</span></span>
                <span className="episode-meta">{e.runtime} min · {new Date(e.airDate).toLocaleDateString('en-US', {month: 'short', day: 'numeric', year: 'numeric'})}{inProgress ? ` · ${formatRemaining(e.runtime * 60 - st.position)}` : ''}</span>
                <span className="episode-syn clamp-2">{e.synopsis}</span>
              </span>
            </Link>
            <button type="button" className="episode-quick" aria-label={`Play ${e.title}`} onClick={() => player.play(e.id, {kind: 'video', from: inProgress ? st.position : 0})}><Icon name="play" size={16} fill="currentColor" /></button>
          </li>
        );
      })}
    </ol>
  );
}

function EpisodeSkeleton() {
  return <div className="episodes">{[0, 1, 2].map(i => <div key={i} className="episode"><span className="episode-link"><Skeleton className="episode-still" style={{aspectRatio: '16/9'}} /><span className="episode-text"><Skeleton w="50%" h={16} /><Skeleton w="30%" h={12} style={{marginTop: 8}} /><Skeleton w="90%" h={12} style={{marginTop: 12}} /></span></span></div>)}</div>;
}

/** The episode panel over its show: details, versions, cast, and previous/next. */
function EpisodePanel({show, episodeId}: {show: Show; episodeId: string}) {
  const services = useServices();
  useDataVersion();
  const navigate = useNavigate();
  const q = useQuery(`item.${episodeId}`, () => services.catalog.item(episodeId));
  const ep = q.data?.kind === 'episode' ? (q.data as Episode) : undefined;
  const season = useQuery(ep ? `item.${ep.seasonId}` : null, () => services.catalog.item(ep!.seasonId));
  const menu = useEntryMenu(ep, 'detail');
  const close = () => void navigate({to: '/show/$showId', params: {showId: show.id}, search: ep ? {season: String(ep.seasonNumber)} : {}, resetScroll: false});
  const ids = (season.data as Season | undefined)?.episodeIds ?? [];
  const i = ep ? ids.indexOf(ep.id) : -1;
  const prev = i > 0 ? ids[i - 1] : undefined;
  const nextId = i >= 0 && i < ids.length - 1 ? ids[i + 1] : undefined;
  const st = ep ? services.user.state(ep.id) : null;
  const inProgress = !!st && st.progress > 0 && !st.watched;
  const go = (id?: string) => id && void navigate({to: '/show/$showId/episode/$episodeId', params: {showId: show.id, episodeId: id}, replace: true, resetScroll: false});
  return (
    <Dialog open onOpenChange={o => !o && close()} title={ep ? ep.title : 'Episode'} kind="sheet" size="lg" hideTitle className="episode-sheet">
      {!ep ? <div className="stack"><Skeleton h={300} /><Skeleton w="60%" h={28} /><Skeleton w="90%" h={14} /></div> : (
        <div className="ep-panel">
          <div className="ep-hero">
            <Art src={ep.art.still} tint={ep.tint} ratio="16/9" eager />
            <div className="ep-hero-scrim" />
            <button type="button" className="ep-hero-play" onClick={() => player.play(ep.id, {kind: 'video', from: inProgress ? st!.position : 0})} aria-label={`${inProgress ? 'Resume' : 'Play'} ${ep.title}`}><Icon name="play" size={30} fill="currentColor" /></button>
          </div>
          <div className="ep-body">
            <div className="eyebrow"><Link to="/show/$showId" params={{showId: show.id}} className="credit-link">{show.title}</Link> · {episodeCode(ep)}</div>
            <h2 className="display ep-title">{ep.title}</h2>
            <Facts items={[new Date(ep.airDate).toLocaleDateString('en-US', {month: 'long', day: 'numeric', year: 'numeric'}), `${ep.runtime} min`, show.contentRating]} badges={st?.watched ? <Badge tone="good" icon="check">Watched</Badge> : null} />
            <p className="ep-syn">{ep.synopsis}</p>
            <div className="action-bar">
              <div className="action-primary">
                <Button variant="primary" size="lg" icon="play" label={inProgress ? 'Resume' : 'Play'} onClick={() => player.play(ep.id, {kind: 'video', from: inProgress ? st!.position : 0})} />
                {inProgress ? <span className="resume-meta"><span className="resume-bar"><span style={{width: `${st!.progress * 100}%`}} /></span><span>{formatRemaining(ep.runtime * 60 - st!.position)}</span></span> : null}
              </div>
              <div className="action-secondary">
                {inProgress ? <IconButton icon="rewind" label="Play from beginning" onClick={() => player.play(ep.id, {kind: 'video', from: 0})} /> : null}
                <IconButton icon="eye" label={st?.watched ? 'Mark unwatched' : 'Mark watched'} active={st?.watched} onClick={() => void services.user.set([ep.id], {watched: !st?.watched}).then(() => toast(st?.watched ? 'Marked unwatched' : 'Marked watched'))} />
                <Menu label="More actions" trigger={<button type="button" className="btn btn-glass btn-md btn-icon btn-round" aria-label="More actions"><Icon name="more" size={19} /></button>} items={menu} />
              </div>
            </div>
            <PlaybackChoices versions={ep.versions} audio={ep.audioTracks} subs={ep.subtitleTracks} itemId={ep.id} />
            {ep.credits.length ? (
              <div className="ep-cast">
                <h3 className="eyebrow">Cast</h3>
                <ul>{ep.credits.map(c => <li key={c.personId}><Link to="/person/$personId" params={{personId: c.personId}} className="credit-link">{c.name}</Link>{c.character ? <span className="faint"> as {c.character}</span> : null}</li>)}</ul>
              </div>
            ) : null}
            <div className="ep-nav">
              <Button variant="secondary" icon="chevronLeft" label="Previous episode" disabled={!prev} onClick={() => go(prev)} />
              <Button variant="secondary" trailingIcon="chevronRight" label="Next episode" disabled={!nextId} onClick={() => go(nextId)} />
            </div>
          </div>
        </div>
      )}
    </Dialog>
  );
}

/** Jump straight to a season and episode number ("S3 E4" or "3 4"). */
function JumpDialog({show, seasons, open, setOpen}: {show: Show; seasons: Season[]; open: boolean; setOpen: (o: boolean) => void}) {
  const [text, setText] = useState('');
  const [error, setError] = useState('');
  const navigate = useNavigate();
  const submit = () => {
    const m = text.trim().match(/^s?\s*(\d+)\s*[\s,x.e:-]+\s*e?\s*(\d+)$/i) ?? text.trim().match(/^s?(\d+)$/i);
    if (!m) return setError('Enter a season and episode, like S2 E5.');
    const s = seasons.find(x => x.number === Number(m[1]));
    if (!s) return setError(`${show.title} doesn’t have a season ${m[1]}.`);
    const e = m[2] ? s.episodeIds[Number(m[2]) - 1] : s.episodeIds[0];
    if (!e) return setError(`Season ${m[1]} has ${plural(s.episodeIds.length, 'episode')}.`);
    setOpen(false);
    setText('');
    void navigate({to: '/show/$showId/episode/$episodeId', params: {showId: show.id, episodeId: e}, search: {season: String(s.number)}, resetScroll: false});
  };
  return (
    <Dialog open={open} onOpenChange={o => { setOpen(o); setError(''); }} title="Jump to an episode" description="Type a season and episode, like S2 E5 or 2 5." size="sm" footer={<Button variant="primary" label="Go" onClick={submit} />}>
      <form onSubmit={e => { e.preventDefault(); submit(); }}>
        <Input value={text} onChange={e => { setText(e.target.value); setError(''); }} placeholder="S1 E1" autoFocus aria-label="Season and episode" aria-invalid={!!error} />
        {error ? <p className="field-error" style={{marginTop: 8}}><Icon name="alert" size={14} />{error}</p> : null}
      </form>
    </Dialog>
  );
}

