import {useCallback, useEffect, useMemo, useState} from 'react';
import {defaultI18n} from '@i18n';
import {accountApi, type AccountScope} from '../../bridge/account-api';
import {useSession} from '../../app/session';
import {ErrorNotice, errorText} from '../../app/errors';
import {Button, ConfirmDialog, Loading, Notice, SettingsGroup, SettingsPage, SettingsRow, StateView, relative} from '../../ui';

const t = defaultI18n.t;

export type AccountServer = {id: string; name?: string; ownedByMe?: boolean; online?: boolean; lastSeenAt?: string | null};

const str = (v: unknown, max = 256): v is string => typeof v === 'string' && v.length > 0 && v.length <= max;
const stamp = (v: unknown): string | null => (typeof v === 'string' && v.length <= 64 && Number.isFinite(Date.parse(v)) ? v : null);

/**
 * Parse `GET /v1/servers` defensively: unknown fields are ignored and malformed
 * rows are skipped, so one bad record never fails the screen (§4a: parsers
 * degrade gracefully).
 */
export function parseAccountServers(value: unknown): AccountServer[] {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return [];
  const items = (value as {items?: unknown}).items;
  if (!Array.isArray(items)) return [];
  const out: AccountServer[] = [];
  for (const raw of items) {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) continue;
    const r = raw as Record<string, unknown>;
    if (!str(r.id, 128)) continue;
    const presence = r.presence && typeof r.presence === 'object' && !Array.isArray(r.presence) ? (r.presence as Record<string, unknown>) : undefined;
    out.push({
      id: r.id,
      ...(str(r.name) ? {name: r.name as string} : {}),
      ...(typeof r.ownedByMe === 'boolean' ? {ownedByMe: r.ownedByMe as boolean} : {}),
      ...(presence && typeof presence.online === 'boolean' ? {online: presence.online as boolean} : {}),
      lastSeenAt: stamp(presence?.lastSeenAt) ?? stamp(r.lastSeenAt),
    });
  }
  return out;
}

export type AccountRequest = <T>(path: string, method?: string, body?: unknown, signal?: AbortSignal) => Promise<T>;

/**
 * Leave a server: `POST /v1/servers/{id}/leave` on the signed-in session only
 * (no step-up; the owner cannot leave and gets a 409).
 */
export const leaveAccountServer = async (request: AccountRequest, serverId: string, signal?: AbortSignal): Promise<void> => {
  await request<unknown>('/v1/servers/' + encodeURIComponent(serverId) + '/leave', 'POST', {}, signal);
};

/**
 * Servers this account belongs to, from the server list, with leave where
 * supported (WEB-AUTH-02). Owners have no Leave control: the server refuses
 * with 409, so none is offered.
 */
export function Servers({scope}: {scope: AccountScope}) {
  const session = useSession();
  const api = useMemo(() => accountApi(scope), [scope.accountId, scope.familyId]);
  const [servers, setServers] = useState<AccountServer[]>();
  const [error, setError] = useState<unknown>();
  const [notice, setNotice] = useState('');
  const [leaving, setLeaving] = useState<AccountServer>();
  const [busy, setBusy] = useState(false);
  const [leaveError, setLeaveError] = useState('');
  const currentId = session.session?.viewer.serverId;
  const load = useCallback(async () => {
    setError(undefined);
    try {
      setServers(parseAccountServers(await api.request<unknown>('/v1/servers')));
    } catch (e) { setError(e ?? new Error()); }
  }, [api]);
  useEffect(() => { void load(); }, [load]);
  const leave = async (server: AccountServer) => {
    setBusy(true); setLeaveError(''); setNotice('');
    try {
      await leaveAccountServer(api.request, server.id);
      setLeaving(undefined);
      setNotice(t('web.hostedAccount.leftServer', {server: server.name ?? t('web.hostedAccount.aServer')}));
      await load();
      // The shell keeps its own copy of the list; ask it to re-read (best effort).
      await session.portico.refreshServers().catch(() => {});
    } catch (e) {
      setLeaveError((e as {status?: unknown})?.status === 409 ? t('web.hostedAccount.leaveOwner') : errorText(e, 'account', 'save'));
    } finally { setBusy(false); }
  };
  return (
    <SettingsPage>
      {error ? <ErrorNotice error={error} context="account" retry={() => void load()} /> : null}
      {notice ? <Notice tone="success" compact>{notice}</Notice> : null}
      {!servers && !error ? <Loading label={t('status.loadingThing', {thing: t('web.hostedAccount.serversThing')})} /> : null}
      {servers && !servers.length ? <StateView icon="server" title={t('web.hostedAccount.noServers')} body={t('web.hostedAccount.noServersBody')} /> : null}
      {servers && servers.length ? (
        <SettingsGroup title={t('web.servers.title')} description={t('web.hostedAccount.serversHelp')}>
          {servers.map(s => {
            const name = s.name ?? t('web.hostedAccount.aServer');
            const help = s.online ? t('web.hostedAccount.serverOnline') : s.lastSeenAt ? t('web.hostedAccount.serverLastSeen', {when: relative(Date.now() - Date.parse(s.lastSeenAt))}) : '';
            return (
              <SettingsRow
                key={s.id}
                icon="server"
                label={name}
                help={help || undefined}
                meta={s.id === currentId ? t('profile.current') : s.ownedByMe ? t('web.hostedAccount.serverOwner') : undefined}
                control={s.ownedByMe ? undefined : <Button size="sm" variant="ghost" label={t('web.hostedAccount.leaveServer')} disabled={busy} onClick={() => { setLeaveError(''); setLeaving(s); }} />}
              />
            );
          })}
        </SettingsGroup>
      ) : null}
      <ConfirmDialog
        open={!!leaving}
        onOpenChange={o => !o && !busy && setLeaving(undefined)}
        title={leaving ? t('web.hostedAccount.leaveTitle', {server: leaving.name ?? t('web.hostedAccount.aServer')}) : ''}
        body={leaving ? t('web.hostedAccount.leaveBody', {server: leaving.name ?? t('web.hostedAccount.aServer')}) : ''}
        confirmLabel={t('web.hostedAccount.leaveServer')}
        cancelLabel={t('action.cancel')}
        onConfirm={() => { const s = leaving; if (s) void leave(s); }}
      />
      {leaveError ? <ErrorNotice error={leaveError} context="account" operation="save" /> : null}
    </SettingsPage>
  );
}
