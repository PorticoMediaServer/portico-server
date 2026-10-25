import {useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {Icon} from '../../../kit/Icon';
import {SettingsAction} from '../../../kit/settings';
import {hardwareMethodName, hardwareTestResult} from './hardware-rows';
import type {SettingsHardwareSource} from './hardware-source';
import styles from './hardware.module.css';

const t = defaultI18n.t;

/** Devices draws the adapters the server found: its driver's name for the card,
 * the method in use on it, and whether conversions go there now. Nothing here is
 * a choice; the override is Device in Advanced. */
export function HardwareDevices({source}: {source: SettingsHardwareSource}) {
  const snapshot = useSyncExternalStore(source.subscribe, source.getSnapshot);
  const devices = snapshot.capacity?.hardware.devices ?? [];
  if (!devices.length) return null;
  return <div className={styles.devices}>{devices.map(device => {
    const method = hardwareMethodName(device.method);
    return <div key={device.id} className={styles.device}>
      <span className={styles.deviceIcon}><Icon name="i-chip" size={20} /></span>
      <div className={styles.deviceText}>
        <b>{device.name}</b>
        {method && <small>{method}</small>}
      </div>
      <span className={styles.badge} data-tone={device.inUse ? 'ok' : 'quiet'}>{device.inUse ? t('next.settings.hardware.device.inUse') : t('next.settings.hardware.device.ready')}</span>
    </div>;
  })}</div>;
}

/** Test hardware probes every adapter and format again now. The button shows its
 * spinner while the server works, and the result line only ever reports good
 * news: what is still wrong stays on Last error. */
export function TestHardware({source}: {source: SettingsHardwareSource}) {
  const snapshot = useSyncExternalStore(source.subscribe, source.getSnapshot);
  const result = hardwareTestResult(snapshot);
  return <div className={styles.test}>
    <SettingsAction loading={snapshot.testing} disabled={snapshot.phase !== 'ready'} onClick={() => {void source.test();}}>{t('next.settingsSpec.server.streaming.groups.3.rows.5.control.label')}</SettingsAction>
    {result && <span className={styles.result} role="status"><Icon name="i-check" size={15} />{result}</span>}
  </div>;
}
