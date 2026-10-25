import {useEffect, useState} from 'react';
import {defaultI18n} from '@i18n';
import type {LibraryContentApi} from '@core/library-content.ts';
import {errorMessage} from '@core/presentation/errors.ts';
import {Button} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays';
import {SettingsRow} from '../../../kit/shell/SettingsSurfaces';
import styles from './owner.module.css';
const t = defaultI18n.t;
type Tuner = Readonly<{deviceId: string; model: string; friendlyName: string; address: string; baseUrl: string; tunerCount: number; configured: boolean}>;
type Discovery = Readonly<{devices: readonly Tuner[]; discoveryEnabled: boolean}>;

/** Discovery does not save or confirm a local address. Use this opens the
 * ordinary source check, including its server-required LAN confirmation. */
export function DiscoverTuners({api, serverId, isCurrent, reportError, close, useTuner}: {
  api: LibraryContentApi; serverId: string; isCurrent: () => boolean; reportError: (error: unknown) => void; close: () => void; useTuner: (tuner: string) => void;
}) {
  const [attempt, setAttempt] = useState(0), [busy, setBusy] = useState(true), [result, setResult] = useState<Discovery>(), [error, setError] = useState<unknown>();
  useEffect(() => {
    const request = new AbortController();let active = true;
    if (!isCurrent()) return;
    setBusy(true);setError(undefined);setResult(undefined);
    void api.request<{serverId: string; result: Discovery}>('/v1/admin/live-sources/discover', 'POST', {windowMs: 5000}, request.signal).then(value => {
      if (!active || !isCurrent()) return;
      if (value.serverId !== serverId || !value.result || !Array.isArray(value.result.devices) || typeof value.result.discoveryEnabled !== 'boolean') throw new Error('live_discovery_response_invalid');
      setResult(value.result);
    }).catch(failure => {
      if (!active || request.signal.aborted || !isCurrent()) return;
      if ([401, 403].includes((failure as {status?: number})?.status ?? 0)) reportError(failure);else setError(failure);
    }).finally(() => {if (active && isCurrent()) setBusy(false);});
    return () => {active = false;request.abort();};
  }, [api, serverId, isCurrent, reportError, attempt]);
  return <Dialog open width={520} title={t('web.liveSource.discoverTitle')} description={t('web.liveSource.discoverLede')} onOpenChange={open => {if (!open) close();}}
    footer={<><Button size="small" variant="text" onClick={close}>{t('action.close')}</Button><Button size="small" icon="i-refresh" loading={busy} onClick={() => {if (isCurrent()) setAttempt(value => value + 1);}}>{t('web.liveSource.searchAgain')}</Button></>}>
    <div className={styles.fields}>
      {busy && <p role="status">{t('web.liveSource.searching')}</p>}
      {!!error && <p role="alert" className={styles.error}>{errorMessage(error)}</p>}
      {result && !result.discoveryEnabled && <p>{t('web.liveSource.searchOff')}</p>}
      {result?.discoveryEnabled && !result.devices.length && <p>{t('web.liveSource.noTuners')}</p>}
      {result?.devices.map(tuner => <SettingsRow key={tuner.deviceId || tuner.address} label={tuner.friendlyName || tuner.model || t('web.liveSource.tunerFallback')}
        help={[tuner.model, tuner.address, tuner.tunerCount ? t('server.live.tunerCount', {count: tuner.tunerCount}) : ''].filter(Boolean).join(' · ')}>
        {tuner.configured ? <span className={styles.badge}>{t('web.liveSource.alreadyAdded')}</span> : <Button size="small" onClick={() => {if (isCurrent() && !busy) useTuner(tuner.baseUrl || tuner.address);}}>{t('web.liveSource.useThis')}</Button>}
      </SettingsRow>)}
    </div>
  </Dialog>;
}
