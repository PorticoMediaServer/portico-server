import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '../../packages/i18n/src/index.ts';
import type {ActivitySummarySession, OwnerActivitySummary} from '@core/admin/activity-summary/index.ts';
import {activitySubject, activityView} from '../src/shell/activity-model.ts';
import {homeFixtures as f} from '../../packages/i18n/src/catalog/en-US/next/dev/home-fixtures.ts';

const environment = {t: defaultI18n.t, locale: 'en-US', timeZone: 'UTC'};
function stream(): ActivitySummarySession { return {id: 'session', kind: 'vod', state: 'playing', positionMs: 480000,
  user: {id: 'account', name: f.account}, profile: {id: 'profile', name: f.profile}, device: {id: 'device', name: f.device},
  decision: {video: {action: 'transcode', reasons: []}}, quality: {mode: 'fixed', id: '2160p', maxHeight: 2160}, videoHeight: 1080,
  item: {id: 'episode', title: f.episode, kind: 'episode', showId: 'show', showTitle: f.show, seasonNumber: 2, episodeNumber: 3, art: []}, durationMs: 1800000}; }
function summary(session = stream()): OwnerActivitySummary { return {scope: {serverId: 'server', viewerFence: 'viewer-fence'}, serverName: f.server, observedAt: '2026-10-22T12:00:00Z',
  streams: {count: 9, items: [session]}, scans: [{libraryId: 'library', libraryName: f.libraryPrefix, kind: 'movies', phase: 'analyzing', analyzed: 12, discovered: 40}],
  tasks: {count: 4, items: [{id: 'task', kind: 'metadata', state: 'running', phase: 'fetching', processed: 5}]},
  update: {available: true, state: 'available', current: '1.0', latest: '1.1'}, health: {degraded: false}, lastScanFinishedAt: '2026-10-22T11:55:00Z'}; }
const actions = {onDashboard: () => {}, onRetry: () => {}, openUpdate: () => {}};

test('Activity maps semantic episode identity while keeping the real profile and device separate', () => {
  const view = activityView(summary(), environment, actions), value = view.streams[0];
  assert.equal(value.identity.title, f.show);
  assert.equal(value.identity.subtitle.find(part => part.id === 'episode-title')?.text, f.episode);
  assert.equal(value.name, f.profile); assert.equal(value.device, f.device);
  assert.equal(value.positionMs, 480000); assert.equal(value.durationMs, 1800000);
  assert.equal(view.streamCount, 9); assert.equal(view.taskCount, 4);
});
test('conversion names the delivered height and never substitutes the requested quality ceiling', () => {
  const view = activityView(summary(), environment, actions);
  assert.ok(view.streams[0].conversion?.text.includes('1080p'));
  assert.ok(!view.streams[0].conversion?.text.includes('2160'));
  assert.equal(activityView(summary({...stream(), videoHeight: undefined}), environment, actions).streams[0].conversion?.text, defaultI18n.t('web.nowPlaying.delivery.convertVideo'));
});
test('unknown totals stay unknown and server work counts and identifiers survive mapping', () => {
  const view = activityView(summary(), environment, actions);
  assert.equal(view.scans[0].discovered, 40); assert.equal(view.scans[0].processed, 12); assert.equal(view.scans[0].total, undefined);
  assert.equal(view.tasks[0].processed, 5); assert.equal(view.tasks[0].total, undefined);
  assert.equal(view.update.available, true); assert.equal(view.update.current, '1.0'); assert.equal(view.update.latest, '1.1');
  assert.equal(view.lastScan, defaultI18n.relativeTime('2026-10-22T11:55:00Z', Date.parse('2026-10-22T12:00:00Z')));
});
test('an absent or unrecognized media projection never becomes an invented title', () => {
  assert.equal(activitySubject({...stream(), item: undefined}), undefined);
  assert.equal(activitySubject({...stream(), item: {...stream().item!, kind: 'unrecognized'}}), undefined);
});
