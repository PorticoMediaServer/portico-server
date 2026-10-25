/**
 * Behaviour QA (P2-BEHAVIOUR-QA): the drawn Show page makes the right calls.
 *
 * A show page: the hero plays next up as the show's own container, Watchlist/Favorite/Watched
 * write the show's container state, the season tabs land in the address and read that season, the
 * season menu plays the season from its first episode and marks it watched, an episode row plays
 * the season from that episode (or opens it in place), the layout control is this device's, and a
 * failed read offers its recovery. The workspace, episodes, group browse and container state are
 * stubbed.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, tree} from './helpers/interaction-harness.mjs';

const t = (key: any, values?: any) => defaultI18n.t(key, values);

const episode = (id: string, title: string, number: number, patch: any = {}) => ({
  id, kind: 'episode', title, libraryId: 'lib', posterUrl: '', backdropUrl: '/e.jpg', duration: 3000, progressSeconds: 0,
  watched: false, subtitle: '', playback: {itemId: id, startSeconds: 0}, number, ...patch,
});
const show = {id: 'sh1', title: 'Severance', libraryId: 'lib', libraryKind: 'tv', posterUrl: '/s.jpg', backdropUrl: '/b.jpg', genres: ['Drama'], credits: [], year: 2022};
const season = (id: string, number: number) => ({id, number, title: `Season ${number}`, episodeCount: 2, watchedCount: 0, unwatchedCount: 2});
const workspace = (patch: any = {}) => ({
  phase: 'ready', error: null,
  data: {
    show, seasons: [season('s1', 1), season('s2', 2)], selected: {seasonId: 's1'}, groups: [], seasonTotalCount: 2,
    episodes: {sections: [{id: 'episodes', heading: {key: 'episodes', fallback: 'Episodes'}, entries: [episode('ep1', 'Good News About Hell', 1), episode('ep2', 'Half Loop', 2)], totalCount: 2, nextCursor: ''}]},
    nextUp: episode('ep1', 'Good News About Hell', 1),
    settings: undefined,
  },
  pagination: {cursor: null, canPrevious: false, nextCursor: ''},
  seasonPagination: {canPrevious: false, nextCursor: ''},
  ...patch,
});

async function mount(options: {snapshot?: any; search?: any} = {}) {
  const h = createRenderer();
  const navigated: any[] = [];
  const played: any[] = [];
  const calls: {name: string; args: any[]}[] = [];
  const snap = options.snapshot ?? workspace();
  const collection = {itemAt: (index: number) => [episode('ep1', 'Good News About Hell', 1), episode('ep2', 'Half Loop', 2)][index], getSnapshot: () => ({total: 2}), subscribe: () => () => {}, pageSize: 40};
  const watched = (kind: string) => ({
    state: {watched: false, favorite: false, watchlisted: false}, saving: false, carries: () => true,
    setFlag: (flag: string, value: boolean) => calls.push({name: 'flag', args: [kind, flag, value]}),
    setWatched: (value: boolean) => calls.push({name: 'watched', args: [kind, value]}),
  });
  const app = await componentModule(new URL('../src/screens/detail/Show.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {
      useParams: () => ({showId: 'sh1'}),
      useSearch: () => (options.search ?? {library: 'lib'}),
      useNavigate: () => (to: any) => { navigated.push(to); return Promise.resolve(); },
    },
    '../../app/back': {useBack: (fallback: any) => fallback},
    '../../app/open': {personHref: () => '/person/x'},
    '../../app/freshness': {useFreshness: () => ({hub: {settled: () => {}}})},
    '../../app/detail': {
      useShowWorkspace: () => ({
        snapshot: snap,
        selectSeason: (id: string) => calls.push({name: 'selectSeason', args: [id]}),
        selectGroup: (id: string) => calls.push({name: 'selectGroup', args: [id]}),
        nextSeasons: () => {}, previousSeasons: () => {}, retry: () => calls.push({name: 'retry', args: []}),
      }),
      useShowRecommendations: () => ({phase: 'ready', data: {rows: []}, error: null, retry: () => {}}),
    },
    '../../app/not-interested-notice': {useNotInterested: () => ({hidden: new Set()})},
    '../../app/not-interested': {withoutHidden: (row: any) => row},
    '../shared/Sections': {LoadingGrid: 'LoadingGrid', SectionView: 'SectionView'},
    '../../app/episodes': {useEpisodes: () => collection},
    '../../app/errors': {ErrorNotice: 'ErrorNotice', ErrorState: 'ErrorState', changedError: {}, errorText: () => 'failed'},
    '../../app/session': {useSession: () => ({api: {request: () => Promise.resolve({})}, owner: false})},
    '../../player/PlayerContext': {usePlayerActions: () => ({playSequence: (...args: any[]) => played.push(args), play: (...args: any[]) => played.push(args), more: (...args: any[]) => calls.push({name: 'more', args})})},
    '../../app/title-layout': {placeholderTint: () => ({})},
    './EpisodeCard': {EpisodeCard: 'EpisodeCard', EpisodePlaceholder: 'EpisodePlaceholder', EpisodeRow: 'EpisodeRow', episodeCode: () => '', episodeListKeys: () => {}},
    './GenreLinks': {GenreLinks: 'GenreLinks'},
    './EpisodeDetails': {EpisodeDetails: 'EpisodeDetails'},
    './Show.module.css': {default: {}},
    '../../app/viewer-scope': {useViewerScope: () => ({viewerId: 'viewer', serverId: 'server'})},
    '../shared/container-watched': {useContainerWatched: (_api: any, kind: string) => watched(kind)},
    '../../app/i18n': {useI18n: () => defaultI18n},
    '../shared/GroupBrowse': {GroupBar: 'GroupBar', GroupRows: 'GroupRows'},
    '../../app/group-browse': {useGroupBrowse: () => ({pending: false, active: false, collection: undefined, total: 2, refresh: () => {}})},
    '../../app/container-play': {episodeGroupSequence: (libraryId: string, showId: string, group: string) => ({episodeGroup: {libraryId, showId, group}}), groupSequence: (_group: any, extra: any = {}) => ({group: true, ...extra})},
    '../../app/quiet-refresh': {useRefreshRequiredRetry: () => false},
    '../../ui': {
      Button: 'Button', Card: 'Card', Dialog: 'Dialog', Inset: 'Inset', KeyValue: 'KeyValue', ListRow: 'ListRow', Menu: 'Menu', Notice: 'Notice',
      Page: 'Page', Section: 'Section', Segmented: 'Segmented', Select: 'Select', SettingsGroup: 'SettingsGroup', SettingsRow: 'SettingsRow',
      Shelf: 'Shelf', StateView: 'StateView', Switch: 'Switch', TitleHero: 'TitleHero', WindowedGrid: 'WindowedGrid',
      anchorOf: () => ({x: 0, y: 0, width: 1, height: 1}),
    },
  });
  h.render(() => app.ShowScreen());
  const flat = () => tree(h.render(() => app.ShowScreen()));
  return {h, flat, navigated, played, calls, snap};
}

const hero = (rendered: any[]) => one(rendered, 'TitleHero');

test('the hero plays next up as the show’s own container', async () => {
  const screen = await mount();
  hero(screen.flat()).props.primary.onClick();
  const [list, index, start, container] = screen.played.at(-1);
  assert.equal(list[0].id, 'ep1');
  assert.equal(index, 0);
  assert.deepEqual(container, {kind: 'show', id: 'sh1'});
});

test('Watchlist, Favorite and Watched write the show’s container state', async () => {
  const screen = await mount();
  const buttons = all(screen.flat(), 'Button');
  buttons.find(button => button.props.icon === 'bookmark')!.props.onClick();
  buttons.find(button => button.props.icon === 'heart')!.props.onClick();
  buttons.find(button => button.props.icon === 'check')!.props.onClick();
  assert.deepEqual(screen.calls.filter(call => call.name === 'flag').map(call => call.args), [['show', 'watchlisted', true], ['show', 'favorite', true]]);
  assert.deepEqual(screen.calls.filter(call => call.name === 'watched').map(call => call.args), [['show', true]]);
});

test('a season tab lands in the address and reads that season', async () => {
  const screen = await mount();
  const tabs = all(screen.flat(), 'button').filter(button => button.props.role === 'tab');
  assert.deepEqual(tabs.map(tab => tab.props.children?.[0] ?? ''), ['Season 1', 'Season 2']);
  tabs[1].props.onClick();
  assert.deepEqual(screen.navigated.at(-1), {to: '/show/$showId', params: {showId: 'sh1'}, search: {library: 'lib', season: '2'}});
  assert.deepEqual(screen.calls.filter(call => call.name === 'selectSeason').map(call => call.args), [['s2']]);
});

test('the season menu plays the season from its first episode and marks it watched', async () => {
  const screen = await mount();
  const menu = all(screen.flat(), 'Menu').find(node => node.props.label === t('title.moreActions'))!;
  assert.deepEqual(menu.props.items.map((item: any) => item.id), ['play-season', 'season-watched']);
  menu.props.onSelect('play-season');
  let [list, index, , container] = screen.played.at(-1);
  assert.equal(list[0].id, 'ep1');
  assert.deepEqual(container, {kind: 'season', id: 's1'});
  menu.props.onSelect('season-watched');
  assert.deepEqual(screen.calls.filter(call => call.name === 'watched').map(call => call.args), [['season', true]]);
});

test('an episode row plays the season from that episode, and its text opens it in place', async () => {
  const screen = await mount();
  const grid = one(screen.flat(), 'WindowedGrid');
  const row = grid.props.renderItem(episode('ep2', 'Half Loop', 2));
  row.props.onPlay(row.props.episode);
  const [list, , , container] = screen.played.at(-1);
  assert.equal(list[0].id, 'ep2');
  assert.deepEqual(container, {kind: 'season', id: 's1'});
  row.props.onOpen(row.props.episode);
  assert.deepEqual(screen.navigated.at(-1), {to: '/show/$showId/episode/$episodeId', params: {showId: 'sh1', episodeId: 'ep2'}, search: {library: 'lib'}, replace: true, resetScroll: false});
});

test('the episode layout is this device’s choice, and a failed read offers its recovery', async () => {
  let screen = await mount();
  const layout = one(screen.flat(), 'Segmented');
  assert.equal(layout.props.value, 'list', 'list by default');
  layout.props.onChange('grid');
  assert.equal(one(screen.flat(), 'Segmented').props.value, 'grid');
  screen = await mount({snapshot: workspace({phase: 'error', data: undefined, error: {code: 'request_failed', message: 'The show could not load.', retryable: true}})});
  one(screen.flat(), 'ErrorState').props.retry();
  assert.deepEqual(screen.calls.filter(call => call.name === 'retry').length, 1);
});
