import {originalTitleLine} from '@core/presentation/index.ts';
import {personHref} from '../../app/open';
import React, {useEffect, useMemo, useRef, useState} from 'react';
import {useNavigate, useParams, useSearch} from '@tanstack/react-router';
import type {DetailProjection} from '@core/detail.ts';
import type {ContentEntry} from '@core/library-content.ts';
import {dedupeNames, detailActions, formatDuration, relatedRowsForPage} from '@core/presentation/index.ts';
import {GenreLinks} from './GenreLinks';
import {PlaybackChoices, type PlaybackChoice} from './PlaybackChoices';
import {TitleFiles} from './TitleFiles';
import {setPendingTrackChoice} from '../../player/pending-choice';
import {useCreditPages, useDetail, useItemRecommendations} from '../../app/detail';
import {titleRecommendationRows} from '@core/recommendations.ts';
import {homeRowHeading} from '@core/home.ts';
import {useNotInterested} from '../../app/not-interested-notice';
import {withoutHidden} from '../../app/not-interested';
import {currentI18n} from '../../app/i18n';
import {repairTargetFor, useMetadataEditor} from '../../app/metadata-editor';
import {useSession} from '../../app/session';
import {usePlayerActions} from '../../player/PlayerContext';
import {anchorOf, AnchoredMenu, Button, Card, Dialog, Icon, Inset, KeyValue, ListRow, Menu, Notice, Page, Section, Shelf, Text, TitleHero, type MenuAnchor, type MenuItem} from '../../ui';
import {CollectionPicker, PlaylistPicker} from '../shared/EntryActions';
import {LoadingGrid, SectionView} from '../shared/Sections';
import s from './Detail.module.css';
import {ErrorNotice, ErrorState, changedError} from '../../app/errors';
import {useDownloads} from '../../app/downloads';

/**
 * Media detail. Artwork hero, identity and decision facts, the primary
 * transport with immediate personal actions, synopsis, then cast and the
 * server's related rows. Lower-frequency actions live in one More menu.
 */
