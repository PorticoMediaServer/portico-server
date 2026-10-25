/**
 * Behaviour QA (P2-BEHAVIOUR-QA): the drawn Live TV entry behaves as it says.
 *
 * The channels shell decides what Live TV opens on and what it says when there is nothing to
 * watch: the address is replaced with the default source, a failed read offers Try again, an
 * empty server says so, the Recordings entry draws the DVR page, and on a phone the source picker
 * chips and dropdown go to their source. The guide grid itself is not rendered here; its geometry
 * is covered by the live smoke and visual lanes.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, tree} from './helpers/interaction-harness.mjs';

const t = (key: any, values?: any) => defaultI18n.t(key, values);

async function mount(options: {entry?: string; sources?: any[]; loading?: boolean; failed?: boolean; compact?: boolean} = {}) {
  const h = createRenderer();
  const navigated: any[] = [];
  const calls: string[] = [];
  const sources = options.sources ?? [{id: 's1', name: 'Living room', type: 'live', recordAvailable: false}, {id: 's2', name: 'Attic', type: 'live', recordAvailable: false}];
  const api = {request: () => Promise.resolve({})};
  const entries = (next: any[]) => [
    ...(next.length >= 2 ? [{id: 'all', name: 'All channels', icon: 'grid', library: false}] : []),
    ...next.map(source => ({id: source.id, name: source.name, icon: 'channels', library: false})),
    ...(next.some(source => source.recordAvailable) ? [{id: 'recordings', name: t('channels.recordings'), icon: 'dvr', library: false}] : []),
  ];
  const app = await componentModule(new URL('../src/screens/channels/Channels.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {
      useParams: () => (options.entry ? {entry: options.entry} : {}),
      useSearch: () => ({}),
      useNavigate: () => (to: any) => { navigated.push(to); return Promise.resolve(); },
    },
    '../../app/channels': {
      ALL: 'all', LIBRARY: 'library', RECORDINGS: 'recordings',
      channelEntries: (list: any[], translate: any) => entries(list).map(entry => ({...entry, name: entry.id === 'recordings' ? translate('channels.recordings') : entry.name})),
      defaultEntry: (list: any[]) => (list.length >= 2 ? 'all' : list[0]?.id ?? 'all'),
      useChannelSources: () => ({sources, loading: !!options.loading, failed: !!options.failed, refresh: () => calls.push('refresh')}),
      useChannelMemory: () => ({view: 'guide', sort: 'number', setView: () => {}}),
      channelGuideSource: () => ({}),
      forgetGuideReads: () => {},
      sourceName: (list: any[], id: string) => list.find(source => source.id === id)?.name ?? '',
      channelReason: () => '',
      serverWideOf: () => undefined,
      hasReminder: () => false,
      readReminders: () => [],
      subscribeReminders: () => () => {},
      toggleReminder: () => {},
      removeReminder: () => {},
      setChannelSurf: () => {},
      localTimezone: () => 'UTC',
    },
    '../live/guide': {useDVRClient: () => null},
    '../live/DVR': {DVRView: 'DVRView'},
    '../live/now-store': {createNowStore: () => ({get: () => 0, getSnapshot: () => 0, subscribe: () => () => {}, dispose: () => {}})},
    '../live/dvr-groups': {dvrPaddings: {}, dvrLimits: {}},
    '../../app/session': {useSession: () => ({api, session: {viewer: {serverId: 'server'}}, owner: false})},
    '../../app/i18n': {useI18n: () => defaultI18n},
    '../../app/errors': {ErrorState: 'ErrorState', errorText: () => 'Live TV could not load.'},
    '../../player/PlayerContext': {usePlayerActions: () => ({playSequence: () => {}, play: () => {}})},
    '../../app/freshness': {useQuietRefresh: () => {}},
    './Channels.module.css': {default: {}},
    '../../ui': {
      Artwork: 'Artwork', Badge: 'Badge', Button: 'Button', Chip: 'Chip', Chips: 'Chips', Dialog: 'Dialog', Icon: 'Icon', IconButton: 'IconButton',
      Menu: 'Menu', Notice: 'Notice', Page: 'Page', PageHeader: 'PageHeader', Segmented: 'Segmented', Select: 'Select', Skeleton: 'Skeleton',
      StateView: 'StateView', Text: 'Text', Toast: 'Toast', cx: (...names: any[]) => names.filter(Boolean).join(' '),
      useArtworkUrl: () => '', useCompact: () => !!options.compact, useHoverCapable: () => false,
    },
  });
  const draw = () => app.ChannelsScreen();
  h.render(draw);
  const flat = () => tree(h.render(draw));
  return {h, flat, navigated, calls};
}

test('no address with two sources opens All channels by replacing the address', async () => {
  const screen = await mount();
  assert.deepEqual(screen.navigated.at(-1), {to: '/channels/$entry', params: {entry: 'all'}, replace: true});
});

test('loading draws the guide’s skeleton, not an empty server', async () => {
  const screen = await mount({loading: true, sources: []});
  const rendered = screen.flat();
  assert.equal(one(rendered, 'PageHeader').props.title, t('channels.title'));
  assert.ok(all(rendered, 'Skeleton').length > 0);
  assert.equal(all(rendered, 'StateView').length, 0);
});

test('a failed source read says so and offers Try again', async () => {
  const screen = await mount({loading: false, failed: true, sources: []});
  const state = one(screen.flat(), 'StateView');
  assert.equal(state.props.body, 'Live TV could not load.');
  state.props.action.onClick();
  assert.deepEqual(screen.calls, ['refresh']);
});

test('an empty server says there are no sources', async () => {
  const screen = await mount({loading: false, sources: []});
  const state = one(screen.flat(), 'StateView');
  assert.equal(state.props.title, t('web.live.empty.noSourcesTitle'));
  assert.equal(state.props.body, t('web.live.empty.noSourcesMember'));
});

test('the Recordings entry draws the DVR page under its own heading', async () => {
  const screen = await mount({entry: 'recordings', sources: [{id: 's1', name: 'Living room', type: 'live', recordAvailable: true}]});
  const rendered = screen.flat();
  assert.equal(all(rendered, 'DVRView').length, 1);
  assert.equal(one(rendered, 'PageHeader').props.eyebrow, t('channels.title'));
});

test('on a phone with few sources, the picker chips go to their source', async () => {
  const screen = await mount({entry: 'recordings', compact: true, sources: [{id: 's1', name: 'Living room', type: 'live', recordAvailable: true}]});
  const chips = all(screen.flat(), 'Chip');
  assert.deepEqual(chips.map(chip => chip.props.label), ['Living room', t('channels.recordings')]);
  assert.equal(chips.find(chip => chip.props.pressed)!.props.label, t('channels.recordings'));
  chips[0].props.onClick();
  assert.deepEqual(screen.navigated.at(-1), {to: '/channels/$entry', params: {entry: 's1'}});
});

test('on a phone with many sources, the picker is a menu that goes to its source', async () => {
  const screen = await mount({entry: 'recordings', compact: true, sources: [
    {id: 's1', name: 'Living room', type: 'live', recordAvailable: true},
    {id: 's2', name: 'Attic', type: 'live', recordAvailable: false},
    {id: 's3', name: 'Studio', type: 'live', recordAvailable: false},
  ]});
  const menu = all(screen.flat(), 'Menu').find(node => node.props.label === t('channels.source'))!;
  assert.deepEqual(menu.props.items.map((item: any) => item.id), ['all', 's1', 's2', 's3', 'recordings']);
  menu.props.onSelect('s2');
  assert.deepEqual(screen.navigated.at(-1), {to: '/channels/$entry', params: {entry: 's2'}});
});
