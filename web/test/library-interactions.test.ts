/**
 * Behaviour QA (P2-BEHAVIOUR-QA): the drawn Library controls do what they say.
 *
 * A library in the merged web app: the authored tab strip and the address it writes, the grid or
 * list choice, the card-size control, the owner's scan action, each Discover rail's See all, the
 * full-row page and its Back, and the empty, scanning and failed states. The browse engine and the
 * content service are stubbed, so these tests exercise the screen's own wiring.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {libraryTabs} from '@core/presentation/index.ts';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, textOf, tree} from './helpers/interaction-harness.mjs';

const t = (key: any, values?: any) => defaultI18n.t(key, values);

const field = (id: string) => ({id, labelKey: `browse.field.${id}`, type: 'enum', operators: ['in'], controlHint: 'facet-multi-select', complexity: 'simple', cost: 'cheap', applicableKinds: ['movie']});
const capabilities = {
  library: {id: 'library', name: 'Movies', kind: 'movie', defaultView: 'browse', pinned: false},
  pivots: [],
  resolvedPivot: {id: 'movies', labelKey: 'browse.pivot.movies', entityKinds: ['movie'], defaultSort: [{field: 'title', direction: 'asc'}], supportedViews: ['browse'], browsable: true},
  fields: [field('genre')],
  sorts: [
    {id: 'title', labelKey: 'sort.title', directions: ['asc', 'desc'], defaultDirection: 'asc', expensive: false, applicableKinds: ['movie']},
    {id: 'added', labelKey: 'sort.added', directions: ['asc', 'desc'], defaultDirection: 'desc', expensive: false, applicableKinds: ['movie']},
  ],
  quickFilters: [],
  queryLimits: {maximumDepth: 3, maximumClauses: 20, maximumBytes: 8192, maximumSorts: 2, defaultLimit: 60, maximumLimit: 120, cursorTtlMs: 60000},
};
const entry = (id: string, title: string) => ({id, kind: 'movie', title, libraryId: 'library', posterUrl: `/${id}.jpg`, backdropUrl: '', duration: 6000, progressSeconds: 0, watched: false, playback: {itemId: id, startSeconds: 0}});
const section = (id: string, entries: any[], extra: any = {}) => ({id, type: 'rail', heading: {key: id, fallback: id}, entries, totalCount: entries.length, nextCursor: '', ...extra});
const page = (patch: any = {}) => {
  const sections = patch.sections ?? [section('recent', [entry('m1', 'Dune')])];
  // Discover draws the pages it has read (`appended`), which in the app holds `sections` too.
  return {phase: 'ready', appended: sections, ...patch, sections};
};

async function mount(options: {search?: any; owner?: boolean; libraries?: any[]; content?: any; scan?: any; capabilities?: any} = {}) {
  const h = createRenderer();
  const navigated: any[] = [];
  const opened: any[] = [];
  const memory = {recall: () => undefined, rememberTab: (libraryId: string, tab: string) => {memory.calls.push(['rememberTab', libraryId, tab]);}, rememberSort: (...args: any[]) => {memory.calls.push(['rememberSort', ...args]);}, rememberFilters: (...args: any[]) => {memory.calls.push(['rememberFilters', ...args]);}, calls: [] as any[]};
  const prefs = {preferences: {libraryViews: {}, libraryView: 'grid', posterSize: 'regular'}, update: (patch: any) => {prefs.updates.push(patch);}, updates: [] as any[]};
  const serverPrefs = {set: (patch: any) => {serverPrefs.sets.push(patch); return Promise.resolve();}, sets: [] as any[]};
  const requests: any[] = [];
  const api = {request: (path: string, method: string) => {requests.push([path, method]); return Promise.resolve({});}};
  const search = {...(options.search ?? {view: 'discover'})};
  const content = options.content ?? page();
  const browseRefresh: string[] = [];
  const app = await componentModule(new URL('../src/screens/library/Library.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {useNavigate: () => (to: any) => { navigated.push(to); return Promise.resolve(); }, useParams: () => ({libraryId: 'library'}), useSearch: () => search},
    '../../app/content': {
      useContent: () => ({snapshot: content, retry: () => { contentRetries.push('retry'); }, refresh: () => { contentRetries.push('refresh'); }, next: () => {}}),
      browseCountLabel: (total: number) => `${total} titles`,
      seeAllTarget: () => ({view: 'browse', sort: 'added', direction: 'desc'}),
      cardCaptionOf: () => '',
    },
    '../../app/selection': {useSelectionRefresh: () => {}, useSelectionQueryTarget: () => {}},
    '../../app/server-preferences': {useServerPreferences: () => serverPrefs},
    '../../app/browse': {
      BROWSE_PAGE: 60,
      decodePredicates: (raw?: string) => (raw ? JSON.parse(raw) : []),
      encodePredicates: (next: readonly any[]) => JSON.stringify(next),
      seeAllPredicates: () => [],
      useBrowseCapabilities: () => ({capabilities: options.capabilities ?? capabilities, error: undefined}),
      useBrowseCollection: () => ({collection: undefined, refresh: () => { browseRefresh.push('refresh'); }}),
      useShuffled: (value: any) => value,
    },
    '../../app/list-position': {listWindowKey: () => 'key'},
    './BrowseFilters': {BrowseBar: 'BrowseBar'},
    './EngineResults': {EngineResults: 'EngineResults'},
    './LibraryViews': {GenreCard: 'GenreCard', LibraryPlaylists: 'LibraryPlaylists'},
    './SaveView': {SaveViewDialog: 'SaveViewDialog'},
    '../shared/NoLibraries': {useEmptyLibraryCopy: () => ({body: 'Nothing in this library yet.', action: {label: 'Scan', onClick: () => {}}})},
    './BrowseFilters.module.css': {default: {}},
    '../../app/container-play': {viewSequence: () => 'sequence'},
    '../../app/libraries': {useLibrariesContext: () => ({items: options.libraries ?? [{id: 'library', name: 'Movies', kind: 'movie'}]}), libraryKindLabel: (kind: string) => kind},
    '../../app/session': {useSession: () => ({api, session: {}, owner: !!options.owner})},
    '../../app/viewer-scope': {useViewerScope: () => ({viewerId: 'viewer', serverId: 'server'})},
    '../../app/library-scan': {useLibraryScan: () => options.scan ?? {scanning: false, found: 0, finished: 0}},
    '../../app/preferences': {usePreferences: () => prefs},
    '../../app/open': {useOpenEntry: () => (...args: any[]) => opened.push(args)},
    '../shared/Sections': {LoadingGrid: 'LoadingGrid', LoadingShelves: 'LoadingShelves', SectionView: 'SectionView', sectionHeading: (_i18n: any, heading: any) => heading.fallback ?? heading.key},
    '../../app/library-memory': {useLibraryMemory: () => memory},
    '../../app/errors': {ErrorNotice: 'ErrorNotice', ErrorState: 'ErrorState', changedError: {}, errorText: () => 'failed'},
    '../../app/not-interested-notice': {useNotInterested: () => ({hidden: new Set()})},
    '../../app/not-interested': {withoutHidden: (raw: any) => raw},
    '../../app/i18n': {useI18n: () => defaultI18n},
    '../../app/quiet-refresh': {useRefreshRequiredRetry: () => false},
    '../../app/group-browse': {acrossGroup: () => undefined, useGroupBrowse: () => ({pending: false, active: false})},
    '../../ui': {Badge: 'Badge', Button: 'Button', Chip: 'Chip', Inset: 'Inset', Menu: 'Menu', Notice: 'Notice', Page: 'Page', PageHeader: 'PageHeader', Row: 'Row', Segmented: 'Segmented', StateView: 'StateView', Tabs: 'Tabs'},
  });
  const contentRetries: string[] = [];
  const draw = () => app.LibraryScreen();
  h.render(draw);
  const flat = () => tree(h.render(draw));
  return {h, flat, navigated, opened, memory, prefs, serverPrefs, requests, search, contentRetries, setSearch: (patch: any) => Object.assign(search, patch)};
}

test('the authored tabs are drawn and choosing one writes the address and the memory', async () => {
  const screen = await mount();
  const tabs = one(screen.flat(), 'Tabs');
  assert.deepEqual(tabs.props.items.map((item: any) => item.id), libraryTabs('movie', {unmatched: true}).map(tab => tab.id));
  assert.deepEqual(tabs.props.items.map((item: any) => item.label), libraryTabs('movie', {unmatched: true}).map(tab => t(tab.label)));
  assert.equal(tabs.props.value, 'discover');
  assert.equal(tabs.props.label, t('web.library.viewsLabel'));
  tabs.props.onChange('browse');
  assert.deepEqual(screen.memory.calls.at(-1), ['rememberTab', 'library', 'browse']);
  assert.equal(screen.navigated.at(-1).search.view, 'browse');
  assert.equal(screen.navigated.at(-1).search.category, undefined);
  assert.equal(screen.navigated.at(-1).search.row, undefined);
  assert.equal(screen.navigated.at(-1).replace, true);
  // The screen follows the address, so the tab is pressed after the router lands.
  screen.setSearch({view: 'browse'});
  assert.equal(one(screen.flat(), 'Tabs').props.value, 'browse');
});

test('on a titles tab, grid or list is written to this device’s library preference', async () => {
  const screen = await mount({search: {view: 'browse'}});
  const choice = one(screen.flat(), 'Segmented');
  assert.equal(choice.props.value, 'grid');
  assert.deepEqual(choice.props.options.map((option: any) => option.id), ['grid', 'list']);
  choice.props.onChange('list');
  assert.deepEqual(screen.prefs.updates.at(-1), {libraryViews: {library: 'list'}});
});

test('the card-size control sets this device and the profile together', async () => {
  const screen = await mount({search: {view: 'browse'}});
  const cardSize = all(screen.flat(), 'Menu').find(node => node.props.label === t('settings.row.cardSize'))!;
  assert.deepEqual(cardSize.props.items.map((item: any) => item.id), ['small', 'medium', 'large']);
  assert.equal(cardSize.props.items.find((item: any) => item.selected).id, 'medium');
  cardSize.props.onSelect('large');
  assert.deepEqual(screen.prefs.updates.at(-1), {posterSize: 'large'});
  assert.equal(screen.serverPrefs.sets.length, 1);
  assert.equal(typeof screen.serverPrefs.sets[0]['appearance.cardSizePercent'], 'number');
});

test('the owner menu scans the library and lands a notice, and Manage opens its settings', async () => {
  const screen = await mount({owner: true});
  let menu = all(screen.flat(), 'Menu').find(node => node.props.label === t('title.moreActions'))!;
  menu.props.onSelect('scan');
  await new Promise(resolve => setTimeout(resolve, 0));
  assert.deepEqual(screen.requests, [['/v1/libraries/library/scans', 'POST']]);
  assert.equal(one(screen.flat(), 'Notice').props.tone, 'success');
  assert.equal(textOf(one(screen.flat(), 'Notice')), t('web.library.scanStarted'));
  menu = all(screen.flat(), 'Menu').find(node => node.props.label === t('title.moreActions'))!;
  menu.props.onSelect('manage');
  assert.deepEqual(screen.navigated.at(-1), {to: '/settings/$section', params: {section: 'server-libraries'}, search: {id: 'library'}});
});

test('a Discover rail’s See all lands on a clean, linkable destination', async () => {
  const many = section('recent', [entry('m1', 'Dune')], {totalCount: 40});
  const screen = await mount({content: page({sections: [many]})});
  const rail = one(screen.flat(), 'SectionView');
  assert.equal(typeof rail.props.onSeeAll, 'function');
  rail.props.onSeeAll();
  assert.deepEqual(screen.navigated.at(-1), {to: '/library/$libraryId', params: {libraryId: 'library'}, search: {view: 'browse', sort: 'added', direction: 'desc'}});
  // A rail that fits entirely offers no See all.
  const short = await mount({content: page({sections: [section('recent', [entry('m1', 'Dune')])]})});
  assert.equal(one(short.flat(), 'SectionView').props.onSeeAll, undefined);
});

test('an empty library says what it is, and a running scan says so inside the empty state', async () => {
  let screen = await mount({content: page({sections: []})});
  assert.equal(one(screen.flat(), 'StateView').props.title, t('library.emptyLibrary'));
  assert.equal(one(screen.flat(), 'StateView').props.body, 'Nothing in this library yet.');
  screen = await mount({content: page({sections: []}), scan: {scanning: true, found: 7, finished: 0}});
  const state = one(screen.flat(), 'StateView');
  assert.equal(state.props.title, t('web.library.scanningTitle'));
  assert.equal(state.props.action, undefined);
});

test('a failed library offers its recovery, and a long read shows the shelf skeleton', async () => {
  let screen = await mount({content: page({phase: 'error', sections: [], error: {code: 'request_failed', message: 'Library could not load.', retryable: true}})});
  one(screen.flat(), 'ErrorState').props.retry();
  assert.deepEqual(screen.contentRetries, ['retry']);
  screen = await mount({content: page({phase: 'loading', sections: []})});
  assert.ok(screen.flat().some(node => node.type === 'LoadingShelves'));
});

test('a Discover row opened as a page has its own heading and a Back to the library', async () => {
  const row = section('recent', [entry('m1', 'Dune')]);
  const screen = await mount({search: {view: 'discover', row: 'recent'}, content: page({sections: [row]})});
  const rendered = screen.flat();
  // The library's own header stays above the row page's.
  const headers = all(rendered, 'PageHeader');
  assert.equal(headers.length, 2);
  const header = headers.at(-1);
  assert.equal(header.props.title, 'recent');
  header.props.onBack();
  assert.equal(screen.navigated.at(-1).search.row, undefined);
  assert.equal(one(rendered, 'SectionView').props.hideTitle, true);
});
