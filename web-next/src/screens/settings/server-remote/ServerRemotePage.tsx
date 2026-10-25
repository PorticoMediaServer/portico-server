import {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import type {ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import {routePath} from '../../../../../contracts/src/routes.ts';
import {loadRemoteAccess, remoteAccessSummary, type RemoteAccess} from '@core/remote-access.ts';
import {parseConnectivityStatus, type ConnectivityStatus} from '@core/server-administration.ts';
import {remoteAccessForm} from '@core/server-admin/server-forms.ts';
import {Button, Switch, TextField} from '../../../kit/controls';
import {SettingNumber} from '../../../kit/settings';
import styles from './server-remote.module.css';

const t = defaultI18n.t;

/** Server › Remote: the section's status facts from the server's own producers,
 * over the canonical settings rows. Certificate and connection rows read the
 * connectivity status; remote reachability reads the remote-access status. */
export function ServerRemotePage({api, isCurrent, groups, advanced}: Readonly<{
  api: Readonly<{request: <T>(path: string, method: string, body?: unknown, signal?: AbortSignal) => Promise<T>}>;
  isCurrent: () => boolean; groups: ReactNode; advanced: ReactNode;
}>) {
  const form = useMemo(() => remoteAccessForm({api} as Parameters<typeof remoteAccessForm>[0]), [api]);
  const formState = useSyncExternalStore(form.subscribe, form.getSnapshot, form.getSnapshot);
  useEffect(() => () => form.dispose(), [form]);
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
    {formState.draft && <section className={styles.controls}>
      <label className={styles.switchRow}><span>{t('web.network.remoteTitle')}<small className={styles.note}>{t('web.network.remoteHelp')}</small></span>
        <Switch aria-label={t('web.network.remoteTitle')} checked={formState.draft.enabled} disabled={formState.saving} onCheckedChange={value => form.set({enabled: value})}/></label>
      <label className={styles.switchRow}><span>{t('web.network.ipv6')}<small className={styles.note}>{t('web.network.ipv6Help')}</small></span>
        <Switch aria-label={t('web.network.ipv6')} checked={formState.draft.ipv6Open} disabled={formState.saving} onCheckedChange={value => form.set({ipv6Open: value})}/></label>
      <label className={styles.switchRow}><span>{t('web.network.mappingTitle')}<small className={styles.note}>{t('web.network.mappingHelp')}</small></span>
        <Switch aria-label={t('web.network.mappingTitle')} checked={formState.draft.mapping} disabled={formState.saving} onCheckedChange={value => form.set({mapping: value})}/></label>
      {formState.draft.mapping && <>
        <label className={styles.switchRow}><span>{t('settings.server.mapping.pcp')}</span>
          <Switch aria-label={t('settings.server.mapping.pcp')} checked={formState.draft.pcp} disabled={formState.saving} onCheckedChange={value => form.set({pcp: value})}/></label>
        <label className={styles.switchRow}><span>{t('settings.server.mapping.natpmp')}</span>
          <Switch aria-label={t('settings.server.mapping.natpmp')} checked={formState.draft.natpmp} disabled={formState.saving} onCheckedChange={value => form.set({natpmp: value})}/></label>
        <label className={styles.switchRow}><span>{t('settings.server.mapping.upnp')}</span>
          <Switch aria-label={t('settings.server.mapping.upnp')} checked={formState.draft.upnp} disabled={formState.saving} onCheckedChange={value => form.set({upnp: value})}/></label>
      </>}
      <div className={styles.numberRow}>
        <SettingNumber label={t('web.network.publicPort')} value={formState.draft.publicPort} min={1} max={65535} disabled={formState.saving}
          onCommit={value => {if (value !== null) form.set({publicPort: Math.round(value)});}}/>
        <TextField label={t('web.network.gateway')} value={formState.draft.gateway} disabled={formState.saving}
          placeholder={t('web.network.gatewayHelp')} onChange={event => form.set({gateway: event.currentTarget.value})}/>
      </div>
      {formState.loadError !== undefined && <p className={styles.error} role="alert">{t('next.remoteStatus.failed')}</p>}
      {formState.dirty && <Button size="compact" variant="primary" loading={formState.saving} onClick={() => {void form.save().catch(() => {});}}>{t('next.remoteStatus.save')}</Button>}
    </section>}
    {groups}
    {advanced}
  </div>;
}
