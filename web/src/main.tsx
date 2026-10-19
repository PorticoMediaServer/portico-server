import './app/polyfills';
import './bridge/invitation-location';
import {createRoot} from 'react-dom/client';
import './ui/tokens.css';
import './ui/base.css';
import {App} from './app/App';
import {preloadI18n} from './app/i18n';

// Radix's existing style singleton reads this global before adding its scroll-lock sheet.
// Development has no CSP nonce; production receives a fresh one from the server document.
const styleNonce = document.querySelector<HTMLMetaElement>('meta[name="portico-style-nonce"]')?.content;
if (styleNonce && styleNonce !== '__PORTICO_STYLE_NONCE__') {
  (window as Window & {__webpack_nonce__?: string}).__webpack_nonce__ = styleNonce;
}

// No StrictMode: the headless core services (activation polling, content
// selection, playback) own real side effects and are not written for
// double-invoked effects. Lifecycle correctness is tested against them
// directly instead.
// Below the browser floor, `browser-check.js` has already said so in place of the app.
// The first screen already reads the viewer's language: its words are fetched first.
if (!(window as Window & {__porticoUnsupportedBrowser?: boolean}).__porticoUnsupportedBrowser) void preloadI18n().finally(() => createRoot(document.getElementById('root')!).render(<App />));
