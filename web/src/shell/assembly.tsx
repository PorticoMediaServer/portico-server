import {appearancePreferences} from '@core/preference-effects.ts';
import {homeSelectionContext} from '@core/home-selection/index.ts';
import type {SelectionContext} from '@core/selection-model/index.ts';
import {setServiceI18n} from '@core/presentation/service-text.ts';
import {PersonalSavedService} from '@core/personal-saved.ts';
import {randomId} from '@core/random-id.ts';
import {WebShellPlaybackRuntime} from '../platform/playback';
import {router} from '../router';
import {navigateArtworkEntry, retireArtworkScope} from './artwork-navigation';
import type {ShellBindings} from './bindings';
import type {ShellAssembler} from './bootstrap';
import {createOwnerActivity} from './Activity';
import {activityIssueSettings} from './activity-model';
import {ShellFeedback, ShellFeedbackHost} from './Feedback';
import {entryDestination, seeAllDestination} from './navigation';
import {hasDock} from './playback-model';
import {createShellSelection} from './selection';
import {shellLanguage, shellPlaybackSettings, shellPlaybackSkips} from './settings';
import {homePlaybackItem} from '../screens/home/model';
import type {HomeSelection} from '../screens/home/Home';
import type {ShellPlaybackBinding} from './Playback';
import {HomeDocumentResources} from './home-resources';
import {openAppLogContext} from '../platform/app-log';
import {createWebPreparingStarts} from '../platform/playback/start-owner';

