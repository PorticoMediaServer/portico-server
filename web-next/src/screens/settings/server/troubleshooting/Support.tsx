import {useEffect, useState, useSyncExternalStore} from 'react';
import {defaultI18n, type MessageId} from '@i18n';
import {CAPABILITY_ROWS, capabilityReasonId, type ServerCapability} from '@core/server-admin/capabilities.ts';
import {Button} from '../../../../kit/controls';
import {SettingsAction} from '../../../../kit/settings';
import {SettingsSupportSource, SUPPORT_COMPONENTS, type SupportComponent} from './support-source';
import styles from './troubleshooting.module.css';

const t = defaultI18n.t;

export type SupportPanelProps = Readonly<{
  source: SettingsSupportSource;
  subscribe: (listener: () => void) => () => void;
  reportError: (error: unknown) => void;
}>;

const KNOWN: ReadonlySet<string> = new Set(CAPABILITY_ROWS);
const WINDOWS: readonly Readonly<{id: string; ms: number; label: MessageId}>[] = Object.freeze([
  {id: '1h', ms: 60 * 60 * 1000, label: 'web.logs.window1h'},
  {id: '6h', ms: 6 * 60 * 60 * 1000, label: 'web.logs.window6h'},
  {id: '24h', ms: 24 * 60 * 60 * 1000, label: 'web.logs.window24h'},
  {id: '3d', ms: 3 * 24 * 60 * 60 * 1000, label: 'web.logs.window3d'},
]);
const PART_LABEL: Readonly<Record<SupportComponent, MessageId>> = Object.freeze({
  snapshot: 'web.logs.includeSnapshot', runtime: 'web.logs.includeServer', client: 'web.logs.includeClient', audit: 'web.logs.includeAudit',
});
const capabilityLabel = (capability: string) => KNOWN.has(capability) ? t(`web.capabilities.${capability}` as MessageId) : t('web.capabilities.otherLabel', {id: capability});

