import {dispatchNativeAccountReturn} from '../platform/native-account-return';
import {captureServicesLink, scrubUnsupportedServicesFragment} from './intake';

/** No rendering, storage or account work can precede the Apps-owned native scrub/dispatch. */
export function captureServicesEntry(page: Pick<Location, 'href' | 'replace' | 'pathname' | 'search' | 'hash'>,
  history: Pick<History, 'state' | 'replaceState'>, expectedHostedOrigin: string) {
  const native = dispatchNativeAccountReturn(page, history, expectedHostedOrigin);
  if (native) return {native};
  const capture = captureServicesLink(page, history);
  return {capture, invalidContinuation: scrubUnsupportedServicesFragment(page, history)};
}
