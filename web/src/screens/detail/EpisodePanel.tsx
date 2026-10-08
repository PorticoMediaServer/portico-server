import {useEffect, useMemo, useState} from 'react';
import type {ContentEntry} from '@core/library-content.ts';
import {formatDuration} from '@core/presentation/index.ts';
import {useCreditPages, useDetail} from '../../app/detail';
import {episodeArt, placeholderTint} from '../../app/title-layout';
import {ErrorNotice} from '../../app/errors';
import {usePlayerActions} from '../../player/PlayerContext';
import {Artwork, Button, Dialog, KeyValue, Loading, Text, anchorOf} from '../../ui';
import {entryOf} from './Detail';
import {TitleFiles} from './TitleFiles';
import {episodeCode, formatAirDate} from './EpisodeCard';
import {PlaybackChoices, type PlaybackChoice} from './PlaybackChoices';
import {setPendingTrackChoice} from '../../player/pending-choice';
import s from './Show.module.css';
import {currentI18n} from '../../app/i18n';

/**
 * The episode view (Spec — Title Pages §3): a panel over the show page, never
 * a destination with its own hero. Still, S2 E4 · title, runtime, synopsis,
 * guest stars, director and writers, pick before play, Play/Resume, Play
 * from beginning, Mark watched, ⋯ (download, playlist), the episode's files,
 * and the previous and next episodes.
 */
