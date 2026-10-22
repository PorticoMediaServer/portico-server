import type {ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import {HomeSelectionController, homeSelectionContext, type HomeSelectionPlaybackPort} from '@core/home-selection/index.ts';
import {homeChannelSelection, homeCollectionSelection, homeEntryCapabilities, homeEntrySelection, type HomeActionPlatform} from '@core/home-selection.ts';
import {entryActions} from '@core/presentation/entry-actions.ts';
import type {HomeDocument, HomeRow, HomeSpotlight} from '@core/home.ts';
import type {ContentEntry} from '@core/library-content.ts';
import type {SelectionContext, SelectionEnvironment, SelectionIdentity} from '@core/selection-model/index.ts';
import {SelectionCircle} from '../kit/selection';
import type {CardSelection} from '../kit/cards';
import {router} from '../router';
import type {HomeSelection} from '../screens/home/Home';
import type {ShellAssemblyContext} from './bootstrap';
import {entryDestination, seeAllDestination} from './navigation';
import {SelectionSurfaces, SelectionPhoneBar, type SelectionRendererSource} from './SelectionSurfaces';

export function createShellSelection(context: ShellAssemblyContext, playback: HomeSelectionPlaybackPort, reportError: (error: unknown) => void) {
  const listeners = new Set<() => void>();
  const platform: HomeActionPlatform = {platform: 'web', playbackAvailable: true, queueAvailable: true};
  const viewer = context.session().viewer;
  const authority = viewer.authority;
  if (authority !== 'local' && authority !== 'hosted') throw new Error('unsupported-home-authority');
  const scope = () => ({serverId: viewer.serverId, authority, accountId: viewer.accountId, profileId: viewer.profileId, viewId: router.state.location.href});
  const document = (): HomeDocument => { const value = context.home.getSnapshot().document; if (!value) throw new Error('home-document-required'); return value; };
  const currentContext = () => homeSelectionContext(scope(), document());
  const environment = (): SelectionEnvironment => ({platform: window.innerWidth >= 1024 ? 'desktop-web' : 'mobile-web', isOwner: viewer.role === 'owner',
    canEditMetadata: document().rows.some(row => row.entries.some(entry => homeEntryCapabilities(entry, platform).editMetadata === true))});
  let retired = false;
  const isCurrent = (value: SelectionContext) => {
    const current = context.home.getSnapshot().document;
    return !retired && context.isCurrent() && !!current && router.state.location.pathname === '/' && JSON.stringify(value) === JSON.stringify(homeSelectionContext(scope(), current));
  };
  const controller = new HomeSelectionController({api: context.api, events: context.events, home: context.home,
    context: currentContext(), environment: environment(), isCurrent, requestId: async () => crypto.randomUUID(), playback,
    routes: {openEntry: entry => router.navigate(entryDestination(entry)), openCollection: collection => router.navigate(seeAllDestination({type: 'collection', resourceId: collection.collection.id, libraryId: collection.collection.libraryId})),
      openDestination: destination => router.navigate({to: '/saved', search: {resourceId: destination.id}})}, reportError});
  const anchor: {current: HTMLElement | null} = {current: null};
  const source: SelectionRendererSource = {controller, anchor, artwork: context.artwork, reportError, owner: viewer.role === 'owner'};
  let acceptedContext = JSON.stringify(currentContext()), acceptedEnvironment = JSON.stringify(environment());
  const sync = () => {
    if (retired || !context.isCurrent()) return;
    const nextDocument = context.home.getSnapshot().document;
    if (!nextDocument) { controller.clear(); return; }
    const next = homeSelectionContext(scope(), nextDocument), nextEnvironment = environment();
    if (JSON.stringify(next) !== acceptedContext || JSON.stringify(nextEnvironment) !== acceptedEnvironment) {
      acceptedContext = JSON.stringify(next); acceptedEnvironment = JSON.stringify(nextEnvironment); controller.setContext(next, nextEnvironment);
    }
  };
  const off = [controller.subscribe(() => { for (const listener of listeners) listener(); }), context.home.subscribe(sync), router.subscribe('onResolved', sync)];
  window.addEventListener('resize', sync);
  const perform = (work: () => void) => { try { work(); } catch (error) { reportError(error); } };
  const windowId = (row: HomeRow) => 'home:' + row.id;
  const spotlightId = (spotlight: HomeSpotlight) => 'home:spotlight:' + document().spotlight.indexOf(spotlight);
  const selected = (key: string) => controller.getSnapshot().selection.selectedKeys.includes(key);
  function descriptor(row: HomeRow, item: SelectionIdentity | undefined, title: string): CardSelection | undefined {
    if (!item) return undefined;
    const key = controller.key(item), snapshot = controller.getSnapshot().selection;
    const toggle = (range: boolean) => perform(() => {
      const change = controller.toggle(windowId(row), key, range);
      if (!change.ok) throw Object.assign(new Error(change.reason), {code: change.reason});
    });
    return {active: snapshot.active, checked: selected(key), onToggle: intent => toggle(intent.range), onExit: () => controller.clear(),
      control: <SelectionCircle selected={selected(key)} selecting={snapshot.active} accessibilityLabel={defaultI18n.t('next.home.select', {title})}
        onSelectedChange={(_next, intent) => toggle(intent.intent === 'range' || intent.shiftKey === true)} />};
  }
  const selection: HomeSelection = {
    toolbar: <SelectionSurfaces source={source} />,
    card: (row, entry) => descriptor(row, homeEntrySelection(entry, platform), entry.title),
    channel: (row, item) => descriptor(row, homeChannelSelection(item, platform), item.channel.name),
    collection: row => row.collection ? descriptor(row, homeCollectionSelection(row.collection, platform), row.collection.collection.name) : undefined,
    registerWindow: (row, sectionOrder, first, last) => {
      if (!isCurrent(controller.getSnapshot().context) || first < 0 || last < first) return undefined;
      // The server's returned row is capped at 100 and is already mounted by Shelf.
      // Keeping this loaded page allows Shift ranges after horizontal scrolling, without reading a query.
      const registered = controller.registerWindow({id: windowId(row), row, sectionOrder, start: row.start});
      if (!registered.ok) { reportError(Object.assign(new Error(registered.reason), {code: registered.reason})); return undefined; }
      return registered.unregister;
    },
    registerSpotlight: spotlight => {
      if (!isCurrent(controller.getSnapshot().context)) return undefined;
      const registered = controller.registerSpotlight({id: spotlightId(spotlight), spotlight});
      if (!registered.ok) { reportError(Object.assign(new Error(registered.reason), {code: registered.reason})); return undefined; }
      return registered.unregister;
    },
    moreAvailable: entry => entryActions(homeEntryCapabilities(entry, platform)).some(group => group.actions.length > 0),
    moreExpanded: (row, entry) => { const item = homeEntrySelection(entry, platform); return !!item && controller.getSnapshot().menu?.key === controller.key(item) && controller.getSnapshot().menu?.windowId === windowId(row); },
  };
  return {selection, controller,
    setSpotlightMyList: async (spotlight: HomeSpotlight) => {
      const item = homeEntrySelection(spotlight.entry, platform); if (!item) throw new Error('spotlight-identity-required');
      controller.openMenu(spotlightId(spotlight), controller.key(item)); await controller.executeMenu('watchlist');
    },
    openMore: (row: HomeRow, entry: ContentEntry, element: HTMLElement) => perform(() => {
      const item = homeEntrySelection(entry, platform); if (!item) throw new Error('home-library-identity-required');
      anchor.current = element; controller.openMenu(windowId(row), controller.key(item));
    }),
    phoneSelection: (): ReactNode => controller.getSnapshot().selection.active ? <SelectionPhoneBar source={source} /> : undefined,
    subscribe: (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; },
    dispose: () => { retired = true; controller.dispose(); for (const cleanup of off) cleanup(); window.removeEventListener('resize', sync); anchor.current = null; listeners.clear(); },
  };
}
