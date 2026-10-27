import {useEffect, useState, useSyncExternalStore} from 'react';
import type {ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import {routePath} from '../../../../../packages/contracts/src/routes.ts';
import {remoteAccessSummary} from '@core/remote-access.ts';
import {parseConnectivityStatus, type ConnectivityStatus} from '@core/server-administration.ts';
import type {RemoteFormSource} from './remote-form-source';
import {Banner} from '../../../kit/overlays';
import {Button} from '../../../kit/controls';
import styles from './server-remote.module.css';

const t = defaultI18n.t;
const noSubscribe = () => () => {};
const noSnapshot = () => undefined;

/** Server › Remote: the section's status facts from the server's own producers,
 * over the canonical settings rows. Certificate and connection rows read the
 * connectivity status; remote reachability reads the remote-access status. */
export function ServerRemotePage({api, isCurrent, source, account, openSection, groups, advanced}: Readonly<{
  api: Readonly<{request: <T>(path: string, method: string, body?: unknown, signal?: AbortSignal) => Promise<T>}>;
  isCurrent: () => boolean;
  source?: RemoteFormSource;
  account?: Readonly<{signIn: 'portico' | 'local'; authority?: string; serverName?: string}>;
  openSection?: (section: string) => void;
  groups: ReactNode; advanced: ReactNode;
}>) {
  const formState = useSyncExternalStore(source?.subscribe ?? noSubscribe, source?.getSnapshot ?? noSnapshot, source?.getSnapshot ?? noSnapshot);
  const remote = formState?.status;
  const [connectivity, setConnectivity] = useState<ConnectivityStatus>();
  useEffect(() => {
    const active = new AbortController();
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
      <div><dt>{t('next.remoteStatus.reachability')}</dt><dd>{remote ? remoteAccessSummary(remote, t) : t('next.remoteStatus.reading')}</dd></div>
      {publicAddresses.length > 0 && <div><dt>{t('next.remoteStatus.addresses')}</dt><dd className={styles.mono}>{publicAddresses.join(', ')}</dd></div>}
      <div><dt>{t('next.remoteStatus.secure')}</dt><dd>{tls ? (tls.publiclyTrusted ? t('next.remoteStatus.trusted') : tls.configured ? t('next.remoteStatus.localOnly') : t('next.remoteStatus.off')) : t('next.remoteStatus.reading')}
        {tls?.notAfter ? ` · ${t('next.remoteStatus.expires', {when: tls.notAfter})}` : ''}</dd></div>
      <div><dt>{t('next.remoteStatus.ownCertificate')}</dt><dd>{custom?.domain ?? connectivity?.policy.customCertificateDomain ?? t('next.remoteStatus.none')}
        {custom?.state ? ` · ${custom.state}` : ''}</dd></div>
      <div><dt>{t('next.remoteStatus.connection')}</dt><dd>{connectivity ? `${connectivity.connection.clientAddress || '—'} · ${connectivity.connection.remote ? t('next.remoteStatus.away') : t('next.remoteStatus.home')}` : t('next.remoteStatus.reading')}</dd></div>
      {connectivity?.policy.customCertificatePath && <div><dt>{t('next.remoteStatus.certificateFiles')}</dt>
        <dd className={styles.mono}>{connectivity.policy.customCertificatePath}{connectivity.policy.customCertificateKeyPath ? ` · ${connectivity.policy.customCertificateKeyPath}` : ''}</dd></div>}
    </dl>
    {!!connectivity?.addresses.length && <section className={styles.addresses}>
      <h3 className={styles.subtitle}>{t('next.remoteStatus.addressesTitle')}</h3>
      <ul className={styles.addressList}>{connectivity.addresses.map(entry => <li key={`${entry.address}-${entry.source}`}>
        <span className={styles.mono}>{entry.address}</span><small>{entry.class}{entry.family ? ` · ${entry.family}` : ''}</small></li>)}</ul>
    </section>}

    {account && <section className={styles.controls}>
      <h3 className={styles.subtitle}>{t('web.general.accountConnection')}</h3>
      <p className={styles.note}>{account.signIn === 'portico'
        ? t('next.remoteStatus.accountConnected', {server: account.serverName ?? ''})
        : t('next.remoteStatus.accountLocal')}</p>
      {account.authority && <p className={styles.note}>{t('next.remoteStatus.accountAuthority', {authority: account.authority})}</p>}
      {openSection && <Button size="compact" variant="glass" onClick={() => openSection('signin')}>{t('next.remoteStatus.openSignIn')}</Button>}
    </section>}
    {groups}
    {advanced}
    {formState?.unavailable && <Banner action={{label: t('next.states.tryAgain'), onAction: () => {if (source?.current()) void source.load();}}}>{t('next.remoteStatus.failed')}</Banner>}
    {formState?.saveError !== undefined && <Banner>{t('next.remoteStatus.failed')}</Banner>}
    {source && formState?.dirty && <div className={styles.controls}>
      <Button variant="glass" disabled={formState.saving} onClick={() => source.discard(formState)}>{t('action.discard')}</Button>
      <Button variant="primary" loading={formState.saving} onClick={() => {void source.save(formState).catch(() => {});}}>{t('action.save')}</Button>
    </div>}
  </div>;
}
