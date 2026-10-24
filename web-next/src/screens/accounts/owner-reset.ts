import type {BrowserAccountEntry} from '../../platform/account-entry';
import {sameServerPin} from '@core/route-identity.ts';

type Entry = Pick<BrowserAccountEntry, 'getSnapshot' | 'finishLocalOwnerResetPassword'>;
type SignIn = Readonly<{signIn: (username: string, password: string) => Promise<void>}>;

/** The reset grant only changes the password. A successful explicit gesture
 * then starts ordinary sign-in to the same verified server; MFA is unchanged. */
export async function finishOwnerResetAndSignIn(entry: Entry, form: SignIn, password: string, turnOffTwoStep: boolean, isCurrent: () => boolean): Promise<void> {
  const before = entry.getSnapshot();
  if (!isCurrent() || before.busy || before.phase !== 'owner-reset-password' || !before.ownerReset || !before.pin || !before.server || before.server.signIn !== 'local') return;
  const username = before.ownerReset.username;
  if (!await entry.finishLocalOwnerResetPassword(password, turnOffTwoStep) || !isCurrent()) return;
  const after = entry.getSnapshot();
  if (after.phase !== 'password' || after.origin !== before.origin || after.server?.id !== before.server.id || !after.pin || !sameServerPin(before.pin, after.pin)) return;
  await form.signIn(username, password);
}
