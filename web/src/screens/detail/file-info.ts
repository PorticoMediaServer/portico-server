import {readFileInfo, type DetailApi, type DetailProjection, type DetailScope, type FileInfoProjection} from '@core/detail.ts';

/** File facts are a separate, paged resource with their own authority fence. */
export async function readDetailFileInfo(api: DetailApi, scope: DetailScope, data: Pick<DetailProjection, 'scope' | 'revision'>, signal: AbortSignal, isCurrent: () => boolean): Promise<FileInfoProjection> {
  const current = () => {
    signal.throwIfAborted();
    if (!isCurrent()) throw new DOMException('Detail retired', 'AbortError');
  };
  const changed = () => { throw {code: 'refresh_required'}; };
  const cursors = new Set<string>();
  const ids = new Set<string>();
  const files: FileInfoProjection['files'][number][] = [];
  let first: FileInfoProjection | undefined;
  let cursor: string | undefined;
  do {
    current();
    const page = await readFileInfo(api, scope, data.scope, {cursor, limit: 100}, signal);
    current();
    if (page.revision.catalog !== data.revision.catalog || page.revision.viewer !== data.revision.viewer) changed();
    if (first && (page.scope.viewerFence !== first.scope.viewerFence || page.kind !== first.kind || JSON.stringify(page.manage) !== JSON.stringify(first.manage))) changed();
    first ??= page;
    for (const file of page.files) {
      if (ids.has(file.id)) changed();
      ids.add(file.id);
      files.push(file);
    }
    cursor = page.nextCursor;
    if (cursor && cursors.has(cursor)) changed();
    if (cursor) cursors.add(cursor);
  } while (cursor);
  current();
  // At least one actual response was read, even when the title has no files.
  if (!first) throw new Error('File information was not read');
  return Object.freeze({...first, files: Object.freeze(files), nextCursor: undefined});
}
