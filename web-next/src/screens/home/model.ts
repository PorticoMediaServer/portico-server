import type {ContentEntry} from '@core/library-content.ts';
import type {HomeDocument, HomeLive, HomeRow} from '@core/home.ts';
import {homeSpotlightModels, homeRowPresentation} from '@core/presentation/home-spotlight.ts';
import {cardText, titleFactParts, type CardSubject, type TextEnvironment} from '@portico/client-core/kit-model';
import {homeEntryCapabilities} from '@core/home-selection.ts';

/** A playback DTO alone is not a grant; resume keeps the actual separately selected child. */
export function homePlaybackTarget(entry: ContentEntry, target: ContentEntry = entry) {
  return homeEntryCapabilities(entry, {platform: 'web', playbackAvailable: true, queueAvailable: true}).playable && target.available !== false && !target.missing ? target.playback : undefined;
}

/** Fields stay semantic. In particular, the old joined subtitle is never parsed. */
export function homeCardSubject(entry: ContentEntry): CardSubject {
  const episode = entry.kind !== 'episode' ? undefined : entry.absoluteNumber !== undefined
    ? {mode: 'absolute' as const, number: entry.absoluteNumber}
    : entry.airDate && entry.episodeNumber === undefined ? {mode: 'date' as const, day: entry.airDate}
    : entry.seasonNumber === 0 ? {mode: 'special' as const}
    : {mode: 'seasonal' as const, season: entry.seasonNumber, episode: entry.episodeNumber};
  return {id: entry.id, kind: entry.kind, title: entry.title,
    year: entry.year, contentRating: entry.contentRating, durationSeconds: entry.duration,
    positionSeconds: entry.progressSeconds, genres: entry.genres, seasonCount: entry.seasonCount,
    artist: entry.artist?.name, album: entry.album?.name, author: entry.author,
    show: entry.show ? {id: entry.show.id, title: entry.show.name} : undefined, episode};
}
export function homeCardModel(entry: ContentEntry, continuing: boolean, env: TextEnvironment) {
  return cardText(homeCardSubject(entry), continuing ? 'continuation' : 'ordinary', env);
}
export function homeSpotlights(document: HomeDocument, env: TextEnvironment) {
  return homeSpotlightModels(document).map(slide => ({...slide, id: slide.entry.id, title: slide.entry.title,
    facts: titleFactParts({...slide.facts, durationSeconds: slide.facts.runtimeSeconds, contentRating: slide.entry.contentRating}, env)}));
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
