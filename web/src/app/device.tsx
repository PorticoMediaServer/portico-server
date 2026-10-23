import React, {useCallback, useEffect, useMemo} from 'react';
import {IdentityClient, announceDevice} from '@core/index.ts';
import {keepBrowserProfilePublished} from '../bridge/client-profile';
import {browserInstallation} from '../bridge/profile-trust';
import {currentAccessToken, useSession} from './session';

/** "Chrome on Mac": what a person would call this browser on a list of their devices. */
export function describeBrowser(): string {
  const ua = navigator.userAgent;
  const browser = /Edg\//.test(ua) ? 'Edge' : /OPR\//.test(ua) ? 'Opera' : /Firefox\//.test(ua) ? 'Firefox' : /Chrome\//.test(ua) ? 'Chrome' : /Safari\//.test(ua) ? 'Safari' : 'Web browser';
  const system = /iPhone/.test(ua) ? 'iPhone' : /iPad/.test(ua) ? 'iPad' : /Android/.test(ua) ? 'Android' : /Mac OS X/.test(ua) ? 'Mac' : /Windows/.test(ua) ? 'Windows' : /CrOS/.test(ua) ? 'Chromebook' : /Linux/.test(ua) ? 'Linux' : '';
  return system ? `${browser} on ${system}` : browser;
}

/** Publish the signed-in device and playback capabilities to its own server. */
export function DeviceGate({children}: {children: React.ReactNode}) {
  const {api, session, local: account} = useSession();
  const identity = useMemo(() => new IdentityClient(api), [api]);
  // Every sign-in is a device on its server, however the viewer proved who they are: the record
  // lives on the server alone, and a Portico Account sign-in never asks Hosted Services about it.
  const family = session?.sessionFamilyId ?? '';
  const rememberAccount = session?.viewer.authority === 'local' && !!account?.canManage && account.managementFamily === family;
  const announce = useCallback(async () => {
    if (!family) return;
    const token = currentAccessToken();
    const installationId = browserInstallation();
    const outcome = await announceDevice(identity, {installationId, name: describeBrowser(), platform: 'web', app: 'Portico Web', appVersion: import.meta.env.VITE_APP_VERSION ?? ''}, family);
    // The sign-in screen can then offer this account by name; nothing stored there signs anyone in.
    if (outcome === 'bound' && rememberAccount && currentAccessToken() === token) void identity.rememberAccount(installationId, false).catch(() => {});
  }, [identity, family, rememberAccount]);
  useEffect(() => { void announce(); }, [announce]);
  // Every sign-in, direct or Portico Account, tells its server what this browser plays, so the
  // server stops converting what would have played as it is.
  // Keyed on the sign-in (session family), not the access token, which rotates every ~13 minutes.
  useEffect(() => (family ? keepBrowserProfilePublished(api, family) : undefined), [api, family]);
  return <>{children}</>;
}
