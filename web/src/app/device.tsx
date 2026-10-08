import React, {useCallback, useEffect, useMemo, useState} from 'react';
import {useI18n} from './i18n';
import {IdentityClient, announceDevice} from '@core/index.ts';
import {keepBrowserProfilePublished} from '../bridge/client-profile';
import {browserInstallation} from '../bridge/profile-trust';
import {StateView} from '../ui';
import {friendlyHost} from '@core/presentation/index.ts';
import {useSession} from './session';

/** "Chrome on Mac": what a person would call this browser on a list of their devices. */
export function describeBrowser(): string {
  const ua = navigator.userAgent;
  const browser = /Edg\//.test(ua) ? 'Edge' : /OPR\//.test(ua) ? 'Opera' : /Firefox\//.test(ua) ? 'Firefox' : /Chrome\//.test(ua) ? 'Chrome' : /Safari\//.test(ua) ? 'Safari' : 'Web browser';
  const system = /iPhone/.test(ua) ? 'iPhone' : /iPad/.test(ua) ? 'iPad' : /Android/.test(ua) ? 'Android' : /Mac OS X/.test(ua) ? 'Mac' : /Windows/.test(ua) ? 'Windows' : /CrOS/.test(ua) ? 'Chromebook' : /Linux/.test(ua) ? 'Linux' : '';
  return system ? `${browser} on ${system}` : browser;
}

/** What the server said about this browser: approved (or not asked), waiting for the owner, or refused. */
export type DeviceAccess = Readonly<{state: 'pending' | 'denied'; checking: boolean; recheck: () => void}>;

/**
 * After any sign-in, tells the server which device this is, so it appears by name under
 * Devices, can be signed out from elsewhere, and is held for the owner's approval when the
 * server asks for that. A device list that is briefly incomplete never costs anyone their
 * sign-in: only an explicit "waiting for approval" or "refused" from the server stops the app
 * here, and that is shown inside the shell (`blocked`, INT gate 3), never as a page of its own.
 */
export function DeviceGate({children, blocked}: {children: React.ReactNode; blocked: (access: DeviceAccess) => React.ReactNode}) {
  const {api, session} = useSession();
  const identity = useMemo(() => new IdentityClient(api), [api]);
  const [state, setState] = useState<'ok' | DeviceAccess['state']>('ok');
  const [checking, setChecking] = useState(false);
  // Every sign-in is a device on its server, however the viewer proved who they are: the record
  // lives on the server alone, and a Portico Account sign-in never asks Hosted Services about it.
  const family = session?.sessionFamilyId ?? '';
  const local = session?.viewer.authority === 'local';
  const announce = useCallback(async () => {
    if (!family) { setState('ok'); return; }
    const installationId = browserInstallation();
    setChecking(true);
    const outcome = await announceDevice(identity, {installationId, name: describeBrowser(), platform: 'web', app: 'Portico Web', appVersion: import.meta.env.VITE_APP_VERSION ?? ''}, family);
    setChecking(false);
    setState(outcome === 'pending' || outcome === 'denied' ? outcome : 'ok');
    // The sign-in screen can then offer this account by name; nothing stored there signs anyone in.
    if (outcome === 'bound' && local) void identity.rememberAccount(installationId, false).catch(() => {});
  }, [identity, family, local]);
  useEffect(() => { void announce(); }, [announce]);
  // Every sign-in, direct or Portico Account, tells its server what this browser plays, so the
  // server stops converting what would have played as it is.
  // Keyed on the sign-in (session family), not the access token, which rotates every ~13 minutes.
  useEffect(() => (family ? keepBrowserProfilePublished(api, family) : undefined), [api, family]);
  if (state !== 'ok') return <>{blocked({state, checking, recheck: () => void announce()})}</>;
  return <>{children}</>;
}

/** The wait for the owner's approval, or their refusal, centred in the shell's content area. */
export function DeviceBlock({access}: {access: DeviceAccess}) {
  const {signOut, system, serverUrl} = useSession();
  const {t} = useI18n();
  const server = system?.name || friendlyHost(serverUrl) || t('web.settings.serverFallback');
  const denied = access.state === 'denied';
  return (
    <StateView
      icon="shield"
      title={denied ? t('web.device.deniedTitle') : t('device.approval.title')}
      body={denied ? t('web.device.deniedBody', {server}) : t('device.approval.body')}
      action={{label: t('device.approval.checkAgain'), onClick: access.recheck, loading: access.checking}}
      secondaryAction={{label: t('action.signOut'), onClick: signOut}}
    />
  );
}
