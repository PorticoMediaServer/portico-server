import {BackHandler, Platform, TVEventControl} from 'react-native';

/**
 * One place decides what Back (Menu on the Siri Remote, back on Android) does. Handlers are asked
 * newest-first; the first that is active handles the press. On tvOS the Menu key is only captured
 * while some handler is active, so at the top level Menu leaves the app as Apple TV users expect.
 */
type Handler = {active: () => boolean; handle: () => void; priority?: number};
const handlers: Handler[] = [];

function sync() {
  if (!Platform.isTV || Platform.OS !== 'ios') return;
  const any = handlers.some(h => h.active());
  try {
    if (any) TVEventControl.enableTVMenuKey();
    else TVEventControl.disableTVMenuKey();
  } catch {
    // older runtimes without TVEventControl
  }
}

function onBack() {
  // Highest priority first (overlays, then players, then navigation, then the rail); ties go to the newest.
  const ordered = handlers.map((h, i) => ({h, i})).sort((a, b) => (b.h.priority ?? 0) - (a.h.priority ?? 0) || b.i - a.i);
  for (const {h} of ordered) {
    if (h.active()) {
      h.handle();
      setTimeout(sync, 0);
      return true;
    }
  }
  return false;
}

// BackHandler calls the most recently added listener first; re-subscribing keeps ours ahead of
// the navigation library's own listener so overlays close before screens pop.
let sub: {remove: () => void} | null = null;
function subscribe() {
  sub?.remove();
  sub = BackHandler.addEventListener('hardwareBackPress', onBack);
}
subscribe();

export const back = {
  register(h: Handler) {
    handlers.push(h);
    subscribe();
    sync();
    return () => {
      const i = handlers.indexOf(h);
      if (i >= 0) handlers.splice(i, 1);
      sync();
    };
  },
  sync,
};
