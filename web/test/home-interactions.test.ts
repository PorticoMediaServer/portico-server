/**
 * Behaviour QA (P2-BEHAVIOUR-QA): the drawn Home controls do what they say.
 *
 * Home is core flow 2: the hero and its Play/Details, the rows the server composed, each row's
 * More and See all, Continue Watching removal with Undo, Customise, and the loading, empty and
 * failed states. The tests drive the real component callbacks; the home service and router are
 * stubbed so nothing leaves the process.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, textOf, tree} from './helpers/interaction-harness.mjs';

const t = (key: any, values?: any) => defaultI18n.t(key, values);
const REMOVED = 'portico:continue-removed';
const events = new EventTarget();
// Each mount adds one listener and the file mounts many screens; hosts without the guard just warn.
if (typeof (events as any).setMaxListeners === 'function') (events as any).setMaxListeners(50);
(globalThis as any).window = {addEventListener: (...args: any[]) => events.addEventListener(...args as [string, any]), removeEventListener: (...args: any[]) => events.removeEventListener(...args as [string, any])};

/** Every renderer a test mounted, so its effects' cleanups can run before the test ends. */
const mounted: Array<{dispose(): void}> = [];
/**
 * Let go of every screen this test mounted, newest first: no listener or deadline outlives it.
 * One screen failing to unmount never strands the others; the first failure is raised afterwards.
 * Tests register this with `t.after` before the first mount, so a mount that throws is still owned.
 */
function disposeScreens() {
  let failure;
  let failed = false;
  for (const h of mounted.splice(0).reverse()) {
    try { h.dispose(); } catch (error) { if (!failed) { failed = true; failure = error; } }
  }
  if (failed) throw failure;
}

const entry = (id: string, title: string, extra: any = {}) => ({
  id, kind: 'movie', title, subtitle: '2026', year: 2026, libraryId: 'library', posterUrl: `/${id}.jpg`,
  backdropUrl: '', overview: '', duration: 6000, progressSeconds: 600, watched: false,
  playback: {itemId: id, startSeconds: 600}, ...extra,
});
const row = (id: string, entries: any[], extra: any = {}) => ({id, kind: 'recent', libraryId: 'library', title: id, titleText: id, entries, total: entries.length, nextCursor: '', ...extra});

async function mount(options: {state?: any; libraries?: any; owner?: boolean; hidden?: Set<string>} = {}) {
  const h = createRenderer();
  mounted.push(h);
  const opened: any[] = [];
  const navigated: any[] = [];
  const calls: string[] = [];
  const state = options.state ?? {phase: 'ready', document: {rows: [row('continue', [entry('m1', 'Dune')])], hero: {rowId: 'continue', entryId: 'm1'}}, error: null, paging: new Set<string>()};
  const app = await componentModule(new URL('../src/screens/home/Home.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {useNavigate: () => (to: any) => { navigated.push(to); return Promise.resolve(); }},
    '../../app/home': {useHome: () => ({state, refresh: () => calls.push('refresh'), more: (id: string) => calls.push(`more:${id}`), saveLayout: () => calls.push('save'), resetLayout: () => calls.push('reset'), loadLayout: () => calls.push('load')}), homeRowTitle: (_id: string, title: string) => title},
    '../../app/open': {useOpenEntry: () => (...args: any[]) => opened.push(args)},
    '../../app/selection': {useSelectionRefresh: () => {}},
    '../../app/session': {useSession: () => ({system: {name: 'Demo'}, owner: !!options.owner})},
    '../../ui': {Artwork: 'Artwork', Backdrop: 'Backdrop', Button: 'Button', Inset: 'Inset', Notice: 'Notice', Page: 'Page', PageHeader: 'PageHeader', ProgressBar: 'ProgressBar', StateView: 'StateView', cx: (...names: any[]) => names.filter(Boolean).join(' ')},
    '../shared/Sections': {LoadingShelves: 'LoadingShelves', SectionView: 'SectionView'},
    './Customise': {CustomiseHome: 'CustomiseHome'},
    '../shared/NoLibraries': {NoLibraries: 'NoLibraries'},
    '../../app/libraries': {useLibrariesContext: () => options.libraries ?? {items: [{id: 'library', name: 'Movies'}], loading: false}},
    './Home.module.css': {default: {}},
    '../../app/errors': {ErrorNotice: 'ErrorNotice', ErrorState: 'ErrorState', changedError: {}},
    '../../app/continue-watching': {CONTINUE_REMOVED: REMOVED},
    '../../app/not-interested-notice': {useNotInterested: () => ({hidden: options.hidden ?? new Set()})},
    '../../app/not-interested': {withoutHidden: (r: any, hidden: Set<string>) => ({...r, entries: r.entries.filter((e: any) => !hidden.has(e.id))})},
    '../../app/i18n': {useI18n: () => defaultI18n},
    '../../app/browse': {encodePredicates: () => 'filters'},
    '../../app/see-all': {homeRowSeeAll: (r: any) => (r.seeAll ? (r.seeAll.type === 'saved_view' ? {to: 'saved_view', resourceId: r.seeAll.resourceId} : {to: 'row', rowId: r.seeAll.rowId}) : r.libraryId ? {to: 'browse', libraryId: r.libraryId, tab: 'browse', sort: undefined, filters: []} : undefined)},
  });
  h.render(() => app.HomeScreen());
  const flat = () => tree(h.render(() => app.HomeScreen()));
  return {h, flat, opened, navigated, calls};
}

