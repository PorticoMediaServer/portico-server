import {useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState} from 'react';
import {defaultI18n, type MessageId} from '@i18n';
import type {ServerInvitation} from '@core/direct-account.ts';
import {settingsSpec} from '@core/presentation/settings-spec.ts';
import {randomId} from '@core/random-id.ts';
import {Button} from '../../kit/controls';
import {Dialog} from '../../kit/overlays/Dialog';
import {SettingsGroup, SettingsPageFailure, SettingsTable, type SettingsTableRow, type TableQuery} from '../../kit/settings';
import kitStyles from '../../kit/settings/settings.module.css';
import type {SettingsInvitations} from './invitations';
import {
  InvitationAdmissions, invitationLease, invitationRequest, sameOpening,
  type InvitationLease, type InvitationOpening, type InvitationPage,
} from './people-invitation-admission';

const t = defaultI18n.t;
const section = settingsSpec.find(candidate => candidate.scope === 'server' && candidate.id === 'people')!;
const group = section.groups![1], model = group.rows[0].control;
const DAY = 86_400_000;
const date = new Intl.DateTimeFormat(undefined, {dateStyle: 'medium'});
const initialQuery: TableQuery = {search: '', filters: {status: 'all'}, sort: {by: model.sort?.by ?? 'sent', direction: (model.sort?.dir as 'asc' | 'desc' | undefined) ?? 'desc'}};

/** Status and every command follow the producer's state. The clock is only a
 * display hint under "Waiting": the table projection already normalizes an
 * expired pending row, so the client never reclassifies status, filter or count. */
function pendingHint(invitation: ServerInvitation, now: number): Readonly<{message: MessageId; count: number}> | undefined {
  const expires = Date.parse(invitation.expiresAt);
  if (!Number.isFinite(expires)) return undefined;
  const days = Math.ceil((expires - now) / DAY);
  return days > 0 && days <= 2 ? Object.freeze({message: 'next.settings.invitations.expiresIn', count: days}) : undefined;
}

