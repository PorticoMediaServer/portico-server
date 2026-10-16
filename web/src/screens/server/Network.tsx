import {useEffect} from 'react';
import {checkRemoteAccess, remoteAccessHelp, type RemoteAccess, type RemoteConfiguration} from '@core/remote-access.ts';
import {certificateDate, retryCertificate, type CertificateApi, type CertificateConfig, type CertificateStatus} from '@core/certificates.ts';
import type {ConnectivityStatus} from '@core/server-administration.ts';
import type {CertificateDraft, ConnectivityDraft} from '@core/server-admin/server-forms.ts';
import type {HttpLocalApi} from '@core/index.ts';
import {secureConnectionView} from '@core/server-admin/secure-connection.ts';
import {ClaimConnectButton} from '../auth/ClaimConnectButton';
import {useClaimStatus} from './General';
import {addressKind, connectionLine, connectivityWarning, listedAddresses, mappingProblem, ownCertificateLine, remoteStatusLine} from '@core/server-admin/status-words.ts';
import {sentence, useAction} from '../../admin/console';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Badge, Button, Freshness, KeyValue, Notice, SettingsRow, Status, Surface, Text} from '../../ui';
import {useServerForm} from '../settings/ServerForms';
import s from './Server.module.css';
import page from '../settings/Settings.module.css';

/**
 * Remote access › status: one sentence that answers "can people reach this server from outside?",
 * before any setting. The settings below it are rows of the page's forms.
 */
