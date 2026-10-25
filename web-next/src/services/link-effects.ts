import {ApiError} from '@core/index.ts';
import {parseSecurityResult} from '@core/hosted-security.ts';
import {routeBody, routePath} from '../../../packages/contracts/src/hosted/routes.ts';
import {verificationLink} from './fragments';
import {accountIdentity, currentAccountWork, type ServicesAccount} from './session';

export async function resetPassword(account: Pick<ServicesAccount, 'client'>, token: string,
  password: string, factor: string, signal: AbortSignal): Promise<void> {
  // This endpoint deliberately does not adopt a session after changing the password.
  await account.client.request(routePath('account_recovery_complete'), 'POST',
    routeBody('account_recovery_complete', {token, newPassword: password, factor: factor.trim()}), signal);
}

export type VerificationOutcome = 'confirmed' | 'elsewhere' | 'changed';
export async function verifyEmail(account: ServicesAccount, token: string, signal: AbortSignal): Promise<VerificationOutcome> {
  const link = verificationLink(token);
  if (link?.kind === 'registration') {
    const registration = await account.client.registration(undefined, signal).catch(error => {
      if ((error as {status?: unknown})?.status === 404) return null;
      throw error;
    });
    if (signal.aborted) throw signal.reason;
    if (registration?.registrationId === link.resource) {
      // An email link may not replace a family signed in from another tab.
      await account.service.authenticateIfSignedOut(inner => account.client.verifyRegistration(link.resource, {code: link.code}, AbortSignal.any([signal, inner])));
      return 'confirmed';
    }
  }
  if (link?.kind === 'email_change' && account.service.getSnapshot().session) {
    const session = await currentAccountWork(account, async session => session);
    const identity = accountIdentity(session);
    signal.throwIfAborted();
    const raw = await account.client.request(routePath('account_security_email_confirm'), 'POST',
      routeBody('account_security_email_confirm', {challengeId: link.resource, code: link.code, operationId: crypto.randomUUID()}), signal, session.accessToken);
    signal.throwIfAborted();
    const result = parseSecurityResult(raw, identity);
    if (result.status !== 'email_changed' || !result.signOut) throw new ApiError(502, 'invalid_response', 'invalid_response', false);
    // Hosted invalidates every sign-in. Retire this browser's reviewed family,
    // preserving an account another tab adopted while cleanup was queued.
    await account.service.logoutIfCurrent(identity);
    return 'changed';
  }
  const value = await account.client.request(routePath('email_link_confirm'), 'POST', routeBody('email_link_confirm', {token}), signal);
  const result = value as {kind?: unknown; status?: unknown} | undefined;
  if (!result || !['registration', 'email_change'].includes(String(result.kind)) ||
    !['confirmed', 'already_confirmed', 'email_changed'].includes(String(result.status)))
    throw new ApiError(502, 'invalid_response', 'invalid_response', false);
  return result.status === 'email_changed' ? 'changed' : 'elsewhere';
}
