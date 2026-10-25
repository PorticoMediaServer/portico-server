import type {DirectSignIn} from '@core/profile-management.ts';
import type {ServerPin} from '@core/route-identity.ts';

/** One completed owner move to local, held until the sign-in entry admits it.
 * The grant is single-use and never enters a URL, storage or public state; the
 * issuer facts are the ones captured when the server confirmed the move, and
 * `retire` revokes exactly that fresh grant through its original issuer. */
export type LocalMoveCustody = Readonly<{
  grant: Extract<DirectSignIn, {kind: 'signed-in'}>;
  issuer: Readonly<{origin: string; pin: ServerPin}>;
  serverId: string;
  /** The app's saved-selection revision at confirmation; not the move revision. */
  selectionRevision: number;
  retire: () => Promise<void>;
}>;

/** In-memory, single-use handoff owned outside the retiring Settings view. The
 * sign-in screen is the one entry instance that consumes it, so its exact
 * challenge/profile state stays authoritative; nothing is persisted. */
let pending: LocalMoveCustody | undefined;
export function handOffLocalMove(custody: LocalMoveCustody): void {pending = custody;}
export function takeLocalMove(): LocalMoveCustody | undefined {const value = pending; pending = undefined; return value;}
export function discardLocalMove(): void {void pending?.retire().catch(() => {});pending = undefined;}
