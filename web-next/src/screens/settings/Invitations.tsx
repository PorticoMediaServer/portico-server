import {useCallback, useEffect, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import type {PeopleInvitationPage, ServerInvitation} from '@core/direct-account.ts';
import {settingsSpec} from '@core/presentation/settings-spec.ts';
import {randomId} from '@core/random-id.ts';
import {SettingsAction, SettingsGroup, SettingsPageFailure, SettingsTable, type TableQuery} from '../../kit/settings';
import {Button} from '../../kit/controls';
import {Dialog} from '../../kit/overlays';
import {AuthError} from '../../kit/accounts';
import {SettingsInvitations} from './invitations';

const t = defaultI18n.t;
const spec = settingsSpec.find(section => section.scope === 'server' && section.id === 'people')!.groups![1];
const table = spec.rows[0].control;
const initial: TableQuery = {search: '', filters: {status: 'all'}, sort: {by: 'sent', direction: 'desc'}};
export function Invitations({source, libraries, isCurrent, home}: {source: SettingsInvitations; libraries: readonly {id: string; name: string}[]; isCurrent: () => boolean; home: () => void}) {
  const [query, setQuery] = useState<TableQuery>(initial), [page, setPage] = useState<PeopleInvitationPage>(), [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(), [version, setVersion] = useState(0), [busy, setBusy] = useState(false), [copied, setCopied] = useState<string>();
  const [revoke, setRevoke] = useState<Readonly<{row: ServerInvitation; operationId: string; opener: HTMLElement}>>();
  const cancel = useRef<HTMLButtonElement>(null);
  const alive = useRef(true), generation = useRef(0), requests = useRef(new Set<AbortController>()), currentPage = useRef(page);
  currentPage.current = page;
  useEffect(() => {const pending = requests.current; alive.current = true; return () => {alive.current = false; for (const request of pending) request.abort();pending.clear();};}, []);
  useEffect(() => {
    const epoch = ++generation.current, controller = new AbortController();setLoading(true);setPage(undefined);setError(undefined);setCopied(undefined);setRevoke(undefined);
    const timer = setTimeout(() => {void source.read(query, '', controller.signal).then(reply => {if (!controller.signal.aborted && isCurrent() && epoch === generation.current) setPage(reply);})
      .catch(failure => {if (!controller.signal.aborted && isCurrent()) setError(failure);}).finally(() => {if (!controller.signal.aborted && isCurrent() && epoch === generation.current) setLoading(false);});}, query.search ? 200 : 0);
    return () => {clearTimeout(timer);controller.abort();};
  }, [source, query, version, isCurrent]);
  const current = (row: ServerInvitation) => alive.current && isCurrent() && currentPage.current?.items.includes(row) === true;
  const more = useCallback(() => {
    if (loading || !page?.nextCursor || !isCurrent()) return;
    const before = page, epoch = generation.current, controller = new AbortController();requests.current.add(controller);setLoading(true);
    void source.read(query, page.nextCursor, controller.signal).then(reply => {
      if (!controller.signal.aborted && isCurrent() && epoch === generation.current) {const ids = new Set(before.items.map(row => row.id));setPage({...reply, items: [...before.items, ...reply.items.filter(row => !ids.has(row.id))]});}
    }).catch(failure => {if (!controller.signal.aborted && isCurrent()) setError(failure);}).finally(() => {requests.current.delete(controller);if (alive.current && epoch === generation.current) setLoading(false);});
  }, [source, query, page, loading, isCurrent]);
  const copy = async (row: ServerInvitation) => {
    if (busy || !current(row) || row.state !== 'pending') return;
    const controller = new AbortController();requests.current.add(controller);setBusy(true);setError(undefined);
    try {const link = await source.link(row, controller.signal);if (controller.signal.aborted || !current(row)) return;await navigator.clipboard.writeText(link);if (current(row)) setCopied(row.id);}
    catch (failure) {if (alive.current && isCurrent() && !controller.signal.aborted) setError(failure);}
    finally {requests.current.delete(controller);if (alive.current) setBusy(false);}
  };
  const revokeCurrent = async () => {
    const captured = revoke;
    if (!captured || busy || !current(captured.row) || captured.row.state !== 'pending') return;
    const controller = new AbortController();requests.current.add(controller);setBusy(true);setError(undefined);
    try {await source.client.revoke(captured.row, captured.operationId, controller.signal);if (!controller.signal.aborted && current(captured.row)) {setRevoke(undefined);setVersion(value => value + 1);}}
    catch (failure) {if (alive.current && isCurrent() && !controller.signal.aborted) setError(failure);}
    finally {requests.current.delete(controller);if (alive.current) setBusy(false);}
  };
  if (!isCurrent()) return null;
  const filtered = !!query.search || query.filters.status !== 'all';
  return <SettingsGroup title={t(spec.title!)} plain>
    {error && !page ? <SettingsPageFailure retry={() => setVersion(value => value + 1)} home={home} /> : <>
      <AuthError error={error} />
      <SettingsTable label={t(spec.title!)} height={table.height} searchLabel={t(table.search!)} query={query} onQueryChange={setQuery}
        filters={table.filters!.map(filter => ({id: filter.id, label: t(filter.label), options: filter.options.map(option => ({value: option.value, label: t(option.label)}))}))}
        rows={(page?.items ?? []).map(row => ({id: row.id, label: row.notes || row.porticoUsername || new Intl.DateTimeFormat(undefined, {dateStyle: 'medium'}).format(new Date(row.createdAt)), cells: {
          invite: {title: row.notes ?? '', meta: row.porticoUsername}, role: {title: t(row.role === 'admin' ? 'next.shell.administrator' : 'next.settings.member'), tone: row.role === 'admin' ? 'accent' as const : 'quiet' as const},
          access: row.allowedLibraries.length ? row.allowedLibraries.flatMap(id => {const library = libraries.find(value => value.id === id);return library ? [library.name] : [];}).join(', ') : t('next.settings.allLibraries'), sent: new Intl.DateTimeFormat(undefined, {dateStyle: 'medium'}).format(new Date(row.createdAt)),
          status: {title: row.state === 'revoked' ? '' : t(table.filters![0].options.find(option => option.value === (row.state === 'pending' ? 'waiting' : row.state === 'accepted' ? 'used' : 'expired'))!.label), tone: row.state === 'pending' ? 'quiet' as const : 'warn' as const},
        }}))} columns={table.columns!.map(column => ({id: column.id!, label: t(column.label), kind: column.kind as 'main', sortable: column.id === 'sent' || column.id === 'status', width: column.width}))}
        loading={loading} countLabel={page ? t(filtered ? 'next.settings.invitationCount' : 'next.settings.invitationTotal', {matched: page.matchedCount, total: page.totalCount}) : undefined}
        matchedCount={page?.matchedCount} hasMore={!!page?.nextCursor} onLoadMore={more} emptyLabel={t(table.empty!)} rowActions={[
          {id: 'copy', label: row => copied === row.id ? t('next.settings.invitationCopied') : t(table.rowActions![0].label), disabled: row => busy || page?.items.find(item => item.id === row.id)?.state !== 'pending', onAction: row => {const item = page?.items.find(item => item.id === row.id);if (item) void copy(item);}},
          {id: 'revoke', label: t(table.rowActions![1].label), disabled: row => busy || page?.items.find(item => item.id === row.id)?.state !== 'pending', onAction: (row, opener) => {const item = page?.items.find(item => item.id === row.id);if (item && current(item) && item.state === 'pending') setRevoke({row: item, operationId: randomId(), opener});}},
        ]} />
    </>}
    {revoke && <Dialog open title={t('next.settings.revokeInvitationTitle')} description={t('server.people.revokeInvitationBody')} initialFocusRef={cancel} returnFocusRef={{current: revoke.opener}} onOpenChange={open => {if (!open && !busy) setRevoke(undefined);}}
      footer={<><Button ref={cancel} disabled={busy} onClick={() => setRevoke(undefined)}>{t('action.cancel')}</Button><SettingsAction appearance="danger" size="small" loading={busy} onClick={() => {void revokeCurrent();}}>{t(table.rowActions![1].label)}</SettingsAction></>}><AuthError error={error} />{revoke.row.notes && <p>{revoke.row.notes}</p>}</Dialog>}
  </SettingsGroup>;
}
