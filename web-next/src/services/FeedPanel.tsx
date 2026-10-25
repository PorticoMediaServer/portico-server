import {HostedError} from '../kit/accounts/ServicesAuth';
import {useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {ServicesEmpty, ServicesFeedRow, ServicesGroup, ServicesWorkspaceRow} from '../kit/accounts/ServicesWorkspace';
import {Button, Switch} from '../kit/controls';
import {ServicesFeed} from './feed';
import {serviceNoticeCopy} from './notification-copy';
import {servicesI18n} from './i18n';
import {servicesNoticeTime} from './time';

export function FeedPanel({feed, onReviewDevices}: {feed: ServicesFeed; onReviewDevices: () => void}) {
  const i18n = useMemo(servicesI18n, []), {t} = i18n;
  const state = useSyncExternalStore(feed.subscribe, feed.getSnapshot);
  const [error, setError] = useState<unknown>(), [downloading, setDownloading] = useState<string>();
  const alive = useRef(true);
  useEffect(() => () => { alive.current = false; }, []);
  const download = async (id: string, noticeId: string) => {
    if (downloading) return;
    setDownloading(id); setError(undefined);
    try {
      const data = await feed.client.downloadExport(id);
      if (!alive.current) return;
      const url = URL.createObjectURL(new Blob([JSON.stringify(data, null, 2)], {type: 'application/json'}));
      const link = document.createElement('a'); link.href = url; link.download = 'portico-account.json'; link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
      await feed.markRead([noticeId]);
    } catch (value) { if (alive.current) setError(value); }
    finally { if (alive.current) setDownloading(undefined); }
  };
  return <>
    {state.error ? <><HostedError error={state.error} /><Button disabled={state.busy} onClick={() => void feed.refresh()}>{t('action.tryAgain')}</Button></> : null}{error ? <HostedError error={error} /> : null}
    {state.loading && !state.error && <p role="status">{t('status.loading')}</p>}
    {!state.loading && !state.error && !state.items.length ? <ServicesGroup><ServicesEmpty title={t('services.workspace.emptyNotifications')} description={t('services.workspace.emptyNotificationsBody')} /></ServicesGroup> : null}
    {state.items.length ? <ServicesGroup>{state.items.map(item => {
      const copy = serviceNoticeCopy(item, i18n);
      const actions = item.kind === 'new_device' ? <Button size="small" disabled={state.busy} onClick={() => { if (!item.read) void feed.markRead([item.id]); onReviewDevices(); }}>{t('services.feed.reviewDevices')}</Button>
        : item.kind === 'account_export_ready' && item.detail.exportId ? <Button size="small" variant="primary" icon="i-download" iconSize={16} loading={downloading === item.detail.exportId} disabled={Boolean(downloading)} onClick={() => void download(item.detail.exportId!, item.id)}>{t('action.download')}</Button> : undefined;
      return <ServicesFeedRow key={item.id} {...copy} unread={!item.read} when={servicesNoticeTime(i18n, item.occurredAt)} actions={actions} />;
    })}</ServicesGroup> : null}
    {state.cursor && <Button size="small" variant="text" disabled={state.busy} onClick={() => void feed.older()}>{t('web.account.notif.older')}</Button>}
    <ServicesGroup title={t('web.account.notif.prefsTitle')} description={t('web.account.notif.prefsHelp')}>
      <ServicesWorkspaceRow title={t('web.account.notif.prefServerOffline')}><Switch checked={state.preferences?.serverOffline ?? false} disabled={!state.preferences || state.busy} aria-label={t('web.account.notif.prefServerOffline')} onCheckedChange={checked => void feed.savePreferences(checked)} /></ServicesWorkspaceRow>
    </ServicesGroup>
  </>;
}
