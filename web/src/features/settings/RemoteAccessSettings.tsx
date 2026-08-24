import type { RemoteAccessSettingsPatch, RemoteAccessStatus } from '@porticomediaserver/client-core';
import { CheckCircle2, ExternalLink, Globe2, Link2Off, RefreshCw, ShieldCheck } from '#portico-icons';
import { useEffect, useState } from 'react';
import { PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { reviewedProductErrorText } from '../../components/ProductLanguage';
import {
  ChoiceControl,
  InlineNotice,
  NumberControl,
  SaveBar,
  SettingRow,
  SettingsError,
  SettingsGroup,
  SettingsLoading,
  TextControl,
  ToggleControl,
} from './SettingsControls';
import { useAbortableMutation, useSettingsQuery } from './settingsHooks';
import type { SettingsDataSource, SettingsViewer } from './settingsTypes';
import { trustedSetupClaimURL } from '../../data/httpSource';

const loadRemoteAccess = (source: SettingsDataSource, signal: AbortSignal) => source.remoteAccess(signal);

type RemoteDraft = RemoteAccessSettingsPatch;

function changed<T extends keyof RemoteAccessSettingsPatch>(draft: RemoteDraft, settings: RemoteAccessStatus['settings'], key: T): boolean {
  return Object.prototype.hasOwnProperty.call(draft, key) && draft[key] !== settings[key];
}

export function RemoteAccessSettingsPanel({ source, viewer }: { source: SettingsDataSource; viewer: SettingsViewer }) {
  const [revision, setRevision] = useState(0);
  const query = useSettingsQuery(loadRemoteAccess, source, revision);
  const [draft, setDraft] = useState<RemoteDraft>({});
  const [feedback, setFeedback] = useState('');
  const [error, setError] = useState('');
  const [confirmUnclaim, setConfirmUnclaim] = useState(false);
  const mutation = useAbortableMutation();
  useEffect(() => { setDraft({}); setFeedback(''); setError(''); }, [revision]);
  const set = <K extends keyof RemoteAccessSettingsPatch>(key: K, value: RemoteAccessSettingsPatch[K]) => { setDraft((current) => ({ ...current, [key]: value })); setFeedback(''); setError(''); };
  const save = async () => {
    if (query.status !== 'success') return;
    const patch = Object.fromEntries(Object.entries(draft).filter(([key]) => changed(draft, query.data.settings, key as keyof RemoteAccessSettingsPatch))) as RemoteAccessSettingsPatch;
    setFeedback(''); setError('');
    try {
      await mutation.run((signal) => source.updateRemoteAccess(patch, signal));
      setFeedback('Remote access settings saved.');
      setRevision((current) => current + 1);
    } catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'save remote access settings' })); }
  };
  const action = async (run: (signal: AbortSignal) => Promise<RemoteAccessStatus>, success: string) => {
    setFeedback(''); setError('');
    try { await mutation.run(run); setFeedback(success); setRevision((current) => current + 1); }
    catch (reason) { setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'complete this remote access action' })); }
  };
  if (query.status === 'loading') return <SettingsLoading label="Loading remote access" />;
  if (query.status === 'error') return <SettingsError title="Remote access is unavailable" message={reviewedProductErrorText(query.error, 'settings.load-failed', { sectionName: 'Remote access' })} onRetry={() => setRevision((current) => current + 1)} />;
  const remote = query.data;
  const settings = remote.settings;
  const claimed = settings.claimStatus === 'claimed';
  const canEdit = viewer.role !== 'user';
  const value = <K extends keyof RemoteAccessSettingsPatch>(key: K): RemoteAccessSettingsPatch[K] | RemoteAccessStatus['settings'][K] => Object.prototype.hasOwnProperty.call(draft, key) ? draft[key] : settings[key];
  const dirty = (Object.keys(draft) as Array<keyof RemoteAccessSettingsPatch>).some((key) => changed(draft, settings, key));
  const routeHealthy = Boolean(settings.enabled && remote.connectivity.troubleshootingStatus === 'ok');
  let claimLink: string | undefined;
  try {
    claimLink = remote.claim?.claimUrl ? trustedSetupClaimURL(remote.claim.claimUrl) : undefined;
  } catch {
    claimLink = undefined;
  }

  return <div className="portico-settings-form portico-remote-access">
    <div className={`portico-remote-route-summary ${routeHealthy ? 'healthy' : ''}`}><span>{routeHealthy ? <CheckCircle2 /> : <Globe2 />}</span><div><strong>{routeHealthy ? 'Direct remote access is available' : settings.enabled ? 'Direct route is not currently reachable' : 'Remote access is off'}</strong><p>{remote.publicEndpoint.url || remote.connectivity.troubleshootingHint || settings.lastHeartbeatError || settings.routerMappingError || 'No public endpoint is active.'}</p></div><span>{claimed ? 'Claimed' : 'Local only'}</span></div>
    <InlineNotice tone="info">Portico’s hosted service handles account authorization, discovery, and certificate coordination. Media travels directly between this server and the client; there is no relay fallback.</InlineNotice>
    {!claimed && <SettingsGroup title="Portico claim" description="Link this server to Portico for free direct remote access and hosted sign-in.">
      {remote.claim && claimLink ? <div className="portico-claim-state"><span><strong>Claim is waiting</strong><small>Expires {new Date(remote.claim.expiresAt).toLocaleString()}</small></span><a className="button primary" href={claimLink} target="_blank" rel="noreferrer">Continue at Portico <ExternalLink /></a><SecondaryButton disabled={mutation.busy} onClick={() => void action((signal) => source.cancelRemoteAccessClaim(signal), 'Claim cancelled.')}>Cancel</SecondaryButton></div> : <SettingRow label="Link a Portico account" description="Creates a short-lived claim. Your server credentials and media stay on this server."><PrimaryButton disabled={!canEdit || mutation.busy} onClick={() => void action((signal) => source.startRemoteAccessClaim(signal), 'Claim started. Continue at Portico to approve it.')}><ShieldCheck /> Start claim</PrimaryButton></SettingRow>}
    </SettingsGroup>}
    <SettingsGroup title="Direct access" description="Public route and authentication policy for remote clients.">
      <SettingRow label="Enable remote access" description="Advertise and maintain a secure direct route to this server."><ToggleControl label="Enable remote access" value={Boolean(value('enabled'))} disabled={!canEdit} onChange={(next) => set('enabled', next)} /></SettingRow>
      <SettingRow label="Remote sign-in" description="Preferred identity provider for clients connecting outside the local network."><ChoiceControl label="Remote sign-in" value={String(value('preferredRemoteAuthMode') ?? 'portico')} disabled={!canEdit} options={[{ value: 'portico', label: 'Portico account' }, { value: 'local', label: 'This Server account' }]} onChange={(next) => set('preferredRemoteAuthMode', next as 'portico' | 'local')} /></SettingRow>
      <SettingRow label="Allow manual local sign-in remotely" description="Permit This Server credentials over the direct TLS route. Portico account sign-in remains safer for shared servers."><ToggleControl label="Allow manual local sign-in remotely" value={Boolean(value('allowManualLocalAuthRemoteLogin'))} disabled={!canEdit} onChange={(next) => set('allowManualLocalAuthRemoteLogin', next)} /></SettingRow>
      <SettingRow label="Remote bitrate limit" description="Server-wide playback cap for remote routes; zero means unlimited."><NumberControl label="Remote bitrate limit" value={Number(value('remoteBitrateLimitMbps') ?? 0)} min={0} max={1000} unit="Mbps" disabled={!canEdit} onChange={(next) => next !== undefined && set('remoteBitrateLimitMbps', next)} /></SettingRow>
      <SettingRow label="LAN discovery" description="Allow compatible Portico clients to find this server on the local network."><ToggleControl label="LAN discovery" value={Boolean(value('lanDiscoveryEnabled'))} disabled={!canEdit} onChange={(next) => set('lanDiscoveryEnabled', next)} /></SettingRow>
    </SettingsGroup>
    <SettingsGroup title="Public route" description="Router and DNS controls used to establish the direct connection.">
      <SettingRow label="Public port" description="Use automatic router mapping, a fixed manual port, or disable public listening."><ChoiceControl label="Public port" value={String(value('publicPortMode') ?? 'automatic')} disabled={!canEdit} options={[{ value: 'automatic', label: 'Automatic' }, { value: 'manual', label: 'Manual' }, { value: 'disabled', label: 'Disabled' }]} onChange={(next) => set('publicPortMode', next as 'automatic' | 'manual' | 'disabled')} /></SettingRow>
      {value('publicPortMode') === 'manual' && <SettingRow label="Manual public port" description="External TCP port forwarded to this server."><NumberControl label="Manual public port" value={Number(value('manualPublicPort') ?? 0)} min={1} max={65535} disabled={!canEdit} onChange={(next) => next !== undefined && set('manualPublicPort', next)} /></SettingRow>}
      <SettingRow label="Router automation" description="Allow Portico to maintain a compatible router mapping automatically."><ToggleControl label="Router automation" value={Boolean(value('routerAutomationEnabled'))} disabled={!canEdit} onChange={(next) => set('routerAutomationEnabled', next)} /></SettingRow>
      <SettingRow label="Hosted service URL" description="Portico control-plane address used for claims, discovery, and policy sync."><TextControl label="Hosted service URL" value={String(value('hostedBaseUrl') ?? '')} disabled={!canEdit} onChange={(next) => set('hostedBaseUrl', next)} /></SettingRow>
    </SettingsGroup>
    <SettingsGroup title="Certificate" description="TLS identity for the public direct route.">
      <SettingRow label="Use owner-managed certificate" description="Read a certificate chain and private key from local files instead of managed issuance."><ToggleControl label="Use owner-managed certificate" value={Boolean(value('customCertificateEnabled'))} disabled={!canEdit} onChange={(next) => set('customCertificateEnabled', next)} /></SettingRow>
      {value('customCertificateEnabled') && <><SettingRow label="Certificate chain" description="Absolute path to the owner-managed certificate chain PEM file."><TextControl label="Certificate chain" value={String(value('customCertificatePath') ?? '')} disabled={!canEdit} onChange={(next) => set('customCertificatePath', next)} /></SettingRow><SettingRow label="Private key" description="Absolute path to the private key PEM file. Portico requires restrictive file permissions."><TextControl label="Private key" value={String(value('customCertificateKeyPath') ?? '')} disabled={!canEdit} onChange={(next) => set('customCertificateKeyPath', next)} /></SettingRow></>}
      {!value('customCertificateEnabled') && claimed && <SettingRow label="Managed certificate" description={`${settings.certificateStatus}${settings.certificateExpiresAt ? ` · Expires ${new Date(settings.certificateExpiresAt).toLocaleDateString()}` : ''}`}><SecondaryButton disabled={!canEdit || mutation.busy} onClick={() => void action((signal) => source.renewRemoteAccessCertificate(signal), 'Certificate renewal requested.')}><RefreshCw /> Renew now</SecondaryButton></SettingRow>}
    </SettingsGroup>
    <SaveBar dirty={dirty} busy={mutation.busy} feedback={feedback} error={error} onSave={save} onReset={() => { setDraft({}); setFeedback(''); setError(''); }} />
    {claimed && viewer.role === 'owner' && <SettingsGroup title="Remove Portico claim" description="Disconnect hosted account authorization and the assigned Portico route. Local server accounts continue to work."><SettingRow label="Unclaim this server" description="Existing Portico remote sessions and hosted membership mapping will stop working.">{confirmUnclaim ? <div className="portico-confirm-actions"><SecondaryButton disabled={mutation.busy} onClick={() => setConfirmUnclaim(false)}>Cancel</SecondaryButton><button type="button" className="button secondary portico-destructive-button" disabled={mutation.busy} onClick={() => void action((signal) => source.unclaimRemoteAccess(signal), 'Server claim removed.').then(() => setConfirmUnclaim(false))}><Link2Off /> Unclaim server</button></div> : <SecondaryButton onClick={() => setConfirmUnclaim(true)}><Link2Off /> Unclaim</SecondaryButton>}</SettingRow></SettingsGroup>}
  </div>;
}
