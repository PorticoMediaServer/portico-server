import type { MediaItem, MediaKind } from '../../data/models';

type LabelTarget = Pick<MediaItem, 'kind' | 'type' | 'title' | 'progress' | 'progressSeconds'>;

export function isListeningMedia(item: Pick<LabelTarget, 'kind' | 'type'>) {
  return item.type === 'music' || ['artist', 'album', 'track'].includes(item.kind);
}

export function playActionLabel(item: Pick<LabelTarget, 'kind' | 'type' | 'progress' | 'progressSeconds'>) {
  if (item.kind === 'artist') return 'Play mix';
  if (item.kind === 'album') return 'Play album';
  if (item.kind === 'track') return item.progress || item.progressSeconds ? 'Resume' : 'Play';
  if (item.kind === 'episode') return 'Resume';
  return item.progress || item.progressSeconds ? 'Resume' : 'Play';
}

export function compactSavedLabel(item: Pick<LabelTarget, 'kind' | 'type'>, saved: boolean) {
  if (isListeningMedia(item)) return saved ? 'Saved' : 'Save';
  return saved ? 'In watchlist' : 'Watchlist';
}

export function savedMenuLabel(item: Pick<LabelTarget, 'kind' | 'type'>, saved: boolean) {
  if (isListeningMedia(item)) return saved ? 'Remove from saved' : 'Save';
  return saved ? 'Remove from watchlist' : 'Add to watchlist';
}

export function savedAriaLabel(item: Pick<LabelTarget, 'kind' | 'type' | 'title'>, saved: boolean) {
  if (isListeningMedia(item)) return saved ? `Remove ${item.title} from saved` : `Save ${item.title}`;
  return `${saved ? 'Remove' : 'Add'} ${item.title} ${saved ? 'from' : 'to'} watchlist`;
}

export function savedMutationError(item: Pick<LabelTarget, 'kind' | 'type'>) {
  return isListeningMedia(item) ? 'This item was not saved.' : 'The watchlist was not changed.';
}

export function selectedSavedCopy(items: Array<Pick<LabelTarget, 'kind' | 'type'>>, remove = false) {
  const listeningOnly = items.length > 0 && items.every(isListeningMedia);
  if (listeningOnly) return remove ? { label: 'Remove saved', notice: 'Removed from saved.' } : { label: 'Save', notice: 'Saved.' };
  return remove ? { label: 'Remove saved', notice: 'Removed from watchlist.' } : { label: 'Watchlist', notice: 'Added to watchlist.' };
}

export function normalizedContextKind(item: Pick<MediaItem, 'kind' | 'entityKind'>): MediaKind {
  const candidate = item.entityKind || item.kind;
  const supported: MediaKind[] = ['show', 'movie', 'season', 'episode', 'artist', 'album', 'track', 'collection', 'playlist', 'category', 'author', 'book', 'series', 'recording'];
  return supported.includes(candidate as MediaKind) ? candidate as MediaKind : item.kind;
}
