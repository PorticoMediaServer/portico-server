import {formatAudioSource, formatDeliveryTarget, formatSubtitleSource, formatVideoSource, formatCodec} from './format';
import type {DashboardSession as AdminSession} from './bindings';
import type {DashboardHistory} from '@core/admin/dashboard.ts';
import type {TelemetryPoint} from '@core/telemetry.ts';

export const mediaKinds = ['movies', 'tv', 'music', 'audiobooks', 'live'] as const;
export type MediaKind = typeof mediaKinds[number];
export function orderedSessions(sessions: readonly AdminSession[]): readonly AdminSession[] {
  const converts = (s: AdminSession) => Object.values(s.decision).some(d => d?.action === 'transcode' || d?.action === 'burn');
  return [...sessions].sort((a, b) => Number(!!b.problem) - Number(!!a.problem) || Number(converts(b)) - Number(converts(a)) || Date.parse(b.startedAt ?? '') - Date.parse(a.startedAt ?? '') || a.id.localeCompare(b.id));
}
export type DashboardRoute = 'original' | 'repackaged' | 'converted';
/** VS-10 per-track display classification only. Never rewrites a raw action or establishes an overall session route. */
export function trackRoute(action?: string): DashboardRoute | undefined {
  if (action === 'original' || action === 'direct') return 'original';
  if (action === 'copy') return 'repackaged';
  if (action === 'convert' || action === 'transcode' || action === 'burn_in' || action === 'burn') return 'converted';
  return undefined;
}
export type Track = Readonly<{reportedAction?: string; route?: DashboardRoute; kind: 'video' | 'audio' | 'subtitles'; source: string; action: string; output?: string; hardware: boolean}>;
export function sessionTracks(s: AdminSession): readonly Track[] {
  const tracks: Track[] = [], v = s.media?.video, a = s.media?.audio, sub = s.media?.subtitles;
  for (const kind of ['video', 'audio', 'subtitles'] as const) {
    const source = kind === 'video' && v ? formatVideoSource(v)
      : kind === 'audio' && a ? formatAudioSource(a, s.item?.kind === 'song')
      : kind === 'subtitles' && sub ? formatSubtitleSource(sub) : '';
    const delivery = s.delivery?.streams.find(t => t.kind === kind || kind === 'subtitles' && t.kind === 'subtitle');
    const decision = s.decision[kind], reported = delivery?.action ?? decision?.action;
    const action = reported === 'original' ? 'direct' : reported === 'convert' ? 'transcode' : reported === 'burn_in' ? 'burn' : reported === 'drop' ? 'off' : reported;
    if (!source && !action) continue;
    tracks.push({reportedAction: reported, route: trackRoute(reported), kind, source: source || (delivery?.inputCodec ? formatCodec(delivery.inputCodec) : ''), action: action ?? 'unknown', output: formatDeliveryTarget(s, kind, delivery?.outputCodec || decision?.to), hardware: kind === 'video' && s.media?.videoHardware === true && (action === 'transcode' || action === 'burn')});
  }
  return tracks;
}
export function quietestHours(grid: readonly (readonly number[])[]): number | undefined {
  if (grid.length !== 7 || grid.some(day => day.length !== 24) || !grid.some(day => day.some(n => n > 0))) return undefined;
  let best = 0, min = Infinity;
  for (let hour = 0; hour <= 21; hour++) {
    const sum = grid.reduce((total, day) => total + day[hour] + day[hour + 1] + day[hour + 2], 0);
    if (sum < min) {min = sum; best = hour;}
  }
  return best;
}
export type PlotGeometry = Readonly<{left: number; top: number; width: number; height: number}>;
/** Monotone cubic segments copied from checked settings/dashboard.js. This draws between actual samples only. */
export function curveSegments(pts: readonly (readonly [number, number])[]): string {
  const n = pts.length; if (n < 2) return '';
  const dx: number[] = [], m: number[] = [], tg: number[] = [], f1 = (v: number) => v.toFixed(1);
  for (let i = 0; i < n - 1; i++) {dx.push(pts[i + 1][0] - pts[i][0]); m.push((pts[i + 1][1] - pts[i][1]) / dx[i]);}
  tg[0] = m[0]; tg[n - 1] = m[n - 2];
  for (let i = 1; i < n - 1; i++) tg[i] = m[i - 1] * m[i] <= 0 ? 0 : 2 / (1 / m[i - 1] + 1 / m[i]);
  let d = '';
  for (let i = 0; i < n - 1; i++) {const k = dx[i] / 3; d += `C${f1(pts[i][0] + k)},${f1(pts[i][1] + tg[i] * k)} ${f1(pts[i + 1][0] - k)},${f1(pts[i + 1][1] - tg[i + 1] * k)} ${f1(pts[i + 1][0])},${f1(pts[i + 1][1])}`;}
  return d;
}
export function plotLine(pts: readonly (readonly [number, number])[], step = false): string {
  if (!pts.length) return '';
  const f1 = (v: number) => v.toFixed(1);
  let d = `M${f1(pts[0][0])},${f1(pts[0][1])}`;
  if (!step) return d + curveSegments(pts);
  for (let i = 1; i < pts.length; i++) d += `H${f1(pts[i][0])}V${f1(pts[i][1])}`;
  return d;
}
/** No synthesized zeroes or line across an unavailable sample. Geometry matches the drawing. */
export function chartPaths(points: readonly TelemetryPoint[], from: number, until: number, max: number, step = false, geometry: PlotGeometry = {left: 64, top: 8, width: 840, height: 216}): readonly string[] {
  if (!points.length || until <= from || max <= 0) return [];
  const intervals = points.slice(1).map((p, i) => p.t - points[i].t).filter(n => n > 0).sort((a, b) => a - b);
  const cadence = intervals[Math.floor(intervals.length / 2)] ?? Infinity;
  const paths: string[] = []; let chunk: [number, number][] = [], last: TelemetryPoint | undefined;
  for (const p of points) {
    if (!Number.isFinite(p.v) || p.t < from || p.t > until) continue;
    const x = geometry.left + (p.t - from) / (until - from) * geometry.width, y = geometry.top + geometry.height - Math.max(0, p.v) / max * geometry.height;
    const gap = last && p.t - last.t > cadence * 2.5;
    if (gap && chunk.length) {paths.push(plotLine(chunk, step)); chunk = [];}
    chunk.push([x, y]);
    last = p;
  }
  if (chunk.length) paths.push(plotLine(chunk, step)); return paths;
}
export function bucketSeries(history: DashboardHistory, kind: string): readonly TelemetryPoint[] {
  return history.buckets.map(bucket => ({t: Date.parse(bucket.start), v: bucket.kinds.find(k => k.kind === kind)?.watchedMs ?? 0}));
}
export const hours = (milliseconds: number) => milliseconds / 3_600_000;
export function clock(milliseconds: number): string {
  const s = Math.floor(Math.max(0, milliseconds) / 1000); return `${Math.floor(s / 3600) ? `${Math.floor(s / 3600)}:` : ''}${Math.floor(s / 3600) ? String(Math.floor(s / 60) % 60).padStart(2, '0') : Math.floor(s / 60) % 60}:${String(s % 60).padStart(2, '0')}`;
}
