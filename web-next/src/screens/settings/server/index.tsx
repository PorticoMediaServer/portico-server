import type {ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import type {SettingsRow} from '@core/presentation/settings-spec.ts';
import type {SettingPresentation} from '../../../kit/settings';
import {Icon} from '../../../kit/Icon';
import type {SettingsBackupSource} from '../backup-source';
import {StorageBackupFolder, StorageBackupNow, StorageBackups, StorageRestoreFile, type BackupSurfaceProps} from './Backups';
import {StorageDataFolder, StorageUsage} from './StorageUsage';
import {SettingsStorageSource, type StorageTransport} from './storage-source';
import {sectionLinkRows, sectionTarget, storageRowState, type SettingsTarget, type StorageRowFacts} from './rows';
import styles from './server.module.css';

const t = defaultI18n.t;

export type {SettingsTarget} from './rows';

export type SettingsServerSurfaces = Readonly<{
  eligible: (row: SettingsRow) => boolean | undefined;
  special: (row: SettingsRow, state: SettingPresentation) => ReactNode | undefined;
  rowState: (row: SettingsRow) => Pick<SettingPresentation, 'present'> | undefined;
  dispose: () => void;
}>;

export type SettingsServerOptions = Readonly<{
  transport: StorageTransport; serverId: string; isCurrent: () => boolean; openTarget: (target: SettingsTarget) => void;
  /** The controller's own owner-scoped backup reader; it is already loaded for this section. */
  backups: SettingsBackupSource; subscribe: (listener: () => void) => () => void; scopeKey: string; reportError: (error: unknown) => void;
}>;

/** Owner-only Storage facts and controls. One shared sweep feeds both drawn rows. */
export function createServerSettingsSurfaces(options: SettingsServerOptions): SettingsServerSurfaces {
  const storage = new SettingsStorageSource(options.transport, options.serverId, options.isCurrent);
  const backups: BackupSurfaceProps = {source: options.backups, subscribe: options.subscribe, isCurrent: options.isCurrent, scopeKey: options.scopeKey, reportError: options.reportError};
  const facts = (): StorageRowFacts => {
    const snapshot = options.backups.getSnapshot();
    return {live: options.isCurrent(), backups: !!snapshot.settings || !!snapshot.document};
  };
  const linkTarget = (row: SettingsRow) => sectionLinkRows.has(row.id) ? sectionTarget(row.control.to) : undefined;
  return {
    // These rows are drawn here, so this page decides when they exist at all.
    eligible: row => storageRowState(row, facts())?.present,
    special: (row, state) => {
      const target = linkTarget(row), text = row.control.label && t(row.control.label);
      if (target && text) return <button type="button" className={styles.sectionLink} onClick={() => {if (options.isCurrent()) options.openTarget(target);}}>{text}<Icon name="i-right" size={16} /></button>;
      const label = state.label ?? row.label;
      return row.id === 'storage.dataFolder' ? <StorageDataFolder source={storage} label={label} />
        : row.id === 'storage.usage' ? <StorageUsage source={storage} label={label} />
        : row.id === 'storage.backupFolder' ? <StorageBackupFolder {...backups} api={options.transport} />
        : row.id === 'storage.backupNow' ? <StorageBackupNow {...backups} />
        : row.id === 'storage.backups' ? <StorageBackups {...backups} />
        : row.id === 'storage.restoreFile' ? <StorageRestoreFile {...backups} api={options.transport} />
        : undefined;
    },
    rowState: row => storageRowState(row, facts()),
    dispose: () => {storage.dispose();},
  };
}
