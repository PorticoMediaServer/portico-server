import React from 'react';
import {Link} from '@tanstack/react-router';
import {useEffect, useMemo, useState} from 'react';
import type {ContentEntry} from '@core/library-content.ts';
import {formatDuration} from '@core/presentation/index.ts';
import {useCreditPages, useDetail} from '../../app/detail';
import {ErrorNotice} from '../../app/errors';
import {usePlayerActions} from '../../player/PlayerContext';
import {Button, KeyValue, Loading, Notice, Text, anchorOf, cx} from '../../ui';
import {episodeCode, formatAirDate} from './EpisodeCard';
import {entryOf} from './Detail';
import {useSession} from '../../app/session';
import {DeleteVersionDialog} from './DeleteVersion';
import type {TitleFile} from '@core/detail.ts';
import {TitleFiles} from './TitleFiles';
import {PlaybackChoices, type PlaybackChoice} from './PlaybackChoices';
import {setPendingTrackChoice} from '../../player/pending-choice';
import s from './Show.module.css';
import d from './Detail.module.css';
import {useI18n} from '../../app/i18n';

/**
 * An episode opened in place (Spec — Title Pages §3, Justin 18 Oct 2026): a small title page under
 * its row in the list, or under its row of cards in the grid, on the page's own background —
 * never a layer or a box of its own. The show and season, the episode's code and title, aired ·
 * runtime · rating, the whole synopsis, pick before play, Play/Resume, Play from start, Mark
 * watched and ⋯ (download, playlist); beside them on wide screens the guest stars, director and
 * writers, and the files behind a disclosure.
 */
