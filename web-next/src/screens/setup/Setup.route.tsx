import {useEffect, useState} from 'react';
import {defineWebScreen} from '../../route-registry';
import {router} from '../../router';
import {browserAccountOrigin} from '../../platform/account-origin';
import {browserSetupDraft} from '../../platform/setup-claim';
import {BrowserSetupComposition} from '../../platform/setup-composition';
import {createBrowserSetupPorts} from './browser-setup';
import {SetupScreen} from './SetupScreen';

function SetupRoute() {
  const [value] = useState(() => {
    const lifetime = new BrowserSetupComposition(browserAccountOrigin(import.meta.env, location));
    const base = createBrowserSetupPorts({draft: browserSetupDraft, acceptLocal: lifetime.acceptLocal,
      continueHosted: lifetime.continueHosted, resumeHosted: lifetime.resumeHosted, addLibrary: lifetime.addLibrary,
      complete: () => {void router.navigate({to: '/', replace: true});}});
    return {lifetime, ports: {...base, connect: (signal: AbortSignal) => lifetime.connect(base.connect, signal)}};
  });
  useEffect(() => () => value.lifetime.dispose(), [value]);
  return <SetupScreen ports={value.ports} />;
}
export default defineWebScreen({id: 'setup', component: SetupRoute, chrome: 'standalone'});
