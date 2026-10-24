import {defineWebScreen} from '../../route-registry';
import {useState, useSyncExternalStore} from 'react';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {WatchTitleBody, type WatchMyList} from '../detail';
import {WatchPlayback} from './WatchPlayback';
import type {WatchBindings} from './types';
import {WatchBodyOwner, type RetainedWatchBody} from './RetainedBody';
import {watchBodyTarget, watchBodyKey} from './body-target';
import {TogetherOwner} from '../together/owner';
import {WatchVersionPresentationOwner} from './VersionPresentationOwner';
import {useWatchPanels} from './use-watch-panels';

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
  const list = useSyncExternalStore(held?.binding.myList.subscribe ?? noSubscribe, held?.binding.myList.getSnapshot ?? noList, noList);
  const matches = !!held && !!target && held.key === watchBodyKey(target);
  return <>
    {owner && runtime && <WatchBodyOwner captured={owner} runtime={runtime} target={target} onReady={setBody} />}
    <WatchPlayback binding={binding} context={owner?.context} surfaces={held ? {...panels, available: panels?.available ?? [], render: panels?.render ?? (() => null),
      body: <div hidden={!matches}><WatchTitleBody binding={held.binding} compact /></div>, bodyIdentity: `${captured?.binding.scopeKey}:${held.key}`,
      myList: matches && list.available && typeof list.watchlisted === 'boolean' ? {selected: list.watchlisted, pending: list.busy,
        toggle: () => {const current = held.binding.myList.getSnapshot();if (typeof current.watchlisted !== 'boolean') throw new DOMException('Title state changed', 'AbortError');return held.binding.myList.set(!current.watchlisted);}} : undefined,
    } : panels} />
    {owner && binding && <TogetherOwner context={owner.context} playback={binding} />}
    {owner && binding && <WatchVersionPresentationOwner context={owner.context} reportError={binding.reportError} />}
  </>;
}

export default defineWebScreen({id: 'player', chrome: 'player', component: renderWatch, playbackHost: RegisteredWatchPlayback});
