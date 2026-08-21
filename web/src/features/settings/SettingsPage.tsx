import { productMessage, type SettingsDocument, type SettingsSummaryResponse } from '@porticomediaserver/client-core';
import { AlertTriangle, ChevronRight, RefreshCw, Server } from '#portico-icons';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link, NavLink, useParams } from 'react-router-dom';
import { productText, reviewedProductErrorText } from '../../components/ProductLanguage';
import type { FilesystemPickerSource } from '../filesystem';
import type { PorticoDataSource } from '../../data/models';
import { canManageLibraries, canManageServer } from '../../data/authority';
import { LibraryOperations } from './LibraryOperations';
import { DLNAOperations } from './IntegrationOperations';
import { LiveTVOperations } from './LiveTVOperations';
import { LibraryChannelOperations } from './LibraryChannelOperations';
import { MaintenanceOperations } from './MaintenanceOperations';
import { PeopleOperations } from './PeopleOperations';
import { PlaybackOperations } from './PlaybackOperations';
import {
  AccountSettings,
  HelpSettings,
  PersonalPreferencesSettings,
} from './PersonalSettings';
import { RemoteAccessSettingsPanel } from './RemoteAccessSettings';
import { ServerConsole } from './ServerConsole';
import { ServerSettingsForm } from './ServerSettingsForm';
import { SettingsError, SettingsLoading, SettingsSaveCoordinator, useSettingsNavigationGuard } from './SettingsControls';
import { useSettingsQuery } from './settingsHooks';
import {
  personalSettingsSections,
  canViewServerSettingsSection,
  serverSectionsForViewer,
  serverSettingsSections,
  type SettingsGroupCapability,
  type SettingsViewerAccess,
} from './settingsModel';
import { StatusDashboard } from './StatusDashboard';
import { ProfilesSettings } from './ProfilesSettings';
import { ViewerMessagesSettings } from './ViewerMessagesSettings';
import type { SettingsDataSource, SettingsOperationalSnapshot, SettingsViewer } from './settingsTypes';
import './settings.css';

type SettingsWorkspace = { document: SettingsDocument; summary: SettingsSummaryResponse };

const loadWorkspace = (source: SettingsDataSource, signal: AbortSignal): Promise<SettingsWorkspace> => Promise.all([
  source.settings(signal), source.settingsSummary(signal),
]).then(([document, summary]) => ({ document, summary }));

const operationalSections = new Set(['media', 'people', 'maintenance', 'diagnostics', 'help']);

function SettingsNavigation({ current, capabilities, access, canManageViewerMessages }: { current: string; capabilities: SettingsGroupCapability[]; access: SettingsViewerAccess; canManageViewerMessages: boolean }) {
  const activeRef = useRef<HTMLAnchorElement>(null);
  const requestNavigation = useSettingsNavigationGuard();
  const serverSections = serverSectionsForViewer(capabilities, access);
  useEffect(() => {
    if (window.innerWidth > 1380) return;
    const frame = window.requestAnimationFrame(() => activeRef.current?.scrollIntoView({ behavior: 'auto', block: 'nearest', inline: 'center' }));
    return () => window.cancelAnimationFrame(frame);
  }, [current]);
  const displayLabel = (id: string, label: string, server: boolean) => id === 'playback' || id === 'personal-playback' ? `${server ? 'Server' : 'My'} playback` : label;
  const link = (id: string, label: string, server: boolean) => <NavLink ref={current === id ? activeRef : undefined} key={id} className={({ isActive }) => isActive ? 'active' : ''} to={`/settings/${id}`} onClick={(event) => { if (!requestNavigation) return; event.preventDefault(); requestNavigation(`/settings/${id}`); }}>{displayLabel(id, label, server)}</NavLink>;
  const options = <>{(serverSections.length > 0 || canManageViewerMessages) && <optgroup label="This Server">{serverSections.map((section) => <option key={section.id} value={section.id}>{displayLabel(section.id, section.label, true)}</option>)}{canManageViewerMessages && <option value="viewer-messages">{productText('feedback.viewer-messages-title')}</option>}</optgroup>}<optgroup label="Personal">{personalSettingsSections.map((section) => <option key={section.id} value={section.id}>{displayLabel(section.id, section.label, false)}</option>)}</optgroup></>;
  return <><label className="portico-settings-mobile-nav"><span>Settings section</span><select value={current} onChange={(event) => requestNavigation?.(`/settings/${event.target.value}`)}>{options}</select></label><aside className="portico-settings-nav" aria-label="Settings sections">
    {(serverSections.length > 0 || canManageViewerMessages) && <section><h2>This Server</h2><div>{serverSections.map((section) => link(section.id, section.label, true))}{canManageViewerMessages && link('viewer-messages', productText('feedback.viewer-messages-title'), true)}</div></section>}
    <section><h2>Personal</h2><div>{personalSettingsSections.map((section) => link(section.id, section.label, false))}</div></section>
  </aside></>;
}

