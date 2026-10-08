import {TelemetryPanel} from './OverviewCharts';
import type {Alert, Measurement} from '@core/console.ts';
import {STATE_PERMISSIONS_CODE} from '@core/administration.ts';
import {formatBytes} from '@core/presentation/index.ts';
import {healthFacts} from '@core/server-admin/dashboard.ts';
import {StatePermissionsCard} from './StatePermissions';
import {alertLabel, alertTone, duration, sentence, useAction, useConsole, useRead, when} from '../../admin/console';
import {Badge, Button, Freshness, Notice, Status, Text} from '../../ui';
import {useI18n} from '../../app/i18n';
import s from './Server.module.css';
import page from '../settings/Settings.module.css';

/**
 * Dashboard › Health: one sentence on how the server is doing, the few facts behind it, and one
 * row of tiles (processor, memory, graphics, network, disk) with their recent history. One
 * refresh for the page covers it; a figure the server can't measure says so once.
 */
export function HealthPanel() {
  const client = useConsole();
  const {t} = useI18n();
  const health = useRead<Measurement>(() => client.measurement('health'), [client], {every: 30000});
  const storage = useRead<Measurement>(() => client.measurement('storage'), [client], {every: 60000});
  const {degraded, uptimeSeconds: uptime, freeBytes: free, databaseBytes: database} = healthFacts(health.data, storage.data);
  const lead = (
    <div style={{padding: '4px 16px 16px', display: 'flex', flexDirection: 'column', gap: 8}}>
      {/* Reports to the page's one refresh. */}
      <Freshness at={health.data?.observedAt} staleAfterMs={90_000} onRefresh={() => { health.reload(); storage.reload(); }} refreshing={health.loading || storage.loading} verb="Updated" />
      {health.error && !health.data ? <Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: health.reload}}>{health.error}</Notice> : null}
      {health.data ? <div className={page.statusLine}><Status tone={degraded ? 'warning' : 'healthy'}>{degraded ? t('settings.server.health.attention') : t('settings.server.health.running')}</Status></div> : null}
      <div className={page.statusFacts}>
        {uptime !== undefined ? <span>{t('settings.server.health.upFor', {duration: duration(uptime)})}</span> : null}
        {free !== undefined ? <span>{t('settings.server.health.free', {bytes: formatBytes(free)})}</span> : null}
        {database !== undefined ? <span>{t('settings.server.health.database', {bytes: formatBytes(database)})}</span> : null}
      </div>
    </div>
  );
  return <TelemetryPanel title={t('settings.server.health')} lead={lead} />;
}

export function AlertsPanel() {
  const client = useConsole();
  const {t} = useI18n();
  const read = useRead<Alert[]>(() => client.alerts(), [client], {every: 60000});
  const action = useAction();
  const open = (read.data ?? []).filter(a => a.status !== 'resolved');
  // The state-permissions warning (§6.2) renders as its own card with the explanation and
  // the owner-only fix, instead of a generic alert row. It appears only while the server
  // reports the item.
  const permissions = open.find(a => a.code === STATE_PERMISSIONS_CODE);
  const rest = permissions ? open.filter(a => a !== permissions) : open;
  if (read.error && !read.data) return <Notice tone="error" action={{label: t('action.tryAgain'), onClick: read.reload}}>Alerts couldn’t be loaded. {read.error}</Notice>;
  if (!read.data) return null;
  if (!open.length) return <Notice tone="success" icon="success">{t('web.overview.noAlerts')}</Notice>;
  return (
    <div style={{display: 'flex', flexDirection: 'column', gap: 8}}>
      {permissions ? <StatePermissionsCard alert={permissions} /> : null}
      {rest.map(a => (
        <div key={a.id} className={s.alert}>
          <Badge tone={alertTone(a)} dot>{sentence(a.severity)}</Badge>
          <div className={s.alertBody}>
            <Text variant="bodyStrong">{alertLabel(a)}</Text>
            <Text variant="caption" tone="tertiary">{[a.occurrences > 1 ? t('server.alert.seen', {count: a.occurrences, when: when(a.lastAt)}) : t('server.alert.firstSeen', {when: when(a.firstAt)}), a.status === 'acknowledged' ? t('server.alert.acknowledged') : ''].filter(Boolean).join(' · ')}</Text>
            <details className={s.details}><summary>{t('server.technicalDetails')}</summary><dl><dt>Code</dt><dd>{a.code}</dd><dt>Alert</dt><dd>{a.id}</dd></dl></details> {/* lint-strings-allow: technical console readout (alert code/id field labels) */}
          </div>
          {a.status === 'open' ? <Button size="sm" variant="outline" label={t('web.overview.acknowledge')} loading={action.busy} onClick={() => void action.run(() => client.acknowledge(a)).then(ok => ok && read.reload())} /> : null}
        </div>
      ))}
      {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
    </div>
  );
}