/** Concrete protected-session assembly. Every command uses the actual current service/engine. */
export const assembleShellBindings: ShellAssembler = async context => {
  const captured = context.session(), viewer = captured.viewer, authority = viewer.authority;
  if (authority !== 'local' && authority !== 'hosted') throw new Error('unsupported-shell-authority');
  let active = true;
  const current = () => active && context.isCurrent();
  const artworkScope = JSON.stringify([viewer.serverId, authority, viewer.accountId, viewer.profileId, captured.sessionFamilyId, context.epoch]);
  const scope = Object.freeze({serverId: viewer.serverId, viewerId: JSON.stringify([authority, viewer.accountId, viewer.profileId])});
  const currentPreferences = () => context.preferences?.getSnapshot('web') ?? context.facts.preferences;
  const language = shellLanguage(currentPreferences());
  setServiceI18n(language);
  context.api.setAppLanguage(language.locale);
  const time = {t: language.t, locale: language.region.formatLocale, timeZone: language.region.timeZone,
    hourCycle: language.region.hour12 === true ? 'h12' as const : language.region.hour12 === false ? 'h23' as const : undefined};
  const feedback = new ShellFeedback(language, current), life = new AbortController();
  const resources: (() => void)[] = [() => retireArtworkScope(artworkScope), feedback.dispose, () => life.abort(), openAppLogContext(current)];
  const preparingStarts = createWebPreparingStarts(context.api, captured, context.session, current);
  if (preparingStarts) resources.push(preparingStarts.dispose);
  const dispose = () => { if (!active) return; active = false; for (const cleanup of resources.splice(0).reverse()) { try { cleanup(); } catch { /* Continue fencing every other private owner. */ } } };
  try {
  const listeners = new Set<() => void>();
  const changed = () => { if (current()) for (const listener of listeners) listener(); };
  resources.push(() => listeners.clear());
  const currentHomeContext = () => {
    const document = context.home.getSnapshot().document; if (!document) throw new Error('home-document-required');
    return homeSelectionContext({serverId: viewer.serverId, authority, accountId: viewer.accountId, profileId: viewer.profileId, viewId: router.state.location.href}, document);
  };
  const homeCurrent = (value: SelectionContext) => current() && !!context.home.getSnapshot().document && router.state.location.pathname === '/' && JSON.stringify(value) === JSON.stringify(currentHomeContext());
  type Player = {runtime: WebShellPlaybackRuntime; playback: ReturnType<WebShellPlaybackRuntime['selectionPlayback']>; selection: ReturnType<typeof createShellSelection>; binding: ShellPlaybackBinding};
  let playerResources: HomeDocumentResources<Player> | undefined;
  const dockVisible = () => {const player = playerResources?.get();return !!player && hasDock(player.runtime.getSnapshot());};
  playerResources = new HomeDocumentResources(context.home, {isCurrent: current, changed, failed: feedback.reportError, create: document => {
    const owned: (() => void)[] = [], close = () => {for (const cleanup of owned.splice(0).reverse()) {try {cleanup();} catch { /* Retire every owned resource. */ }}};
    try {
      const runtime = new WebShellPlaybackRuntime({api: context.api, preparingStarts, authority: {...viewer, authority, epoch: context.epoch, viewerFence: document.viewerFence},
        localScope: {serverId: viewer.serverId, authority, accountId: viewer.accountId, profileId: viewer.profileId, sessionFamilyId: captured.sessionFamilyId},
        contentScope: scope, events: context.events,
        isCurrent: () => current() && context.home.getSnapshot().document?.viewerFence === document.viewerFence, settings: shellPlaybackSettings(currentPreferences)});
      owned.push(() => runtime.dispose());
      const playback = runtime.selectionPlayback(homeCurrent), selection = createShellSelection(context, playback, feedback.reportError, dockVisible);
      owned.push(selection.dispose, runtime.subscribe(changed), selection.subscribe(changed));
      const binding: ShellPlaybackBinding = {runtime, artwork: context.artwork, environment: time, reportError: feedback.reportError,
        get skips() {return shellPlaybackSkips(currentPreferences(), runtime.getSnapshot().dock.local?.kind === 'audiobook');}};
      return {value: {runtime, playback, selection, binding}, dispose: close};
    } catch (error) {close();throw error;}
  }});
  resources.push(playerResources.dispose);
  const player = () => {const value = playerResources?.get();if (!value) throw new Error('home-document-required');return value;};
  const homeSelection: HomeSelection = {
    get toolbar() {return playerResources?.get()?.selection.selection.toolbar;},
    card: (row, entry) => playerResources?.get()?.selection.selection.card(row, entry),
    channel: (row, channel) => playerResources?.get()?.selection.selection.channel(row, channel),
    collection: row => playerResources?.get()?.selection.selection.collection(row),
    registerWindow: (row, order, first, last) => playerResources?.get()?.selection.selection.registerWindow(row, order, first, last),
    registerSpotlight: spotlight => playerResources?.get()?.selection.selection.registerSpotlight(spotlight),
    moreAvailable: entry => playerResources?.get()?.selection.selection.moreAvailable(entry) ?? false,
    moreExpanded: (row, entry) => playerResources?.get()?.selection.selection.moreExpanded(row, entry) ?? false,
  };
  const activity = createOwnerActivity(context, time,
    section => { void router.navigate({to: '/dashboard', search: section ? {section} : {}}).catch(feedback.reportError); },
    () => { void router.navigate({to: '/dashboard', search: {section: 'updates'}}).catch(feedback.reportError); },
    destination => {
      const section = activityIssueSettings(destination.kind);
      if (section) void router.navigate({to: '/settings', search: {scope: 'server', section}}).catch(feedback.reportError);
    });
  resources.push(activity.dispose);
  let homeLayout = JSON.stringify([currentPreferences().effective['home.rowOrder'], currentPreferences().effective['home.hiddenRowIds']]);
  if (context.preferences) resources.push(context.preferences.subscribe(() => {
    if (!current()) return;
    playerResources?.get()?.runtime.refreshSettings();
    const nextLayout = JSON.stringify([currentPreferences().effective['home.rowOrder'], currentPreferences().effective['home.hiddenRowIds']]);
    if (nextLayout !== homeLayout) {homeLayout = nextLayout;void context.home.revalidate().catch(feedback.reportError);}
    changed();
  }));
  const getBinding = (): ShellBindings => {
    const visibleDock = dockVisible();
    return {api: context.api, events: context.events, scopeKey: JSON.stringify([viewer.serverId, authority, viewer.accountId, viewer.profileId, captured.sessionFamilyId, context.epoch]),
      playback: playerResources?.get()?.binding,
      shell: {identity: context.facts.identity, OwnerActivity: activity.OwnerActivity, dockVisible: visibleDock, onSignOut: context.signOut,onSignOutPorticoAccount:context.signOutPorticoAccount,onSwitchProfile:context.switchProfile,onSwitchServer:context.switchServer,savedServers:context.savedServers,
        phoneSelection: playerResources?.get()?.selection.phoneSelection(), overlays: <ShellFeedbackHost source={feedback} dockVisible={visibleDock} />},
      home: {service: context.home, artwork: context.artwork, time, serverName: context.facts.identity.serverName, reducedMotion: appearancePreferences(currentPreferences(), globalThis.matchMedia('(prefers-reduced-motion: reduce)').matches).reducedMotion, selection: homeSelection,
        customization: {isCurrent: current, requestId: randomId, createSavedViews: () => {
          if (!current()) throw new Error('shell_authority_retired');
          return new PersonalSavedService(context.api, scope, async () => randomId());
        }},
        effects: {play: async (entry, target) => {
          const document = context.home.getSnapshot().document;
          const item = document && homePlaybackItem(document, entry, target);
          if (!item) throw new Error('stale-home-playback-target');
          await player().playback.play({context: currentHomeContext(), items: [item], signal: life.signal});
        }, openEntry: entry => {const destination = entryDestination(entry);void navigateArtworkEntry(entry, destination, artworkScope, current, current, manual => router.navigate({...destination, ...(manual ? {viewTransition: false} : {})})).catch(feedback.reportError);}, openSeeAll: target => { void router.navigate(seeAllDestination(target)).catch(feedback.reportError); },
          openMore: (row, entry, element) => player().selection.openMore(row, entry, element), reportError: feedback.reportError,
          openHome: () => {void router.navigate({to: '/'}).catch(feedback.reportError);},
          playChannel: async channel => {
            const document = context.home.getSnapshot().document;
            const item = document?.rows.flatMap(row => row.live?.channels ?? []).find(value => value.channel === channel);
            if (!item) throw new Error('stale-home-channel');
            await player().playback.playChannel(item, currentHomeContext(), life.signal);
          },
          setMyList: async spotlight => { await player().selection.setSpotlightMyList(spotlight); if (current()) await context.home.revalidate(); },
        }},
    };
  };
  return {getBinding, subscribe: listener => { listeners.add(listener); return () => { listeners.delete(listener); }; },
    busy: () => current() && playerResources?.get()?.runtime.getSnapshot().playback.intent === 'playing', onBusy: listener => {listeners.add(listener);return () => {listeners.delete(listener);};},
    dispose,
  };
  } catch (error) { dispose(); throw error; }
};
