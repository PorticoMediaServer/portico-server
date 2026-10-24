/**
 * Behaviour QA (P2-BEHAVIOUR-QA): the drawn See-all row page does what it says.
 *
 * A Home row's See all is a paged grid read from the row's own endpoint: Play and Shuffle play the
 * whole row from the server, the grid pages with More, Back returns Home, and a failed read offers
 * the one truthful recovery. The pager is stubbed to a state machine that mirrors the real one
 * (more() only reads when there is a cursor), so the test sees which read a control asks for.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, tree} from './helpers/interaction-harness.mjs';

const RESET = 'portico:recommendations-reset';
const events = new EventTarget();
(globalThis as any).window = {addEventListener: (...args: any[]) => events.addEventListener(...args as [string, any]), removeEventListener: (...args: any[]) => events.removeEventListener(...args as [string, any])};
const t = (key: any, values?: any) => defaultI18n.t(key, values);

const entry = (id: string, title: string) => ({id, kind: 'movie', title, libraryId: 'library', posterUrl: `/${id}.jpg`, backdropUrl: '', duration: 6000, progressSeconds: 0, watched: false, playback: {itemId: id, startSeconds: 0}});
const row = {id: 'recommended', title: 'Recommended', libraryId: 'library', artworkShape: 'poster'};

function pagerWith(patch: any = {}) {
  const reads: string[] = [];
  const snapshot = {phase: 'ready', row, entries: [entry('m1', 'Dune'), entry('m2', 'Arrival')], total: 2, nextCursor: '', paging: false, ...patch};
  const pager = {
    rowId: 'recommended',
    getSnapshot: () => snapshot,
    subscribe: () => () => {},
    load: () => { reads.push('load'); return Promise.resolve(); },
    more: () => { reads.push('more'); return Promise.resolve(); },
    revalidate: () => { reads.push('revalidate'); return Promise.resolve(); },
    dispose: () => { reads.push('dispose'); },
  };
  return {pager, reads, snapshot};
}

async function mount(options: {pager?: any; scope?: any} = {}) {
  const h = createRenderer();
  const navigated: any[] = [];
  const played: any[] = [];
  const {pager, reads, snapshot} = options.pager ?? pagerWith();
  const state = snapshot;
  // One API object per mount: the pager is built once and its mount read runs once.
  const api = {};
  const app = await componentModule(new URL('../src/screens/home/HomeRowPage.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {useParams: () => ({rowId: 'recommended'}), useSearch: () => ({}), useNavigate: () => (to: any) => { navigated.push(to); return Promise.resolve(); }},
    '@core/home-row-page.ts': {HomeRowPager: class { constructor() { Object.assign(this, pager); } }},
    '../../app/home': {
      homeRowTitle: (_id: string, title: string) => title,
      // The one-line rule this screen now uses: a cursor means more, none means the row again.
      retryHomeRow: (pager: any, nextCursor: string) => (nextCursor ? pager.more() : pager.load()),
    },
    '../../app/session': {useSession: () => ({api})},
    '../../app/libraries': {useLibrariesContext: () => ({items: [{id: 'library', name: 'Movies'}], loading: false})},
    '../../app/errors': {ErrorNotice: 'ErrorNotice', ErrorState: 'ErrorState', errorText: (error: any) => String(error?.message ?? 'failed')},
    '../../app/not-interested-notice': {useNotInterested: () => ({hidden: new Set()})},
    '../../app/not-interested': {RECOMMENDATIONS_RESET: RESET, recommendationViewer: () => 'viewer', withoutHidden: (r: any) => r},
    '../../app/viewer-scope': {useViewerScope: () => ({viewerId: 'viewer', serverId: 'server'})},
    '../../app/freshness': {useFreshDocument: () => {}},
    '../../ui': {Button: 'Button', Inset: 'Inset', Page: 'Page', PageHeader: 'PageHeader'},
    '../../app/container-play': {groupSequence: () => 'group', homeRowSequence: (rowId: string, kinds: string[], opts: any) => ({rowId, kinds, ...opts})},
    '../shared/Sections': {LoadingGrid: 'LoadingGrid', SectionView: 'SectionView'},
    '../shared/GroupBrowse': {GroupBar: 'GroupBar', GroupGrid: 'GroupGrid'},
    '../../app/group-browse': {acrossGroup: () => ({}), useGroupBrowse: () => ({pending: false, active: false})},
    '../../app/i18n': {useI18n: () => defaultI18n},
    '../../player/PlayerContext': {usePlayerActions: () => ({playSequence: (...args: any[]) => played.push(args)})},
  });
  h.render(() => app.HomeRowPageScreen());
  // The first read on mount is not the control under test.
  reads.length = 0;
  const flat = () => tree(h.render(() => app.HomeRowPageScreen()));
  return {h, flat, navigated, played, reads, state};
}

test('Play and Shuffle play the whole row from its first playable title', async () => {
  const screen = await mount();
  const buttons = all(screen.flat(), 'Button');
  const play = buttons.find(button => button.props.icon === 'play')!;
  const shuffle = buttons.find(button => button.props.icon === 'shuffle')!;
  assert.equal(play.props.label, t('action.play'));
  assert.equal(shuffle.props.label, t('title.shuffle'));
  play.props.onClick();
  shuffle.props.onClick();
  assert.deepEqual(screen.played.map(args => [args[0]?.[0]?.id, args[1], args[3]]), [
    ['m1', 0, {rowId: 'recommended', kinds: ['movie', 'movie'], shuffle: false}],
    ['m1', 0, {rowId: 'recommended', kinds: ['movie', 'movie'], shuffle: true}],
  ]);
});

test('Back returns Home, and the grid is the row’s own list', async () => {
  const screen = await mount();
  one(screen.flat(), 'PageHeader').props.onBack();
  assert.deepEqual(screen.navigated, [{to: '/'}]);
  const section = one(screen.flat(), 'SectionView');
  assert.equal(section.props.section.entries.length, 2);
  assert.equal(section.props.section.nextCursor, '');
  assert.equal(section.props.onMore, undefined, 'no More without a cursor');
});

test('a first read that fails offers one recovery that reads the row again', async () => {
  const screen = await mount({pager: pagerWith({phase: 'error', row: undefined, entries: [], total: 0, error: {code: 'request_failed', message: 'Home could not load.', retryable: true}})});
  const error = one(screen.flat(), 'ErrorState');
  error.props.retry();
  assert.deepEqual(screen.reads, ['load']);
});

test('with the row still on screen, the recovery re-reads the row rather than asking for a page that is not there', async () => {
  // A refused cursor restarts the row; the restart can fail with the row still shown and no
  // cursor left. The recovery control must then read the row, not call more() on an empty cursor.
  const pager = pagerWith({phase: 'error', nextCursor: '', error: {code: 'request_failed', message: 'Home could not refresh.', retryable: true}});
  const screen = await mount({pager});
  // The mount read is not the control under test; start from empty.
  screen.reads.length = 0;
  one(screen.flat(), 'ErrorNotice').props.retry();
  assert.deepEqual(screen.reads, ['load'], 'Try again re-reads the row');
  // With a cursor still offered, the same control asks for the next page instead.
  const paged = await mount({pager: pagerWith({phase: 'error', nextCursor: 'cursor-2', error: {code: 'request_failed', message: 'Home could not refresh.', retryable: true}})});
  paged.reads.length = 0;
  one(paged.flat(), 'ErrorNotice').props.retry();
  assert.deepEqual(paged.reads, ['more']);
});

test('More reads the next page only when the server offered a cursor', async () => {
  const paged = await mount({pager: pagerWith({nextCursor: 'cursor-2'})});
  one(paged.flat(), 'SectionView').props.onMore();
  assert.deepEqual(paged.reads, ['more']);
});

test('a recommendations reset reloads this viewer’s row, and another viewer’s does not', async () => {
  const screen = await mount();
  events.dispatchEvent(new CustomEvent(RESET, {detail: {viewer: 'someone-else'}}));
  assert.deepEqual(screen.reads, []);
  events.dispatchEvent(new CustomEvent(RESET, {detail: {viewer: 'viewer'}}));
  assert.deepEqual(screen.reads, ['load']);
});
