import {useEffect, useRef, useState, useSyncExternalStore, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import {settingsRowIndex, settingsSpec, type SettingsRow, type SettingsSection, type SettingsValue} from '@core/presentation/settings-spec.ts';
import {SettingsNav, type SettingsSearchGroup} from '../../kit/shell/SettingsSurfaces';
import {SettingsPageHeading, SettingsPanel, SettingsRows, SettingsWorkspace, searchSettings, settingsRoute, type SettingPresentation, type SettingsRoute, type SettingsPanelProps} from '../../kit/settings';
import {StateView} from '../../kit/states';
import type {FactPart} from '@core/presentation/kit-model/index.ts';
import {settingsIconName} from '../../kit/settings/icons';
import {ServerHeader, type ServerAudience} from '../../kit/shell/ServerIdentity';
import {useShellWidth} from '../../shell/viewport';

const t = defaultI18n.t;
export type SettingsContext = Readonly<{section: SettingsSection; panelId?: string; tabId?: string; recordId?: string}>;
export type SettingsPageSnapshot = Readonly<{loading: boolean; error?: unknown; saveState?: 'saving' | 'saved'}>;
export type SettingsConfigurationMatch = Readonly<{id: string; key: string; label: string; current: unknown; effective: unknown; default: unknown; source: string; configOnly: boolean; path: string}>;
/** Session-owned, checked readers and commands. The screen holds navigation and focus only. */
export type SettingsBindings = Readonly<{
  accountName: string; serverName: string; role: 'owner' | 'admin' | 'member';
  subscribe: (listener: () => void) => () => void; getSnapshot: () => SettingsPageSnapshot;
  sectionVisible: (section: SettingsSection) => boolean;
  audience: (section: SettingsSection) => ServerAudience;
  serverHealth?: () => 'normal' | 'attention' | undefined;
  /** Approved uptime, free-space and database facts, in order, from checked live readers. */
  serverFacts: () => readonly FactPart[];
  rowState: (row: SettingsRow, context: SettingsContext) => SettingPresentation;
  change: (row: SettingsRow, value: SettingsValue, context: SettingsContext) => void;
  reset: (row: SettingsRow, context: SettingsContext) => void;
  special: (row: SettingsRow, state: SettingPresentation, context: SettingsContext, openPanel: (panelId: string, recordId?: string) => void) => ReactNode;
  section?: (context: SettingsContext, openPanel: (panelId: string, recordId?: string) => void) => ReactNode;
  configurationMatches?: (query: string) => readonly SettingsConfigurationMatch[];
  panel?: (context: SettingsContext) => Readonly<{title?: string; description?: string; create?: SettingsPanelProps['create']}>;
  load: () => Promise<void>;
}>;

export function SettingsScreen({bindings, route, navigate}: {bindings: SettingsBindings; route: SettingsRoute; navigate: (route: SettingsRoute) => void}) {
  const snapshot = useSyncExternalStore(bindings.subscribe, bindings.getSnapshot, bindings.getSnapshot);
  const width = useShellWidth(), compact = width < 900;
  const [query, setQuery] = useState(''), [compactPage, setCompactPage] = useState(false), [advanced, setAdvanced] = useState(false);
  const [panel, setPanel] = useState<{id: string; recordId?: string; tab?: string} | undefined>();
  const [configurationKey, setConfigurationKey] = useState<string>();
  const opener = useRef<HTMLElement | null>(null), heading = useRef<HTMLDivElement>(null);
  const selected = settingsRoute(route, bindings.sectionVisible);
  const section = settingsSpec.find(section => section.scope === selected.scope && section.id === selected.section)!;
  const context: SettingsContext = {section};
  const openPanel = (id: string, recordId?: string) => {
    if (!section.panels?.[id]) return;
    opener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    setPanel({id, recordId, tab: section.panels[id].tabs?.[0]?.id});
  };
  const rowState = (row: SettingsRow, context: SettingsContext) => bindings.rowState(row, context);
  const matches = searchSettings(query, t, (row, owner) => bindings.sectionVisible(owner) && rowState(row, {section: owner}).present);
  const configuration = bindings.configurationMatches?.(query) ?? [];
  const configurationGroups: SettingsSearchGroup[] = configuration.filter(fact => fact.configOnly).length ? [{id: 'configuration', label: t('next.settings.configurationFile'), icon: 'i-server', area: 'server', configuration: true, rows: configuration.filter(fact => fact.configOnly).map(fact => ({id: fact.key, label: fact.label, description: fact.key}))}] : [];
  const groups: SettingsSearchGroup[] = settingsSpec.map(section => ({id: `${section.scope}/${section.id}`, label: t(section.title), icon: settingsIconName(section.icon), area: section.scope,
    rows: matches.filter(hit => hit.section === section).map(hit => ({id: hit.rowId, label: hit.label, description: hit.context.join(' · ')}))})).filter(group => group.rows.length);
  const go = (id: string, row?: string) => {
    if (id === 'configuration') {setConfigurationKey(row); setCompactPage(true); return;}
    const [scope, page] = id.split('/');
    if (scope !== 'account' && scope !== 'server') return;
    navigate({scope, section: page, ...(row ? {row} : {})}); setConfigurationKey(undefined); setPanel(undefined); setAdvanced(false); setCompactPage(true);
  };
  useEffect(() => {void bindings.load();}, [bindings]);
  useEffect(() => {
    const found = selected.row && settingsRowIndex.get(selected.row);
    if (!found) return;
    if (found.advanced) setAdvanced(true);
    if (found.panelId) setPanel({id: found.panelId, tab: found.tabId});
    setCompactPage(true);
    const frame = requestAnimationFrame(() => document.getElementById(found.row.id)?.scrollIntoView({block: 'center', behavior: 'instant'}));
    return () => cancelAnimationFrame(frame);
  }, [selected.scope, selected.section, selected.row]);
  const rows = (holder: Parameters<typeof SettingsRows>[0]['holder'], context: SettingsContext) => <SettingsRows holder={holder} state={row => rowState(row, context)}
    change={(row, value) => bindings.change(row, value, context)} reset={row => bindings.reset(row, context)} renderSpecial={(row, state) => bindings.special(row, state, context, openPanel)}
    query={query} foundRowId={selected.row} advancedOpen={advanced} onAdvancedChange={setAdvanced} />;
  const navigation = <SettingsNav accountName={bindings.accountName} serverName={bindings.serverName} viewerRole={bindings.role === 'admin' ? 'administrator' : bindings.role}
    account={settingsSpec.filter(section => section.scope === 'account' && bindings.sectionVisible(section)).map(section => ({id: `account/${section.id}`, label: t(section.title), icon: settingsIconName(section.icon), cluster: section.navGroup}))}
    server={settingsSpec.filter(section => section.scope === 'server' && bindings.sectionVisible(section)).map(section => ({id: `server/${section.id}`, label: t(section.title), icon: settingsIconName(section.icon), cluster: section.navGroup}))}
    selectedId={`${selected.scope}/${selected.section}`} selectedSearchResult={selected.row ? {pageId: `${selected.scope}/${selected.section}`, rowId: selected.row} : undefined}
    query={query} onQueryChange={setQuery} onNavigate={go} matches={[...groups, ...configurationGroups]} onSearchResult={go} layout={compact ? 'phone' : 'desktop'} />;
  const panelSpec = panel && section.panels?.[panel.id], tab = panelSpec?.tabs?.find(tab => tab.id === panel?.tab);
  const panelContext: SettingsContext = {section, panelId: panel?.id, tabId: tab?.id, recordId: panel?.recordId};
  const panelState = bindings.panel?.(panelContext);
  const audience = bindings.audience(section);
  const fact = configuration.find(fact => fact.key === configurationKey);
  return <SettingsWorkspace scope={selected.scope} navigation={navigation} compactPage={compactPage} onBack={() => setCompactPage(false)}>
    {selected.scope === 'server' && bindings.role !== 'member' && (bindings.role === 'owner' ? <ServerHeader serverName={bindings.serverName} viewerRole="owner" audience={audience} health={bindings.serverHealth?.()} facts={bindings.serverFacts()} /> : <ServerHeader serverName={bindings.serverName} viewerRole="administrator" audience="administrators" health={bindings.serverHealth?.()} facts={bindings.serverFacts()} />)}
    <div ref={heading}><SettingsPageHeading title={t(section.title)} saveState={snapshot.saveState} /></div>
    {fact ? <ConfigurationFact fact={fact} /> : snapshot.error ? <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')} primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => {void bindings.load();}}} /> : snapshot.loading ? <p role="status">{t('next.settings.loading')}</p> : bindings.section?.(context, openPanel) ?? rows(section, context)}
    {panel && panelSpec && <SettingsPanel open onClose={() => setPanel(undefined)} audience={t(audience === 'owner' ? 'next.shell.editorAudience.owner' : 'next.shell.serverVisibility.ownerShared')} title={panelState?.title ?? (panelSpec.title ? t(panelSpec.title) : t(section.title))} description={panelState?.description} create={panelState?.create} returnFocusRef={opener}
      saveState={snapshot.saveState} tabs={panelSpec.tabs?.map(tab => ({id: tab.id, title: t(tab.title), content: rows(tab, {...panelContext, tabId: tab.id})}))} selectedTab={panel.tab} onTabChange={value => setPanel(current => current && {...current, tab: value})}>
      {rows(tab ?? panelSpec, panelContext)}
    </SettingsPanel>}
  </SettingsWorkspace>;
}

function ConfigurationFact({fact}: {fact: SettingsConfigurationMatch}) {
  const value = (value: unknown) => value === null ? t('next.settings.notSet') : typeof value === 'string' ? value : JSON.stringify(value);
  return <section aria-label={fact.label}><h2>{fact.label}</h2><p>{t('next.settings.configOnly', {path: fact.path})}</p><code>{fact.key}</code><dl>
    <dt>{t('next.settings.current')}</dt><dd>{value(fact.current)}</dd><dt>{t('next.settings.effective')}</dt><dd>{value(fact.effective)}</dd>
    <dt>{t('next.settings.default')}</dt><dd>{value(fact.default)}</dd><dt>{t('next.settings.source')}</dt><dd>{t(fact.source === 'file' ? 'next.settings.source.file' : fact.source === 'server' ? 'next.settings.source.server' : 'next.settings.source.default')}</dd>
  </dl></section>;
}
