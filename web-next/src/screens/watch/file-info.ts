import {readFileInfo, type DetailApi, type DetailScope, type DetailTarget, type FileInfoProjection} from '@core/detail.ts';
import {sameFence, type PlaybackFence} from '../../../../packages/player-core/src/index.ts';
import type {WebPlaybackAuthority, WebPlaybackRuntimeSnapshot} from '../../platform/playback';
import type {MediaVersion, PlaybackOptions} from '@core/playback-v1/types.ts';

export type WatchFileInfoSource = Readonly<{
  api: DetailApi;
  scope: DetailScope;
  /** Capture the real runtime authority object; DetailScope.viewerId is opaque. */
  authority: WebPlaybackAuthority;
  getSnapshot: () => WebPlaybackRuntimeSnapshot;
  isCurrent: () => boolean;
}>;
/** Distinct File info read for this playback. The parent owns open/close and aborts it on exit.
 * No title-details fallback, cached private facts, selected-version guess or raw-file count. */
export async function readWatchFileInfo(source: WatchFileInfoSource, expected: PlaybackFence, target: DetailTarget, signal: AbortSignal): Promise<FileInfoProjection | null> {
  const captured = Object.freeze({...target});
  const current = () => {
    const snapshot = source.getSnapshot();
    return !signal.aborted && source.isCurrent() && !snapshot.disposed && expected.kind === 'queue'
      && sameFence(snapshot.fence, expected) && snapshot.authority === source.authority
      && snapshot.authority.serverId === source.scope.serverId && captured.itemId === expected.itemId;
  };
  if (!current()) return null;
  const projection = await readFileInfo(source.api, source.scope, captured, {limit: 50}, signal);
  return current() ? projection : null;
}

export type WatchFileGroup = Readonly<{
  version: MediaVersion;
  members: readonly Readonly<{id: string; index: number; file?: FileInfoProjection['files'][number]}>[];
}>;
/** Server contract: options.parts[].id and File info files[].id are catalog_assets.token.
 * Options alone defines grouping and available-version count. A missing member can be unpaged
 * or unavailable; neither its details nor a selection is inferred from list order. */
export function watchFileGroups(files: FileInfoProjection, options: PlaybackOptions): Readonly<{
  groups: readonly WatchFileGroup[];
  availableVersionCount: number;
  ungroupedFiles: FileInfoProjection['files'];
}> {
  if (files.scope.itemId !== options.itemId) throw new Error('watch_file_info_target_changed');
  const byId = new Map(files.files.map(file => [file.id, file])), ids = new Set<string>(), members = new Set<string>();
  if (byId.size !== files.files.length) throw new Error('watch_file_info_duplicate_file');
  const groups = options.versions.map(version => {
    if (ids.has(version.id)) throw new Error('watch_file_info_duplicate_version');
    ids.add(version.id);
    return Object.freeze({version, members: Object.freeze(version.parts.map(part => {
      if (members.has(part.id)) throw new Error('watch_file_info_duplicate_member');
      members.add(part.id);
      const file = byId.get(part.id);
      return Object.freeze({id: part.id, index: part.index, ...(file ? {file} : {})});
    }))});
  });
  return Object.freeze({groups: Object.freeze(groups), availableVersionCount: options.versions.length, ungroupedFiles: Object.freeze(files.files.filter(file => !members.has(file.id)))});
}