export function EpisodeDetails({libraryId, episodeId, preview, playList, showTitle, heading = true}: {
  libraryId: string;
  episodeId: string;
  /** The show, named above the episode ("Breaking Bad · Season 2"). */
  showTitle?: string;
  /** The episode's code and title as a heading; off in the list, where its own row says them just above. */
  heading?: boolean;
  /** The row's entry, so the block isn't empty while the detail loads. */
  preview?: ContentEntry;
  /** Plays within the season's resident episodes so Up Next continues in order. */
  playList?: (episode: ContentEntry, startSeconds: number) => void;
}) {
  const i18n = useI18n();
  const {t} = i18n;
  const player = usePlayerActions();
  const target = useMemo(() => ({libraryId, itemId: episodeId}), [libraryId, episodeId]);
  const {snapshot, mutate, retry} = useDetail(target);
  const data = snapshot.data;
  const item = data?.item;
  const {owner} = useSession();
  const [deletingVersion, setDeletingVersion] = React.useState<{file: TitleFile; summary: string}>();
  const title = item?.title ?? preview?.title ?? t('title.episodeFallback');
  const actions = data?.actions.filter(a => a.enabled) ?? [];
  const play = actions.find(a => a.playback && a.id !== 'start_over');
  const restart = actions.find(a => a.id === 'start_over');
  const canWatched = actions.some(a => a.id === 'watched');
  const resume = !!item && item.progressSeconds > 0 && (play?.playback?.startSeconds ?? 0) > 0;
  const credits = data?.metadata.credits ?? [];
  const byRole = (test: (department: string, role: string) => boolean) => credits.filter(c => test(c.department.toLowerCase(), c.role.toLowerCase()));
  const guests = byRole(d => d === 'acting' || d === 'cast');
  const directors = byRole((d, r) => d === 'directing' || r === 'director');
  const writers = byRole((d, r) => d === 'writing' || r.includes('writer'));
  // The cast line names the first twelve; "All cast" lists every credit,
  // paging the rest from the server (nothing is cut off).
  const castTotal = data?.metadata.creditTotals?.cast ?? guests.length;
  const castPages = useCreditPages(data?.item.id, 'cast', guests, castTotal);
  const [allCast, setAllCast] = useState(false);
  const [choice, setChoice] = useState<PlaybackChoice>({});
  useEffect(() => { setAllCast(false); setChoice({}); }, [episodeId]);
  const start = (seconds: number) => {
    if (!item || !play?.playback) return;
    const entry = preview ?? entryOf(item);
    // Pick-before-play travels like the movie page's: tracks through pending-choice, the version with Play.
    setPendingTrackChoice(choice.audioStreamIndex !== undefined || choice.subtitle !== undefined ? {itemId: play.playback.itemId, audioStreamIndex: choice.audioStreamIndex, subtitle: choice.subtitle} : undefined);
    // An audio pick alone keeps the season queue: the track is applied to the session once it starts.
    if (playList && !choice.prepared && !choice.quality && !choice.choices?.versionId && !choice.choices?.quality) playList(entry, seconds);
    else player.play(play.playback.itemId, seconds, entry, choice.prepared, choice.quality, choice.choices);
  };
  /* Guest stars, directors and writers, each a link to the person's page where the server knows
     who they are (B-18.30). One entry per person even when the server lists them in two roles
     (writer and teleplay, say). */
  const people = (list: typeof credits, withRole = false, all = false) => {
    const unique = [...new Map(list.map(c => [c.personId ?? c.name, c])).values()].slice(0, all ? undefined : 12);
    return (
      <span>
        {unique.map((c, i) => {
          const label = withRole && c.role ? `${c.name} (${c.role})` : c.name;
          return (
            <React.Fragment key={c.personId ?? c.name}>
              {i ? ', ' : null}
              {c.personId ? <Link className={d.genreLink} to="/person/$personId" params={{personId: c.personId}}>{label}</Link> : label}
            </React.Fragment>
          );
        })}
      </span>
    );
  };
  const rows = [
    guests.length ? [t('title.cast'), allCast ? people(castPages.credits, true, true) : people(guests, true)] : null,
    directors.length ? [t('title.directedByLabel'), people(directors)] : null,
    writers.length ? [t('title.writtenByLabel'), people(writers)] : null,
  ].filter((r): r is [string, React.ReactElement] => !!r);
  const overview = item?.overview ?? preview?.overview;
  const numbers = item?.episode ? {seasonNumber: item.episode.seasonNumber ?? undefined, episodeNumber: item.episode.number} : preview ?? {};
  const code = episodeCode(t, numbers);
  const seasonNumber = item?.episode?.seasonNumber ?? preview?.seasonNumber;
  const context = [showTitle, seasonNumber === 0 ? t('title.specials') : seasonNumber != null ? t('title.season', {number: seasonNumber}) : undefined].filter(Boolean).join(' · ');
  const airDate = item?.episode?.airDate || preview?.airDate;
  const duration = item?.duration ?? preview?.duration;
  // Under its list row the row itself says when it aired and how long it runs; only the rating is added.
  const facts = heading ? [airDate ? t('title.airedOn', {date: formatAirDate(i18n, airDate)}) : undefined, duration ? formatDuration(duration) : undefined].filter(Boolean) : [];
  const rating = data?.facts?.contentRating;
  const side = rows.length || castTotal > 12 || !!data?.files?.length;
  return (
    <div className={cx(s.details, side && s.detailsWide)} aria-label={[code, title].filter(Boolean).join(', ')} role="region">
      <div className={s.detailsMain}>
        {context ? <span className={s.detailsContext}>{context}</span> : null}
        {heading ? <h3 className={s.detailsTitle}>{code ? <span className={s.detailsCode}>{code}</span> : null}<span>{title}</span></h3> : null}
        {facts.length || rating ? (
          <div className={s.detailsFacts}>
            {facts.map((f, i) => <span key={i}>{f}</span>)}
            {rating ? <span className={s.detailsRating}>{rating}</span> : null}
          </div>
        ) : null}
        {snapshot.phase === 'error' && !data ? <ErrorNotice error={snapshot.error} context="detail" retry={retry} /> : null}
        {!data && snapshot.phase !== 'error' ? <Loading label={t('title.loadingEpisode', {title})} /> : null}
        {overview ? <Text as="p" variant="body" tone="secondary" className={s.detailsSynopsis}>{overview}</Text> : null}
        {item && play?.playback ? <PlaybackChoices compact libraryId={libraryId} itemId={play.playback.itemId} onChange={setChoice} /> : null}
        {item ? (
          <div className={s.detailsActions}>
          {play?.playback ? <Button variant="primary" icon="play" label={resume ? t('title.resumeRemaining', {remaining: formatDuration(item.duration - item.progressSeconds)}) : t('action.play')} onClick={() => start(play.playback!.startSeconds ?? 0)} /> : null}
          {restart?.playback && resume ? <Button variant="secondary" icon="restart" label={t('title.playFromStart')} onClick={() => start(0)} /> : null}
          {canWatched ? <Button variant="secondary" icon={data?.personal.watched ? 'watched' : 'check'} label={t(data?.personal.watched ? 'title.markUnwatched' : 'title.markWatched')} selected={data?.personal.watched} onClick={() => mutate({action: 'watched', value: !data?.personal.watched})} /> : null}
          <Button variant="secondary" icon="more" aria-label={t('title.moreActions')} title={t('title.moreActions')} onClick={(e: React.MouseEvent<HTMLElement>) => player.more(preview ?? entryOf(item), anchorOf(e.currentTarget))} />
          </div>
        ) : null}
        {snapshot.mutationError ? <ErrorNotice error={snapshot.mutationError} context="detail" operation="save" /> : null}
        {data?.missingSince ? <Notice tone="warning" compact title={i18n.t('title.missing')}>{i18n.t('title.missingSince', {when: i18n.date(data.missingSince, 'long')})}</Notice> : null}
      </div>
      {side ? (
        <div className={s.detailsSide}>
          {rows.length ? <KeyValue rows={rows} /> : null}
          {castTotal > 12 && !(allCast && castPages.done) ? <div><Button variant="link" label={t(allCast ? 'title.moreCast' : 'title.allCast')} onClick={() => { setAllCast(true); castPages.more(); }} /></div> : null}
          {data?.files?.length ? (
            <details className={s.detailsFiles}>
              <summary>{t('title.files')}</summary>
              <TitleFiles heading={false} files={data.files} onDelete={owner && item ? (file, summary) => setDeletingVersion({file, summary}) : undefined} />
            </details>
          ) : null}
        </div>
      ) : null}
      {deletingVersion && item ? <DeleteVersionDialog itemId={item.id} title={item.title} file={deletingVersion.file} summary={deletingVersion.summary} onClose={() => setDeletingVersion(undefined)} onDeleted={() => { setDeletingVersion(undefined); void retry(); }} /> : null}
    </div>
  );
}
