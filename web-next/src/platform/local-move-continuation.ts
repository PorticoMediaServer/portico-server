import {createBrowserAccountEntry, type BrowserAccountEntry} from './account-entry';
import {AccountSignInForm} from '../screens/accounts/sign-in-form';

/** One private sign-in entry instance, created outside the retiring Settings view
 * and handed to the sign-in screen so its own challenge, profile and ready states
 * render exactly as for an ordinary sign-in. The grant itself never leaves the
 * custody in memory; no route URL or storage carries it. */
type Continuation = Readonly<{entry: BrowserAccountEntry; form: AccountSignInForm}>;
let pending: Continuation | undefined;

export function startLocalMoveContinuation(): Continuation {
  if (pending) return pending;
  const entry = createBrowserAccountEntry();
  pending = Object.freeze({entry, form: new AccountSignInForm(entry)});
  return pending;
}
/** The sign-in screen takes the single continuation; a second take gets nothing. */
export function takeLocalMoveContinuation(): Continuation | undefined {
  const value = pending; pending = undefined; return value;
}
export function disposeLocalMoveContinuation(): void {
  const value = pending; pending = undefined;
  if (!value) return;
  value.form.dispose(); value.entry.dispose();
}
