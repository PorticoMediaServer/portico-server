import React, {useMemo, useState} from 'react';
import type {PlaybackHistoryPeriod, TelemetryPoint, TelemetryWindow} from '@core/admin/index.ts';
import {loadValue, measuredLoadTiles} from '@core/server-admin/dashboard.ts';
import {VIEWING_PERIODS, fetchPlaySummary, viewingFigures, viewingPeriodLabel, type ViewingPeriod} from '@core/play-history.ts';
import {useSession} from '../../app/session';
import {useConsole, useRead} from '../../admin/console';
import {useI18n} from '../../app/i18n';
import {Freshness, Notice, Segmented, Surface, Text} from '../../ui';
import s from './Server.module.css';

/** A plain SVG line: no chart library for six small lines. */
function Spark({points, max}: {points: readonly TelemetryPoint[]; max: number}) {
  if (points.length < 2) return <div style={{height: 44}} />;
  const t0 = points[0].t, span = Math.max(1, points[points.length - 1].t - t0), top = Math.max(max, 1e-9);
  const path = points.map((p, i) => `${i ? 'L' : 'M'}${(((p.t - t0) / span) * 100).toFixed(2)},${(40 - Math.min(1, p.v / top) * 38).toFixed(2)}`).join(' ');
  return (
    <svg viewBox="0 0 100 42" preserveAspectRatio="none" style={{width: '100%', height: 44, display: 'block'}} aria-hidden>
      <path d={`${path} L100,42 L0,42 Z`} fill="var(--accent-tint)" stroke="none" />
      <path d={path} fill="none" stroke="var(--accent)" strokeWidth={1.4} vectorEffect="non-scaling-stroke" />
    </svg>
  );
}

/** Server load over time. A metric this platform cannot measure says so; it is never drawn as zero. */
export function TelemetryPanel({title, lead}: {title?: string; lead?: React.ReactNode} = {}) {
  const client = useConsole();
  const {t} = useI18n();
  const [window, setWindow] = useState<TelemetryWindow>('1h');
  const read = useRead(() => client.telemetry(window), [client, window], {every: window === '10m' ? 15000 : 60000});
  const data = read.data;
  const tiles = data ? measuredLoadTiles(data.status) : [];
  return (
    <Surface padless>
      <div className={s.panelHead} style={{padding: '16px 16px 0', flexWrap: 'wrap'}}>
        <span className={s.panelTitle}>{title ?? t('web.telemetry.load')}</span>
        <div style={{display: 'flex', gap: 12, alignItems: 'center'}}>
          <Segmented size="sm" label={t('web.telemetry.period')} value={window} onChange={v => setWindow(v as TelemetryWindow)} options={[{id: '10m', label: t('web.telemetry.window10m')}, {id: '1h', label: t('web.telemetry.window1h')}, {id: '24h', label: t('web.telemetry.window24h')}]} />
          <Freshness at={read.at} onRefresh={read.reload} refreshing={read.loading} />
        </div>
      </div>
      {lead}
      {read.error && !data ? <div style={{padding: 16}}><Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></div> : null}
      {/* Only what this server measures: a metric it cannot read has no tile (the iPhone app does the same). */}
      {data && tiles.length ? (
        <div style={{display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 2, background: 'var(--line-soft)'}}>
          {tiles.map(c => {
            const status = data.status[c.metric], series = data.series[c.metric] ?? [];
            const peak = c.unit === 'percent' ? 100 : Math.max(...series.map(p => p.v), 1);
            return (
              <div key={c.metric} style={{background: 'var(--surface-raised, var(--surface))', padding: '12px 16px'}}>
                <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', gap: 8}}>
                  <Text variant="caption" tone="secondary">{t(c.label)}</Text>
                  <Text variant="bodyStrong">{loadValue(c, status)}</Text>
                </div>
                <Spark points={series} max={peak} />
              </div>
            );
          })}
        </div>
      ) : null}
      {data && !tiles.length ? <div style={{padding: '0 16px 16px'}}><Text variant="caption" tone="tertiary">{t('server.health.loadNotMeasured')}</Text></div> : null}
    </Surface>
  );
}

/**
 * What was watched, by whom, and how it was delivered: totals of the play history for a period.
 * The same plays the Play history page lists, kept for as long as the owner keeps that history.
 */
export function ViewingPanel() {
  const {api} = useSession();
  const {t} = useI18n();
  const [period, setPeriod] = useState<ViewingPeriod>('24h');
  const read = useRead(() => fetchPlaySummary(api, period), [api, period]);
  const figures = read.data ? viewingFigures(read.data) : undefined;
  return (
    <Surface padless>
      <div className={s.panelHead} style={{padding: '16px 16px 0', flexWrap: 'wrap'}}>
        <span className={s.panelTitle}>{t('web.telemetry.viewing')}</span>
        <div style={{display: 'flex', gap: 12, alignItems: 'center'}}>
          <Segmented size="sm" label={t('web.telemetry.period')} value={period} onChange={v => setPeriod(v as ViewingPeriod)} options={VIEWING_PERIODS.map(id => ({id, label: t(viewingPeriodLabel(id))}))} />
          <Freshness at={read.at} onRefresh={read.reload} refreshing={read.loading} />
        </div>
      </div>
      {read.error && !read.data ? <div style={{padding: 16}}><Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></div> : null}
      {read.data && figures ? (
        <div style={{padding: 16, display: 'flex', flexDirection: 'column', gap: 16}}>
          <div style={{display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(120px, 1fr))', gap: 12}}>
            {[[t('server.viewing.plays'), figures.plays], [t('server.viewing.people'), figures.people], [t('server.viewing.hours'), figures.hours], [t('server.viewing.converted'), figures.converted]].map(([label, value]) => (
              <div key={label}><Text as="p" variant="caption" tone="tertiary">{label}</Text><Text as="p" variant="heading">{value}</Text></div>
            ))}
          </div>
          {read.data.plays ? (
            <div style={{display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: 20}}>
              {([[t('server.viewing.mostPlayed'), read.data.mostPlayed], [t('server.viewing.mostActive'), read.data.mostActive]] as const).map(([heading, rows]) => (
                <div key={heading}>
                  <Text as="p" variant="label" tone="tertiary" style={{marginBottom: 8}}>{heading}</Text>
                  {rows.map(row => <div key={row.name} style={{display: 'flex', justifyContent: 'space-between', gap: 12, padding: '4px 0'}}><Text variant="caption" clamp={1}>{row.name}</Text><Text variant="caption" tone="tertiary">{row.plays}</Text></div>)}
                </div>
              ))}
            </div>
          ) : <Text variant="caption" tone="tertiary">{t('web.telemetry.noViewing')}</Text>}
        </div>
      ) : null}
    </Surface>
  );
}
