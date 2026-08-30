import { productMessage, type ProductMessageId } from '@porticomediaserver/client-core';
import type { MediaItem, MediaStream } from '../../data/models';
import { mediaPresentation } from '../catalog/mediaPresentation';
import type { PlaybackCollectionContext } from '../player/watchNavigation';

export function rawDetailKind(item: MediaItem) {
  return item.entityKind;
}

export function detailKind(item: MediaItem) {
  return rawDetailKind(item);
}

export function orderedDetailItems(items: MediaItem[]) {
  const numericPosition = (item: MediaItem) => {
    const metadata = item.typedMetadata ?? {};
    const value = item.seasonNumber != null && item.episodeNumber != null
      ? item.seasonNumber * 10_000 + item.episodeNumber
      : item.seasonNumber
        ?? item.episodeNumber
      ?? metadata.trackNumber
      ?? metadata.track
      ?? metadata.chapterNumber
      ?? metadata.position;
    const number = Number(value);
    return Number.isFinite(number) ? number : Number.POSITIVE_INFINITY;
  };
  return items.map((item, index) => ({ item, index })).sort((left, right) => {
    const leftPosition = numericPosition(left.item);
    const rightPosition = numericPosition(right.item);
    if (leftPosition !== rightPosition) return leftPosition < rightPosition ? -1 : 1;
    return left.index - right.index;
  }).map(({ item }) => item);
}

export function showEpisodes(item: MediaItem) {
  if (detailKind(item) !== 'show') return [];
  return orderedDetailItems((item.children ?? []).flatMap((child) => detailKind(child) === 'season'
    ? orderedDetailItems((child.children ?? []).filter((episode) => detailKind(episode) === 'episode'))
    : detailKind(child) === 'episode' ? [child] : []));
}

export function showPlaybackTarget(item: MediaItem) {
  if (item.playbackTarget) return item.playbackTarget;
  const episodes = showEpisodes(item);
  return episodes.find((episode) => (episode.progressSeconds ?? 0) > 0 && !episode.watched)
    ?? episodes.find((episode) => (episode.progress ?? 0) > 0 && !episode.watched)
    ?? episodes.find((episode) => !episode.watched)
    ?? episodes[0];
}

export function formatResumeTime(seconds: number) {
  const safe = Math.max(0, Math.floor(seconds));
  const hours = Math.floor(safe / 3600);
  const minutes = Math.floor((safe % 3600) / 60);
  const remainder = safe % 60;
  return hours > 0
    ? `${hours}:${String(minutes).padStart(2, '0')}:${String(remainder).padStart(2, '0')}`
    : `${minutes}:${String(remainder).padStart(2, '0')}`;
}

export function detailPlaybackContext(item: MediaItem): PlaybackCollectionContext {
  const kind = detailKind(item);
  const type = ['album', 'artist', 'playlist', 'collection'].includes(kind)
    ? kind as 'album' | 'artist' | 'playlist' | 'collection'
    : 'queue';
  return { type, id: item.id, title: item.title };
}

export function isMusicDetail(item: MediaItem) {
  return ['artist', 'album', 'track'].includes(detailKind(item));
}

export function isAudiobookDetail(item: MediaItem) {
  return ['author', 'audiobook', 'audiobook-series', 'chapter'].includes(rawDetailKind(item));
}

export function isChannelDetail(item: MediaItem) {
  return detailKind(item) === 'live-channel';
}

export function isContainerDetail(item: MediaItem) {
  return ['show', 'season', 'artist', 'album', 'author', 'audiobook-series', 'collection', 'playlist', 'category'].includes(detailKind(item));
}

export function isPlayableDetail(item: MediaItem) {
  return !isContainerDetail(item) && ['movie', 'episode', 'track', 'audiobook', 'chapter', 'recording', 'live-channel'].includes(detailKind(item));
}

export function detailArtworkShape(item: MediaItem): 'square' | 'poster' {
  return mediaPresentation(item).artworkShape === 'square' ? 'square' : 'poster';
}

