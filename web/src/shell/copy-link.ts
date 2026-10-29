import {copyText} from '../platform/copy-text';
import {router} from '../router';

/** A registered route and its params/search, as `router.navigate` takes them. */
type Destination = Readonly<{to: string; params?: Readonly<Record<string, string>>; search?: Readonly<Record<string, unknown>>}>;
/** O-161: the full shareable address of a place in the app. On Hosted it names this server
 * (`/s/<serverId>/…`); a server's own UI uses its plain path. Never a server address. */
export function shareableLink(destination: Destination): string {
  return new URL(router.buildLocation(destination as never).publicHref, location.origin).href;
}
/** Call from the menu choice itself: the copy must run inside the person's gesture. */
export function copyLink(destination: Destination, source?: HTMLElement): Promise<void> {
  return copyText(shareableLink(destination), source);
}
