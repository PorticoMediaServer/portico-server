// Visual boundary only: the actual App/Shell/Settings renderer with immutable approved frame data.
// No service writes, engine availability or producer acceptance are established by this fixture.
import {createRoot} from 'react-dom/client';
import {useRef, useState} from 'react';
import {createMemoryHistory} from '@tanstack/react-router';
import {App} from '../../src/App';
import {router} from '../../src/router';
import type {ShellBindings} from '../../src/shell/bindings';
import type {SettingsBindings, SettingsPageSnapshot} from '../../src/screens/settings/Settings';
import {settingsRowIndex, type SettingsValue} from '../../../packages/client-core/src/presentation/settings-spec';
import {TopBarActivity} from '../../src/kit/shell';
import {BackupPassphraseDialog, SettingsAction, SettingsGroup, SettingsTable, type TableQuery} from '../../src/kit/settings';
import {Button} from '../../src/kit/controls';
import '../../src/kit/tokens.css';
import '../../src/kit/fonts.css';
import '../../src/bootstrap.css';

const parameters = new URLSearchParams(location.search);
const approved = await fetch('http://100.120.50.24:5269/spec.json').then(response => {
  if (!response.ok) throw new Error('immutable_design_fixture_unavailable');
  return response.json();
});
const originals = new Map<string, any>();
const walk = (holder: any) => {
  for (const group of [...(holder.groups ?? []), ...(holder.advanced?.groups ?? [])]) for (const row of group.rows ?? []) visit(row);
  for (const tab of holder.tabs ?? []) walk(tab);
  for (const panel of Object.values(holder.panels ?? {})) walk(panel);
};
const visit = (row: any) => {originals.set(row.id, row);for (const child of row.children ?? []) visit(child);};
for (const section of approved.sections) walk(section);
const changed = new Set<() => void>(), values = new Map<string, SettingsValue>();
let snapshot: SettingsPageSnapshot = Object.freeze({loading: false, ...(parameters.get('state') === 'failed' ? {error: new Error('fixture_load_failure')} : {})}), loads = 0;
const notify = () => {snapshot = Object.freeze({...snapshot});for (const listener of changed) listener();};
function OwnerActivity() {
  const [open, setOpen] = useState(false);
  return <TopBarActivity viewerRole="owner" open={open} onOpenChange={setOpen} view={{phase: 'ready', serverName: 'Portico demo', streamCount: 2, streams: [], scans: [], tasks: [], onRetry() {}, onDashboard() {}}} />;
}
function PeopleTable() {
  const table = originals.get('people.table').control;
  const [query, setQuery] = useState<TableQuery>({search: '', filters: {role: 'all', status: 'all'}, sort: {by: 'active', direction: 'desc'}});
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const cellKey = (row: any) => {const cell = row.cells[query.sort.by];return typeof cell === 'string' ? cell : cell.sort ?? cell.title;};
  const rows = table.rows.filter((row: any) => (!query.search || JSON.stringify(row.cells).toLowerCase().includes(query.search.toLowerCase())) && (query.filters.role === 'all' || row.f.role === query.filters.role) && (query.filters.status === 'all' || row.f.status.includes(query.filters.status))).sort((a: any, b: any) => {
    const x = cellKey(a), y = cellKey(b), direction = query.sort.direction === 'desc' ? -1 : 1;
    return direction * (typeof x === 'number' && typeof y === 'number' ? x - y : String(x).localeCompare(String(y), undefined, {numeric: true}));
  });
  return <SettingsGroup title="People" plain actions={<Button size="small" variant="primary" icon="i-plus" iconSize={20}>{table.addLabel}</Button>}>
    <SettingsTable label="People" rows={rows.map((row: any) => ({...row, cells: {...row.cells, role: {...row.cells.role, tone: row.cells.role.tone === 'o' ? 'accent' : row.cells.role.tone}}, label: row.cells.person.title}))} columns={table.columns.map((column: any) => ({...column, sortable: column.sort}))} query={query} onQueryChange={setQuery} filters={table.filters.map((filter: any) => ({...filter, showAllValue: false}))}
      searchLabel={table.search} countLabel={`${rows.length} people`} emptyLabel={table.empty} height={table.height} matchedCount={rows.length} selected={selected} onSelectionChange={setSelected}
      rowAction={{label: table.rowAction.label, onAction() {}}} bulkActions={table.select.bulk.map((action: any, index: number) => ({id: String(index), label: action.label, onAction() {}}))} />
  </SettingsGroup>;
}
const settings: SettingsBindings = {
  accountName: 'Demo', serverName: 'Portico demo', role: 'owner', environment: {platform: 'web', os: 'browser', role: 'owner', signIn: 'local'},
  subscribe: listener => {changed.add(listener);return () => {changed.delete(listener);};}, getSnapshot: () => snapshot,
  sectionVisible: () => true, eligible: row => row.id !== 'libraries.episodeThumbs', audience: section => ['dashboard', 'libraries', 'people'].includes(section.id) ? 'administrators' : 'owner',
  serverHealth: () => 'normal', serverFacts: () => [{id: 'uptime', role: 'value', text: 'Up for 6 days'}, {id: 'free', role: 'value', text: '1.2 TB free'}, {id: 'database', role: 'value', text: 'Database 412 MB'}],
  rowState: row => {
    const original = originals.get(row.id), c = original?.control;
    const value = values.has(row.id) ? values.get(row.id) : c?.type === 'matrix' ? Object.fromEntries((c.items ?? []).map((item: any) => [item.id, item.default])) : c?.default;
    return {present: !!original, value, defaultValue: c?.default, changed: values.has(row.id), statusTone: c?.tone, displayValue: c?.text ?? (typeof value === 'string' ? value : undefined)};
  },
  change: (row, value) => {values.set(row.id, value);notify();}, reset: row => {values.delete(row.id);notify();},
  special: row => {const c = originals.get(row.id)?.control;return c?.type === 'action' || c?.type === 'link' ? <SettingsAction appearance={c.style}>{c.label}</SettingsAction> : null;},
  section: (context, _openPanel, renderRows) => context.section.id === 'people' ? <><PeopleTable />{renderRows({groups: context.section.groups?.slice(1), advanced: context.section.advanced})}</> : undefined,
  load: async () => {if (parameters.get('state') === 'failed' && ++loads > 1) {snapshot = Object.freeze({loading: false});notify();}},
  openHome: () => {void router.navigate({to: '/'});},
};
const viewer = {serverId: 'frame-server', authority: 'local', accountId: 'frame-account', profileId: 'frame-profile', role: 'owner'};
const api = {getRouteSnapshot: () => ({phase: 'ready'}), subscribeRoute: () => () => {}};
const events = {on: () => () => {}, onAny: () => () => {}, onResync: () => () => {}, getStatus: () => ({connected: true}), subscribeStatus: () => () => {}};
const binding = {api, events, scopeKey: 'immutable-settings-frame', settings, shell: {identity: {viewer, username: 'demo', profileName: 'Demo', profileArt: 'blue', serverName: 'Portico demo', signIn: 'local', capabilities: {manageAccount: true, managePeople: true, manageServer: true, manageSignIn: true}, primaryProfile: true, profiles: [], canManageProfiles: true, accountFacts: [],
  libraries: [{id: 'movies', name: 'Movies', kind: 'movie'}, {id: 'tv', name: 'TV Shows', kind: 'tv'}, {id: 'anime', name: 'Anime', kind: 'anime'}, {id: 'music', name: 'Music', kind: 'music'}, {id: 'audiobooks', name: 'Audiobooks', kind: 'audiobook'}], liveAvailable: true, unread: 3, showNotificationDot: true}, OwnerActivity, dockVisible: false, onSignOut() {}}, playback: undefined} as unknown as ShellBindings;
