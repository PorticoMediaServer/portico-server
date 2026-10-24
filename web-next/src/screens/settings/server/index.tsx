import type {ReactNode} from 'react';
import type {SettingsRow} from '@core/presentation/settings-spec.ts';
import type {SettingPresentation} from '../../../kit/settings';
import {StorageDataFolder, StorageUsage} from './StorageUsage';
import {SettingsStorageSource, type StorageTransport} from './storage-source';

/** Rows this batch does not draw yet stay absent rather than showing an empty control (ruling 3). */
const pending = new Set(['storage.networkNotice', 'storage.emptyCache', 'storage.backupFolder', 'storage.backupNow', 'storage.backups', 'storage.restoreFile',
  'storage.backupPassphrase', 'storage.downloads', 'storage.upgradeCopy', 'storage.backupKeysNote', 'storage.keepHistory', 'storage.keepLogsLink']);

export type SettingsServerSurfaces = Readonly<{
  special: (row: SettingsRow, state: SettingPresentation) => ReactNode | undefined;
  rowState: (row: SettingsRow) => Pick<SettingPresentation, 'present'> | undefined;
  dispose: () => void;
}>;

/** Owner-only Storage facts and controls. One shared sweep feeds both drawn rows. */
export function createServerSettingsSurfaces(options: Readonly<{transport: StorageTransport; serverId: string; isCurrent: () => boolean}>): SettingsServerSurfaces {
  const storage = new SettingsStorageSource(options.transport, options.serverId, options.isCurrent);
  return {
    special: (row, state) => {
      const label = state.label ?? row.label;
      return row.id === 'storage.dataFolder' ? <StorageDataFolder source={storage} label={label} />
        : row.id === 'storage.usage' ? <StorageUsage source={storage} label={label} />
        : undefined;
    },
    rowState: row => pending.has(row.id) ? {present: false} : undefined,
    dispose: () => {storage.dispose();},
  };
}
