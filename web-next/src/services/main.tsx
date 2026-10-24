import {createRoot} from 'react-dom/client';
import '../kit/tokens.css';
import '../kit/fonts.css';
import '../bootstrap.css';
import {IconSprite} from '../kit/Icon';
import {browserAccountOrigin} from '../platform/account-origin';
import {initializeAppInstallation} from '../platform/installation-provider';
import {ServicesApp, ServicesInvalidLink} from './ServicesApp';
import {captureServicesEntry} from './entry';
import {servicesBuildMode} from './mode';

// Scrub a link synchronously, before React, storage or the account singleton is started.
const entry = captureServicesEntry(location, history, browserAccountOrigin(import.meta.env, location));
const nonce = document.querySelector<HTMLMetaElement>('meta[name="portico-style-nonce"]')?.content;
if (nonce && nonce !== '__PORTICO_STYLE_NONCE__') (window as Window & {__webpack_nonce__?: string}).__webpack_nonce__ = nonce;
if (!servicesBuildMode(import.meta.env)) throw new Error('Services entry requires both explicit Hosted build flags.');
if (entry.native !== 'forwarded' && !(window as Window & {__porticoUnsupportedBrowser?: boolean}).__porticoUnsupportedBrowser) {
  // Reuse the MAIN browser-floor shims only in a protected context with real cross-tab locks.
  // Its LAN fallbacks must never substitute for Services credential/continuation protection.
  if (entry.native === 'invalid') createRoot(document.getElementById('root')!).render(<><IconSprite /><ServicesInvalidLink /></>);
  else {
    if (globalThis.isSecureContext && globalThis.crypto?.subtle && navigator.locks) await import('../platform/polyfills');
    // Device metadata only: a failed claim leaves it unset and never blocks sign-in.
    await initializeAppInstallation(import.meta.env.VITE_APP_VERSION ?? '').catch(() => undefined);
    createRoot(document.getElementById('root')!).render(<ServicesApp capture={entry.capture} invalidContinuation={entry.invalidContinuation} join={entry.join} signIn={entry.signIn} owner={entry.owner} />);
  }
}
