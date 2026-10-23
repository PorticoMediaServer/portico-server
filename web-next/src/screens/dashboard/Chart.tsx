import {useId, useState} from 'react';
import {defaultI18n} from '@i18n';
import type {TelemetryPoint} from '@core/telemetry.ts';
import {chartPaths} from './model';
import styles from './dashboard.module.css';
export type ChartSeries = Readonly<{key: string; label: string; points: readonly TelemetryPoint[]}>;
const t = defaultI18n.t;
/** Keyboard and pointer inspect the same actual sample. Missing samples stay gaps. */
export function Chart({title, series, format, step = false, compact = false}: {title: string; series: readonly ChartSeries[]; format: (value: number) => string; step?: boolean; compact?: boolean}) {
  const id = useId(), [selected, select] = useState<number | undefined>();
  const stamps = [...new Set(series.flatMap(s => s.points.map(p => p.t)))].sort((a, b) => a - b);
  if (!stamps.length) return compact ? null : <p className={styles.empty}>{t('next.dashboard.unavailable')}</p>;
  const from = stamps[0], until = stamps.at(-1) === from ? from + 2000 : stamps.at(-1)!;
  const max = Math.max(1, ...series.flatMap(s => s.points.map(p => p.v))) * 1.1;
  const active = selected === undefined ? undefined : stamps[Math.min(stamps.length - 1, selected)];
  const time = (stamp: number) => new Intl.DateTimeFormat(defaultI18n.locale, until - from > 86_400_000 ? {month: 'short', day: 'numeric'} : {hour: 'numeric', minute: '2-digit'}).format(stamp);
  return <div className={compact ? styles.spark : styles.plot} tabIndex={compact ? undefined : 0} role={compact ? undefined : 'group'} aria-label={compact ? undefined : title} aria-describedby={compact ? undefined : id}
    onPointerMove={compact ? undefined : event => {const rect = event.currentTarget.getBoundingClientRect(); select(Math.max(0, Math.min(stamps.length - 1, Math.round(((event.clientX - rect.left) / rect.width * 912 - 48) / 816 * (stamps.length - 1)))));}}
    onPointerLeave={() => select(undefined)} onBlur={() => select(undefined)} onKeyDown={event => {
      if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
      event.preventDefault(); select(old => event.key === 'Home' ? 0 : event.key === 'End' ? stamps.length - 1 : Math.max(0, Math.min(stamps.length - 1, (old ?? (event.key === 'ArrowLeft' ? stamps.length : -1)) + (event.key === 'ArrowLeft' ? -1 : 1))));
    }}>
    <svg viewBox={compact ? '48 10 816 210' : '0 0 912 250'} role="img" aria-label={title} className={styles.chart}>
      {!compact && [0, .25, .5, .75, 1].map(f => <g key={f}><path className={styles.gridline} d={`M48,${210 - f * 190}H864`} /><text x="40" y={214 - f * 190} textAnchor="end">{format(f * max)}</text></g>)}
      {series.map(s => <g key={s.key} data-series={s.key} className={styles.series}>{chartPaths(s.points, from, until, max, step).map((path, i) => <path key={i} d={path} />)}{s.points.length === 1 && <circle cx="48" cy={210 - s.points[0].v / max * 190} r="3" />}</g>)}
      {!compact && <><text x="48" y="242">{time(from)}</text><text x="864" y="242" textAnchor="end">{time(until)}</text></>}
      {active !== undefined && <path className={styles.guide} d={`M${48 + (active - from) / (until - from) * 816},20V210`} />}
    </svg>
    {!compact && <><div id={id} className={styles.chartTip} aria-live="polite">{active !== undefined && <><b>{new Intl.DateTimeFormat(defaultI18n.locale, {dateStyle: 'medium', timeStyle: 'short'}).format(active)}</b>{series.map(s => {const point = s.points.find(p => p.t === active); return point && <span key={s.key} data-series={s.key}>{s.label}<strong>{format(point.v)}</strong></span>;})}</>}</div>
      <div className={styles.legend}>{series.map(s => <span key={s.key} data-series={s.key}><i />{s.label}<b>{format(s.points.length ? s.points.reduce((sum, p) => sum + p.v, 0) / s.points.length : 0)}</b></span>)}<small>{t('next.dashboard.average')}</small></div></>}
  </div>;
}
