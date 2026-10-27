import {ApiError} from '@core/index.ts';
import {routeBody, routePath} from '../../../packages/contracts/src/hosted/routes.ts';
import {verificationLink} from './fragments';
import {accountIdentity, type ServicesAccount} from './session';

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
  if (link?.kind === 'email_change') {
    const session = account.service.getSnapshot().session;
    const original = session?.familyId ? accountIdentity(session) : undefined;
    signal.throwIfAborted();
    const result = await account.client.request(routePath('email_link_confirm'), 'POST',
      routeBody('email_link_confirm', {token}), signal) as {kind?: unknown; status?: unknown; accountId?: unknown} | undefined;
    if (!result || result.kind !== 'email_change' || !['email_changed', 'already_confirmed'].includes(String(result.status)) ||
      typeof result.accountId !== 'string' || !/^[A-Za-z0-9_-]{1,128}$/.test(result.accountId))
      throw new ApiError(502, 'invalid_response', 'invalid_response', false);
    // A real new change revoked the affected account's families. Its known ACK
    // retires only the original locally captured family, even after view exit.
    // A receipt replay revokes nobody and must preserve a fresh valid sign-in.
    if (result.status === 'email_changed' && original?.accountId === result.accountId)
      await account.service.logoutIfCurrent(original);
    signal.throwIfAborted();
    return 'changed';
  }
  const value = await account.client.request(routePath('email_link_confirm'), 'POST', routeBody('email_link_confirm', {token}), signal);
  const result = value as {kind?: unknown; status?: unknown} | undefined;
  if (!result || !['registration', 'email_change'].includes(String(result.kind)) ||
    !['confirmed', 'already_confirmed', 'email_changed'].includes(String(result.status)))
    throw new ApiError(502, 'invalid_response', 'invalid_response', false);
  return result.status === 'email_changed' ? 'changed' : 'elsewhere';
}
