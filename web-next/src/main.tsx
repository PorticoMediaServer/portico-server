import './platform/polyfills';
import {createRoot} from 'react-dom/client';
import './kit/tokens.css';
import './kit/fonts.css';
import './bootstrap.css';
import {App} from './App';
import {ShellBootstrap} from './shell/bootstrap';
import {assembleShellBindings} from './shell/assembly';
import {initializeBrowserInstallation} from './platform/installation';
import {captureWebAccountReturn} from './route-registry-return';

const takeAccountReturn = captureWebAccountReturn(location, history);
const styleNonce = document.querySelector<HTMLMetaElement>('meta[name="portico-style-nonce"]')?.content;
if (styleNonce && styleNonce !== '__PORTICO_STYLE_NONCE__') {
  (window as Window & {__webpack_nonce__?: string}).__webpack_nonce__ = styleNonce;
}
// Core services own real effects; test their lifecycle directly instead of double mounting them.
if (!(window as Window & {__porticoUnsupportedBrowser?: boolean}).__porticoUnsupportedBrowser) {
  initializeBrowserInstallation(import.meta.env.VITE_APP_VERSION ?? '');
  const bootstrap = import.meta.env.DEV && location.pathname.startsWith('/dev/') ? undefined : new ShellBootstrap(assembleShellBindings);
  createRoot(document.getElementById('root')!).render(<App bootstrap={bootstrap} takeAccountReturn={takeAccountReturn} />);
}
