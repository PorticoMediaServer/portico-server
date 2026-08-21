import { productMessage, type SearchGroupCapability } from '@porticomediaserver/client-core';
import type { MediaItem } from '../../data/models';

export type CanonicalMediaKind =
  | 'movie' | 'show' | 'season' | 'episode'
  | 'artist' | 'album' | 'track'
  | 'author' | 'audiobook-series' | 'audiobook' | 'chapter'
  | 'live-channel' | 'live-program' | 'recording'
  | 'collection' | 'playlist' | 'person' | 'unknown';

export type MediaPresentationGroup =
  | 'movies' | 'shows' | 'episodes' | 'music' | 'audiobooks'
  | 'live-dvr' | 'collections-playlists' | 'people' | 'other';

export type MediaPresentationDescriptor = {
  kind: CanonicalMediaKind;
  group: MediaPresentationGroup;
  label: string;
  pluralLabel: string;
  artworkShape: 'poster' | 'square' | 'landscape';
  savedVerb: 'watchlist' | 'save';
  completionNoun: 'watched' | 'played' | 'finished';
  playable: boolean;
  container: boolean;
  person: boolean;
};

const aliases: Record<string, CanonicalMediaKind> = {
  audiobook: 'audiobook',
  book: 'audiobook',
  series: 'audiobook-series',
  'audiobook-series': 'audiobook-series',
  audiobook_series: 'audiobook-series',
  'audiobook-chapter': 'chapter',
  audiobook_chapter: 'chapter',
  channel: 'live-channel',
  live_channel: 'live-channel',
  program: 'live-program',
  live_program: 'live-program',
  live_recording: 'recording',
  'live-recording': 'recording',
  special: 'episode',
};

const descriptors: Record<CanonicalMediaKind, MediaPresentationDescriptor> = {
  movie: descriptor('movie', 'movies', 'Movie', 'Movies', 'poster', 'watchlist', 'watched', true, false),
  show: descriptor('show', 'shows', 'Show', 'Shows', 'poster', 'watchlist', 'watched', false, true),
  season: descriptor('season', 'shows', 'Season', 'Seasons', 'poster', 'watchlist', 'watched', false, true),
  episode: descriptor('episode', 'episodes', 'Episode', 'Episodes', 'landscape', 'watchlist', 'watched', true, false),
  artist: descriptor('artist', 'music', 'Artist', 'Artists', 'square', 'save', 'played', false, true),
  album: descriptor('album', 'music', 'Album', 'Albums', 'square', 'save', 'played', false, true),
  track: descriptor('track', 'music', 'Track', 'Tracks', 'square', 'save', 'played', true, false),
  author: descriptor('author', 'audiobooks', 'Author', 'Authors', 'square', 'save', 'finished', false, true),
  'audiobook-series': descriptor('audiobook-series', 'audiobooks', 'Audiobook series', 'Audiobook series', 'poster', 'save', 'finished', false, true),
  audiobook: descriptor('audiobook', 'audiobooks', 'Audiobook', 'Audiobooks', 'poster', 'save', 'finished', true, false),
  chapter: descriptor('chapter', 'audiobooks', 'Chapter', 'Chapters', 'landscape', 'save', 'finished', true, false),
  'live-channel': descriptor('live-channel', 'live-dvr', productMessage('media.kind.channel').text ?? 'Live channel', 'Live channels', 'landscape', 'save', 'watched', true, false),
  'live-program': descriptor('live-program', 'live-dvr', 'Program', 'Programs', 'landscape', 'save', 'watched', true, false),
  recording: descriptor('recording', 'live-dvr', productMessage('media.kind.recording').text ?? 'DVR recording', 'DVR recordings', 'landscape', 'watchlist', 'watched', true, false),
  collection: descriptor('collection', 'collections-playlists', 'Collection', 'Collections', 'poster', 'save', 'watched', false, true),
  playlist: descriptor('playlist', 'collections-playlists', 'Playlist', 'Playlists', 'square', 'save', 'played', false, true),
  person: descriptor('person', 'people', 'Person', 'People', 'square', 'save', 'watched', false, false, true),
  unknown: descriptor('unknown', 'other', productMessage('media.kind.media').text ?? 'Media', 'Media', 'poster', 'save', 'watched', false, false),
};

function descriptor(
  kind: CanonicalMediaKind,
  group: MediaPresentationGroup,
  label: string,
  pluralLabel: string,
  artworkShape: MediaPresentationDescriptor['artworkShape'],
  savedVerb: MediaPresentationDescriptor['savedVerb'],
  completionNoun: MediaPresentationDescriptor['completionNoun'],
  playable: boolean,
  container: boolean,
  person = false,
): MediaPresentationDescriptor {
  return { kind, group, label, pluralLabel, artworkShape, savedVerb, completionNoun, playable, container, person };
}

export function canonicalMediaKind(item: Pick<MediaItem, 'entityKind' | 'kind'>): CanonicalMediaKind {
  const raw = String(item.entityKind || item.kind || '').trim().toLocaleLowerCase();
  const normalized = aliases[raw] ?? raw.replaceAll('_', '-');
  return Object.hasOwn(descriptors, normalized) ? normalized as CanonicalMediaKind : 'unknown';
}

export function mediaPresentation(item: Pick<MediaItem, 'entityKind' | 'kind'>): MediaPresentationDescriptor {
  return descriptors[canonicalMediaKind(item)];
}

export function searchGroupFilterKinds(group: SearchGroupCapability | Pick<SearchGroupCapability, 'entityKind' | 'resultKinds'>, allowed?: ReadonlySet<string>) {
  const kinds = [...new Set([group.entityKind, ...group.resultKinds].map((kind) => String(kind).trim()).filter(Boolean))];
  if (!allowed) return kinds;
  return kinds.filter((kind) => allowed.has(kind));
}

export function searchGroupMatchesSelection(group: SearchGroupCapability, selectedKinds: readonly string[]) {
  if (selectedKinds.length === 0) return true;
  const members = new Set([group.id, ...searchGroupFilterKinds(group)]);
  return selectedKinds.some((kind) => members.has(kind));
}

export function mediaCountLabel(item: Pick<MediaItem, 'entityKind' | 'kind'>, count: number) {
  const presentation = mediaPresentation(item);
  return `${count} ${count === 1 ? presentation.label : presentation.pluralLabel}`;
}
