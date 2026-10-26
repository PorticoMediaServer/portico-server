import type {MetricName, TelemetryPoint, TelemetryReading} from '@core/telemetry.ts';
import {defaultI18n} from '@i18n';
import {LOAD_TILES, loadValue} from '@core/server-admin/dashboard.ts';

const warmingUp = 'Waiting for a second reading before a rate can be reported.';
type History = 'samples' | 'withheld' | 'gap';
export type HealthTile = Readonly<{
  metric: MetricName; label: (typeof LOAD_TILES)[number]['label'] | 'next.dashboard.sentToViewers'; value: string;
  points: readonly TelemetryPoint[]; history: History;
}>;

/** Both viewer traffic streams must be available and have the same actual timestamps. */
function viewerTraffic(reading?: TelemetryReading): Pick<HealthTile, 'value' | 'points' | 'history'> | undefined {
  if (!reading || reading.status.homeMbps?.status !== 'available' || reading.status.awayMbps?.status !== 'available') return undefined;
  const home = reading.series.homeMbps ?? [], away = reading.series.awayMbps ?? [];
  const h = home.at(-1), a = away.at(-1);
  if (!h || !a || h.t !== a.t || !Number.isFinite(h.v) || !Number.isFinite(a.v) || h.v < 0 || a.v < 0) return undefined;
  // Without per-point status, do not connect across an unmatched observation.
  const paired = home.length === away.length && home.every((point, i) => point.t === away[i].t && Number.isFinite(point.v) && Number.isFinite(away[i].v) && point.v >= 0 && away[i].v >= 0);
  return {value: defaultI18n.t('next.dashboard.mbps', {value: String(Number((h.v + a.v).toFixed(1)))}),
    points: paired ? home.map((point, i) => ({t: point.t, v: point.v + away[i].v})) : [], history: paired ? 'samples' : 'gap'};
}

/** Omit unmeasurable metrics; retain actual measured zero. */
export function healthTiles(reading?: TelemetryReading): readonly HealthTile[] {
  return LOAD_TILES.flatMap(tile => {
    if (tile.metric === 'netOut') {
      const viewer = viewerTraffic(reading);
      return viewer ? [{metric: tile.metric, label: 'next.dashboard.sentToViewers' as const, ...viewer}] : [];
    }
    const metric = reading?.status[tile.metric];
    if (!metric || metric.status === 'unavailable' || !Number.isFinite(metric.value) || metric.value < 0 || metric.detail === warmingUp) return [];
    // The collector can include synthetic warm-up zeros in these series and
    // publishes no per-point provenance, including in aggregated history.
    const withheld = tile.metric === 'cpu' || tile.metric === 'diskRead';
    return [{metric: tile.metric, label: tile.label, value: loadValue(tile, metric),
      points: withheld ? [] : reading?.series[tile.metric] ?? [], history: withheld ? 'withheld' as const : 'samples' as const}];
  });
}

export type HealthPlot = Readonly<{line: string; area: string; endX: number; endY: number}>;
/** Measured nonnegative observations only: actual interval on x, zero baseline and 10% headroom. */
export function healthPlot(points: readonly TelemetryPoint[], from: number, until: number): HealthPlot | undefined {
  if (!Number.isFinite(from) || !Number.isFinite(until) || until <= from || points.length < 2) return undefined;
  if (points.some((p, i) => !Number.isFinite(p.t) || !Number.isFinite(p.v) || p.v < 0 || p.t < from || p.t > until || (i > 0 && p.t <= points[i - 1].t))) return undefined;
  const peak = Math.max(...points.map(p => p.v)), top = peak * 1.1;
  const coords = points.map(p => ({x: (p.t - from) / (until - from) * 100, y: top === 0 ? 44 : 44 - p.v / top * 40}));
  const line = coords.map((p, i) => `${i ? 'L' : 'M'}${p.x.toFixed(2)},${p.y.toFixed(2)}`).join(' ');
  const first = coords[0], last = coords[coords.length - 1];
  return {line, area: `${line} L${last.x.toFixed(2)},46 L${first.x.toFixed(2)},46 Z`, endX: last.x, endY: last.y};
}
