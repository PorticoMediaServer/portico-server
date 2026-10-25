/** Prepared offline for checkpoint execution: actual SearchScreen callback wiring. */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {componentModule} from '../../web/test/helpers/component-harness.mjs';
import {all, createRenderer, one, tree, textOf} from '../../web/test/helpers/interaction-harness.mjs';
const t = (key: any, values?: any) => defaultI18n.t(key, values);
async function mount(context: any, options: {text?: string; remember?: boolean; failClear?: () => boolean} = {}) {
  context.mock.timers.enable({apis: ['setTimeout', 'setInterval']});
  const h = createRenderer(), calls: {name: string; args: any[]}[] = [];
  let current = true;
  const record = (name: string) => (...args: any[]) => { calls.push({name, args}); return Promise.resolve(); };
  const initial = options.text ?? 'harbor';
  const store = (value: any) => ({subscribe: () => () => {}, getSnapshot: () => value});
  const service = {...store({phase: 'ready', query: {q: initial}, groups: [], pagination: {cursor: ''}}),
    cancel: record('cancel'), retry: record('retry'), refresh: record('refresh')};
  const binding = {service, session: () => ({text: initial}), summary: () => [], select: record('select'),
    recent: async () => { calls.push({name: 'recent', args: []}); return {entries: [{q: 'Stored Query'}]}; },
    clearRecent: async () => { calls.push({name: 'clearRecent', args: []}); if (options.failClear?.()) throw new Error('unavailable'); },
    genres: {...store({phase: 'ready', genres: []}), load: record('genres')},
    onNow: {...store({phase: 'ready', cards: [], sourceNames: []}), load: record('onNow')},
    context: {isCurrent: () => current, identity: {liveAvailable: false}, effects: {reportError: record('error')},
      preferences: store({effective: {'search.rememberHistory': options.remember !== false, 'appearance.cardSizePercent': 100}})}};
  const app = await componentModule(new URL('../src/screens/search/Search.tsx', import.meta.url), {
    react: h.react, '../detail/text': {t, locale: () => 'en-US'},
    '../../kit/controls': {Button: 'Button', Chip: 'Chip', TextField: 'TextField'},
    '../../kit/cards': {PosterCard: 'PosterCard', SquareCard: 'SquareCard', CircleCard: 'CircleCard', EpisodeTile: 'EpisodeTile', GenreTile: 'GenreTile', LiveCard: 'LiveCard'},
    '../../kit/artwork': {Artwork: 'Artwork'}, '../../kit/layout': {WindowedGrid: 'WindowedGrid'},
    '../../kit/states': {Skeleton: 'Skeleton'}, '../../kit/overlays': {Banner: 'Banner'},
    '../../kit/states/SearchEmpty': {SearchEmpty: 'SearchEmpty'}, '../../kit/Icon': {Icon: 'Icon'},
    '../../platform/freshness': {useFreshDocument() {}}, '../../shell/viewport': {useShellWidth: () => 1440},
    '../detail/Loading': {useLoadingClock: () => 0}, '../detail/ScreenProblem': {ScreenProblem: 'ScreenProblem'},
    './RecentSearches': {RecentSearches: 'RecentSearches'}, './TopResult': {TopResult: 'TopResult'},
    './window': {appendedWindow: () => ({collection: {}, set() {}, dispose() {}})},
    './search.module.css': {default: {}},
  });
  const flat = () => tree(h.render(() => app.SearchScreen({binding})));
  const settle = async () => { for (let i = 0; i < 6; i++) { await Promise.resolve(); flat(); } };
  flat(); await settle();
  return {flat, settle, calls, focused: h.focused, retire: () => { current = false; }};
}
const selects = (f: any) => f.calls.filter((call: any) => call.name === 'select');

test('typing cancels old results immediately and debounces the trimmed query without recording', async context => {
  const f = await mount(context);
  one(f.flat(), 'TextField').props.onChange({currentTarget: {value: '  Different Query  '}}); f.flat();
  assert.equal(f.calls.filter(call => call.name === 'cancel').length, 1);
  context.mock.timers.tick(259); await f.settle(); assert.equal(selects(f).length, 0);
  context.mock.timers.tick(1); await f.settle();
  assert.deepEqual(selects(f).map((call: any) => call.args), [['Different Query', {}]]);
});

test('explicit submit records only when history is enabled', async context => {
  const f = await mount(context); let prevented = 0;
  one(f.flat(), 'form').props.onSubmit({preventDefault() { prevented++; }});
  assert.equal(prevented, 1);
  assert.deepEqual(selects(f).at(-1)?.args, ['harbor', {record: true}]);
});

test('disabled history neither loads recent queries nor asks submit to record them', async context => {
  const f = await mount(context, {remember: false});
  one(f.flat(), 'form').props.onSubmit({preventDefault() {}});
  assert.equal(f.calls.some(call => call.name === 'recent'), false);
  assert.deepEqual(selects(f).at(-1)?.args, ['harbor', {}]);
});

test('ArrowDown focuses the first result while other keys preserve their default action', async context => {
  const f = await mount(context); let prevented = 0;
  const field = one(f.flat(), 'TextField');
  field.props.onKeyDown({key: 'ArrowDown', preventDefault() { prevented++; }});
  field.props.onKeyDown({key: 'ArrowUp', preventDefault() { prevented++; }});
  assert.equal(prevented, 1);
  assert.ok(f.focused.includes('first-result'));
});

test('failed Clear keeps real history and reports failure; explicit retry clears both', async context => {
  let failed = true;
  const f = await mount(context, {text: '', failClear: () => failed});
  one(f.flat(), 'RecentSearches').props.onClear(); await f.settle();
  assert.deepEqual(one(f.flat(), 'RecentSearches').props.queries, ['Stored Query']);
  assert.equal(textOf(all(f.flat(), 'p').find(node => node.props.role === 'alert')), t('web.search.clearFailed'));
  failed = false; one(f.flat(), 'RecentSearches').props.onClear(); await f.settle();
  assert.deepEqual(one(f.flat(), 'RecentSearches').props.queries, []);
  assert.equal(all(f.flat(), 'p').some(node => node.props.role === 'alert'), false);
  assert.equal(f.calls.filter(call => call.name === 'clearRecent').length, 2);
});

test('a retired screen does not submit or apply a recent-query selection', async context => {
  const f = await mount(context, {text: ''}); f.retire();
  one(f.flat(), 'RecentSearches').props.onSelect('Stored Query');
  one(f.flat(), 'form').props.onSubmit({preventDefault() {}});
  assert.equal(one(f.flat(), 'TextField').props.value, '');
  assert.equal(selects(f).length, 0);
});
