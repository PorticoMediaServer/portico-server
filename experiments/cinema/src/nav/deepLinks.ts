import type {Services} from '../data/services';
import {ui} from '../design/overlays';
import {go} from './routes';
import {screenIndex} from './screenIndex';

let services: Services | null = null;
/** When a link last navigated, so a launch flow that is still running doesn't navigate over it. */
let linkAt = 0;
export const linkNavigatedRecently = () => Date.now() - linkAt < 4000;
export function setDeepLinkServices(s: Services) {
  services = s;
}

/**
 * `portico-cinema://screen/<id>` opens any screen from the screen index (for review);
 * `portico-cinema://join#<code>`, `…/together/join?code=…`, `…/verify-email?token=…` and
 * `…/reset` mirror the web's email and share links.
 */
export function handleDeepLink(url: string) {
  const m = url.match(/^portico-cinema:\/\/([^?#]*)(?:\?([^#]*))?(?:#(.*))?$/);
  if (!m) return;
  const path = m[1].replace(/\/$/, '');
  linkAt = Date.now();
  const query = parseQuery(m[2] ?? '');
  if (path.startsWith('screen/')) {
    const id = decodeURIComponent(path.slice('screen/'.length));
    const entry = screenIndex.find(e => e.id === id);
    if (!entry) return go('NotFound');
    ui.closeAll();
    if (services) void entry.open(services);
    return;
  }
  if (path === 'join') return go('Join', {direct: query.get('direct') === '1'});
  if (path === 'together/join') return go('TogetherJoin', {code: query.get('code') ?? m[3] ?? undefined});
  if (path === 'verify-email') return go('VerifyEmail', {token: query.get('token') ?? undefined});
  if (path === 'reset') return go('ResetPassword');
  if (path === 'device') return go('DeviceApprove');
  go('NotFound');
}

function parseQuery(q: string) {
  const map = new Map<string, string>();
  q.split('&').filter(Boolean).forEach(pair => {
    const [k, v = ''] = pair.split('=');
    map.set(decodeURIComponent(k), decodeURIComponent(v.replace(/\+/g, ' ')));
  });
  return {get: (k: string) => map.get(k) ?? null};
}
