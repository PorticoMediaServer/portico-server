import {useEffect, useState} from 'react';
import type {ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import {routePath} from '../../../../../contracts/src/routes.ts';
import {loadRemoteAccess, remoteAccessSummary, type RemoteAccess} from '@core/remote-access.ts';
import {parseConnectivityStatus, type ConnectivityStatus} from '@core/server-administration.ts';
import styles from './server-remote.module.css';

const t = defaultI18n.t;

/** Server › Remote: the section's status facts from the server's own producers,
 * over the canonical settings rows. Certificate and connection rows read the
 * connectivity status; remote reachability reads the remote-access status. */
export function ServerRemotePage({api, isCurrent, groups, advanced}: Readonly<{
  api: Readonly<{request: <T>(path: string, method: string, body?: unknown, signal?: AbortSignal) => Promise<T>}>;
  isCurrent: () => boolean; groups: ReactNode; advanced: ReactNode;
}>) {
  const [remote, setRemote] = useState<RemoteAccess>();
  const [connectivity, setConnectivity] = useState<ConnectivityStatus>();
  useEffect(() => {
    const active = new AbortController();
    void loadRemoteAccess(api as Parameters<typeof loadRemoteAccess>[0], active.signal).then(value => {if (!active.signal.aborted && isCurrent()) setRemote(value);}).catch(() => {});
    void api.request(routePath('get_connectivity_status'), 'GET', undefined, active.signal)
      .then(value => {if (!active.signal.aborted && isCurrent()) setConnectivity(parseConnectivityStatus(value));}).catch(() => {});
    return () => active.abort();
  }, [api, isCurrent]);
  const tls = connectivity?.tls;
  const custom = tls?.customCertificate;
  const publicAddresses = remote?.topology.public ?? [];
  return <div className={styles.page}>
    <h2 className={styles.title}>{t('next.settingsSpec.server.remote.title')}</h2>
    <dl className={styles.facts}>
      <div><dt>{t('next.remoteStatus.reachability')}</dt><dd>{remote ? remoteAccessSummary(remote) : t('next.remoteStatus.reading')}</dd></div>
      {publicAddresses.length > 0 && <div><dt>{t('next.remoteStatus.addresses')}</dt><dd className={styles.mono}>{publicAddresses.join(', ')}</dd></div>}
      <div><dt>{t('next.remoteStatus.secure')}</dt><dd>{tls ? (tls.publiclyTrusted ? t('next.remoteStatus.trusted') : tls.configured ? t('next.remoteStatus.localOnly') : t('next.remoteStatus.off')) : t('next.remoteStatus.reading')}
        {tls?.notAfter ? ` · ${t('next.remoteStatus.expires', {when: tls.notAfter})}` : ''}</dd></div>
      <div><dt>{t('next.remoteStatus.ownCertificate')}</dt><dd>{custom?.domain ?? connectivity?.policy.customCertificateDomain ?? t('next.remoteStatus.none')}
        {custom?.state ? ` · ${custom.state}` : ''}</dd></div>
      <div><dt>{t('next.remoteStatus.connection')}</dt><dd>{connectivity ? `${connectivity.connection.clientAddress || '—'} · ${connectivity.connection.remote ? t('next.remoteStatus.away') : t('next.remoteStatus.home')}` : t('next.remoteStatus.reading')}</dd></div>
    </dl>
    {groups}
    {advanced}
  </div>;
}
