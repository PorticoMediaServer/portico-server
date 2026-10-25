/** Programme-sheet action wiring. Real legacy adapters register source objects; DVR calls
 * are captured, so no schedules or reminders leave this isolated test process. */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {legacyChannelRow, legacyProgram} from '@core/guide/legacy-source.ts';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, tree} from './helpers/interaction-harness.mjs';
const T = Date.UTC(2026, 9, 24, 18);
const MIN = 60_000;
// Use protocol-valid identities: seriesRuleConfig validates the real source/channel IDs.
const SOURCE = 'a'.repeat(48);
const CHANNEL = 'b'.repeat(64);
const GENERATION = 'c'.repeat(48);
const PROGRAMME = 'd'.repeat(64);
const RULE = 'e'.repeat(64);
const t = (key: any, values?: any) => defaultI18n.t(key, values);

async function mount(options: {when?: 'past' | 'future'; series?: boolean; noProgram?: boolean; unavailable?: boolean; recordable?: boolean; fail?: () => boolean} = {}) {
  const h = createRenderer();
  const calls = {watch: [] as any[], favorite: [] as any[], remind: [] as any[], schedule: [] as any[], rules: [] as any[], navigate: [] as any[], changed: 0, close: 0};
  const rawChannel: any = {id: CHANNEL, sourceId: SOURCE, provenance: 'live-source', name: 'One', number: '1', group: '', generation: GENERATION,
    programmes: [], tuneAvailable: !options.unavailable, recordAvailable: options.recordable !== false,
    tuneUnavailableReason: '', recordUnavailableReason: '', favorite: false, hidden: false, preferenceRevision: 1};
  const start = options.when === 'past' ? T - 120 * MIN : options.when === 'future' ? T + 60 * MIN : T - 15 * MIN;
  const rawProgram: any = {id: PROGRAMME, channelId: CHANNEL, title: 'Programme', start: new Date(start).toISOString(), end: new Date(start + 60 * MIN).toISOString(),
    lineage: 'provider-id', seriesId: options.series ? 'series-0' : '', episodeId: '', newEvidence: 'unknown', description: 'Description'};
  const channel = legacyChannelRow(rawChannel), program = legacyProgram(rawProgram);
  const dvr = {schedule: async (...args: any[]) => {
    calls.schedule.push(args);
    if (options.fail?.()) throw new Error('unavailable');
  }, newRuleID: async () => RULE, saveRule: async (...args: any[]) => { calls.rules.push(args); }};
  const nowStore = {get: () => T, getSnapshot: () => T, subscribe: () => () => {}, dispose() {}};
  const app = await componentModule(new URL('../src/screens/channels/Channels.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {useParams: () => ({}), useSearch: () => ({}), useNavigate: () => (to: any) => { calls.navigate.push(to); return Promise.resolve(); }},
    '../../app/channels': {
      ALL: 'all', LIBRARY: 'library', RECORDINGS: 'recordings',
      channelEntries: () => [], defaultEntry: () => 'all', hasReminder: () => false, readReminders: () => [],
      removeReminder: () => {}, sourceName: (list: any[], id: string) => list.find(source => source.id === id)?.name ?? '',
      setChannelSurf: () => {}, forgetGuideReads: () => {}, channelReason: () => '', serverWideOf: () => undefined,
      subscribeReminders: () => () => {}, toggleReminder: (value: any) => calls.remind.push(value),
      useChannelMemory: () => ({view: 'guide', sort: 'number', setView() {}, setSort() {}}),
      useChannelSources: () => ({sources: [{id: SOURCE, name: 'Living room', type: 'live', position: 0, recordAvailable: true}], loading: false, failed: false, refresh() {}}),
      channelGuideSource: () => ({}), localTimezone: () => 'UTC',
    },
    '../../app/session': {useSession: () => ({api: {}, session: {viewer: {serverId: 'server'}}, owner: false})},
    '../../app/i18n': {useI18n: () => defaultI18n},
    '../../app/errors': {ErrorState: 'ErrorState', errorText: () => 'Could not schedule.'},
    '../../player/PlayerContext': {usePlayerActions: () => ({playSequence() {}, play() {}})},
    '../live/guide': {useDVRClient: () => dvr},
    '../live/now-store': {createNowStore: () => nowStore},
    '../live/DVR': {DVRView: 'DVRView'},
    '../../app/freshness': {useQuietRefresh: () => {}},
    './Channels.module.css': {default: {}},
    '../live/dvr-groups': {dvrPaddings: [0, 60, 120], dvrLimits: [0, 5, 10]},
    '../../ui': {
      Artwork: 'Artwork', Badge: 'Badge', Button: 'Button', Chip: 'Chip', Chips: 'Chips', Dialog: 'Dialog', Icon: 'Icon',
      IconButton: 'IconButton', Menu: 'Menu', Notice: 'Notice', Page: 'Page', PageHeader: 'PageHeader', Segmented: 'Segmented',
      Select: 'Select', Skeleton: 'Skeleton', StateView: 'StateView', Text: 'Text', Toast: 'Toast',
      cx: (...names: unknown[]) => names.filter(Boolean).join(' '),
      useArtworkUrl: () => ({url: ''}), useCompact: () => false, useHoverCapable: () => true,
    },
  });
  const format = {zone: 'UTC', t, dayLabel: () => t('guide.today'), clock: (ms: number) => defaultI18n.time(ms),
    range: (a: number, b: number) => t('guide.range', {start: defaultI18n.time(a), end: defaultI18n.time(b)}), duration: () => ''};
  const draw = () => app.ProgramSheet({selection: {channel, ...(options.noProgram ? {} : {program})}, format,
    sources: [{id: SOURCE, name: 'Living room'}], showSource: true,
    onClose: () => { calls.close++; }, onWatch: (row: any) => calls.watch.push(row),
    onChanged: () => { calls.changed++; }, onFavorite: (row: any) => calls.favorite.push(row)});
  const render = () => tree(h.render(draw));
  const settle = async () => { for (let i = 0; i < 8; i++) { await Promise.resolve(); render(); } return render(); };
  return {calls, render, settle, rawChannel, rawProgram};
}
const buttons = (rendered: any[], label: string) => all(rendered, 'Button').filter(node => node.props.label === label);
const lastButton = (rendered: any[], label: string) => { const button = buttons(rendered, label).at(-1); assert.ok(button, label); return button; };
const freeze = (context: any) => context.mock.method(Date, 'now', () => T);

