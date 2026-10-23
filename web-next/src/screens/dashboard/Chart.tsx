import {useId, useState} from 'react';
import {defaultI18n} from '@i18n';
import type {ReportInterval} from '@core/report-context.ts';
import type {TelemetryPoint} from '@core/telemetry.ts';
import {formatLoadAxis, loadScale, type DashboardLoadKind} from './format';
import {chartPaths, loadAxisTicks} from './model';
import styles from './dashboard.module.css';
export type ChartSeries = Readonly<{key: string; label: string; points: readonly TelemetryPoint[]}>;
const t = defaultI18n.t;
/** Keyboard and pointer inspect the same actual sample. Missing samples stay gaps. */
export function Chart({title, series, format, formatAverage = format, step = false, compact = false, kind = 'bandwidth', interval}: {interval: ReportInterval; kind?: DashboardLoadKind; title: string; series: readonly ChartSeries[]; format: (value: number) => string; formatAverage?: (value: number) => string; step?: boolean; compact?: boolean}) {
  const id = useId().replace(/:/g, ''), [selected, select] = useState<number | undefined>();
  const stamps = [...new Set(series.flatMap(s => s.points.map(p => p.t)))].sort((a, b) => a - b);
  if (!stamps.length) return null;
  const from = stamps[0], until = stamps.at(-1) === from ? from + 2000 : stamps.at(-1)!;
  const actualMax = Math.max(0, ...series.flatMap(s => s.points.map(p => p.v)));
  const scale = loadScale(kind, actualMax), max = compact ? kind === 'memory' ? Math.max(60, actualMax * 1.15) : kind === 'conversions' ? Math.max(3.2, actualMax) : Math.max(1, actualMax * (kind === 'bandwidth' ? 1.12 : 1.15)) : scale.top;
  const geometry = compact ? {left: 3, top: 3, width: 191, height: 34} : {left: kind === 'bandwidth' ? 64 : 40, top: 8, width: kind === 'bandwidth' ? 840 : 864, height: 216};
  const x = (stamp: number) => geometry.left + (stamp - from) / (until - from) * geometry.width;
  const y = (value: number) => geometry.top + geometry.height - value / max * geometry.height;
  const active = selected === undefined ? undefined : stamps[Math.min(stamps.length - 1, selected)];
  const tipTitle = (stamp: number) => interval.period === 'live' ? stamp === until ? t('next.dashboard.axis.Now') : t('next.dashboard.secondsAgo', {seconds: Math.round((until - stamp) / 1000)}) : new Intl.DateTimeFormat(defaultI18n.locale, {dateStyle: 'medium', timeStyle: 'short', timeZone: interval.timeZone}).format(stamp);
  return <div className={compact ? styles.spark : styles.plot} tabIndex={compact ? undefined : 0} role={compact ? undefined : 'group'} aria-label={compact ? undefined : title} aria-describedby={compact ? undefined : id}
    onPointerMove={compact ? undefined : event => {const rect = event.currentTarget.getBoundingClientRect(); select(Math.max(0, Math.min(stamps.length - 1, Math.round(((event.clientX - rect.left) / rect.width * 912 - geometry.left) / geometry.width * (stamps.length - 1)))));}}
    onPointerLeave={() => select(undefined)} onBlur={() => select(undefined)} onKeyDown={event => {
      if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
      event.preventDefault(); select(old => event.key === 'Home' ? 0 : event.key === 'End' ? stamps.length - 1 : Math.max(0, Math.min(stamps.length - 1, (old ?? (event.key === 'ArrowLeft' ? stamps.length : -1)) + (event.key === 'ArrowLeft' ? -1 : 1))));
    }}>
    <svg viewBox={compact ? '0 0 197 40' : '0 0 912 250'} role="img" aria-label={title} className={styles.chart}>
      <defs>{series.map(s => <linearGradient data-series={s.key} key={s.key} id={`${id}-${s.key}`} x1="0" y1="0" x2="0" y2="1"><stop offset="0" stopColor="currentColor" stopOpacity={compact ? .22 : .16} /><stop offset="1" stopColor="currentColor" stopOpacity="0" /></linearGradient>)}</defs>
      {!compact && scale.ticks.map(value => <g key={value}><path className={styles.gridline} d={`M${geometry.left},${Math.round(y(value)) + .5}H904`} /><text x={geometry.left - 10} y={Math.round(y(value)) + 4.5} textAnchor="end">{formatLoadAxis(value, kind)}</text></g>)}
      {series.map((s, si) => <g key={s.key} data-series={s.key} className={styles.series}>{chartPaths(s.points, from, until, max, step, geometry).map((path, i) => {
        const first = /^M([\d.-]+),/.exec(path)?.[1], last = /(?:C[^C]* |H|L)([\d.-]+)[,V][\d.-]+$/.exec(path)?.[1] ?? first;
        return <g key={i}>{(si === 0 || !compact && kind === 'bandwidth') && <path className={styles.lineFill} style={{fill: `url(#${id}-${s.key})`}} d={`${path}L${last},${compact ? 40 : 224}L${first},${compact ? 40 : 224}Z`} />}<path d={path} /></g>;
      })}{(compact || s.points.length === 1 || active !== undefined) && (() => {const point = active === undefined ? s.points.at(-1) : s.points.find(p => p.t === active); return point && <circle cx={x(point.t)} cy={y(point.v)} r={active === undefined ? 3 : 4.5} />;})()}</g>)}
      {!compact && loadAxisTicks(interval, from, until, geometry.width).map(tick => <text key={tick.f} x={(geometry.left + geometry.width * tick.f).toFixed(1)} y="244" textAnchor={tick.f < .02 ? 'start' : tick.f > .98 ? 'end' : 'middle'}>{tick.text}</text>)}
      {active !== undefined && <path className={styles.guide} d={`M${x(active)},8V224`} />}
    </svg>
    {!compact && <><div id={id} className={styles.chartTip} aria-live="polite">{active !== undefined && <><b>{tipTitle(active)}</b>{series.map(s => {const point = s.points.find(p => p.t === active); return point && <span key={s.key} data-series={s.key}><i />{s.label}<strong>{format(point.v)}</strong></span>;})}</>}</div>
      <div className={styles.legend}>{series.map(s => <span key={s.key} data-series={s.key}><i />{s.label}</span>)}<small className={styles.averages}>{t('next.dashboard.average')}{series.filter(s => s.points.length).map(s => <b key={s.key} data-series={s.key}>{s.label} {formatAverage(s.points.reduce((sum, p) => sum + p.v, 0) / s.points.length)}</b>)}</small></div></>}
  </div>;
}
