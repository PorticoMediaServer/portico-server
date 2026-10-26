import type {MetricName, TelemetryPoint, TelemetryReading} from '@core/telemetry.ts';
import {LOAD_TILES, loadValue} from '@core/server-admin/dashboard.ts';

const warmingUp = 'Waiting for a second reading before a rate can be reported.';
export type HealthTile = Readonly<{
  metric: MetricName; label: (typeof LOAD_TILES)[number]['label']; value?: string;
  points: readonly TelemetryPoint[]; status: 'available' | 'limited' | 'unavailable';
}>;

/** The server's exact six metrics and statuses, without promoting an unknown value to zero. */
export function healthTiles(reading?: TelemetryReading): readonly HealthTile[] {
  return LOAD_TILES.map(tile => {
    const metric = reading?.status[tile.metric];
    const measured = metric && metric.status !== 'unavailable' && Number.isFinite(metric.value) && metric.detail !== warmingUp;
    return {metric: tile.metric, label: tile.label, value: measured ? loadValue(tile, metric) : undefined,
      status: measured ? metric.status : 'unavailable', points: measured ? reading?.series[tile.metric] ?? [] : []};
  });
}

/** Real observations only, positioned within the server's own interval. */
export function healthPath(points: readonly TelemetryPoint[], from: number, until: number): string | undefined {
  if (!Number.isFinite(from) || !Number.isFinite(until) || until <= from || points.length < 2) return undefined;
  if (points.some((p, i) => !Number.isFinite(p.t) || !Number.isFinite(p.v) || p.t < from || p.t > until || (i > 0 && p.t <= points[i - 1].t))) return undefined;
  const values = points.map(p => p.v), min = Math.min(...values), max = Math.max(...values);
  return points.map((p, i) => `${i ? 'L' : 'M'}${((p.t - from) / (until - from) * 100).toFixed(2)},${(max === min ? 20 : 38 - (p.v - min) / (max - min) * 36).toFixed(2)}`).join(' ');
}