test('on-air Watch and Favorite dispatch the shown channel and close the sheet', async context => {
  freeze(context);
  const f = await mount();
  const rendered = f.render();
  lastButton(rendered, t('guide.watchLive')).props.onClick();
  assert.equal(f.calls.watch.at(-1).id, CHANNEL);
  assert.equal(f.calls.close, 1);
  lastButton(rendered, t('channels.addFavorite')).props.onClick();
  assert.equal(f.calls.favorite.at(-1).id, CHANNEL);
  assert.equal(f.calls.close, 2);
});

test('a future programme offers Remind rather than Watch and passes the programme identity', async context => {
  freeze(context);
  const f = await mount({when: 'future'});
  const rendered = f.render();
  assert.equal(buttons(rendered, t('guide.watchLive')).length, 0);
  assert.equal(buttons(rendered, t('guide.watchChannel')).length, 0);
  lastButton(rendered, t('guide.remind')).props.onClick();
  assert.deepEqual(f.calls.remind, [{programId: PROGRAMME, channelId: CHANNEL, title: 'Programme', channelName: 'One', start: T + 60 * MIN}]);
});

test('past programmes and non-recordable channels have no Record action', async context => {
  freeze(context);
  const past = await mount({when: 'past'});
  assert.equal(buttons(past.render(), t('guide.record')).length, 0);
  assert.ok(lastButton(past.render(), t('guide.watchChannel')));
  const unavailable = await mount({recordable: false});
  assert.equal(buttons(unavailable.render(), t('guide.record')).length, 0);
  const untunable = await mount({unavailable: true});
  assert.equal(lastButton(untunable.render(), t('guide.watchLive')).props.disabled, true);
});

test('Record asks first; padding choices reach the real legacy recording draft', async context => {
  freeze(context);
  const f = await mount();
  lastButton(f.render(), t('guide.record')).props.onClick();
  let rendered = f.render();
  assert.equal(all(rendered, 'Dialog').length, 2);
  assert.equal(f.calls.schedule.length, 0, 'opening confirmation does not schedule');
  all(rendered, 'Select').find(node => node.props.label === t('web.dvr.startEarly')).props.onChange({target: {value: '60'}});
  all(rendered, 'Select').find(node => node.props.label === t('web.dvr.endLate')).props.onChange({target: {value: '120'}});
  rendered = f.render();
  lastButton(rendered, t('guide.record')).props.onClick();
  rendered = await f.settle();
  assert.equal(f.calls.schedule.length, 1);
  const [draft, options] = f.calls.schedule[0];
  assert.equal(draft.channel, f.rawChannel);
  assert.equal(draft.programme, f.rawProgram);
  assert.equal(draft.series, false);
  assert.equal(options.beforeSeconds, 60);
  assert.equal(options.afterSeconds, 120);
  assert.equal(f.calls.changed, 1);
  assert.equal(all(rendered, 'Dialog').length, 1);
  assert.equal(buttons(rendered, t('guide.record')).length, 0);
  const success = all(rendered, 'Notice').find(node => node.props.tone === 'success');
  assert.ok(success);
  success.props.action.onClick();
  assert.deepEqual(f.calls.navigate.at(-1), {to: '/channels/$entry', params: {entry: 'recordings'}});
  assert.equal(f.calls.close, 1);
});

