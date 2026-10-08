import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

const load = () => componentModule(new URL('../src/screens/server/Channels.tsx', import.meta.url), {
  react: {default: {}, useEffect: () => {}, useMemo: (f: () => unknown) => f(), useRef: (v: unknown) => ({current: v}), useState: (v: unknown) => [v, () => {}]},
  '@core/library-channels.ts': {}, '@core/live-source-draft.ts': {channelRequestID: () => 'op-test'},
  '../../admin/console': {}, '../../app/session': {}, '../../ui': {}, './Server': {}, './ChannelBuilder': {},
  './operation-ids': {createOperationIds: () => ({forPayload: (k) => k, release: () => {}})}, '../../app/i18n': {currentI18n: () => ({t: (s: string) => s}), useI18n: () => ({t: (s: string) => s})},
}) as Promise<any>;

test('CD-15: regeneration sends the published replacementBoundary unchanged', async () => {
  const {regenerationBody} = await load();
  const body = regenerationBody({revision: 7, replacementBoundary: '2026-09-23T12:00:00Z'}, 'req-1');
  assert.deepEqual(body, {requestId: 'req-1', expectedRevision: 7, boundary: '2026-09-23T12:00:00Z'});
  assert.notEqual(regenerationBody({revision: 7, replacementBoundary: '2026-09-23T12:00:00Z'}, 'req-2').requestId, body.requestId, 'a changed intent uses a new request id');
});

test('CD-15: an unloaded or stale boundary cannot submit', async () => {
  const {regenerationBody} = await load();
  assert.throws(() => regenerationBody({revision: 7, replacementBoundary: ''}, 'req-1'), 'an idle channel with no boundary cannot submit');
  assert.throws(() => regenerationBody({revision: 0, replacementBoundary: '2026-09-23T12:00:00Z'}, 'req-1'), 'revision zero cannot submit');
  assert.throws(() => regenerationBody({revision: 7, replacementBoundary: '2026-09-23T12:00:00Z'}, ''), 'no request without an id');
});
