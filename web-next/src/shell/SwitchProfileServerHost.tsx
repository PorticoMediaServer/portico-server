import {useEffect, useRef, useState} from 'react';
import type {AppSessionRecord} from '@core/app-sessions.ts';
import {defaultI18n} from '@i18n';
import {AuthError} from '../kit/accounts';
import {SwitchProfileServerDialog, type SwitchProfileServerEntry, type SwitchProfileServerProfile} from '../kit/shell/SwitchProfileServerDialog';
import type {ProfileIdentity} from '../kit/shell';
import {browserAppSessions, observeBrowserSessionChanges} from '../platform/server-connections';
import {browserServerDisplay} from '../platform/server-display';
import {browserAccount} from '../platform/account';
import {accountIdentity} from '../platform/account-work';
import {createAccountServerDirectory} from '../platform/account-server-directory';
import {createAccountServerAccess, type AccountServerAccess} from '../platform/account-server-access';
import type {HostedDirectoryServer} from '../platform/account-entry';
import {createBrowserAccountEntry} from '../platform/account-entry';
import {AccountSignInView} from '../screens/accounts/AccountSignInView';
import {AccountSignInForm} from '../screens/accounts/sign-in-form';

const t = defaultI18n.t;
export type HostedSwitchSelection = Readonly<{server: HostedDirectoryServer; access: AccountServerAccess}>;
export type SwitchProfileServerOpening = Readonly<{
  current: () => boolean; serverId: string; serverName: string; portico: boolean;
  profiles: readonly ProfileIdentity[]; profileId: string;
  selectProfile: (profile: ProfileIdentity) => void;
  selectServer?: (id: string) => Promise<void>;
  selectHosted: (selection: HostedSwitchSelection) => void;
  useLocal: () => void;
}>;
type Choice = Readonly<{view: SwitchProfileServerEntry; select?: () => Promise<void>}>;
type Published = Readonly<{record: AppSessionRecord; servers: readonly Choice[]; locals: readonly Choice[]}>;

/** One actual menu opening. No identity is looked up again by a rendered id.
 * Refresh replaces the whole immutable projection; callbacks retain its entries. */
