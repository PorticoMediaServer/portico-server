import type {ManagedLibrary} from '@core/library-management.ts';

/** One drawn Libraries row: the library's own name, folder and scan state. */
export type LibraryRowModel = Readonly<{
  id: string; name: string; folder: string;
  scanning: boolean; canScan: boolean; canMoveUp: boolean; canMoveDown: boolean;
}>;

/** A scan is running only while the server says so. */
export const scanning = (library: ManagedLibrary) => library.lastScan?.status === 'running';

/** The frame draws the folder and, while a scan runs, the one word "Scanning".
 * It never draws a percentage, a file count or a bar on this page. */
export function libraryRows(libraries: readonly ManagedLibrary[]): readonly LibraryRowModel[] {
  return libraries.map((library, index) => Object.freeze({
    id: library.id,
    name: library.name,
    folder: library.source.path,
    scanning: scanning(library),
    canScan: library.actions.includes('scan') && !scanning(library),
    canMoveUp: index > 0,
    canMoveDown: index < libraries.length - 1,
  }));
}

/** Stable complete order, or undefined when the move changes nothing. */
export function movedIds(libraries: readonly ManagedLibrary[], id: string, offset: -1 | 1): readonly string[] | undefined {
  const ids = libraries.map(library => library.id), from = ids.indexOf(id), to = from + offset;
  if (from < 0 || to < 0 || to >= ids.length) return undefined;
  const next = [...ids];
  next.splice(to, 0, ...next.splice(from, 1));
  return next;
}
