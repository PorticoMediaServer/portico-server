import {useEffect, useLayoutEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {noticeWords, type Notification, type NotificationAction} from '@core/notifications.ts';
import type {InboxAction} from '@core/inbox.ts';
import {defineWebScreen} from '../../route-registry';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import {NotificationItem} from '../../kit/shell/AccountSurfaces';
import {NotificationsPanel} from '../../kit/shell/NotificationsPanel';
import {Popover} from '../../kit/overlays';
import {StateView} from '../../kit/states';
import {useShellWidth} from '../../shell/viewport';
import {HomeScreen} from '../home/Home';
import {notificationDestination} from './notification-destination';

const t = defaultI18n.t;
function NotificationsScreen() {
  const captured = useProtectedWebScreenContext();
  return captured ? <BoundNotifications captured={captured} /> : null;
}
function BoundNotifications({captured}: {captured: NonNullable<ReturnType<typeof useProtectedWebScreenContext>>}) {
  const {context, binding} = captured, service = context.inbox, state = useSyncExternalStore(service.subscribe, service.getSnapshot);
  const width = useShellWidth(), phone = width < 900, [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const anchorRef = useMemo(() => ({current: anchor}), [anchor]);
  useLayoutEffect(() => {setAnchor(document.querySelector<HTMLElement>('[data-shell-navigation] a[href="/notifications"]'));}, []);
  // The root owns this shared inbox/badge service. Closing a panel unsubscribes
  // this view; authority retirement disposes the one real service at the root.
  useEffect(() => {if (context.isCurrent()) void service.load();}, [context, service]);
  const close = () => {if (context.isCurrent()) void router.navigate({to: '/'});};
  const apply = (action: InboxAction, note: Notification) => {if (context.isCurrent() && service.getSnapshot().items.includes(note)) void service.apply(action, [note.id]);};
  const open = (note: Notification, action: NotificationAction) => {
    if (!context.isCurrent() || !service.getSnapshot().items.includes(note) || !note.actions.includes(action)) return;
    const target = notificationDestination(action, context.facts.identity.libraries, context.facts.identity.capabilities?.manageServer === true, note);
    if (!target) return;
    void service.apply('read', [note.id]);
    if (target.view === 'home') void router.navigate({to: '/'});
    else if (target.view === 'settings') void router.navigate({to: '/settings'});
    else if (target.view === 'dashboard') void router.navigate({to: '/dashboard'});
    else if (target.view === 'library') void router.navigate({to: '/library/$libraryId', params: {libraryId: target.id}});
    else if (target.view === 'downloads') void router.navigate({to: '/saved', search: {view: 'downloads'}});
    else void router.navigate({to: '/item/$itemId', params: {itemId: target.id}});
  };
  const panel = <NotificationsPanel snapshot={state} onAudience={audience => {if (context.isCurrent()) void service.load(audience, state.view);}}
    onView={view => {if (context.isCurrent()) void service.load(state.audience, view);}} onReadAll={() => {if (context.isCurrent()) void service.readAll();}} onMore={() => {if (context.isCurrent()) void service.more();}}>
    {state.phase === 'error' ? <StateView variant="compact" title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')}
      primary={{id: 'retry', label: t('action.tryAgain'), onPress: () => {if (context.isCurrent()) void service.load();}}} secondary={{id: 'home', label: t('next.states.goHome'), onPress: close}} />
      : state.phase === 'ready' && !state.items.length ? <StateView variant="compact" icon="i-bell" title={t(state.view === 'unread' ? 'notifications.caughtUp' : state.view === 'archived' ? 'notifications.noneArchived' : 'notifications.none')} body={t('notifications.emptyBody')} />
        : state.items.map(note => {
          const words = noticeWords(note, defaultI18n);
          return <NotificationItem key={note.id} title={words.title} body={words.body} unread={!note.read} archived={note.archived} layout={phone ? 'phone' : 'desktop'}
            when={[{id: 'when', role: 'context', text: defaultI18n.relativeTime(note.createdAt)}]} actions={note.actions.map((action, index) => ({id: String(index), label: action.label,
              disabled: !notificationDestination(action, context.facts.identity.libraries, context.facts.identity.capabilities?.manageServer === true, note), onAction: () => open(note, action)}))}
            onMarkRead={() => apply('read', note)} onArchive={() => apply('archive', note)} onMoveToInbox={() => apply('unarchive', note)} />;
        })}
  </NotificationsPanel>;
  if (phone) return panel;
  return <><HomeScreen {...binding.home} />{anchor && <Popover open title={t('notifications.title')} accessibilityLabel={t('notifications.title')} anchorRef={anchorRef} returnFocusRef={anchorRef}
    onOpenChange={value => {if (!value) close();}} modal width={452} sideOffset={10}>{panel}</Popover>}</>;
}
export default defineWebScreen({id: 'notifications', component: NotificationsScreen});
