import {useEffect, useState} from 'react';
import {useRouterState} from '@tanstack/react-router';
import {settingsBinding} from '@core/presentation/settings-bindings.ts';
import {captureReportContext, type ReportContext} from '@core/report-context.ts';
import {encodedRoutePath} from '../../../../packages/contracts/src/routes.ts';
import {defineWebScreen} from '../../route-registry';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import type {ShellAssemblyContext} from '../../shell/bootstrap';
import {shellLanguage} from '../../shell/settings';
import {settingsRoute} from '../../kit/settings/search';
import {SettingsPageFailure} from '../../kit/settings';
import {settingsPlatform} from '../../platform/settings-platform';
import {DashboardScreen, HistoryScreen, createDashboardBindings, type DashboardBindings} from '../dashboard';
import {SettingsScreen, type SettingsBindings} from './Settings';
import {SettingsController, type SettingsControllerOptions} from './controller';
import {createSettingsBindings} from './bindings';
import {readingSettingsAdapter} from './reading-adapter';

// A carry contains calendar bounds only. The captured session object owns its lifetime.
const historyContexts = new WeakMap<ShellAssemblyContext, ReportContext>();
const playbackEffects = new Set(['playback.defaultSpeed', 'playback.introSkip', 'playback.creditsSkip', 'playback.recapSkip', 'playback.passoutProtection', 'playback.passoutAfterEpisodes', 'playback.skipBackMs', 'playback.skipForwardMs', 'audiobooks.defaultSpeed', 'accessibility.reducedMotion']);

export function SettingsEntry() {
  const protectedContext = useProtectedWebScreenContext();
  const location = useRouterState({select: state => state.location});
  const [owned, setOwned] = useState<{context: ShellAssemblyContext; settings: SettingsBindings; dashboard?: DashboardBindings}>();
  const [failure, setFailure] = useState(false), [retry, setRetry] = useState(0);
  const context = protectedContext?.context;
  const raw = Object.fromEntries(new URLSearchParams(location.searchStr));
  const route = settingsRoute(location.pathname === '/dashboard' ? {scope: 'server', section: 'dashboard'} : raw, () => true);
  useEffect(() => {
    const reader = context?.preferences, snapshot = reader?.getSnapshot('web');
    if (!context || !reader || !snapshot || !context.isCurrent()) return;
    let active = true;
    const current = () => active && context.isCurrent();
    const reportError = () => {if (current()) setFailure(true);};
    const identity = context.facts.identity, viewer = context.session().viewer;
    const role = viewer.role === 'owner' || viewer.role === 'admin' ? viewer.role : 'member';
    const openHome = () => {if (current()) void router.navigate({to: '/'});};
    const openSettings = (section: string) => {if (current()) void router.navigate({to: '/settings', search: () => ({scope: 'server', section})});};
    const platform = settingsPlatform();
    const options: SettingsControllerOptions = {api: context.api, serverId: viewer.serverId, viewerId: JSON.stringify([viewer.authority, viewer.accountId, viewer.profileId]),
      role, mainProfile: identity.primaryProfile === true,
      capabilities: {manageServer: identity.capabilities?.manageServer === true, managePeople: identity.capabilities?.managePeople === true, manageSignIn: identity.capabilities?.manageSignIn === true},
      preferences: snapshot, reader, isCurrent: current, reportError, reading: readingSettingsAdapter,
      environment: {...(platform?.environment ?? {platform: 'web' as const, os: 'browser' as const}), role, signIn: identity.signIn}, device: platform?.device,
      eligibility: {library: () => undefined, hasEngine: (row: Parameters<SettingsController['eligible']>[0]) => {
        const binding = settingsBinding(row.id);
        if (!binding) return row.id === 'device.name' && !!platform?.device?.getSnapshot().facts.name || row.id === 'device.appVersion' && !!platform?.device?.getSnapshot().facts.appVersion;
        if (binding.form === 'preferences') return playbackEffects.has(binding.key) || binding.key.startsWith('subtitles.');
        return binding.form !== 'library' && binding.form !== 'remote' && binding.form !== 'managedRoute';
      }},
    };
    const controller = new SettingsController(options);
    let dashboard: DashboardBindings | undefined;
    if (identity.primaryProfile === true && identity.capabilities?.manageServer && (role === 'owner' || role === 'admin')) {
      const carried = route.section === 'history' ? historyContexts.get(context) : undefined;
      if (carried) historyContexts.delete(context);
      dashboard = createDashboardBindings({api: context.api, events: context.events, scope: {serverId: viewer.serverId, viewerId: options.viewerId},
        context: carried ?? captureReportContext(shellLanguage(snapshot).region.timeZone ?? Intl.DateTimeFormat().resolvedOptions().timeZone), role, isCurrent: current, isMainProfile: () => current() && context.facts.identity.primaryProfile === true,
        artwork: (id, kind, revision) => revision ? context.artwork.foreground(encodedRoutePath('get_items_id_art_kind', {id: encodeURIComponent(id), kind}) + '?' + new URLSearchParams({v: revision}), kind === 'backdrop' ? 800 : 400, kind === 'poster' ? 2 / 3 : kind === 'backdrop' ? 16 / 9 : 1) : undefined,
        effects: {openHome, openSettings, reportError, openHistory: report => {if (current()) {historyContexts.set(context, report);openSettings('history');}}}});
    }
    const settings = createSettingsBindings(controller, options, identity, {openHome,
      special: () => null,
      section: selected => selected.section.id === 'dashboard' && dashboard ? <DashboardScreen bindings={dashboard} embedded /> : selected.section.id === 'history' && dashboard ? <HistoryScreen bindings={dashboard} embedded /> : undefined,
    });
    setFailure(false);setOwned({context, settings, dashboard});
    return () => {active = false;dashboard?.dispose();controller.dispose();};
  }, [context, retry, route.section]);
  if (!protectedContext || !owned || owned.context !== context) return null;
  const navigate = (next: Parameters<typeof SettingsScreen>[0]['route']) => {if (context?.isCurrent()) void router.navigate({to: '/settings', search: () => next});};
  const settings = failure ? {...owned.settings, section: () => <SettingsPageFailure retry={() => setRetry(value => value + 1)} home={owned.settings.openHome} />} : owned.settings;
  return <SettingsScreen bindings={settings} route={route} navigate={navigate} />;
}

export default defineWebScreen({id: 'settings', component: SettingsEntry});
