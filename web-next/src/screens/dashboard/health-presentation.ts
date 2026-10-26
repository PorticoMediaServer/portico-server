import type {MetricName, TelemetryPoint, TelemetryReading} from '@core/telemetry.ts';
import {defaultI18n} from '@i18n';
import {LOAD_TILES, loadValue} from '@core/server-admin/dashboard.ts';

const warmingUp = 'Waiting for a second reading before a rate can be reported.';
export type HealthTile = Readonly<{
  metric: MetricName; label: (typeof LOAD_TILES)[number]['label'] | 'next.dashboard.sentToViewers'; value?: string;
  points: readonly TelemetryPoint[]; status: 'available' | 'limited' | 'unavailable';
}>;

/** Both viewer traffic streams must be available and have the same actual timestamps. */
function viewerTraffic(reading?: TelemetryReading): Pick<HealthTile, 'value' | 'points' | 'status'> {
  if (!reading || reading.status.homeMbps?.status !== 'available' || reading.status.awayMbps?.status !== 'available')
    return {status: 'unavailable', points: []};
  const home = reading.series.homeMbps ?? [], away = reading.series.awayMbps ?? [];
  const h = home.at(-1), a = away.at(-1);
  if (!h || !a || h.t !== a.t || !Number.isFinite(h.v) || !Number.isFinite(a.v)) return {status: 'unavailable', points: []};
  // Do not connect across a missing component: without per-point status the
  // only safe line is one whose complete published timestamps pair exactly.
  const paired = home.length === away.length && home.every((point, i) => point.t === away[i].t && Number.isFinite(point.v) && Number.isFinite(away[i].v));
  return {status: 'available', value: defaultI18n.t('next.dashboard.mbps', {value: String(Number((h.v + a.v).toFixed(1)))}),
    points: paired ? home.map((point, i) => ({t: point.t, v: point.v + away[i].v})) : []};
}

/** Exact published metrics and statuses, without promoting an unknown value to zero. */
export function healthTiles(reading?: TelemetryReading): readonly HealthTile[] {
  return LOAD_TILES.map(tile => {
    if (tile.metric === 'netOut') return {metric: tile.metric, label: 'next.dashboard.sentToViewers', ...viewerTraffic(reading)};
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
