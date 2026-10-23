import type {ShellBootstrap} from './shell/bootstrap';

export function isAccountEntryPath(path: string): boolean {
  return path === '/sign-in' || path === '/profiles' || path === '/servers';
}
type Location = Readonly<{pathname: string}>;
type Navigation = Readonly<{fromLocation?: Location; toLocation: Location}>;
type EntryRouter = Readonly<{
  state: Readonly<{location: Location}>;
  subscribe: (event: 'onBeforeNavigate', listener: (event: Navigation) => void) => () => void;
}>;
/** Account entry retires private media/feeds before its controls can select a
 * successor. Returning starts normal restoration, never a reused private binding. */
export function bindWebAccountEntry(router: EntryRouter, bootstrap: ShellBootstrap): () => void {
  const off = router.subscribe('onBeforeNavigate', event => {
    if (isAccountEntryPath(event.toLocation.pathname)) {
      if (!isAccountEntryPath(event.fromLocation?.pathname ?? '')) bootstrap.suspendForAccountEntry();
    } else if (isAccountEntryPath(event.fromLocation?.pathname ?? '')) {
      void bootstrap.resumeAfterAccountEntry();
    }
  });
  if (isAccountEntryPath(router.state.location.pathname)) bootstrap.suspendForAccountEntry();
  else void bootstrap.start();
  return () => {off();bootstrap.dispose();};
}
