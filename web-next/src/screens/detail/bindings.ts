import {DetailService, readFileInfo, type DetailProjection, type DetailTarget, type FileInfoProjection, type PersonalIntent} from '@core/detail.ts';
import {PlaybackOptionsClient} from '@core/playback-v1/options.ts';
import {localV1Http} from '@core/playback-v1/local-http.ts';
import {parseMediaItem} from '@core/media-item.ts';
import {encodedRoutePath} from '../../../../packages/contracts/src/routes.ts';
import {screenLifetime, type ScreenContext} from './context';
import {createRecommendations} from './recommendations';

/** Constructor is inert. Only select/readFileInfo/readVersions perform reads. */
export function createDetailBindings(context: ScreenContext) {
  const life = screenLifetime(context);
  const service = new DetailService({api: life.api, scope: context.scope, requestId: async () => { life.assert(); return crypto.randomUUID(); }});
  const options = new PlaybackOptionsClient(localV1Http(life.api));
  const recommendations = createRecommendations(context);
  let selected: DetailTarget | undefined, intent = 0;
  const current = (data: DetailProjection) => {
    life.assert();
    if (service.getSnapshot().phase !== 'ready' || service.getSnapshot().data !== data) throw new DOMException('Title revision changed', 'AbortError');
  };
  const select = async (target: DetailTarget | Readonly<{itemId: string}>) => {
    life.assert(); const generation = ++intent;
    service.cancel(); recommendations.clear(); selected = undefined;
    // The real item reader supplies library identity for a bare /item/$itemId route.
    const libraryId = 'libraryId' in target ? target.libraryId : parseMediaItem(await life.api.request(encodedRoutePath('get_items_id', {id: encodeURIComponent(target.itemId)}))).libraryId;
    life.assert(); if (generation !== intent) return;
    selected = {libraryId, itemId: target.itemId};
    await service.select(selected);
  };
  const files = async (data: DetailProjection, cursor?: string): Promise<FileInfoProjection> => {
    current(data);
    const value = await readFileInfo(life.api, context.scope, data.scope, {cursor, limit: 40}, life.signal());
    current(data);
    if (value.scope.viewerFence !== data.scope.viewerFence || value.revision.catalog !== data.revision.catalog || value.revision.viewer !== data.revision.viewer) throw new DOMException('File facts changed', 'AbortError');
    return value;
  };
  const versions = async (data: DetailProjection) => {
    current(data); const value = await options.options(data.item.id, {}, life.signal()); current(data);
    if (value.itemId !== data.item.id) throw new Error('Playback options target mismatch');
    return value;
  };
  return {
    context, service, select, current, recommendations,
    readFileInfo: files, readVersions: versions,
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
