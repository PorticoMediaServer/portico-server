import {DetailService, readFileInfo, parseCreditPage, type DetailProjection, type DetailTarget, type FileInfoProjection, type PersonalIntent} from '@core/detail.ts';
import {readListeningSimilar} from '@core/library-content.ts';
import {PlaybackOptionsClient} from '@core/playback-v1/options.ts';
import {localV1Http} from '@core/playback-v1/local-http.ts';
import {parseMediaItem} from '@core/media-item.ts';
import {encodedRoutePath} from '../../../../packages/contracts/src/routes.ts';
import {screenLifetime, type ScreenContext} from './context';
import {createRecommendations} from './recommendations';
import {versionDeletionService, deletionAllowed, type VersionDeletePreview} from '@core/administration.ts';
import {findLibraryFile, libraryFolders, folderOfPath, separateLibraryFile, canMergeTitles, mergeTitles, SPLITTABLE_KINDS} from '@core/title-identity.ts';
import {validateContentEntry, type ContentEntry} from '@core/library-content.ts';
import {titleView} from './view-state';
import {openGenre} from './genre-binding';
import type {PlaybackOptions} from '@core/playback-v1/types.ts';
import type {RefObject} from 'react';

/** Constructor is inert. Only select/readFileInfo/readVersions perform reads. */
export function createDetailBindings(context: ScreenContext) {
  const life = screenLifetime(context);
  const service = new DetailService({api: life.api, scope: context.scope, requestId: async () => { life.assert(); return crypto.randomUUID(); }});
  const options = new PlaybackOptionsClient(localV1Http(life.api));
  const versionReads = new WeakMap<PlaybackOptions, DetailProjection>();
  const recommendations = createRecommendations(context);
  const view = titleView('related', ['related', 'details', 'extras'], life.assert);
  let viewIdentity = '';
  let selected: DetailTarget | undefined, intent = 0, disposed = false;
  let subject: Readonly<{itemId: string; libraryId: string; title: string}> | undefined;
  const offSubject = service.subscribe(() => { const data = service.getSnapshot().data; if (data && service.getSnapshot().phase === 'ready') subject = {itemId: data.item.id, libraryId: data.scope.libraryId, title: data.item.title}; });
  /** Unmount cleanup may follow the route's dispose; it must never revive a retired read. */
  const cancel = () => { if (!disposed) service.cancel(); };
  const fileReads = new WeakSet<FileInfoProjection>();
  const fileContexts = new WeakMap<DetailProjection, FileInfoProjection>();
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
    const identity = JSON.stringify([libraryId, target.itemId]);
    if (identity !== viewIdentity) { viewIdentity = identity; view.reset(); }
    selected = {libraryId, itemId: target.itemId};
    const cancel = () => { if (generation === intent) service.cancel(); }; signal?.addEventListener('abort', cancel, {once: true});
    try { await service.select(selected); signal?.throwIfAborted(); } finally { signal?.removeEventListener('abort', cancel); }
  };
  const files = async (data: DetailProjection, cursor?: string, signal?: AbortSignal): Promise<FileInfoProjection> => {
    current(data);
    const value = await readFileInfo(life.api, context.scope, data.scope, {cursor, limit: 40}, life.signal(signal));
    current(data);
    // FileInfo uses the server's Home authority fence; detail has its own fence.
    // Their library revisions agree, while continuations stay in FileInfo's own domain.
    if (value.revision.catalog !== data.revision.catalog || value.revision.viewer !== data.revision.viewer) throw new DOMException('File facts changed', 'AbortError');
    const before = fileContexts.get(data);
    if (cursor && (!before || value.scope.viewerFence !== before.scope.viewerFence)) throw new DOMException('File authority changed', 'AbortError');
    fileContexts.set(data, value);
    fileReads.add(value);
    return value;
  };
  const authorizeFileCommand = async (data: DetailProjection, captured: FileInfoProjection, action: 'delete' | 'split' | 'merge', fileId?: string, signal?: AbortSignal) => {
    current(data); signal?.throwIfAborted();
    if (!fileReads.has(captured) || !captured.manage?.[action] || captured.scope.itemId !== data.item.id) throw new DOMException('File action unavailable', 'AbortError');
    // The panel is informational. Its old permission bit cannot authorize a later command.
    const fresh = await files(data, undefined, signal);
    if (fresh.scope.viewerFence !== captured.scope.viewerFence) throw new DOMException('File authority changed', 'AbortError');
    if (!fresh.manage?.[action]) throw new DOMException('File permission changed', 'AbortError');
    if (fileId && !fresh.files.some(file => file.id === fileId)) throw new DOMException('File changed', 'AbortError');
    return fresh;
  };
  const commandApi = (signal?: AbortSignal) => ({request: <T,>(path: string, method?: string, body?: unknown) => life.api.request<T>(path, method, body, signal)});
  const resolveEntry = async (itemId: string, signal?: AbortSignal) => {
    const item = parseMediaItem(await life.api.request(encodedRoutePath('get_items_id', {id: encodeURIComponent(itemId)}), 'GET', undefined, signal));
    return validateContentEntry({id: item.id, libraryId: item.libraryId, kind: item.kind, title: item.title, posterUrl: item.posterUrl, backdropUrl: item.backdropUrl, navigation: {view: 'item', entityId: item.id}});
  };
  /** music-plus f7 (MC-9): the album's own More like this, ranked by sound when the server has
   * analyzed it and by today's rule otherwise. The same captured page, api and revisions. */
  const similar = async (data: DetailProjection, signal?: AbortSignal) => {
    current(data);
    const viewer = context.identity?.viewer;
    if (!viewer || (viewer.authority !== 'local' && viewer.authority !== 'hosted')) throw new DOMException('Title authority changed', 'AbortError');
    const value = await readListeningSimilar(life.api, {serverId: context.scope.serverId, authority: viewer.authority, accountId: viewer.accountId, profileId: viewer.profileId,
      controllerId: 'v1', controllerEpoch: 'v1', commandLaneId: 'v1'}, data.item.id, {limit: 12, by: 'sound', signal: life.signal(signal)});
    current(data);
    if (value.itemId !== data.item.id || value.libraryId !== data.item.libraryId || value.revision.catalog !== data.revision.catalog || value.revision.viewer !== data.revision.viewer) throw new DOMException('Title recommendations changed', 'AbortError');
    return value;
  };
  const versions = async (data: DetailProjection, signal?: AbortSignal) => {
    current(data); const value = await options.options(data.item.id, {}, life.signal(signal)); current(data);
    if (value.itemId !== data.item.id) throw new Error('Playback options target mismatch');
    versionReads.set(value, data);
    return value;
  };
  const binding = {
    context, service, select, current, recommendations, view,
    subject() { const target = service.getSnapshot().target; return life.current() && target?.itemId === subject?.itemId && target?.libraryId === subject?.libraryId ? subject?.title : undefined; },
    readFileInfo: files, readVersions: versions, readSimilar: similar,
    async chooseVersion(data: DetailProjection, captured: PlaybackOptions, signal: AbortSignal, returnFocusRef?: RefObject<HTMLElement | null>): Promise<void> {
      current(data);signal.throwIfAborted();
      const action = data.actions.find(value => value.enabled && value.playback?.itemId === data.item.id && value.id !== 'start_over');
      if (!context.effects.chooseVersion || !action || versionReads.get(captured) !== data || captured.itemId !== data.item.id || captured.versions.length < 2) throw new DOMException('Version choice unavailable', 'AbortError');
      const isCurrent = () => {try {current(data);return !signal.aborted;} catch {return false;}};
      await context.effects.chooseVersion({subject: {libraryId: data.scope.libraryId, itemId: data.item.id, title: data.item.title}, options: captured,
        signal: life.signal(signal), isCurrent, returnFocusRef,
        playVersion: (versionId, active) => binding.play(data, action.id, versionId, active)});
    },
    browseGenre(data: DetailProjection, genre: string) {
      current(data);
      if (!data.metadata.genres.some(value => value.name === genre)) throw new DOMException('Title genre changed', 'AbortError');
      return openGenre(context, life.api, data.scope.libraryId, genre, () => current(data), life.signal());
    },
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
    play: async (data: DetailProjection, actionId: string, versionId?: string, signal?: AbortSignal) => {
      current(data); signal?.throwIfAborted();
      const action = data.actions.find(value => value.id === actionId && value.enabled && value.playback);
      if (!action?.playback || action.playback.itemId !== data.item.id) throw new Error('Title playback action unavailable');
      let startSeconds = action.playback.startSeconds;
      if (versionId) {
        const offered = await versions(data, signal);
        if (!offered.versions.some(value => value.id === versionId)) throw new Error('Version no longer available');
        const positionMs = offered.resume?.positionMs ?? 0;
        if (!Number.isFinite(positionMs) || positionMs < 0) throw new Error('Unreadable title resume position');
        startSeconds = positionMs / 1000;
      }
      current(data);
      await context.effects.play({...action.playback, startSeconds, libraryId: data.scope.libraryId, ...(versionId ? {versionId} : {})}, life.signal(signal),
        () => life.current() && service.getSnapshot().phase === 'ready' && service.getSnapshot().data === data);
    },
    dispose() { if (disposed) return; disposed = true; intent++; selected = undefined; subject = undefined; offSubject(); viewIdentity = ''; life.dispose(); view.dispose(); recommendations.dispose(); service.cancel(); service.dispose(); },
  };
  return {...binding, cancel};
}
export type DetailBindings = ReturnType<typeof createDetailBindings>;
