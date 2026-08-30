import type { DLNAStatus } from '@porticomediaserver/client-core';
import { StatusSuccessIcon, ActionOpenExternalIcon, LibraryCollectionIcon, DeviceNetworkIcon, ActionRefreshIcon, StatusWarningIcon, DeviceWifiIcon, ActionCloseIcon } from '#portico-icons';
import { useCallback, useState } from 'react';
import { SecondaryButton } from '../../components/controls/Buttons';
import { reviewedProductErrorText } from '../../components/ProductLanguage';
import {
  InlineNotice,
  SettingsError,
  SettingsGroup,
  SettingsLoading,
} from './SettingsControls';
import { useSettingsQuery } from './settingsHooks';
import type { SettingsDataSource, SettingsViewer } from './settingsTypes';

export function DLNAOperations({ source, viewer }: { source: SettingsDataSource; viewer: SettingsViewer }) {
  const canManage = viewer.permissions?.manageServer ?? viewer.role !== 'user';
  const [revision, setRevision] = useState(0);
  const load = useCallback((next: SettingsDataSource, signal: AbortSignal) => canManage ? next.dlnaStatus(signal) : Promise.resolve(null), [canManage]);
  const query = useSettingsQuery<DLNAStatus | null>(load, source, revision);
  if (!canManage) return <SettingsGroup title="DLNA runtime" description="Local discovery and unauthenticated library exposure."><div className="portico-settings-readonly-note"><DeviceWifiIcon />Your account cannot inspect DLNA exposure.</div></SettingsGroup>;
  if (query.status === 'loading') return <SettingsGroup title="DLNA runtime" description="Local discovery and unauthenticated library exposure."><SettingsLoading label="Loading DLNA status" /></SettingsGroup>;
  if (query.status === 'error') return <SettingsGroup title="DLNA runtime" description="Local discovery and unauthenticated library exposure."><SettingsError title="DLNA status is unavailable" message={reviewedProductErrorText(query.error, 'settings.load-failed', { sectionName: 'DLNA status' })} onRetry={() => setRevision((current) => current + 1)} /></SettingsGroup>;
  const status = query.data;
  if (!status) return null;
  const exposed = status.exposedLibraries.filter((library) => library.exposed);
  return <SettingsGroup title="DLNA runtime" description="Local discovery and unauthenticated library exposure." actions={<SecondaryButton onClick={() => setRevision((current) => current + 1)}><ActionRefreshIcon /> Refresh</SecondaryButton>}>
    {status.unauthenticatedLanAccess && <InlineNotice tone="warn"><StatusWarningIcon /> {exposed.length} {exposed.length === 1 ? 'library is' : 'libraries are'} browsable without Portico authentication by devices that can reach this server on the LAN.</InlineNotice>}
    <div className="portico-dlna-ledger">
      <div><DeviceWifiIcon /><span><small>Service</small><strong>{status.enabled ? 'Enabled' : 'Disabled'}</strong><em>{status.friendlyName}</em></span></div>
      <div><ActionRefreshIcon /><span><small>Discovery</small><strong>{status.ssdpDiscovery}</strong><em>{status.mediaServerUrn}</em></span></div>
      <div><LibraryCollectionIcon /><span><small>Exposed libraries</small><strong>{exposed.length}</strong><em>{status.exposedLibraries.length} total</em></span></div>
      <div><DeviceNetworkIcon /><span><small>Byte ranges</small><strong>{status.byteRangeStreamingSupported ? 'Supported' : 'Unavailable'}</strong><em>Local streaming</em></span></div>
    </div>
    <div className="portico-dlna-library-list">{status.exposedLibraries.map((library) => <div key={library.id}><span className={library.exposed ? 'healthy' : ''}>{library.exposed ? <StatusSuccessIcon /> : <ActionCloseIcon />}</span><span><strong>{library.name}</strong><small>{library.type} · {library.count} items</small></span><b>{library.exposed ? 'Exposed' : 'Private'}</b></div>)}</div>
    {status.enabled && <div className="portico-dlna-links"><a href={status.deviceDescriptionUrl} target="_blank" rel="noreferrer"><ActionOpenExternalIcon /> Device description</a><a href={status.contentDirectoryUrl} target="_blank" rel="noreferrer"><ActionOpenExternalIcon /> Content directory</a></div>}
  </SettingsGroup>;
}
