import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

const load = () => componentModule(new URL('../src/screens/server/Diagnostics.tsx', import.meta.url), {
  react: {default: {}, useEffect: () => {}, useMemo: (f: () => unknown) => f(), useState: (v: unknown) => [v, () => {}]},
  '@core/index.ts': {}, '../../bridge/protectedArtwork': {}, '@core/admin/index.ts': {},
  '../../admin/console': {}, '../../admin/capabilities': {}, '../../app/session': {},
  '../../app/i18n': {}, '../../ui': {}, './operation-ids': {createOperationIds: () => ({forPayload: (k: string) => k, release: () => {}})},
}) as Promise<any>;

test('BE-API-15: reconnects resume from the retained SSE id', async () => {
  const {logTailHeaders} = await load();
  assert.deepEqual(logTailHeaders('tok', ''), {Authorization: 'Bearer tok', Accept: 'text/event-stream'});
  // Ids are opaque recorderNonce:sequence strings, sent back verbatim.
  assert.deepEqual(logTailHeaders('tok', 'rec-9:412'), {Authorization: 'Bearer tok', Accept: 'text/event-stream', 'Last-Event-ID': 'rec-9:412'});
});

test('BE-API-15: a 429 honors Retry-After, otherwise exponential backoff', async () => {
  const {logTailBackoffMs, logTailRetryDelayMs} = await load();
  assert.equal(logTailBackoffMs(1), 2000);
  assert.equal(logTailBackoffMs(6), 64000);
  assert.equal(logTailBackoffMs(99), 64000);
  assert.equal(logTailRetryDelayMs(429, 25000, 6), 25000, 'the server asked delay wins');
  for (const attempt of [1, 3, 6]) {
    const delay = logTailRetryDelayMs(undefined, undefined, attempt);
    const base = logTailBackoffMs(attempt);
    assert.ok(delay >= base && delay < base * 1.5, `backoff around ${base}ms`);
  }
  const noHeader = logTailRetryDelayMs(429, undefined, 2);
  assert.ok(noHeader >= 4000 && noHeader < 6000, 'a 429 without Retry-After still backs off');
});

test('BE-API-15: a reset frame drops the window and later frames rebuild it', async () => {
  const {applyLogTailFrame} = await load();
  const r = (sequence: string) => ({sequence, at: '2026-09-23T00:00:00Z', level: 'info', category: 'server', message: 'm'});
  let lines = [r('1'), r('2')];
  lines = applyLogTailFrame(lines, {type: 'reset'});
  assert.deepEqual(lines, [], 'the earlier window is gone');
  lines = applyLogTailFrame(lines, {type: 'record', record: r('3')});
  assert.deepEqual(lines.map(x => x.sequence), ['3'], 'subsequent frames rebuild');
  lines = applyLogTailFrame(lines, {type: 'record', record: r('3')});
  assert.deepEqual(lines.map(x => x.sequence), ['3'], 'duplicates still collapse');
  let many: any[] = [];
  for (let n = 0; n < 600; n++) many = applyLogTailFrame(many, {type: 'record', record: r(String(n))});
  assert.equal(many.length, 500, 'the window stays bounded');
  assert.equal(many[0].sequence, '100');
});

test('BE-API-15: the client reconnects when the 30-minute stream ends, and stops only when aborted', async () => {
  const {logTailShouldReconnect, logTailRetryDelayMs, logTailBackoffMs} = await load();
  assert.equal(logTailShouldReconnect(false), true, 'a normal 30-minute end reconnects');
  assert.equal(logTailShouldReconnect(true), false, 'unmounting or toggling live off stops');
  // The normal-end reconnect backs off like any other reconnect, resuming from Last-Event-ID.
  for (const attempt of [1, 2, 6]) {
    const delay = logTailRetryDelayMs(undefined, undefined, attempt);
    const base = logTailBackoffMs(attempt);
    assert.ok(delay >= base && delay < base * 1.5);
  }
});
