import {router} from '../../router';
import {useRef, useState} from 'react';
import {Licences} from '../../kit/states/Licences';
import {defaultI18n} from '@i18n';
import type {SettingsRow} from '@core/presentation/settings-spec.ts';
import {Button} from '../../kit/controls';
import {Icon} from '../../kit/Icon';
import {deviceHelpAddress, deviceHelpURL} from './device-rows-model';
import {LinkTvDialog} from './LinkTvDialog';
import {SendAppLog} from './SendAppLog';

/** The drawn Device link control: accent text with the frame's 16px trailing chevron. */
function DeviceLink({label, onClick}: {label: string; onClick: () => void}) {
  return <Button variant="text" size="small" style={{color: 'var(--color-accent-strong)'}} onClick={onClick}>
    <span style={{display: 'inline-flex', alignItems: 'center', gap: 4}}>{label}<Icon name="i-right" size={16} /></span>
  </Button>;
}

/** The drawn Device rows whose control is a route or an external address, not a stored value. */
export function DeviceRowControl({row, onSwitchServer, api, owner = '', isCurrent = () => true}: {row: SettingsRow; onSwitchServer: () => void; owner?: string; isCurrent?: () => boolean; api?: Readonly<{request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T>; requestRaw?(path: string, method: string, options?: {headers?: Record<string, string>; body?: unknown; signal?: AbortSignal}): Promise<{status: number; body: unknown}>}>}) {
  const [licensesOpen, setLicensesOpen] = useState(false), [linkOpen, setLinkOpen] = useState(false), [logOpen, setLogOpen] = useState(false);
  const linkReturn = useRef<HTMLButtonElement>(null);
  if (row.id === 'device.appVersion') return <DeviceLink label={defaultI18n.t('settings.about')} onClick={() => {void router.navigate({to: '/about'});}}/>;
  if (row.id === 'device.licenses') return <><DeviceLink label={defaultI18n.t(row.control.label ?? row.label)} onClick={() => setLicensesOpen(true)}/><Licences open={licensesOpen} onOpenChange={setLicensesOpen}/></>;
  if (row.id === 'device.sendReport' && api?.requestRaw) return <><Button size="small" variant="glass" onClick={() => setLogOpen(true)}>{defaultI18n.t(row.control.label ?? row.label)}</Button><SendAppLog open={logOpen} onOpenChange={setLogOpen} api={api} owner={owner} isCurrent={isCurrent} returnFocusRef={linkReturn}/></>;
  if (row.id === 'device.linkTv' && api) return <><DeviceLink label={defaultI18n.t('next.settingsSpec.account.device.groups.0.rows.1.control.label')} onClick={() => setLinkOpen(true)}/><LinkTvDialog open={linkOpen} onOpenChange={setLinkOpen} api={api} returnFocusRef={linkReturn}/></>;
  if (row.id === 'device.server') return <DeviceLink label={defaultI18n.t('next.settingsSpec.account.device.groups.3.rows.0.control.label')} onClick={onSwitchServer}/>;
  if (row.id === 'device.help') return <DeviceLink label={deviceHelpAddress} onClick={() => window.open(deviceHelpURL, '_blank', 'noopener,noreferrer')}/>;
  return null;
}
