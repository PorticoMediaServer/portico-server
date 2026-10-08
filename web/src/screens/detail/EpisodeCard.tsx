import {episodeCode as sharedEpisodeCode} from '@core/presentation/index.ts';
import React from 'react';
import type {ContentEntry} from '@core/library-content.ts';
import {formatDuration, progressFor} from '@core/presentation/index.ts';
import {episodeArt} from '../../app/title-layout';
import {Artwork, Button, Icon, StillCard, StillCardPlaceholder} from '../../ui';
import s from './Show.module.css';
import {currentI18n} from '../../app/i18n';

/** An episode as a landscape card: the still plays, the title opens the episode panel. The synopsis runs two lines. No still → a styled placeholder with the episode code, never the show backdrop. */
export const EpisodeCard = React.memo(function EpisodeCard({episode, onPlay, onOpen, selected}: {episode: ContentEntry; onPlay?: (episode: ContentEntry) => void; onOpen: (episode: ContentEntry) => void; selected?: boolean}) {
  const label = [episodeCode(episode), episode.title].filter(Boolean).join(', ');
  const playable = !!episode.playback && !!onPlay;
  const art = episodeArt(episode);
  return (
    <StillCard
      still={art.kind === 'still' ? art.path : undefined}
      placeholder={art.kind === 'placeholder' ? episodeCode(episode) : undefined}
      number={episode.absoluteNumber ?? episode.episodeNumber}
      title={episode.title}
      meta={episode.duration ? formatDuration(episode.duration) : undefined}
      synopsis={episode.overview}
      progress={progressFor(episode)}
      watched={episode.watched}
      selected={selected}
      playLabel={`${currentI18n().t('action.play')} ${label}`}
      openLabel={label}
      onPlay={playable ? () => onPlay!(episode) : undefined}
      onOpen={() => onOpen(episode)}
    />
  );
});

export const EpisodePlaceholder = StillCardPlaceholder;

/**
 * An episode as a list row (the default episode layout on web, Spec — Title Pages §2): still,
 * number and title, aired date · runtime, a two-line synopsis, the progress bar on the still and
 * a check once watched. The still plays; the copy opens the episode panel; Play and ⋯ appear on
 * hover so the row stays quiet. The row itself takes focus: Enter plays it, Up and Down move
 * through the list (the list owns the arrows).
 */
export const EpisodeRow = React.memo(function EpisodeRow({episode, onPlay, onOpen, onMore, selected, showSeason}: {episode: ContentEntry; onPlay?: (episode: ContentEntry) => void; onOpen: (episode: ContentEntry) => void; onMore?: (episode: ContentEntry, anchor: HTMLElement) => void; selected?: boolean; /** Print "S2 E4" in place of the bare number (the pinned Next up row, which may sit above another season's list). */ showSeason?: boolean}) {
  const t = currentI18n().t;
  const label = [episodeCode(episode), episode.title].filter(Boolean).join(', ');
  const playable = !!episode.playback && !!onPlay;
  const art = episodeArt(episode);
  const meta = [episode.airDate ? t('title.airedOn', {date: formatAirDate(episode.airDate)}) : undefined, episode.duration ? formatDuration(episode.duration) : undefined].filter(Boolean).join(' · ');
  const progress = progressFor(episode);
  return (
    <div className={s.episodeRow} data-selected={selected || undefined} data-episode-row tabIndex={0} role="group" aria-label={label}
      onKeyDown={e => { if (e.target === e.currentTarget && e.key === 'Enter') { e.preventDefault(); if (playable) onPlay!(episode); else onOpen(episode); } }}>
      <div className={s.episodeRowStill}>
        {art.kind === 'still'
          ? <button type="button" tabIndex={-1} className={s.episodeRowArt} aria-label={playable ? `${t('action.play')} ${label}` : label} onClick={playable ? () => onPlay!(episode) : () => onOpen(episode)}><Artwork path={art.path} shape="landscape" icon="tv" alt="" progress={progress} />{playable ? <span className={s.episodeRowPlay} aria-hidden><Icon name="play" size={18} /></span> : null}</button>
          : <button type="button" tabIndex={-1} className={s.episodeRowPlaceholder} aria-label={playable ? `${t('action.play')} ${label}` : label} onClick={playable ? () => onPlay!(episode) : () => onOpen(episode)}><span>{episodeCode(episode)}</span>{progress ? <span className={s.episodeRowProgress} style={{width: `${Math.round(progress * 100)}%`}} /> : null}</button>}
        {episode.watched ? <span className={s.episodeRowWatched} title={t('status.watched')}><Icon name="check" size={14} strokeWidth={2.2} /></span> : null}
      </div>
      <button type="button" className={s.episodeRowCopy} onClick={() => onOpen(episode)} aria-label={label}>
        <span className={s.episodeRowTitle}>{episode.episodeNumber != null ? <span className={s.episodeRowNumber}>{episode.absoluteNumber ?? (showSeason ? episodeCode(episode) : episode.episodeNumber)}</span> : null}<span>{episode.title}</span></span>
        {meta ? <span className={s.episodeRowMeta}>{meta}</span> : null}
        {episode.overview ? <span className={s.episodeRowSynopsis}>{episode.overview}</span> : null}
      </button>
      <div className={s.episodeRowActions}>
        {playable ? <Button variant="ghost" size="sm" icon="play" aria-label={`${t('action.play')} ${label}`} title={t('action.play')} onClick={() => onPlay!(episode)} /> : null}
        {onMore ? <Button variant="ghost" size="sm" icon="more" aria-label={t('title.moreActions')} title={t('title.moreActions')} onClick={(e: React.MouseEvent<HTMLButtonElement>) => onMore(episode, e.currentTarget)} /> : null}
      </div>
    </div>
  );
});

/** "12 Mar 2024" without a timezone shift (air dates are calendar dates). */
export function formatAirDate(iso: string): string {
  const d = new Date(iso.slice(0, 10) + 'T00:00:00');
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleDateString(undefined, {year: 'numeric', month: 'short', day: 'numeric'});
}

/** "S2 E4" style labels. */
/** The episode's code in the viewer's language (the shared rule: "S1 E3", "E3", or "Episode 1043" under absolute numbering). */
export function episodeCode(entry: Pick<ContentEntry, 'seasonNumber' | 'episodeNumber' | 'absoluteNumber'>): string {
  return sharedEpisodeCode(entry, currentI18n().t) ?? '';
}

/**
 * Up and Down move through the episode rows of a list (the rows in the window; scrolling brings
 * the next ones in). Put it on the element that contains the rows.
 */
export function episodeListKeys(e: React.KeyboardEvent<HTMLElement>) {
  if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
  const current = (e.target as HTMLElement).closest<HTMLElement>('[data-episode-row]');
  if (!current) return;
  const rows = [...e.currentTarget.querySelectorAll<HTMLElement>('[data-episode-row]')];
  const next = rows[rows.indexOf(current) + (e.key === 'ArrowDown' ? 1 : -1)];
  if (!next) return;
  e.preventDefault();
  next.focus({preventScroll: true});
  next.scrollIntoView({block: 'nearest'});
}