export function SwitchProfileServerHost({opening, close}: {opening: SwitchProfileServerOpening; close: () => void}) {
  const [published, setPublished] = useState<Published>(), [busy, setBusy] = useState(false), [error, setError] = useState<unknown>();
  const holder = useRef<Published | undefined>(undefined), alive = useRef(false), request = useRef<AbortController | undefined>(undefined), claimed = useRef(false);
  const offAccount = useRef<(() => void) | undefined>(undefined);
  const current = () => alive.current && opening.current();
  const retire = () => {alive.current = false; request.current?.abort(); offAccount.current?.(); offAccount.current = undefined; holder.current = undefined;};
  const dismiss = () => {retire(); close();};
  const refresh = async () => {
    if (!current() || claimed.current) return;
    request.current?.abort(); const operation = request.current = new AbortController();
    const valid = () => current() && request.current === operation && !operation.signal.aborted;
    holder.current = undefined; setPublished(undefined); setBusy(true); setError(undefined);
    try {
      const record = await browserAppSessions().read(); if (!valid()) return;
      const names = await browserServerDisplay().choices(record.servers.map(row => ({signIn: row.signIn, pin: row.connection.pin, origin: row.connection.logicalOrigin})), operation.signal);
      if (!valid()) return;
      const nameById = new Map(names.map(row => [row.id, row.label]));
      const account = opening.portico ? browserAccount() : undefined;
      if (account) await account.service.synchronize();
      if (!valid()) return;
      const session = account?.service.getSnapshot().session;
      const access = account && session?.familyId ? createAccountServerAccess(account, accountIdentity(session)) : undefined;
      offAccount.current?.(); offAccount.current = undefined;
      if (access) offAccount.current = access.observe?.(() => {if (!access.isCurrent()) dismiss();});
      if (!valid()) return;
      let directory: Awaited<ReturnType<ReturnType<typeof createAccountServerDirectory>['read']>> | undefined;
      if (access && account) {
        try {directory = await createAccountServerDirectory(account, accountIdentity(session!)).read(operation.signal);}
        catch (failure) {if (!valid() || !access.isCurrent()) return; setError(failure);}
      }
      if (!valid() || access && !access.isCurrent()) return;
      const servers: Choice[] = [], locals: Choice[] = [];
      const shown = new Set<string>();
      const selectSaved = (id: string) => opening.selectServer ? async () => {await opening.selectServer!(id);} : undefined;
      for (const server of [...(directory?.items ?? [])].sort((a, b) => Number(b.status === 'active') - Number(a.status === 'active'))) {
        const isCurrent = server.id === opening.serverId;
        const detail = isCurrent ? t('web.servers.current') : server.status === 'active' ? t('web.servers.online')
          : server.presence.lastSeenAt ? t('web.chooser.lastSeen', {date: defaultI18n.date(server.presence.lastSeenAt, 'dayMonth')}) : t('web.chooser.neverSeen');
        const saved = record.servers.find(row => row.connection.pin.serverId === server.id && row.signIn === 'portico' && row.connection.pin.publicKey === server.publicKey);
        const immutable = Object.freeze({...server, ...(server.routes ? {routes: structuredClone(server.routes)} : {})});
        servers.push(Object.freeze({view: Object.freeze({id: server.id, name: server.name, detail, current: isCurrent,
          detailTone: server.status === 'active' && !isCurrent ? 'online' : undefined, dimmed: server.status !== 'active', available: true}),
          select: saved && opening.selectServer ? selectSaved(server.id) : access ? async () => {
            if (!access.isCurrent()) throw new DOMException('Account changed', 'AbortError');
            opening.selectHosted(Object.freeze({server: immutable, access}));
          } : undefined}));
        shown.add(server.id);
      }
      for (const saved of record.servers) {
        const id = saved.connection.pin.serverId;
        if (shown.has(id)) continue;
        const name = id === opening.serverId ? opening.serverName : nameById.get(id); if (!name) continue;
        const isCurrent = id === opening.serverId;
        const choice = Object.freeze({view: Object.freeze({id, name, current: isCurrent, available: !!opening.selectServer,
          detail: isCurrent ? t('web.servers.current') : t(saved.signIn === 'local' ? 'accounts.session.local' : 'accounts.session.portico')}), select: selectSaved(id)});
        (saved.signIn === 'local' ? locals : servers).push(choice);
      }
      const value = Object.freeze({record, servers: Object.freeze(servers), locals: Object.freeze(locals)});
      if (!valid()) return; holder.current = value; setPublished(value);
    } catch (failure) {if (valid()) setError(failure);}
    finally {if (valid()) setBusy(false);}
  };
  useEffect(() => {
    alive.current = true; void refresh();
    const off = observeBrowserSessionChanges(() => {if (!claimed.current) void refresh();});
    return () => {retire(); off();};
  }, [opening]);
  const choose = async (entry: SwitchProfileServerEntry) => {
    const value = published, choice = value && [...value.servers, ...value.locals].find(row => row.view === entry);
    if (!current() || !value || holder.current !== value || claimed.current || busy || !choice?.select || entry.current || !entry.available) return;
    claimed.current = true; setBusy(true); setError(undefined);
    try {
      const fresh = await browserAppSessions().read();
      if (!current() || holder.current !== value || fresh.revision !== value.record.revision || fresh.selectedServerId !== value.record.selectedServerId) return;
      await choice.select();
      if (current()) dismiss();
    } catch (failure) {if (current()) setError(failure);}
    finally {claimed.current = false; if (current()) setBusy(false);}
  };
  const profiles: readonly SwitchProfileServerProfile[] = Object.freeze(opening.profiles.map(profile => Object.freeze({...profile, current: profile.id === opening.profileId})));
  return <SwitchProfileServerDialog open onClose={dismiss} profilesLabel={t('web.account.profilesOnServer', {server: opening.serverName})}
    profiles={profiles} servers={published?.servers.map(row => row.view)} localServers={published?.locals.map(row => row.view)} busy={busy}
    error={error ? <AuthError error={error} /> : undefined}
    onSelectProfile={profile => {
      if (!current() || busy || claimed.current) return;
      const index = profiles.indexOf(profile), original = opening.profiles[index];
      if (!original || profile.current) return;
      claimed.current = true; opening.selectProfile(original); dismiss();
    }} onSelectServer={entry => {void choose(entry);}}
    onRefreshServers={() => {void refresh();}} onUseLocalServer={() => {if (current() && !busy && !claimed.current) {claimed.current = true; opening.useLocal(); dismiss();}}} />;
}

/** Transfer a captured Hosted row to the existing canonical sign-in state
 * machine. It owns route proof, membership, challenges and profile selection. */
export function HostedServerOpening({selection, cancel}: {selection: HostedSwitchSelection; cancel: () => void}) {
  const [instance, setInstance] = useState<Readonly<{entry: ReturnType<typeof createBrowserAccountEntry>; form: AccountSignInForm}>>();
  useEffect(() => {
    const entry = createBrowserAccountEntry(), form = new AccountSignInForm(entry);
    setInstance({entry, form}); void entry.openHostedServer(selection.server, selection.access);
    return () => {form.dispose(); entry.dispose();};
  }, [selection]);
  return instance ? <AccountSignInView {...instance} initialize={false} onCancel={cancel} /> : null;
}
