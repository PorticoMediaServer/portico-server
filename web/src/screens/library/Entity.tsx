import {useSelectionRefresh} from '../../app/selection';
import {useBack} from '../../app/back';
import React, {useCallback, useMemo, useState} from 'react';
import {useNavigate, useParams, useSearch} from '@tanstack/react-router';
import type {ContentEntry, ContentRoute, ContentView} from '@core/library-content.ts';
import {useContent} from '../../app/content';
import {usePlayingItemId} from '../../player/engine';
import {activeYearsLabel, artistReleaseGroups, captionFor, formatClock, formatDuration, iconFor, shapeFor} from '@core/presentation/index.ts';
import {mosaicPaths} from '../../app/title-layout';
import {albumHeroFacts, artistHeroFacts, discGroups, trackSubtitleForAlbum, type T as WordsT} from '../../app/library-words';
import {useOpenEntry} from '../../app/open';
import {groupSequence, sequenceContainer} from '../../app/container-play';
import {usePlayerActions} from '../../player/PlayerContext';
import {useSession} from '../../app/session';
import {useDownloads} from '../../app/downloads';
import {containerKindFor, useContainerWatched} from '../shared/container-watched';
import {Button, Inset, KeyValue, ListRow, Page, Section, Skeleton, StateView, Surface, Text, TitleHero, type MenuAnchor} from '../../ui';
import {ManualOrderList, useWholeList} from '../shared/ManualOrder';
import {moveRequest} from '@core/presentation/manual-order.ts';
import {moveCollectionTitle} from '@core/library-collection-order.ts';
import {LoadingShelves, SectionView, sectionHeading} from '../shared/Sections';
import {GroupBar, GroupGrid, GroupRows} from '../shared/GroupBrowse';
import {entityGroups, useGroupBrowse, type GroupChoice} from '../../app/group-browse';
import {useLibrariesContext} from '../../app/libraries';
import body from '../detail/Detail.module.css';
import {ErrorNotice, ErrorState, changedError} from '../../app/errors';
import {useI18n} from '../../app/i18n';
import {useRefreshRequiredRetry} from '../../app/quiet-refresh';

/**
 * Generic entity workspace: artist, album, collection, category, author,
 * series, disc. The server describes the entity and its sections; the hero
 * adapts its artwork geometry to the entity kind (album square, artist
 * circle). Album and artist track lists are drawn here (WEB-DETAIL-10):
 * numbered rows with duration and ⋯, the subtitle only for guest artists,
 * and "Disc N" subheadings inside the list for multi-disc albums.
 */
