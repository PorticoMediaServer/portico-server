import {useEffect, useMemo, useRef} from 'react';
import type {ContentEntry} from '@core/library-content.ts';
import type {SearchEntry} from '@core/search.ts';
import type {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import {entryDestination} from '../../shell/navigation';
import type {ShellBindings} from '../../shell/bindings';
import type {ScreenContext} from '../detail/context';

export type ProtectedLibraryScreen = NonNullable<ReturnType<typeof useProtectedWebScreenContext>>;
const content = (entry: SearchEntry): entry is ContentEntry => entry.kind !== 'person' && entry.kind !== 'channel';

/** A current binding can gain its real media owner after Home arrives. The API,
 * viewer and reader remain captured to the original authority throughout. */
export function useLibraryScreenContext(captured: ProtectedLibraryScreen): ScreenContext {
  const binding = useRef<ShellBindings>(captured.binding);
  binding.current = captured.binding;
  const context = captured.context, scopeKey = captured.binding.scopeKey;
  const owner = useMemo(() => {
    const session = context.session(), viewer = session.viewer;
    if (viewer.authority !== 'local' && viewer.authority !== 'hosted') throw new DOMException('Screen authority unavailable', 'AbortError');
    let active = true;
    const current = () => {
      if (!active || !context.isCurrent() || binding.current.scopeKey !== scopeKey) return false;
      const value = context.session();
      return value.sessionFamilyId === session.sessionFamilyId && value.viewer.serverId === viewer.serverId && value.viewer.authority === viewer.authority
        && value.viewer.accountId === viewer.accountId && value.viewer.profileId === viewer.profileId && value.viewer.role === viewer.role;
    };
    const assert = () => {if (!current()) throw new DOMException('Screen authority changed', 'AbortError');};
    const reportError = (error: unknown) => {if (current()) binding.current.home.effects.reportError(error);};
    const navigate = (action: Promise<void>) => {void action.catch(reportError);};
    const value: ScreenContext = {
      api: context.api, events: context.events, artwork: context.artwork, identity: context.facts.identity,
      scope: {serverId: viewer.serverId, viewerId: JSON.stringify([viewer.authority, viewer.accountId, viewer.profileId])}, isCurrent: current,
      preferences: {getSnapshot: () => context.preferences?.getSnapshot() ?? context.facts.preferences,
        subscribe: listener => context.preferences?.subscribe(listener) ?? (() => {})},
      effects: {
        reportError,
        openEntry(entry) {
          assert();
          if (content(entry)) {
            const target = entryDestination(entry);
            navigate(router.navigate(target.to === '/library/$libraryId' && entry.navigation?.category
              ? {...target, search: {...target.search, category: entry.navigation.category}} : target));
          } else if (entry.kind === 'person') navigate(router.navigate({to: '/person/$personId', params: {personId: entry.navigation.entityId}}));
          else navigate(router.navigate({to: '/live', search: {channelId: entry.navigation.entityId}}));
        },
        openHome() {assert();navigate(router.navigate({to: '/'}));},
        openSettings() {assert();navigate(router.navigate({to: '/settings'}));},
        chooseProfile() {assert();navigate(router.navigate({to: '/profiles'}));},
        back() {assert();router.history.back();},
        openBrowse(selector) {assert();navigate(router.navigate({to: '/library/$libraryId', params: {libraryId: selector.libraryId},
          search: {pivot: selector.pivot, query: selector.query, sort: selector.sort}}));},
        async play(target, signal) {
          assert();signal.throwIfAborted();
          const playback = binding.current.playback;
          if (!playback) throw new DOMException('Playback is still loading', 'AbortError');
          await playback.runtime.playFromScreen(target, {signal, isCurrent: current});
          assert();signal.throwIfAborted();
          await router.navigate({to: '/player'});
        },
      },
    };
    return {value, dispose: () => {active = false;}};
  }, [context, scopeKey]);
  useEffect(() => () => owner.dispose(), [owner]);
  return owner.value;
}
