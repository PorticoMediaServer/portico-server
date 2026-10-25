import {defineWebScreen} from '../../route-registry';
import {useState, useSyncExternalStore} from 'react';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {WatchTitleBody, type WatchMyList} from '../detail';
import {WatchPlayback} from './WatchPlayback';
import type {WatchBindings, WatchPanelContext} from './types';
import {WatchBodyOwner, type RetainedWatchBody} from './RetainedBody';
import {watchBodyTarget, watchBodyKey} from './body-target';
import {TogetherOwner} from '../together/owner';
import {WatchVersionPresentationOwner} from './VersionPresentationOwner';
import {useWatchPanels} from './use-watch-panels';
import {useLiveChromePort} from '../live/use-live-chrome';
import {watchEndCard} from './end-card';
import {WatchEpisodesDrawer} from './EpisodesDrawer';
import {watchEpisodes, type WatchEpisodeRow} from './episodes';
import {router} from '../../router';

const noSubscribe = () => () => {};
const noSnapshot = () => undefined;
const emptyList: WatchMyList = Object.freeze({available: false, busy: false});
const noList = () => emptyList;

/** The persistent host renders the live player across routes; the outlet owns no media. */
export function renderWatch() { return null; }
function RegisteredWatchPlayback({binding}: {binding: WatchBindings | undefined}) {
  const captured = useProtectedWebScreenContext();
  const runtime = binding?.runtime;
  const snapshot = useSyncExternalStore(runtime?.subscribe ?? noSubscribe, runtime?.getSnapshot ?? noSnapshot, noSnapshot);
  const target = watchBodyTarget(snapshot), [body, setBody] = useState<RetainedWatchBody | undefined>(undefined);
  const owner = captured && runtime && captured.binding.playback?.runtime === runtime ? captured : undefined;
  const held = body && body.runtime === runtime && body.context.isCurrent() ? body : undefined;
  const panels = useWatchPanels(binding, snapshot, owner?.context);
  const live = useLiveChromePort(owner?.context, runtime, snapshot, binding?.skips ?? {back: 10, forward: 30});
  const list = useSyncExternalStore(held?.binding.myList.subscribe ?? noSubscribe, held?.binding.myList.getSnapshot ?? noList, noList);
  // desktop-player f4b: the retained title read is the only source of the film-end card.
  const detailBinding = held && held.binding.body.kind === 'title' ? held.binding.body.binding : undefined;
  const detail = useSyncExternalStore(detailBinding?.service.subscribe ?? noSubscribe, detailBinding?.service.getSnapshot ?? noSnapshot, noSnapshot);
  const end = held && binding && detailBinding && detail?.data
    ? watchEndCard(detail.data, entry => binding.artwork.foreground(entry.posterUrl, 260, 2 / 3, {group: 'playback'}),
      id => router.navigate({to: '/item/$itemId', params: {itemId: id}}),
      value => detail.data ? detailBinding.mutate(detail.data, {action: 'rating', value}) : Promise.reject(new Error('rating_unavailable')))
    : undefined;
  const matches = !!held && !!target && held.key === watchBodyKey(target);
  // desktop-player f6: the drawer reads the same retained title page the portrait body owns.
  const showBinding = held && held.binding.body.kind === 'show' ? held.binding.body.binding : undefined;
  const show = useSyncExternalStore(showBinding?.service.subscribe ?? noSubscribe, showBinding?.service.getSnapshot ?? noSnapshot, noSnapshot);
  const showData = showBinding && show?.phase === 'ready' ? show.data : null;
  const episodes = binding && held && showData && snapshot?.fence?.kind === 'queue' && matches
    ? watchEpisodes({data: showData, queue: snapshot.queueView, playingItemId: snapshot.fence.itemId,
      positionSeconds: snapshot.presentation?.positionSeconds, durationSeconds: snapshot.presentation?.durationSeconds}) : undefined;
  const playEpisode = (row: WatchEpisodeRow) => {
    if (!binding) return Promise.reject(new DOMException('Playback source retired', 'AbortError'));
    const live = binding.runtime.getSnapshot();
    if (!live.fence || live.fence.kind !== 'queue' || live.disposed) return Promise.reject(new DOMException('Playback source retired', 'AbortError'));
    return binding.runtime.selectEpisode(live.fence, row.itemId);
  };
  const selectSeason = (seasonId: string | null) => {
    if (!showBinding || !showData) return Promise.reject(new DOMException('Title changed', 'AbortError'));
    return showBinding.select({libraryId: showData.scope.libraryId, showId: showData.show.id, ...(seasonId ? {selectedSeasonId: seasonId} : {})});
  };
  const renderPanel = (context: WatchPanelContext) => context.panel === 'episodes' && episodes && binding
    ? <WatchEpisodesDrawer episodes={episodes} context={context} locale={binding.environment.locale}
      artwork={url => binding.artwork.foreground(url, 632, 16 / 9, {group: 'playback'})}
      onPlay={row => playEpisode(row).then(() => context.close())} onSeason={selectSeason} reportError={binding.reportError} />
    : panels?.render(context) ?? null;
  return <>
    {owner && runtime && <WatchBodyOwner captured={owner} runtime={runtime} target={target} onReady={setBody} />}
    <WatchPlayback binding={binding} context={owner?.context} surfaces={held ? {...panels, available: [...(panels?.available ?? []), ...(episodes ? ['episodes' as const] : [])],
      render: renderPanel,
      body: <div hidden={!matches}><WatchTitleBody binding={held.binding} compact /></div>, bodyIdentity: `${captured?.binding.scopeKey}:${held.key}`,
      myList: matches && list.available && typeof list.watchlisted === 'boolean' ? {selected: list.watchlisted, pending: list.busy,
        toggle: () => {const current = held.binding.myList.getSnapshot();if (typeof current.watchlisted !== 'boolean') throw new DOMException('Title state changed', 'AbortError');return held.binding.myList.set(!current.watchlisted);}} : undefined,
      ...(end ? {end} : {}),
    } : panels} live={live} />
    {owner && binding && <TogetherOwner context={owner.context} playback={binding} />}
    {owner && binding && <WatchVersionPresentationOwner context={owner.context} reportError={binding.reportError} />}
  </>;
}

export default defineWebScreen({id: 'player', chrome: 'player', component: renderWatch, playbackHost: RegisteredWatchPlayback});
