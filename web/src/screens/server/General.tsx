import {useCallback, useEffect, useMemo, useSyncExternalStore} from 'react';
import {ClaimOnboarding} from '@core/claim-onboarding.ts';
import {ClaimConnectButton} from '../auth/ClaimConnectButton';
import {sentence} from '../../admin/console';
import {useSession} from '../../app/session';
import {Badge, Button, KeyValue, Notice, SettingsGroup, SettingsRow, Text} from '../../ui';
import {defaultI18n} from '@i18n';

const t = defaultI18n.t;

/** General › Identity: what this server is, beside its name. */
export function IdentityRows() {
  const session = useSession();
  return (
    <>
      <SettingsRow label={t('web.general.serverProduct')} meta={session.system?.version ?? '—'} />
      <SettingsRow label={t('web.general.serverId')} meta={<Text variant="mono" tone="tertiary" style={{fontSize: 12}}>{session.system?.id ?? '—'}</Text>} />
    </>
  );
}

/** Remote access › Portico Account: whether this server is connected to its owner's account. */
export function AccountConnectionPanel() {
  const session = useSession();
  const pin = session.api.getRouteConnection()?.pin;
  const claim = useMemo(() => (pin ? new ClaimOnboarding(session.api, pin) : undefined), [session.api, pin]);
  const empty = useMemo(() => ({subscribe: () => () => {}, getSnapshot: () => null}), []);
  const state = useSyncExternalStore(claim?.subscribe ?? empty.subscribe, claim?.getSnapshot ?? empty.getSnapshot);
  useEffect(() => {
    void claim?.start();
    return () => claim?.dispose();
  }, [claim]);
  // Claim rewire (22 Sep): Connect prepares the claim, opens the Portico Account approval page with the claim code and waits for the decision; this panel just refreshes.
  const refreshClaim = useCallback(() => void claim?.refresh(true), [claim]);
  useEffect(() => { const onFocus = () => refreshClaim(); window.addEventListener('focus', onFocus); return () => window.removeEventListener('focus', onFocus); }, [refreshClaim]);
  const status = state?.status;
  const connected = !!status && (status.state === 'installed' || status.state === 'claimed' || status.state === 'connected');
  const pending = !!status && !connected && (status.approvalRequired || !!status.operation);
  return (
    <SettingsGroup title={t('web.general.accountConnection')} description={t('web.general.accountConnectionHelp')}>
      <SettingsRow label={t('web.general.status')} state={status ? (connected ? t('web.general.signedIn') : pending ? t('web.general.approvalWaiting') : t('web.general.notConnected')) : state?.error ? t('server.claim.notConnected') : t('web.general.checking')} control={status ? <Badge tone={connected ? 'healthy' : pending ? 'warning' : 'neutral'} dot>{connected ? t('web.general.connectedBadge') : pending ? t('web.general.approvalPending') : t('web.general.notConnectedBadge')}</Badge> : state?.error ? <ClaimConnectButton api={session.api} pin={pin} hostedOrigin={session.hostedUrl} onDone={refreshClaim} /> : null} />
      {status ? <SettingsRow label={connected ? t('web.general.ownerAccount') : t('web.general.connectAction')} help={connected ? undefined : t('web.general.connectHelp')} meta={connected && status.accountId ? <Text variant="mono" tone="tertiary" style={{fontSize: 12}}>{status.accountId}</Text> : undefined} control={<div style={{display: 'flex', gap: 8}}>
        {status.actions.includes('continue') || pending ? <Button size="sm" variant="secondary" label={t('web.general.checkStatus')} loading={state?.busy} onClick={() => void claim?.refresh(true)} /> : null}
        {status.actions.includes('cancel') ? <Button size="sm" variant="danger" label={t('web.general.cancelRequest')} loading={state?.busy} onClick={() => void claim?.cancel()} /> : null}
        {!connected && !pending ? <ClaimConnectButton api={session.api} pin={pin} hostedOrigin={session.hostedUrl} onDone={refreshClaim} /> : null}
      </div>} /> : null}
      {status && !connected && !pending ? <SettingsRow label={t('web.general.recoveryOwner')} help={t('web.general.recoveryOwnerHelp')} meta={status.installationAcknowledged ? t('web.general.ackOn') : t('web.general.ackOff')} /> : null}
      {status && state?.error ? <div style={{padding: 12}}><Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: () => void claim?.refresh(true)}}>{t('server.claim.checkFailed')}</Notice></div> : null}
      {status ? <details style={{padding: '4px 16px 12px', fontSize: 12.5, color: 'var(--text-tertiary)'}}><summary>{t('server.technicalDetails')}</summary><div style={{marginTop: 8}}><KeyValue rows={[[t('web.general.techState'), sentence(status.state)], [t('web.general.techServer'), status.identity.serverId], [t('web.general.techGeneration'), status.identity.localGeneration], ...(status.operation ? [[t('web.general.techOperation'), status.operation.operationId] as [string, string]] : [])]} /></div></details> : null}
    </SettingsGroup>
  );
}
