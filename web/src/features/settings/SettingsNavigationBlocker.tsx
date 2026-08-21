import { useContext, useEffect } from 'react';
import { UNSAFE_DataRouterContext, useBlocker } from 'react-router-dom';
import { isSettingsNavigationDirty, presentSettingsBlockedNavigation } from './settingsNavigationGuard';

export function SettingsNavigationBlocker() {
  // The production entry point uses a data router, which is required by
  // useBlocker. Several isolated product-surface tests intentionally use the
  // declarative MemoryRouter; leave those surfaces unblocked rather than
  // crashing the entire application tree.
  const dataRouter = useContext(UNSAFE_DataRouterContext);
  if (!dataRouter) return null;
  return <DataRouterSettingsNavigationBlocker />;
}

function DataRouterSettingsNavigationBlocker() {
  const blocker = useBlocker(isSettingsNavigationDirty);
  useEffect(() => {
    if (blocker.state !== 'blocked') return;
    presentSettingsBlockedNavigation({ proceed: blocker.proceed, reset: blocker.reset });
  }, [blocker]);
  return null;
}
