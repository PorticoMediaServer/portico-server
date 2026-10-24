import {installationIdentity} from '@core/installation.ts';
import {browserSettingsEnvironment} from '@core/presentation/settings-visibility.ts';
import type {SettingsDevicePort, SettingsPlatform} from './settings-platform';

/**
 * The browser is this device. Its name and app version are the real installation claim the
 * server already binds this session to; a browser cannot rename the system device, so the name
 * stays read-only. A browser has no offline downloads store and no self-updater, so the port
 * supplies facts only and no document, apply or restart action.
 */
export function webSettingsPlatform(): SettingsPlatform {
  const device: SettingsDevicePort = {
    subscribe: () => () => {},
    getSnapshot: () => {
      const identity = installationIdentity();
      return identity
        ? {facts: {name: {value: identity.name, readOnly: true}, appVersion: identity.appVersion}}
        : {facts: {}};
    },
  };
  return {environment: browserSettingsEnvironment, device};
}
