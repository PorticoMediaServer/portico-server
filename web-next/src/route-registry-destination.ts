/** Public navigation only. This marker contains no account, username, credential,
 * proof or arbitrary URL, and cannot authorize a Settings read or mutation. */
export const profileSettingsAliasPath = '/settings/profile';
export const profileSettingsDestination = Object.freeze({to: '/settings' as const, search: Object.freeze({scope: 'account' as const, section: 'profile' as const})});
const key = 'portico.web.account-entry-destination.v1';
const marker = 'profile-settings';
type NavigationStorage = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;
export function createEntryDestinationStore(storage: () => NavigationStorage | undefined = () => undefined) {
  let remembered = false;
  const clear = () => {remembered = false;try {storage()?.removeItem(key);} catch { /* Public navigation also works in memory. */ }};
  const hasProfileSettings = () => {
    try {
      const value = storage()?.getItem(key);
      if (value === marker) return true;
      if (value !== undefined && value !== null) {clear();return false;}
    } catch { /* A storage restriction never grants authority or blocks sign-in. */ }
    return remembered;
  };
  return Object.freeze({
    rememberProfileSettings() {remembered = true;try {storage()?.setItem(key, marker);} catch { /* Keep only this tab's public intent. */ }},
    take() {if (!hasProfileSettings()) return;clear();return profileSettingsDestination;},
    clear,
  });
}
// No I/O on import. Each browser origin/tab owns its own sessionStorage marker.
export const browserEntryDestination = createEntryDestinationStore(() => typeof window === 'undefined' ? undefined : window.sessionStorage);
export function isProfileSettingsLocation(location: Readonly<{pathname: string; search?: Record<string, unknown>}>): boolean {
  return location.pathname === '/settings' && location.search?.scope === 'account' && location.search.section === 'profile';
}
