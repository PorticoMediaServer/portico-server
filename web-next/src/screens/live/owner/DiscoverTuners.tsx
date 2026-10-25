import {useEffect, useMemo, useRef, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {LibraryContentApi} from '@core/library-content.ts';
import {errorMessage} from '@core/presentation/errors.ts';
import {Button} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays';
import {SettingsRow} from '../../../kit/shell/SettingsSurfaces';
import styles from './owner.module.css';
const t = defaultI18n.t;
import {DiscoveryController} from './discovery-controller';
import {dialogAction} from './dialog-lifetime';
/** Discovery does not save or confirm a local address. Use this opens the
 * ordinary source check, including its server-required LAN confirmation. */
export function DiscoverTuners({api, serverId, isCurrent, reportError, close, onUseTuner}: {
  api: LibraryContentApi; serverId: string; isCurrent: () => boolean; reportError: (error: unknown) => void; close: () => void; onUseTuner: (tuner: string) => void;
}) {
  const controller = useMemo(() => new DiscoveryController(api, serverId, isCurrent, reportError), [api, serverId, isCurrent, reportError]);
  const owner = useRef(controller);owner.current = controller;
  const {busy, result, error} = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  useEffect(() => {void controller.search();return () => controller.dispose();}, [controller]);
  const closeCurrent = dialogAction(owner, controller, () => {controller.dispose();if (owner.current === controller) close();});
  return <Dialog open width={520} title={t('web.liveSource.discoverTitle')} description={t('web.liveSource.discoverLede')} onOpenChange={open => {if (!open) closeCurrent();}}
    footer={<><Button size="small" variant="text" onClick={closeCurrent}>{t('action.close')}</Button><Button size="small" icon="i-refresh" loading={busy} onClick={dialogAction(owner, controller, () => void controller.search())}>{t('web.liveSource.searchAgain')}</Button></>}>
    <div className={styles.fields}>
      {busy && <p role="status">{t('web.liveSource.searching')}</p>}
      {!!error && <p role="alert" className={styles.error}>{errorMessage(error)}</p>}
      {result && !result.discoveryEnabled && <p>{t('web.liveSource.searchOff')}</p>}
      {result?.discoveryEnabled && !result.devices.length && <p>{t('web.liveSource.noTuners')}</p>}
      {result?.devices.map(tuner => <SettingsRow key={tuner.deviceId || tuner.address} label={tuner.friendlyName || tuner.model || t('web.liveSource.tunerFallback')}
        help={[tuner.model, tuner.address, tuner.tunerCount ? t('server.live.tunerCount', {count: tuner.tunerCount}) : ''].filter(Boolean).join(' · ')}>
        {tuner.configured ? <span className={styles.badge}>{t('web.liveSource.alreadyAdded')}</span> : <Button size="small" onClick={dialogAction(owner, controller, () => {if (!busy) onUseTuner(tuner.baseUrl);})}>{t('web.liveSource.useThis')}</Button>}
      </SettingsRow>)}
    </div>
  </Dialog>;
}
