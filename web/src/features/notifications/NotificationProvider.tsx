import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import { safeProductMessage, type NotificationAudience, type NotificationInvalidation, type NotificationPage, type NotificationReceiptAction, type ProductMessagePresentation, type ViewerNotification } from '@porticomediaserver/client-core';
import { useAuthSession, usePorticoDataSource } from '../../data/DataProvider';
import { productProblem, productText } from '../../components/ProductLanguage';

export type NotificationState = {
  status: 'loading' | 'ready' | 'error';
  page?: NotificationPage;
  error?: ProductMessagePresentation;
};

type NotificationContextValue = {
  profile: NotificationState;
  accountAdmin?: NotificationState;
  isAccountAdmin: boolean;
  unreadCount: number;
  refresh: () => Promise<void>;
  loadMore: (audience: NotificationAudience) => Promise<void>;
  mutate: (audience: NotificationAudience, ids: string[], action: NotificationReceiptAction) => Promise<void>;
  markAllRead: (audience: NotificationAudience) => Promise<void>;
};

const NotificationContext = createContext<NotificationContextValue | null>(null);

type AudienceLoadOperation = {
  generation: number;
  controller: AbortController;
  cursor?: string;
  recipientKey?: string;
};

function recipientKey(page: NotificationPage) {
  const { authority, accountId, serverId, profileId, audience } = page.recipient;
  return `${authority}:${accountId}:${serverId}:${profileId ?? ''}:${audience}`;
}

function mergePage(current: NotificationPage | undefined, next: NotificationPage, append: boolean): NotificationPage {
  if (!append || !current) return next;
  const seen = new Set(current.items.map((item) => item.id));
  return { ...next, items: [...current.items, ...next.items.filter((item) => !seen.has(item.id))] };
}

export function notificationPresentation(notification: ViewerNotification) {
  const shared = safeProductMessage(notification.messageId, 'notification.fallback-title', notification.interpolation);
  return {
    title: notification.content?.title ?? shared.title ?? shared.text ?? productText('notification.fallback-title'),
    body: notification.content?.body ?? shared.body ?? shared.text ?? '',
  };
}