export function detailKindLabel(item: MediaItem) {
  return mediaPresentation(item).label;
}

export function detailLibraryDestination(item: MediaItem) {
  const kind = detailKind(item);
  if (kind === 'collection') return { path: '/saved?tab=collections', label: productMessage('destination.collections').text ?? '' };
  if (kind === 'playlist') return { path: '/saved?tab=playlists', label: productMessage('destination.playlists').text ?? '' };
  if (kind === 'live-channel') return { path: '/live?tab=channels', label: productMessage('destination.live-tv').text ?? '' };
  const path = item.libraryId ? `/library/${encodeURIComponent(item.libraryId)}` : '/libraries';
  if (item.libraryName?.trim()) return { path, label: item.libraryName.trim() };
  if (isAudiobookDetail(item)) return { path, label: productMessage('destination.audiobooks').text ?? '' };
  if (kind === 'recording') return { path, label: productMessage('destination.recorded-tv').text ?? '' };
  if (isMusicDetail(item)) return { path, label: productMessage('destination.music').text ?? '' };
  if (kind === 'movie') return { path, label: productMessage('destination.movies').text ?? '' };
  if (kind === 'show' || kind === 'season' || kind === 'episode') return { path, label: productMessage('destination.tv-shows').text ?? '' };
  return { path, label: productMessage('destination.libraries').text ?? 'Libraries' };
}

export function detailSavedLabel(item: MediaItem, saved: boolean) {
  const presentation = mediaPresentation(item);
  if (saved) return presentation.savedVerb === 'save' ? 'Remove from Saved' : 'Remove from watchlist';
  const kind = detailKind(item);
  const labels: Record<string, string> = {
    artist: 'Save artist',
    album: 'Save album',
    track: 'Save track',
    author: 'Save author',
    audiobook: 'Save audiobook',
    'audiobook-series': 'Save series',
    collection: 'Save collection',
    playlist: 'Save playlist',
  };
  return labels[kind] ?? 'Add to watchlist';
}

export function detailWatchedLabel(item: MediaItem, watched: boolean) {
  const kind = detailKind(item);
  const completion = mediaPresentation(item).completionNoun;
  if (completion === 'finished') return watched ? 'Mark unfinished' : 'Mark finished';
  if (completion === 'played') return watched ? 'Mark unplayed' : 'Mark played';
  if (kind === 'show') return watched ? 'Mark show unwatched' : 'Mark show watched';
  if (kind === 'season') return watched ? 'Mark season unwatched' : 'Mark season watched';
  if (kind === 'collection' || kind === 'playlist') return watched ? 'Mark all unwatched' : 'Mark all watched';
  return watched ? 'Mark as unwatched' : 'Mark as watched';
}

export function detailChildTitle(item: MediaItem) {
  const messages: Record<string, ProductMessageId> = {
    show: 'media.seasons-title',
    season: 'media.episodes-title',
    artist: 'media.albums-title',
    album: 'media.tracks-title',
    author: 'media.audiobooks-title',
    audiobook: 'media.chapters-title',
    'audiobook-series': 'media.audiobooks-title',
    collection: 'media.included-title',
    playlist: 'media.playlist-title',
    'live-channel': 'media.programming-title',
  };
  return productMessage(messages[detailKind(item)] ?? 'media.included-title').text ?? '';
}

export function detailEmptyChildCopy(item: MediaItem) {
  const messages: Record<string, ProductMessageId> = {
    show: 'media.empty-show',
    season: 'media.empty-season',
    artist: 'media.empty-artist',
    album: 'media.empty-album',
    author: 'media.empty-author',
    audiobook: 'media.empty-audiobook',
    'audiobook-series': 'media.empty-series',
    collection: 'media.empty-collection',
    playlist: 'media.empty-playlist',
    'live-channel': 'media.empty-channel',
  };
  return productMessage(messages[detailKind(item)] ?? 'media.empty-included').text ?? '';
}

