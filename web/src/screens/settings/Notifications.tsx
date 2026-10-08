import {useEffect} from 'react';
import {useNavigate} from '@tanstack/react-router';
import type {InboxView, Notification, NotificationAction, NotificationAudience} from '../../../../packages/client-core/src/index';
import {useInbox} from '../../app/inbox';
import {defaultI18n} from '@i18n';
import {ErrorNotice, ErrorState} from '../../app/errors';
import {Badge, Button, IconButton, Icon, Loading, Segmented, StateView, Surface, Text, type IconName} from '../../ui';
import {useDownloads} from '../../app/downloads';

const t = defaultI18n.t;

const views = (): {id: InboxView; label: string}[] => [{id: 'unread', label: t('notifications.view.unread')}, {id: 'all', label: t('notifications.view.all')}, {id: 'archived', label: t('notifications.view.archived')}];
const audienceLabel = (a: NotificationAudience) => a === 'profile' ? t('notifications.audience.profile') : t('notifications.audience.admin');
const severityIcon: Record<Notification['severity'], IconName> = {info: 'bell', warning: 'warning', critical: 'warning'};

/** Where a notification's "navigate" action goes. An action this build does not know is not
 * offered at all, rather than shown and then failing. */
function destination(action: NotificationAction): {to: string; params?: Record<string, string>; search?: Record<string, never>} | null {
  if (action.kind !== 'navigate' || !action.target) return null;
  const {view, entityId, libraryId} = action.target;
  switch (view) {
    case 'settings': return {to: '/settings'};
    case 'server': return {to: '/settings/$section', params: {section: 'server-dashboard'}};
    case 'home': return {to: '/'};
    case 'library': return libraryId ? {to: '/library/$libraryId', params: {libraryId}} : null;
    case 'item': return entityId ? {to: '/media/$itemId', params: {itemId: entityId}, search: {}} : null;
    default: return null;
  }
}

/** The server-owned inbox for this viewer: messages, not settings. */
export function NotificationsSection() {
  const downloadsOff = useDownloads().state.unavailable === true;
  const {service, state} = useInbox();
  const navigate = useNavigate();
  useEffect(() => { void service?.load(); }, [service]);
  if (!service) return null;
  const empty = state.view === 'unread' ? t('notifications.caughtUp') : state.view === 'archived' ? t('notifications.noneArchived') : t('notifications.none');
  return (
    <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
      <div style={{display: 'flex', flexWrap: 'wrap', gap: 8, alignItems: 'center', justifyContent: 'space-between'}}>
        <div style={{display: 'flex', flexWrap: 'wrap', gap: 8}}>
          {state.audiences.length > 1 ? <Segmented label={t('web.notifications.audience')} value={state.audience} onChange={v => void service.load(v as NotificationAudience, state.view)} options={state.audiences.map(a => ({id: a, label: audienceLabel(a)}))} /> : null}
          <Segmented label={t('web.notifications.show')} value={state.view} onChange={v => void service.load(state.audience, v as InboxView)} options={views()} />
        </div>
        {state.counts.unread > 0 && state.view !== 'archived' ? <Button size="sm" variant="ghost" icon="check" label={t('notifications.markAllRead')} onClick={() => void service.readAll()} /> : null}
      </div>
      {state.error && state.items.length ? <ErrorNotice error={state.error} context="notifications" operation="action" compact retry={() => void service.load()} /> : null}
      {state.phase === 'loading' && !state.items.length ? <Loading label={t('notifications.loading')} /> : null}
      {state.phase === 'error' ? <ErrorState error={state.error} context="notifications" retry={() => void service.load()} /> : null}
      {state.phase === 'ready' && !state.items.length ? <StateView icon="bell" title={empty} body={t(downloadsOff ? 'notifications.emptyBodyNoDownloads' : 'notifications.emptyBody')} /> : null}
      {state.items.length ? (
        <Surface padless>
          {state.items.map((n, index) => (
            <article key={n.id} style={{display: 'flex', gap: 12, padding: '16px 16px', borderTop: index ? '1px solid var(--line-soft)' : undefined, opacity: n.read && state.view !== 'archived' ? 0.72 : 1}}>
              <div style={{paddingTop: 2, color: n.severity === 'info' ? 'var(--text-tertiary)' : n.severity === 'critical' ? 'var(--status-danger)' : 'var(--status-warning)'}}><Icon name={severityIcon[n.severity]} size={18} /></div>
              <div style={{flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', gap: 4}}>
                <div style={{display: 'flex', gap: 8, alignItems: 'baseline', flexWrap: 'wrap'}}>
                  <Text variant="bodyStrong">{n.title}</Text>
                  {!n.read ? <Badge tone="accent" dot>{t('notifications.new')}</Badge> : null}
                  <Text variant="caption" tone="tertiary">{defaultI18n.relativeTime(n.createdAt)}</Text>
                </div>
                {n.body ? <Text as="p" variant="body" tone="secondary" style={{whiteSpace: 'pre-wrap'}}>{n.body}</Text> : null}
                {n.actions.some(a => destination(a)) ? (
                  <div style={{display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 4}}>
                    {n.actions.map((a, i) => { const to = destination(a); return to ? <Button key={i} size="sm" variant="secondary" label={a.label} onClick={() => { void service.apply('read', [n.id]); void navigate(to as never); }} /> : null; })}
                  </div>
                ) : null}
              </div>
              <div style={{display: 'flex', gap: 2, alignItems: 'flex-start'}}>
                {state.view !== 'archived' ? <IconButton name={n.read ? 'bell' : 'check'} label={n.read ? t('notifications.markUnread') : t('notifications.markRead')} variant="ghost" size="sm" onClick={() => void service.apply(n.read ? 'unread' : 'read', [n.id])} /> : null}
                <IconButton name={n.archived ? 'back' : 'close'} label={n.archived ? t('notifications.unarchive') : t('notifications.archive')} variant="ghost" size="sm" onClick={() => void service.apply(n.archived ? 'unarchive' : 'archive', [n.id])} />
              </div>
            </article>
          ))}
        </Surface>
      ) : null}
      {state.more ? <div><Button variant="outline" size="sm" label={t('notifications.loadMore')} loading={state.busy} onClick={() => void service.more()} /></div> : null}
    </div>
  );
}
