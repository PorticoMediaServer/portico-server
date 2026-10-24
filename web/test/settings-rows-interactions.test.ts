/**
 * Behaviour QA (P2-BEHAVIOUR-QA): the drawn settings rows save what they say.
 *
 * Settings is core flow 7: each row's control must write the value it shows. These tests drive the
 * real RowView for a switch, a choice (including a value only an API client stored), a
 * device-only choice, a browser playback fix, the preferred-language list, and the action rows
 * (Link TV, sign out). The registry, device preferences and session are stubbed.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {addLanguage, moveLanguage, removeLanguage} from '@core/presentation/index.ts';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, tree} from './helpers/interaction-harness.mjs';

const t = (key: any, values?: any) => defaultI18n.t(key, values);

async function mount(options: {values?: Record<string, any>; fields?: any[]; fixes?: any; libraryView?: string} = {}) {
  const h = createRenderer();
  const saved: any[] = [];
  const navigated: any[] = [];
  const updated: any[] = [];
  const writes: any[] = [];
  let signsOut = 0;
  const fields = options.fields ?? [{key: 'playback.autoPlayNext', default: false}, {key: 'playback.quality', default: 'auto'}, {key: 'audio.languages', default: ['eng']}];
  const values = options.values ?? {};
  let fixes = options.fixes ?? {forceStereo: false, disablePassthrough: false, disableHdr: false};
  const server = {
    snapshot: {registry: {fields}},
    value: (key: string, fallback: any) => (key in values ? values[key] : fallback),
    set: () => Promise.resolve(), reload: () => {},
  };
  const device = {preferences: {libraryView: options.libraryView ?? 'grid'}, update: (patch: any) => updated.push(patch)};
  const app = await componentModule(new URL('../src/screens/settings/SettingsRows.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {useNavigate: () => (to: any) => { navigated.push(to); return Promise.resolve(); }},
    '../../app/session': {useSession: () => ({signOut: () => { signsOut++; }})},
    '../../app/i18n': {useI18n: () => defaultI18n},
    '../../app/preferences': {usePreferences: () => device},
    '../../app/server-preferences': {useServerPreferences: () => server},
    '../../bridge/playback-fixes': {
      currentPlaybackFixes: () => fixes,
      writePlaybackFixes: (patch: any) => { writes.push(patch); fixes = {...fixes, ...patch}; return fixes; },
    },
    './ResetRecommendations': {ResetRecommendationsRow: 'ResetRecommendationsRow'},
    './Settings.module.css': {default: {}},
    '../../ui': {Badge: 'Badge', Button: 'Button', IconButton: 'IconButton', Select: 'Select', SettingsRow: 'SettingsRow', Switch: 'Switch'},
  });
  const draw = (row: any) => app.RowView({row, anchor: 'anchor', save: (patch: any) => saved.push(patch)});
  const flat = (row: any) => tree(h.render(() => draw(row)));
  return {h, flat, saved, navigated, updated, writes, fixes: () => fixes, signsOut: () => signsOut};
}

const preference = (key: string, control: any, patch: any = {}) => ({kind: 'preference', key, label: 'settings.row.test', control, ...patch});
const composite = (id: string, storage: 'registry' | 'device', control: any) => ({kind: 'composite', id, storage, label: 'settings.row.test', control});

test('a switch row writes its key and shows what the registry holds', async () => {
  const screen = await mount();
  const row = preference('playback.autoPlayNext', {type: 'switch'});
  const control = one(screen.flat(row), 'Switch');
  assert.equal(control.props.checked, false);
  assert.equal(control.props.label, t('settings.row.test'));
  control.props.onCheckedChange(true);
  assert.deepEqual(screen.saved, [{'playback.autoPlayNext': true}]);
});

test('a choice row writes the picked value, and picking the current one writes nothing', async () => {
  const screen = await mount({values: {'playback.quality': 'high'}});
  const row = preference('playback.quality', {type: 'choice', choices: [{value: 'auto', label: 'settings.choice.auto'}, {value: 'high', label: 'settings.choice.high'}, {value: 'low', label: 'settings.choice.low'}]});
  let select = one(screen.flat(row), 'Select');
  assert.equal(select.props.value, 'high');
  assert.deepEqual(select.props.options.map((option: any) => option.value), ['auto', 'high', 'low']);
  select.props.onChange({target: {value: 'low'}});
  assert.deepEqual(screen.saved, [{'playback.quality': 'low'}]);
  select = one(screen.flat(row), 'Select');
  select.props.onChange({target: {value: 'high'}});
  assert.equal(screen.saved.length, 1, 'the current value is not written again');
});

test('a value only an API client stored is shown as itself, never as the nearest choice', async () => {
  const screen = await mount({values: {'playback.quality': 'experimental'}});
  const row = preference('playback.quality', {type: 'choice', choices: [{value: 'auto', label: 'settings.choice.auto'}, {value: 'high', label: 'settings.choice.high'}]});
  const select = one(screen.flat(row), 'Select');
  assert.equal(select.props.value, 'experimental');
  assert.ok(select.props.options.some((option: any) => option.value === 'experimental'), 'the stored value has its own option');
  select.props.onChange({target: {value: 'experimental'}});
  assert.deepEqual(screen.saved, [], 'showing it does not rewrite it');
});

test('a device-only choice updates this browser and never the profile', async () => {
  const screen = await mount({libraryView: 'grid'});
  const row = composite('libraryView', 'device', {type: 'choice', choices: [{value: 'grid', label: 'web.prefs.grid'}, {value: 'list', label: 'web.prefs.list'}]});
  const select = one(screen.flat(row), 'Select');
  assert.equal(select.props.value, 'grid');
  select.props.onChange({target: {value: 'list'}});
  assert.deepEqual(screen.updated, [{libraryView: 'list'}]);
  assert.deepEqual(screen.saved, []);
});

test('a browser playback fix is kept with the browser and sent with what it can play', async () => {
  const screen = await mount({fixes: {forceStereo: false, disablePassthrough: false, disableHdr: false}});
  const row = composite('forceStereo', 'device', {type: 'switch'});
  const control = one(screen.flat(row), 'Switch');
  assert.equal(control.props.checked, false);
  control.props.onCheckedChange(true);
  assert.deepEqual(screen.writes, [{forceStereo: true}]);
  assert.equal(one(screen.flat(row), 'Switch').props.checked, true, 'the row redraws from the fix it kept');
});

test('the preferred languages move, leave and join in order through the row', async () => {
  const screen = await mount({values: {'audio.languages': ['eng', 'fra']}});
  const row = preference('audio.languages', {type: 'languages'});
  let rendered = screen.flat(row);
  // Each language has Move up, Move down and Remove, in that order.
  const buttons = () => all(rendered, 'IconButton');
  assert.equal(buttons().length, 6);
  buttons()[1].props.onClick();
  assert.deepEqual(screen.saved.at(-1), {'audio.languages': moveLanguage(['eng', 'fra'], 0, 'down')});
  buttons()[5].props.onClick();
  assert.deepEqual(screen.saved.at(-1), {'audio.languages': removeLanguage(['eng', 'fra'], 1)});
  rendered = screen.flat(row);
  const add = one(rendered, 'Select');
  add.props.onChange({target: {value: 'deu'}});
  assert.deepEqual(screen.saved.at(-1), {'audio.languages': addLanguage(['eng', 'fra'], 'deu')});
});

test('the action rows link a TV and sign out', async () => {
  const screen = await mount();
  const link = {kind: 'action', id: 'linkTV', label: 'settings.row.linkTv'};
  one(screen.flat(link), 'SettingsRow').props.onClick();
  assert.deepEqual(screen.navigated, [{to: '/device'}]);
  const signOut = {kind: 'action', id: 'signOut', label: 'settings.row.signOut'};
  const button = all(screen.flat(signOut), 'Button').find(node => node.props.label === t('settings.row.signOut'))!;
  button.props.onClick();
  assert.equal(screen.signsOut(), 1);
});
