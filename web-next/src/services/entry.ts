import {dispatchNativeAccountReturn} from '../platform/native-account-return';
import {parseJoinLink} from '@core/join-link.ts';
import {captureServicesLink, scrubUnsupportedServicesFragment} from './intake';
import {captureHostedSignIn} from './sign-in';
import {captureHostedOwnerClaim} from './owner-claim';

/** No rendering, storage or account work can precede the Apps-owned native scrub/dispatch. */
export function captureServicesEntry(page: Pick<Location, 'href' | 'replace' | 'pathname' | 'search' | 'hash'>,
  history: Pick<History, 'state' | 'replaceState'>, expectedHostedOrigin: string) {
  const native = dispatchNativeAccountReturn(page, history, expectedHostedOrigin);
  if (native) return {native};
  const signIn = captureHostedSignIn(page, history, expectedHostedOrigin);
  if (signIn) return {signIn};
  const owner = captureHostedOwnerClaim(page, history, expectedHostedOrigin);
  if (owner) return {owner};
  if (page.pathname === '/join' || page.pathname === '/join/') {
    const href = page.href;
    if (page.hash) history.replaceState(history.state, '', page.pathname + page.search);
    // Canonical invitations stay only in this document. A provider navigation requires reopening
    // the invitation; it is never serialized into another continuation or the account store.
    const join = parseJoinLink(href);
    return {join, invalidContinuation: !join};
  }
  const capture = captureServicesLink(page, history);
  return {capture, invalidContinuation: scrubUnsupportedServicesFragment(page, history)};
}
