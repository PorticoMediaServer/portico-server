import {defaultI18n, type MessageId} from '@i18n';
import type {DashboardSession} from './bindings';
import type {Track} from './model';

const t = defaultI18n.t;
export type DashboardLoadKind = 'bandwidth' | 'processor' | 'memory' | 'conversions';
export type ChartScale = Readonly<{top: number; ticks: readonly number[]}>;
/** settings/dashboard.js:169–176, copied with TypeScript annotations only. */
export function niceScale(max: number, target = 4): ChartScale {
  if (!(max > 0)) return { top: 1, ticks: [0, 1] };
  const raw = max / target, p = Math.pow(10, Math.floor(Math.log10(raw))), f = raw / p;
  const step = (f <= 1 ? 1 : f <= 2 ? 2 : f <= 2.5 ? 2.5 : f <= 5 ? 5 : 10) * p, top = Math.ceil(max / step - 1e-9) * step, ticks = [];
  for (let v = 0; v <= top + 1e-9; v += step) ticks.push(+v.toFixed(6));
  return { top, ticks };
}
/** paint.detail: the same range scale and integer conversion ticks as the drawing. */
export function loadScale(kind: DashboardLoadKind, max: number): ChartScale {
  return kind === 'conversions'
    ? {top: Math.max(3, Math.ceil(max)), ticks: Array.from({length: Math.max(3, Math.ceil(max)) + 1}, (_, i) => i)}
    : niceScale(max * (kind === 'bandwidth' ? 1.06 : 1.12), 4);
}
/** Drawing fmtMbps/fmtPct/fmtDur/fmtHrs; only their unit text goes through the catalogue. */
export const fmtMbps = (v: number): string => t('next.dashboard.mbps', {value: v >= 100 ? String(Math.round(v)) : v.toFixed(1)});
export const formatStreamRate = (kbps: number): string => kbps < 1000 ? t('next.dashboard.format.kbps', {value: String(Math.round(kbps))}) : t('next.dashboard.mbps', {value: String(Number((kbps / 1000).toFixed(1)))});
export const fmtPct = (v: number): string => t('next.dashboard.percent', {value: v.toFixed(1)});
export function fmtDur(h: number): string {
  if (h >= 100) return t('next.dashboard.hours', {value: String(Math.round(h))});
  const m = Math.round(h * 60);
  if (m < 60) return t('next.dashboard.format.minutes', {minutes: String(m)});
  return t('next.dashboard.format.hoursMinutes', {hours: String(Math.floor(m / 60)), minutes: String(m % 60).padStart(2, '0')});
}
export const fmtHrs = (h: number): string => t('next.dashboard.hours', {value: String(h >= 10 ? Math.round(h) : Math.round(h * 10) / 10)});
/** History's human lengths: whole-minute labels follow ruling 7, short exact totals retain their seconds. */
export function formatHistoryDuration(seconds: number, exactShort = false): string {
  const total = Math.max(0, Math.floor(seconds)), minutes = Math.round(total / 60);
  if (exactShort && total < 300 && total % 60) return t('next.dashboard.format.minutesSeconds', {minutes: Math.floor(total / 60), seconds: total % 60});
  return minutes >= 60 ? t('next.dashboard.format.historyHoursMinutes', {hours: Math.floor(minutes / 60), minutes: minutes % 60}) : t('next.dashboard.format.runtimeMinutes', {minutes});
}
/** Date presentation only. Bucket boundaries always come from the server. */
export function formatHistoryStarted(startedAt: string, timeZone: string, capturedUntil: string): string {
  const stamp = Date.parse(startedAt), captured = Date.parse(capturedUntil);
  const day = new Intl.DateTimeFormat(defaultI18n.locale, {year: 'numeric', month: 'numeric', day: 'numeric', timeZone});
  const time = new Intl.DateTimeFormat(defaultI18n.locale, {hour: 'numeric', minute: '2-digit', timeZone}).format(stamp);
  return day.format(stamp) === day.format(captured) ? t('next.dashboard.format.todayAt', {time}) : new Intl.DateTimeFormat(defaultI18n.locale, {month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit', timeZone}).format(stamp);
}
/** These are already-local heat-map column ordinals, never timestamps to convert to UTC. */
export function formatHourOrdinal(hour: number): string {
  return new Intl.DateTimeFormat(defaultI18n.locale, {hour: 'numeric', timeZone: 'UTC'}).format(Date.UTC(2026, 0, 1, hour));
}
export function formatQuietHours(start: number): string {
  const format = new Intl.DateTimeFormat(defaultI18n.locale, {hour: 'numeric', timeZone: 'UTC'});
  const from = format.formatToParts(Date.UTC(2026, 0, 1, start)), until = format.formatToParts(Date.UTC(2026, 0, 1, start + 3));
  const period = from.find(p => p.type === 'dayPeriod')?.value, same = period && period === until.find(p => p.type === 'dayPeriod')?.value;
  return t('next.dashboard.quietWindow', {from: same ? from.filter(p => p.type !== 'dayPeriod').map(p => p.value).join('').trim() : formatHourOrdinal(start), until: formatHourOrdinal(start + 3)});
}
export function formatLoadAxis(value: number, kind: DashboardLoadKind): string {
  if (value === 0 || kind === 'conversions') return String(value);
  return t(kind === 'bandwidth' ? 'next.dashboard.mbps' : 'next.dashboard.percent', {value: String(value)});
}
/** Source strings in SESSIONS:284–305. Codec identity is not a delivery-quality ceiling. */
const codecIds: Readonly<Record<string, MessageId>> = {
  h264: 'next.kit.codec.h264', avc: 'next.kit.codec.h264', hevc: 'next.kit.codec.hevc', h265: 'next.kit.codec.hevc',
  aac: 'next.kit.codec.aac', truehd: 'next.dashboard.format.truehd', eac3: 'next.dashboard.format.eac3',
  flac: 'next.kit.codec.flac', pgs: 'next.kit.codec.pgs', hdmv_pgs_subtitle: 'next.kit.codec.pgs', srt: 'next.kit.codec.srt', subrip: 'next.kit.codec.srt',
};
export const formatCodec = (codec: string): string => codecIds[codec.toLowerCase()] ? t(codecIds[codec.toLowerCase()]) : codec.toUpperCase();
export function formatLanguage(code?: string): string | undefined {
  if (!code || code === 'und') return undefined;
  try {return new Intl.DisplayNames([defaultI18n.locale], {type: 'language', fallback: 'none'}).of(code);} catch {return undefined;}
}
export const formatResolution = (height?: number): string | undefined => height && height > 0 ? height === 2160 ? t('next.kit.4k') : t('quality.height', {height}) : undefined;
function sourceChannels(audio: NonNullable<NonNullable<DashboardSession['media']>['audio']>): string | undefined {
  const layout = /^(\d+\.\d+)(?:\([^)]*\))?$/.exec(audio.layout ?? '')?.[1];
  if (layout) return layout;
  if (audio.layout === 'stereo' || audio.channels === 2) return '2.0';
  // A channel count alone cannot establish the LFE layout of a multichannel source.
  return undefined;
}
export function formatVideoSource(video: NonNullable<NonNullable<DashboardSession['media']>['video']>): string {
  const hdr = video.hdr && video.hdr.toLowerCase() !== 'sdr' ? video.hdr.toUpperCase() : undefined;
  const profile = !hdr && video.profile?.toLowerCase() === 'main 10' ? t('next.dashboard.format.main10') : undefined;
  return [formatResolution(video.height), formatCodec(video.codec), hdr ?? profile].filter(Boolean).join(' ');
}
export function formatAudioSource(audio: NonNullable<NonNullable<DashboardSession['media']>['audio']>, music: boolean): string {
  if (music && audio.codec.toLowerCase() === 'flac') return [formatCodec(audio.codec),
    audio.bitDepth && t('next.dashboard.format.bitDepth', {bits: audio.bitDepth}),
    audio.sampleRate && t('next.dashboard.format.sampleRate', {rate: audio.sampleRate / 1000})].filter(Boolean).join(' ');
  return [formatLanguage(audio.language), formatCodec(audio.codec), sourceChannels(audio)].filter(Boolean).join(' ');
}
export function formatSubtitleSource(subtitle: NonNullable<NonNullable<DashboardSession['media']>['subtitles']>): string {
  return [formatLanguage(subtitle.language), formatCodec(subtitle.format)].filter(Boolean).join(' ');
}
export function formatDeliveryTarget(session: DashboardSession, kind: 'video' | 'audio' | 'subtitles', codec?: string): string | undefined {
  if (kind === 'video') return formatResolution(session.delivery?.videoHeight);
  if (!codec) return undefined;
  if (kind === 'audio') return [formatCodec(codec), (session.delivery?.audioChannels ?? session.decision.audio?.channels) === 2 ? t('next.dashboard.format.stereo') : undefined].filter(Boolean).join(' ');
  return formatCodec(codec);
}
/** Checked settings/dashboard.js sessionHTML: a route belongs to this track, never an inferred session total. */
export function formatTrackRoute(track: Track): string | undefined {
  if (track.route === 'original') return t('next.dashboard.original');
  if (track.route === 'repackaged') return t('next.dashboard.copy');
  if (track.route !== 'converted') return undefined;
  if (track.kind === 'subtitles' && track.action === 'burn') return t('next.dashboard.burn');
  return track.output ? t('next.dashboard.convert', {target: track.output}) : t('next.dashboard.converted');
}
