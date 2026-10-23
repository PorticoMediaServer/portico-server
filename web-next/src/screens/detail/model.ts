import type {ContentEntry} from '@core/library-content.ts';
import type {TitleFile} from '@core/detail.ts';
import {cardText, type CardSubject, type TextEnvironment, type FileFacts} from '@core/presentation/kit-model';

/** Same semantic TitleKind projection for Search, browse and details; no joined subtitle parsing. */
export function entrySubject(entry: ContentEntry): CardSubject {
  const episode = entry.kind !== 'episode' ? undefined : entry.absoluteNumber !== undefined
    ? {mode: 'absolute' as const, number: entry.absoluteNumber}
    : entry.airDate && entry.episodeNumber === undefined ? {mode: 'date' as const, day: entry.airDate}
    : entry.seasonNumber === 0 ? {mode: 'special' as const}
    : {mode: 'seasonal' as const, season: entry.seasonNumber, episode: entry.episodeNumber};
  return {id: entry.id, kind: entry.kind, title: entry.title, year: entry.year, contentRating: entry.contentRating,
    durationSeconds: entry.duration, positionSeconds: entry.progressSeconds, genres: entry.genres, seasonCount: entry.seasonCount,
    artist: entry.artist?.name, album: entry.album?.name, author: entry.author,
    show: entry.show ? {id: entry.show.id, title: entry.show.name} : undefined, episode};
}
export function entryCard(entry: ContentEntry, env: TextEnvironment, context: 'search' | 'ordinary' | 'in-show' = 'ordinary') {
  return cardText(entrySubject(entry), context, env);
}
export function fileFacts(file: TitleFile): FileFacts {
  return {id: file.id, available: file.available, edition: file.edition, container: file.container, sizeBytes: file.size,
    durationSeconds: file.duration, bitRate: file.bitRate, videoCodec: file.video?.codec, width: file.video?.width,
    height: file.video?.height, dynamicRange: file.video?.dynamicRange, hdr10Plus: file.video?.hdr10Plus,
    frameRate: file.video?.frameRate, bitDepth: file.video?.bitDepth,
    audio: file.audio.map((track, i) => ({...track, id: `audio:${i}`, objectAudio: track.objectAudio === 'atmos' || track.objectAudio === 'dtsx' ? track.objectAudio : undefined})),
    subtitles: file.subtitles.map((track, i) => ({...track, id: `subtitle:${i}`, objectAudio: undefined}))};
}
