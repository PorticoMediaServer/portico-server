import {ShowWorkspaceService, saveShowSettings, type ShowSettings, type ShowWorkspaceData, type ShowWorkspaceRoute} from '@core/show-workspace.ts';
import type {ContentEntry} from '@core/library-content.ts';
import {createDetailBindings} from './bindings';
import {screenLifetime, type ScreenContext} from './context';
import {getContainerState, setContainerFlag, setContainerState, type ContainerState} from '@core/container-state.ts';
import {createRecommendations} from './recommendations';

export function createShowBindings(context: ScreenContext) {
  const life = screenLifetime(context), service = new ShowWorkspaceService({api: life.api, scope: context.scope});
  const episode = createDetailBindings(context), primary = createDetailBindings(context);
  const recommendations = createRecommendations(context);
  let personal: ContainerState | undefined, busy = false, intent = 0;
  let selection: AbortController | undefined;
  const listeners = new Set<() => void>(), publish = () => { for (const listener of listeners) listener(); };
  const current = (data: ShowWorkspaceData) => {
    life.assert();
    if (service.getSnapshot().phase !== 'ready' || service.getSnapshot().data !== data) throw new DOMException('Show revision changed', 'AbortError');
  };
  return {
    context, service, episode, primary, current, recommendations,
    personal: {getSnapshot: () => personal, getBusy: () => busy, subscribe: (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; }},
    async select(route: ShowWorkspaceRoute, signal?: AbortSignal) {
      life.assert(); signal?.throwIfAborted(); const generation = ++intent;
      selection?.abort(); const controller = new AbortController(); selection = controller;
      const active = life.signal(signal ? AbortSignal.any([controller.signal, signal]) : controller.signal);
      const cancel = () => { if (generation === intent) { service.cancel(); episode.service.cancel(); primary.service.cancel(); } };
      active.addEventListener('abort', cancel, {once: true});
      recommendations.clear(); personal = undefined; publish(); episode.service.cancel(); primary.service.cancel();
      try {
        await service.select(route); life.assert(); active.throwIfAborted(); if (generation !== intent) return;
        const data = service.getSnapshot().data;
        if (data) { const value = await getContainerState(life.api, 'show', data.show.id, active); if (generation !== intent) return; current(data); personal = value; publish(); }
        if (data?.nextUp?.libraryId && data.nextUp.playback) await primary.select({libraryId: data.nextUp.libraryId, itemId: data.nextUp.playback.itemId}, active);
      } finally { active.removeEventListener('abort', cancel); }
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
    async revalidate(refresh?: Parameters<ShowWorkspaceService['revalidate']>[0]) {
      life.assert(); await service.revalidate(refresh);
      const data = service.getSnapshot().data; if (!data) return;
      const value = await getContainerState(life.api, 'show', data.show.id, life.signal(refresh?.signal)); current(data); personal = value; publish();
      const nextUp = data.nextUp;
      if (nextUp?.libraryId && nextUp.playback && primary.service.getSnapshot().target?.itemId !== nextUp.playback.itemId) await primary.select({libraryId: nextUp.libraryId, itemId: nextUp.playback.itemId}, refresh?.signal);
      else await primary.service.revalidate(refresh);
    },
    async setMyList(data: ShowWorkspaceData, value: boolean) {
      current(data); const before = personal; if (!before || busy) return; busy = true; publish();
      try { const result = await setContainerFlag(life.api, 'show', data.show.id, 'watchlisted', value, life.signal()); current(data); if (before === personal) { personal = result; publish(); } }
      finally { busy = false; publish(); }
    },
    async setWatched(data: ShowWorkspaceData, value: boolean) {
      current(data); const before = personal; if (!before || busy) return; busy = true; publish();
      try { const result = await setContainerState(life.api, 'show', data.show.id, {expectedRevision: before.revision, watched: value}, life.signal()); current(data); if (before === personal) { personal = result; publish(); } await service.refresh(); }
      catch (error) {
        if (life.current() && (error as {status?: number})?.status === 412) { const latest = await getContainerState(life.api, 'show', data.show.id, life.signal()); current(data); personal = latest; publish(); }
        throw error;
      }
      finally { busy = false; publish(); }
    },
    canManage() { return life.current() && context.identity?.primaryProfile === true && context.identity.capabilities?.manageServer === true; },
    async setSettings(data: ShowWorkspaceData, settings: ShowSettings, signal?: AbortSignal) {
      current(data);
      if (context.identity?.primaryProfile !== true || context.identity.capabilities?.manageServer !== true || settings.revision !== data.settings.revision) throw new DOMException('Show management changed', 'AbortError');
      const api = {request: <T,>(path: string, method?: string, body?: unknown) => life.api.request<T>(path, method, body, signal)};
      // The canonical endpoint authorizes this command inside the revision-checked transaction.
      await saveShowSettings(api, data.scope.libraryId, data.show.id, settings);
      current(data); signal?.throwIfAborted(); await service.refresh();
    },
    cancel() { intent++; selection?.abort(); service.cancel(); episode.service.cancel(); primary.service.cancel(); recommendations.clear(); personal = undefined; publish(); },
    dispose() { intent++; selection?.abort(); life.dispose(); recommendations.dispose(); personal = undefined; busy = false; publish(); listeners.clear(); episode.dispose(); primary.dispose(); service.cancel(); service.dispose(); },
  };
}
export type ShowBindings = ReturnType<typeof createShowBindings>;
