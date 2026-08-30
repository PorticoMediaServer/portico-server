import type { MediaItem, MediaKind } from '../../data/models';

type LabelTarget = Pick<MediaItem, 'entityKind' | 'title' | 'progress' | 'progressSeconds'>;

export function isListeningMedia(item: Pick<LabelTarget, 'entityKind'>) {
  return ['artist', 'album', 'track', 'author', 'audiobook-series', 'audiobook', 'chapter'].includes(item.entityKind);
}

export function playActionLabel(item: Pick<LabelTarget, 'entityKind' | 'progress' | 'progressSeconds'>) {
  if (item.entityKind === 'artist') return 'Play mix';
  if (item.entityKind === 'album') return 'Play album';
  if (item.entityKind === 'track') return item.progress || item.progressSeconds ? 'Resume' : 'Play';
  if (item.entityKind === 'episode') return 'Resume';
  return item.progress || item.progressSeconds ? 'Resume' : 'Play';
}

export function compactSavedLabel(item: Pick<LabelTarget, 'entityKind'>, saved: boolean) {
  if (isListeningMedia(item)) return saved ? 'Saved' : 'Save';
  return saved ? 'In watchlist' : 'Watchlist';
}

export function savedMenuLabel(item: Pick<LabelTarget, 'entityKind'>, saved: boolean) {
  if (isListeningMedia(item)) return saved ? 'Remove from saved' : 'Save';
  return saved ? 'Remove from watchlist' : 'Add to watchlist';
}

export function savedAriaLabel(item: Pick<LabelTarget, 'entityKind' | 'title'>, saved: boolean) {
  if (isListeningMedia(item)) return saved ? `Remove ${item.title} from saved` : `Save ${item.title}`;
  return `${saved ? 'Remove' : 'Add'} ${item.title} ${saved ? 'from' : 'to'} watchlist`;
}

export function savedMutationError(item: Pick<LabelTarget, 'entityKind'>) {
  return isListeningMedia(item) ? 'This item was not saved.' : 'The watchlist was not changed.';
}

export function selectedSavedCopy(items: Array<Pick<LabelTarget, 'entityKind'>>, remove = false) {
  const listeningOnly = items.length > 0 && items.every(isListeningMedia);
  if (listeningOnly) return remove ? { label: 'Remove saved', notice: 'Removed from saved.' } : { label: 'Save', notice: 'Saved.' };
  return remove ? { label: 'Remove saved', notice: 'Removed from watchlist.' } : { label: 'Watchlist', notice: 'Added to watchlist.' };
}

export function normalizedContextKind(item: Pick<MediaItem, 'entityKind'>): MediaKind {
  return item.entityKind;
}