export function NotificationProvider({ children }: { children: ReactNode }) {
  const source = usePorticoDataSource();
  const auth = useAuthSession();
  const [profile, setProfile] = useState<NotificationState>({ status: 'loading' });
  const [accountAdmin, setAccountAdmin] = useState<NotificationState>();
  const [isAccountAdmin, setIsAccountAdmin] = useState(false);
  const statesRef = useRef({ profile, accountAdmin });
  const loadGenerations = useRef<Record<NotificationAudience, number>>({ profile: 0, 'account-admin': 0 });
  const activeLoads = useRef<Record<NotificationAudience, AudienceLoadOperation | undefined>>({ profile: undefined, 'account-admin': undefined });
  const mutationChains = useRef<Record<NotificationAudience, Promise<void>>>({ profile: Promise.resolve(), 'account-admin': Promise.resolve() });
  statesRef.current = { profile, accountAdmin };
  const profileRecipientKey = profile.page ? recipientKey(profile.page) : '';
  const accountAdminRecipientKey = accountAdmin?.page ? recipientKey(accountAdmin.page) : '';

  const abortAudienceLoads = useCallback(() => {
    activeLoads.current.profile?.controller.abort();
    activeLoads.current['account-admin']?.controller.abort();
    activeLoads.current = { profile: undefined, 'account-admin': undefined };
  }, []);

  const setAudienceError = useCallback((audience: NotificationAudience, error: ProductMessagePresentation) => {
    const apply = (current: NotificationState | undefined): NotificationState => ({ status: 'error', page: current?.page, error });
    if (audience === 'profile') setProfile(apply);
    else setAccountAdmin(apply);
  }, []);

  const loadAudience = useCallback(async (audience: NotificationAudience, append = false) => {
    const current = audience === 'profile' ? statesRef.current.profile : statesRef.current.accountAdmin;
    const cursor = append ? current?.page?.pageInfo.nextCursor ?? undefined : undefined;
    if (append && !cursor) return;
    const expectedRecipientKey = append && current?.page ? recipientKey(current.page) : undefined;
    activeLoads.current[audience]?.controller.abort();
    const controller = new AbortController();
    const operation: AudienceLoadOperation = {
      generation: ++loadGenerations.current[audience],
      controller,
      cursor,
      recipientKey: expectedRecipientKey,
    };
    activeLoads.current[audience] = operation;
    const generationIsCurrent = () => loadGenerations.current[audience] === operation.generation && !controller.signal.aborted;
    const isCurrent = () => activeLoads.current[audience] === operation && generationIsCurrent();
    try {
      const page = await source.viewerNotifications(audience, cursor, controller.signal);
      if (!isCurrent()) return;
      if (page.recipient.audience !== audience || (append && recipientKey(page) !== expectedRecipientKey)) {
        throw new Error('The notification page did not match the requested audience.');
      }
      const apply = (state: NotificationState | undefined): NotificationState => {
        if (!generationIsCurrent()) return state ?? { status: 'loading' };
        if (append && (!state?.page
          || state.page.pageInfo.nextCursor !== operation.cursor
          || recipientKey(state.page) !== operation.recipientKey)) return state ?? { status: 'loading' };
        return { status: 'ready', page: mergePage(state?.page, page, append) };
      };
      if (audience === 'profile') setProfile(apply);
      else setAccountAdmin(apply);
    } catch (reason) {
      if (!isCurrent()) return;
      // Visibility polling and invalidation refreshes are background work.
      // Keep the last valid page instead of flashing an offline banner for a
      // route that may already be recovering. Explicit pagination still gets
      // feedback because the user asked for more content.
      if (!current?.page || append) setAudienceError(audience, productProblem(reason));
      throw reason;
    } finally {
      if (activeLoads.current[audience] === operation) activeLoads.current[audience] = undefined;
    }
  }, [setAudienceError, source]);

  const refresh = useCallback(async () => {
    const requests = [loadAudience('profile')];
    if (isAccountAdmin) requests.push(loadAudience('account-admin'));
    await Promise.all(requests);
  }, [isAccountAdmin, loadAudience]);

  useEffect(() => {
    if (auth.status !== 'ready' || !auth.viewer?.authenticated || !auth.viewerScopeKey) return;
    const controller = new AbortController();
    setProfile((current) => ({ status: 'loading', page: current.page }));
    // Personal/profile notifications are the primary audience. The profile
    // directory is only capability discovery for the optional admin audience,
    // so a directory outage must not prevent the personal inbox from loading.
    void loadAudience('profile').catch(() => undefined);
    void source.accountProfiles(controller.signal).then((directory) => {
      if (controller.signal.aborted) return;
      const primary = directory.profiles.find((item) => item.id === auth.viewer?.viewerScope?.profileId)?.isAccountAdmin === true;
      setIsAccountAdmin(primary);
      if (!primary) {
        setAccountAdmin(undefined);
        return;
      }
      // Admin audience failure is independently represented by its own state;
      // it must not replace or downgrade the already-started personal load.
      return loadAudience('account-admin').catch(() => undefined);
    }).catch(() => {
      if (controller.signal.aborted) return;
      // The current viewer contract does not expose a separate canonical
      // account-admin capability. A failed directory lookup therefore leaves
      // the last known admin state untouched and keeps profile notifications
      // available without guessing at privilege.
    });
    const onVisible = () => { if (document.visibilityState === 'visible') refresh().catch(() => undefined); };
    document.addEventListener('visibilitychange', onVisible);
    return () => {
      controller.abort();
      abortAudienceLoads();
      document.removeEventListener('visibilitychange', onVisible);
    };
  }, [abortAudienceLoads, auth.status, auth.viewer?.authenticated, auth.viewerScopeKey, loadAudience, refresh, setAudienceError, source]);

  useEffect(() => {
    if (auth.status !== 'ready' || !auth.viewer?.authenticated || !auth.viewerScopeKey) return;
    const subscriptions: Array<{ audience: NotificationAudience; page: NotificationPage | undefined }> = [
      { audience: 'profile', page: profile.page },
      ...(isAccountAdmin ? [{ audience: 'account-admin' as const, page: accountAdmin?.page }] : []),
    ];
    const controller = new AbortController();
    for (const { audience, page } of subscriptions) {
      if (!page) continue;
      const onInvalidation = (_event: NotificationInvalidation) => {
        void loadAudience(audience).catch(() => undefined);
      };
      void source.watchViewerNotificationInvalidations(audience, onInvalidation, controller.signal).catch(() => {
        // The shared subscription runtime owns bounded reconnect and continuity
        // repair. Visibility refresh remains a user-lifecycle fallback without
        // multiplying idle requests once per minute.
      });
    }
    return () => controller.abort();
  }, [accountAdminRecipientKey, auth.status, auth.viewer?.authenticated, auth.viewerScopeKey, isAccountAdmin, loadAudience, profileRecipientKey, source]);

  const mutate = useCallback(async (audience: NotificationAudience, ids: string[], action: NotificationReceiptAction) => {
    if (ids.length === 0) return;
    const scheduled = mutationChains.current[audience].catch(() => undefined).then(async () => {
      const state = audience === 'profile' ? statesRef.current.profile : statesRef.current.accountAdmin;
      if (!state?.page) return;
      const controller = new AbortController();
      try {
        const result = await source.updateViewerNotificationReceipts(audience, {
          recipient: state.page.recipient,
          notificationIds: ids,
          action,
          expectedRevision: state.page.revision,
        }, controller.signal);
        const nextState = { ...state, page: { ...state.page, revision: result.revision, unreadCount: result.unreadCount } };
        if (audience === 'profile') statesRef.current.profile = nextState;
        else statesRef.current.accountAdmin = nextState;
        await loadAudience(audience);
      } catch (reason) {
        await loadAudience(audience).catch(() => undefined);
        setAudienceError(audience, productProblem(reason));
        throw reason;
      }
    });
    mutationChains.current[audience] = scheduled.then(() => undefined, () => undefined);
    return scheduled;
  }, [loadAudience, setAudienceError, source]);

  const markAllRead = useCallback(async (audience: NotificationAudience) => {
    const scheduled = mutationChains.current[audience].catch(() => undefined).then(async () => {
      const state = audience === 'profile' ? statesRef.current.profile : statesRef.current.accountAdmin;
      if (!state?.page) return;
      const controller = new AbortController();
      try {
        const result = await source.markAllViewerNotificationsRead(audience, controller.signal);
        const nextState = { ...state, page: { ...state.page, revision: result.revision, unreadCount: result.unreadCount } };
        if (audience === 'profile') statesRef.current.profile = nextState;
        else statesRef.current.accountAdmin = nextState;
        await loadAudience(audience);
      } catch (reason) {
        await loadAudience(audience).catch(() => undefined);
        setAudienceError(audience, productProblem(reason));
        throw reason;
      }
    });
    mutationChains.current[audience] = scheduled.then(() => undefined, () => undefined);
    return scheduled;
  }, [loadAudience, setAudienceError, source]);

  const value = useMemo<NotificationContextValue>(() => ({
    profile,
    accountAdmin,
    isAccountAdmin,
    unreadCount: (profile.page?.unreadCount ?? 0) + (accountAdmin?.page?.unreadCount ?? 0),
    refresh,
    loadMore: (audience) => loadAudience(audience, true),
    mutate,
    markAllRead,
  }), [accountAdmin, isAccountAdmin, loadAudience, markAllRead, mutate, profile, refresh]);

  return <NotificationContext.Provider value={value}>{children}</NotificationContext.Provider>;
}

export function useNotifications() {
  const value = useContext(NotificationContext);
  if (!value) throw new Error('Notifications must be used inside NotificationProvider.');
  return value;
}
