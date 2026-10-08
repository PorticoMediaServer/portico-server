import {useSelectionRefresh} from '../../app/selection';
import React, {useMemo} from 'react';
import {useNavigate, useParams, useSearch} from '@tanstack/react-router';
import type {ContentEntry, ContentRoute, ContentView} from '@core/library-content.ts';
import {useContent} from '../../app/content';
import {currentI18n} from '../../app/i18n';
import {usePlayingItemId} from '../../player/engine';
import {activeYearsLabel, artistReleaseGroups, captionFor, formatClock, formatDuration, iconFor, shapeFor} from '@core/presentation/index.ts';
import {mosaicPaths} from '../../app/title-layout';
import {albumHeroFacts, artistHeroFacts, discNumberFromSubtitle, trackSubtitleForAlbum, type T as WordsT} from '../../app/library-words';
import {useOpenEntry} from '../../app/open';
import {sequenceContainer} from '../../app/container-play';
import {usePlayerActions} from '../../player/PlayerContext';
import {useSession} from '../../app/session';
import {useDownloads} from '../../app/downloads';
import {containerKindFor, useContainerWatched} from '../shared/container-watched';
import {Button, Inset, KeyValue, ListRow, Page, Section, StateView, Text, TitleHero, type MenuAnchor} from '../../ui';
import {LoadingShelves, SectionView, useAppendedSections} from '../shared/Sections';
import body from '../detail/Detail.module.css';
import {ErrorNotice, ErrorState, changedError} from '../../app/errors';

/**
 * Generic entity workspace: artist, album, collection, category, author,
 * series, disc. The server describes the entity and its sections; the hero
 * adapts its artwork geometry to the entity kind (album square, artist
 * circle). Album and artist track lists are drawn here (WEB-DETAIL-10):
 * numbered rows with duration and ⋯, the subtitle only for guest artists,
 * and "Disc N" subheadings inside the list for multi-disc albums.
 */
