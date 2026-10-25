import type {ContentEntry} from '@core/library-content.ts';
import type {CardSubject, PlaybackSubject} from '@core/presentation/kit-model/index.ts';
import type {GetItemsIdResponse} from '../../../../packages/contracts/src/types.ts';

export function entrySubject(entry: ContentEntry): PlaybackSubject {
  if (entry.kind === 'ebook' || entry.kind === 'comic' || entry.kind === 'radio') throw new Error('invalid_playback_item');
  const item: CardSubject = {
    id: entry.id, kind: entry.kind, title: entry.title, year: entry.year,
    artist: entry.artist?.name, album: entry.album?.name, author: entry.author,
    ...(entry.show ? {show: {id: entry.show.id, title: entry.show.name}} : {}),
    ...(entry.episodeNumber !== undefined ? {episode: entry.absoluteNumber !== undefined ? {mode: 'absolute' as const, number: entry.absoluteNumber} : {mode: 'seasonal' as const, season: entry.seasonNumber, episode: entry.episodeNumber}} : {}),
  };
  return {kind: entry.kind === 'song' ? 'music' : ['audiobook_file', 'book', 'chapter'].includes(entry.kind) ? 'audiobook' : 'video', item};
}

export function itemSubject(raw: GetItemsIdResponse): PlaybackSubject {
  if (!['movie', 'episode', 'extra', 'song', 'audiobook_file'].includes(raw.kind) || !raw.id || !raw.title) throw new Error('invalid_playback_item');
  const episode = raw.episode;
  const item: CardSubject = {
    id: raw.id, kind: raw.kind as CardSubject['kind'], title: raw.title, year: raw.year,
    artist: raw.song?.artist, album: raw.song?.albumTitle,
    author: raw.bookFile?.localMetadata?.authors.join(', '), narrator: raw.bookFile?.localMetadata?.narrators.join(', '),
    ...(episode ? {show: {id: episode.showId, title: episode.showTitle}, episode: episode.numbering === 'date' ? {mode: 'date' as const, day: episode.airDate} : episode.numbering === 'absolute' ? {mode: 'absolute' as const, number: episode.number} : episode.seasonNumber === 0 ? {mode: 'special' as const} : {mode: 'seasonal' as const, season: episode.seasonNumber, episode: episode.number}} : {}),
  };
  return {kind: raw.kind === 'song' ? 'music' : raw.kind === 'audiobook_file' ? 'audiobook' : 'video', item};
}
