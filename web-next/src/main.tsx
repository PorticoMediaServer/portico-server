import './platform/polyfills';
import {createRoot} from 'react-dom/client';
import './kit/tokens.css';
import './kit/fonts.css';
import './bootstrap.css';
import {App} from './App';
import {ShellBootstrap} from './shell/bootstrap';
import {assembleShellBindings} from './shell/assembly';
import {initializeAppInstallation} from './platform/installation-provider';
import {captureWebAccountArrival} from './route-registry-arrival';
import {AppLifetime} from './AppLifetime';
import {installSettingsPlatform, settingsPlatform} from './platform/settings-platform';
import {webSettingsPlatform} from './platform/web-settings-platform';
import {IconSprite} from './kit/Icon';
import {LastResortBoundary, StartupFailure} from './shell/Errors';

const accountArrival = captureWebAccountArrival(location, history);
/** A native host installs its own device port before it imports this module; the browser is the fallback. */
const retireSettingsPlatform = settingsPlatform() ? () => {} : installSettingsPlatform(webSettingsPlatform());
const styleNonce = document.querySelector<HTMLMetaElement>('meta[name="portico-style-nonce"]')?.content;
if (styleNonce && styleNonce !== '__PORTICO_STYLE_NONCE__') {
  (window as Window & {__webpack_nonce__?: string}).__webpack_nonce__ = styleNonce;
}
const lifetime = new AppLifetime();
/** Desktop closes the real app before removing native brokers/platform ports.
 * This retires mounted owners, never signs out or clears protected credentials. */
export function disposeApp(): void {
  try { lifetime.dispose(); } finally { try { retireSettingsPlatform(); } finally { accountArrival.dispose(); } }
}
// Core services own real effects; test their lifecycle directly instead of double mounting them.
function start() {
  let failure: {error: unknown} | undefined;
  return lifetime.start(async signal => {
    try { await initializeAppInstallation(import.meta.env.VITE_APP_VERSION ?? '', signal); }
    catch (error) { signal.throwIfAborted(); failure = {error}; }
  }, () => {
    const bootstrap = failure || (import.meta.env.DEV && location.pathname.startsWith('/dev/')) ? undefined : new ShellBootstrap(assembleShellBindings);
    let root: ReturnType<typeof createRoot> | undefined;
    const unmount = () => {try { bootstrap?.dispose(); } finally { root?.unmount(); root = undefined; }};
    try {
      root = createRoot(document.getElementById('root')!);
      root.render(failure
        ? <><IconSprite /><LastResortBoundary><StartupFailure error={failure.error} /></LastResortBoundary></>
        : <App bootstrap={bootstrap} takeAccountReturn={accountArrival.takeAccountReturn} takeAccountMove={accountArrival.takeAccountMove} />);
      return unmount;
    } catch (error) { unmount(); throw error; }
  });
}
if (!(window as Window & {__porticoUnsupportedBrowser?: boolean}).__porticoUnsupportedBrowser) void start();
