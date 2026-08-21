import { type NotificationAudience } from '@portico/client-core';
import { SemanticProductIcon, productText } from '../../components/ProductLanguage';
import { type NotificationState, useNotifications } from './NotificationProvider';
import { NotificationList } from './NotificationList';
import './notifications.css';

export function NotificationsPage() {
  const notifications = useNotifications();
  const states: Array<{ audience: NotificationAudience; label: string; state: NotificationState | undefined }> = [
    { audience: 'profile', label: productText('notification.for-you-audience'), state: notifications.profile },
    ...(notifications.isAccountAdmin ? [{ audience: 'account-admin' as const, label: productText('notification.account-audience'), state: notifications.accountAdmin }] : []),
  ];
  return <div className="standard-page notifications-page">
    <header className="page-header"><div><p className="route-context">{productText('notification.inbox-context')}</p><h1>{productText('notification.title')}</h1><p>{productText('notification.description')}</p></div><button className="button secondary" type="button" disabled={notifications.unreadCount === 0} onClick={() => void Promise.all(states.map(({ audience }) => notifications.markAllRead(audience))).catch(() => undefined)}><SemanticProductIcon id="action.mark-read" /> {productText('notification.mark-all-read')}</button></header>
    {states.map(({ audience, label, state }) => state && <section key={audience} className="notification-section" aria-labelledby={`notifications-${audience}`}><h2 id={`notifications-${audience}`}>{label}</h2><NotificationList state={state} audience={audience} /></section>)}
  </div>;
}

export default NotificationsPage;
