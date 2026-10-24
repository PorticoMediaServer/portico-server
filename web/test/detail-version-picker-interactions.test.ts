/**
 * Behaviour QA (P2-BEHAVIOUR-QA): the drawn Detail version picker sends what it shows.
 *
 * Title pages offer Version, Audio and Subtitles before Play. These tests drive the real
 * PlaybackChoices component: the row is absent when there is nothing to choose, a second version
 * or audio track appears as its own control, the subtitle list offers Automatic, Off and each
 * track, choosing a version clears the track picks, and the choice it reports carries the start
 * request's version and audio identity. The offers and playback-options clients are stubbed.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import * as offersCore from '@core/player-offers.ts';
import * as optionsCore from '@core/playback-v1/options.ts';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, tree} from './helpers/interaction-harness.mjs';

const t = (key: any, values?: any) => defaultI18n.t(key, values);

const stream = (patch: any = {}) => ({index: 0, type: 'audio', codec: 'aac', default: false, forced: false, enabled: true, reason: '', ...patch});
const source = (id: string, patch: any = {}) => ({
  id, available: true, container: 'mkv', videoCodec: 'h264', audioCodec: 'aac', width: 1920, height: 1080, duration: 3600,
  factsRevision: 1, factsStatus: 'known', relation: 'unknown', boundary: 'whole_source',
  enabled: false, reason: 'source_selection_unavailable', streams: [stream({index: 1})], qualities: [], ...patch,
});
const offers = (patch: any = {}) => ({
  preparedVersions: [], preparedOffersRevision: '', subtitlePlan: null, subtitlePlanUnavailableReason: null,
  scope: {serverId: 'server', libraryId: 'library', itemId: 'item', viewerFence: 'fence'}, revision: 'rev1',
  sources: [source('s1')], current: null, controls: [], markers: null, deliveryPolicy: null,
  offersRevision: 'a'.repeat(64), transcodingEnabled: false, ...patch,
});

async function mount(data: any, options?: any) {
  const h = createRenderer();
  const chosen: any[] = [];
  const service = {
    subscribe: () => () => {},
    getSnapshot: () => ({generation: 1, phase: 'ready', data, error: null, query: null, scope: {}}),
    select: () => Promise.resolve(), cancel: () => {}, refresh: () => Promise.resolve(),
  };
  const api = {request: () => Promise.resolve({})};
  const optionsClient = {options: () => Promise.resolve(options ?? {versions: []}), dispose: () => {}};
  const app = await componentModule(new URL('../src/screens/detail/PlaybackChoices.tsx', import.meta.url), {
    react: h.react,
    '@core/player-offers.ts': {...offersCore, PlayerOffersService: class { constructor() { Object.assign(this, service); } }},
    '@core/playback-v1/options.ts': {...optionsCore, PlaybackOptionsClient: class { constructor() { Object.assign(this, optionsClient); } }},
    '@core/playback-v1/local-http.ts': {localV1Http: () => ({})},
    '../../app/freshness': {useFreshDocument: () => {}},
    '../../app/session': {useSession: () => ({api})},
    '../../app/viewer-scope': {useViewerScope: () => ({viewerId: 'viewer', serverId: 'server'})},
    '../../app/i18n': {useI18n: () => defaultI18n},
    './Detail.module.css': {default: {}},
    '../../ui': {Select: 'Select', Button: 'Button'},
  });
  const draw = () => app.PlaybackChoices({libraryId: 'library', itemId: 'item', onChange: (choice: any) => chosen.push(choice)});
  h.render(draw);
  const flat = () => tree(h.render(draw));
  return {h, flat, chosen};
}

const selectByLabel = (rendered: any[], label: string) => all(rendered, 'Select').find(node => node.props.label === label);

test('nothing to choose draws no row at all', async () => {
  const screen = await mount(offers());
  const rendered = screen.flat();
  assert.equal(all(rendered, 'Select').length, 0);
  assert.equal(rendered.filter(node => node.props?.['aria-label'] === t('title.playbackChoices')).length, 0);
});

test('a second version appears as its own control and is sent with the start', async () => {
  const data = offers({sources: [source('s1'), source('s2', {height: 2160, videoCodec: 'hevc'})]});
  const screen = await mount(data);
  const version = selectByLabel(screen.flat(), t('title.version'))!;
  assert.deepEqual(version.props.options.map((option: any) => option.value), ['s1', 's2']);
  assert.equal(version.props.value, 's1', 'the current source is preselected');
  version.props.onChange({target: {value: 's2'}});
  const last = screen.chosen.at(-1);
  assert.equal(last.choices.versionId, 's2');
});

test('a second audio track is its own control, and the track identity travels with Play', async () => {
  const audio = [stream({index: 1, language: 'eng', default: true}), stream({index: 2, language: 'fra', channelLayout: '5.1', channels: 6})];
  const screen = await mount(offers({sources: [source('s1', {streams: audio})]}));
  const control = selectByLabel(screen.flat(), t('title.audio'))!;
  assert.deepEqual(control.props.options.map((option: any) => option.value), ['1', '2']);
  assert.equal(control.props.value, '1', 'the default track is preselected');
  control.props.onChange({target: {value: '2'}});
  const last = screen.chosen.at(-1);
  assert.equal(last.audioStreamIndex, 2);
  assert.equal(last.choices.audioTrackId, 'a2');
});

test('the subtitle list offers Automatic, Off and each delivered track', async () => {
  const plan = {resources: [
    {id: 'sub1', enabled: true, language: 'eng', forced: false, hearingImpaired: false},
    {id: 'sub2', enabled: true, language: 'fra', forced: true, hearingImpaired: false},
    {id: 'sub3', enabled: false, language: 'deu', forced: false, hearingImpaired: false},
  ]};
  const screen = await mount(offers({subtitlePlan: plan}));
  let control = selectByLabel(screen.flat(), t('title.subtitles'))!;
  assert.deepEqual(control.props.options.map((option: any) => option.value), ['', 'off', 'sub1', 'sub2']);
  assert.equal(control.props.value, '', 'untouched reads Automatic, never a guess');
  control.props.onChange({target: {value: 'off'}});
  assert.equal(screen.chosen.at(-1).subtitle, 'off');
  control = selectByLabel(screen.flat(), t('title.subtitles'))!;
  control.props.onChange({target: {value: 'sub1'}});
  assert.deepEqual(screen.chosen.at(-1).subtitle, {resourceId: 'sub1'});
  control = selectByLabel(screen.flat(), t('title.subtitles'))!;
  control.props.onChange({target: {value: ''}});
  assert.ok(!('subtitle' in screen.chosen.at(-1)), 'Automatic sends no subtitle choice');
});

test('choosing another version clears the track picks it no longer applies to', async () => {
  const audio = [stream({index: 1}), stream({index: 2, language: 'fra'})];
  const plan = {resources: [{id: 'sub1', enabled: true, language: 'eng', forced: false, hearingImpaired: false}]};
  const screen = await mount(offers({sources: [source('s1', {streams: audio}), source('s2')], subtitlePlan: plan}));
  selectByLabel(screen.flat(), t('title.audio'))!.props.onChange({target: {value: '2'}});
  selectByLabel(screen.flat(), t('title.subtitles'))!.props.onChange({target: {value: 'sub1'}});
  assert.equal(screen.chosen.at(-1).audioStreamIndex, 2);
  selectByLabel(screen.flat(), t('title.version'))!.props.onChange({target: {value: 's2'}});
  const last = screen.chosen.at(-1);
  assert.equal(last.choices.versionId, 's2');
  assert.ok(!('audioStreamIndex' in last), 'the old version’s audio pick is gone');
  assert.ok(!('subtitle' in last), 'the old version’s subtitle pick is gone');
});