/** The drawn Support tab: permissions, what this server can do, the export and what was left out. */
export function SupportPanel(props: SupportPanelProps) {
  const state = useSyncExternalStore(props.subscribe, props.source.getSnapshot, props.source.getSnapshot);
  const [busy, setBusy] = useState(false), [fixed, setFixed] = useState(false);
  const [window, setWindow] = useState('24h'), [parts, setParts] = useState<readonly SupportComponent[]>([...SUPPORT_COMPONENTS]);
  useEffect(() => {void props.source.load();}, [props.source]);
  if (state.phase === 'error') return <p className={styles.failure} role="alert"><span>{t('next.states.pageFailedBody')}</span><button type="button" onClick={() => {void props.source.retry();}}>{t('next.shell.tryAgain')}</button></p>;
  if (state.phase === 'idle' || state.phase === 'loading') return <p role="status">{t('next.settings.loading')}</p>;
  const exportReady = state.export;
  return <div className={`${styles.reports} ${styles.support}`}>
    {state.permissions && <section className={styles.permissions}>
      <div className={styles.panelHead}><h3>{t('web.maintenance.permissionsTitle')}</h3></div>
      <p className={styles.lede}>{t('web.maintenance.permissionsBody')}</p>
      <div><Button variant="glass" size="small" disabled={busy} onClick={() => {
        setBusy(true);
        void props.source.fixPermissions().then(done => {if (done) setFixed(true);}).catch(props.reportError).finally(() => setBusy(false));
      }}>{t('web.maintenance.fixPermissions')}</Button>
        {fixed && <span className={styles.fixed} role="status">{t('web.maintenance.permissionsFixed')}</span>}</div>
    </section>}
    <section className={styles.panel}>
      <div className={styles.panelHead}><h3>{t('settings.server.capabilities')}</h3></div>
      <p className={styles.lede}>{t('web.capabilities.help')}</p>
      <ul className={styles.rows}>{state.capabilities.map(capability => <CapabilityRow key={capability.capability} capability={capability} />)}</ul>
    </section>
    <section className={styles.panel}>
      <div className={styles.panelHead}><h3>{t('web.logs.exportTitle')}</h3></div>
      <p className={styles.lede}>{t('web.logs.exportLede')}</p>
      <>
        <div className={styles.exportRow}><span className={styles.fieldLabel}>{t('web.logs.timeWindow')}</span>
          <div className={styles.statusChoice} role="radiogroup" aria-label={t('web.logs.timeWindow')}>{WINDOWS.map(entry => <button key={entry.id} type="button" role="radio"
            aria-checked={entry.id === window} className={styles.statusOption} disabled={state.preparing} onClick={() => setWindow(entry.id)}>{t(entry.label)}</button>)}</div></div>
        <div className={styles.exportRow}><span className={styles.fieldLabel}>{t('web.logs.include')}</span>
          <div className={styles.checks}>{SUPPORT_COMPONENTS.map(component => <label key={component} className={styles.check}>
            <input type="checkbox" checked={parts.includes(component)} disabled={state.preparing}
              onChange={event => setParts(current => event.currentTarget.checked ? [...new Set([...current, component])] : current.filter(part => part !== component))} />
            {t(PART_LABEL[component])}</label>)}</div></div>
      </>
      <div className={styles.exportRow}><span className={styles.fieldLabel}>{t('web.logs.prepareExport')}</span>
        <SettingsAction size="small" appearance="glass" loading={state.preparing} disabled={state.preparing || !parts.length}
          onClick={() => {void props.source.prepare(WINDOWS.find(entry => entry.id === window)!.ms, parts).catch(props.reportError);}}>{t('web.logs.prepare')}</SettingsAction></div>
      {exportReady && <div className={styles.exportReady}>
        <b>{t('web.logs.readyToDownload')}</b>
        <p className={styles.note}>{t('web.logs.readyHelp', {included: exportReady.manifest.included.join(', ') || t('web.logs.exportNothing'),
          excluded: exportReady.manifest.excluded.length ? exportReady.manifest.excluded.join(', ') : t('web.logs.exportNothing'),
          redaction: exportReady.manifest.redaction, expires: defaultI18n.dateTime(exportReady.expiresAt)})}</p>
        <Button variant="glass" size="small" onClick={() => {
          void props.source.download().then(bundle => {
            const blob = new Blob([JSON.stringify(bundle, null, 2)], {type: 'application/json'});
            const url = URL.createObjectURL(blob), link = document.createElement('a');
            link.href = url;link.download = 'portico-support.json';link.click();URL.revokeObjectURL(url);
          }).catch(props.reportError);
        }}>{t('web.logs.downloadJson')}</Button>
      </div>}
    </section>
    <section className={styles.panel}>
      <div className={styles.panelHead}><h3>{t('settings.server.skippedParts')}</h3></div>
      <p className={styles.lede}>{t('settings.server.skippedPartsLede')}</p>
      {state.skipped.length ? <ul className={styles.rows}>{state.skipped.map(part => <li key={`${part.document}/${part.part}`} className={styles.recordRow}>
        <div className={styles.reportText}><b>{part.part}</b><small>{part.document}</small></div>
        <span className={styles.sequence}>{t('settings.server.skippedPartsCount', {count: part.count})}</span>
      </li>)}</ul> : <p className={styles.none}>{t('settings.server.skippedPartsNone')}</p>}
    </section>
  </div>;
}

function CapabilityRow({capability}: Readonly<{capability: ServerCapability}>) {
  const available = capability.available;
  return <li className={styles.capabilityRow}>
    <div className={styles.reportText}><b>{capabilityLabel(capability.capability)}</b>
      {!available && capability.code && <small>{t(capabilityReasonId(capability.code) as MessageId)}</small>}
      {!available && capability.code && <details className={styles.technical}><summary>{t('web.capabilities.technical')}</summary>
        <p className={styles.note}><code>{capability.code}</code>{capability.detail && <span> {capability.detail}</span>}</p>
      </details>}
    </div>
    <span className={styles.status} data-status={available ? 'resolved' : 'closed'}>{t(available ? 'web.capabilities.available' : 'web.capabilities.unavailable')}</span>
  </li>;
}
