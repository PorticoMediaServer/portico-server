/** X-13 (client): an offline server's failed connect shows the specific dialog, not the generic notice. */
import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {componentModule} from './helpers/component-harness.mjs';
import * as core from '../../packages/client-core/src/index.ts';

const load = () => componentModule(new URL('../src/screens/auth/HostedChooser.tsx', import.meta.url), {
  react: {default: {}}, '@core/index.ts': core, '@core/hosted-gate.ts': {}, '../../app/foreground-retry': {}, '../../app/session': {}, '../../app/i18n': {}, '../../ui': {}, './DirectProfiles': {}, './Auth.module.css': {default: {}},
}) as Promise<any>;
const now = 1_800_000_000_000;
const iso = (ms: number) => new Date(ms).toISOString();
const server = (id: string, presence?: {online: boolean; lastSeenAt: string | null}) => ({id, name: id, baseUrl: `https://${id}.example`, presence});

test('a stale offline server gets the specific dialog; recent, unknown and online servers keep waiting', async () => {
  const {chooserFailureView} = await load();
  assert.equal(chooserFailureView(server('s', {online: false, lastSeenAt: iso(now - 8 * 86_400_000)}), 'unreachable', now), 'offline-dialog');
  assert.equal(chooserFailureView(server('s', {online: false, lastSeenAt: null}), 'unreachable', now), 'offline-dialog', 'never seen');
  assert.equal(chooserFailureView(server('s', {online: false, lastSeenAt: iso(now - 30 * 60_000)}), 'unreachable', now), 'waiting', 'seen half an hour ago: the keep-trying copy');
  assert.equal(chooserFailureView(server('s'), 'unreachable', now), 'waiting', 'unknown presence renders normally');
  assert.equal(chooserFailureView(server('s', {online: true, lastSeenAt: iso(now - 30 * 60_000)}), 'unreachable', now), 'waiting');
  assert.equal(chooserFailureView(server('s', {online: false, lastSeenAt: iso(now - 8 * 86_400_000)}), 'failed', now), 'none');
  assert.equal(chooserFailureView(server('s', {online: false, lastSeenAt: iso(now - 8 * 86_400_000)}), 'account-unavailable', now), 'none');
  assert.equal(chooserFailureView(server('s', {online: false, lastSeenAt: iso(now - 8 * 86_400_000)}), undefined, now), 'none');
});

test('the chooser branches on the decision: offline dialog copy, never the waiting copy for a stale server', async () => {
  const source = readFileSync(new URL('../src/screens/auth/HostedChooser.tsx', import.meta.url), 'utf8');
  assert.match(source, /chooserFailureView\(target, outcome\) === 'offline-dialog'/);
  assert.match(source, /web\.chooser\.unreachableTitle/);
  assert.match(source, /web\.chooser\.signInDirectly/);
  assert.match(source, /sortServersByPresence\(servers\)/);
  assert.match(source, /web\.chooser\.lastSeen/);
  assert.match(source, /web\.chooser\.neverSeen/);
});
