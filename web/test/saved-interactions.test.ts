/**
 * Behaviour QA (P2-BEHAVIOUR-QA): the drawn Saved controls do what they say.
 *
 * Saved is the bookmark in the top bar: six tabs, the list toolbar, the quick filters, the pager,
 * the history period chips and the Clear-history confirmation. These tests drive the real
 * component callbacks and the address the screen writes; the services are stubbed, so nothing
 * leaves the process. Empty, filtered-empty and failed states are covered too.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import * as savedCore from '@core/saved.ts';
import * as personalCore from '@core/personal-saved.ts';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, tree} from './helpers/interaction-harness.mjs';

// The screen listens for personal-change events on the window; Node has none.
(globalThis as any).window = {addEventListener: () => {}, removeEventListener: () => {}};

const calls: {name: string; args: any[]}[] = [];
const record = (name: string) => (...args: any[]) => { calls.push({name, args}); return Promise.resolve(true); };
const t = (key: any, values?: any) => defaultI18n.t(key, values);

const media = (id: string, title: string) => ({
  id, kind: 'movie', title, libraryId: 'library', posterUrl: `/${id}.jpg`, backdropUrl: '',
  duration: 6000, progressSeconds: 0, watched: false, playback: {itemId: id, startSeconds: 0},
});
const snapshot = (patch: any = {}) => ({
  generation: 1, scope: {viewerId: 'viewer', serverId: 'server'}, route: null, phase: 'ready', projection: null,
  playlist: null, error: null, pagination: {cursor: null, history: [], canPrevious: false, next: []}, pending: [],
  mutationError: null, lastReceipt: null, candidates: null, candidatesLoading: false, candidatesError: null,
  order: null, orderLoading: false, orderError: null, ...patch,
});
const projection = (sections: any[], extra: any = {}) => ({
  sections, sorts: [{id: 'added', labelKey: 'sort.added'}], filters: [], query: {sort: 'added', direction: 'desc'}, actions: [], ...extra,
});
const personalSnapshot = (patch: any = {}) => ({
  route: null, loading: false, error: null, entries: [], resources: [], history: [], nextCursor: '', viewerFence: 'fence',
  pagination: {canPrevious: false}, selected: null, ...patch,
});

async function mount(options: {search?: any; saved?: any; personal?: any} = {}) {
  calls.length = 0;
  const h = createRenderer();
  const navigated: any[] = [];
  const opened: any[] = [];
  const fixed = !!options.search;
  const search = {...(options.search ?? {view: 'watchlist'})};
  const savedSnapshot = options.saved ?? snapshot({projection: projection([{id: 'watchlist', entries: [media('m1', 'Dune')], nextCursor: ''}])});
  const personal = options.personal ?? personalSnapshot();
  const savedService = {
    getSnapshot: () => savedSnapshot,
    select: record('saved.select'), refresh: record('saved.refresh'), retry: record('saved.retry'),
    next: record('saved.next'), previous: record('saved.previous'), mutate: record('saved.mutate'),
  };
  const personalService = {
    getSnapshot: () => personal,
    refresh: record('personal.refresh'), select: record('personal.select'), retry: record('personal.retry'),
    next: record('personal.next'), previous: record('personal.previous'), mutate: record('personal.mutate'),
    revalidate: record('personal.revalidate'),
  };
  // The screen keeps one selection memo per service; start it on the address so no read happens.
  const savedMemo = {selected: JSON.stringify({view: search.view})};
  const personalMemo = {selected: ['history', 'collections', 'views'].includes(search.view) ? JSON.stringify({view: search.view}) : null};
  const ui = ['Artwork', 'Button', 'Card', 'Chip', 'Chips', 'ConfirmDialog', 'Dialog', 'Grid', 'Icon', 'Input', 'Inset', 'ListRow', 'Loading', 'Menu', 'Notice', 'Page', 'PageHeader', 'Row', 'StateView', 'Surface', 'Switch', 'Tabs', 'Text', 'TextArea', 'TitleHero']
    .reduce<Record<string, string>>((given, name) => ({...given, [name]: name}), {});
  const app = await componentModule(new URL('../src/screens/saved/Saved.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {useNavigate: () => (to: any) => { navigated.push(to); return Promise.resolve(); }, useSearch: () => search},
    '../../app/content': {cardCaptionOf: (value: any) => value.caption ?? ''},
    '../../app/selection': {useSelectionRefresh: () => {}},
    '../../app/open': {useEntryHref: () => (value: any) => `/item/${value.id}`, useOpenEntry: () => (...args: any[]) => opened.push(args)},
    '../../app/session': {sessionIdentity: () => 'identity', useSession: () => ({api: {}, session: {}})},
    '../../ui': {...ui, useCompact: () => false},
    '../shared/Sections': {LoadingGrid: 'LoadingGrid'},
    '../shared/GroupBrowse': {GroupBar: 'GroupBar', GroupGrid: 'GroupGrid'},
    '../../app/errors': {ErrorNotice: 'ErrorNotice', ErrorState: 'ErrorState', changedError: {code: 'changed', message: 'The library changed.', retryable: false}, errorText: (error: any) => String(error?.message ?? 'failed')},
    '../../app/viewer-scope': {useViewerScope: () => ({viewerId: 'viewer', serverId: 'server'})},
    '../../app/freshness': {
      usePooledService: () => ({service: savedService, snapshot: savedSnapshot, memo: savedMemo}),
      usePooled: () => ({service: personalService, snapshot: personal, memo: personalMemo}),
    },
    '../../app/i18n': {useI18n: () => defaultI18n},
    '../../app/group-browse': {acrossGroup: () => ({id: 'watchlist'}), useGroupBrowse: () => ({pending: false, active: false})},
    '../../app/quiet-refresh': {useRefreshRequiredRetry: () => false},
    '../../app/pins': {usePinnedResources: () => ({order: []})},
    '../../shell/PinEditor': {pinnedResourceIcon: () => 'bookmark'},
    '@core/saved.ts': {...savedCore, SavedService: class { constructor() { Object.assign(this, savedService); } }},
    '@core/personal-saved.ts': {...personalCore, PersonalSavedService: class { constructor() { Object.assign(this, personalService); } }},
  });
  const draw = () => {
    // Land the address the screen itself wrote, as the router would (unless the test pins one).
    const last = navigated.filter(to => to?.to === '/saved').at(-1);
    if (last && !fixed) Object.assign(search, last.search);
    return app.SavedScreen();
  };
  h.render(draw);
  const flat = () => tree(h.render(draw));
  return {h, flat, navigated, opened, search};
}

const tabKey = (id: string) => `saved.tab.${({watchlist: 'watchlist', favorites: 'favorites', playlists: 'playlists', collections: 'collections', views: 'savedViews', history: 'history'} as Record<string, string>)[id]}`;

test('the six tabs are drawn and choosing one writes the address', async () => {
  const screen = await mount();
  const tabs = one(screen.flat(), 'Tabs');
  assert.deepEqual(tabs.props.items.map((item: any) => item.label), ['watchlist', 'favorites', 'playlists', 'collections', 'views', 'history'].map(tabKey).map(key => t(key)));
  assert.equal(tabs.props.value, 'watchlist');
  assert.equal(tabs.props.label, t('web.saved.sections'));
  tabs.props.onChange('favorites');
  assert.deepEqual(screen.navigated.at(-1), {to: '/saved', search: {view: 'favorites', sort: undefined, direction: undefined, filter: undefined, period: undefined, library: undefined, filters: undefined}, replace: true});
  // The screen follows the address, so the tab is pressed after the router lands.
  assert.equal(one(screen.flat(), 'Tabs').props.value, 'favorites');
});

test('the refresh control asks the saved list for a fresh read', async () => {
  const screen = await mount();
  const refresh = all(screen.flat(), 'Button').find(button => button.props['aria-label'] === t('action.refresh'))!;
  assert.ok(refresh, 'a refresh control in the page header');
  refresh.props.onClick();
  assert.deepEqual(calls.map(call => call.name), ['saved.refresh']);
});

test('a saved title opens, plays and offers its menu', async () => {
  const screen = await mount();
  const card = all(screen.flat(), 'Card')[0];
  assert.equal(card.props.title, 'Dune');
  card.props.onOpen();
  card.props.onPlay();
  card.props.onMore({x: 1, y: 2, width: 3, height: 4});
  assert.deepEqual(screen.opened.map(args => [args[0]?.id, args[1]]), [['m1', undefined], ['m1', 'play'], ['m1', 'more']]);
});

test('an empty list says what belongs there', async () => {
  const screen = await mount({saved: snapshot({projection: projection([{id: 'watchlist', entries: [], nextCursor: ''}])})});
  const state = one(screen.flat(), 'StateView');
  assert.equal(state.props.title, t('web.saved.empty.watchlist'));
  assert.equal(state.props.body, t('web.saved.empty.watchlistBody'));
  assert.equal(state.props.action, undefined);
});

test('a playlist filter that matches nothing says so and offers Show all', async () => {
  const screen = await mount({search: {view: 'playlists', filter: 'mine'}, saved: snapshot({projection: projection([])})});
  const state = one(screen.flat(), 'StateView');
  assert.equal(state.props.title, t('playlists.filter.none'));
  assert.equal(state.props.action.label, t('web.saved.showAll'));
  state.props.action.onClick();
  assert.equal(screen.navigated.at(-1).search.filter, undefined);
});

test('a failed list offers Try again, and the pager turns its pages', async () => {
  const broken = await mount({saved: snapshot({phase: 'error', error: {code: 'request_failed', message: 'Saved could not load.', retryable: true}})});
  one(broken.flat(), 'ErrorState').props.retry();
  assert.deepEqual(calls.map(call => call.name), ['saved.retry']);
  // Two pages: Previous is live only when there is one, and Next asks for the section's cursor.
  const paged = await mount({saved: snapshot({projection: projection([{id: 'watchlist', entries: [media('m1', 'Dune')], nextCursor: 'cursor-2'}]), pagination: {cursor: null, history: [null], canPrevious: true, next: []}})});
  const rendered = paged.flat();
  const next = all(rendered, 'Button').find(button => button.props.label === t('web.search.next'))!;
  const previous = all(rendered, 'Button').find(button => button.props.label === t('web.live.previous'))!;
  assert.equal(next.props.disabled, false);
  assert.equal(previous.props.disabled, false);
  next.props.onClick();
  previous.props.onClick();
  assert.deepEqual(calls.map(call => [call.name, ...call.args]), [['saved.next', 'watchlist'], ['saved.previous']]);
});

test('history has its period chips and a Clear that asks first', async () => {
  const entries = [{id: 'h1', media: media('m1', 'Dune'), updatedAt: '2026-10-24T10:00:00Z', positionSeconds: 0, completed: false}];
  const screen = await mount({search: {view: 'history'}, personal: personalSnapshot({route: {view: 'history'}, entries})});
  let rendered = screen.flat();
  const periods = all(rendered, 'Chip').filter(chip => chip.props.pressed !== undefined);
  assert.deepEqual(periods.map(chip => chip.props.label), [t('web.live.today'), t('web.saved.thisWeek'), t('web.saved.days', {count: 30}), t('web.saved.days', {count: 90}), t('web.saved.allTime')]);
  assert.equal(periods.find(chip => chip.props.pressed)!.props.label, t('web.saved.allTime'));
  periods[0].props.onClick();
  assert.deepEqual(screen.navigated.at(-1), {to: '/saved', search: {view: 'history', period: '24h'}, replace: true});
  // Clear history is behind a confirmation, and confirming asks the service once.
  rendered = screen.flat();
  all(rendered, 'Button').find(button => button.props.label === t('web.saved.clearHistory'))!.props.onClick();
  const dialog = one(screen.flat(), 'ConfirmDialog');
  assert.equal(dialog.props.open, true);
  assert.equal(dialog.props.title, t('web.saved.clearHistoryTitle'));
  await dialog.props.onConfirm();
  assert.deepEqual(calls.map(call => [call.name, ...call.args]), [['personal.mutate', {action: 'clear-history'}], ['personal.refresh']]);
});

test('a new playlist and a new collection open their dialogs', async () => {
  const playlists = await mount({search: {view: 'playlists'}, saved: snapshot({projection: projection([], {actions: ['create_playlist']})})});
  all(playlists.flat(), 'Button').find(button => button.props.label === t('web.saved.newPlaylist'))!.props.onClick();
  assert.ok(all(playlists.flat(), 'Dialog').some(dialog => dialog.props.open === true), 'the new-playlist dialog opens');
  // A collection needs a name before Create is offered.
  const collections = await mount({search: {view: 'collections'}, personal: personalSnapshot({route: {view: 'collections'}})});
  all(collections.flat(), 'Button').find(button => button.props.label === t('web.saved.newCollection'))!.props.onClick();
  let dialog = all(collections.flat(), 'Dialog').find(node => node.props.open === true)!;
  assert.ok(dialog, 'the new-collection dialog opens');
  const name = all(tree(dialog), 'Input').find(node => node.props.label === t('profile.name'))!;
  name.props.onChange({target: {value: 'Rewatch'}});
  dialog = all(collections.flat(), 'Dialog').find(node => node.props.open === true)!;
  const create = all(tree(dialog), 'Button').find(node => node.props.label === t('web.saved.create'))!;
  assert.equal(create.props.disabled, false);
  create.props.onClick();
  assert.deepEqual(calls.map(call => [call.name, ...call.args]), [['personal.mutate', {action: 'create', kind: 'collection', name: 'Rewatch', summary: '', visibility: 'private'}]]);
});
