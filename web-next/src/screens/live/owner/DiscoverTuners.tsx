import {useLayoutEffect, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {LibraryContentApi} from '@core/library-content.ts';
import {errorMessage} from '@core/presentation/errors.ts';
import {Button} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays';
import {SettingsRow} from '../../../kit/shell/SettingsSurfaces';
import styles from './owner.module.css';
const t = defaultI18n.t;
const noSubscribe = () => () => {};
const noSnapshot = () => undefined;
type DiscoveryLease = {id: string; live: boolean; current: () => boolean; controller: DiscoveryController};
import {DiscoveryController} from './discovery-controller';
/** Discovery does not save or confirm a local address. Use this opens the
 * ordinary source check, including its server-required LAN confirmation. */
export function DiscoverTuners({api, serverId, isCurrent, reportError, close, onUseTuner}: {
  api: LibraryContentApi; serverId: string; isCurrent: () => boolean; reportError: (error: unknown) => void; close: () => void; onUseTuner: (tuner: string) => void;
}) {
  const [published, publish] = useState<DiscoveryLease>();
  useLayoutEffect(() => {
    let live = true;
    const current = () => live && isCurrent();
    const controller = new DiscoveryController(api, serverId, current, reportError);
    const lease: DiscoveryLease = {id: crypto.randomUUID(), live: true, current, controller};
    publish(lease);
    void controller.search();
    return () => {lease.live = false;controller.dispose();};
  }, [api, serverId, isCurrent, reportError]);
  const lease = published?.live && published.current() ? published : undefined;
  const state = useSyncExternalStore(lease?.controller.subscribe ?? noSubscribe, lease?.controller.getSnapshot ?? noSnapshot, lease?.controller.getSnapshot ?? noSnapshot);
  const closeCurrent = () => {
    if (!lease?.current()) return;
    lease.live = false;lease.controller.dispose();close();
  };
  if (!lease || !state) return null;
  const {busy, result, error} = state;
  const searchAgain = () => {if (lease.current()) void lease.controller.search();};
  return <Dialog open width={520} title={t('web.liveSource.discoverTitle')} description={t('web.liveSource.discoverLede')} onOpenChange={open => {if (!open) closeCurrent();}}
    footer={<><Button size="small" variant="text" onClick={closeCurrent}>{t('action.close')}</Button><Button size="small" icon="i-refresh" loading={busy} onClick={searchAgain}>{t('web.liveSource.searchAgain')}</Button></>}>
    <div className={styles.fields}>
      {busy && <p role="status">{t('web.liveSource.searching')}</p>}
      {!!error && <p role="alert" className={styles.error}>{errorMessage(error)}</p>}
      {result && !result.discoveryEnabled && <p>{t('web.liveSource.searchOff')}</p>}
      {result?.discoveryEnabled && !result.devices.length && <p>{t('web.liveSource.noTuners')}</p>}
      {result?.devices.map(tuner => <SettingsRow key={tuner.deviceId || tuner.address} label={tuner.friendlyName || tuner.model || t('web.liveSource.tunerFallback')}
        help={[tuner.model, tuner.address, tuner.tunerCount ? t('server.live.tunerCount', {count: tuner.tunerCount}) : ''].filter(Boolean).join(' · ')}>
        {tuner.configured ? <span className={styles.badge}>{t('web.liveSource.alreadyAdded')}</span> : <Button size="small" onClick={() => {if (lease.current() && !busy) onUseTuner(tuner.baseUrl);}}>{t('web.liveSource.useThis')}</Button>}
      </SettingsRow>)}
    </div>
  </Dialog>;
}