export function EntityScreen() {
  const {libraryId, view, entityId} = useParams({from: '/app/library/$libraryId/$view/$entityId'});
  const {title} = useSearch({from: '/app/library/$libraryId/$view/$entityId'});
  const navigate = useNavigate();
  const player = usePlayerActions();
  const open = useOpenEntry();
  const {api} = useSession();
  const downloads = useDownloads();
  const content = useMemo<ContentRoute>(() => ({libraryId, view: view as Exclude<ContentView, 'home'>, entityId}), [libraryId, view, entityId]);
  const {snapshot, retry, refresh, next} = useContent(content);
  useSelectionRefresh(refresh);
  // PERF-S02: section "More" appends inside its own section; the page's other
  // sections stay rendered.
  const appendedSections = useAppendedSections(JSON.stringify(content), snapshot.pagination.cursor, snapshot.sections);
  const t = currentI18n().t;
  const projection = snapshot.projection;
  const entity = projection?.entity;
  const kind = entity?.kind ?? view;
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
  const shuffle = () => {
    if (!loaded.length) return;
    player.playSequence(loaded, 0, 0, containerOf(true));
  };
  const back = () => (history.length > 1 ? history.back() : void navigate({to: '/library/$libraryId', params: {libraryId}, search: {}}));
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
  // An album, book or artist keeps its own personal state on the container route (O(1)): the
  // watched default for a book, and Favorite for all three. Music has no "watched" at all. Other
  // kinds have no container state, so the hook idles. Every hook stays above the returns below.
  const containerKind = entity ? containerKindFor(entity.kind) : undefined;
  const containerWatched = useContainerWatched(api, containerKind, containerKind && entity ? entity.id : undefined);
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
  return (
    <div style={{position: 'relative'}}>
        <TitleHero
          onBack={back}
          loading={!entity}
          backdrop={entity?.backdropUrl}
          artwork={{path: entity?.posterUrl ?? entity?.backdropUrl, shape: heroShape === 'landscape' ? 'poster' : heroShape, icon: iconFor(entity?.kind ?? view), initial: heroShape === 'circle' ? entity?.title.replace(/^(the|an?)\s+/i, '').slice(0, 1) : undefined, mosaic: heroMosaic}}
          title={entity?.title ?? title ?? projection?.heading.fallback}
          context={heroContext}
          onContext={albumArtist?.id ? () => void navigate({to: '/library/$libraryId/$view/$entityId', params: {libraryId, view: 'artist', entityId: albumArtist.id!}, search: {title: albumArtist.name}}) : undefined}
          facts={heroFacts}
          synopsis={entity?.overview}
          primary={resumeEntry?.playback ? {label: currentI18n().t('title.resumeRemaining', {remaining: formatDuration(Math.max(0, (resumeEntry.duration ?? 0) - (resumeEntry.progressSeconds ?? 0)))}), onClick: () => player.play(resumeEntry.playback!.itemId, resumeEntry.playback!.startSeconds ?? 0, resumeEntry), progress: resumeEntry.duration ? (resumeEntry.progressSeconds ?? 0) / resumeEntry.duration : undefined}
            : playable?.playback ? {label: currentI18n().t('action.play'), onClick: () => (loaded.length ? player.playSequence(loaded, Math.max(0, loaded.indexOf(playable)), playable.playback!.startSeconds ?? 0, containerOf(false)) : player.play(playable.playback!.itemId, playable.playback!.startSeconds ?? 0, playable))} : undefined}
          secondary={music && loaded.length > 1 || entityMore || watchedKind || canFavorite || canList || canDownload ? (
            <>
              {/* One row on every title (Justin, 2 Oct 2026): Play · Shuffle (music) · My List (books) · Favorite · Finished (books; music has no "watched") · Download · ⋯. */}
              {music && loaded.length > 1 ? <Button variant="secondary" size="lg" icon="shuffle" label={t('title.shuffle')} onClick={shuffle} /> : null}
              {canList && entity ? <Button variant="secondary" size="lg" icon={listed ? 'bookmarkFilled' : 'bookmark'} label={t(listed ? 'title.inWatchlist' : 'title.watchlist')} selected={listed} disabled={!containerWatched.state} onClick={() => void containerWatched.setFlag('watchlisted', !listed)} /> : null}
              {canFavorite && entity ? <Button variant="secondary" size="lg" icon={favorite ? 'heartFilled' : 'heart'} aria-label={t(favorite ? 'entry.removeFromFavorites' : 'entry.addToFavorites')} title={t(favorite ? 'entry.removeFromFavorites' : 'entry.addToFavorites')} selected={favorite} disabled={!containerWatched.state} onClick={() => void containerWatched.setFlag('favorite', !favorite)} /> : null}
              {watchedKind && entity ? <Button variant="secondary" size="lg" icon={containerWatched.state?.watched ? 'watched' : 'check'} aria-label={t(containerWatched.state?.watched ? 'title.markNotFinished' : 'title.markFinished')} title={t(containerWatched.state?.watched ? 'title.markNotFinished' : 'title.markFinished')} selected={containerWatched.state?.watched} disabled={containerWatched.saving} onClick={() => void containerWatched.setWatched(!(containerWatched.state?.watched ?? false)).then(done => { if (done) retry(); })} /> : null}
              {canDownload && downloadKind && entity ? <Button variant="secondary" size="lg" icon="download" aria-label={t('entry.download')} title={t('entry.download')} onClick={() => downloads.ask({target: {containerId: entity.id, containerKind: downloadKind}, title: entity.title})} /> : null}
              {entityMore ? <Button variant="secondary" size="lg" icon="more" aria-label={currentI18n().t('title.moreActions')} title={currentI18n().t('title.moreActions')} onClick={(e: React.MouseEvent<HTMLElement>) => player.more(entityMore, anchorOf(e.currentTarget), 'page')} /> : null}
            </>
          ) : undefined}
        />
      <Page>
        {snapshot.phase === 'loading' && !snapshot.sections.length ? <LoadingShelves density={shape === 'square' ? 'square' : 'poster'} /> : null}
        {snapshot.phase === 'error' && !snapshot.sections.length ? <ErrorState error={snapshot.error} context="collection" retry={retry} refresh={refresh} /> : null}
        {snapshot.phase === 'refresh-required' ? <Inset><ErrorNotice error={snapshot.error ?? changedError} context="collection" refresh={refresh} /></Inset> : null}
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
            <MusicTracks
              kind={isArtist ? 'artist' : 'album'}
              albumArtist={isAlbum ? entity?.subtitle : undefined}
              entries={songs.entries}
              onPlay={index => playTrack(songs.entries, index)}
              onMore={(entry, el) => open(entry, 'more', libraryId, anchorOf(el))}
              onShowMore={songs.nextCursor ? () => next(songs.id) : undefined}
            />
            </ArtistSongsHeading>
          ) : null}
          {/* Releases by type: Albums, Singles & EPs, Compilations, Appears on (client-core `artistReleaseGroups`). */}
          {isArtist && hasReleaseRoles && releases ? releaseGroups.map((group, index) => {
            const last = index === releaseGroups.length - 1;
            return (
              <SectionView
                key={group.id}
                section={{...releases, id: group.id, heading: {key: group.label, fallback: currentI18n().t(group.label)}, entries: group.entries, totalCount: group.entries.length, nextCursor: last ? releases.nextCursor : ''}}
                libraryId={libraryId}
                loadingMore={snapshot.phase === 'loading'}
                onMore={last && releases.nextCursor ? () => next(releases.id) : undefined}
              />
            );
          }) : null}
          {isMusicList
            ? rest.map(section => <SectionView key={section.id} section={section} libraryId={libraryId} loadingMore={snapshot.phase === 'loading'} onMore={section.nextCursor ? () => next(section.id) : undefined} />)
            : legacySections.map(section => <SectionView key={section.id} section={section} libraryId={libraryId} loadingMore={snapshot.phase === 'loading'} onMore={section.nextCursor ? () => next(section.id) : undefined} />)}
        </div>
        {entity && (detailRows.length || entity.overviewSource) ? (
          <Section title={t('title.details')}>
            <div style={{padding: '0 var(--page-gutter)', display: 'flex', flexDirection: 'column', gap: 12}}>
              {detailRows.length ? <KeyValue rows={detailRows} /> : null}
              {entity.overviewSource ? <Text variant="caption" tone="tertiary">{entity.overviewSourceUrl ? <a href={entity.overviewSourceUrl} target="_blank" rel="noreferrer">{t('title.fromSource', {source: entity.overviewSource})}</a> : t('title.fromSource', {source: entity.overviewSource})}</Text> : null}
            </div>
          </Section>
        ) : null}
        {snapshot.phase === 'ready' && !snapshot.sections.length ? <StateView icon={iconFor(entity?.kind ?? '')} title={currentI18n().t('web.entity.emptyTitle')} body={currentI18n().t('web.entity.emptyBody')} /> : null}
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
  const t = currentI18n().t;
  const wordsT = t as unknown as WordsT;
  // The playing track is marked in its list.
  const playingId = usePlayingItemId();
  if (!entries.length) return null;
  const row = (entry: ContentEntry, index: number) => {
    const subtitle = kind === 'album' ? trackSubtitleForAlbum(entry.subtitle, albumArtist) : undefined;
    return (
      <ListRow
        key={`${entry.id}:${index}`}
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
  };
  // Multi-disc albums group rows under one Section per disc ("Disc N"); a
  // single disc — or tracks without a disc number — renders with no heading
  // (never "Unnumbered disc").
  if (kind === 'album') {
    const discs = entries.map(e => discNumberFromSubtitle(e.subtitle) ?? 0);
    if (new Set(discs).size > 1) {
      const groups = new Map<number, Array<{entry: ContentEntry; index: number}>>();
      entries.forEach((entry, index) => {
        const disc = discNumberFromSubtitle(entry.subtitle) ?? 0;
        const list = groups.get(disc) ?? [];
        list.push({entry, index});
        groups.set(disc, list);
      });
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
