import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useI18n, type I18n} from '../../app/i18n';
import {AccountNotificationsClient, ACCOUNT_SECURITY_EVENTS, isAccountFeedNotification, runAccountExport, type AccountNotification, type NotificationPreferences} from '@core/account-notifications.ts';
import {saveJSON} from '../../app/account-notifications';
import type {AccountScope} from '../../bridge/account-api';
import {useAccountApi} from './account-hooks';
import {ErrorNotice, errorText} from '../../app/errors';
import {Button, Icon, Loading, Notice, SettingsGroup, SettingsRow, StateView, type IconName} from '../../ui';
import styles from './Notifications.module.css';

/** What a notification says: a title, a body line and its icon, from the kind and its detail. */
export function describeNotification(n: AccountNotification, i18n: I18n): {title: string; body: string; icon: IconName; tone:'amber'|'green'|'blue'}|undefined {
  if(!isAccountFeedNotification(n))return;
  if(n.kind==='server_offline'&&!n.server?.name||n.kind==='new_device'&&!n.detail.name||n.kind==='account_export_ready'&&!n.detail.exportId)return;
  const {t} = i18n;
  const server = n.server?.name ?? '';
  switch (n.kind) {
    case 'server_offline': return {title: t('web.account.notif.serverOffline', {server}), body: t('web.account.notif.serverOfflineBody'), icon: 'server',tone:'amber'};
    case 'new_device': return {title: t('web.account.notif.newDevice', {name: n.detail.name || ''}), body: t('web.account.notif.newDeviceBody'), icon: 'tvDevice',tone:'amber'};
    case 'security_event': {
      const event = (ACCOUNT_SECURITY_EVENTS as readonly string[]).includes(n.detail.event ?? '') ? n.detail.event! : '';
      return {title: event ? t(`web.account.notif.event.${event}` as 'web.account.notif.event.password_changed') : t('web.account.notif.security'), body: t('web.account.notif.securityBody'), icon: 'shield',tone:'blue'};
    }
    case 'account_export_ready': return {title: t('web.account.notif.exportReady'), body: t('web.account.notif.exportReadyBody'), icon: 'download',tone:'green'};
  }
}
/** The feed uses the frame's separate short time column, never app/platform
 * metadata appended to its body. Format in the account's actual locale/zone. */
export function notificationWhen(occurredAt:string,i18n:I18n,now=Date.now()):string{
 const at=Date.parse(occurredAt),age=now-at;
 if(age>=0&&age<3600000)return i18n.relativeTime(at,now);
 const day={year:'numeric',month:'2-digit',day:'2-digit'} as const;
 if(i18n.formatDate(at,day)===i18n.formatDate(now,day))return i18n.t('web.account.notif.today');
 if(age>=0&&age<7*86400000)return i18n.formatDate(at,{weekday:'long'});
 return i18n.date(at,'dayMonth');
}

/**
 * The account's notification feed (BE-hosted in-app notifications): newest first, 50 a page,
 * unread marked; opening one marks it read and goes where it points (a new sign-in → devices,
 * data ready → the download). Below it, one server-reachability switch.
 */
