import React, {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {personHref} from '../../app/open';
import {useNavigate, useParams, useSearch} from '@tanstack/react-router';
import type {ContentEntry} from '@core/library-content.ts';
import {defaultShowSettings, saveShowSettings, type ShowSettings, type ShowWorkspaceRoute, type WorkspaceSeason} from '@core/show-workspace.ts';
import {useShowRecommendations, useShowWorkspace} from '../../app/detail';
import {homeRowHeading} from '@core/home.ts';
import {useNotInterested} from '../../app/not-interested-notice';
import {withoutHidden} from '../../app/not-interested';
import {LoadingGrid, SectionView} from '../shared/Sections';
import {useEpisodes, type EpisodeScope} from '../../app/episodes';
import {ErrorNotice, ErrorState, changedError, errorText} from '../../app/errors';
import {useSession} from '../../app/session';
import {usePlayerActions} from '../../player/PlayerContext';
import {Button, Card, Dialog, Inset, KeyValue, Menu, Notice, Page, Section, Segmented, Select, SettingsGroup, SettingsRow, Shelf, StateView, Switch, TitleHero, WindowedGrid, anchorOf, type MenuItem} from '../../ui';
import {SHOW_SETTING_ROWS, dedupeNames, networkName, nextUpPin, originalTitleLine, relatedRowsForPage, seasonProgress, showCreditLines, showPeople} from '@core/presentation/index.ts';
import {placeholderTint} from '../../app/title-layout';
import {EpisodeCard, EpisodePlaceholder, EpisodeRow, episodeCode, episodeListKeys} from './EpisodeCard';
import {GenreLinks} from './GenreLinks';
import {EpisodePanel} from './EpisodePanel';
import s from './Show.module.css';
import {currentI18n} from '../../app/i18n';
import {useViewerScope} from '../../app/viewer-scope';
import {useContainerWatched} from '../shared/container-watched';
import {getContainerState} from '@core/container-state.ts';

type ShowSearch = {library?: string; season?: string; episode?: string; title?: string};
/** Up to this many seasons show as tabs; beyond it, a menu (Spec — Title Pages §2). */
const MAX_SEASON_TABS = 8;
type EpisodeLayout = 'list' | 'grid';
const LAYOUT_KEY = 'portico.episodes.layout';
/** The episode layout is a device preference: list by default (Justin, 2 Oct 2026), grid for those who want stills first. */
function readLayout(): EpisodeLayout { try { return localStorage.getItem(LAYOUT_KEY) === 'grid' ? 'grid' : 'list'; } catch { return 'list'; } }
function writeLayout(v: EpisodeLayout) { try { localStorage.setItem(LAYOUT_KEY, v); } catch { /* blocked storage */ } }

/**
 * The show page (TV and anime alike; Spec — Title Pages §2): the shared hero,
 * then the episodes of one season. The season is state, not a page: it lives
 * in the URL (`?season=2`), so Back and links work, and changing it never
 * reloads the hero. The hero's actions are a movie's: Play/Resume · Watchlist
 * · Favorite · Watched · ⋯. Episodes are a windowed list (or a grid, for
 * those who choose it): the still plays, the copy opens the episode panel at
 * `/show/<id>/episode/<episodeId>` (a direct visit opens the show with the
 * panel open). Next up is pinned above the list when it is not the first row,
 * and the season's progress sits beside the switcher.
 *
 * Below the episodes: cast and key crew, then the show's recommendation rows (More like the
 * show, From its creator…), each named by its titleText; then Details.
 */
export function ShowScreen() {
  const t = currentI18n().t;
  const params = useParams({strict: false}) as {showId: string; episodeId?: string};
  const search = useSearch({strict: false}) as ShowSearch;
  const navigate = useNavigate();
  const player = usePlayerActions();
  const {api, owner} = useSession();
  const scope = useViewerScope();
  const showId = params.showId;
  const libraryId = search.library ?? '';
  // A link without `?library=` (a copied or typed address) resolves the library from the show, then continues, as a movie's does.
  const [missing, setMissing] = useState<unknown>();
  useEffect(() => {
    if (search.library || showId === 'season') return;
    let live = true;
    getContainerState(api, 'show', showId).then(state => { if (live && state.libraryId) void navigate({to: params.episodeId ? '/show/$showId/episode/$episodeId' : '/show/$showId', params: {showId, ...(params.episodeId ? {episodeId: params.episodeId} : {})}, search: {...search, library: state.libraryId}, replace: true}); else if (live) setMissing({code: 'not_found'}); }, e => { if (live) setMissing(e ?? {code: 'not_found'}); });
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api, showId, search.library]);
  const seasonParam = search.season;
  const episodeId = params.episodeId ?? search.episode;
  // Opaque season ids (older links, season cards) load that season directly; numbers are resolved below.
  const seasonIdParam = seasonParam && !/^\d+$/.test(seasonParam) ? seasonParam : undefined;
  const route = useMemo<ShowWorkspaceRoute>(() => ({libraryId, ...(showId !== 'season' ? {showId} : {}), ...(seasonIdParam ? {seasonId: seasonIdParam} : {})}), [libraryId, showId, seasonIdParam]);
  const {snapshot, selectSeason, selectGroup, nextSeasons, previousSeasons, retry} = useShowWorkspace(route);
  const recommendations = useShowRecommendations(showId);
  const notInterested = useNotInterested();
  const recommendationRows = relatedRowsForPage((recommendations.data?.rows ?? []).map(row => withoutHidden(row, notInterested.hidden)));
  const [layout, setLayout] = useState<EpisodeLayout>(readLayout);
  const chooseLayout = (v: EpisodeLayout) => { setLayout(v); writeLayout(v); };
  const data = snapshot.data;
  const show = data?.show;
  const seasons = useMemo(() => orderSeasons(data?.seasons ?? []), [data?.seasons]);
  // People from the show's own credits (the server sends them with the workspace): cast in billing order, key crew by role.
  const people = useMemo(() => showPeople(show?.credits), [show?.credits]);
  // The hero's ⋯ is the menu a card of this show opens (playlist, download all, metadata), anchored to the button.
  const showEntry = useMemo<ContentEntry | undefined>(() => (show ? {id: show.id, libraryId: show.libraryId, kind: 'show', title: show.title, posterUrl: show.posterUrl, backdropUrl: show.backdropUrl, navigation: {view: 'show', entityId: show.id}} : undefined), [show]);
  const selected = data?.selected;
  // The owner's settings for this show (Spec — Title Pages §4): a flat show lists episodes in groups, not seasons.
  const settings = data?.settings ?? defaultShowSettings;
  const flat = settings.hideSeasons || settings.ranges;
  const [editingSettings, setEditingSettings] = useState(false);

  // `?season=2` → that season's id, once the list is known.
  useEffect(() => {
    if (!data || !seasonParam || !/^\d+$/.test(seasonParam)) return;
    const match = data.seasons.find(x => String(x.number) === seasonParam);
    if (match && match.id !== data.selected.seasonId) selectSeason(match.id);
  }, [data, seasonParam, selectSeason]);

  // Episodes: a windowed collection per season, seeded with the workspace's first page.
  const firstPage = data?.episodes.sections[0];
  const where = useMemo<EpisodeScope | undefined>(() => (show && selected ? {libraryId: show.libraryId, showId: show.id, seasonId: selected.seasonId, ...(selected.group ? {group: selected.group} : {})} : undefined), [show, selected]);
  const seed = useMemo(() => (firstPage && snapshot.pagination.cursor === null ? {entries: firstPage.entries, nextCursor: firstPage.nextCursor || null, total: firstPage.totalCount} : undefined), [firstPage, snapshot.pagination.cursor]);
  const episodes = useEpisodes(api, scope, where, seed);

  /** Loaded episodes from `index` on, in order, so Up Next continues through the season. */
  const sequenceFrom = useCallback((episode: ContentEntry): ContentEntry[] => {
    if (!episodes) return [episode];
    const total = episodes.getSnapshot().total ?? 0;
    let start = -1;
    for (let i = 0; i < total && i < 2000; i++) { const item = episodes.itemAt(i); if (item?.id === episode.id) { start = i; break; } if (!item && start < 0 && i > episodes.pageSize * 8) break; }
    if (start < 0) return [episode];
    const out: ContentEntry[] = [];
    for (let i = start; i < total; i++) { const item = episodes.itemAt(i); if (!item) break; out.push(item); }
    return out;
  }, [episodes]);
  const playEpisode = useCallback((episode: ContentEntry, startSeconds?: number) => {
    const list = sequenceFrom(episode).filter(e => e.playback);
    if (!list.length) return;
    // On Playback v1 the season plays as one server request from this episode (a plain season list only; an episode group isn't a season).
    const container = selected?.seasonId && !selected.group ? {kind: 'season' as const, id: selected.seasonId} : undefined;
    player.playSequence(list, 0, startSeconds ?? episode.playback?.startSeconds ?? episode.progressSeconds ?? 0, container);
  }, [player, sequenceFrom, selected?.seasonId, selected?.group]);

  const showSearch = useCallback((extra: Partial<ShowSearch> = {}): ShowSearch => ({library: search.library, ...(search.title ? {title: search.title} : {}), ...(seasonParam ? {season: seasonParam} : {}), ...extra}), [search.library, search.title, seasonParam]);
  const openedHere = useRef(false);
  const openEpisode = useCallback((episode: ContentEntry) => {
    openedHere.current = true;
    // The panel opens over the show where the reader is; the route change must not scroll the page to the top.
    void navigate({to: '/show/$showId/episode/$episodeId', params: {showId, episodeId: episode.id}, search: showSearch(), replace: !!params.episodeId, resetScroll: false});
  }, [navigate, showId, showSearch, params.episodeId]);
  const closeEpisode = useCallback(() => {
    if (openedHere.current && history.length > 1) { openedHere.current = false; history.back(); return; }
    void navigate({to: '/show/$showId', params: {showId}, search: showSearch({episode: undefined}), replace: true});
  }, [navigate, showId, showSearch]);
  const chooseSeason = (season: WorkspaceSeason) => {
    void navigate({to: '/show/$showId', params: {showId}, search: showSearch({season: String(season.number)})});
    selectSeason(season.id);
  };

  // M32: the server's nextUp is the primary action (Resume SxEy / Play next
  // episode / Play S1E1). Until the contract carries it, a bounded client
  // rule over the first page of the season the server opened with (the one
  // holding next-up) — in progress, else the first unwatched, else the
  // first. Pinned per show, so switching seasons never changes the hero.
  const pinnedNextUp = useRef<{showId: string; entry?: ContentEntry} | undefined>(undefined);
  if (show && firstPage && pinnedNextUp.current?.showId !== show.id && snapshot.pagination.cursor === null) {
    const loaded = firstPage.entries;
    pinnedNextUp.current = {showId: show.id, entry: loaded.find(e => e.playback && (e.progressSeconds ?? 0) > 0 && !e.watched) ?? loaded.find(e => e.playback && !e.watched) ?? loaded.find(e => e.playback)};
  }
  const back = () => (history.length > 1 ? history.back() : void navigate({to: '/'}));
  // Show and season watched go through the container personal-state
  // route (O(1)) — never one write per episode. Every hook stays above the
  // error return below; the hook idles until the show has loaded.
  const showWatched = useContainerWatched(api, show ? 'show' : undefined, show?.id);
  const seasonWatched = useContainerWatched(api, selected?.seasonId ? 'season' : undefined, selected?.seasonId ?? undefined);
  if ((snapshot.phase === 'error' && !data) || missing) {
    return (
      <div style={{position: 'relative'}}>
        <TitleHero onBack={back} title={search.title} />
        <Page><ErrorState error={missing ?? snapshot.error} context="show" retry={retry} /></Page>
      </div>
    );
  }

  // M32: the server's nextUp wins when it carries playback; the pinned
  // client rule is the fallback for older servers.
  const nextUp = data?.nextUp?.playback ? data.nextUp : (pinnedNextUp.current?.showId === show?.id ? pinnedNextUp.current?.entry : undefined);
  const resuming = !!nextUp && (nextUp.progressSeconds ?? 0) > 0;
  // The hero plays the show from next up (one v1 request, anchored at the
  // episode), so it never depends on which season is on screen and Up Next
  // continues across seasons.
  const playNextUp = (episode: ContentEntry) => {
    if (!episode.playback || !show) return;
    player.playSequence([episode], 0, episode.playback.startSeconds ?? episode.progressSeconds ?? 0, {kind: 'show', id: show.id});
  };
  const seasonCount = data?.seasonTotalCount ?? 0;
  const selectedSeason = seasons.find(x => x.id === selected?.seasonId);
  const firstEpisode = episodes?.itemAt(0);
  // "6 of 10 watched" beside the switcher, from the season's own counts (the viewer's visible episodes).
  const progress = seasonProgress(seasonWatched.state?.watchedCount, seasonWatched.state?.unwatchedCount);
  // Next up sits above the season's list unless it already is its first row (or belongs to another season).
  const pinned = selected?.group ? undefined : nextUpPin(nextUp, firstEpisode, selectedSeason?.number);
  const watchlisted = !!showWatched.state?.watchlisted;
  const favorite = !!showWatched.state?.favorite;
  const creditLines = showCreditLines(people.crew, (role, names) => t(role === 'creator' ? 'title.createdBy' : role === 'director' ? 'title.directedBy' : 'title.writtenBy', {names}));
  const starring = dedupeNames(people.cast.map(c => c.name), 4).join(', ');
  if (starring) creditLines.push(t('title.starring', {names: starring}));
  const detailRows: [string, string][] = show ? ([
    networkName(show.network) ? [t('title.network'), networkName(show.network)!] : null,
    show.studio ? [t('title.studio'), show.studio] : null,
    show.country ? [t('title.country'), show.country] : null,
    show.genres?.length ? [t('title.genres'), <GenreLinks key="genres" libraryId={show.libraryId} genres={dedupeNames(show.genres, 20)} />] : null,
  ] as ([string, string] | null)[]).filter((r): r is [string, string] => !!r) : [];
  const seasonMenu: MenuItem[] = [
    ...(firstEpisode?.playback ? [{id: 'play-season', label: selectedSeason ? t('title.playSeasonNamed', {season: seasonName(selectedSeason)}) : t('title.playSeason'), icon: 'play' as const}] : []),
    ...(selected?.seasonId ? [{id: 'season-watched', label: t(seasonWatched.state?.watched ? 'entry.markSeasonUnwatched' : 'entry.markSeasonWatched'), icon: (seasonWatched.state?.watched ? 'watched' : 'check') as 'watched' | 'check'}] : []),
  ];
  if (owner && show) seasonMenu.push({id: 'show-settings', label: t('title.showSettings'), icon: 'settings', separatorBefore: seasonMenu.length > 0});
  const onSeasonMenu = (id: string) => {
    if (id === 'show-settings') setEditingSettings(true);
    if (id === 'play-season' && firstEpisode) playEpisode(firstEpisode, 0);
    if (id === 'season-watched' && selected?.seasonId) void seasonWatched.setWatched(!(seasonWatched.state?.watched ?? false)).then(done => { if (done) retry(); });
  };
  const found = episodeId ? findResident(episodes, episodeId) : undefined;

  return (
    <div style={{position: 'relative'}}>
        <TitleHero
          onBack={back}
          loading={!show}
          backdrop={show?.backdropUrl}
          artwork={{path: show?.posterUrl, shape: 'poster', icon: 'tv'}}
          logo={show?.logoUrl}
          title={show?.title ?? search.title}
          originalTitle={originalTitleLine(show?.title, show?.originalTitle)}
          facts={[show?.year ? String(show.year) : undefined, seasonCount ? t('title.seasonCount', {count: seasonCount}) : undefined, show?.genres?.length ? <GenreLinks key="genres" libraryId={show.libraryId} genres={dedupeNames(show.genres)} /> : undefined, networkName(show?.network)]}
          badge={show?.contentRating}
          synopsis={show?.overview}
          credits={creditLines}
          primary={nextUp?.playback ? {label: episodeCode(nextUp) ? t(resuming ? 'title.resumeEpisode' : 'title.playEpisode', {code: episodeCode(nextUp)}) : t(resuming ? 'entry.resume' : 'action.play'), onClick: () => playNextUp(nextUp), progress: resuming && nextUp.duration ? (nextUp.progressSeconds ?? 0) / nextUp.duration : undefined} : undefined}
          secondary={show ? (
            <div className={s.heroActions}>
              {/* The same row as a movie (Justin, 2 Oct 2026): Watchlist · Favorite · Watched · ⋯. The flags are the show's own, on the container route. */}
              <Button variant="secondary" size="lg" icon={watchlisted ? 'bookmarkFilled' : 'bookmark'} label={t(watchlisted ? 'title.inWatchlist' : 'title.watchlist')} selected={watchlisted} disabled={!showWatched.state} onClick={() => void showWatched.setFlag('watchlisted', !watchlisted)} />
              <Button variant="secondary" size="lg" icon={favorite ? 'heartFilled' : 'heart'} aria-label={t(favorite ? 'entry.removeFromFavorites' : 'entry.addToFavorites')} title={t(favorite ? 'entry.removeFromFavorites' : 'entry.addToFavorites')} selected={favorite} disabled={!showWatched.state} onClick={() => void showWatched.setFlag('favorite', !favorite)} />
              <Button variant="secondary" size="lg" icon={showWatched.state?.watched ? 'watched' : 'check'} aria-label={t(showWatched.state?.watched ? 'entry.markShowUnwatched' : 'entry.markShowWatched')} title={t(showWatched.state?.watched ? 'entry.markShowUnwatched' : 'entry.markShowWatched')} selected={showWatched.state?.watched} disabled={showWatched.saving} onClick={() => void showWatched.setWatched(!(showWatched.state?.watched ?? false)).then(done => { if (done) retry(); })} />
              {showEntry ? <Button variant="secondary" size="lg" icon="more" aria-label={t('title.moreActions')} title={t('title.moreActions')} onClick={(e: React.MouseEvent<HTMLElement>) => player.more(showEntry, anchorOf(e.currentTarget), 'page')} /> : null}
            </div>
          ) : undefined}
        />
      {show && editingSettings ? <ShowSettingsDialog libraryId={show.libraryId} showId={show.id} settings={settings} onClose={() => setEditingSettings(false)} onSaved={() => { setEditingSettings(false); retry(); }} /> : null}
      <Page>
        <div style={{position: 'relative', zIndex: 1, display: 'flex', flexDirection: 'column', gap: 32}}>
          {snapshot.phase === 'error' && data ? <Inset><ErrorNotice error={snapshot.error} context="show" retry={retry} /></Inset> : null}
          {snapshot.phase === 'refresh-required' ? <Inset><ErrorNotice error={snapshot.error ?? changedError} context="show" refresh={retry} /></Inset> : null}
          <Section title={t('title.episodes')}>
            <div className={s.seasonBar}>
              <SeasonSwitcher
                seasons={flat ? [] : seasons}
                selectedId={selected?.seasonId ?? null}
                groups={flat && (data?.groups.length ?? 0) < 2 ? [] : data?.groups ?? []}
                selectedGroup={selected?.group}
                morePrevious={snapshot.seasonPagination.canPrevious}
                moreNext={!!snapshot.seasonPagination.nextCursor}
                onSeason={chooseSeason}
                onGroup={selectGroup}
                onPrevious={previousSeasons}
                onNext={nextSeasons}
              />
              <div className={s.episodeToolbar}>
                {progress ? <span className={s.seasonProgress}>{t('title.seasonProgress', progress)}</span> : null}
                <Segmented size="sm" label={t('title.episodeLayout')} value={layout} onChange={chooseLayout} options={[{id: 'list', icon: 'list', label: t('title.viewList')}, {id: 'grid', icon: 'grid', label: t('title.viewGrid')}]} />
                {seasonMenu.length ? <Menu label={t('title.moreActions')} trigger={<Button variant="ghost" size="sm" icon="more" aria-label={t('title.moreActions')} title={t('title.moreActions')} />} items={seasonMenu} onSelect={onSeasonMenu} /> : null}
              </div>
            </div>
            {pinned ? (
              <div className={s.nextUp} style={placeholderTint(show?.id)} onKeyDown={episodeListKeys}>
                <span className={s.nextUpLabel}>{t(resuming ? 'title.continueEpisode' : 'title.nextUp')}</span>
                <EpisodeRow episode={pinned} showSeason onPlay={playNextUp} onOpen={openEpisode} onMore={(e, el) => player.more(e, anchorOf(el))} selected={pinned.id === episodeId} />
              </div>
            ) : null}
            {episodes ? (
              <div style={placeholderTint(show?.id)} onKeyDown={layout === 'list' ? episodeListKeys : undefined}>
              <WindowedGrid
                key={layout}
                collection={episodes}
                columnMin={layout === 'grid' ? 'clamp(240px, 24vw, 320px)' : undefined}
                estimateRowHeight={layout === 'grid' ? 240 : 112}
                className={layout === 'list' ? s.episodeList : undefined}
                label={selectedSeason ? `${seasonName(selectedSeason)} · ${t('title.episodes')}` : t('title.episodes')}
                renderItem={episode => layout === 'grid'
                  ? <EpisodeCard episode={episode} onPlay={playEpisode} onOpen={openEpisode} selected={episode.id === episodeId} />
                  : <EpisodeRow episode={episode} onPlay={playEpisode} onOpen={openEpisode} onMore={(e, el) => player.more(e, anchorOf(el))} selected={episode.id === episodeId} />}
                renderPlaceholder={() => layout === 'grid' ? <EpisodePlaceholder /> : <div className={s.episodeRowPlaceholderRow} aria-hidden />}
              />
              </div>
            ) : data && !firstPage?.entries.length ? (
              <Inset><StateView icon="tv" title={t('title.noEpisodes')} body={t('title.noEpisodesBody')} inline /></Inset>
            ) : (
              <div className={s.loadingGrid} aria-busy>{Array.from({length: 4}, (_, i) => <EpisodePlaceholder key={i} />)}</div>
            )}
          </Section>
          {people.cast.length ? <Shelf title={t('title.cast')} density="person">{people.cast.slice(0, 12).map(c => <Card key={c.id ?? c.name} title={c.name} caption={c.role} path={c.portraitUrl} shape="circle" icon="person" href={c.id ? personHref(c.id) : undefined} onOpen={c.id ? () => void navigate({to: '/person/$personId', params: {personId: c.id!}}) : undefined} />)}</Shelf> : null}
          {people.crew.length ? <Shelf title={t('title.crew')} density="person">{people.crew.map(c => <Card key={c.id ?? c.name} title={c.name} caption={c.role || c.department} path={c.portraitUrl} shape="circle" icon="person" href={c.id ? personHref(c.id) : undefined} onOpen={c.id ? () => void navigate({to: '/person/$personId', params: {personId: c.id!}}) : undefined} />)}</Shelf> : null}
          {recommendations.phase === 'loading' ? <LoadingGrid /> : null}
          {recommendations.phase === 'error' ? <Inset><ErrorNotice error={recommendations.error} context="show" retry={recommendations.retry} refresh={recommendations.retry} /></Inset> : null}
          {recommendationRows.map(row => <SectionView key={row.id} origin="recommendation" section={{id: row.id, type: 'rail', heading: homeRowHeading(row), entries: row.entries, totalCount: row.entries.length, nextCursor: ''}} libraryId={show?.libraryId ?? libraryId} />)}
          {detailRows.length ? <Section title={t('title.details')}><div className={s.showFacts}><KeyValue rows={detailRows} /></div></Section> : null}
        </div>
      </Page>
      {episodeId && show ? (
        <EpisodePanel
          key={episodeId}
          libraryId={show.libraryId}
          showId={show.id}
          episodeId={episodeId}
          preview={found?.entry}
          previous={found?.previous}
          next={found?.next}
          onOpenEpisode={openEpisode}
          onClose={closeEpisode}
          playList={(entry, start) => playEpisode(entry, start)}
        />
      ) : null}
    </div>
  );
}

/** Tabs for up to eight seasons (plus the unassigned group), a menu beyond; Specials last. */
function SeasonSwitcher({seasons, selectedId, groups, selectedGroup, morePrevious, moreNext, onSeason, onGroup, onPrevious, onNext}: {
  seasons: readonly WorkspaceSeason[]; selectedId: string | null; groups: readonly {id: string; title: string; count: number}[]; selectedGroup?: string;
  morePrevious: boolean; moreNext: boolean; onSeason: (season: WorkspaceSeason) => void; onGroup: (id: string) => void; onPrevious: () => void; onNext: () => void;
}) {
  if (!seasons.length && !groups.length) return <span />;
  const tabs = seasons.length + groups.length <= MAX_SEASON_TABS && !morePrevious && !moreNext;
  if (tabs) {
    return (
      <div className={s.seasonTabs} role="tablist" aria-label={currentI18n().t('title.seasons')}>
        {seasons.map(season => <button key={season.id} type="button" role="tab" className={s.seasonTab} aria-selected={!selectedGroup && season.id === selectedId} onClick={() => onSeason(season)}>{seasonName(season)}</button>)}
        {groups.map(group => <button key={group.id} type="button" role="tab" className={s.seasonTab} aria-selected={selectedGroup === group.id} onClick={() => onGroup(group.id)}>{group.id === 'unassigned_absolute' ? `${group.title} (${group.count})` : group.title}</button>)}
      </div>
    );
  }
  const value = selectedGroup ? `group:${selectedGroup}` : selectedId ?? '';
  const options = [
    ...(morePrevious ? [{value: 'seasons:previous', label: currentI18n().t('title.earlierSeasons')}] : []),
    ...seasons.map(season => ({value: season.id, label: seasonName(season)})),
    ...groups.map(group => ({value: `group:${group.id}`, label: group.id === 'unassigned_absolute' ? `${group.title} (${group.count})` : group.title})),
    ...(moreNext ? [{value: 'seasons:next', label: currentI18n().t('title.laterSeasons')}] : []),
  ];
  return (
    <div style={{minWidth: 220}}>
      <Select hideLabel label={currentI18n().t('title.seasonMenu')} value={value} options={options} onChange={e => {
        const v = e.target.value;
        if (v === 'seasons:previous') onPrevious();
        else if (v === 'seasons:next') onNext();
        else if (v.startsWith('group:')) onGroup(v.slice('group:'.length));
        else { const season = seasons.find(x => x.id === v); if (season) onSeason(season); }
      }} />
    </div>
  );
}

function seasonName(season: WorkspaceSeason): string {
  if (season.number === 0) return season.title || currentI18n().t('title.specials');
  return season.title || currentI18n().t('title.season', {number: season.number});
}
/** Specials (season 0) go last. */
function orderSeasons(list: readonly WorkspaceSeason[]): WorkspaceSeason[] {
  return [...list].sort((a, b) => (a.number === 0 ? 1 : 0) - (b.number === 0 ? 1 : 0) || a.number - b.number);
}
/** The episode and its neighbors, when they're among the loaded pages. */
function findResident(collection: ReturnType<typeof useEpisodes>, id: string): {entry: ContentEntry; previous?: ContentEntry; next?: ContentEntry} | undefined {
  if (!collection) return undefined;
  const total = collection.getSnapshot().total ?? 0;
  for (let i = 0; i < total && i < collection.pageSize * 8; i++) {
    const item = collection.itemAt(i);
    if (item?.id === id) return {entry: item, previous: i > 0 ? collection.itemAt(i - 1) : undefined, next: collection.itemAt(i + 1)};
  }
  return undefined;
}

/** The owner's four settings for one show. They change the list for everyone, so they are saved explicitly. */
function ShowSettingsDialog({libraryId, showId, settings, onClose, onSaved}: {libraryId: string; showId: string; settings: ShowSettings; onClose: () => void; onSaved: () => void}) {
  const {api} = useSession();
  const t = currentI18n().t;
  const [draft, setDraft] = useState(settings);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const dirty = SHOW_SETTING_ROWS.some(row => draft[row.key] !== settings[row.key]);
  const save = () => {
    setBusy(true); setError('');
    saveShowSettings(api, libraryId, showId, draft).then(onSaved, e => setError(errorText(e, 'show', 'action'))).finally(() => setBusy(false));
  };
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={t('title.showSettings')} description={t('title.showSettings.lede')} width={480}
      actions={<><Button variant="ghost" label={t('action.cancel')} onClick={onClose} /><Button variant="primary" label={t('action.save')} loading={busy} disabled={!dirty} onClick={save} /></>}>
      {error ? <Notice tone="error" compact>{error}</Notice> : null}
      <SettingsGroup>
        {SHOW_SETTING_ROWS.map(row => <SettingsRow key={row.key} label={t(row.label)} help={t(row.help)} control={<Switch checked={draft[row.key]} onCheckedChange={v => setDraft({...draft, [row.key]: v})} label={t(row.label)} />} />)}
      </SettingsGroup>
    </Dialog>
  );
}