function OperationState({ state, retry, label, children }: { state: ReturnType<typeof useSettingsQuery<SettingsOperationalSnapshot | null>>; retry: () => void; label: string; children: (operations: SettingsOperationalSnapshot) => React.ReactNode }) {
  if (state.status === 'loading') return <SettingsLoading label={`Loading ${label}`} />;
  if (state.status === 'error') return <SettingsError title={`${label} are unavailable`} message={reviewedProductErrorText(state.error, 'settings.load-failed', { sectionName: label })} onRetry={retry} />;
  if (!state.data) return null;
  return children(state.data);
}

function ServerSection({ section, workspace, viewer, source, filesystemSource, operations, canManageServerSettings, onWorkspaceChange, onWorkspaceReload, onOperationsReload }: {
  section: string;
  workspace?: SettingsWorkspace;
  viewer: SettingsViewer;
  source: SettingsDataSource;
  filesystemSource?: FilesystemPickerSource;
  operations: ReturnType<typeof useSettingsQuery<SettingsOperationalSnapshot | null>>;
  canManageServerSettings: boolean;
  onWorkspaceChange: (document: SettingsDocument) => void;
  onWorkspaceReload: () => void;
  onOperationsReload: () => void;
}) {
  if (section === 'status') return <StatusDashboard source={source} viewer={viewer} />;
  const form = canManageServerSettings && workspace ? <ServerSettingsForm section={section} document={workspace.document} summary={workspace.summary} viewer={viewer} source={source} onDocumentChange={onWorkspaceChange} onReload={onWorkspaceReload} /> : null;
  if (section === 'media') return <div className="portico-settings-content">{form}<OperationState state={operations} retry={onOperationsReload} label="library administration tools">{(data) => <LibraryOperations
    libraries={data.libraries}
    source={source}
    filesystemSource={filesystemSource}
    canManage={canManageLibraries(viewer)}
    canBrowseFilesystem={canManageServer(viewer)}
    onChanged={onOperationsReload}
  />}</OperationState></div>;
  if (section === 'playback') return <div className="portico-settings-content">{form}<PlaybackOperations source={source} viewer={viewer} /></div>;
  if (section === 'live') return <div className="portico-settings-content"><LiveTVOperations source={source} viewer={viewer} /><LibraryChannelOperations source={source} viewer={viewer} />{form}</div>;
  if (section === 'connectivity') return <div className="portico-settings-content"><RemoteAccessSettingsPanel source={source} viewer={viewer} />{form}<DLNAOperations source={source} viewer={viewer} /></div>;
  if (section === 'people') return <div className="portico-settings-content">{form}<OperationState state={operations} retry={onOperationsReload} label="people and access data">{(data) => <PeopleOperations operations={data} source={source} onChanged={onOperationsReload} />}</OperationState></div>;
  if (section === 'maintenance') return <div className="portico-settings-content">{form}<OperationState state={operations} retry={onOperationsReload} label="maintenance data">{(data) => <MaintenanceOperations tasks={data.tasks} backups={data.backups} storage={data.storage} source={source} onChanged={onOperationsReload} />}</OperationState></div>;
  if (section === 'diagnostics') return <div className="portico-settings-content">{form}<OperationState state={operations} retry={onOperationsReload} label="diagnostic data">{(data) => <ServerConsole source={source} diagnostics={data.diagnostics} release={data.release} />}</OperationState></div>;
  return form;
}

