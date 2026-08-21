import { useEffect, useMemo, useState } from 'react';
import { usePorticoDataSource } from '../../data/DataProvider';
import type {
  DVRConsumerStatus,
  ExtendedLiveTVDataSource,
  FeatureQueryState,
  LiveTVChannelPage,
  LiveTVChannelPageInput,
  LiveTVGuidePageInput,
  LiveTVGuideWorkspacePage,
} from './liveTypes';

function useFeatureQuery<T>(key: string, load: (signal: AbortSignal) => Promise<T>, enabled = true): FeatureQueryState<T> {
  const [state, setState] = useState<FeatureQueryState<T>>({ status: 'loading' });
  useEffect(() => {
    if (!enabled) {
      setState({ status: 'loading' });
      return;
    }
    const controller = new AbortController();
    setState({ status: 'loading' });
    load(controller.signal).then(
      (data) => !controller.signal.aborted && setState({ status: 'success', data }),
      (reason: unknown) => !controller.signal.aborted && setState({
        status: 'error',
        error: reason instanceof Error ? reason : new Error('Portico request failed.'),
      }),
    );
    return () => controller.abort();
  }, [enabled, key, load]);
  return state;
}

export function useGuidePage(sourceId: string, input: LiveTVGuidePageInput, revision: number) {
  const source = usePorticoDataSource() as ExtendedLiveTVDataSource;
  const supportsPaging = typeof source.liveTVGuidePage === 'function';
  const key = `${sourceId}:${JSON.stringify(input)}:${revision}:${supportsPaging}`;
  const load = useMemo(() => (signal: AbortSignal) => {
    if (!sourceId) return Promise.reject(new Error('No Live TV source is selected.'));
    if (source.liveTVGuidePage) return source.liveTVGuidePage(sourceId, input, signal);
    return source.liveTVGuide(sourceId, input, signal) as Promise<LiveTVGuideWorkspacePage>;
  }, [key, source]);
  return { query: useFeatureQuery<LiveTVGuideWorkspacePage>(key, load), supportsPaging };
}

export function useChannelPage(sourceId: string, input: LiveTVChannelPageInput, revision: number, enabled = true) {
  const source = usePorticoDataSource() as ExtendedLiveTVDataSource;
  const supportsPaging = typeof source.liveTVChannelsPage === 'function';
  const key = `${sourceId}:${JSON.stringify(input)}:${revision}:${supportsPaging}`;
  const load = useMemo(() => async (signal: AbortSignal): Promise<LiveTVChannelPage> => {
    if (!sourceId) throw new Error('No Live TV source is selected.');
    if (source.liveTVChannelsPage) return source.liveTVChannelsPage(sourceId, input, signal);
    const channels = await source.liveTVChannels(sourceId, signal);
    const query = input.query?.trim().toLocaleLowerCase() ?? '';
    const items = channels.filter((channel) => {
      if (input.favoritesOnly && !channel.favorite) return false;
      if (input.group && input.group !== 'all' && channel.groupTitle !== input.group) return false;
      return !query || `${channel.number ?? ''} ${channel.name} ${channel.groupTitle ?? ''}`.toLocaleLowerCase().includes(query);
    });
    return {
      items,
      pageInfo: { nextCursor: null, hasMore: false, total: items.length },
      groups: [...new Set(channels.flatMap((channel) => channel.groupTitle ? [channel.groupTitle] : []))].sort(),
    };
  }, [key, source]);
  return { query: useFeatureQuery<LiveTVChannelPage>(key, load, enabled), supportsPaging };
}

export function useDVRStatus(sourceId: string | undefined, revision: number) {
  const source = usePorticoDataSource() as ExtendedLiveTVDataSource;
  const supported = typeof source.dvrStatus === 'function';
  const key = `${sourceId ?? ''}:${revision}:${supported}`;
  const load = useMemo(() => (signal: AbortSignal) => source.dvrStatus
    ? source.dvrStatus(sourceId, signal)
    : Promise.reject(new Error('DVR operational status is not supported by this server.')), [key, source]);
  return { query: useFeatureQuery<DVRConsumerStatus>(key, load), supported };
}
