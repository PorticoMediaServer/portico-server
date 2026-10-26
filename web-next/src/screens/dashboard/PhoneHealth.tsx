import {useId} from 'react';
import {defaultI18n} from '@i18n';
import type {TelemetryReading} from '@core/telemetry.ts';
import {healthPlot, healthTiles} from './health-presentation';
import styles from './phone-health.module.css';

const t = defaultI18n.t;
type HealthRange = '10m' | '1h' | '24h';
const ranges = [
  {value: '10m', label: 'web.telemetry.window10m'},
  {value: '1h', label: 'web.telemetry.window1h'},
  {value: '24h', label: 'web.telemetry.window24h'},
] as const;

/** Presentation only; the caller owns telemetry, its lifetime and range. */
export function PhoneHealth({reading, loading, failed, range, onRangeChange, onRetry}: Readonly<{
  reading?: TelemetryReading; loading: boolean; failed: boolean; range: HealthRange;
  onRangeChange: (range: HealthRange) => void; onRetry: () => void;
}>) {
  const gradient = useId();
  const current = reading?.window === range ? reading : undefined;
  const tiles = healthTiles(current);
  const from = current ? Date.parse(current.from) : NaN, until = current ? Date.parse(current.until) : NaN;
  const shortHistory = tiles.some(tile => tile.history === 'samples' && tile.points.length < 2);
  return <section className={styles.section} aria-labelledby={`${gradient}-title`} aria-busy={loading}>
    <div className={styles.heading}><h2 id={`${gradient}-title`}>{t('settings.server.health')}</h2></div>
    <div className={styles.segment} role="group" aria-label={t('web.telemetry.period')}>
      {ranges.map(item => <button key={item.value} type="button" aria-pressed={range === item.value} onClick={() => onRangeChange(item.value)}>{t(item.label)}</button>)}
    </div>
    {failed && <div className={styles.feedback} role="alert">{t('next.states.pageFailed')} <button type="button" onClick={onRetry}>{t('next.dashboard.retry')}</button></div>}
    {loading && !current && <div className={styles.feedback} role="status">{t('next.settings.loading')}</div>}
    {current && (tiles.length ? <div className={styles.tiles}>{tiles.map(tile => {
      const plot = healthPlot(tile.points, from, until);
      return <div className={styles.tile} key={tile.metric}>
        <div className={styles.key}><span>{t(tile.label)}</span><b>{tile.value}</b></div>
        <div className={`${styles.spark} ${plot ? '' : styles.empty}`}>
          {plot && <svg viewBox="0 0 100 46" preserveAspectRatio="none" aria-hidden="true">
            <defs><linearGradient id={`${gradient}-${tile.metric}`} x1="0" y1="0" x2="0" y2="1"><stop offset="0" stopColor="#409fd4" stopOpacity=".24"/><stop offset="1" stopColor="#409fd4" stopOpacity="0"/></linearGradient></defs>
            <path d={plot.area} fill={`url(#${gradient}-${tile.metric})`}/><path d={plot.line} fill="none" stroke="#5cb3e2" strokeWidth="1.6" vectorEffect="non-scaling-stroke" strokeLinejoin="round" />
          </svg>}
          {plot && <i className={styles.dot} style={{left: `${plot.endX}%`, top: `${plot.endY / 46 * 100}%`}} aria-hidden="true"/>}
        </div>
      </div>;
    })}{shortHistory && <div className={styles.note}>{t('next.dashboard.healthLater')}</div>}</div> : <div className={styles.feedback}>{t('server.health.loadNotMeasured')}</div>)}
  </section>;
}
