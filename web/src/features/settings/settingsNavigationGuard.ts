export type SettingsBlockedNavigation = {
  proceed: () => void;
  reset: () => void;
};

let dirty = false;
let blockedListener: ((navigation: SettingsBlockedNavigation) => void) | undefined;

export function setSettingsNavigationDirty(next: boolean) {
  dirty = next;
}

export function isSettingsNavigationDirty() {
  return dirty;
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
