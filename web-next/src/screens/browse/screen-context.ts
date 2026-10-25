import {createEntryMetadataEffect, retireEntryMetadata, type EntryMetadataPorts} from '../../shell/entry-metadata';
import {createBrowserDownloadPort} from '../../platform/browser-download';
import {useEffect, useLayoutEffect, useMemo, useRef, useState} from 'react';
import type {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import type {ShellBindings} from '../../shell/bindings';
import type {ScreenContext} from '../detail/context';
import {screenEntryDestination} from './navigation';
import {openWatchVersionChooser} from '../watch/version-presentation';
import {createScreenPlayEffect, createScreenSeasonPlayEffect, createScreenArtistPlayEffect, createScreenAlbumPlayEffect, createScreenRadioPlayEffect} from './playback-effect';
import {addToDestinationEffect, deleteTitleEffect, entryActions} from '../../shell/entry-actions';

export type ProtectedLibraryScreen = NonNullable<ReturnType<typeof useProtectedWebScreenContext>>;

function libraryScreenOwner(context: ProtectedLibraryScreen['context'], scopeKey: string, getBinding: () => ShellBindings, callerCurrent: () => boolean = () => true) {
  const session = context.session(), viewer = session.viewer;
  if (viewer.authority !== 'local' && viewer.authority !== 'hosted') throw new DOMException('Screen authority unavailable', 'AbortError');
  let active = true;
  const downloadLifetime = new AbortController();
  const current = () => {
    if (!active || !callerCurrent() || !context.isCurrent() || getBinding().scopeKey !== scopeKey) return false;
    const value = context.session();
    return value.sessionFamilyId === session.sessionFamilyId && value.viewer.serverId === viewer.serverId && value.viewer.authority === viewer.authority
      && value.viewer.accountId === viewer.accountId && value.viewer.profileId === viewer.profileId && value.viewer.role === viewer.role;
  };
  const assert = () => {if (!current()) throw new DOMException('Screen authority changed', 'AbortError');};
  const reportError = (error: unknown) => {if (current()) getBinding().home.effects.reportError(error);};
  const navigate = (action: Promise<void>) => {void action.catch(reportError);};
  const entryPorts: EntryMetadataPorts = {artwork: context.artwork,pickerIdentity: {authority: viewer.authority, accountId: viewer.accountId, profileId: viewer.profileId, events: context.events}, api: context.api, serverId: viewer.serverId, viewerId: JSON.stringify([viewer.authority, viewer.accountId, viewer.profileId]), isCurrent: current, requestId: async () => crypto.randomUUID()};
  const entryEffects = {...(viewer.authority === 'local' && viewer.role === 'owner' ? {editMetadata: createEntryMetadataEffect(entryPorts, () => current() && context.session().viewer.role === 'owner' && context.session().viewer.authority === 'local')} : {}), addToPlaylist: addToDestinationEffect(entryPorts, 'playlist'), addToCollection: addToDestinationEffect(entryPorts, 'collection', viewer.role === 'owner'), ...(viewer.role === 'owner' ? {deleteTitle: deleteTitleEffect(entryPorts)} : {})};
  const value: ScreenContext = {
    api: context.api, events: context.events, artwork: context.artwork, identity: context.facts.identity,
    scope: {serverId: viewer.serverId, viewerId: JSON.stringify([viewer.authority, viewer.accountId, viewer.profileId])}, isCurrent: current,
    preferences: {getSnapshot: () => context.preferences?.getSnapshot() ?? context.facts.preferences,
      subscribe: listener => context.preferences?.subscribe(listener) ?? (() => {})},
    effects: {
      download: createBrowserDownloadPort(context.api, current, downloadLifetime.signal),
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
      playSeason: createScreenSeasonPlayEffect({isCurrent: current, runtime: () => getBinding().playback?.runtime}),
      playArtist: createScreenArtistPlayEffect({isCurrent: current, runtime: () => getBinding().playback?.runtime}),
      playAlbum: createScreenAlbumPlayEffect({isCurrent: current, runtime: () => getBinding().playback?.runtime}),
      playRadio: createScreenRadioPlayEffect({isCurrent: current, runtime: () => getBinding().playback?.runtime}),
      play: createScreenPlayEffect({isCurrent: current, runtime: () => getBinding().playback?.runtime}),
      ...entryEffects,
      async chooseVersion(request) {
        assert();request.signal.throwIfAborted();
        await openWatchVersionChooser(context, {...request, isCurrent: () => current() && request.isCurrent()});
      },
    },
  };
  return {value, dispose: () => {active = false; downloadLifetime.abort(); entryActions.retire(entryPorts); retireEntryMetadata(entryPorts);}};
}

/** A current binding can gain its real media owner after Home arrives. The API,
 * viewer and reader remain captured to the original authority throughout. */
export function useLibraryScreenContext(captured: ProtectedLibraryScreen): ScreenContext {
  const binding = useRef<ShellBindings>(captured.binding);
  binding.current = captured.binding;
  const context = captured.context, scopeKey = captured.binding.scopeKey;
  const owner = useMemo(() => libraryScreenOwner(context, scopeKey, () => binding.current), [context, scopeKey]);
  useEffect(() => () => owner.dispose(), [owner]);
  return owner.value;
}

/** Fresh committed context for generic Browse. The original binding/runtime and
 * route own all effects; a successor never revives an already retired owner.
 * Existing callers keep the previous hook until their own areas migrate. */
export function useCommittedLibraryScreenContext(captured: ProtectedLibraryScreen, routeKey: string, routeCurrent: () => boolean): ScreenContext | undefined {
  const context = captured.context, binding = captured.binding;
  const [published, publish] = useState<Readonly<{context: typeof context; binding: ShellBindings; routeKey: string; owner: ReturnType<typeof libraryScreenOwner>}>>();
  useLayoutEffect(() => {
    const owner = libraryScreenOwner(context, binding.scopeKey, () => binding, routeCurrent);
    publish({context, binding, routeKey, owner});
    return owner.dispose;
    // routeCurrent belongs to this exact route key and is never adopted later.
  }, [context, binding, routeKey]);
  return published?.context === context && published.binding === binding && published.routeKey === routeKey && published.owner.value.isCurrent()
    ? published.owner.value : undefined;
}
