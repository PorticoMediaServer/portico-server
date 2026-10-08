import {checkRemoteAccess, remoteAccessHelp, type RemoteAccess, type RemoteConfiguration} from '@core/remote-access.ts';
import {certificateDate, certificateSummary, retryCertificate, type CertificateApi, type CertificateConfig, type CertificateStatus} from '@core/certificates.ts';
import type {ConnectivityStatus} from '@core/server-administration.ts';
import type {ConnectivityDraft} from '@core/server-admin/server-forms.ts';
import type {HttpLocalApi} from '@core/index.ts';
import {addressKind, certificateBadge, connectivityWarning, listedAddresses, mappingProblem, remoteStatusLine} from '@core/server-admin/status-words.ts';
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

/** The certificate's state and details, above its two settings. */
export function CertificateStatusRow() {
  const {api} = useSession();
  const {t} = useI18n();
  const {form, state} = useServerForm<CertificateConfig, CertificateStatus>('certificate');
  const action = useAction();
  const st = state.status;
  if (!st) return null;
  const certApi: CertificateApi = {requestCertificate: (act, body, signal) => (act === 'status' ? api.request('/v1/networking/certificate', 'GET', undefined, signal) : api.request(`/v1/networking/certificate/${act}`, 'POST', body, signal))};
  const badge = certificateBadge(t, st);
  return (
    <>
      <SettingsRow label={t('web.general.status')} state={certificateSummary(st)} control={<Badge tone={badge.tone} dot>{badge.label}</Badge>} />
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