export function Notifications({scope, onUnread}: {scope: AccountScope; onUnread: (count: number) => void}) {
  const i18n = useI18n();
  const {t} = i18n;
  const navigate = useNavigate();
  const api = useAccountApi(scope);
  const client = useMemo(() => new AccountNotificationsClient(api.request), [api]);
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
    <div className={styles.page}>
      <span className={styles.scope}><Icon name="person" size={16}/>{t('web.account.notif.scope')}<span>{t('web.account.notif.scopeHelp')}</span></span>
      <header className={styles.header}><h2>{t('web.account.notifications')}</h2><button className={`${styles.button} ${styles.headerAction}`} type="button" disabled={!unread} onClick={()=>void markRead('all')}><Icon name="check" size={18}/>{t('web.account.notif.markAll')}</button></header>
      {phase === 'error' ? <ErrorNotice error={error} context="notifications" retry={() => void load()} /> : null}
      {notice ? <Notice tone="info" compact>{notice}</Notice> : null}
      {phase === 'loading' && !items.length ? <Loading label={t('status.loadingThing', {thing: t('web.account.notifications').toLowerCase()})} /> : null}
      {phase === 'ready' && !items.length ? <StateView icon="bell" title={t('web.account.notif.empty')} body={t('web.account.notif.emptyBody')} /> : null}
      {items.length ? (
        <section className={styles.feed} aria-label={t('web.account.notifications')}><div className={styles.box}>
          {items.map(n => {
            const d = describeNotification(n, i18n);
            if(!d)return null;
            return <div key={n.id} className={`${styles.item} ${n.read?'':styles.unread}`}>
              <span className={`${styles.icon} ${styles[d.tone]}`}><Icon name={d.icon} size={22}/></span>
              <div><b>{n.kind==='security_event'||n.kind==='server_offline'?<button type="button" className={styles.copyButton} onClick={()=>open(n)}>{d.title}</button>:d.title}</b><p>{d.body}</p>
                {n.kind==='new_device'&&<div className={styles.actions}><button className={styles.button} type="button" onClick={()=>open(n)}>{t('web.account.notif.reviewDevices')}</button></div>}
                {n.kind==='account_export_ready'&&n.detail.exportId&&<div className={styles.actions}><button className={`${styles.button} ${styles.primary}`} type="button" onClick={()=>open(n)}><Icon name="download" size={16}/>{t('web.account.export.download')}</button></div>}
              </div><time className={styles.when} dateTime={n.occurredAt}>{notificationWhen(n.occurredAt,i18n)}</time>
            </div>;
          })}
        </div></section>
      ) : null}
      {cursor ? <div className={styles.pager}><button className={styles.button} type="button" disabled={more} onClick={()=>void older()}>{t('web.account.notif.older')}</button></div> : null}
      <NotificationPrefs client={client} />
    </div>
  );
}

function NotificationPrefs({client}: {client: AccountNotificationsClient}) {
  const {t} = useI18n();
  const [prefs, setPrefs] = useState<NotificationPreferences>();
  const [error, setError] = useState('');
  const saving=useRef(false);
  useEffect(() => { client.preferences().then(setPrefs, e => setError(errorText(e, 'account', 'load'))); }, [client]);
  const save = async (next: NotificationPreferences) => {
    if(saving.current)return;saving.current=true;
    const before = prefs;
    setPrefs(next); setError('');
    try { setPrefs(await client.savePreferences(next)); } catch (e) { setPrefs(before); setError(errorText(e, 'account', 'save')); }finally{saving.current=false;}
  };
  return (
    <section className={styles.preferences} aria-labelledby="account-notify-about">
      <h3 id="account-notify-about">{t('web.account.notif.prefsTitle')}</h3><p className={styles.preferenceHelp}>{t('web.account.notif.prefsHelp')}</p>
      {error ? <Notice tone="error" compact>{error}</Notice> : null}
      {!prefs && !error ? <Loading /> : null}
      {prefs&&<div className={styles.box}><div className={styles.preferenceRow}><span>{t('web.account.notif.prefServerOffline')}</span><button type="button" role="switch" className={styles.switch} aria-label={t('web.account.notif.prefServerOffline')} aria-checked={prefs.serverOffline} onClick={()=>void save({serverOffline:!prefs.serverOffline})}/></div></div>}
    </section>
  );
}

/** "Download your data": request the export, wait while it's prepared, then save the JSON. No email, no link. */
export function DataExport({scope}: {scope: AccountScope}) {
  const {t} = useI18n();
  const api = useAccountApi(scope);
  const client = useMemo(() => new AccountNotificationsClient(api.request), [api]);
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