export function EpisodePanel({libraryId, showId, episodeId, preview, previous, next, onOpenEpisode, onClose, playList}: {
  libraryId: string;
  /** Tints the placeholder when the episode has no still. */
  showId: string;
  episodeId: string;
  /** The card's entry, so the panel isn't empty while the detail loads. */
  preview?: ContentEntry;
  previous?: ContentEntry;
  next?: ContentEntry;
  onOpenEpisode: (episode: ContentEntry) => void;
  onClose: () => void;
  /** Plays within the season's resident episodes so Up Next continues in order. */
  playList?: (episode: ContentEntry, startSeconds: number) => void;
}) {
  const player = usePlayerActions();
  const target = useMemo(() => ({libraryId, itemId: episodeId}), [libraryId, episodeId]);
  const {snapshot, mutate, retry} = useDetail(target);
  const data = snapshot.data;
  const item = data?.item;
  const title = item?.title ?? preview?.title ?? currentI18n().t('title.episodeFallback');
  const code = item?.episode ? episodeCode({seasonNumber: item.episode.seasonNumber ?? undefined, episodeNumber: item.episode.number}) : preview ? episodeCode(preview) : '';
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
    // The panel's pick-before-play travels like the movie page's: tracks through pending-choice, the version with Play.
    setPendingTrackChoice(choice.audioStreamIndex !== undefined || choice.subtitle !== undefined ? {itemId: play.playback.itemId, audioStreamIndex: choice.audioStreamIndex, subtitle: choice.subtitle} : undefined);
    if (playList && !choice.prepared && !choice.quality) playList(entry, seconds);
    else player.play(play.playback.itemId, seconds, entry, choice.prepared, choice.quality);
    onClose();
  };
  // One entry per person even when the server lists them in two roles (writer and teleplay, say).
  const names = (list: typeof credits, withRole = false, all = false) => [...new Map(list.map(c => [c.personId ?? c.name, withRole && c.role ? `${c.name} (${c.role})` : c.name])).values()].slice(0, all ? undefined : 12).join(', ');
  // M26: the episode's own still, else the styled placeholder — never the show backdrop.
  const still = episodeArt({stillUrl: item?.stillUrl ?? preview?.stillUrl});
  const rows = [guests.length ? [currentI18n().t('title.cast'), allCast ? names(castPages.credits, true, true) : names(guests, true)] : null].filter((r): r is [string, string] => !!r);
  const crewLines = [directors.length ? currentI18n().t('title.directedBy', {names: names(directors)}) : null, writers.length ? currentI18n().t('title.writtenBy', {names: names(writers)}) : null].filter((l): l is string => !!l);
  return (
    <Dialog open onOpenChange={open => !open && onClose()} title={title} description={[code, item?.duration ? formatDuration(item.duration) : preview?.duration ? formatDuration(preview.duration) : undefined, preview?.airDate ? currentI18n().t('title.airedOn', {date: formatAirDate(preview.airDate)}) : undefined].filter(Boolean).join(' · ') || undefined} placement="side" width={480}>
      {still.kind === 'still' ? <Artwork path={still.path} shape="landscape" icon="tv" className={s.panelStill} alt="" progress={item && item.duration ? item.progressSeconds / item.duration : undefined} /> : <div className={s.panelStillPlaceholder} style={placeholderTint(showId)} aria-hidden><span>{code}</span></div>}
      {snapshot.phase === 'error' && !data ? <ErrorNotice error={snapshot.error} context="detail" retry={retry} /> : null}
      {!data && snapshot.phase !== 'error' ? <Loading label={currentI18n().t('title.loadingEpisode', {title})} /> : null}
      {item?.overview ?? preview?.overview ? <Text as="p" variant="body" tone="secondary">{item?.overview ?? preview?.overview}</Text> : null}
      {item && play?.playback ? <PlaybackChoices compact libraryId={libraryId} itemId={play.playback.itemId} onChange={setChoice} /> : null}
      {item ? (
        <div style={{display: 'flex', flexWrap: 'wrap', gap: 8}}>
          {play?.playback ? <Button variant="primary" icon="play" label={resume ? currentI18n().t('title.resumeRemaining', {remaining: formatDuration(item.duration - item.progressSeconds)}) : currentI18n().t('action.play')} onClick={() => start(play.playback!.startSeconds ?? 0)} /> : null}
          {restart?.playback && resume ? <Button variant="secondary" icon="restart" label={currentI18n().t('title.playFromStart')} onClick={() => start(0)} /> : null}
          {canWatched ? <Button variant="secondary" icon={data?.personal.watched ? 'watched' : 'check'} label={currentI18n().t(data?.personal.watched ? 'title.markUnwatched' : 'title.markWatched')} selected={data?.personal.watched} onClick={() => mutate({action: 'watched', value: !data?.personal.watched})} /> : null}
          <Button variant="secondary" icon="more" aria-label={currentI18n().t('title.moreActions')} title={currentI18n().t('title.moreActions')} onClick={(e: React.MouseEvent<HTMLElement>) => player.more(preview ?? entryOf(item), anchorOf(e.currentTarget))} />
        </div>
      ) : null}
      {snapshot.mutationError ? <ErrorNotice error={snapshot.mutationError} context="detail" operation="save" /> : null}
      {rows.length ? <KeyValue rows={rows} /> : null}
      {castTotal > 12 && !(allCast && castPages.done) ? <div><Button variant="link" label={currentI18n().t(allCast ? 'title.moreCast' : 'title.allCast')} onClick={() => { setAllCast(true); castPages.more(); }} /></div> : null}
      {crewLines.map(line => <Text key={line} as="p" variant="caption" tone="secondary">{line}</Text>)}
      {data?.files?.length ? <TitleFiles files={data.files} /> : null}
      {previous || next ? (
        <div className={s.panelNav}>
          {previous ? <Button variant="ghost" size="sm" icon="back" label={episodeCode(previous) || previous.title} aria-label={`${currentI18n().t('title.previousEpisode')}: ${episodeCode(previous)} ${previous.title}`} onClick={() => onOpenEpisode(previous)} /> : <span />}
          {next ? <Button variant="ghost" size="sm" iconAfter="forward" label={episodeCode(next) || next.title} aria-label={`${currentI18n().t('title.nextEpisode')}: ${episodeCode(next)} ${next.title}`} onClick={() => onOpenEpisode(next)} /> : null}
        </div>
      ) : null}
    </Dialog>
  );
}