export function DetailScreen() {
  const {itemId} = useParams({from: '/app/media/$itemId'});
  const {library} = useSearch({from: '/app/media/$itemId'});
  const navigate = useNavigate();
  const editor = useMetadataEditor();
  const {owner, session, api} = useSession();
  const downloads = useDownloads();
  // WEB-DETAIL-01: a link without `?library=` resolves the library from the item, then continues.
  const [missing, setMissing] = useState<unknown>();
  useEffect(() => {
    if (library) return;
    let live = true;
    api.item(itemId).then(item => { if (live) void navigate({to: '/media/$itemId', params: {itemId}, search: {library: item.libraryId}, replace: true}); }, e => { if (live) setMissing(e ?? {code: 'not_found'}); });
    return () => { live = false; };
  }, [api, itemId, library, navigate]);
  const target = useMemo(() => (library ? {libraryId: library, itemId} : null), [library, itemId]);
  const {snapshot, mutate, retry, retryMutation, dismissMutation} = useDetail(target);
  const player = usePlayerActions();
  const data = snapshot.data;
  const item = data?.item;
  // Movies read their rows from the recommendations endpoint (below).
  const recommendations = useItemRecommendations(item ? {libraryId: item.libraryId, itemId: item.id, kind: item.kind} : null);
  // P5: every title recommendation offers Not interested; a card turned down leaves at once.
  const notInterested = useNotInterested();
  // P4/P7: every row the recommendations endpoint returns for this title, in its order (More
  // like X, Starring, From its creator, Viewers also watched, then director, genre, person and
  // collections), each named by its titleText. detail.related stands in only until the endpoint
  // answers (and where it doesn't: a failure, or a title it isn't read for).
  const titleRows = titleRecommendationRows(recommendations.data).map(row => withoutHidden(row, notInterested.hidden));
  const actions = data?.actions.filter(a => a.enabled) ?? [];
  const play = actions.find(a => a.playback && a.id !== 'start_over');
  const restart = actions.find(a => a.id === 'start_over');
  const personal = data?.personal;
  const resume = (item?.progressSeconds ?? 0) > 0 && (play?.playback?.startSeconds ?? 0) > 0;
  const remaining = item && resume ? formatDuration(item.duration - item.progressSeconds) : '';
  // CON-25: one shared model decides the hero actions. Primary Play/Resume with the
  // remaining time, quick Watchlist + Favorite toggles, everything else in More
  // grouped like the title ⋯ menu. No plus or star buttons; Media information stays.
  const detail = useMemo(() => (data ? detailActions(data, {owner, local: owner && session?.viewer.authority === 'local', platform: 'web'}) : undefined), [data, owner, session]);
  const menuItems = useMemo<MenuItem[]>(() => {
    const t = currentI18n().t;
    if (!detail) return [];
    const items: MenuItem[] = [];
    for (const group of detail.more) {
      group.actions.forEach((action, i) => items.push({
        // Follow-up: Rate shows its state (Rated N stars vs Rate…), as Apple did before.
        id: action.id, label: (action.id as string) === 'rate' ? (personal?.rating != null ? t('title.rated', {stars: t('title.stars', {count: personal.rating})}) : t('entry.rate')) : t(action.label), icon: (action.id as string) === 'rate' ? (personal?.rating != null ? 'starFilled' : 'star') : action.icon, 
        ...(i === 0 && items.length ? {separatorBefore: true} : {}),
      }));
    }
    return items;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [detail, item, data]);
  // The shared sheet positions itself from the control that opened it. The More
  // menu's own trigger is inside <Menu>, so the actions row is measured instead:
  // it is the same corner of the screen and it always exists when a command runs.
  const actionsRow = useRef<HTMLDivElement>(null);
  const [organise, setOrganise] = useState<{mode: 'playlist' | 'collection'; anchor: MenuAnchor} | undefined>();
  const [organiseNotice, setOrganiseNotice] = useState<string>();
  useEffect(() => { setOrganise(undefined); setOrganiseNotice(undefined); }, [itemId]);
  const openOrganise = (mode: 'playlist' | 'collection', anchor = handoffAnchor()) => {
    if (anchor) setOrganise({mode, anchor});
  };
  const handoffAnchor = () => (actionsRow.current ? anchorOf(actionsRow.current) : undefined);
  function onMenu(id: string) {
    if (!item) return;
    switch (id) {
      case 'playFromStart': if (restart?.playback) player.play(restart.playback.itemId, 0, entryOf(item)); break;
      case 'rate': setRatingOpen(true); break;
      case 'refreshMetadata': void api.request('/v1/items/' + encodeURIComponent(item.id) + '/metadata/refresh', 'POST', {}).then(() => setOrganiseNotice(currentI18n().t('entry.refreshQueued')), () => setOrganiseNotice(currentI18n().t('entry.refreshFailed'))); break;
      case 'editMetadata': { const target = repairTargetFor(entryOf(item)); if (target) editor?.open({targets: [target], titles: [item.title], onSaved: () => void retry()}); break; }
      case 'addToPlaylist': openOrganise('playlist'); break;
      case 'addToCollection': openOrganise('collection'); break;
      case 'download': if (play?.playback) downloads.ask({target: {mediaId: play.playback.itemId}, title: item.title}); break;
    }
  }
  const back = () => (history.length > 1 ? history.back() : void navigate({to: '/'}));
  // Spec — Title Pages §3: an episode isn't a destination of its own; it opens as the panel over its show.
  useEffect(() => {
    const episode = data?.item.episode;
    if (episode?.showId && data) void navigate({to: '/show/$showId/episode/$episodeId', params: {showId: episode.showId, episodeId: data.item.id}, search: {library: data.item.libraryId}, replace: true});
  }, [data, navigate]);
  const [ratingOpen, setRatingOpen] = useState(false);
  const credits = data?.metadata.credits ?? [];
  const cast = credits.filter(c => ['acting', 'cast'].includes(c.department.toLowerCase()));
  const crew = credits.filter(c => !cast.includes(c));
  // WEB-DETAIL-05: the crew shelf shows the people who shaped the title, in
  // credit order, not every department. "All crew" opens the full list.
  // Every hook stays above the error early-return below (a loading→error
  // transition must not call fewer hooks than the previous render).
  const keyCrewList = useMemo(() => keyCrew(crew), [crew]);
  const [allPeople, setAllPeople] = useState(false);
  const [choice, setChoice] = useState<PlaybackChoice>({});
  useEffect(() => { setAllPeople(false); setChoice({}); }, [itemId]);
  // The detail carries the cast's first page and the key crew; the rest of
  // either group pages in as its row scrolls (nothing is cut off).
  const totals = data?.metadata.creditTotals;
  const castPages = useCreditPages(data?.item.id, 'cast', cast, totals?.cast ?? cast.length);
  const crewPages = useCreditPages(data?.item.id, 'crew', crew, totals?.crew ?? crew.length);
  if ((snapshot.phase === 'error' && !data) || missing) {
    return (
      <Page>
        <div className={s.top}><Button variant="ghost" size="sm" icon="back" label={currentI18n().t('action.back')} onClick={back} /></div>
        <ErrorState error={missing ?? snapshot.error} context="detail" retry={retry} />
      </Page>
    );
  }
  // Providers repeat a genre under different ids ("Science Fiction" from TMDB and from TVDB); one name once.
  const t = currentI18n().t;
  const genres = data ? dedupeNames(data.metadata.genres.map(g => g.name)).join(', ') : '';
  const ratings = data?.metadata.ratings ?? [];
  const parent = item?.episode ? t('title.episodeContext', {show: item.episode.showTitle, code: episodeCodeOf(item.episode.seasonNumber, item.episode.number)}) : item?.song ? `${item.song.albumArtist} · ${item.song.albumTitle}` : item?.bookFile ? item.bookFile.bookTitle : undefined;
  // WEB-DETAIL-03: who made it and who is in it, under the synopsis (one entry per person).
  const namesFor = (test: (d: string, r: string) => boolean) => dedupeNames(crew.filter(c => test(c.department.toLowerCase(), c.role.toLowerCase())).map(c => c.name)).join(', ');
  const directors = namesFor((d, r) => d === 'directing' || r === 'director');
  const writers = namesFor((d, r) => d === 'writing' || r.includes('writer') || r === 'screenplay');
  const starring = dedupeNames(cast.map(c => c.name), 4).join(', ');
  const creditLines = [directors ? t('title.directedBy', {names: directors}) : '', writers ? t('title.writtenBy', {names: writers}) : '', starring ? t('title.starring', {names: starring}) : ''].filter(Boolean);
  const trailer = data?.extras?.find(g => g.type === 'trailer')?.items.find(e => e.playback);
  // Genres are links: each opens this library filtered to it.
  const genreNames = data ? dedupeNames(data.metadata.genres.map(g => g.name)) : [];
  const genreLinks = item && genreNames.length ? <GenreLinks libraryId={item.libraryId} genres={genreNames} /> : undefined;
  const facts: React.ReactNode[] = item ? [item.year ? String(item.year) : undefined, item.duration ? formatDuration(item.duration) : undefined, genreLinks] : [];
  const playWithChoice = (startSeconds: number) => {
    if (!item || !play?.playback) return;
    setPendingTrackChoice(choice.audioStreamIndex !== undefined || choice.subtitle !== undefined ? {itemId: play.playback.itemId, audioStreamIndex: choice.audioStreamIndex, subtitle: choice.subtitle} : undefined);
    player.play(play.playback.itemId, startSeconds, entryOf(item), choice.prepared, choice.quality);
  };
  const titleRowsShown = relatedRowsForPage(titleRows);
  const shownCast = castPages.credits.slice(0, CAST_ON_PAGE);
  const detailRows: [string, React.ReactNode][] = item ? ([
    data?.facts?.releaseDate ? [t('title.releaseDate'), formatDate(data.facts.releaseDate)] : null,
    data?.facts?.studio ? [t('title.studio'), data.facts.studio] : null,
    data?.facts?.network ? [t('title.network'), data.facts.network] : null,
    data?.facts?.country ? [t('title.country'), data.facts.country] : null,
    data?.facts?.edition ? [t('title.edition'), data.facts.edition] : null,
    genreLinks ? [t('title.genres'), genreLinks] : null,
    ratings.length ? [t('title.ratings'), ratings.map(r => `${providerLabel(r.provider)} ${ratingLabel(r.value, r.max)}`).join(' · ')] : null,
    item.addedAt ? [t('title.added'), formatDate(item.addedAt)] : null,
  ] as ([string, React.ReactNode] | null)[]).filter((r): r is [string, React.ReactNode] => !!r) : [];
  return (
    <div style={{position: 'relative'}}>
        <TitleHero
          onBack={back}
          loading={!item}
          backdrop={item?.backdropUrl}
          artwork={{path: item?.posterUrl, shape: item?.song || item?.bookFile ? 'square' : 'poster', icon: item?.song ? 'music' : item?.bookFile ? 'book' : 'film'}}
          context={parent}
          title={item?.title}
          originalTitle={originalTitleLine(item?.title, data?.facts?.originalTitle)}
          facts={facts}
          badge={data?.facts?.contentRating}
          extra={ratings.length ? <span className={s.ratings}>{ratings.slice(0, 3).map(r => <span key={r.provider} className={s.rating}><span className={s.ratingProvider}>{providerLabel(r.provider)}</span><strong>{ratingLabel(r.value, r.max)}</strong></span>)}</span> : undefined}
          tagline={data?.facts?.tagline}
          synopsis={item?.overview}
          credits={creditLines}
          choices={item && library && play?.playback ? <PlaybackChoices libraryId={library} itemId={play.playback.itemId} onChange={setChoice} /> : undefined}
          primary={item && play?.playback ? {label: resume ? (remaining ? t('title.resumeRemaining', {remaining}) : t('entry.resume')) : t('action.play'), onClick: () => playWithChoice(play.playback!.startSeconds), progress: resume && item.duration ? item.progressSeconds / item.duration : undefined} : undefined}
          secondary={item ? (
            <div className={s.actionsRow} ref={actionsRow}>
              {/* The same row on every title (CON-25, Justin 2 Oct): Watchlist · Favorite · Watched · ⋯. Like/Dislike is gone. */}
              {detail?.quick.some(q => q.id === 'watchlist') ? <Button variant="secondary" size="lg" icon={personal?.watchlisted ? 'bookmarkFilled' : 'bookmark'} label={t(personal?.watchlisted ? 'title.inWatchlist' : 'title.watchlist')} selected={personal?.watchlisted} onClick={() => mutate({action: 'watchlist', value: !personal?.watchlisted})} /> : null}
              {detail?.quick.some(q => q.id === 'favorite') ? <Button variant="secondary" size="lg" icon={personal?.favorite ? 'heartFilled' : 'heart'} aria-label={t(personal?.favorite ? 'entry.removeFromFavorites' : 'entry.addToFavorites')} title={t(personal?.favorite ? 'entry.removeFromFavorites' : 'entry.addToFavorites')} selected={personal?.favorite} onClick={() => mutate({action: 'favorite', value: !personal?.favorite})} /> : null}
              {detail?.quick.some(q => q.id === 'watched') ? <Button variant="secondary" size="lg" icon={personal?.watched ? 'watched' : 'check'} aria-label={t(personal?.watched ? 'title.markUnwatched' : 'title.markWatched')} title={t(personal?.watched ? 'title.markUnwatched' : 'title.markWatched')} selected={personal?.watched} onClick={() => mutate({action: 'watched', value: !personal?.watched})} /> : null}
              {trailer?.playback ? <Button variant="secondary" size="lg" icon="film" label={t('title.trailer')} onClick={() => player.play(trailer.playback!.itemId, trailer.playback!.startSeconds ?? 0, trailer)} /> : null}
              {menuItems.length ? <Menu label={t('title.moreActions')} trigger={<Button variant="secondary" size="lg" icon="more" aria-label={t('title.moreActions')} title={t('title.moreActions')} />} items={menuItems} onSelect={onMenu} /> : null}
            </div>
          ) : undefined}
        />
      <Page>
        <div className={s.body}>
          {organiseNotice ? <Inset><Notice tone="info">{organiseNotice}</Notice></Inset> : null}
          {snapshot.mutationError ? <Inset><ErrorNotice error={snapshot.mutationError} context="detail" operation="save" retry={retryMutation} refresh={retry} /></Inset> : null}
          {snapshot.pending.some(p => p.phase === 'conflict') ? <Inset><Notice tone="warning" action={{label: t('title.ok'), onClick: dismissMutation}}>{t('title.changedElsewhere')}</Notice></Inset> : null}
          {item && !item.available ? <Inset><Notice tone="warning">{t('title.fileUnavailable')}</Notice></Inset> : null}
          {/* People: the first twelve of the cast and the key crew on the page; See all opens the whole list, paged. */}
          {shownCast.length ? <Shelf title={t('title.cast')} density="person" count={totals?.cast} action={(totals?.cast ?? cast.length) > CAST_ON_PAGE ? {label: t('title.seeAll'), onClick: () => { setAllPeople(true); castPages.more(); crewPages.more(); }} : undefined}>{shownCast.map(c => <Card key={c.id} title={c.name} caption={c.role} path={c.portraitUrl} shape="circle" icon="person" href={c.personId ? personHref(c.personId) : undefined} onOpen={c.personId ? () => void navigate({to: '/person/$personId', params: {personId: c.personId!}}) : undefined} />)}</Shelf> : null}
          {keyCrewList.length ? <Shelf title={t('title.crew')} density="person" action={(totals?.crew ?? crew.length) > keyCrewList.length ? {label: t('title.allCrew'), onClick: () => { setAllPeople(true); castPages.more(); crewPages.more(); }} : undefined}>{keyCrewList.map(c => <Card key={c.id} title={c.name} caption={c.role || c.department} path={c.portraitUrl} shape="circle" icon="person" href={c.personId ? personHref(c.personId) : undefined} onOpen={c.personId ? () => void navigate({to: '/person/$personId', params: {personId: c.personId!}}) : undefined} />)}</Shelf> : null}
          {/* Related rows (P4/P7): three titles or more each, never the same titles twice. */}
          {recommendations.phase === 'error' ? <Inset><ErrorNotice error={recommendations.error} context="detail" retry={recommendations.retry} refresh={recommendations.retry} /></Inset> : null}
          {recommendations.phase === 'loading' && !data?.related?.rows.length ? <LoadingGrid /> : null}
          {recommendations.phase === 'ready' ? titleRowsShown.map(row => <SectionView key={row.id} origin="recommendation" section={{id: row.id, type: 'rail', heading: homeRowHeading(row), entries: row.entries, totalCount: row.entries.length, nextCursor: ''}} libraryId={item?.libraryId} />)
            : relatedRowsForPage((data?.related?.rows ?? []).map(raw => withoutHidden(raw, notInterested.hidden))).map(row => <SectionView key={row.id} origin="recommendation" section={{id: row.id, type: 'rail', heading: {key: row.id, fallback: row.heading}, entries: row.entries, totalCount: row.entries.length, nextCursor: ''}} libraryId={item?.libraryId} />)}
          {/* Extras ride along with the title: trailers, featurettes, deleted scenes, each an ordinary playable item. */}
          {data?.extras?.map(group => <SectionView key={`extra:${group.type}`} section={{id: `extra:${group.type}`, type: 'rail', heading: {key: `extras.${group.type}`, fallback: group.label}, entries: group.items, totalCount: group.items.length, nextCursor: ''}} libraryId={item?.libraryId} />)}
          {/* Details and Files close the page (Spec — Page Content §0.3): the facts, then every version of the file. */}
          {detailRows.length || data?.files?.length ? (
            <Section title={t('title.details')}>
              <div className={s.details}>
                {detailRows.length ? <KeyValue rows={detailRows} /> : null}
                {data?.files?.length ? <TitleFiles files={data.files} /> : null}
              </div>
            </Section>
          ) : null}
        </div>
      </Page>
      {allPeople && item ? (
        <Dialog open onOpenChange={o => !o && setAllPeople(false)} title={t('title.allPeople')} description={item.title} width={560}>
          <div className={s.peopleList}>
            {castPages.credits.length ? <Text variant="label" tone="tertiary">{t('title.cast')}</Text> : null}
            {castPages.credits.map(c => <ListRow key={c.id} title={c.name} subtitle={c.role} art={{path: c.portraitUrl, icon: 'person'}} artShape="circle" onClick={c.personId ? () => { setAllPeople(false); void navigate({to: '/person/$personId', params: {personId: c.personId!}}); } : undefined} />)}
            {!castPages.done ? <div><Button variant="ghost" size="sm" label={t('title.moreCast')} onClick={castPages.more} /></div> : null}
            {crewPages.credits.length ? <Text variant="label" tone="tertiary" style={{marginTop: 12}}>{t('title.crew')}</Text> : null}
            {crewPages.credits.map(c => <ListRow key={c.id} title={c.name} subtitle={c.role || c.department} art={{path: c.portraitUrl, icon: 'person'}} artShape="circle" onClick={c.personId ? () => { setAllPeople(false); void navigate({to: '/person/$personId', params: {personId: c.personId!}}); } : undefined} />)}
            {!crewPages.done ? <div><Button variant="ghost" size="sm" label={t('title.allCrew')} onClick={crewPages.more} /></div> : null}
          </div>
        </Dialog>
      ) : null}
      {ratingOpen && item ? <RatingDialog title={item.title} value={personal?.rating ?? null} onClose={() => setRatingOpen(false)} onRate={value => { mutate({action: 'rating', value}); setRatingOpen(false); }} /> : null}
      <AnchoredMenu open={!!organise} anchor={organise?.anchor} onOpenChange={open => !open && setOrganise(undefined)} label={organise?.mode === 'playlist' ? currentI18n().t('entry.sheet.addToPlaylist') : currentI18n().t('entry.sheet.addToCollection')}>
        {item && organise ? (organise.mode === 'playlist'
          ? <PlaylistPicker key={item.id} itemId={item.id} itemTitles={[item.title]} onDone={notice => { setOrganiseNotice(notice); setOrganise(undefined); }} />
          : <CollectionPicker key={item.id} itemId={item.id} onDone={notice => { setOrganiseNotice(notice); setOrganise(undefined); }} />) : null}
      </AnchoredMenu>
    </div>
  );
}

/** Cast on the page before See all. */
const CAST_ON_PAGE = 12;
function episodeCodeOf(season: number | null | undefined, episode: number): string {
  return season != null ? currentI18n().t('title.episodeCode', {season, episode}) : currentI18n().t('title.episodeOnly', {episode});
}
function formatDate(iso: string): string {
  const d = /^\d{4}-\d{2}-\d{2}/.test(iso) ? new Date(iso.slice(0, 10) + 'T00:00:00') : new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleDateString(undefined, {year: 'numeric', month: 'long', day: 'numeric'});
}
/** A rating's source as its badge names it: provider names are names; Portico's own aggregates are words from the catalogue. */
export function providerLabel(provider: string): string {
  const t = currentI18n().t;
  const names: Record<string, string> = {tmdb: 'TMDB', imdb: 'IMDb', tvdb: 'TVDB', anilist: 'AniList', mal: 'MyAnimeList', rottentomatoes: 'Rotten Tomatoes', metacritic: 'Metacritic'};
  switch (provider.toLowerCase()) {
    case 'effective': return t('title.rating.overall');
    case 'community': return t('title.rating.community');
    case 'audience': return t('title.rating.audience');
    case 'critic': case 'critics': return t('title.rating.critics');
    default: return names[provider.toLowerCase()] ?? provider.charAt(0).toUpperCase() + provider.slice(1);
  }
}
export function ratingLabel(value: number, max: number): string {
  if (max === 100) return `${Math.round(value)}%`;
  const rounded = Math.round(value * 10) / 10;
  return `${Number.isInteger(rounded) ? rounded.toFixed(1) : rounded}/${max}`;
}
export function entryOf(item: DetailProjection['item']): ContentEntry {
  return {id: item.id, libraryId: item.libraryId, kind: (item.kind as ContentEntry['kind']) ?? 'movie', title: item.title, posterUrl: item.posterUrl, backdropUrl: item.backdropUrl, duration: item.duration, progressSeconds: item.progressSeconds, playback: {itemId: item.id, startSeconds: item.progressSeconds}};
}

/**
 * WEB-DETAIL-05: the crew shelf's credit order — Directing, Writing,
 * Screenplay, Creator, Composer, Producer. A credit takes its best (lowest)
 * rank; anything else (property masters, editors…) is crew, not key crew.
 */
export function crewRank(department: string, role: string): number {
  const d = department.trim().toLowerCase();
  const r = role.trim().toLowerCase();
  if (d === 'directing' || r.includes('director')) return 0;
  if (d === 'writing' || r.includes('writ')) return 1;
  if (r.includes('screenplay')) return 2;
  if (d === 'creator' || r.includes('creator') || r.includes('created by')) return 3;
  if (d === 'music' || r.includes('composer')) return 4;
  if (r.includes('producer') || (d === 'production' && r === '')) return 5;
  return -1;
}

/** Key crew in credit order: one row per person, at most 8. */
export function keyCrew<T extends {personId?: string; name: string; department: string; role: string}>(credits: readonly T[]): T[] {
  const ranked = credits.map(c => ({c, rank: crewRank(c.department, c.role)})).filter(x => x.rank >= 0);
  ranked.sort((a, b) => a.rank - b.rank);
  const seen = new Set<string>();
  const out: T[] = [];
  for (const {c} of ranked) {
    const key = c.personId ?? `name:${c.name.trim().toLowerCase()}`;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(c);
    if (out.length >= 8) break;
  }
  return out;
}

/**
 * WEB-DETAIL-02: your rating, 0.5–5 stars in half steps (the server's scale).
 * Click the left half of a star for a half; arrow keys step by a half.
 */
function RatingDialog({title, value, onRate, onClose}: {title: string; value: number | null; onRate: (value: number | null) => void; onClose: () => void}) {
  const t = currentI18n().t;
  const [hover, setHover] = useState<number | null>(null);
  const [draft, setDraft] = useState<number>(value ?? 0);
  const shown = hover ?? draft;
  const pick = (e: React.MouseEvent<HTMLButtonElement>, star: number) => {
    const r = e.currentTarget.getBoundingClientRect();
    return e.clientX - r.left < r.width / 2 ? star - 0.5 : star;
  };
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={t('title.rate', {title})} width={440} actions={<>
      {value != null ? <Button variant="ghost" label={t('title.clearRating')} onClick={() => onRate(null)} /> : null}
      <span style={{flex: 1}} />
      <Button variant="ghost" label={t('action.cancel')} onClick={onClose} />
      <Button variant="primary" label={t('action.save')} disabled={!draft} onClick={() => onRate(draft)} />
    </>}>
      <div role="slider" tabIndex={0} aria-label={t('title.rate', {title})} aria-valuemin={0.5} aria-valuemax={5} aria-valuenow={draft || undefined} aria-valuetext={draft ? t('title.rated', {stars: t('title.stars', {count: draft})}) : undefined}
        onKeyDown={e => { if (e.key === 'ArrowRight' || e.key === 'ArrowUp') { e.preventDefault(); setDraft(d => Math.min(5, (d || 0) + 0.5)); } else if (e.key === 'ArrowLeft' || e.key === 'ArrowDown') { e.preventDefault(); setDraft(d => Math.max(0.5, (d || 1) - 0.5)); } }}
        style={{display: 'flex', gap: 4, justifyContent: 'center', padding: '8px 0'}} onMouseLeave={() => setHover(null)}>
        {[1, 2, 3, 4, 5].map(star => (
          <button key={star} type="button" tabIndex={-1} aria-hidden style={{padding: 4, lineHeight: 0, display: 'block', color: shown >= star - 0.5 ? 'var(--accent)' : 'var(--text-tertiary)', position: 'relative'}} onMouseMove={e => setHover(pick(e, star))} onClick={e => setDraft(pick(e, star))}>
            <Icon name={shown >= star ? 'starFilled' : 'star'} size={36} />
            {shown === star - 0.5 ? <span aria-hidden style={{position: 'absolute', left: 4, top: 4, width: 18, height: 36, overflow: 'hidden', lineHeight: 0, color: 'var(--accent)'}}><Icon name="starFilled" size={36} /></span> : null}
          </button>
        ))}
      </div>
      <Text as="p" variant="caption" tone="secondary" center>{shown ? t('title.stars', {count: shown}) : '\u00a0'}</Text>
    </Dialog>
  );
}
