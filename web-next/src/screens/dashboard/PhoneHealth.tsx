import {defaultI18n} from '@i18n';
import type {TelemetryReading} from '@core/telemetry.ts';
import {healthPath, healthTiles} from './health-presentation';
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
  const current = reading?.window === range ? reading : undefined;
  const tiles = healthTiles(current);
  const from = current ? Date.parse(current.from) : NaN, until = current ? Date.parse(current.until) : NaN;
  const shortHistory = current && tiles.some(tile => tile.value !== undefined && !healthPath(tile.points, from, until));
  return <section className={styles.section} aria-labelledby="phone-health-title" aria-busy={loading}>
    <div className={styles.heading}><h2 id="phone-health-title">{t('settings.server.health')}</h2></div>
    <div className={styles.segment} role="group" aria-label={t('web.telemetry.period')}>
      {ranges.map(item => <button key={item.value} type="button" aria-pressed={range === item.value} onClick={() => onRangeChange(item.value)}>{t(item.label)}</button>)}
    </div>
    {failed && <div className={styles.feedback} role="alert">{t('next.states.pageFailed')} <button type="button" onClick={onRetry}>{t('next.dashboard.retry')}</button></div>}
    {loading && !current && <div className={styles.feedback} role="status">{t('next.settings.loading')}</div>}
    {current && <div className={styles.tiles}>{tiles.map(tile => {
      const path = healthPath(tile.points, from, until);
      return <div className={styles.tile} key={tile.metric}>
        <div className={styles.key}><span>{t(tile.metric === 'netOut' ? 'next.states.network' : tile.label)}</span><b>{tile.value ?? t('next.kit.browse.unavailable')}</b></div>
        <div className={`${styles.spark} ${path ? '' : styles.empty}`}>
          {path && <svg viewBox="0 0 100 40" preserveAspectRatio="none" aria-hidden="true"><path d={path} fill="none" stroke="currentColor" strokeWidth="1.6" vectorEffect="non-scaling-stroke" strokeLinejoin="round" /></svg>}
        </div>
      </div>;
    })}{shortHistory && <div className={styles.note}>{t('next.dashboard.healthLater')}</div>}</div>}
  </section>;
}
