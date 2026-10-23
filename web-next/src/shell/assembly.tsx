import {preferenceValue} from '@core/preferences.ts';
import {appearancePreferences} from '@core/preference-effects.ts';
import {homeSelectionContext} from '@core/home-selection/index.ts';
import type {SelectionContext} from '@core/selection-model/index.ts';
import {setServiceI18n} from '@core/presentation/service-text.ts';
import {WebShellPlaybackRuntime} from '../platform/playback';
import {router} from '../router';
import type {ShellBindings} from './bindings';
import type {ShellAssembler} from './bootstrap';
import {createOwnerActivity} from './Activity';
import {ShellFeedback, ShellFeedbackHost} from './Feedback';
import {entryDestination, seeAllDestination} from './navigation';
import {hasDock} from './playback-model';
import {createShellSelection} from './selection';
import {shellLanguage, shellPlaybackSettings} from './settings';
import {homePlaybackItem} from '../screens/home/model';

/** Concrete protected-session assembly. Every command uses the actual current service/engine. */
export const assembleShellBindings: ShellAssembler = async context => {
  const viewer = context.session().viewer, authority = viewer.authority;
  if (authority !== 'local' && authority !== 'hosted') throw new Error('unsupported-shell-authority');
  let active = true;
  const current = () => active && context.isCurrent();
  const currentPreferences = () => context.preferences?.getSnapshot('web') ?? context.facts.preferences;
  const language = shellLanguage(currentPreferences());
  setServiceI18n(language);
  context.api.setAppLanguage(language.locale);
  const time = {t: language.t, locale: language.region.formatLocale, timeZone: language.region.timeZone,
    hourCycle: language.region.hour12 === true ? 'h12' as const : language.region.hour12 === false ? 'h23' as const : undefined};
  const feedback = new ShellFeedback(language, current), life = new AbortController();
  const resources: (() => void)[] = [feedback.dispose, () => life.abort()];
  const dispose = () => { if (!active) return; active = false; for (const cleanup of resources.splice(0).reverse()) { try { cleanup(); } catch { /* Continue fencing every other private owner. */ } } };
  try {
  const runtime = new WebShellPlaybackRuntime({api: context.api, authority: {...viewer, authority, epoch: context.epoch, viewerFence: context.viewerFence},
    contentScope: {serverId: viewer.serverId, viewerId: JSON.stringify([authority, viewer.accountId, viewer.profileId])}, events: context.events, isCurrent: current, settings: shellPlaybackSettings(currentPreferences)});
  resources.push(() => runtime.dispose());
  const currentHomeContext = () => {
    const document = context.home.getSnapshot().document; if (!document) throw new Error('home-document-required');
    return homeSelectionContext({serverId: viewer.serverId, authority, accountId: viewer.accountId, profileId: viewer.profileId, viewId: router.state.location.href}, document);
  };
  const homeCurrent = (value: SelectionContext) => current() && !!context.home.getSnapshot().document && router.state.location.pathname === '/' && JSON.stringify(value) === JSON.stringify(currentHomeContext());
  const playback = runtime.selectionPlayback(homeCurrent);
  const selection = createShellSelection(context, playback, feedback.reportError, () => hasDock(runtime.getSnapshot()));
  resources.push(selection.dispose);
  const activity = createOwnerActivity(context, time,
    section => { void router.navigate({to: '/dashboard', search: section ? {section} : {}}).catch(feedback.reportError); },
    () => { void router.navigate({to: '/dashboard', search: {section: 'updates'}}).catch(feedback.reportError); });
  resources.push(activity.dispose);
  const listeners = new Set<() => void>();
  const changed = () => { if (current()) for (const listener of listeners) listener(); };
  resources.push(runtime.subscribe(changed), selection.subscribe(changed), () => listeners.clear());
  let homeLayout = JSON.stringify([currentPreferences().effective['home.rowOrder'], currentPreferences().effective['home.hiddenRowIds']]);
  if (context.preferences) resources.push(context.preferences.subscribe(() => {
    if (!current()) return;
    runtime.refreshSettings();
    const nextLayout = JSON.stringify([currentPreferences().effective['home.rowOrder'], currentPreferences().effective['home.hiddenRowIds']]);
    if (nextLayout !== homeLayout) { homeLayout = nextLayout; void context.home.revalidate().catch(feedback.reportError); }
    changed();
  }));
  const playbackBinding = {runtime, artwork: context.artwork, environment: time, reportError: feedback.reportError,
    get skips() { return {back: preferenceValue<number>(currentPreferences(), 'playback.skipBackMs') / 1000, forward: preferenceValue<number>(currentPreferences(), 'playback.skipForwardMs') / 1000}; }};
  const getBinding = (): ShellBindings => {
    const dockVisible = hasDock(runtime.getSnapshot());
    return {api: context.api, events: context.events, scopeKey: JSON.stringify([viewer.serverId, authority, viewer.accountId, viewer.profileId, context.epoch, context.viewerFence]),
      playback: playbackBinding,
      shell: {identity: context.facts.identity, OwnerActivity: activity.OwnerActivity, dockVisible, onSignOut: context.signOut,onSignOutPorticoAccount:context.signOutPorticoAccount,onSwitchProfile:context.switchProfile,onSwitchServer:context.switchServer,savedServers:context.savedServers,
        phoneSelection: selection.phoneSelection(), overlays: <ShellFeedbackHost source={feedback} dockVisible={dockVisible} />},
      home: {service: context.home, artwork: context.artwork, time, reducedMotion: appearancePreferences(currentPreferences(), globalThis.matchMedia('(prefers-reduced-motion: reduce)').matches).reducedMotion, selection: selection.selection,
        effects: {play: async (entry, target) => {
          const document = context.home.getSnapshot().document;
          const item = document && homePlaybackItem(document, entry, target);
          if (!item) throw new Error('stale-home-playback-target');
          await playback.play({context: currentHomeContext(), items: [item], signal: life.signal});
        }, openEntry: entry => { void router.navigate(entryDestination(entry)).catch(feedback.reportError); }, openSeeAll: target => { void router.navigate(seeAllDestination(target)).catch(feedback.reportError); },
          openMore: selection.openMore, reportError: feedback.reportError,
          playChannel: async channel => {
            const document = context.home.getSnapshot().document;
            const item = document?.rows.flatMap(row => row.live?.channels ?? []).find(value => value.channel === channel);
            if (!item) throw new Error('stale-home-channel');
            await playback.playChannel(item, currentHomeContext(), life.signal);
          },
          setMyList: async spotlight => { await selection.setSpotlightMyList(spotlight); if (current()) await context.home.revalidate(); },
        }},
    };
  };
  return {getBinding, subscribe: listener => { listeners.add(listener); return () => { listeners.delete(listener); }; },
    busy: () => current() && runtime.getSnapshot().playback.intent === 'playing', onBusy: runtime.subscribe,
    dispose,
  };
  } catch (error) { dispose(); throw error; }
};
