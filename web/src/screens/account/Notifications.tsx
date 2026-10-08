import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {defaultI18n} from '@i18n';
import {AccountNotificationsClient, ACCOUNT_SECURITY_EVENTS, runAccountExport, type AccountNotification, type NotificationPreferences} from '@core/account-notifications.ts';
import {saveJSON} from '../../app/account-notifications';
import {accountApi, type AccountScope} from '../../bridge/account-api';
import {ErrorNotice, errorText} from '../../app/errors';
import {Button, Loading, Notice, SettingsGroup, SettingsPage, SettingsRow, StateView, Switch, relative, type IconName} from '../../ui';

const t = defaultI18n.t;

/** What a notification says: a title, a body line and its icon, from the kind and its detail. */
export function describeNotification(n: AccountNotification): {title: string; body: string; icon: IconName} {
  const server = n.server?.name ?? '';
  switch (n.kind) {
    case 'server_offline': return {title: t('web.account.notif.serverOffline', {server}), body: t('web.account.notif.serverOfflineBody'), icon: 'server'};
    case 'storage_nearly_full': return {title: t('web.account.notif.storage', {server}), body: t('web.account.notif.storageBody'), icon: 'storage'};
    case 'invitation_accepted': return {title: t('web.account.notif.joined', {server}), body: t('web.account.notif.joinedBody'), icon: 'people'};
    case 'new_device': return {title: t('web.account.notif.newDevice', {name: n.detail.name || n.detail.app || n.detail.platform || ''}), body: [[n.detail.app, n.detail.platform].filter(Boolean).join(' · '), t('web.account.notif.newDeviceBody')].filter(Boolean).join(' '), icon: 'tvDevice'};
    case 'security_event': {
      const event = (ACCOUNT_SECURITY_EVENTS as readonly string[]).includes(n.detail.event ?? '') ? n.detail.event! : '';
      return {title: event ? t(`web.account.notif.event.${event}` as 'web.account.notif.event.password_changed') : t('web.account.notif.security'), body: t('web.account.notif.securityBody'), icon: 'shield'};
    }
    case 'account_export_ready': return {title: t('web.account.notif.exportReady'), body: t('web.account.export.help'), icon: 'download'};
    case 'signing_key_expiring': return {title: t('web.account.notif.keyExpiring'), body: t('web.account.notif.keyExpiringBody', {key: n.detail.keyId ?? '', date: n.detail.expiresAt ? new Date(n.detail.expiresAt).toLocaleDateString() : ''}), icon: 'lock'};
    default: return {title: t('web.account.notif.other'), body: '', icon: 'bell'};
  }
}

/**
 * The account's notification feed (BE-hosted in-app notifications): newest first, 50 a page,
 * unread marked; opening one marks it read and goes where it points (a new sign-in → devices,
 * data ready → the download). Below it, which owner alerts to show.
 */
export function Notifications({scope, onUnread}: {scope: AccountScope; onUnread: (count: number) => void}) {
  const navigate = useNavigate();
  const client = useMemo(() => new AccountNotificationsClient(accountApi(scope).request), [scope.accountId, scope.familyId]); // eslint-disable-line react-hooks/exhaustive-deps
  const [items, setItems] = useState<AccountNotification[]>([]);
  const [cursor, setCursor] = useState<string>();
  const [unread, setUnread] = useState(0);
  const [phase, setPhase] = useState<'loading' | 'ready' | 'error'>('loading');
  const [error, setError] = useState<unknown>();
  const [more, setMore] = useState(false);
  const [notice, setNotice] = useState('');
  const load = useCallback(async () => {
    setPhase('loading');
    try {
      const page = await client.page();
      setItems([...page.items]); setCursor(page.nextCursor); setUnread(page.unread); onUnread(page.unread); setPhase('ready');
    } catch (e) { setError(e); setPhase('error'); }
  }, [client, onUnread]);
  useEffect(() => { void load(); }, [load]);
  const older = async () => {
    if (!cursor) return;
    setMore(true);
    try { const page = await client.page(cursor); setItems(prev => [...prev, ...page.items.filter(i => !prev.some(p => p.id === i.id))]); setCursor(page.nextCursor); }
    catch (e) { setNotice(errorText(e, 'account', 'load')); } finally { setMore(false); }
  };
  const markRead = async (ids: readonly string[] | 'all') => {
    try {
      await client.markRead(ids);
      setItems(prev => prev.map(i => (ids === 'all' || ids.includes(i.id) ? {...i, read: true} : i)));
      const next = ids === 'all' ? 0 : Math.max(0, unread - items.filter(i => !i.read && ids.includes(i.id)).length);
      setUnread(next); onUnread(next);
    } catch (e) { setNotice(errorText(e, 'account', 'save')); }
  };
  const open = (n: AccountNotification) => {
    if (!n.read) void markRead([n.id]);
    if (n.kind === 'new_device' || n.kind === 'security_event') void navigate({to: '/account', search: {tab: 'sessions'}, replace: true});
    else if (n.kind === 'account_export_ready' && n.detail.exportId) void (async () => {
      try { saveJSON(await client.downloadExport(n.detail.exportId!)); setNotice(t('web.account.export.downloaded')); }
      catch (e) { setNotice(errorText(e, 'account', 'load')); }
    })();
  };
  return (
    <SettingsPage>
      {phase === 'error' ? <ErrorNotice error={error} context="notifications" retry={() => void load()} /> : null}
      {notice ? <Notice tone="info" compact>{notice}</Notice> : null}
      {phase === 'loading' && !items.length ? <Loading label={t('status.loadingThing', {thing: t('web.account.notifications').toLowerCase()})} /> : null}
      {phase === 'ready' && !items.length ? <StateView icon="bell" title={t('web.account.notif.empty')} body={t('web.account.notif.emptyBody')} /> : null}
      {items.length ? (
        <SettingsGroup title={t('web.account.notifications')} action={unread ? <Button size="sm" variant="ghost" icon="check" label={t('web.account.notif.markAll')} onClick={() => void markRead('all')} /> : undefined}>
          {items.map(n => {
            const d = describeNotification(n);
            return <SettingsRow key={n.id} icon={d.icon} label={<span style={{fontWeight: n.read ? 500 : 700}}>{d.title}</span>} help={[d.body, relative(Date.now() - Date.parse(n.occurredAt))].filter(Boolean).join(' · ')} meta={n.read ? undefined : t('web.account.notif.unread')} onClick={() => open(n)} />;
          })}
        </SettingsGroup>
      ) : null}
      {cursor ? <div><Button size="sm" variant="ghost" label={t('web.account.notif.older')} loading={more} onClick={() => void older()} /></div> : null}
      <NotificationPrefs client={client} />
    </SettingsPage>
  );
}

