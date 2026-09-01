export type SettingsBlockedNavigation = {
  proceed: () => void;
  reset: () => void;
};

const dirtySources = new Set<string>();
let blockedListener: ((navigation: SettingsBlockedNavigation) => void) | undefined;

export function setSettingsNavigationDirty(next: boolean, source = 'settings-form') {
  if (next) dirtySources.add(source);
  else dirtySources.delete(source);
}

export function isSettingsNavigationDirty() {
  return dirtySources.size > 0;
}

export function isSettingsNavigationSensitive() {
  return dirtySources.has('api-key-token');
}

export function subscribeSettingsBlockedNavigation(listener: (navigation: SettingsBlockedNavigation) => void) {
  blockedListener = listener;
  return () => {
    if (blockedListener === listener) blockedListener = undefined;
  };
}

export function presentSettingsBlockedNavigation(navigation: SettingsBlockedNavigation) {
  if (!blockedListener) {
    navigation.reset();
    return;
  }
  blockedListener(navigation);
}
