import type {ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import styles from './server-remote.module.css';

const t = defaultI18n.t;

/** Server › Remote: the dedicated presenter for the section's canonical groups.
 * The rows themselves are the settings controller's own projections; this page
 * gives the section a named surface for its certificate, connection,
 * reachability and outside-access consumers instead of falling through the
 * generic shell. */
export function ServerRemotePage({groups, advanced}: Readonly<{groups: ReactNode; advanced: ReactNode}>) {
  return <div className={styles.page}>
    <h2 className={styles.title}>{t('next.settingsSpec.server.remote.title')}</h2>
    {groups}
    {advanced}
  </div>;
}