function PersonalSection({ section, viewer, source, productSource, operations, onOperationsReload }: { section: string; viewer: SettingsViewer; source: SettingsDataSource; productSource?: PorticoDataSource; operations: ReturnType<typeof useSettingsQuery<SettingsOperationalSnapshot | null>>; onOperationsReload: () => void }) {
  if (section === 'appearance' || section === 'personal-playback' || section === 'privacy') return <PersonalPreferencesSettings section={section} source={source} />;
  if (section === 'account') return <AccountSettings viewer={viewer} source={source} />;
  if (section === 'profiles') return productSource ? <ProfilesSettings source={productSource} /> : <SettingsError title="Profiles are unavailable" message="Reconnect to this server and try again." onRetry={() => window.location.reload()} />;
  if (section === 'help') return <OperationState state={operations} retry={onOperationsReload} label="Help & About details">{(data) => <HelpSettings operations={data} />}</OperationState>;
  return <div className="portico-settings-state error"><AlertTriangle /><strong>This settings link is invalid</strong><p>Choose a section from the settings navigation.</p><Link className="button secondary" to="/settings/account">Open account settings</Link></div>;
}

export function SettingsPage({ source, productSource, viewer, filesystemSource }: { source: SettingsDataSource; productSource?: PorticoDataSource; viewer: SettingsViewer; filesystemSource?: FilesystemPickerSource }) {
  const { section: routeSection = 'status' } = useParams();
  const routeIsKnown = routeSection === 'viewer-messages' || serverSettingsSections.some((candidate) => candidate.id === routeSection) || personalSettingsSections.some((candidate) => candidate.id === routeSection);
  const section = routeSection;
  const serverSection = serverSettingsSections.find((candidate) => candidate.id === section);
  const personalSection = personalSettingsSections.find((candidate) => candidate.id === section);
  const isViewerMessages = section === 'viewer-messages';
  const isServer = Boolean(serverSection) || isViewerMessages;
  const access = useMemo<SettingsViewerAccess>(() => ({
    canManageServer: canManageServer(viewer),
    canManageLibraries: canManageLibraries(viewer),
  }), [viewer]);
  const canManageViewerMessages = access.canManageServer;
  const serverRouteDenied = isViewerMessages ? !canManageViewerMessages : isServer && !canViewServerSettingsSection(section, access);
  const needsWorkspace = isServer && !isViewerMessages && access.canManageServer;
  const [workspaceRevision, setWorkspaceRevision] = useState(0);
  const [operationsRevision, setOperationsRevision] = useState(0);
  const [workspaceOverride, setWorkspaceOverride] = useState<SettingsWorkspace | null>(null);
  const loadSectionWorkspace = useCallback((next: SettingsDataSource, signal: AbortSignal) => needsWorkspace ? loadWorkspace(next, signal) : Promise.resolve(null), [needsWorkspace]);
  const workspace = useSettingsQuery<SettingsWorkspace | null>(loadSectionWorkspace, source, workspaceRevision);
  const needsOperations = !serverRouteDenied && operationalSections.has(section);
  const loadOperations = useCallback((next: SettingsDataSource, signal: AbortSignal) => needsOperations ? next.settingsOperations(section as 'media' | 'people' | 'maintenance' | 'diagnostics' | 'help', signal) : Promise.resolve(null), [needsOperations, section]);
  const operations = useSettingsQuery(loadOperations, source, operationsRevision);

  useEffect(() => {
    if (!needsWorkspace) {
      setWorkspaceOverride(null);
      return;
    }
    if (workspace.status === 'success' && workspace.data) setWorkspaceOverride(workspace.data);
  }, [needsWorkspace, workspace]);

  const currentWorkspace = workspaceOverride ?? (workspace.status === 'success' ? workspace.data ?? undefined : undefined);
  const definition = isViewerMessages ? { label: productText('feedback.viewer-messages-title'), description: productText('feedback.viewer-messages-page-description') } : serverSection ?? personalSection;
  const title = definition?.label ?? 'Status';
  const description = definition?.description ?? 'Server health, activity, capacity, connectivity, and work that needs attention.';
  const viewerMessagesUnavailable = productMessage('feedback.viewer-messages-unavailable');
  const capabilities = useMemo<SettingsGroupCapability[]>(() => currentWorkspace?.summary.groups.map((group) => ({
    id: group.id,
    label: group.label,
    summary: group.summary,
    implemented: group.implemented,
    readOnly: group.readOnly,
    configured: group.configured,
    dangerous: group.dangerous,
    requiresPorticoClaim: group.requiresPorticoClaim,
    requiresRuntimeDependency: group.requiresRuntimeDependency,
    status: group.status,
  })) ?? [], [currentWorkspace]);

  if (!routeIsKnown) return <div className="portico-settings-page"><div className="portico-settings-state error" role="status"><AlertTriangle /><strong>Settings section not found</strong><p>The address does not match a settings section available in Portico.</p><Link className="button secondary" to="/settings/account">Open account settings</Link></div></div>;
  if (serverRouteDenied) return <div className="portico-settings-page"><div className="portico-settings-state error" role="status"><AlertTriangle /><strong>Server settings aren’t available</strong><p>Your account can use personal settings, but it cannot administer this server.</p><Link className="button secondary" to="/settings/account">Open personal settings</Link></div></div>;
  if (needsWorkspace && workspace.status === 'loading' && !currentWorkspace) return <div className="portico-settings-page"><SettingsLoading label={`Loading ${title}`} /></div>;
  if (needsWorkspace && workspace.status === 'error' && !currentWorkspace) return <div className="portico-settings-page"><SettingsError title={`${title} is unavailable`} message={reviewedProductErrorText(workspace.error, 'settings.load-failed', { sectionName: title })} onRetry={() => setWorkspaceRevision((current) => current + 1)} /></div>;
  const canRenderServerSection = isServer && (Boolean(currentWorkspace) || (section === 'media' && access.canManageLibraries));
  return <div className="portico-settings-page">
    <header className="portico-settings-header"><nav aria-label="Breadcrumb"><span>Settings</span><ChevronRight /><span>{isServer ? 'This Server' : 'Personal'}</span></nav><div><h1>{title}</h1><p>{description}</p></div>{isServer && section !== 'status' && currentWorkspace && <span className="portico-settings-document-meta"><Server /> Updated {new Date(currentWorkspace.document.updatedAt).toLocaleString()}</span>}</header>
    <SettingsSaveCoordinator><div className="portico-settings-layout">
      <SettingsNavigation current={section} capabilities={capabilities} access={access} canManageViewerMessages={canManageViewerMessages} />
      <section className="portico-settings-workspace" aria-label={`${title} settings`}>
        {isServer && currentWorkspace && workspace.status === 'error' && <div className="portico-settings-inline-error"><AlertTriangle />Unable to refresh the settings document. Showing the last loaded revision.<button type="button" onClick={() => setWorkspaceRevision((current) => current + 1)}><RefreshCw /> Retry</button></div>}
        {isViewerMessages && productSource ? <ViewerMessagesSettings source={productSource} /> : isViewerMessages ? <SettingsError title={viewerMessagesUnavailable.title ?? productText('feedback.viewer-messages-title')} message={viewerMessagesUnavailable.body ?? productText('problem.request-failed')} onRetry={() => window.location.reload()} /> : canRenderServerSection ? <ServerSection section={section} workspace={currentWorkspace} viewer={viewer} source={source} filesystemSource={filesystemSource} operations={operations} canManageServerSettings={access.canManageServer} onWorkspaceChange={(document) => setWorkspaceOverride((current) => current ? { ...current, document } : currentWorkspace ? { document, summary: currentWorkspace.summary } : { document, summary: { generatedAt: new Date().toISOString(), groups: [], statusCards: [] } })} onWorkspaceReload={() => { setWorkspaceOverride(null); setWorkspaceRevision((current) => current + 1); }} onOperationsReload={() => setOperationsRevision((current) => current + 1)} /> : !isServer ? <PersonalSection section={section} viewer={viewer} source={source} productSource={productSource} operations={operations} onOperationsReload={() => setOperationsRevision((current) => current + 1)} /> : null}
      </section>
    </div></SettingsSaveCoordinator>
  </div>;
}

export default SettingsPage;
