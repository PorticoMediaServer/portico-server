import { knownSemanticIconId, productMessage, safeProductMessage, type NotificationAction, type NotificationAudience, type ViewerNotification } from '@porticomediaserver/client-core';
import { useNavigate } from 'react-router-dom';
import { ProductMessageIcon, SemanticProductIcon, productText } from '../../components/ProductLanguage';
import { notificationPresentation, type NotificationState, useNotifications } from './NotificationProvider';
import './notifications.css';

function destination(action: NotificationAction, audience: NotificationAudience): string | undefined {
  if (action.kind !== 'navigate') return undefined;
  if (action.target === 'media.detail' && action.parameters.mediaId) return `/media/${encodeURIComponent(action.parameters.mediaId)}`;
  if (action.target === 'notifications') return '/notifications';
  if (action.target === 'account.security') return '/settings/account';
  if (action.target === 'dvr.conflicts') return '/live';
  if (action.target === 'downloads') return '/saved';
  if (audience === 'account-admin' && action.target === 'feedback.detail' && action.parameters.feedbackId) return `/settings/viewer-messages?feedback=${encodeURIComponent(action.parameters.feedbackId)}`;
  return undefined;
}

function NotificationItem({ notification, audience, onDismiss }: { notification: ViewerNotification; audience: NotificationAudience; onDismiss?: () => void }) {
  const notifications = useNotifications();
  const navigate = useNavigate();
  const presentation = notificationPresentation(notification);
  const open = async (action: NotificationAction) => {
    const path = destination(action, audience);
    if (path) {
      if (!notification.readAt) await notifications.mutate(audience, [notification.id], 'mark-read').catch(() => undefined);
      onDismiss?.();
      navigate(path);
      return;
    }
    if (action.target === 'notification.archive') await notifications.mutate(audience, [notification.id], 'archive');
    if (action.target === 'notification.mark-read') await notifications.mutate(audience, [notification.id], 'mark-read');
    if (action.target === 'notification.mark-unread') await notifications.mutate(audience, [notification.id], 'mark-unread');
  };
  const supportedActions = notification.actions.filter((action) => destination(action, audience) || action.target.startsWith('notification.'));
  return <article className={`notification-item ${notification.readAt ? '' : 'unread'}`} data-severity={notification.severity}>
    <div className="notification-glyph" aria-hidden="true"><SemanticProductIcon id={knownSemanticIconId(notification.iconId) ?? 'status.notification'} /></div>
    <div className="notification-copy">
      <div><strong>{presentation.title}</strong><time dateTime={notification.createdAt}>{new Date(notification.createdAt).toLocaleString()}</time></div>
      {presentation.body && <p>{presentation.body}</p>}
      {supportedActions.length > 0 && <div className="notification-actions">{supportedActions.map((action) => {
        const label = safeProductMessage(action.labelMessageId, 'action.open').text ?? productText('action.open');
        return <button key={action.id} type="button" onClick={() => void open(action).catch(() => undefined)}>{label}<SemanticProductIcon id="action.open" /></button>;
      })}</div>}
    </div>
    <div className="notification-receipts">
      <button type="button" aria-label={productText(notification.readAt ? 'action.mark-unread' : 'action.mark-read')} onClick={() => void notifications.mutate(audience, [notification.id], notification.readAt ? 'mark-unread' : 'mark-read').catch(() => undefined)}><SemanticProductIcon id={notification.readAt ? 'action.mark-unread' : 'action.mark-read'} /></button>
      <button type="button" aria-label={productText('action.archive')} onClick={() => void notifications.mutate(audience, [notification.id], 'archive').catch(() => undefined)}><SemanticProductIcon id="action.archive" /></button>
    </div>
  </article>;
}

export function NotificationList({ state, audience, compact = false, onDismiss }: { state: NotificationState; audience: NotificationAudience; compact?: boolean; onDismiss?: () => void }) {
  const notifications = useNotifications();
  const loading = productMessage('notification.loading');
  const empty = productMessage('notification.empty');
  if (state.status === 'loading' && !state.page) return <div className="notification-state" aria-live="polite"><ProductMessageIcon presentation={loading} className="state-spinner" /><span>{loading.title}</span></div>;
  if (state.status === 'error' && !state.page) return <div className="notification-state error" role="alert"><ProductMessageIcon presentation={state.error ?? productMessage('notification.load-failed')} /><span>{state.error?.body ?? productMessage('notification.load-failed').body}</span><button type="button" onClick={() => void notifications.refresh().catch(() => undefined)}>{productText('action.retry')}</button></div>;
  const items = state.page?.items ?? [];
  if (!items.length) return <div className="notification-state"><ProductMessageIcon presentation={empty} /><strong>{empty.title}</strong>{!compact && <span>{empty.body}</span>}</div>;
  return <div className={`notification-list ${compact ? 'compact' : ''}`}>
    {state.status === 'error' && <div className="notification-offline" role="status"><ProductMessageIcon presentation={state.error ?? productMessage('notification.offline')} /> {state.error?.body ?? productMessage('notification.offline').body}</div>}
    {items.map((notification) => <NotificationItem key={notification.id} notification={notification} audience={audience} onDismiss={onDismiss} />)}
    {!compact && state.page?.pageInfo.hasMore && <button className="button secondary notification-more" type="button" onClick={() => void notifications.loadMore(audience).catch(() => undefined)}>{productText('action.load-more')}</button>}
  </div>;
}
