import {useCallback, useEffect, useRef, useState} from 'react';
import {defaultI18n, type MessageId} from '@i18n';
import type {ServerInvitation} from '@core/direct-account.ts';
import {settingsSpec} from '@core/presentation/settings-spec.ts';
import {randomId} from '@core/random-id.ts';
import {Button} from '../../kit/controls';
import {Dialog} from '../../kit/overlays/Dialog';
import {SettingsGroup, SettingsPageFailure, SettingsTable, type SettingsTableRow, type TableQuery} from '../../kit/settings';
import kitStyles from '../../kit/settings/settings.module.css';
import type {SettingsInvitations} from './invitations';

const t = defaultI18n.t;
const section = settingsSpec.find(candidate => candidate.scope === 'server' && candidate.id === 'people')!;
const group = section.groups![1], model = group.rows[0].control;
const DAY = 86_400_000;
const date = new Intl.DateTimeFormat(undefined, {dateStyle: 'medium'});
const initialQuery: TableQuery = {search: '', filters: {status: 'all'}, sort: {by: model.sort?.by ?? 'sent', direction: (model.sort?.dir as 'asc' | 'desc' | undefined) ?? 'desc'}};

/** The producer already normalizes an expired pending row; a clock that moved
 * between read and paint is still shown as expired, never as waiting. */
function stateOf(invitation: ServerInvitation, now: number): ServerInvitation['state'] {
  if (invitation.state === 'pending' && Date.parse(invitation.expiresAt) <= now) return 'expired';
  return invitation.state;
}

function invitationRow(invitation: ServerInvitation, libraries: readonly {id: string; name: string}[], now: number): SettingsTableRow {
  const state = stateOf(invitation, now), notes = invitation.notes?.trim() ?? '';
  const title = notes || t('next.settings.invitations.linkOnly');
  const role = invitation.role === 'admin';
  const names = invitation.allowedLibraries.map(id => libraries.find(library => library.id === id)?.name).filter((name): name is string => !!name);
  const access = !invitation.allowedLibraries.length ? t('next.settings.allLibraries')
    : names.length === invitation.allowedLibraries.length && names.length <= 2 ? defaultI18n.list(names)
    : t('next.settings.invitations.libraries', {count: invitation.allowedLibraries.length});
  const days = Math.ceil((Date.parse(invitation.expiresAt) - now) / DAY);
  const status = state === 'pending' ? days <= 2
      ? {title: t('next.settings.invitations.expiresIn', {count: Math.max(1, days)}), tone: 'warn' as const}
      : {title: t('next.settings.personStatus.waiting'), tone: 'quiet' as const}
    : state === 'accepted' ? {title: invitation.porticoUsername ? t('next.settings.invitations.usedBy', {name: invitation.porticoUsername}) : t('next.settings.invitations.used'), tone: 'ok' as const}
    : state === 'expired' ? {title: t('next.settings.invitations.expired'), tone: 'bad' as const}
    : {title: t('next.settings.invitations.revokedState'), tone: 'quiet' as const};
  return {id: invitation.id, label: title, cells: {
    invite: {title, ...(invitation.porticoUsername ? {meta: invitation.porticoUsername} : {}), avatar: Array.from(title)[0]?.toLocaleUpperCase() ?? 'L', avatarKey: title},
    role: {title: t(role ? 'next.shell.administrator' : 'next.settings.member'), tone: role ? 'accent' : 'quiet'},
    access: {title: access},
    sent: {title: date.format(new Date(invitation.createdAt))},
    status,
  }};
}

/** The drawn Invitations group: the honest server table, its counted pages and
 * the link/revoke commands. Nothing is cached: every read reauthorizes and the
 * short link is issued only by Copy link, for the row it was drawn for. */
