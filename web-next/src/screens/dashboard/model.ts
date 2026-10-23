import type {DashboardSession as AdminSession} from './bindings';
import type {DashboardHistory} from '@core/admin/dashboard.ts';
import type {TelemetryPoint} from '@core/telemetry.ts';

export const mediaKinds = ['movies', 'tv', 'music', 'audiobooks', 'live'] as const;
export type MediaKind = typeof mediaKinds[number];
export function orderedSessions(sessions: readonly AdminSession[]): readonly AdminSession[] {
  const converts = (s: AdminSession) => Object.values(s.decision).some(d => d?.action === 'transcode' || d?.action === 'burn');
  return [...sessions].sort((a, b) => Number(!!b.problem) - Number(!!a.problem) || Number(converts(b)) - Number(converts(a)) || Date.parse(b.startedAt ?? '') - Date.parse(a.startedAt ?? '') || a.id.localeCompare(b.id));
}
export type Track = Readonly<{kind: 'video' | 'audio' | 'subtitles'; source: string; action: string; output?: string; hardware: boolean}>;
export function sessionTracks(s: AdminSession): readonly Track[] {
  const tracks: Track[] = [], v = s.media?.video, a = s.media?.audio, sub = s.media?.subtitles;
  for (const kind of ['video', 'audio', 'subtitles'] as const) {
    const source = kind === 'video' && v ? [v.height && `${v.height}p`, v.codec, v.profile, v.hdr].filter(Boolean).join(' ')
      : kind === 'audio' && a ? [a.language, a.codec, a.channels && String(a.channels)].filter(Boolean).join(' ')
      : kind === 'subtitles' && sub ? [sub.language, sub.format].filter(Boolean).join(' ') : '';
    const delivery = s.delivery?.streams.find(t => t.kind === kind || kind === 'subtitles' && t.kind === 'subtitle');
    const decision = s.decision[kind], reported = delivery?.action ?? decision?.action;
    const action = reported === 'original' ? 'direct' : reported === 'convert' ? 'transcode' : reported === 'burn_in' ? 'burn' : reported === 'drop' ? 'off' : reported;
    if (!source && !action) continue;
    tracks.push({kind, source: source || delivery?.inputCodec || '', action: action ?? 'unknown', output: delivery?.outputCodec || decision?.to, hardware: kind === 'video' && s.media?.videoHardware === true && (action === 'transcode' || action === 'burn')});
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
/** No synthesized zeroes or interpolation across an unavailable sample. */
export function chartPaths(points: readonly TelemetryPoint[], from: number, until: number, max: number, step = false): readonly string[] {
  if (!points.length || until <= from || max <= 0) return [];
  const intervals = points.slice(1).map((p, i) => p.t - points[i].t).filter(n => n > 0).sort((a, b) => a - b);
  const cadence = intervals[Math.floor(intervals.length / 2)] ?? Infinity;
  const paths: string[] = []; let path = '', last: TelemetryPoint | undefined;
  for (const p of points) {
    if (!Number.isFinite(p.v) || p.t < from || p.t > until) continue;
    const x = 48 + (p.t - from) / (until - from) * 816, y = 210 - Math.max(0, p.v) / max * 190;
    const gap = last && p.t - last.t > cadence * 2.5;
    if (gap && path) {paths.push(path); path = '';}
    path += !path ? `M${x},${y}` : step ? `H${x}V${y}` : `L${x},${y}`;
    last = p;
  }
  if (path) paths.push(path); return paths;
}
export function bucketSeries(history: DashboardHistory, kind: string): readonly TelemetryPoint[] {
  return history.buckets.map(bucket => ({t: Date.parse(bucket.start), v: bucket.kinds.find(k => k.kind === kind)?.watchedMs ?? 0}));
}
export const hours = (milliseconds: number) => milliseconds / 3_600_000;
export function clock(milliseconds: number): string {
  const s = Math.floor(Math.max(0, milliseconds) / 1000); return `${Math.floor(s / 3600) ? `${Math.floor(s / 3600)}:` : ''}${String(Math.floor(s / 60) % 60).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`;
}
