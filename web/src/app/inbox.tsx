import React, {createContext, useContext, useEffect, useMemo, useSyncExternalStore} from 'react';
import {InboxService, type InboxSnapshot} from '../../../packages/client-core/src/index';
import {sessionIdentity, useSession} from './session';

const InboxContext = createContext<InboxService | null>(null);

/** One inbox per signed-in session, so the badge and the inbox page share state. The unread
 * count is refreshed when the tab becomes visible and once a minute while it is. */
export function InboxProvider({children}: {children: React.ReactNode}) {
  const {api, session} = useSession();
  const key = sessionIdentity(session);
  const service = useMemo(() => (key ? new InboxService(api) : null), [api, key]);
  useEffect(() => {
    if (!service) return;
    const refresh = () => { if (document.visibilityState === 'visible') void service.refreshUnread(); };
    refresh();
    const timer = setInterval(refresh, 60_000);
    document.addEventListener('visibilitychange', refresh);
    return () => { clearInterval(timer); document.removeEventListener('visibilitychange', refresh); service.dispose(); };
  }, [service]);
  return <InboxContext.Provider value={service}>{children}</InboxContext.Provider>;
}

const idle: InboxSnapshot = Object.freeze<InboxSnapshot>({phase: 'idle', audience: 'profile', audiences: ['profile'], view: 'unread', items: [], counts: {unread: 0, read: 0, archived: 0, total: 0}, unread: 0, revision: 0, more: false, busy: false});
const noSubscribe = () => () => {};
const idleSnapshot = () => idle;

export function useInbox(): {service: InboxService | null; state: InboxSnapshot} {
  const service = useContext(InboxContext);
  const state = useSyncExternalStore(service?.subscribe ?? noSubscribe, service?.getSnapshot ?? idleSnapshot);
  return {service, state};
}

/** Just the number, for the badge: re-renders only when it changes. */
export function useUnreadCount(): number {
  const service = useContext(InboxContext);
  return useSyncExternalStore(service?.subscribe ?? noSubscribe, () => service?.getSnapshot().unread ?? 0);
}