export function EntityScreen() {
  const i18n = useI18n();
  const {t} = i18n;
  const {libraryId, view, entityId} = useParams({from: '/app/library/$libraryId/$view/$entityId'});
  const address = useSearch({from: '/app/library/$libraryId/$view/$entityId'});
  const {title} = address;
  const navigate = useNavigate();
  const player = usePlayerActions();
  const open = useOpenEntry();
  const {api, owner, administrator} = useSession();
  const downloads = useDownloads();
  const content = useMemo<ContentRoute>(() => ({libraryId, view: view as Exclude<ContentView, 'home'>, entityId}), [libraryId, view, entityId]);
  const {snapshot, retry, refresh, next} = useContent(content);
  const changedStuck = useRefreshRequiredRetry(snapshot.phase === 'refresh-required', refresh);
  const [reordering, setReordering] = useState(false);
  useSelectionRefresh(refresh);
  // PERF-S02: section "More" appends inside its own section; the page's other
  // sections stay rendered.
  const appendedSections = snapshot.appended;
  const projection = snapshot.projection;
  const entity = projection?.entity;
  const kind = entity?.kind ?? view;
  // The library's sort and filters on what the page lists: a collection's or an author's titles, an
  // artist's albums and songs, an album's songs. The page's own list stands until one is chosen.
  const libraryKind = useLibrariesContext().items.find(l => l.id === libraryId)?.kind ?? '';
  const groups = useMemo(() => entityGroups(libraryId, libraryKind, {kind, id: entityId}), [libraryId, libraryKind, kind, entityId]);
  const choose = useCallback((patch: Record<string, string | undefined>) => void navigate({to: '/library/$libraryId/$view/$entityId', params: {libraryId, view, entityId}, search: {...address, ...patch}, replace: true, resetScroll: false}), [navigate, libraryId, view, entityId, address]);
  const titleGroup = useGroupBrowse(groups.titles, {sort: address.sort, direction: address.direction, filters: address.filters}, useCallback((next: GroupChoice) => choose({sort: next.sort, direction: next.direction, filters: next.filters}), [choose]));
  const songGroup = useGroupBrowse(groups.songs, {sort: address.songSort, direction: address.songDirection, filters: address.songFilters}, useCallback((next: GroupChoice) => choose({songSort: next.sort, songDirection: next.direction, songFilters: next.filters}), [choose]));
  const isAlbum = kind === 'album' || kind === 'disc';
  const isArtist = kind === 'artist';
  const isMusicList = isAlbum || isArtist;
  const shape = entity ? shapeFor(entity.kind) : 'square';
  const songs = appendedSections.find(s => s.id === 'songs');
  const releases = appendedSections.find(s => s.id === 'releases');
  // M24: artist Popular tracks (first page only, absent with no history) and
  // releases split client-side by role, behind presence.
  const popularTracks = appendedSections.find(s => s.id === 'popularTracks');
  const releaseEntries = releases?.entries ?? [];
  const hasReleaseRoles = releaseEntries.some(e => e.role !== undefined);
  const releaseGroups = hasReleaseRoles ? artistReleaseGroups(releaseEntries) : [];
  const playable = snapshot.sections.flatMap(x => x.entries).find(e => e.playback);
  // WEB-DETAIL-11: a book (or anything the server gives a resume point) resumes where you left off.
  const resumeEntry = entity?.playback && (entity.playback.startSeconds ?? 0) > 0 ? entity : undefined;
  const loaded = snapshot.sections.flatMap(x => x.entries).filter(e => e.playback);
  // On Playback v1 a container plays (or shuffles) as one server-snapshotted request, not the loaded page.
  // A disc page id maps to the v1 disc selector; popular tracks play their own five (no container).
  const containerOf = (shuffle: boolean) => sequenceContainer(entity?.kind, entity?.id, {shuffle});
  // Sorted or filtered, the hero's Play and Shuffle play that list (the server's query, however long).
  const sortedGroup = songGroup.active ? songGroup : titleGroup.active ? titleGroup : undefined;
  let sortedFirst: ContentEntry | undefined;
  for (let i = 0; sortedGroup?.collection && i < 50 && !sortedFirst; i++) { const item = sortedGroup.collection.itemAt(i); if (!item) break; if (item.playback) sortedFirst = item; }
  const shuffle = () => {
    if (sortedGroup && sortedFirst) { player.playSequence([sortedFirst], 0, 0, groupSequence(sortedGroup, {shuffle: true})); return; }
    if (!loaded.length) return;
    player.playSequence(loaded, 0, 0, containerOf(true));
  };
  const back = useBack(() => void navigate({to: '/library/$libraryId', params: {libraryId}, search: {}}));
  const heroShape = entity ? shapeFor(entity.kind, 'hero') : 'square';
  // M26 item 5: a collection without custom art shows a 2×2 mosaic of the
  // first 4 item posters or covers in the hero (undefined = custom art wins).
  const heroMosaic = entity && (entity.kind === 'collection' || entity.kind === 'category') ? mosaicPaths(entity, appendedSections.flatMap(s => s.entries)) : undefined;
  const music = entity?.kind === 'album' || entity?.kind === 'artist' || entity?.kind === 'collection';
  // The page's own More menu: the same actions a card of this album, artist or collection offers
  // (playlist, collection, metadata, delete for owners), anchored to the hero button.
  const entityMore = useMemo<ContentEntry | undefined>(() => (entity ? {...entity, libraryId: entity.libraryId || libraryId, navigation: entity.navigation ?? {view: view as NonNullable<ContentEntry['navigation']>['view'], entityId}} : undefined), [entity, libraryId, view, entityId]);
  const anchorOf = (el: Element): MenuAnchor => { const r = el.getBoundingClientRect(); return {x: r.left, y: r.top, width: r.width, height: r.height}; };
  const playTrack = (list: readonly ContentEntry[], index: number, container = containerOf(false)) => {
    const queue = list.slice(index).filter(e => e.playback);
    if (!queue.length) return;
    player.playSequence(queue, 0, queue[0]!.playback!.startSeconds ?? 0, container);
  };
  // A sorted or filtered song list plays on in the order shown, as the Songs tab does: the whole
  // list, from the server's query, starting at the song pressed.
  const playSorted = (index: number) => {
    const pressed = songGroup.collection?.itemAt(index);
    if (pressed?.playback) player.playSequence([pressed], 0, 0, groupSequence(songGroup));
  };
  // An album, book or artist keeps its own personal state on the container route (O(1)): the
  // watched default for a book, and Favorite for all three. Music has no "watched" at all. Other
  // kinds have no container state, so the hook idles. Every hook stays above the returns below.
  const containerKind = entity ? containerKindFor(entity.kind) : undefined;
  const containerWatched = useContainerWatched(api, containerKind, containerKind && entity ? entity.id : undefined, {title: entity?.title, onChanged: refresh});
  const watchedKind = entity?.kind === 'book' ? containerKind : undefined;
  const canFavorite = containerWatched.carries('favorite');
  // My List: a book to listen to sits beside the shows and films to watch.
  const canList = containerWatched.carries('watchlisted');
  const listed = !!containerWatched.state?.watchlisted;
  const favorite = !!containerWatched.state?.favorite;
  // M25-3: container-kind download through DownloadsService.requestContainer with
  // the policy choices from app/downloads.tsx (the dialog owns them). Only
  // where downloads are enabled; artist has no container download.
  const downloadKind = entity?.kind === 'album' || entity?.kind === 'book' ? entity.kind : undefined;
  const canDownload = !!downloadKind && !!entity && !!downloads.service && !downloads.state.unavailable;
  // Hero facts per type (Spec — Title Pages §1; WEB-DETAIL-10; M24: album year
  // from the entity projection, behind presence).
  const heroFacts: readonly (string | undefined | null | false)[] = (() => {
    if (!entity) return [];
    const wordsT = t as unknown as WordsT;
    if (isAlbum) {
      const songCount = songs?.totalCount ?? entity.count;
      return albumHeroFacts(wordsT, {type: entity.albumType, year: entity.year ? String(entity.year) : undefined, songCount, durationSeconds: entity.duration});
    }
    if (isArtist) {
      // Spec — Page Content §3: country and active years beside the counts.
      return [...artistHeroFacts(wordsT, {albumCount: releases?.totalCount, songCount: songs?.totalCount}), entity.country, activeYearsLabel(entity.activeBeginYear, entity.activeEndYear, y => t('title.activeSince', {year: y}))];
    }
    if (entity.kind === 'book') {
      // An audiobook is "finished", never "watched" (Spec — Page Content §4).
      const counts = containerWatched.state?.watchedCount !== undefined && containerWatched.state?.unwatchedCount !== undefined ? {finished: containerWatched.state.watchedCount, remaining: containerWatched.state.unwatchedCount} : undefined;
      // One part reads "Finished" or "Not started"; several read as counts.
      const finished = !counts ? undefined : counts.finished + counts.remaining <= 1 ? (counts.finished ? t('title.finished') : resumeEntry ? undefined : t('title.notStarted')) : t('title.finishedCount', counts);
      const files = appendedSections.find(section => section.entries.length > 0 && section.entries.every(e => e.kind === 'audiobook_file'));
      const length = entity.duration ?? (files && !files.nextCursor ? files.entries.reduce((sum, e) => sum + (e.duration ?? 0), 0) : 0);
      return [entity.subtitle, captionFor(entity) !== entity.subtitle ? captionFor(entity) : undefined, length ? formatDuration(length) : undefined, finished];
    }
    return [entity.subtitle, captionFor(entity) !== entity.subtitle ? captionFor(entity) : undefined, entity.duration ? formatDuration(entity.duration) : undefined];
  })();
  // M24: for an album the artist name sits above the title; when the entity
  // carries the artist id it links to the artist page, otherwise plain text.
  const albumArtist = isAlbum ? entity?.artist : undefined;
  const heroContext = isAlbum && entity ? albumArtist?.name ?? entity.subtitle : undefined;
  // WEB-DETAIL-10: a one-disc album has no Discs section; multi-disc albums
  // get "Disc N" subheadings inside the track list instead of a Discs list.
  // M24: on an artist page Popular tracks and role-split Releases are drawn
  // below, so they leave `rest` only when present (older servers without them
  // render the single Releases section as before).
  const handled = isMusicList
    ? new Set(['songs', 'discs', ...(isArtist && popularTracks ? ['popularTracks'] : []), ...(isArtist && hasReleaseRoles ? ['releases'] : [])])
    : new Set<string>();
  const rest = appendedSections.filter(s => !handled.has(s.id));
  // Details: the facts that don't belong in the hero line (label, release type, country, years).
  // Type and year are in the hero line already; they join Details only beside something the hero does not say.
  const beyondHero = !!entity && (!!entity.label || !!entity.country || (isArtist && !!entity.activeBeginYear));
  const detailRows: [string, string][] = entity && beyondHero ? ([
    entity.albumType ? [t('title.albumType'), entity.albumType] : null,
    entity.label ? [t('title.label'), entity.label] : null,
    entity.kind !== 'artist' && entity.year ? [t('title.year'), String(entity.year)] : null,
    entity.country ? [t('title.country'), entity.country] : null,
    isArtist && activeYearsLabel(entity.activeBeginYear, entity.activeEndYear) ? [t('title.activeYears'), activeYearsLabel(entity.activeBeginYear, entity.activeEndYear, y => t('title.activeSince', {year: y}))!] : null,
  ] as ([string, string] | null)[]).filter((r): r is [string, string] => !!r) : [];
  // A book in one file has no parts to list: the page is the book. Its length is then the file's.
  const parts = entity?.kind === 'book' ? appendedSections.find(section => section.entries.length > 0 && section.entries.every(e => e.kind === 'audiobook_file')) : undefined;
  const onePart = !!parts && parts.entries.length === 1 && !parts.nextCursor;
  const legacySections = !isMusicList ? appendedSections.filter(section => !(section.entries.length === 1 && section.entries[0]!.kind === 'disc') && !(onePart && section === parts)) : [];
  // The toolbar's list: a book's parts, an artist's releases, else a collection's or author's titles (the first section).
  const barredSection = kind === 'book' ? (onePart ? undefined : parts) : legacySections[0];
  const titleBar = groups.titles && snapshot.sections.length && (kind !== 'book' || barredSection) ? <Inset><GroupBar group={titleGroup} total={isArtist ? releases?.totalCount : barredSection?.totalCount} /></Inset> : null;
  // The owner arranges a library collection's own order here (only while it is shown in that order).
  const ownOrderShown = kind === 'collection' && !address.sort && !address.filters;
  const canReorder = (owner || administrator) && ownOrderShown && !!entity && (titleGroup.total ?? 0) > 1;
  const reorderList = useWholeList(reordering && canReorder ? titleGroup.collection : undefined);
  const moveTitle = async (from: number, to: number) => {
    const request = moveRequest(reorderList.entries.map(e => e.id), from, to);
    if (!request || !entity) return;
    await moveCollectionTitle(<T,>(path: string, method: 'GET' | 'PUT', body?: unknown) => api.request<T>(path, method, body), entity.id, request.entryId, request.afterEntryId);
    titleGroup.refresh?.();
  };
  return (
    <div style={{position: 'relative'}}>
        <TitleHero
          onBack={back}
          loading={!entity}
          pageKey={entity?.id}
          backdrop={entity?.backdropUrl}
          artwork={{path: entity?.posterUrl ?? entity?.backdropUrl, shape: heroShape === 'landscape' ? 'poster' : heroShape, icon: iconFor(entity?.kind ?? view), initial: heroShape === 'circle' ? entity?.title.replace(/^(the|an?)\s+/i, '').slice(0, 1) : undefined, mosaic: heroMosaic}}
          title={entity?.title ?? title ?? projection?.heading.fallback}
          context={heroContext}
          onContext={albumArtist?.id ? () => void navigate({to: '/library/$libraryId/$view/$entityId', params: {libraryId, view: 'artist', entityId: albumArtist.id!}, search: {title: albumArtist.name}}) : undefined}
          facts={heroFacts}
          synopsis={entity?.overview}
          primary={resumeEntry?.playback ? {label: t('title.resumeRemaining', {remaining: formatDuration(Math.max(0, (resumeEntry.duration ?? 0) - (resumeEntry.progressSeconds ?? 0)))}), onClick: () => player.play(resumeEntry.playback!.itemId, resumeEntry.playback!.startSeconds ?? 0, resumeEntry), progress: resumeEntry.duration ? (resumeEntry.progressSeconds ?? 0) / resumeEntry.duration : undefined}
            : sortedGroup && sortedFirst ? {label: t('action.play'), onClick: () => player.playSequence([sortedFirst!], 0, 0, groupSequence(sortedGroup))}
            : playable?.playback ? {label: t('action.play'), onClick: () => (loaded.length ? player.playSequence(loaded, Math.max(0, loaded.indexOf(playable)), playable.playback!.startSeconds ?? 0, containerOf(false)) : player.play(playable.playback!.itemId, playable.playback!.startSeconds ?? 0, playable))} : undefined}
          secondary={music && loaded.length > 1 || entityMore || watchedKind || canFavorite || canList || canDownload ? (
            <>
              {/* One row on every title (Justin, 2 Oct 2026): Play · Shuffle (music) · My List (books) · Favorite · Finished (books; music has no "watched") · Download · ⋯. */}
              {music && (loaded.length > 1 || sortedFirst) ? <Button variant="secondary" size="lg" icon="shuffle" label={t('title.shuffle')} onClick={shuffle} /> : null}
              {canList && entity ? <Button variant="secondary" size="lg" icon={listed ? 'bookmarkFilled' : 'bookmark'} label={t(listed ? 'title.inWatchlist' : 'title.watchlist')} selected={listed} disabled={!containerWatched.state} onClick={() => void containerWatched.setFlag('watchlisted', !listed)} /> : null}
              {canFavorite && entity ? <Button variant="secondary" size="lg" icon={favorite ? 'heartFilled' : 'heart'} aria-label={t(favorite ? 'entry.removeFromFavorites' : 'entry.addToFavorites')} title={t(favorite ? 'entry.removeFromFavorites' : 'entry.addToFavorites')} selected={favorite} disabled={!containerWatched.state} onClick={() => void containerWatched.setFlag('favorite', !favorite)} /> : null}
              {watchedKind && entity ? <Button variant="secondary" size="lg" icon={containerWatched.state?.watched ? 'watched' : 'check'} aria-label={t(containerWatched.state?.watched ? 'title.markNotFinished' : 'title.markFinished')} title={t(containerWatched.state?.watched ? 'title.markNotFinished' : 'title.markFinished')} selected={containerWatched.state?.watched} disabled={containerWatched.saving} onClick={() => void containerWatched.setWatched(!(containerWatched.state?.watched ?? false))} /> : null}
              {canDownload && downloadKind && entity ? <Button variant="secondary" size="lg" icon="download" aria-label={t('entry.download')} title={t('entry.download')} onClick={() => downloads.ask({target: {containerId: entity.id, containerKind: downloadKind}, title: entity.title})} /> : null}
              {entityMore ? <Button variant="secondary" size="lg" icon="more" aria-label={t('title.moreActions')} title={t('title.moreActions')} onClick={(e: React.MouseEvent<HTMLElement>) => player.more(entityMore, anchorOf(e.currentTarget), 'page')} /> : null}
            </>
          ) : undefined}
        />
      <Page>
        {snapshot.phase === 'loading' && !snapshot.sections.length ? <LoadingShelves density={shape === 'square' ? 'square' : 'poster'} /> : null}
        {snapshot.phase === 'error' && !snapshot.sections.length ? <ErrorState error={snapshot.error} context="collection" retry={retry} refresh={refresh} /> : null}
        {changedStuck ? <Inset><ErrorNotice error={snapshot.error ?? changedError} context="collection" refresh={refresh} /></Inset> : null}
        {/* Entity sections sit above the backdrop scrim (Detail `.body` rule). */}
        <div className={body.body}>
          {/* The five most played first ("Popular" when the owner shares activity across profiles, "Your most played" otherwise), then every song, then the releases by type. */}
          {/* PERF-S06: the top-5 list plays its own five as an item list, never the artist container. */}
          {isArtist && popularTracks && popularTracks.entries.length ? (
            <Section title={popularTracks.heading.fallback}>
              <MusicTracks
                kind="artist"
                entries={popularTracks.entries}
                onPlay={index => playTrack(popularTracks.entries, index, undefined)}
                onMore={(entry, el) => open(entry, 'more', libraryId, anchorOf(el))}
              />
            </Section>
          ) : null}
          {isMusicList && songs?.entries.length ? (
            <ArtistSongsHeading show={isArtist} title={t('title.tracks')}>
            <Inset><GroupBar group={songGroup} total={songs.totalCount} /></Inset>
            {songGroup.active ? (
              <GroupRows
                group={songGroup}
                label={t('title.tracks')}
                context="collection"
                estimateRowHeight={64}
                renderItem={(entry, index) => <TrackRow kind={isArtist ? 'artist' : 'album'} albumArtist={isAlbum ? entity?.subtitle : undefined} entry={entry} index={index} onPlay={playSorted} onMore={(e, el) => open(e, 'more', libraryId, anchorOf(el))} />}
                renderPlaceholder={() => <div style={{height: 64, padding: '8px 16px'}}><Skeleton height={48} /></div>}
              />
            ) : (
            <MusicTracks
              kind={isArtist ? 'artist' : 'album'}
              albumArtist={isAlbum ? entity?.subtitle : undefined}
              entries={songs.entries}
              onPlay={index => playTrack(songs.entries, index)}
              onMore={(entry, el) => open(entry, 'more', libraryId, anchorOf(el))}
              onShowMore={songs.nextCursor ? () => next(songs.id) : undefined}
            />
            )}
            </ArtistSongsHeading>
          ) : null}
          {/* An artist's albums, a collection's or an author's titles: the library's toolbar under their heading, and the library's own grid once a sort or filter is chosen. */}
          {titleGroup.active ? (
            <Section title={isArtist ? t('title.albums') : barredSection ? sectionHeading(i18n, barredSection.heading) : undefined}>
              {canReorder || reordering ? <Inset><Button variant={reordering ? 'primary' : 'secondary'} icon={reordering ? 'check' : 'list'} label={reordering ? t('order.done') : t('order.reorder')} aria-pressed={reordering} onClick={() => setReordering(on => !on)} /></Inset> : null}
              {reordering && canReorder ? null : titleBar}
              {reordering && canReorder ? <Inset><Surface padless><ManualOrderList entries={reorderList.entries} whole={reorderList.whole} label={entity?.title ?? ''} onMove={moveTitle} /></Surface></Inset> : <GroupGrid group={titleGroup} square={libraryKind === 'music' || libraryKind === 'audiobook'} />}
            </Section>
          ) : null}
          {/* Releases by type: Albums, Singles & EPs, Compilations, Appears on (client-core `artistReleaseGroups`). */}
          {isArtist && hasReleaseRoles && releases && !titleGroup.active ? releaseGroups.map((group, index) => {
            const last = index === releaseGroups.length - 1;
            const barred = index === 0 && !!titleBar;
            const view = (
              <SectionView
                key={group.id}
                section={{...releases, id: group.id, heading: {key: group.label, fallback: t(group.label)}, entries: group.entries, totalCount: group.entries.length, nextCursor: last ? releases.nextCursor : ''}}
                libraryId={libraryId}
                hideTitle={barred}
                loadingMore={snapshot.phase === 'loading'}
                onMore={last && releases.nextCursor ? () => next(releases.id) : undefined}
              />
            );
            return barred ? <Section key={group.id} title={t(group.label)}>{titleBar}{view}</Section> : view;
          }) : null}
          {(isMusicList ? rest.filter(section => !(titleGroup.active && section.id === 'releases')) : titleGroup.active ? legacySections.filter(section => kind === 'book' && section !== barredSection) : legacySections).map(section => {
            // The list the toolbar is for: an artist's undivided releases, a book's parts, or a collection's or author's titles.
            const barred = !!titleBar && (isArtist ? section.id === 'releases' : !isMusicList && section === barredSection);
            const view = <SectionView key={section.id} section={section} libraryId={libraryId} hideTitle={barred} loadingMore={snapshot.phase === 'loading'} onMore={section.nextCursor ? () => next(section.id) : undefined} />;
            return barred ? <Section key={section.id} title={sectionHeading(i18n, section.heading)}>{titleBar}{view}</Section> : view;
          })}
        </div>
        {entity && (detailRows.length || entity.overviewSource) ? (
          <Section title={t('title.details')}>
            <div style={{padding: '0 var(--page-gutter)', display: 'flex', flexDirection: 'column', gap: 12}}>
              {detailRows.length ? <KeyValue rows={detailRows} /> : null}
              {entity.overviewSource ? <Text variant="caption" tone="tertiary">{entity.overviewSourceUrl ? <a href={entity.overviewSourceUrl} target="_blank" rel="noreferrer">{t('title.fromSource', {source: entity.overviewSource})}</a> : t('title.fromSource', {source: entity.overviewSource})}</Text> : null}
            </div>
          </Section>
        ) : null}
        {snapshot.phase === 'ready' && !snapshot.sections.length ? <StateView icon={iconFor(entity?.kind ?? '')} title={t('web.entity.emptyTitle')} body={t('web.entity.emptyBody')} /> : null}
      </Page>
    </div>
  );
}

