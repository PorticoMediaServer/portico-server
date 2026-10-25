import {useEffect, useMemo, useRef} from 'react';
import type {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import type {ShellBindings} from '../../shell/bindings';
import type {ScreenContext} from '../detail/context';
import {screenEntryDestination} from './navigation';
import {openWatchVersionChooser} from '../watch/version-presentation';
import {createScreenPlayEffect, createScreenSeasonPlayEffect} from './playback-effect';
import {addToDestinationEffect, deleteTitleEffect, entryActions, type EntryActionPorts} from '../../shell/entry-actions';

export type ProtectedLibraryScreen = NonNullable<ReturnType<typeof useProtectedWebScreenContext>>;

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
    const entryPorts: EntryActionPorts = {pickerIdentity: {authority: viewer.authority, accountId: viewer.accountId, profileId: viewer.profileId, events: context.events}, api: context.api, serverId: viewer.serverId, viewerId: JSON.stringify([viewer.authority, viewer.accountId, viewer.profileId]), isCurrent: current, requestId: async () => crypto.randomUUID()};
    const entryEffects = {addToPlaylist: addToDestinationEffect(entryPorts, 'playlist'), addToCollection: addToDestinationEffect(entryPorts, 'collection', viewer.role === 'owner'), ...(viewer.role === 'owner' ? {deleteTitle: deleteTitleEffect(entryPorts)} : {})};
    const value: ScreenContext = {
      api: context.api, events: context.events, artwork: context.artwork, identity: context.facts.identity,
      scope: {serverId: viewer.serverId, viewerId: JSON.stringify([viewer.authority, viewer.accountId, viewer.profileId])}, isCurrent: current,
      preferences: {getSnapshot: () => context.preferences?.getSnapshot() ?? context.facts.preferences,
        subscribe: listener => context.preferences?.subscribe(listener) ?? (() => {})},
      effects: {
        reportError,
        openEntry(entry) {
          assert();
          navigate(router.navigate(screenEntryDestination(entry)));
        },
        openHome() {assert();navigate(router.navigate({to: '/'}));},
        openSettings() {assert();navigate(router.navigate({to: '/settings'}));},
        chooseProfile() {assert();navigate(router.navigate({to: '/profiles'}));},
        back() {assert();router.history.back();},
        openBrowse(selector) {assert();navigate(router.navigate({to: '/library/$libraryId', params: {libraryId: selector.libraryId},
          search: {pivot: selector.pivot, query: selector.query, sort: selector.sort}}));},
        playSeason: createScreenSeasonPlayEffect({isCurrent: current, runtime: () => binding.current.playback?.runtime}),
        play: createScreenPlayEffect({isCurrent: current, runtime: () => binding.current.playback?.runtime}),
        ...entryEffects,
        async chooseVersion(request) {
          assert();request.signal.throwIfAborted();
          await openWatchVersionChooser(context, {...request, isCurrent: () => current() && request.isCurrent()});
        },
      },
    };
    return {value, dispose: () => {active = false; entryActions.retire(entryPorts);}};
  }, [context, scopeKey]);
  useEffect(() => () => owner.dispose(), [owner]);
  return owner.value;
}
