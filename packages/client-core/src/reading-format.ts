/** Physical comic publication formats sharing the typed page-resource interface. */
export type ComicReadingFormat = 'cbz' | 'image-folder';
export function isComicReadingFormat(value: unknown): value is ComicReadingFormat {
  return value === 'cbz' || value === 'image-folder';
}
