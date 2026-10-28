import type {ContentEntry} from '@core/library-content.ts';
import type {HomeDocument, HomeLive, HomeRow} from '@core/home.ts';
import {homeSpotlightModels, homeRowPresentation, homeTitleFactParts} from '@core/presentation/home-spotlight.ts';
import {contentEntrySubject} from '@core/presentation/content-entry-subject.ts';
import {cardText, type CardSubject, type TextEnvironment} from '@portico/client-core/kit-model';
import {homeEntryCapabilities, homeEntrySelection} from '@core/home-selection.ts';
import type {HomeSelectionPlaybackItem} from '@core/home-selection/index.ts';

/** A playback DTO alone is not a grant; resume keeps the actual separately selected child. */
export function homePlaybackTarget(entry: ContentEntry, target: ContentEntry = entry) {
  return homeEntryCapabilities(entry, {platform: 'web', playbackAvailable: true, queueAvailable: true}).playable && target.available !== false && !target.missing ? target.playback : undefined;
}

/** A stale render cannot dispatch an entry/target pair absent from the current bounded Home document. */
export function homePlaybackItem(document: HomeDocument, entry: ContentEntry, target: ContentEntry): HomeSelectionPlaybackItem | undefined {
  const offered = document.spotlight.some(slide => slide.entry === entry && (slide.resumeTarget ?? slide.entry) === target);
  const inRows = (rows: readonly HomeRow[]): boolean => rows.some(row => row.entries.includes(entry) && (row.resumes?.find(resume => resume.entryId === entry.id)?.target ?? entry) === target || inRows(row.children ?? []));
  if (!offered && !inRows(document.rows)) return undefined;
  const playback = homePlaybackTarget(entry, target), identity = homeEntrySelection(entry, {platform: 'web', playbackAvailable: true, queueAvailable: true});
  return playback && identity ? {identity, entry, target: playback} : undefined;
}

/** Fields stay semantic. In particular, the old joined subtitle is never parsed. */
export function homeCardSubject(entry: ContentEntry): CardSubject {
  return contentEntrySubject(entry);
}
export function homeCardModel(entry: ContentEntry, continuing: boolean, env: TextEnvironment) {
  return cardText(homeCardSubject(entry), continuing ? 'continuation' : 'ordinary', env);
}
export function homeSpotlights(document: HomeDocument, env: TextEnvironment) {
  return homeSpotlightModels(document).map(slide => ({...slide, id: slide.entry.id, title: slide.entry.displayTitle ?? slide.entry.title,
    facts: homeTitleFactParts(slide.entry, env)}));
}
export type HeroArtworkTreatment = 'colour' | 'generated' | 'poster' | 'panel' | 'backdrop';
/**
 * The author's extras frame draws three hero states from what a title actually has.
 * The colour preference withdraws every image, and the panel crop is that frame's
 * backdrop excerpt, never the full hero.
 */
export function heroArtworkTreatment(artwork: Readonly<{backdropUrl?: string; posterUrl?: string}>, showBackdrops: boolean, variant: 'hero' | 'panel' = 'hero'): HeroArtworkTreatment {
  if (!showBackdrops) return 'colour';
  if (artwork.backdropUrl) return variant === 'panel' ? 'panel' : 'backdrop';
  return artwork.posterUrl ? 'poster' : 'generated';
}
/** Expired live facts are withdrawn, never presented as current while refreshing. */
export function currentHomeLive(live: HomeLive | undefined, nowMs: number): HomeLive | undefined {
  if (!live || !Number.isFinite(nowMs)) return undefined;
  const observed = Date.parse(live.observedAt), expires = Date.parse(live.expiresAt);
  return Number.isFinite(observed) && Number.isFinite(expires) && observed <= nowMs && nowMs < expires ? live : undefined;
}
/** Only the published bounded row is inspected. No guide or library scan is involved. */
export function homeLiveDeadline(rows: readonly HomeRow[], nowMs: number): number | undefined {
  let deadline = Infinity;
  for (const row of rows) {
    const live = currentHomeLive(row.live, nowMs);
    if (!live) continue;
    deadline = Math.min(deadline, Date.parse(live.expiresAt));
    for (const {channel} of live.channels) for (const programme of channel.programmes) {
      const start = Date.parse(programme.start), end = Date.parse(programme.end);
      if (start > nowMs) deadline = Math.min(deadline, start);
      if (end > nowMs) deadline = Math.min(deadline, end);
    }
  }
  return Number.isFinite(deadline) ? deadline : undefined;
}
export function homeRows(document: HomeDocument) {
  return document.rows.filter(row => !row.platforms || row.platforms.includes('web')).map(homeRowPresentation);
}
