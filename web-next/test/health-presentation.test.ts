import test from 'node:test';
import assert from 'node:assert/strict';
import type {MetricName, TelemetryPoint, TelemetryReading} from '@core/telemetry.ts';
import {healthDetail, healthPlot, healthTiles} from '../src/screens/dashboard/health-presentation';

const point = (t: number, v: number, status?: 'available' | 'limited', detail?: string): TelemetryPoint => ({t, v, ...(status ? {status} : {}), ...(detail ? {detail} : {})});
function reading(): TelemetryReading {
  const names: MetricName[] = ['cpu', 'memory', 'diskRead', 'diskWrite', 'netIn', 'netOut', 'gpuUsage', 'gpuMemory', 'gpuEncoder', 'processCPU', 'processMemory', 'homeMbps', 'awayMbps', 'hardwareConversions', 'softwareConversions'];
  return {window: '10m', period: '10m', from: '2026-01-01T00:00:00Z', until: '2026-01-01T00:10:00Z', timeZone: 'UTC', buckets: [], observedAt: 600,
    status: Object.fromEntries(names.map(name => [name, {status: 'unavailable', value: 0}])),
    series: Object.fromEntries(names.map(name => [name, []]))} as unknown as TelemetryReading;
}

test('unknown zero or unknown middle point never becomes a measured line or statistics', () => {
  const r = reading(); r.status.cpu = {status: 'available', value: 0};
  r.series.cpu = [point(1, 1, 'available'), point(2, 0), point(3, 2, 'available')];
  const tile = healthTiles(r).find(item => item.metric === 'cpu')!;
  assert.equal(tile.value, '0%'); assert.equal(tile.history, 'gap'); assert.deepEqual(tile.points, []);
  assert.equal(healthPlot(r.series.cpu, 0, 10), undefined); assert.equal(healthDetail(r.series.cpu, 0, 10, true), undefined);
});

test('known CPU and disk history preserve available zero and real limited detail', () => {
  const r = reading(); r.status.cpu = {status: 'available', value: 0}; r.status.diskRead = {status: 'limited', value: 5, detail: 'partial volume'};
  r.series.cpu = [point(1, 0, 'available'), point(2, 3, 'available')];
  r.series.diskRead = [point(1, 0, 'available'), point(2, 5, 'limited', 'partial volume')];
  const tiles = healthTiles(r), cpu = tiles.find(item => item.metric === 'cpu')!, disk = tiles.find(item => item.metric === 'diskRead')!;
  assert.equal(cpu.history, 'samples'); assert.equal(cpu.points[0].v, 0); assert.equal(healthDetail(cpu.points, 0, 10, true)?.low, 0);
  assert.equal(disk.points[1].detail, 'partial volume'); assert.equal(disk.limited, true); assert.deepEqual(disk.details, ['partial volume']);
});

test('viewer rates use current metrics and retain paired provenance, refusing gaps', () => {
  const r = reading(); r.status.homeMbps = {status: 'available', value: 0}; r.status.awayMbps = {status: 'limited', value: 2, detail: 'limited away'};
  r.series.homeMbps = [point(1, 0, 'available'), point(2, 1, 'limited', 'limited home')];
  r.series.awayMbps = [point(1, 0, 'available'), point(2, 2, 'limited', 'limited away')];
  let viewer = healthTiles(r).find(item => item.metric === 'netOut')!;
  assert.equal(viewer.value, '2 Mbps'); assert.equal(viewer.points[0].v, 0); assert.equal(viewer.points[1].status, 'limited');
  assert.match(viewer.points[1].detail!, /limited home/); assert.match(viewer.points[1].detail!, /limited away/);
  r.series.awayMbps[1] = point(3, 2, 'available'); viewer = healthTiles(r).find(item => item.metric === 'netOut')!;
  assert.equal(viewer.value, '2 Mbps'); assert.equal(viewer.history, 'gap'); assert.equal(healthDetail(viewer.points, 0, 10, false), undefined);
  r.series.awayMbps[1] = point(2, 2); viewer = healthTiles(r).find(item => item.metric === 'netOut')!;
  assert.equal(viewer.history, 'gap'); assert.deepEqual(viewer.points, []);
});