export function PeopleInvitations({invitations, libraries, authority, version, openInvite, openHome, reportError}: Readonly<{
  invitations: SettingsInvitations;
  libraries: readonly {id: string; name: string}[];
  authority: Readonly<{isCurrent: () => boolean; onAuthorityChanged: () => void}>;
  /** Bumped by the create flow so a new invitation appears without a reload. */
  version: number;
  openInvite: ((opener: HTMLElement) => void) | undefined;
  openHome: () => void;
  reportError: (error: unknown) => void;
}>) {
  const [query, setQuery] = useState<TableQuery>(initialQuery);
  const [page, setPage] = useState<Readonly<{items: readonly ServerInvitation[]; nextCursor?: string; matchedCount: number; totalCount: number}>>();
  const [refresh, setRefresh] = useState(0), [loading, setLoading] = useState(false), [error, setError] = useState<unknown>();
  const [note, setNote] = useState<MessageId>(), [confirming, setConfirming] = useState<ServerInvitation>();
  const [revoking, setRevoking] = useState(false);
  const mounted = useRef(true), generation = useRef(0), noteTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const confirmOpener = useRef<HTMLElement | null>(null), revokeOperation = useRef('');
  useEffect(() => {mounted.current = true;return () => {mounted.current = false;clearTimeout(noteTimer.current);};}, []);
  const current = useCallback(() => mounted.current && authority.isCurrent(), [authority]);
  const failure = useCallback((value: unknown) => {
    if (!current()) return;
    const status = value && typeof value === 'object' && 'status' in value ? value.status : undefined;
    if (status === 401 || status === 403) {authority.onAuthorityChanged();return;}
    setError(value);
  }, [authority, current]);
  useEffect(() => {
    if (!current()) return;
    const epoch = ++generation.current, controller = new AbortController();
    setPage(undefined); setLoading(true); setError(undefined);
    const timer = setTimeout(() => {
      void invitations.read(query, '', {signal: controller.signal, isCurrent: current}).then(value => {
        if (!controller.signal.aborted && epoch === generation.current && current()) setPage(value);
      }).catch(value => {if (!controller.signal.aborted && epoch === generation.current) failure(value);})
        .finally(() => {if (epoch === generation.current) setLoading(false);});
    }, query.search ? 200 : 0);
    return () => {clearTimeout(timer); controller.abort();};
  }, [invitations, query, refresh, version, current, failure]);
  const more = () => {
    if (!page?.nextCursor || loading || !current()) return;
    const epoch = generation.current, before = page, controller = new AbortController();
    setLoading(true);
    void invitations.read(query, page.nextCursor, {signal: controller.signal, isCurrent: current}).then(next => {
      if (!controller.signal.aborted && epoch === generation.current && current()) {
        const ids = new Set(before.items.map(invitation => invitation.id));
        setPage({...next, items: [...before.items, ...next.items.filter(invitation => !ids.has(invitation.id))]});
      }
    }).catch(value => {if (!controller.signal.aborted && epoch === generation.current) failure(value);})
      .finally(() => {if (epoch === generation.current) setLoading(false);});
  };
  const show = (message: MessageId) => {
    clearTimeout(noteTimer.current); setNote(message);
    noteTimer.current = setTimeout(() => {if (mounted.current) setNote(undefined);}, 4000);
  };
  const copy = async (invitation: ServerInvitation) => {
    if (!current()) return;
    const controller = new AbortController();
    try {
      const link = await invitations.link(invitation, {signal: controller.signal, isCurrent: current});
      if (!current()) return;
      await navigator.clipboard.writeText(link);
      if (current()) show('next.settings.invitations.linkCopied');
    } catch (value) {if (current()) reportError(value);}
  };
  const revoke = async () => {
    if (!confirming || revoking || !current()) return;
    setRevoking(true);
    const controller = new AbortController();
    try {
      // One operation for this confirmation: a retry repeats that exact request.
      if (!revokeOperation.current) revokeOperation.current = randomId();
      await invitations.revoke(confirming, revokeOperation.current, {signal: controller.signal, isCurrent: current});
      if (!current()) return;
      revokeOperation.current = ''; setConfirming(undefined); show('next.settings.invitations.revoked'); setRefresh(value => value + 1);
    } catch (value) {if (current()) reportError(value);}
    finally {if (mounted.current) setRevoking(false);}
  };
  const now = Date.now();
  const rows = (page?.items ?? []).map(invitation => invitationRow(invitation, libraries, now));
  const filtered = !!query.search.trim() || Object.values(query.filters).some(value => value !== 'all');
  const actionCopy = model.rowActions![0], revokeCopy = model.rowActions![1];
  return <>
    <SettingsGroup title={t(group.title!)} plain actions={openInvite && <Button size="small" variant="primary" icon="i-plus" iconSize={20}
      onClick={event => {if (authority.isCurrent()) openInvite(event.currentTarget);}}>{t(model.addLabel!)}</Button>}>
    {error && !page ? <SettingsPageFailure retry={() => setRefresh(value => value + 1)} home={openHome} /> : <SettingsTable height={model.height}
      label={t(group.title!)} rows={rows}
      columns={model.columns!.map(column => ({id: column.id!, label: t(column.label), kind: column.kind as 'main' | 'text' | 'muted' | 'badge' | 'num' | undefined, sortable: column.sort && (column.id === 'sent' || column.id === 'status'), width: column.width}))}
      query={query} onQueryChange={setQuery}
      filters={model.filters!.map(filter => ({id: filter.id, label: t(filter.label), options: filter.options.map(option => ({value: option.value, label: t(option.label)}))}))}
      searchLabel={t(model.search!)} emptyLabel={t(model.empty!)} loading={loading}
      countLabel={page ? t(filtered ? 'next.settings.invitationsCount' : 'next.settings.invitationsTotal', {matched: page.matchedCount, total: page.totalCount}) : undefined}
      matchedCount={page?.matchedCount} hasMore={!!page?.nextCursor} onLoadMore={more}
      actions={error && page ? <span className={kitStyles.status} role="status" data-tone="bad">{t('next.states.pageFailedBody')}</span>
        : note && <span className={kitStyles.status} role="status" data-tone="ok">{t(note)}</span>}
      rowActions={row => {
        const invitation = page?.items.find(item => item.id === row.id);
        if (!invitation) return null;
        const state = stateOf(invitation, now);
        return <span style={{display: 'inline-flex', gap: 4}}>
          {state === 'pending' && <Button size="xs" variant="text" onClick={() => {void copy(invitation);}}>{t(actionCopy.label)}</Button>}
          {(state === 'pending' || state === 'expired') && <Button size="xs" variant="text" onClick={event => {confirmOpener.current = event.currentTarget; revokeOperation.current = ''; setConfirming(invitation);}}>{t(revokeCopy.label)}</Button>}
        </span>;
      }} />}
    </SettingsGroup>
    {confirming && <Dialog open title={t(revokeCopy.label)} onOpenChange={open => {if (!open && !revoking) {revokeOperation.current = ''; setConfirming(undefined);}}}
      returnFocusRef={confirmOpener} footer={<><Button disabled={revoking} onClick={() => setConfirming(undefined)}>{t('next.settings.cancel')}</Button>
        <Button variant="primary" className={kitStyles.danger} loading={revoking} onClick={() => {void revoke();}}>{t(revokeCopy.label)}</Button></>}>
      <p>{t(revokeCopy.confirm!)}</p>
    </Dialog>}
  </>;
}
