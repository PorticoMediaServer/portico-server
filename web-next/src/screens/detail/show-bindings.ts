import {ShowWorkspaceService, type ShowWorkspaceData, type ShowWorkspaceRoute} from '@core/show-workspace.ts';
import type {ContentEntry} from '@core/library-content.ts';
import {createDetailBindings} from './bindings';
import {screenLifetime, type ScreenContext} from './context';
import {getContainerState, setContainerFlag, setContainerState, type ContainerState} from '@core/container-state.ts';
import {createRecommendations} from './recommendations';

export function createShowBindings(context: ScreenContext) {
  const life = screenLifetime(context), service = new ShowWorkspaceService({api: life.api, scope: context.scope});
  const episode = createDetailBindings(context), primary = createDetailBindings(context);
  const recommendations = createRecommendations(context);
  let personal: ContainerState | undefined;
  const listeners = new Set<() => void>(), publish = () => { for (const listener of listeners) listener(); };
  const current = (data: ShowWorkspaceData) => {
    life.assert();
    if (service.getSnapshot().phase !== 'ready' || service.getSnapshot().data !== data) throw new DOMException('Show revision changed', 'AbortError');
  };
  return {
    context, service, episode, primary, current, recommendations,
    personal: {getSnapshot: () => personal, subscribe: (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; }},
    async select(route: ShowWorkspaceRoute) {
      life.assert(); recommendations.clear(); personal = undefined; publish(); episode.service.cancel(); primary.service.cancel();
      await service.select(route); life.assert();
      const data = service.getSnapshot().data;
      if (data) { const value = await getContainerState(life.api, 'show', data.show.id, life.signal()); current(data); personal = value; publish(); }
      if (data?.nextUp?.libraryId && data.nextUp.playback) await primary.select({libraryId: data.nextUp.libraryId, itemId: data.nextUp.playback.itemId});
    },
    async expand(data: ShowWorkspaceData, entry: ContentEntry) {
      current(data);
      if (!data.episodes.sections.some(section => section.entries.includes(entry))) throw new DOMException('Episode changed', 'AbortError');
      const anchor = service.getSnapshot().anchor;
      if (anchor.expandedEpisodeId === entry.id) { service.updateAnchor({...anchor, expandedEpisodeId: null}); episode.service.cancel(); return; }
      service.updateAnchor({...anchor, expandedEpisodeId: entry.id, focusedEpisodeId: entry.id});
      await episode.select({libraryId: data.scope.libraryId, itemId: entry.id});
    },
    closeEpisode() { life.assert(); service.updateAnchor({...service.getSnapshot().anchor, expandedEpisodeId: null}); episode.service.cancel(); },
    async setMyList(data: ShowWorkspaceData, value: boolean) { current(data); const before = personal; if (!before) return; const result = await setContainerFlag(life.api, 'show', data.show.id, 'watchlisted', value, life.signal()); current(data); if (before !== personal) return; personal = result; publish(); },
    async setWatched(data: ShowWorkspaceData, value: boolean) { current(data); const before = personal; if (!before) return; const result = await setContainerState(life.api, 'show', data.show.id, {expectedRevision: before.revision, watched: value}, life.signal()); current(data); if (before !== personal) return; personal = result; publish(); await service.refresh(); },
    dispose() { life.dispose(); recommendations.dispose(); personal = undefined; publish(); listeners.clear(); episode.dispose(); primary.dispose(); service.cancel(); service.dispose(); },
  };
}
export type ShowBindings = ReturnType<typeof createShowBindings>;
