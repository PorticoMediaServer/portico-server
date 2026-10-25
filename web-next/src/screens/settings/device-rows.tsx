import {useState} from 'react';
import {Licences} from '../../kit/states/Licences';
import {defaultI18n} from '@i18n';
import type {SettingsRow} from '@core/presentation/settings-spec.ts';
import {Button} from '../../kit/controls';
import {Icon} from '../../kit/Icon';
import {deviceHelpAddress, deviceHelpURL} from './device-rows-model';

/** The drawn Device link control: accent text with the frame's 16px trailing chevron. */
function DeviceLink({label, onClick}: {label: string; onClick: () => void}) {
  return <Button variant="text" size="small" style={{color: 'var(--color-accent-strong)'}} onClick={onClick}>
    <span style={{display: 'inline-flex', alignItems: 'center', gap: 4}}>{label}<Icon name="i-right" size={16} /></span>
  </Button>;
}

/** The drawn Device rows whose control is a route or an external address, not a stored value. */
export function DeviceRowControl({row, onSwitchServer}: {row: SettingsRow; onSwitchServer: () => void}) {
  const [licensesOpen, setLicensesOpen] = useState(false);
  if (row.id === 'device.licenses') return <><DeviceLink label={defaultI18n.t(row.control.label ?? row.label)} onClick={() => setLicensesOpen(true)}/><Licences open={licensesOpen} onOpenChange={setLicensesOpen}/></>;
  if (row.id === 'device.server') return <DeviceLink label={defaultI18n.t('next.settingsSpec.account.device.groups.3.rows.0.control.label')} onClick={onSwitchServer}/>;
  if (row.id === 'device.help') return <DeviceLink label={deviceHelpAddress} onClick={() => window.open(deviceHelpURL, '_blank', 'noopener,noreferrer')}/>;
  return null;
}
