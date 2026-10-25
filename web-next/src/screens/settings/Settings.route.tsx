import {useEffect, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {Button} from '../../kit/controls';
import {useRouterState} from '@tanstack/react-router';
import {peopleStreamLimits, settingsBinding} from '@core/presentation/settings-bindings.ts';
import {settingsRowIndex} from '@core/presentation/settings-spec.ts';
import {captureReportContext, type ReportContext} from '@core/report-context.ts';
import {SettingsPeopleClient} from '@core/settings-people.ts';
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
import {CustomizeHomeRow} from './account-appearance/CustomizeHome';
import {SettingsController, type SettingsControllerOptions} from './controller';
import {createSettingsBindings} from './bindings';
import {LibraryManagementService} from '@core/library-management.ts';
import {LibrariesPanel} from './libraries/LibrariesPanel';
import {LibraryScanAction} from './libraries/LibraryActions';
import {isLibraryPanelContext, libraryScanAction} from './libraries/model';
import {createServerSettingsSurfaces} from './server';
import {createTroubleshootingSurfaces} from './server/troubleshooting';
import {presetEnginesAvailable} from './server/engines';
import {LibraryFolders} from './libraries/LibraryFolders';
import {readingSettingsAdapter} from './reading-adapter';
import {hasWebPreferenceEffect} from './effects';
import {createPrivacySettingsSurfaces} from './privacy-controls';
import {PeopleScreen, type PeopleBindings} from './People';
import {SignInSettingsSource} from './signin-source';
import {SettingsOwnerSwitch} from './owner-switch';
import {DeviceRowControl} from './device-rows';
import {deviceRowState, isDeviceSyntheticRow} from './device-rows-model';
import {AudioSubtitleLanguageList, SubtitlePreview, isSubtitlePreview, preferredLanguageKind} from './audio-subtitles-rows';
import {subtitlePreviewPaint} from './audio-subtitles-model';
import {MusicEqualizerRow, musicEqualizerAvailable} from './account-music/MusicEqualizerRow';
import {RemoteSourcesPage} from './remote-sources/RemoteSourcesPage';
import {browserAccountOrigin} from '../../platform/account-origin';
import {SettingsInvitations} from './invitations';
import {createOwnerSources, OwnerSourcesPage} from '../live/owner/OwnerSourcesPage';

// A carry contains calendar bounds only. The captured session object owns its lifetime.
const historyContexts = new WeakMap<ShellAssemblyContext, ReportContext>();
const noPreferences = () => () => {};
/** Stable snapshot: the Remote sources page owns its reads and never enters the generic section loader. */
const remoteSourcesPageSnapshot = Object.freeze({loading: false});

export function SettingsEntry() {
  const protectedContext = useProtectedWebScreenContext();
  const location = useRouterState({select: state => state.location});
  const [owned, setOwned] = useState<{context: ShellAssemblyContext; settings: SettingsBindings; dashboard?: DashboardBindings}>();
  const [failure, setFailure] = useState(false), [retry, setRetry] = useState(0);
  const context = protectedContext?.context;
  // The same confirmed reader drives calendar lifetime. A zone change retires
  // the old report factory/results; no old rows can acquire the new zone label.
  const preferences = useSyncExternalStore(context?.preferences?.subscribe ?? noPreferences, () => context?.preferences?.getSnapshot('web'), () => undefined);
  const reportTimeZone = preferences && shellLanguage(preferences).region.timeZone || Intl.DateTimeFormat().resolvedOptions().timeZone;
  const raw = Object.fromEntries(new URLSearchParams(location.searchStr));
  const route = settingsRoute(location.pathname === '/dashboard' ? {scope: 'server', section: 'dashboard'} : raw, () => true);
  useEffect(() => {
    const reader = context?.preferences, snapshot = reader?.getSnapshot('web');
    if (!context || !reader || !snapshot || !context.isCurrent()) return;
    let active = true;
    const authorityListeners = new Set<() => void>();
    const current = () => active && context.isCurrent();
    const reportError = () => {if (current()) setFailure(true);};
    const language = snapshot ? shellLanguage(snapshot) : defaultI18n;
    const uiLocale = language.locale, appLocale = language.region.formatLocale;
    const identity = context.facts.identity, viewer = context.session().viewer;
    const role = viewer.role === 'owner' || viewer.role === 'admin' ? viewer.role : 'member';
    const openHome = () => {if (current()) void router.navigate({to: '/'});};
    const openSettings = (section: string) => {if (current()) void router.navigate({to: '/settings', search: () => ({scope: 'server', section})});};
    // A drawn link names the page that owns its value, so it opens that row too.
    const openSettingsRow = (target: {scope: 'account' | 'server'; section: string; row?: string}) => {if (current()) void router.navigate({to: '/settings', search: () => target});};
    const platform = settingsPlatform();
    const privacy = createPrivacySettingsSurfaces({api: context.api, serverId: viewer.serverId, viewerId: JSON.stringify([viewer.authority, viewer.accountId, viewer.profileId]), isCurrent: current, reportError});
    // One row's engine. A preset row is answered by the rule below instead, so its
    // own missing binding never decides for the rows it sets.
    const engine = (row: Parameters<SettingsController['eligible']>[0]) => {
      if (privacy.eligible(row)) return true;
      if (row.id === 'music.eq') return musicEqualizerAvailable(platform);
      const binding = settingsBinding(row.id);
      if (!binding) return row.id === 'device.name' && !!platform?.device?.getSnapshot().facts.name || row.id === 'device.appVersion' && !!platform?.device?.getSnapshot().facts.appVersion;
      if (binding.form === 'preferences') return hasWebPreferenceEffect(row);
      return binding.form !== 'library' && binding.form !== 'remote' && binding.form !== 'managedRoute';
    };
    const options: SettingsControllerOptions = {api: context.api, serverId: viewer.serverId, viewerId: JSON.stringify([viewer.authority, viewer.accountId, viewer.profileId]),
      role, mainProfile: identity.primaryProfile === true,
      capabilities: {manageServer: identity.capabilities?.manageServer === true, managePeople: identity.capabilities?.managePeople === true, manageSignIn: identity.capabilities?.manageSignIn === true},
      preferences: snapshot, reader, isCurrent: current, reportError, reading: readingSettingsAdapter,
      environment: {...(platform?.environment ?? {platform: 'web' as const, os: 'browser' as const}), role, signIn: identity.signIn}, device: platform?.device,
      eligibility: {library: () => undefined, hasEngine: row => privacy.eligible(row) ? true : presetEnginesAvailable(row, engine) ?? engine(row)},
    };
    const controller = new SettingsController(options);
    const signIn = identity.primaryProfile === true && role === 'owner' && identity.capabilities?.manageSignIn === true &&
      (identity.signIn === 'local' || identity.signIn === 'portico') ? new SignInSettingsSource(context.api, {
        serverId: viewer.serverId, signIn: identity.signIn, isCurrent: current,
        capabilities: () => ({manageAccount: identity.capabilities?.manageAccount === true, managePeople: identity.capabilities?.managePeople === true,
          manageServer: identity.capabilities?.manageServer === true, manageSignIn: current() && identity.capabilities?.manageSignIn === true}),
        onAuthorityChanged: () => {if (current()) void router.navigate({to: '/sign-in', replace: true});},
        ownerSwitch: identity.signIn === 'local' ? new SettingsOwnerSwitch(context.api, {serverId: viewer.serverId,
          isCurrent: () => current() && context.session().viewer.role === 'owner' && context.facts.identity.primaryProfile === true && context.facts.identity.capabilities?.manageSignIn === true && context.facts.identity.signIn === 'local',
          hostedOrigin: browserAccountOrigin(import.meta.env, globalThis.location),
        }) : undefined,
      }) : undefined;
    const session = context.session();
    const people: PeopleBindings | undefined = route.section === 'people' && identity.primaryProfile === true && identity.capabilities?.managePeople === true &&
      (role === 'owner' || role === 'admin') && (viewer.authority === 'local' || viewer.authority === 'hosted') ? {
        client: new SettingsPeopleClient(context.api, current), serverName: context.facts.serverName, libraries: identity.libraries,
        actor: {accountId: viewer.accountId, role}, streamLimits: peopleStreamLimits,
        invitations: new SettingsInvitations(context.api, current, context.api.baseUrl, viewer.serverId), openHome, reportError,
        authority: {scope: {serverId: viewer.serverId, authority: viewer.authority, accountId: viewer.accountId, profileId: viewer.profileId, sessionFamilyId: session.sessionFamilyId},
          isCurrent: current, subscribe: listener => {authorityListeners.add(listener); return () => {authorityListeners.delete(listener);};}, onAuthorityChanged: openHome},
      } : undefined;
    let dashboard: DashboardBindings | undefined;
    if (identity.primaryProfile === true && identity.capabilities?.manageServer && (role === 'owner' || role === 'admin')) {
      const pending = route.section === 'history' ? historyContexts.get(context) : undefined;
      if (pending) historyContexts.delete(context);
      const carried = pending?.timeZone === reportTimeZone ? pending : undefined;
      dashboard = createDashboardBindings({api: context.api, events: context.events, scope: {serverId: viewer.serverId, viewerId: options.viewerId},
        context: carried ?? captureReportContext(reportTimeZone), role, isCurrent: current, isMainProfile: () => current() && context.facts.identity.primaryProfile === true,
        artwork: (id, kind, revision) => revision ? context.artwork.foreground(encodedRoutePath('get_items_id_art_kind', {id: encodeURIComponent(id), kind}) + '?' + new URLSearchParams({v: revision}), kind === 'backdrop' ? 800 : 400, kind === 'poster' ? 2 / 3 : kind === 'backdrop' ? 16 / 9 : 1) : undefined,
        effects: {openHome, openSettings, reportError, openHistory: report => {if (current()) {historyContexts.set(context, report);openSettings('history');}}}});
    }
    // The owner's Libraries page owns its own bounded directory read and scan
    // commands; the settings controller keeps the per-library panel only.
    const libraries = route.section === 'libraries' && identity.primaryProfile === true && identity.capabilities?.manageServer === true && (role === 'owner' || role === 'admin')
      ? new LibraryManagementService({api: context.api, scope: {serverId: viewer.serverId, viewerId: options.viewerId}}) : undefined;
    // Storage & backups owns one bounded usage sweep shared by its drawn rows.
    const server = route.section === 'storage' ? createServerSettingsSurfaces({transport: context.api, serverId: viewer.serverId, isCurrent: current, openTarget: openSettingsRow,
      backups: controller.backups, subscribe: controller.subscribe, scopeKey: `${viewer.serverId}/${options.viewerId}`, reportError}) : undefined;
    const troubleshooting = route.section === 'troubleshooting' ? createTroubleshootingSurfaces({transport: context.api, serverId: viewer.serverId,
      viewerId: options.viewerId, downloadBase: context.api.baseUrl, isCurrent: current, subscribe: controller.subscribe, reportError}) : undefined;
    const live = route.section === 'live' && role === 'owner' && identity.primaryProfile === true && identity.capabilities?.manageServer === true
      ? createOwnerSources({api: context.api, serverId: viewer.serverId, viewerId: options.viewerId,
        isCurrent: () => current() && context.session().viewer.role === 'owner' && context.session().viewer.serverId === viewer.serverId && context.session().viewer.authority === viewer.authority && context.session().viewer.accountId === viewer.accountId && context.session().viewer.profileId === viewer.profileId && context.facts.identity.primaryProfile === true && context.facts.identity.capabilities?.manageServer === true,
        reportError}) : undefined;
    const source = createSettingsBindings(controller, options, identity, {openHome, signIn,
      special: (row, state, rowContext, openPanel) => {
        const storageControl = server?.special(row, state);
        if (storageControl !== undefined) return storageControl;
        const privacyControl = privacy.special(row, state);
        if (privacyControl !== undefined) return privacyControl;
        const languageKind = preferredLanguageKind(row.id);
        if (languageKind) return <AudioSubtitleLanguageList row={row} state={state} locale={uiLocale} appLocale={appLocale}
          onChange={value => {if (current()) controller.change(row, value, rowContext);}} onError={reportError} />;
        if (isSubtitlePreview(row.id)) {
          const value = (id: string) => controller.rowState(settingsRowIndex.get(id)!.row, rowContext).value;
          return <SubtitlePreview paint={subtitlePreviewPaint({size: value('audioSubtitles.subtitleSize'), color: value('audioSubtitles.subtitleColor'),
            background: value('audioSubtitles.subtitleBackground'), position: value('audioSubtitles.subtitlePosition')})}
            text={defaultI18n.t('next.settingsSpec.account.audio-subtitles.groups.2.rows.0.control.text')} />;
        }
        if (isDeviceSyntheticRow(row.id)) return <DeviceRowControl row={row} onSwitchServer={() => {if (current()) void router.navigate({to: '/servers'});}}/>;
        if (row.id === 'music.eq') return <MusicEqualizerRow platform={platform} isCurrent={current}/>;
        if (row.id === 'appearance.customizeHome') return <CustomizeHomeRow api={context.api} home={context.home} serverId={viewer.serverId} viewerId={options.viewerId} isCurrent={current}/>;
        if (row.id === 'libraries.list' && libraries) return <LibrariesPanel api={context.api} service={libraries}
          onEdit={libraryId => { if (current()) openPanel('library', libraryId); }} onError={reportError} />;
        if (row.id === 'libraries.folders' && isLibraryPanelContext(rowContext) && rowContext.recordId) {
          return <LibraryFolders libraryId={rowContext.recordId} api={options.api}
            scope={{serverId: viewer.serverId, viewerId: options.viewerId}} isCurrent={current} onError={reportError} />;
        }
        if (row.control.type === 'action' && libraryScanAction(row.id) && isLibraryPanelContext(rowContext) && rowContext.recordId && libraries)
          return <LibraryScanAction rowId={row.id} label={defaultI18n.t(row.control.label ?? row.label)} libraryId={rowContext.recordId} service={libraries} onError={reportError} />;
        return row.id === 'metadata.perLibrary' ? <Button variant="glass" size="small" onClick={() => openSettings('libraries')}>{defaultI18n.t(row.control.label ?? row.label)}</Button> : null;
      },
      section: selected => selected.section.id === 'live' && live ? <OwnerSourcesPage source={live}/> : selected.section.id === 'remote-sources' ? <RemoteSourcesPage api={context.api} serverId={viewer.serverId} viewerId={options.viewerId} isCurrent={current} reportError={reportError} /> : selected.section.id === 'people' && people ? <PeopleScreen bindings={people} /> : selected.section.id === 'dashboard' && dashboard ? <DashboardScreen bindings={dashboard} embedded /> : selected.section.id === 'history' && dashboard ? <HistoryScreen bindings={dashboard} embedded /> : selected.section.id === 'troubleshooting' ? troubleshooting?.render() : undefined,
    });
    const deviceFacts = {serverName: identity.serverName, ...(identity.serverVersion === undefined ? {} : {serverVersion: identity.serverVersion})};
    const settings: SettingsBindings = {...source,
      // The Remote sources page owns its own checked source reads; the generic
      // server-section configuration read would replace that page with a failure.
      getSnapshot: section => section.scope === 'server' && (section.id === 'remote-sources' || section.id === 'live' && !!live) ? remoteSourcesPageSnapshot : source.getSnapshot(section),
      load: section => section.scope === 'server' && (section.id === 'remote-sources' || section.id === 'live' && !!live) ? Promise.resolve() : source.load(section),
      eligible: (row, selected) => server?.eligible(row) ?? (row.id === 'libraries.folders' ? current() && isLibraryPanelContext(selected) && controller.sectionVisible(selected.section) : libraryScanAction(row.id) ? current() && !!libraries && selected.panelId === 'library' && !!selected.recordId && controller.sectionVisible(selected.section) : isSubtitlePreview(row.id) ? current() && controller.sectionVisible(selected.section) : isDeviceSyntheticRow(row.id) ? current() && controller.sectionVisible(selected.section) && deviceRowState(row, deviceFacts) !== undefined : row.id === 'music.eq' ? current() && controller.sectionVisible(selected.section) && musicEqualizerAvailable(platform) : row.id === 'people.table' ? !!people && current() && controller.sectionVisible(selected.section) : row.id === 'metadata.perLibrary' || row.id === 'appearance.customizeHome' ? current() && controller.sectionVisible(selected.section) : source.eligible(row, selected)),
      rowState: (row, selected) => {
        const privacyState = privacy.rowState(row);
        if (privacyState !== undefined) return privacyState;
        if (row.id === 'libraries.folders') return {present: current() && isLibraryPanelContext(selected) && controller.sectionVisible(selected.section)};
        if (libraryScanAction(row.id)) return {present: current() && !!libraries && selected.panelId === 'library' && !!selected.recordId && controller.sectionVisible(selected.section)};
        if (isSubtitlePreview(row.id)) return {present: current() && controller.sectionVisible(selected.section)};
        const device = isDeviceSyntheticRow(row.id) ? deviceRowState(row, deviceFacts) : undefined;
        return server?.rowState(row) ?? device ?? (row.id === 'music.eq' ? {present: current() && controller.sectionVisible(selected.section) && musicEqualizerAvailable(platform)} : row.id === 'people.table' ? {present: !!people && current() && controller.sectionVisible(selected.section)} : row.id === 'metadata.perLibrary' || row.id === 'appearance.customizeHome' ? {present: current() && controller.sectionVisible(selected.section)} : source.rowState(row, selected));
      },
    };
    setFailure(false);setOwned({context, settings, dashboard});
    return () => {active = false;signIn?.dispose();people?.invitations?.dispose();people?.client.dispose();for (const listener of authorityListeners) {try {listener();} catch { /* Retire the other captured readers too. */ }}authorityListeners.clear();dashboard?.dispose();libraries?.dispose();privacy.dispose();server?.dispose();troubleshooting?.dispose();live?.dispose();controller.dispose();};
  // The captured identity decides which sections exist (main profile, manageServer,
  // role). It can be replaced without the assembly context object changing, so
  // every fact the gate reads is a dependency: a newer identity must rebuild the
  // controller instead of leaving the old section set on screen.
  }, [context, retry, route.section, reportTimeZone, context?.facts.identity.primaryProfile,
    context?.facts.identity.capabilities?.manageServer, context?.facts.identity.capabilities?.managePeople,
    context?.facts.identity.capabilities?.manageSignIn, context?.facts.identity.signIn,
    context?.session().viewer.role, context?.session().viewer.serverId, context?.session().viewer.authority, context?.session().viewer.accountId, context?.session().viewer.profileId]);
  if (!protectedContext || !owned || owned.context !== context) return null;
  const navigate = (next: Parameters<typeof SettingsScreen>[0]['route']) => {if (context?.isCurrent()) void router.navigate({to: '/settings', search: () => next});};
  const settings = failure ? {...owned.settings, section: () => <SettingsPageFailure retry={() => setRetry(value => value + 1)} home={owned.settings.openHome} />} : owned.settings;
  // A URL that names its scope or section was a link to that page (a Dashboard
  // or storage alert), so the compact layout opens the page, not the list.
  const openDetail = typeof raw.scope === 'string' || typeof raw.section === 'string';
  return <SettingsScreen bindings={settings} route={route} navigate={navigate} openDetail={openDetail} />;
}

export default defineWebScreen({id: 'settings', component: SettingsEntry, validateSearch: raw => settingsRoute(raw, () => true)});