export function detailMetaParts(item: MediaItem) {
  const kind = detailKind(item);
  const typed = item.typedMetadata ?? {};
  const parts: string[] = [];
  const add = (value: unknown) => {
    const text = String(value ?? '').trim();
    if (text && !parts.includes(text)) parts.push(text);
  };
  if (kind === 'episode') {
    if (item.seasonNumber != null && item.episodeNumber != null) add(productMessage('media.episode-code', { seasonNumber: item.seasonNumber, episodeNumber: item.episodeNumber }).text);
    else if (item.seasonNumber != null) add(productMessage('media.season-number', { seasonNumber: item.seasonNumber }).text);
    else if (item.episodeNumber != null) add(productMessage('media.episode-number', { episodeNumber: item.episodeNumber }).text);
  }
  if (kind === 'season' && item.seasonNumber != null) add(productMessage(item.seasonNumber === 0 ? 'media.specials-label' : 'media.season-number', { seasonNumber: item.seasonNumber }).text);
  if (kind === 'track') {
    const track = typed.trackNumber || typed.track || typed.trackIndex;
    const disc = typed.discNumber || typed.disc || typed.mediaNumber;
    if (disc && track) add(productMessage('media.disc-track', { discNumber: disc, trackNumber: track }).text);
    else if (track) add(productMessage('media.track-number', { trackNumber: track }).text);
  }
  if (kind === 'album') add(typed.albumArtist || typed.artist || item.parentTitle);
  if (kind === 'track') add(typed.trackArtist || typed.artist || item.grandparentTitle);
  if (isAudiobookDetail(item)) {
    add(typed.author || item.grandparentTitle);
    add(typed.narrator ? productMessage('media.narrated-by', { narrator: typed.narrator }).text : undefined);
    add(typed.series ? (typed.seriesPosition
      ? productMessage('media.series-book', { series: typed.series, bookNumber: typed.seriesPosition }).text
      : typed.series) : undefined);
  }
  add(item.edition);
  add(item.year || undefined);
  add(item.contentRating || item.rating || undefined);
  add(item.length || undefined);
  add(item.genre || undefined);
  if (item.communityRating != null) add(`${item.communityRating.toFixed(1)}/10`);
  if (item.criticRating != null) add(productMessage('media.critic-score', { score: Math.round(item.criticRating) }).text);
  return parts;
}

export function formatDetailBytes(value: number) {
  if (!value) return productMessage('media.size-unavailable').text ?? '';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let amount = value;
  let unit = 0;
  while (amount >= 1024 && unit < units.length - 1) {
    amount /= 1024;
    unit += 1;
  }
  return `${amount >= 10 || unit === 0 ? amount.toFixed(0) : amount.toFixed(1)} ${units[unit]}`;
}

export function formatDetailDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleDateString([], { month: 'short', day: 'numeric', year: 'numeric' });
}

export function friendlyStreamLabel(stream: MediaStream) {
  const codec = stream.codec
    .replace(/^h264$/i, 'H.264')
    .replace(/^h265$/i, 'H.265')
    .replace(/^hevc$/i, 'HEVC')
    .toLocaleUpperCase()
    .replace('H.264', 'H.264')
    .replace('H.265', 'H.265');
  const resolution = stream.height ? `${stream.height}p` : stream.width ? `${stream.width}px wide` : undefined;
  const language = stream.language && !/^und$/i.test(stream.language) ? stream.language : undefined;
  const channels = stream.channels ? (stream.channels === 6 ? '5.1' : stream.channels === 8 ? '7.1' : `${stream.channels} ch`) : undefined;
  return [codec, resolution, stream.dynamicRange, language, channels].filter(Boolean).join(' · ') || stream.displayTitle;
}

export function displayMetadataLabel(value: string) {
  return value.replaceAll('_', ' ').replace(/([a-z])([A-Z])/g, '$1 $2').replace(/\b\w/g, (letter) => letter.toLocaleUpperCase());
}
