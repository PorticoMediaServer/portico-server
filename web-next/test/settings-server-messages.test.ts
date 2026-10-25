import test from 'node:test';
import assert from 'node:assert/strict';
import {SettingsMessagesSource, appendMessage, messageRetryMs} from '../src/screens/settings/server/troubleshooting/messages-source.ts';
import type {LogRecord} from '../../packages/client-core/src/server-administration.ts';
const line = (sequence: string): LogRecord => ({sequence, at: '2026-10-22T19:00:00Z', level: 'info', category: 'server', message: 'Backup finished.'});
const settings = {revision: 1, logLevel: 'info', effectiveLevel: 'info', retention: [], levels: ['error','warn','info','debug'], categories: ['server','playback','scan','network','client']};
const page = {items: [line('2'), line('1')], limit: 200, nextCursor: '', level: 'info', effectiveLevel: 'info'};
const transport = {request: async <T>(path: string) => (path.includes('/settings') ? settings : page) as T};

test('list is chronological and filter requests use server filters', async () => {
  const calls: string[] = [];
  const source = new SettingsMessagesSource({request: async <T>(path: string) => {calls.push(path);return transport.request<T>(path);}}, undefined, () => true);
  await source.load();assert.equal(source.getSnapshot().phase, 'ready');assert.deepEqual(source.getSnapshot().lines.map(x => x.sequence), ['1','2']);
  await source.select('warn', 'network');assert.ok(calls.some(path => path.includes('level=warn') && path.includes('category=network')));source.dispose();
});

test('bounded records dedupe sequence strings without numeric precision loss', () => {
  const lines = Array.from({length: 500}, (_, i) => line(String(i)));
  assert.equal(appendMessage(lines, line('499')), lines);
  const next = appendMessage(lines, line('9007199254740993'));assert.equal(next.length, 500);assert.equal(next[0].sequence, '1');assert.equal(next[499].sequence, '9007199254740993');
});

test('held reads cannot publish into a retired authority', async () => {
  let current = true, release!: (value: unknown) => void;
  const source = new SettingsMessagesSource({request: async <T>(path: string) => path.includes('/settings') ? settings as T : new Promise<T>(resolve => {release = value => resolve(value as T);})}, undefined, () => current);
  const pending = source.load();current = false;release(page);await pending;assert.equal(source.getSnapshot().phase, 'idle');source.dispose();
});

test('live tail resets, dedupes, and resumes with opaque Last-Event-ID after normal end', async t => {
  t.mock.timers.enable({apis: ['setTimeout']});
  const headers: Record<string,string>[] = [];let calls = 0;
  const source = new SettingsMessagesSource(transport, {open: async h => {
    headers.push(h);calls++;
    const events = calls === 1 ? `event: reset\nid: nonce:8\ndata: {}\n\nevent: log\nid: nonce:9\ndata: ${JSON.stringify(line('9'))}\n\nevent: log\nid: nonce:9\ndata: ${JSON.stringify(line('9'))}\n\n` : '';
    return new Response(events, {headers: {'Content-Type': 'text/event-stream'}});
  }}, () => true);
  await source.load();source.setLive(true);
  for (let n = 0; n < 20; n++) await Promise.resolve();
  assert.deepEqual(source.getSnapshot().lines.map(x => x.sequence), ['9']);assert.equal(source.getSnapshot().reconnecting, true);
  t.mock.timers.tick(2000);for (let n = 0; n < 20; n++) await Promise.resolve();
  assert.equal(headers[1]['Last-Event-ID'], 'nonce:9');source.pause();const count = calls;t.mock.timers.tick(120000);await Promise.resolve();assert.equal(calls, count);source.dispose();
});

test('429 Retry-After wins, exponential delay is bounded', () => {
  assert.equal(messageRetryMs(1, '90'), 90000);assert.equal(messageRetryMs(1, 'Thu, 01 Jan 1970 00:00:10 GMT', 0), 10000);assert.equal(messageRetryMs(50, null), 64000);
});

test('permission refusal stops live retries and preserves listed lines', async () => {
  const source = new SettingsMessagesSource(transport, {open: async () => new Response('', {status: 403})}, () => true);
  await source.load();source.setLive(true);for (let n = 0; n < 15; n++) await Promise.resolve();
  assert.equal(source.getSnapshot().live, false);assert.equal(source.getSnapshot().phase, 'error');assert.equal(source.getSnapshot().lines.length, 2);source.dispose();
});