/** An artist's song list is headed "Songs"; an album's tracks are the page itself and need no heading. */
function ArtistSongsHeading({show, title, children}: {show: boolean; title: string; children: React.ReactNode}) {
  return show ? <Section title={title}>{children}</Section> : <>{children}</>;
}

/**
 * Numbered track rows: number, title, duration, ⋯. The subtitle appears only
 * for guest artists; multi-disc albums group rows under "Disc N" subheadings
 * (never "Unnumbered disc"). Artist rows show square art instead of numbers.
 */
function MusicTracks({kind, albumArtist, entries, onPlay, onMore, onShowMore}: {
  kind: 'album' | 'artist';
  albumArtist?: string;
  entries: readonly ContentEntry[];
  onPlay: (index: number) => void;
  onMore: (entry: ContentEntry, el: Element) => void;
  onShowMore?: () => void;
}) {
  const {t} = useI18n();
  const wordsT = t as unknown as WordsT;
  if (!entries.length) return null;
  const row = (entry: ContentEntry, index: number) => <TrackRow key={`${entry.id}:${index}`} kind={kind} albumArtist={albumArtist} entry={entry} index={index} onPlay={onPlay} onMore={onMore} />;
  // Multi-disc albums group rows under one Section per disc ("Disc N"); a
  // single disc — or tracks without a disc number — renders with no heading
  // (never "Unnumbered disc").
  if (kind === 'album') {
    const groups = discGroups(entries);
    if (groups) {
      return (
        <div>
          {[...groups.entries()].map(([disc, items]) =>
            disc > 0 ? (
              <Section key={`disc-${disc}`} title={wordsT('title.disc', {number: disc})}>
                {items.map(({entry, index}) => row(entry, index))}
              </Section>
            ) : (
              <div key="disc-0">{items.map(({entry, index}) => row(entry, index))}</div>
            ),
          )}
          {onShowMore ? (
            <div style={{display: 'flex', justifyContent: 'center', padding: '12px 0'}}>
              <Button variant="secondary" size="sm" label={t('action.showMore')} onClick={onShowMore} />
            </div>
          ) : null}
        </div>
      );
    }
  }
  return (
    <div>
      {entries.map((entry, index) => row(entry, index))}
      {onShowMore ? (
        <div style={{display: 'flex', justifyContent: 'center', padding: '12px 0'}}>
          <Button variant="secondary" size="sm" label={t('action.showMore')} onClick={onShowMore} />
        </div>
      ) : null}
    </div>
  );
}

