import {useCallback, useEffect, useRef, useState, type RefObject} from 'react';
import type {HttpLocalApi} from '@core/index.ts';
import {defaultI18n} from '@i18n';
import {Dialog} from '../../kit/overlays';
import {Button} from '../../kit/controls';
import {captureAppLog} from '../../platform/app-log';
import styles from './app-log.module.css';

const t = defaultI18n.t;
type Recipient = Readonly<{displayName: string; revision: string}>;
function recipient(value: unknown): Recipient {
  const row = value as Partial<Recipient> | null;
  if (!row || typeof row.displayName !== 'string' || !row.displayName.trim() || row.displayName.length > 512
    || typeof row.revision !== 'string' || !/^[a-f0-9]{64}$/.test(row.revision)) throw new Error('invalid_app_report_recipient');
  return Object.freeze({displayName: row.displayName, revision: row.revision});
}

/** One explicit opening captures the journal and recipient. An uncertain retry
 * keeps that same body/key; closing aborts and discards consent. */
export function SendAppLog({api, isCurrent, onClose, returnFocusRef}: {
  api: Pick<HttpLocalApi, 'request' | 'requestRaw'>; isCurrent(): boolean; onClose(): void;
  returnFocusRef: RefObject<HTMLElement | null>;
}) {
  const [capture] = useState(() => ({api, isCurrent, log: captureAppLog(), life: new AbortController(), key: crypto.randomUUID()}));
  const [owner, setOwner] = useState<Recipient>();
  const [consent, setConsent] = useState(false);
  const [phase, setPhase] = useState<'loading' | 'idle' | 'busy' | 'done' | 'failed'>('loading');
  const busy = useRef(false), sent = useRef(false);
  const current = useCallback(() => !capture.life.signal.aborted && capture.api === api && capture.isCurrent() && isCurrent() && capture.log.isCurrent(), [capture, api, isCurrent]);
  useEffect(() => {
    if (!current()) return;
    if (!capture.log.entries.length) {setPhase('idle'); return () => capture.life.abort();}
    void capture.api.request('/v1/app-reports/recipient', 'GET', undefined, capture.life.signal)
      .then(value => {if (current()) {setOwner(recipient(value));setPhase('idle');}})
      .catch(() => {if (current()) setPhase('failed');});
    return () => capture.life.abort();
  }, [capture, current]);
  const close = () => {capture.life.abort();onClose();};
  const send = async () => {
    if (!current() || !owner || !consent || busy.current || sent.current) return;
    if (!capture.log.sendable()) {setPhase('failed');return;}
    const body = Object.freeze({recipientRevision: owner.revision, entries: capture.log.entries});
    if (new TextEncoder().encode(JSON.stringify(body)).byteLength > 16384) {setPhase('failed');return;}
    busy.current = true;setPhase('busy');
    try {
      const response = await capture.api.requestRaw('/v1/app-reports', 'POST', {headers: {'Idempotency-Key': capture.key}, body, signal: capture.life.signal});
      if (!current()) return;
      const receipt = response.body as {id?: unknown; entryCount?: unknown} | null;
      if ((response.status !== 200 && response.status !== 201) || !receipt || typeof receipt.id !== 'string' || !receipt.id || receipt.entryCount !== body.entries.length) throw new Error('app_report_not_acknowledged');
      sent.current = true;capture.log.acknowledge();setPhase('done');
    } catch {if (current()) setPhase('failed');}
    finally {busy.current = false;}
  };
  return <Dialog open onOpenChange={value => {if (!value) close();}} size="sm"
    title={t('next.settingsSpec.account.device.groups.5.rows.5.label')} returnFocusRef={returnFocusRef}
    footer={<><Button variant="glass" size="small" onClick={close}>{t('action.cancel')}</Button>
      {phase !== 'done' && <Button size="small" loading={phase === 'busy'} disabled={!current() || !owner || !consent || !capture.log.entries.length || phase === 'busy'} onClick={() => {void send();}}>{t('next.settingsSpec.account.device.groups.5.rows.5.control.label')}</Button>}</>}>
    <div className={styles.body}>
      <p>{t('next.settingsSpec.account.device.groups.5.rows.5.help')}</p>
      {!capture.log.entries.length ? <p role="status">{t('next.settings.appLog.empty')}</p>
        : phase === 'loading' ? <p role="status">{t('status.loading')}</p>
        : phase === 'done' ? <p role="status">{t('next.settings.appLog.sent', {owner: owner!.displayName})}</p>
        : owner && <>
          <p>{t('next.settings.appLog.recipient', {owner: owner.displayName})}</p>
          <label className={styles.confirm}><input type="checkbox" checked={consent} disabled={phase === 'busy'} onChange={event => setConsent(event.currentTarget.checked)} />
            <span>{t('next.settingsSpec.account.device.groups.5.rows.5.control.confirm')}</span></label>
        </>}
      {phase === 'failed' && <p role="alert">{t('about.report.failed')}</p>}
    </div>
  </Dialog>;
}