test('the hero is drawn with Play and Details that open the title', async ctx => {
  ctx.after(disposeScreens);
  const screen = await mount();
  const hero = one(screen.flat(), 'section');
  assert.equal(hero.props['aria-label'], 'Dune');
  assert.ok(textOf(hero).includes('Dune'));
  const buttons = all(tree(hero), 'Button');
  const play = buttons.find(button => button.props.icon === 'play');
  assert.ok(play, 'a resume button');
  const details = buttons.find(button => button.props.label === t('web.home.details'))!;
  play.props.onClick();
  details.props.onClick();
  assert.deepEqual(screen.opened.map(args => [args[0]?.id, args[1]]), [['m1', 'play'], ['m1', undefined]]);
});

test('every row is drawn with its heading, and More asks for its next page', async ctx => {
  ctx.after(disposeScreens);
  const pages = new Set<string>();
  const document = {rows: [row('continue', [entry('m1', 'Dune')], {nextCursor: 'cursor-2'}), row('recent', [entry('m2', 'Arrival')], {libraryId: 'library'})], hero: undefined};
  const screen = await mount({state: {phase: 'ready', document, error: null, paging: pages}});
  const rows = all(screen.flat(), 'SectionView');
  assert.deepEqual(rows.map(node => node.props.section.heading.fallback), ['continue', 'recent']);
  // More is offered only for a row the server can page, and asking for it names that row.
  assert.equal(typeof rows[0].props.onMore, 'function');
  assert.equal(rows[1].props.onMore, undefined);
  rows[0].props.onMore();
  assert.deepEqual(screen.calls, ['more:continue']);
  // A row already paging hides More until its page lands.
  pages.add('continue');
  assert.equal(all(screen.flat(), 'SectionView')[0].props.onMore, undefined);
});

test('See all follows the row’s destination, and a saved view opens its own page', async ctx => {
  ctx.after(disposeScreens);
  const document = {rows: [row('continue', [entry('m1', 'Dune')]), row('custom', [entry('m2', 'Arrival')], {seeAll: {type: 'saved_view', resourceId: 'view-1'}, title: 'My picks'})], hero: undefined};
  const screen = await mount({state: {phase: 'ready', document, error: null, paging: new Set()}});
  const rows = all(screen.flat(), 'SectionView');
  rows[0].props.onSeeAll();
  rows[1].props.onSeeAll();
  assert.deepEqual(screen.navigated, [
    {to: '/library/$libraryId', params: {libraryId: 'library'}, search: {view: 'browse'}},
    {to: '/saved/$kind/$resourceId', params: {kind: 'view', resourceId: 'view-1'}, search: {title: 'My picks'}},
  ]);
});

