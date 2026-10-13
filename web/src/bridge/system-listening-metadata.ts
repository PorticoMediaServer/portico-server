import type {DetailItem} from '@core/detail';
import type {MediaItem} from '@core/index';

/** Book credit and the current chapter use the same server-authored facts as the player. */
export function listeningMetadata(item: MediaItem, position: number, chapters: readonly {title: string; startSeconds: number; endSeconds: number}[] = []): MediaMetadataInit {
  const detail = item as DetailItem;
  if (item.kind !== 'audiobook_file') return {title: item.title, artist: detail.song?.artist ?? '', album: detail.song?.albumTitle ?? ''};
  const book = detail.bookFile;
  const authors = book?.localMetadata?.authors.join(', ') ?? '';
  const narrators = book?.localMetadata?.narrators.join(', ') ?? '';
  const chapter = chapters.find(c => position >= c.startSeconds && position < c.endSeconds);
  return {title: chapter?.title || item.title, artist: [authors, narrators].filter(Boolean).join(' • '), album: book?.bookTitle ?? ''};
}
