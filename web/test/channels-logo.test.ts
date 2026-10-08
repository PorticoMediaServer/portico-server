import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

const load = () => componentModule(new URL('../src/screens/server/Channels.tsx', import.meta.url), {
  react: {default: {}, useEffect: () => {}, useMemo: (f: () => unknown) => f(), useRef: (v: unknown) => ({current: v}), useState: (v: unknown) => [v, () => {}]},
  '@core/library-channels.ts': {}, '@core/live-source-draft.ts': {channelRequestID: () => 'op-test'},
  '../../admin/console': {}, '../../app/session': {}, '../../ui': {}, './Server': {}, './ChannelBuilder': {},
  './operation-ids': {createOperationIds: () => ({forPayload: (k) => k, release: () => {}})}, '../../app/i18n': {currentI18n: () => ({t: (s: string) => s}), useI18n: () => ({t: (s: string) => s})},
}) as Promise<any>;

test('CD-13: logo uploads omit the configuration revision fence', async () => {
  const {logoUploadForm} = await load();
  const file = new File(['bytes'], 'logo.png', {type: 'image/png'});
  const form = logoUploadForm(file, 'op-1');
  assert.equal(form.get('operationId'), 'op-1');
  assert.equal(form.has('expectedRevision'), false, 'the configuration revision is not the logo revision');
  assert.equal(form.get('file'), file);
});

test('CD-13: retries of the same bytes reuse the operation id; a new file starts a new one', async () => {
  const {logoOperationKey} = await load();
  const a = {name: 'logo.png', size: 10, lastModified: 1};
  const b = {name: 'logo.png', size: 11, lastModified: 1};
  assert.equal(logoOperationKey('c1', a), logoOperationKey('c1', a), 'same bytes/intent share a key');
  assert.notEqual(logoOperationKey('c1', a), logoOperationKey('c1', b), 'different bytes start a new operation');
  assert.notEqual(logoOperationKey('c1', a), logoOperationKey('c2', a), 'different channels differ');
});