function NotificationPrefs({client}: {client: AccountNotificationsClient}) {
  const [prefs, setPrefs] = useState<NotificationPreferences>();
  const [error, setError] = useState('');
  useEffect(() => { client.preferences().then(setPrefs, e => setError(errorText(e, 'account', 'load'))); }, [client]);
  const save = async (next: NotificationPreferences) => {
    const before = prefs;
    setPrefs(next); setError('');
    try { setPrefs(await client.savePreferences(next)); } catch (e) { setPrefs(before); setError(errorText(e, 'account', 'save')); }
  };
  const row = (key: keyof NotificationPreferences, label: string) => {
    if (!prefs) return null;
    return <SettingsRow label={label} control={<Switch checked={prefs[key]} onCheckedChange={v => void save({...prefs, [key]: v})} label={label} />} />;
  };
  return (
    <SettingsGroup title={t('web.account.notif.prefsTitle')} description={t('web.account.notif.prefsHelp')}>
      {error ? <Notice tone="error" compact>{error}</Notice> : null}
      {!prefs && !error ? <Loading /> : null}
      {row('serverOffline', t('web.account.notif.prefServerOffline'))}
      {row('storageNearlyFull', t('web.account.notif.prefStorage'))}
      {row('invitationAccepted', t('web.account.notif.prefJoined'))}
    </SettingsGroup>
  );
}

/** "Download your data": request the export, wait while it's prepared, then save the JSON. No email, no link. */
export function DataExport({scope}: {scope: AccountScope}) {
  const client = useMemo(() => new AccountNotificationsClient(accountApi(scope).request), [scope.accountId, scope.familyId]); // eslint-disable-line react-hooks/exhaustive-deps
  const [phase, setPhase] = useState<'idle' | 'working' | 'pending' | 'done'>('idle');
  const [error, setError] = useState('');
  const operation = useRef<string>(undefined);
  const abort = useRef<AbortController>(undefined);
  useEffect(() => () => abort.current?.abort(), []);
  const run = async () => {
    setPhase('working'); setError('');
    operation.current ??= crypto.randomUUID();
    abort.current = new AbortController();
    try {
      const out = await runAccountExport(client, operation.current, {signal: abort.current.signal});
      if (out.state === 'ready') { saveJSON(out.data); setPhase('done'); operation.current = undefined; }
      else setPhase('pending');
    } catch (e) { setPhase('idle'); setError(errorText(e, 'account', 'action')); }
  };
  return (
    <SettingsGroup title={t('web.account.export.button')} description={t('web.account.export.help')}>
      {error ? <Notice tone="error" compact>{error}</Notice> : null}
      {phase === 'pending' ? <Notice tone="info" compact>{t('web.account.export.preparing')}</Notice> : null}
      {phase === 'done' ? <Notice tone="success" compact>{t('web.account.export.downloaded')}</Notice> : null}
      <SettingsRow icon="download" label={t('web.account.export.button')} control={<Button size="sm" variant="secondary" label={phase === 'working' ? t('web.account.export.working') : t('web.account.export.download')} loading={phase === 'working'} disabled={phase === 'working'} onClick={() => void run()} />} />
    </SettingsGroup>
  );
}
