import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

const liveTv = () => componentModule(new URL('../src/screens/server/LiveTV.tsx', import.meta.url), {
  react: {default: {}, useEffect: () => {}, useMemo: (f: () => unknown) => f(), useState: (v: unknown) => [v, () => {}]},
  '@core/linear-api.ts': {}, '@core/live-source-draft.ts': {channelRequestID: () => 'op', newLiveSourceInput: () => ({})},
  '@core/dvr.ts': {}, '@core/presentation/index.ts': {}, '../../admin/console': {}, '../../admin/poll-while': {},
  '../../app/session': {}, '../../ui': {}, './Server': {}, './DVRSettings': {}, './LiveSourceExtras': {}, './operation-ids': {createOperationIds: () => ({forPayload: (k: string) => k, release: () => {}})},
  '../../app/i18n': {currentI18n: () => ({t: (s: string) => s}), useI18n: () => ({t: (s: string) => s})},
}) as Promise<any>;

const extras = () => componentModule(new URL('../src/screens/server/LiveSourceExtras.tsx', import.meta.url), {
  react: {default: {}, useEffect: () => {}, useMemo: (f: () => unknown) => f(), useState: (v: unknown) => [v, () => {}]},
  '@core/administration.ts': {}, '@core/channel-guide.ts': {}, '../../admin/recording-policy': {},
  './DVRSettings': {}, '../../admin/console': {}, '../../app/session': {}, './operation-ids': {createOperationIds: () => ({forPayload: (k: string) => k, release: () => {}})},
  '../../app/i18n': {useI18n: () => ({t: (s: string) => s})}, '../../ui': {},
}) as Promise<any>;

test('CD-49: refresh is offered only for a remote refreshable source that is enabled', async () => {
  const {canRefreshSource} = await liveTv();
  assert.equal(canRefreshSource({id: 's1', state: 'active'}, [{sourceId: 's1'}]), true);
  assert.equal(canRefreshSource({id: 's1', state: 'disabled'}, [{sourceId: 's1'}]), false, 'paused sources must first be enabled');
  assert.equal(canRefreshSource({id: 's1', state: 'active'}, [{sourceId: 'other'}]), false, 'uploaded sources have no remote refresh entry');
  assert.equal(canRefreshSource({id: 's1', state: 'active'}, []), false);
  assert.equal(canRefreshSource({id: 's1', state: 'active'}, undefined), false);
});

test('CD-49: a bare HDHomeRun host becomes an explicit http URL; bad schemes are refused', async () => {
  const {normalizeHdHomerunLocator} = await liveTv();
  assert.equal(normalizeHdHomerunLocator('192.168.1.50'), 'http://192.168.1.50');
  assert.equal(normalizeHdHomerunLocator('192.168.1.50:5004'), 'http://192.168.1.50:5004');
  assert.equal(normalizeHdHomerunLocator('  my-tuner.local  '), 'http://my-tuner.local');
  assert.equal(normalizeHdHomerunLocator('http://192.168.1.50'), 'http://192.168.1.50', 'existing schemes preserved');
  assert.equal(normalizeHdHomerunLocator('https://192.168.1.50:5004/lineup'), 'https://192.168.1.50:5004/lineup');
  assert.throws(() => normalizeHdHomerunLocator('ftp://192.168.1.50'), 'unsupported schemes rejected');
  assert.throws(() => normalizeHdHomerunLocator('file:///etc/passwd'), 'unsupported schemes rejected');
});

test('CD-49: selecting guide-only clears both the guide pointer and the stream buffer', async () => {
  const {guideOnlyPatch} = await extras();
  assert.deepEqual(guideOnlyPatch('xmltv-guide'), {kind: 'xmltv-guide', guideSourceId: '', streamBufferSeconds: null}, 'null/omitted, not zero');
  assert.deepEqual(guideOnlyPatch('playlist'), {kind: 'playlist'});
});

test('CD-49: 201 channel edits chunk into 200+1 sequential batches', async () => {
  const {chunkChannelMapEntries, channelMapBatchLimit} = await extras();
  assert.equal(channelMapBatchLimit, 200, 'the server caps overrides at MaxPageSize');
  const entries = Array.from({length: 201}, (_, i) => ({channelId: `c${i}`}));
  const batches = chunkChannelMapEntries(entries);
  assert.equal(batches.length, 2);
  assert.equal(batches[0]!.length, 200);
  assert.equal(batches[1]!.length, 1);
  assert.deepEqual(chunkChannelMapEntries([]), []);
  assert.equal(chunkChannelMapEntries(Array.from({length: 200}, (_, i) => i)).length, 1);
});