export function RemoteStatusPanel() {
  const {api} = useSession();
  const {t} = useI18n();
  const remote = useServerForm<RemoteConfiguration, RemoteAccess>('remote');
  const certificate = useServerForm<CertificateConfig, CertificateStatus>('certificate');
  const connectivity = useServerForm<ConnectivityDraft, ConnectivityStatus>('connectivity');
  const action = useAction();
  const st = remote.state.status;
  const {tone, text: sentenceText} = remoteStatusLine(t, {unavailable: remote.state.unavailable, remote: st, certificate: certificate.state.status, connectivity: connectivity.state.status});
  return (
    <Surface>
      <div className={s.panelHead}>
        <div className={page.statusLine}>{st || remote.state.unavailable ? <Status tone={tone}>{sentenceText}</Status> : null}</div>
        {/* Reports to the page's one refresh; checking asks the server to test the route again. */}
        <Freshness at={st?.checkedAt} onRefresh={st ? () => void action.run(async () => { await checkRemoteAccess(api as HttpLocalApi); await remote.form.load(); }) : undefined} refreshing={action.busy} />
      </div>
      {st?.errorCode ? <Notice tone="warning" compact>{remoteAccessHelp(st.errorCode)}</Notice> : null}
      {connectivity.state.status?.warnings.map(w => <Notice key={w} tone="warning" compact>{connectivityWarning(t, w)}</Notice>)}
      {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
    </Surface>
  );
}

/** The certificate's details and any problem with getting it, under the Secure connection status line. */
export function CertificateStatusRow() {
  const {api} = useSession();
  const {t} = useI18n();
  const {form, state} = useServerForm<CertificateConfig, CertificateStatus>('certificate');
  const action = useAction();
  const st = state.status;
  if (!st) return null;
  const certApi: CertificateApi = {requestCertificate: (act, body, signal) => (act === 'status' ? api.request('/v1/networking/certificate', 'GET', undefined, signal) : api.request(`/v1/networking/certificate/${act}`, 'POST', body, signal))};
  return (
    <>
      {st.dnsName || st.notAfter ? <SettingsRow label={t('web.network.certDetails')} stack start control={<KeyValue rows={[...(st.dnsName ? [[t('server.cert.name'), st.dnsName] as [string, string]] : []), ...(st.issuer ? [[t('server.cert.issuer'), st.issuer] as [string, string]] : []), [t('server.cert.expires'), certificateDate(st.notAfter)], [t('server.cert.renews'), certificateDate(st.renewAt)], ...(st.routeUrl ? [[t('server.cert.secureAddress'), st.routeUrl] as [string, string]] : [])]} />} /> : null}
      {st.errorCode ? <div style={{padding: '0 16px 12px'}}><Notice tone="warning" compact action={{label: t('action.tryAgain'), onClick: () => void action.run(async () => { await retryCertificate(certApi, AbortSignal.timeout(15000)); await form.load(); })}}>{t('web.network.certificateProblem')}{st.nextAttemptAt ? ` · ${t('server.cert.nextAttempt', {when: certificateDate(st.nextAttemptAt)})}` : ''}</Notice></div> : null}
    </>
  );
}

/** Home network › what the server answers on, and what the router mapped for it. */
export function AddressesRow() {
  const {t} = useI18n();
  const remote = useServerForm<RemoteConfiguration, RemoteAccess>('remote').state.status;
  const status = useServerForm<ConnectivityDraft, ConnectivityStatus>('connectivity').state.status;
  if (!status?.addresses.length && !remote) return null;
  return (
    <>
      {status?.addresses.length ? <SettingsRow label={t('web.connectivity.answersOn')} stack start control={<div style={{display: 'flex', flexDirection: 'column', gap: 4}}>{listedAddresses(status.addresses).map((a, i) => <Text key={`${i}:${a.address}`} variant="mono" tone="secondary" style={{fontSize: 12}}>{a.address} · {addressKind(t, a.class)}</Text>)}{status.omittedAddresses ? <Text variant="caption" tone="tertiary">{t('web.network.moreAddresses', {count: status.omittedAddresses})}</Text> : null}</div>} /> : null}
      {remote?.mappings.length ? <SettingsRow label={t('web.network.mappings')} stack start control={<KeyValue rows={remote.mappings.map(m => [m.protocol.toUpperCase(), `${sentence(m.state)} · ${t('server.address.port', {port: m.externalPort})}${m.errorCode ? ` · ${mappingProblem(t, m.errorCode)}` : ''}`])} />} /> : null}
    </>
  );
}

/**
 * Home network › where the owner is connected from right now and whether that counts as home:
 * the quickest check that the list below says what was meant. It reads the saved list, so an
 * edit shows here after Save.
 */
export function ConnectionRow() {
  const {t} = useI18n();
  const status = useServerForm<ConnectivityDraft, ConnectivityStatus>('connectivity').state.status;
  const line = connectionLine(t, status?.connection);
  if (!line || !status) return null;
  return <SettingsRow label={t('settings.server.yourConnection')} help={status.connection.plainHttpOutsideHome ? t('web.connectivity.httpOutsideHome') : undefined} state={line.text} control={<Badge tone={status.connection.remote ? 'neutral' : 'healthy'}>{t(status.connection.remote ? 'server.connection.badgeAway' : 'server.connection.badgeHome')}</Badge>} />;
}

/** Your own certificate › whether the server is using the files named below, and until when. */
export function OwnCertificateStatusRow() {
  const i18n = useI18n();
  const {t} = i18n;
  const status = useServerForm<ConnectivityDraft, ConnectivityStatus>('connectivity').state.status;
  const line = ownCertificateLine(t, status?.tls.customCertificate, iso => i18n.date(iso));
  if (!line) return null;
  return <SettingsRow label={t('web.general.status')} control={<Status tone={line.tone}>{line.text}</Status>} />;
}

/**
 * Secure connection › status: whether devices reach this server encrypted, in one sentence, and
 * the one step that turns it on when it isn't (connecting the server to a Portico Account; the
 * certificate then follows by itself). Without a Portico Account the sentence points to the
 * owner's own certificate instead. Plain HTTP keeps working either way.
 */
export function SecureConnectionRow() {
  const session = useSession();
  const {t} = useI18n();
  const certificate = useServerForm<CertificateDraft, CertificateStatus>('certificate');
  const connectivity = useServerForm<ConnectivityDraft, ConnectivityStatus>('connectivity');
  const claim = useClaimStatus();
  const view = secureConnectionView({certificate: certificate.state.status, own: connectivity.state.status?.tls.customCertificate, claim: claim.state?.status});
  const pin = session.api.getRouteConnection()?.pin;
  // While the certificate is on its way, look again every so often; nothing else needs doing.
  const reload = certificate.form.load;
  useEffect(() => {
    if (view.state !== 'starting') return;
    const timer = setInterval(() => void reload(), 15000);
    return () => clearInterval(timer);
  }, [view.state, reload]);
  const connected = () => { claim.refresh(); void certificate.form.load(); };
  return (
    <>
      <SettingsRow label={t('web.general.status')} state={t(view.body, view.address ? {address: view.address} : undefined)} control={<Badge tone={view.tone} dot>{t(view.badge)}</Badge>} />
      {view.canConnect || view.state === 'waiting' ? <SettingsRow label={t('settings.server.secure.connect')} help={t('settings.server.secure.connectHelp')} control={<ClaimConnectButton api={session.api} pin={pin} hostedOrigin={session.hostedUrl} onDone={connected} label={t('settings.server.secure.connect')} />} /> : null}
    </>
  );
}
