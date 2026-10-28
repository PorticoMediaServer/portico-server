import test from 'node:test';
import assert from 'node:assert/strict';
import type {DashboardStatus} from '@core/admin/dashboard.ts';
import type {DashboardSessions} from '../src/screens/dashboard/bindings';
import {desktopDashboardQuietState, type DashboardQuietInput} from '../src/screens/dashboard/dashboard-quiet';

const status = (overrides: Partial<DashboardStatus> = {}): DashboardStatus => ({observedAt: '2026-10-26T00:00:00Z', alerts: [], scans: [], tasks: {count: 0, items: []}, ...overrides});
const sessions = (streams = 0, items: DashboardSessions['items'] = []): DashboardSessions => ({items, totals: {streams, paused: 0, converting: 0, bandwidthKbps: 0, people: 0, atHome: 0, away: 0}} as DashboardSessions);
const ready = (overrides: Partial<DashboardQuietInput> = {}): DashboardQuietInput => ({status: status(), statusLoading: false, statusFailed: false, sessions: sessions(), sessionsLoading: false, sessionsFailed: false, ...overrides});

test('only all three known empty documents group', () => {
  assert.deepEqual(desktopDashboardQuietState(ready()), {alerts: 'quiet', tasks: 'quiet', streams: 'quiet', grouped: true});
  for (const override of [{status: undefined}, {statusLoading: true}, {statusFailed: true}]) {
    const value = desktopDashboardQuietState(ready(override)); assert.equal(value.alerts, 'unknown'); assert.equal(value.tasks, 'unknown'); assert.equal(value.grouped, false);
  }
  for (const override of [{sessions: undefined}, {sessionsLoading: true}, {sessionsFailed: true}]) {
    const value = desktopDashboardQuietState(ready(override)); assert.equal(value.streams, 'unknown'); assert.equal(value.grouped, false);
  }
});

test('raw unknown alerts and queued or unknown jobs never disappear through presentation filters', () => {
  const alert = {code: 'future-alert', action: {kind: 'future', target: ''}, major: false, params: {}, severity: 'warning'};
  const job = {id: 'job', kind: 'future-kind', state: 'queued', phase: 'waiting'};
  assert.equal(desktopDashboardQuietState(ready({status: status({alerts: [alert]})})).alerts, 'not-quiet');
  assert.equal(desktopDashboardQuietState(ready({status: status({tasks: {count: 1, items: [job]}})})).tasks, 'not-quiet');
  assert.equal(desktopDashboardQuietState(ready({status: status({tasks: {count: 1, items: []}})})).tasks, 'not-quiet');
  assert.equal(desktopDashboardQuietState(ready({status: status({tasks: {count: 0, items: [{...job, state: 'running'}]}})})).tasks, 'not-quiet');
  assert.equal(desktopDashboardQuietState(ready({status: status({scans: [{libraryId: 'id', libraryName: 'Library', kind: 'movie', phase: 'scan', discovered: 0, analyzed: 0}]})})).tasks, 'not-quiet');
});

test('an empty loaded page cannot hide a positive stream total', () => {
  const active = desktopDashboardQuietState(ready({sessions: sessions(1)}));
  assert.equal(active.streams, 'not-quiet'); assert.equal(active.grouped, false);
  const item = {} as DashboardSessions['items'][number];
  assert.equal(desktopDashboardQuietState(ready({sessions: sessions(0, [item])})).streams, 'not-quiet');
});
