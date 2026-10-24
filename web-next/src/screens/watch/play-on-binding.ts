import type {GetItemsIdResponse} from '../../../../packages/contracts/src/types.ts';
import {encodedRoutePath} from '../../../../packages/contracts/src/routes.ts';
import {parseMediaItem} from '@core/media-item.ts';
import type {CastMetadata} from '@core/cast/index.ts';
import {WebCastSender} from '../../platform/cast';
import {PlayOnController, mediaAirPlayPort} from '../../platform/playback/play-on';
import type {ShellAssemblyContext} from '../../shell/bootstrap';
import type {WatchBindings} from './types';

export type WatchPlaybackContext = Pick<ShellAssemblyContext, 'api' | 'session' | 'isCurrent' | 'epoch'>;
/** Created only by the mounted persistent host. The same real video owns AirPlay. */
export function createWatchPlayOn(context: WatchPlaybackContext, binding: WatchBindings, video: HTMLVideoElement, applicationId?: string) {
  const life = new AbortController(), authority = binding.runtime.getSnapshot().authority;
  const items = new Map<string, GetItemsIdResponse>();
  const isCurrent = () => {
    const viewer = context.session().viewer;
    return !life.signal.aborted && context.isCurrent() && !binding.runtime.getSnapshot().disposed && context.epoch === authority.epoch
      && viewer.serverId === authority.serverId && viewer.authority === authority.authority && viewer.accountId === authority.accountId && viewer.profileId === authority.profileId;
  };
  const assertCurrent = () => {if (!isCurrent()) throw new DOMException('Playback authority retired', 'AbortError');};
  const api = {async request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T> {
    assertCurrent(); const active = signal ? AbortSignal.any([signal, life.signal]) : life.signal;
    const value = await context.api.request<T>(path, method, body, active); assertCurrent();active.throwIfAborted();return value;
  }};
  const describeItem = async (itemId: string): Promise<CastMetadata> => {
    const raw = await api.request<GetItemsIdResponse>(encodedRoutePath('get_items_id', {id: encodeURIComponent(itemId)}));
    const item = parseMediaItem(raw);
    if (item.id !== itemId) throw new Error('cast_item_mismatch');
    items.set(itemId, raw);
    while (items.size > 4) items.delete(items.keys().next().value!);
    return {title: item.title, kind: item.kind, artworkPath: item.posterUrl ?? item.backdropUrl,
      ...(raw.episode ? {seriesTitle: raw.episode.showTitle, season: raw.episode.seasonNumber, episode: raw.episode.number} : {})};
  };
  const sender = new WebCastSender({api, serverOrigin: context.api.baseUrl, castApplicationId: applicationId,
    describe: async itemId => (await describeItem(itemId)).title});
  const controller = new PlayOnController({sender, runtime: binding.runtime, airPlay: mediaAirPlayPort(video, undefined, {
    isCurrent: () => {const shown = binding.runtime.getSnapshot();return isCurrent() && !!shown.fence && binding.runtime.browserMediaReady(shown.fence);},
    subscribe: binding.runtime.subscribe,
  }), isCurrent, describeItem, reportError: binding.reportError});
  return {controller, item: (id: string) => isCurrent() ? items.get(id) : undefined,
    dispose: () => {life.abort();controller.dispose();items.clear();}};
}
export type WatchPlayOnBinding = ReturnType<typeof createWatchPlayOn>;