test('a title removed from Continue Watching leaves at once, and Undo puts it back', async ctx => {
  ctx.after(disposeScreens);
  const screen = await mount();
  let undone = 0;
  events.dispatchEvent(new CustomEvent(REMOVED, {detail: {itemId: 'm1', title: 'Dune', phase: 'done', undo: () => { undone++; return Promise.resolve(); }}}));
  let rendered = screen.flat();
  assert.equal(all(rendered, 'SectionView').length, 0, 'the emptied Continue row and its hero are gone');
  const notice = one(rendered, 'Notice');
  assert.equal(notice.props.action.label, t('web.continue.undo'));
  await notice.props.action.onClick();
  await new Promise(resolve => setTimeout(resolve, 0));
  rendered = screen.flat();
  assert.equal(undone, 1);
  assert.ok(screen.calls.includes('refresh'));
  assert.equal(all(rendered, 'SectionView').length, 1, 'Undo brings the row back');
});

test('a failed removal says so and keeps the title', async ctx => {
  ctx.after(disposeScreens);
  const screen = await mount();
  events.dispatchEvent(new CustomEvent(REMOVED, {detail: {itemId: 'm1', title: 'Dune', phase: 'failed', undo: () => Promise.resolve()}}));
  const rendered = screen.flat();
  assert.equal(all(rendered, 'SectionView').length, 1, 'the title stays');
  assert.equal(one(rendered, 'Notice').props.tone, 'error');
  assert.equal(one(rendered, 'Notice').props.action, undefined);
});

test('customise opens from above the rows and hands the layout to the screen', async ctx => {
  ctx.after(disposeScreens);
  const screen = await mount();
  const customize = all(screen.flat(), 'Button').find(button => button.props.label === t('home.customize.action'))!;
  customize.props.onClick();
  const sheet = one(screen.flat(), 'CustomiseHome');
  assert.equal(sheet.props.open, true);
  sheet.props.onClose();
  assert.equal(one(screen.flat(), 'CustomiseHome').props.open, false);
  sheet.props.onSave([{id: 'continue'}]);
  sheet.props.onReset();
  sheet.props.loadLayout();
  assert.deepEqual(screen.calls, ['save', 'reset', 'load']);
});

test('the empty states are never a dead end: no libraries, nothing composed, and a failed read', async ctx => {
  ctx.after(disposeScreens);
  // Everything composed but empty: the catalogue's own words, for a member and for the owner.
  let screen = await mount({state: {phase: 'ready', document: {rows: [], hero: undefined}, error: null, paging: new Set()}});
  assert.equal(one(screen.flat(), 'StateView').props.title, t('web.empty.homeTitle'));
  assert.equal(one(screen.flat(), 'StateView').props.body, t('web.empty.homeMemberBody'));
  screen = await mount({owner: true, state: {phase: 'ready', document: {rows: [], hero: undefined}, error: null, paging: new Set()}});
  assert.equal(one(screen.flat(), 'StateView').props.body, t('web.empty.homeOwnerBody'));
  // No libraries at all is the sharper message.
  screen = await mount({libraries: {items: [], loading: false}, state: {phase: 'ready', document: {rows: [], hero: undefined}, error: null, paging: new Set()}});
  assert.ok(screen.flat().some(node => node.type === 'NoLibraries'));
  // A failed first read offers retry; a failed refresh beside content keeps the content.
  screen = await mount({state: {phase: 'error', document: undefined, error: {code: 'request_failed', message: 'Home could not load.', retryable: true}, paging: new Set()}});
  one(screen.flat(), 'ErrorState').props.retry();
  assert.deepEqual(screen.calls, ['refresh']);
  screen = await mount({state: {phase: 'error', document: {rows: [row('continue', [entry('m1', 'Dune')])], hero: undefined}, error: {code: 'request_failed', message: 'Home could not refresh.', retryable: true}, paging: new Set()}});
  assert.equal(all(screen.flat(), 'SectionView').length, 1);
  one(screen.flat(), 'ErrorNotice');
});

test('deferred shelves are loading, not empty', async ctx => {
  ctx.after(disposeScreens);
  const screen = await mount({state: {phase: 'loading', document: {rows: [{...row('recent', [], {deferred: true})}], hero: undefined}, error: null, paging: new Set()}});
  const rendered = screen.flat();
  assert.ok(rendered.some(node => node.type === 'LoadingShelves'));
  assert.equal(all(rendered, 'StateView').length, 0);
});
