import React, {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {Link, useNavigate, useSearch} from '@tanstack/react-router';
import {defaultI18n} from '@i18n';
import {AccountSessionReviewService} from '@core/account-sessions';
import {accountApi, type AccountScope} from '../../bridge/account-api';
import {useSession} from '../../app/session';
import {AccountNotificationsClient} from '@core/account-notifications.ts';
import {DataExport, Notifications} from './Notifications';
import {Servers} from './Servers';
import {AccountShell, type AccountSection} from './AccountShell';
import {AccountForm} from '../auth/AccountForm';
import {AuthFrame} from '../auth/AuthFrame';
import {ErrorNotice, errorText} from '../../app/errors';
import {Badge, Button, ConfirmDialog, Dialog, Input, Loading, Notice, SettingsGroup, SettingsPage, SettingsRow, StateView, Text, relative} from '../../ui';
import {AccountSecurity} from './Security';

const t = defaultI18n.t;
// Account profiles and Hosted invitations are gone (Spec — Hosted at Scale): each server keeps its own
// profiles and invitations.
const tabIds = ['sessions', 'security', 'servers', 'notifications'] as const;
export type AccountTab = (typeof tabIds)[number];
const when = (iso: string) => new Date(iso).toLocaleDateString(undefined, {year: 'numeric', month: 'short', day: 'numeric'});
function useService<S extends {subscribe(fn: () => void): () => void; getSnapshot(): unknown; dispose(): void}>(make: () => S, deps: readonly unknown[]): S {
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const service = useMemo(make, deps);
  useEffect(() => () => service.dispose(), [service]);
  return service;
}

/** The Portico Account workspace: the things that belong to the account rather than to any
 * one server. It needs only the account's own sign-in, so it works with no server at all. */
export function AccountWorkspace() {
  const session = useSession();
  const navigate = useNavigate();
  const search = useSearch({strict: false}) as {tab?: AccountTab; identityTransaction?: string};
  // Back from Google or Apple after Connect (`/account?identityTransaction=`): Security finishes it.
  const returning = session.hosted ? search.identityTransaction : undefined;
  const tab = returning ? 'security' : tabIds.some(id => id === search.tab) ? search.tab! : 'sessions';
  const hosted = session.hosted;
  // The feed's unread count labels its tab (BE-hosted: in-app notifications replace alert email).
  const [unread, setUnread] = useState(0);
  const unreadScope = hosted ? `${hosted.account.id}/${hosted.familyId}` : '';
  useEffect(() => {
    if (!hosted) return;
    const controller = new AbortController();
    new AccountNotificationsClient(accountApi({accountId: hosted.account.id, familyId: hosted.familyId}).request).page(undefined, controller.signal).then(p => setUnread(p.unread), () => {});
    return () => controller.abort();
  }, [unreadScope]); // eslint-disable-line react-hooks/exhaustive-deps
  if (!hosted) {
    // Signed out: sign in right here, so the tab survives.
    return (
      <AuthFrame title={t('web.hostedAccount.signInTitle')} lede={t('web.hostedAccount.signInBody')}>
        <AccountForm onSignedIn={() => session.accountSignedIn()} />
      </AuthFrame>
    );
  }
  const scope: AccountScope = {accountId: hosted.account.id, familyId: hosted.familyId};
  const sections: AccountSection[] = [
    {id: 'sessions', label: t('devices.title')},
    {id: 'security', label: t('account.security')},
    {id: 'servers', label: t('web.servers.title')},
    {id: 'notifications', label: t('web.account.notifications'), ...(unread ? {badge: String(unread)} : {})},
  ];
  return (
    <AccountShell username={hosted.account.username} sections={sections} section={tab} onSection={id => void navigate({to: '/account', search: {tab: id as AccountTab}, replace: true})}>
      {tab === 'sessions' ? <Sessions key={scope.familyId} scope={scope} /> : null}
      {tab === 'security' ? <><AccountSecurity key={scope.familyId} scope={scope} returning={returning} onReturned={() => void navigate({to: '/account', search: {tab: 'security'}, replace: true})} /><DataExport key={'x' + scope.familyId} scope={scope} /></> : null}
      {tab === 'servers' ? <Servers key={scope.familyId} scope={scope} /> : null}
      {tab === 'notifications' ? <Notifications key={scope.familyId} scope={scope} onUnread={setUnread} /> : null}
    </AccountShell>
  );
}


type AccountDevice = {id: string; name: string; platform: string; app: string; ip: string; familyId: string; lastSeenAt: string; current: boolean};

/** The sign-in families "Sign out all other devices" ends: every active family except this browser's. */
export function otherActiveIds(items: readonly {id: string; status: string; current: boolean}[]): string[] {
  return items.filter(s => s.status === 'active' && !s.current).map(s => s.id);
}

export type RevokeAllService = {
  getSnapshot(): {phase: string; items: readonly {id: string; status: string; current: boolean}[]; nextCursor: string; mutationError: unknown};
  revoke(id: string): Promise<void>;
  next(): Promise<void>;
};

/**
 * Revoke every other family across all pages. Stops on the first failure
 * (the service keeps the mutation error for retry) or when the list ends.
 * There is no "all others" endpoint (logout-all ends this browser too), so
 * this walks the pages and revokes one family at a time.
 */
export const revokeAllOtherSessions = async (service: RevokeAllService): Promise<boolean> => {
  const seen = new Set<string>();
  for (;;) {
    const snap = service.getSnapshot();
    if (snap.phase !== 'ready') return false;
    const next = otherActiveIds(snap.items).filter(id => !seen.has(id));
    if (!next.length) {
      if (!snap.nextCursor) return !service.getSnapshot().mutationError;
      try { await service.next(); } catch { return false; }
      continue;
    }
    const id = next[0]!;
    seen.add(id);
    try {
      await service.revoke(id);
    } catch {
      // The id was in this snapshot: still active means the revoke failed,
      // gone means something else already ended it.
      const now = service.getSnapshot();
      if (now.phase !== 'ready') return false;
      if (now.items.some(r => r.id === id && r.status === 'active' && !r.current)) return false;
    }
    if (service.getSnapshot().mutationError) return false;
  }
}

function Sessions({scope}: {scope: AccountScope}) {
  const api = useMemo(() => accountApi(scope), [scope.accountId, scope.familyId]);
  const service = useService(() => new AccountSessionReviewService({scope, api}), [api]);
  const state = useSyncExternalStore(service.subscribe, service.getSnapshot);
  const [devices, setDevices] = useState<Record<string, AccountDevice>>({});
  const [ending, setEnding] = useState<string>();
  const [confirmAll, setConfirmAll] = useState(false);
  const [allBusy, setAllBusy] = useState(false);
  const [done, setDone] = useState('');
  const refresh = () => void service.refresh().catch(() => {});
  useEffect(() => {
    refresh();
    // Names are a courtesy on top of the sign-in list; without them the list still works.
    api.request<{items?: AccountDevice[]}>('/v1/account/devices').then(list => setDevices(Object.fromEntries((list.items ?? []).filter(d => d.familyId).map(d => [d.familyId, d]))), () => {});
  }, [service, api]);
  const active = state.items.filter(s => s.status === 'active');
  const others = otherActiveIds(state.items);
  const deviceName = (id: string | undefined) => { const s = active.find(x => x.id === id); return (s && devices[s.id]?.name) || (s?.mode === 'browser' ? t('web.hostedAccount.aBrowser') : t('web.hostedAccount.anApp')); };
  const revoke = async (id: string) => {
    setDone('');
    await service.revoke(id).catch(() => {});
    if (!service.getSnapshot().mutationError) setDone(t('web.hostedAccount.signedOut'));
  };
  // No "sign out others" endpoint (logout-all ends this browser too): revoke each
  // other family on the signed-in session. A failure stops the run and the
  // mutation error below says which state to retry from.
  const revokeOthers = async () => {
    setConfirmAll(false);
    setAllBusy(true);
    setDone('');
    try {
      if (await revokeAllOtherSessions(service)) setDone(t('web.hostedAccount.signedOutOthers'));
    } finally { setAllBusy(false); }
  };
  return (
    <SettingsPage>
      {state.error ? <ErrorNotice error={state.error} context="devices" retry={refresh} /> : null}
      {state.mutationError ? <ErrorNotice error={state.mutationError} context="devices" operation="save" retry={state.retryId ? () => void service.retryRevoke().catch(() => {}) : undefined} /> : null}
      {done && !state.mutationError ? <Notice tone="success" compact>{done}</Notice> : null}
      {state.phase === 'loading' && !state.items.length ? <Loading label={t('status.loadingThing', {thing: t('web.hostedAccount.signInsThing')})} /> : null}
      {state.phase === 'ended' ? <Notice tone="info">{t('web.hostedAccount.ended')} <Link to="/">{t('web.hostedAccount.signInAgain')}</Link></Notice> : null}
      {active.length ? (
        <SettingsGroup title={t('web.hostedAccount.whereSignedIn')} description={t('web.hostedAccount.whereSignedInHelp')} action={others.length ? <Button size="sm" variant="ghost" label={t('web.hostedAccount.signOutOthers')} loading={allBusy} disabled={state.pending !== null || allBusy} onClick={() => setConfirmAll(true)} /> : undefined}>
          {active.map(s => {
            const d = devices[s.id];
            const help = [d?.app && d.platform ? t('web.hostedAccount.appOn', {app: d.app, platform: d.platform}) : d?.app || d?.platform || '', t('web.hostedAccount.signedInOn', {date: when(s.createdAt)}), d?.lastSeenAt ? t('web.hostedAccount.lastUsed', {when: relative(Date.now() - Date.parse(d.lastSeenAt))}) : ''].filter(Boolean).join(' · ');
            return <SettingsRow key={s.id} icon={s.mode === 'browser' ? 'globe' : 'tvDevice'} label={<>{deviceName(s.id)}{s.current ? <> <Badge tone="accent">{t('web.hostedAccount.thisBrowser')}</Badge></> : null}</>} help={help} control={s.current ? undefined : <Button size="sm" variant="ghost" label={t('action.signOut')} loading={state.pending === s.id} disabled={state.pending !== null || allBusy} onClick={() => setEnding(s.id)} />} />;
          })}
        </SettingsGroup>
      ) : null}
      {state.hasPrevious || state.nextCursor ? <div style={{display: 'flex', gap: 8}}><Button size="sm" variant="ghost" label={t('web.hostedAccount.newer')} disabled={!state.hasPrevious} onClick={() => void service.previous().catch(() => {})} /><Button size="sm" variant="ghost" label={t('web.hostedAccount.older')} disabled={!state.nextCursor} onClick={() => void service.next().catch(() => {})} /></div> : null}
      <ConfirmDialog open={!!ending} onOpenChange={o => !o && setEnding(undefined)} title={t('confirm.signOutDevice.title', {device: deviceName(ending)})} body={t('web.hostedAccount.signOutBody')} confirmLabel={t('action.signOut')} destructive onConfirm={() => { const id = ending!; setEnding(undefined); void revoke(id); }} />
      <ConfirmDialog open={confirmAll} onOpenChange={o => !o && !allBusy && setConfirmAll(false)} title={t('web.hostedAccount.signOutOthersTitle')} body={t('web.hostedAccount.signOutOthersBody')} confirmLabel={t('action.signOut')} destructive onConfirm={() => void revokeOthers()} />
    </SettingsPage>
  );
}

/** ONB-09: an invitation link or code shared in a chat can be pasted here; `/join` reviews it. */
