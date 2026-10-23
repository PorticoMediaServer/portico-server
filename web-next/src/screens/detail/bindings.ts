import {DetailService, readFileInfo, parseCreditPage, type DetailProjection, type DetailTarget, type FileInfoProjection, type PersonalIntent} from '@core/detail.ts';
import {PlaybackOptionsClient} from '@core/playback-v1/options.ts';
import {localV1Http} from '@core/playback-v1/local-http.ts';
import {parseMediaItem} from '@core/media-item.ts';
import {encodedRoutePath} from '../../../../packages/contracts/src/routes.ts';
import {screenLifetime, type ScreenContext} from './context';
import {createRecommendations} from './recommendations';
import {versionDeletionService, deletionAllowed, type VersionDeletePreview} from '@core/administration.ts';
import {findLibraryFile, libraryFolders, folderOfPath, separateLibraryFile, canMergeTitles, mergeTitles, SPLITTABLE_KINDS} from '@core/title-identity.ts';
import {validateContentEntry, type ContentEntry} from '@core/library-content.ts';

/** Constructor is inert. Only select/readFileInfo/readVersions perform reads. */
export function createDetailBindings(context: ScreenContext) {
  const life = screenLifetime(context);
  const service = new DetailService({api: life.api, scope: context.scope, requestId: async () => { life.assert(); return crypto.randomUUID(); }});
  const options = new PlaybackOptionsClient(localV1Http(life.api));
  const recommendations = createRecommendations(context);
  let selected: DetailTarget | undefined, intent = 0;
  const fileReads = new WeakSet<FileInfoProjection>();
  const previews = new WeakMap<VersionDeletePreview, Readonly<{data: DetailProjection; fileId: string}>>();
  const current = (data: DetailProjection) => {
    life.assert();
    if (service.getSnapshot().phase !== 'ready' || service.getSnapshot().data !== data) throw new DOMException('Title revision changed', 'AbortError');
  };
  const select = async (target: DetailTarget | Readonly<{itemId: string}>, signal?: AbortSignal) => {
    life.assert(); const generation = ++intent;
    signal?.throwIfAborted();
    service.cancel(); recommendations.clear(); selected = undefined;
    // The real item reader supplies library identity for a bare /item/$itemId route.
    const libraryId = 'libraryId' in target ? target.libraryId : parseMediaItem(await life.api.request(encodedRoutePath('get_items_id', {id: encodeURIComponent(target.itemId)}), 'GET', undefined, signal)).libraryId;
    life.assert(); if (generation !== intent) return;
    signal?.throwIfAborted();
    selected = {libraryId, itemId: target.itemId};
    const cancel = () => { if (generation === intent) service.cancel(); }; signal?.addEventListener('abort', cancel, {once: true});
    try { await service.select(selected); signal?.throwIfAborted(); } finally { signal?.removeEventListener('abort', cancel); }
  };
  const files = async (data: DetailProjection, cursor?: string, signal?: AbortSignal): Promise<FileInfoProjection> => {
    current(data);
    const value = await readFileInfo(life.api, context.scope, data.scope, {cursor, limit: 40}, life.signal(signal));
    current(data);
    if (value.scope.viewerFence !== data.scope.viewerFence || value.revision.catalog !== data.revision.catalog || value.revision.viewer !== data.revision.viewer) throw new DOMException('File facts changed', 'AbortError');
    fileReads.add(value);
    return value;
  };
  const authorizeFileCommand = async (data: DetailProjection, captured: FileInfoProjection, action: 'delete' | 'split' | 'merge', fileId?: string, signal?: AbortSignal) => {
    current(data); signal?.throwIfAborted();
    if (!fileReads.has(captured) || !captured.manage?.[action] || captured.scope.itemId !== data.item.id) throw new DOMException('File action unavailable', 'AbortError');
    // The panel is informational. Its old permission bit cannot authorize a later command.
    const fresh = await files(data, undefined, signal);
    if (!fresh.manage?.[action]) throw new DOMException('File permission changed', 'AbortError');
    if (fileId && !fresh.files.some(file => file.id === fileId)) throw new DOMException('File changed', 'AbortError');
    return fresh;
  };
  const commandApi = (signal?: AbortSignal) => ({request: <T,>(path: string, method?: string, body?: unknown) => life.api.request<T>(path, method, body, signal)});
  const resolveEntry = async (itemId: string, signal?: AbortSignal) => {
    const item = parseMediaItem(await life.api.request(encodedRoutePath('get_items_id', {id: encodeURIComponent(itemId)}), 'GET', undefined, signal));
    return validateContentEntry({id: item.id, libraryId: item.libraryId, kind: item.kind, title: item.title, posterUrl: item.posterUrl, backdropUrl: item.backdropUrl, navigation: {view: 'item', entityId: item.id}});
  };
  const versions = async (data: DetailProjection, signal?: AbortSignal) => {
    current(data); const value = await options.options(data.item.id, {}, life.signal(signal)); current(data);
    if (value.itemId !== data.item.id) throw new Error('Playback options target mismatch');
    return value;
  };
  return {
    context, service, select, current, recommendations,
    readFileInfo: files, readVersions: versions,
    async previewDelete(data: DetailProjection, captured: FileInfoProjection, fileId: string, signal?: AbortSignal) {
      await authorizeFileCommand(data, captured, 'delete', fileId, signal);
      const preview = await versionDeletionService(commandApi(signal).request).preview(data.item.id, fileId);
      current(data); signal?.throwIfAborted();
      if (preview.assetId !== fileId || preview.targets.length !== 1 || preview.targets[0]?.itemId !== data.item.id) throw new Error('Delete preview target mismatch');
      previews.set(preview, {data, fileId}); return preview;
    },
    async deleteVersion(data: DetailProjection, captured: FileInfoProjection, preview: VersionDeletePreview, confirmation: string, operationId: string, signal?: AbortSignal) {
      const held = previews.get(preview);
      if (held?.data !== data || !deletionAllowed(preview) || confirmation !== preview.confirmation) throw new DOMException('Delete preview changed', 'AbortError');
      await authorizeFileCommand(data, captured, 'delete', held.fileId, signal);
      const result = await versionDeletionService(commandApi(signal).request).remove(preview, confirmation, operationId);
      current(data); signal?.throwIfAborted(); return result;
    },
    async splitVersion(data: DetailProjection, captured: FileInfoProjection, fileId: string, signal?: AbortSignal) {
      const fresh = await authorizeFileCommand(data, captured, 'split', fileId, signal);
      if (!SPLITTABLE_KINDS.includes(data.item.kind)) throw new Error('Title cannot be split');
      const file = fresh.files.find(value => value.id === fileId)!;
      const folders = file.path ? await libraryFolders(life.api, data.scope.libraryId, life.signal(signal)) : [];
      current(data);
      const record = await findLibraryFile(life.api, data.scope.libraryId, fileId, {folderId: file.path ? folderOfPath(folders, file.path) : undefined, signal: life.signal(signal)});
      current(data); if (!record) return undefined;
      const itemId = await separateLibraryFile(commandApi(signal), data.scope.libraryId, record);
      current(data); signal?.throwIfAborted(); return resolveEntry(itemId, signal);
    },
    async mergeTitle(data: DetailProjection, captured: FileInfoProjection, candidate: ContentEntry, keepId: string, signal?: AbortSignal) {
      await authorizeFileCommand(data, captured, 'merge', undefined, signal);
      if (!canMergeTitles(data.item, candidate) || ![data.item.id, candidate.id].includes(keepId) || candidate.libraryId !== data.scope.libraryId) throw new Error('Merge target changed');
      const fresh = await resolveEntry(candidate.id, signal); current(data);
      if (!canMergeTitles(data.item, fresh) || fresh.libraryId !== data.scope.libraryId) throw new Error('Merge target changed');
      const itemId = await mergeTitles(commandApi(signal), keepId === data.item.id ? candidate.id : data.item.id, keepId);
      current(data); signal?.throwIfAborted(); return resolveEntry(itemId, signal);
    },
    async readCredits(data: DetailProjection, group: 'cast' | 'crew', cursor?: string, signal?: AbortSignal) {
      current(data);
      const query = new URLSearchParams({group, limit: '40', ...(cursor ? {cursor} : {})});
      const path = encodedRoutePath('get_items_id_credits', {id: encodeURIComponent(data.item.id)});
      const page = parseCreditPage(await life.api.request(path + '?' + query, 'GET', undefined, signal), data.item.id, group);
      current(data);
      if (page.revision.catalog !== data.revision.catalog || page.revision.viewer !== data.revision.viewer) throw new DOMException('Credits revision changed', 'AbortError');
      return page;
    },
    mutate: async (data: DetailProjection, action: PersonalIntent) => { current(data); await service.mutate(action); },
    play: async (data: DetailProjection, actionId: string, versionId?: string) => {
      current(data);
      const action = data.actions.find(value => value.id === actionId && value.enabled && value.playback);
      if (!action?.playback || action.playback.itemId !== data.item.id) throw new Error('Title playback action unavailable');
      if (versionId) {
        const offered = await versions(data);
        if (!offered.versions.some(value => value.id === versionId)) throw new Error('Version no longer available');
      }
      current(data);
      await context.effects.play({...action.playback, libraryId: data.scope.libraryId, ...(versionId ? {versionId} : {})}, life.signal());
    },
    dispose() { intent++; selected = undefined; life.dispose(); recommendations.dispose(); service.cancel(); service.dispose(); },
  };
}
export type DetailBindings = ReturnType<typeof createDetailBindings>;