/** One track: its number on an album (square art on an artist's page), title, a guest artist, duration and ⋯. The playing track is marked. */
function TrackRow({kind, albumArtist, entry, index, onPlay, onMore}: {kind: 'album' | 'artist'; albumArtist?: string; entry: ContentEntry; index: number; onPlay: (index: number) => void; onMore: (entry: ContentEntry, el: Element) => void}) {
  const {t} = useI18n();
  const playingId = usePlayingItemId();
  const subtitle = kind === 'album' ? trackSubtitleForAlbum(entry.subtitle, albumArtist) : undefined;
  return (
    <ListRow
      index={kind === 'album' ? entry.trackNumber ?? index + 1 : undefined}
      title={entry.title}
      subtitle={subtitle}
      meta={entry.duration ? formatClock(entry.duration) : undefined}
      art={kind === 'artist' ? {path: entry.posterUrl ?? entry.backdropUrl, icon: !entry.posterUrl && !entry.backdropUrl ? iconFor(entry.kind) : undefined} : undefined}
      artShape="square"
      selected={!!playingId && playingId === (entry.playback?.itemId ?? entry.id)}
      onClick={entry.playback ? () => onPlay(index) : undefined}
      actions={entry.navigation ? <Button variant="ghost" size="sm" icon="more" aria-label={t('card.more', {title: entry.title})} onClick={(e: React.MouseEvent<HTMLElement>) => onMore(entry, e.currentTarget)} /> : undefined}
    />
  );
}
