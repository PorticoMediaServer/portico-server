import {ApiError} from '@core/index.ts';
import {parseHostedSignInStartURL, verifyHostedSignInStart} from '@core/hosted-sign-in-start.ts';
import {completeRegisteredReturn} from '@core/same-tab-return.ts';
import {BrowserProtectedStore} from '../platform/server-connections';
import {servicesTab} from './intake';
import {currentAccountWork, type AccountIdentity, type ServicesAccount} from './session';

export type SignInCapture = Readonly<{href?: string}>;
type Pending = Readonly<{version: 1; href: string}>;
export type SignInStore = Pick<BrowserProtectedStore<Pending>, 'read' | 'change'>;
export type HostedSignIn = Awaited<ReturnType<typeof verifyHostedSignInStart>>;
const invalid = () => new ApiError(400, 'invalid_return', 'invalid_return', false);

export function captureHostedSignIn(page: Pick<Location, 'href' | 'pathname' | 'search' | 'hash'>,
  history: Pick<History, 'state' | 'replaceState'>, origin: string): SignInCapture | undefined {
  if (!page.hash.startsWith('#signIn=')) return;
  const href = page.href;
  history.replaceState(history.state, '', page.pathname + page.search);
  // Shape only. Signature and all signed bindings are checked before rendering.
  return parseHostedSignInStartURL(href, origin) ? {href} : {};
}
export function hostedSignInStore(): SignInStore {
  return new BrowserProtectedStore<Pending>('portico.services-sign-in.v1:' + servicesTab(), 16384, true);
}
export async function restoreHostedSignIn(store: SignInStore, origin: string, capture?: SignInCapture): Promise<HostedSignIn | undefined> {
  if (capture) await store.change(() => undefined);
  const saved = capture ? undefined : await store.read();
  if (saved && saved.version !== 1) {await store.change(() => undefined); throw invalid();}
  const href = capture ? capture.href : saved?.href;
  if (!href) {if (capture) throw invalid(); return;}
  try {
    const parsed = parseHostedSignInStartURL(href, origin);
    if (!parsed) throw invalid();
    const verified = await verifyHostedSignInStart(parsed);
    if (capture) await store.change(() => ({version: 1, href}));
    return verified;
  } catch (error) {await store.change(() => undefined); throw error;}
}

/** The only answer-producing entry point. The view calls this on Continue;
 * restoration and cached-account presentation never call Start or Complete. */
export async function continueHostedSignIn(account: ServicesAccount, pending: HostedSignIn, expected: AccountIdentity,
  store: SignInStore, signal: AbortSignal, navigate: (location: string) => void) {
  const {handoff} = pending;
  const current = async () => {
    signal.throwIfAborted();
    const saved = await store.read();
    const parsed = saved && parseHostedSignInStartURL(saved.href, handoff.hostedOrigin);
    if (!parsed || parsed.context.payload !== handoff.context.payload || parsed.start.state !== handoff.start.state ||
        parsed.start.codeChallenge !== handoff.start.codeChallenge || parsed.context.signature !== handoff.context.signature) throw invalid();
    await verifyHostedSignInStart(parsed);
    signal.throwIfAborted();
  };
  await current();
  const location = await completeRegisteredReturn({request: async <T>(path: string, method = 'GET', body?: unknown) => {
    await current();
    return currentAccountWork(account, session => account.api.send<T>(path, method, body, signal, session.accessToken), expected);
  }}, handoff.start, handoff.registration, signal);
  await current();
  await currentAccountWork(account, async () => {signal.throwIfAborted();}, expected);
  let cleared = false;
  await store.change(saved => {
    if (saved && parseHostedSignInStartURL(saved.href, handoff.hostedOrigin)?.start.state === handoff.start.state) {cleared = true; return undefined;}
    return saved;
  });
  if (!cleared) throw invalid();
  signal.throwIfAborted();
  // No await between the final live-family check and same-tab navigation.
  const session = account.service.getSnapshot().session;
  if (!session || session.account.id !== expected.accountId || session.familyId !== expected.familyId) throw invalid();
  navigate(location);
}