const page = parameters.get('section') ?? 'general', scope = parameters.get('scope') ?? 'server';
router.update({history: createMemoryHistory({initialEntries: [`/settings?scope=${scope}&section=${page}`]})});
Object.assign(window, {settingsFrame: {authority: '69f2d45a972ff9ec1bd8743c760183026c92d555', settings, originals, binding, router, settingsRowIndex}});
function PassphraseFixture({mode}: {mode: 'protect' | 'change' | 'restore'}) {
  const [open, setOpen] = useState(true);
  const proof = useRef({calls: 0, completed: 0, failed: 0, aborted: 0, resolve: (_result: 'done' | 'wrong-passphrase') => {}}).current;
  const authority = useRef({current: true, listeners: new Set<() => void>()}).current;
  Object.assign(window, {settingsPassphraseProof: proof});
  return <BackupPassphraseDialog open={open} mode={mode} scopeKey="controlled-passphrase" isCurrent={() => authority.current} subscribeAuthority={listener => {authority.listeners.add(listener);return () => {authority.listeners.delete(listener);};}} onClose={() => setOpen(false)} onDone={() => {proof.completed++;}} onFailure={() => {proof.failed++;}}
    submit={async (_passphrase, signal) => {proof.calls++;return new Promise(resolve => {proof.resolve = resolve;signal.addEventListener('abort', () => {proof.aborted++;}, {once: true});});}} />;
}
const dialog = parameters.get('dialog');
createRoot(document.getElementById('root')!).render(<><App binding={binding} />{(dialog === 'protect' || dialog === 'change' || dialog === 'restore') && <PassphraseFixture mode={dialog} />}</>);