function invitationRow(invitation: ServerInvitation, libraries: readonly {id: string; name: string}[], now: number): SettingsTableRow {
  const notes = invitation.notes?.trim() ?? '', title = notes || t('next.settings.invitations.linkOnly');
  const role = invitation.role === 'admin', hint = invitation.state === 'pending' ? pendingHint(invitation, now) : undefined;
  const names = invitation.allowedLibraries.map(id => libraries.find(library => library.id === id)?.name).filter((name): name is string => !!name);
  const access = !invitation.allowedLibraries.length ? t('next.settings.allLibraries')
    : names.length === invitation.allowedLibraries.length && names.length <= 2 ? defaultI18n.list(names)
    : t('next.settings.invitations.libraries', {count: invitation.allowedLibraries.length});
  const status = invitation.state === 'pending'
      ? {title: t('next.settings.personStatus.waiting'), tone: hint ? 'warn' as const : 'quiet' as const, ...(hint ? {meta: t(hint.message, {count: hint.count})} : {})}
    : invitation.state === 'accepted' ? {title: invitation.porticoUsername ? t('next.settings.invitations.usedBy', {name: invitation.porticoUsername}) : t('next.settings.invitations.used'), tone: 'ok' as const}
    : invitation.state === 'expired' ? {title: t('next.settings.invitations.expired'), tone: 'bad' as const}
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
 * the link/revoke commands. Every control and read speaks for the exact request,
 * page, cursor and opening it was painted or claimed with; `InvitationAdmissions`
 * owns the token layer, so a newer commit, cancellation or activation retires
 * the old one before it can act. */
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
  const [component] = useState<object>(() => ({}));
  const authorityRef = useRef(authority);
  useLayoutEffect(() => {authorityRef.current = authority;});
  const guardRef = useRef<InvitationAdmissions | undefined>(undefined);
  if (!guardRef.current) guardRef.current = new InvitationAdmissions(() => authorityRef.current.isCurrent());
  const guard = guardRef.current;
  const [query, setQuery] = useState<TableQuery>(initialQuery);
  const [refresh, setRefresh] = useState(0), [loading, setLoading] = useState(false), [error, setError] = useState<unknown>();
  const [note, setNote] = useState<MessageId>(), [confirming, setConfirming] = useState<InvitationOpening | null>(null);
  const [revoking, setRevoking] = useState<InvitationOpening | null>(null);
  const noteTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined), confirmOpener = useRef<HTMLElement | null>(null);
  useEffect(() => () => {clearTimeout(noteTimer.current);}, []);
  // The request admission changes at commit for a new query, refresh, version or
  // service/authority identity, so the old page and its controls retire at that
  // commit, before the passive read below asks for the new one.
  const request = useMemo(() => invitationRequest(component,
    Object.freeze({query, refresh, version, invitations, authorityCurrent: authority.isCurrent})),
    [component, query, refresh, version, invitations, authority.isCurrent]);
  const [loaded, setLoaded] = useState<Readonly<{request: typeof request; page: InvitationPage}>>();
  // A page belongs only to the request that fetched it; a newer request hides it
  // until its own read answers, so no row from the old query can be commanded.
  const page = loaded && loaded.request === request ? loaded.page : undefined;
  const lease = useMemo(() => invitationLease(request, page), [request, page]);
  useLayoutEffect(() => {
    const admission = guard.activate(component);
    return () => guard.retire(admission);
  }, [guard, component]);
  useLayoutEffect(() => {
    guard.publishRequest(request);
    return () => guard.retireRequest(request);
  }, [guard, request]);
  useLayoutEffect(() => {
    guard.publishLease(lease);
    return () => guard.retireLease(lease);
  }, [guard, lease]);
  const failure = useCallback((value: unknown) => {
    if (!guard.live()) return;
    const status = value && typeof value === 'object' && 'status' in value ? value.status : undefined;
    if (status === 401 || status === 403) {authority.onAuthorityChanged();return;}
    setError(value);
  }, [authority, guard]);
  useEffect(() => {
    const read = guard.requestRead(request, '');
    if (!read) return;
    setLoading(true); setError(undefined);
    const timer = setTimeout(() => {
      if (!guard.readCurrent(read)) return;
      void invitations.read(query, '', {signal: read.signal, isCurrent: () => guard.readCurrent(read)}).then(value => {
        if (guard.readCurrent(read)) setLoaded({request: read.request, page: value});
      }).catch(value => {if (guard.readCurrent(read)) failure(value);})
        .finally(() => {if (guard.readCurrent(read)) {setLoading(false); guard.releaseRead(read);}});
    }, query.search ? 200 : 0);
    return () => {clearTimeout(timer); guard.releaseRead(read);};
  }, [guard, invitations, request, query, failure]);
  const more = () => {
    if (!page?.nextCursor) return;
    const before = page, read = guard.requestRead(request, before.nextCursor);
    if (!read) return;
    setLoading(true);
    void invitations.read(query, before.nextCursor, {signal: read.signal, isCurrent: () => guard.readCurrent(read)}).then(next => {
      if (!guard.readCurrent(read)) return;
      const ids = new Set(before.items.map(invitation => invitation.id));
      setLoaded({request: read.request, page: {...next, items: [...before.items, ...next.items.filter(invitation => !ids.has(invitation.id))]}});
    }).catch(value => {if (guard.readCurrent(read)) failure(value);})
      .finally(() => {if (guard.readCurrent(read)) {setLoading(false); guard.releaseRead(read);}});
  };
  const show = (message: MessageId) => {
    clearTimeout(noteTimer.current); setNote(message);
    noteTimer.current = setTimeout(() => {if (guard.activated()) setNote(undefined);}, 4000);
  };
  /** A definite refusal means the producer's row moved. Re-read rather than
   * repaint the old state; an uncertain failure keeps the opening for its retry. */
  const refreshOnRefusal = (value: unknown) => {
    const code = value && typeof value === 'object' && 'code' in value ? value.code : undefined;
    const status = value && typeof value === 'object' && 'status' in value ? value.status : undefined;
    if (code === 'invitation_expired' || code === 'invitation_changed' || status === 409) setRefresh(current => current + 1);
  };
  const copy = async (target: InvitationLease, invitation: ServerInvitation) => {
    const claim = guard.copy(target, invitation);
    if (!claim) return;
    const controller = new AbortController(), exact = () => guard.copyCurrent(claim);
    try {
      const link = await invitations.link(invitation, {signal: controller.signal, isCurrent: exact});
      if (!exact()) return;
      await navigator.clipboard.writeText(link);
      if (exact()) show('next.settings.invitations.linkCopied');
    } catch (value) {if (exact()) {refreshOnRefusal(value); reportError(value);}}
    finally {guard.releaseCopy(claim);}
  };
  const openRevoke = (target: InvitationLease, invitation: ServerInvitation, opener: HTMLElement) => {
    const opening = guard.open(target, invitation, randomId());
    if (!opening) return;
    confirmOpener.current = opener;
    setConfirming(opening);
  };
  /** Cancel and dismissal retire the opening synchronously, before React commits. */
  const closeRevoke = (opening: InvitationOpening | null) => {
    if (!opening) return;
    guard.close(opening);
    setConfirming(current => sameOpening(current, opening) ? null : current);
  };
  const confirmRevoke = async (opening: InvitationOpening) => {
    const claimed = guard.claim(opening);
    if (!claimed) return;
    setRevoking(claimed);
    const controller = new AbortController(), exact = () => guard.commandCurrent(opening);
    try {
      await invitations.revoke(opening.invitation, opening.operationId, {signal: controller.signal, isCurrent: exact});
      if (!exact()) return;
      guard.close(opening); guard.release(opening);
      setConfirming(current => sameOpening(current, opening) ? null : current);
      setRevoking(current => sameOpening(current, opening) ? null : current);
      show('next.settings.invitations.revoked'); setRefresh(value => value + 1);
    } catch (value) {if (exact()) {refreshOnRefusal(value); reportError(value);}}
    finally {
      // Only this exact claim may release, and only its own busy paint.
      if (guard.settled(opening)) {
        guard.release(opening);
        if (guard.activated()) setRevoking(current => sameOpening(current, opening) ? null : current);
      }
    }
  };
  const now = Date.now();
  const rows = (page?.items ?? []).map(invitation => invitationRow(invitation, libraries, now));
  const filtered = !!query.search.trim() || Object.values(query.filters).some(value => value !== 'all');
  const actionCopy = model.rowActions![0], revokeCopy = model.rowActions![1];
  const confirmBusy = sameOpening(revoking, confirming);
  // A reload, query change or activation retires the confirmation's lease.
  const confirmStale = !!confirming && (confirming.lease !== lease || !guard.isOpen(confirming) || !guard.resident(lease, confirming.invitation));
  useEffect(() => {
    if (!confirming || !confirmStale) return;
    if (guard.settled(confirming)) return;  // its own command still runs; keep that dialog
    guard.close(confirming);
    setConfirming(current => sameOpening(current, confirming) ? null : current);
  }, [confirming, confirmStale, revoking, guard]);
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
        const canCopy = invitation.state === 'pending';
        const canRevoke = invitation.state === 'pending' || invitation.state === 'expired';
        return <span style={{display: 'inline-flex', gap: 4}}>
          {canCopy && <Button size="xs" variant="text" onClick={() => {void copy(lease, invitation);}}>{t(actionCopy.label)}</Button>}
          {canRevoke && <Button size="xs" variant="text" onClick={event => openRevoke(lease, invitation, event.currentTarget)}>{t(revokeCopy.label)}</Button>}
        </span>;
      }} />}
    </SettingsGroup>
    {confirming && <Dialog open title={t(revokeCopy.label)} onOpenChange={open => {
      if (open) return;
      // A confirmation whose own command is in flight cannot be dismissed.
      if (guard.settled(confirming)) return;
      closeRevoke(confirming);
    }}
      returnFocusRef={confirmOpener} footer={<><Button disabled={confirmBusy} onClick={() => {
        if (guard.settled(confirming)) return;
        closeRevoke(confirming);
      }}>{t('next.settings.cancel')}</Button>
        <Button variant="primary" className={kitStyles.danger} loading={confirmBusy} disabled={confirmStale}
          onClick={() => {void confirmRevoke(confirming);}}>{t(revokeCopy.label)}</Button></>}>
      <p>{t(revokeCopy.confirm!)}</p>
    </Dialog>}
  </>;
}