test('Cancel closes the recording confirmation without scheduling', async context => {
  freeze(context);
  const f = await mount();
  lastButton(f.render(), t('guide.record')).props.onClick();
  lastButton(f.render(), t('action.cancel')).props.onClick();
  assert.equal(all(f.render(), 'Dialog').length, 1);
  assert.deepEqual(f.calls.schedule, []);
});

test('failed scheduling keeps confirmation open, reports failure and allows retry', async context => {
  freeze(context);
  let fail = true;
  const f = await mount({fail: () => fail});
  lastButton(f.render(), t('guide.record')).props.onClick();
  lastButton(f.render(), t('guide.record')).props.onClick();
  let rendered = await f.settle();
  assert.equal(all(rendered, 'Dialog').length, 2);
  assert.equal(f.calls.changed, 0);
  assert.equal(all(rendered, 'Notice').find(node => node.props.tone === 'error')?.props.children[0], 'Could not schedule.');
  fail = false;
  lastButton(rendered, t('guide.record')).props.onClick();
  rendered = await f.settle();
  assert.equal(f.calls.schedule.length, 2);
  assert.equal(f.calls.changed, 1);
  assert.equal(all(rendered, 'Dialog').length, 1);
});

test('series confirmation maps episode, channel scope, keep and padding choices into its rule', async context => {
  freeze(context);
  const f = await mount({series: true});
  lastButton(f.render(), t('guide.recordSeries')).props.onClick();
  let rendered = f.render();
  assert.equal(f.calls.rules.length, 0, 'opening confirmation does not create a rule');
  const segments = all(rendered, 'Segmented');
  segments.find(node => node.props.options.some((option: any) => option.id === 'all')).props.onChange('all');
  segments.find(node => node.props.options.some((option: any) => option.id === 'any')).props.onChange('any');
  all(rendered, 'Select').find(node => node.props.label === t('web.dvr.keepLabel')).props.onChange({target: {value: '5'}});
  all(rendered, 'Select').find(node => node.props.label === t('web.dvr.startEarly')).props.onChange({target: {value: '60'}});
  all(rendered, 'Select').find(node => node.props.label === t('web.dvr.endLate')).props.onChange({target: {value: '120'}});
  lastButton(f.render(), t('guide.recordSeriesTitle')).props.onClick();
  rendered = await f.settle();
  assert.equal(f.calls.rules.length, 1);
  const [id, revision, config, anchor] = f.calls.rules[0];
  assert.equal(id, RULE);
  assert.equal(revision, 0);
  assert.equal(config.seriesId, 'series-0');
  assert.equal(config.episodes, 'all');
  assert.deepEqual(config.allowedChannels, []);
  assert.equal(config.options.episodeLimit, 5);
  assert.equal(config.options.beforeSeconds, 60);
  assert.equal(config.options.afterSeconds, 120);
  assert.deepEqual(anchor, {sourceId: SOURCE, channelId: CHANNEL, generation: GENERATION, programmeId: PROGRAMME});
  assert.equal(f.calls.changed, 1);
  assert.equal(all(rendered, 'Dialog').length, 1);
  assert.equal(buttons(rendered, t('guide.recordSeries')).length, 0);
});

test('series Cancel creates no rule, and default scope is this channel with new episodes', async context => {
  freeze(context);
  const f = await mount({series: true});
  lastButton(f.render(), t('guide.recordSeries')).props.onClick();
  lastButton(f.render(), t('action.cancel')).props.onClick();
  assert.equal(f.calls.rules.length, 0);
  assert.equal(all(f.render(), 'Dialog').length, 1);
  lastButton(f.render(), t('guide.recordSeries')).props.onClick();
  lastButton(f.render(), t('guide.recordSeriesTitle')).props.onClick();
  await f.settle();
  assert.equal(f.calls.rules.length, 1);
  assert.equal(f.calls.rules[0][2].episodes, 'new');
  assert.deepEqual(f.calls.rules[0][2].allowedChannels, [CHANNEL]);
});
