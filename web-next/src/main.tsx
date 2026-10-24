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

const accountArrival = captureWebAccountArrival(location, history);
const styleNonce = document.querySelector<HTMLMetaElement>('meta[name="portico-style-nonce"]')?.content;
if (styleNonce && styleNonce !== '__PORTICO_STYLE_NONCE__') {
  (window as Window & {__webpack_nonce__?: string}).__webpack_nonce__ = styleNonce;
}
const lifetime = new AppLifetime();
/** Desktop closes the real app before removing native brokers/platform ports.
 * This retires mounted owners, never signs out or clears protected credentials. */
export function disposeApp(): void {
  try { lifetime.dispose(); } finally { accountArrival.dispose(); }
}
// Core services own real effects; test their lifecycle directly instead of double mounting them.
function start() { return lifetime.start(signal => initializeAppInstallation(import.meta.env.VITE_APP_VERSION ?? '', signal), () => {
  const bootstrap = import.meta.env.DEV && location.pathname.startsWith('/dev/') ? undefined : new ShellBootstrap(assembleShellBindings);
  let root: ReturnType<typeof createRoot> | undefined;
  const unmount = () => {try { bootstrap?.dispose(); } finally { root?.unmount(); root = undefined; }};
  try {
    root = createRoot(document.getElementById('root')!);
    root.render(<App bootstrap={bootstrap} takeAccountReturn={accountArrival.takeAccountReturn} takeAccountMove={accountArrival.takeAccountMove} />);
    return unmount;
  } catch (error) { unmount(); throw error; }
}); }
if (!(window as Window & {__porticoUnsupportedBrowser?: boolean}).__porticoUnsupportedBrowser) void start();
