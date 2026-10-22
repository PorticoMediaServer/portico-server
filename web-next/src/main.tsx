import './platform/polyfills';
import {createRoot} from 'react-dom/client';
import './kit/tokens.css';
import './kit/fonts.css';
import './bootstrap.css';
import {App} from './App';

const styleNonce = document.querySelector<HTMLMetaElement>('meta[name="portico-style-nonce"]')?.content;
if (styleNonce && styleNonce !== '__PORTICO_STYLE_NONCE__') {
  (window as Window & {__webpack_nonce__?: string}).__webpack_nonce__ = styleNonce;
}
// Core services own real effects; test their lifecycle directly instead of double mounting them.
if (!(window as Window & {__porticoUnsupportedBrowser?: boolean}).__porticoUnsupportedBrowser) {
  createRoot(document.getElementById('root')!).render(<App />);
}
