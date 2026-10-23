import {installationIdentity, setInstallationIdentity, setCredentialLock, type InstallationIdentity} from '@core/installation.ts';

export type PlatformInstallationProvider = InstallationIdentity | (() => InstallationIdentity | Promise<InstallationIdentity>);
let installed: Readonly<{provider: PlatformInstallationProvider}> | undefined;

/** Register before importing main. Native owns persistence and actual OS/app facts;
 * registration itself performs no storage, bridge or credential work. */
export function installPlatformInstallationProvider(provider: PlatformInstallationProvider): () => void {
  const captured = installed = {provider};
  return () => {if (installed === captured) installed = undefined;};
}

/** A native provider's absence/failure is never permission to mint a web identity. */
export async function initializeAppInstallation(appVersion: string): Promise<InstallationIdentity | undefined> {
  const captured = installed;
  if (!captured) {
    const {initializeBrowserInstallation} = await import('./installation');
    if (installed) throw new Error('platform_installation_changed');
    return initializeBrowserInstallation(appVersion);
  }
  let identity: InstallationIdentity;
  try {identity = typeof captured.provider === 'function' ? await captured.provider() : captured.provider;}
  catch {throw new Error('platform_installation_unavailable');}
  if (installed !== captured) throw new Error('platform_installation_changed');
  if (!identity || (['installationId', 'name', 'platform', 'app', 'appVersion'] as const).some(key => typeof identity[key] !== 'string')) throw new Error('invalid_platform_installation');
  setInstallationIdentity(identity);
  // The shared web credential vault still uses its accepted cross-context lock.
  setCredentialLock((name, work) => navigator.locks.request(name, work));
  return installationIdentity();
}
