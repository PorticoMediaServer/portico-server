import {useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import {Dialog} from '../../kit/overlays';
import {Button} from '../../kit/controls';
import {appLogSnapshot, clearAppLog, subscribeAppLog} from '../../platform/app-log';

const t = defaultI18n.t;

/** Settings › This device › Send app log. One explicit opening, one explicit confirmation
 * naming the owner who receives it, one send with its own operation key. Closing cancels
 * the request; a retired authority never publishes a result. */
export function SendAppLog({open, onOpenChange, api, owner, isCurrent, returnFocusRef}: {
  open: boolean; onOpenChange(open: boolean): void; owner: string; isCurrent(): boolean;
  api: Readonly<{requestRaw(path: string, method: string, options?: {headers?: Record<string, string>; body?: unknown; signal?: AbortSignal}): Promise<{status: number; body: unknown}>}>;
  returnFocusRef: React.RefObject<HTMLElement | null>;
}) {
  const [entries] = useState(() => [...appLogSnapshot()]);
  const [consent, setConsent] = useState(false), [phase, setPhase] = useState<'idle' | 'busy' | 'done' | 'failed'>('idle');
  const life = useRef<AbortController | null>(null), key = useRef(crypto.randomUUID());
  const close = () => { life.current?.abort(); life.current = null; setConsent(false); setPhase('idle'); onOpenChange(false); };
  const send = () => {
    if (!consent || !entries.length || life.current) return;
    const run = new AbortController(), mine = key.current;
    life.current = run; setPhase('busy');
    void api.requestRaw('/v1/app-reports', 'POST', {headers: {'Idempotency-Key': mine}, body: {entries}, signal: run.signal})
      .then(response => { if (run.signal.aborted || !isCurrent()) return; setPhase(response.status === 200 || response.status === 201 ? 'done' : 'failed'); if (response.status === 200 || response.status === 201) clearAppLog(); })
      .catch(() => { if (!run.signal.aborted && isCurrent()) setPhase('failed'); })
      .finally(() => { if (life.current === run) life.current = null; });
  };
  return <Dialog open={open} onOpenChange={value => { if (!value) close(); }} title={t('next.settingsSpec.account.device.groups.5.rows.5.label')} returnFocusRef={returnFocusRef}>
    {phase === 'done' ? <p role="status">{t('next.cast.linkTvApproved')}</p> : <>
      <p>{t('next.settingsSpec.account.device.groups.5.rows.5.help')}</p>
      <p><b>{owner}</b>{' '}{entries.length > 0 ? `(${entries.length})` : ''}</p>
      <label><input type="checkbox" checked={consent} disabled={phase === 'busy' || !entries.length} onChange={event => setConsent(event.currentTarget.checked)} />
        {t('next.settingsSpec.account.device.groups.5.rows.5.control.confirm')}</label>
      <div style={{display: 'flex', gap: 8, marginTop: 12}}>
        <Button size="small" disabled={!consent || !entries.length || phase === 'busy'} onClick={send}>{t('next.settingsSpec.account.device.groups.5.rows.5.control.label')}</Button>
        <Button size="small" variant="glass" onClick={close}>{t('action.cancel')}</Button>
      </div>
      {phase === 'failed' && <p role="status">{t('next.cast.linkTvFailed')}</p>}
    </>}
  </Dialog>;
}
