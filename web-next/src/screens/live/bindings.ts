import {GuideWindowStore, legacyGuideSource} from '@core/guide/index.ts';
import {fetchGuideSourceSummaries} from '@core/guide/directory.ts';
import type {LegacyGuideOptions} from '@core/guide/legacy-source.ts';
import {screenLifetime, type ScreenContext} from '../detail/context';

/** Only the mounted viewport starts reads. No source listing or guide walk at import. */
export function createLiveBindings(context: ScreenContext, options: LegacyGuideOptions) {
  const life = screenLifetime(context);
  let store: GuideWindowStore;
  let failure: unknown;
  const listeners = new Set<() => void>();
  const api = {async request<T>(...args: Parameters<typeof life.api.request>) {
    try {return await life.api.request<T>(...args);}
    catch (error) {
      const status = (error as {status?: number})?.status;
      if (status === 401 || status === 403) {failure = error;life.dispose();store?.dispose();for (const listener of listeners) listener();}
      throw error;
    }
  }};
  const source = legacyGuideSource(api, context.scope.serverId, Object.freeze({...options}));
  store = new GuideWindowStore({source, channelPageSize: 50, maxChannelPages: 6, maxTiles: 12, maxInFlight: 1, marginPages: 0, marginBlocks: 0});
  return {store, life, failure: {getSnapshot: () => failure, subscribe: (listener: () => void) => {listeners.add(listener);return () => {listeners.delete(listener);};}},
    async sources() {
      const value = await fetchGuideSourceSummaries(api, context.scope.serverId, options.timezone, life.signal());
      life.assert();return value;
    },
    dispose() {life.dispose();store.dispose();listeners.clear();failure = undefined;},
  };
}
export type LiveBindings = ReturnType<typeof createLiveBindings>;
