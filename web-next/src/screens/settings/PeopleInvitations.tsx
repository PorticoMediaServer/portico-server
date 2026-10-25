import {useEffect, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import type {SettingsRow} from '@core/presentation/settings-spec.ts';
import type {ServerInvitation} from '@core/direct-account.ts';
import {SettingsTable, type SettingsTableRow} from '../../kit/settings/SettingsTable';
import {SettingsAction} from '../../kit/settings';
import type {TableQuery} from '../../kit/settings/table-model';
import type {SettingsInvitations} from './invitations';
import type {PeopleBindings} from './People';

const t = defaultI18n.t;

/** One captured invitations view. The producer filters, counts and pages; this
 * keeps the current page and the query the owner chose, never a code or a list
 * cache, and opens no second fetch. */
type Snapshot = Readonly<{phase: 'idle' | 'loading' | 'ready' | 'error'; rows: readonly ServerInvitation[];
  cursor?: string; more: boolean; matched: number; busy: string; result?: string}>;

const invited = (invitation: ServerInvitation) => invitation.porticoUsername ?? invitation.notes ?? invitation.id;

function statusOf(invitation: ServerInvitation): {title: string; tone: 'ok' | 'quiet' | 'warn' | 'bad'} {
  switch (invitation.state) {
    case 'pending': return {title: t('next.people.invitation.waiting'), tone: 'ok'};
    case 'accepted': return {title: t('next.people.invitation.used'), tone: 'quiet'};
    case 'expired': return {title: t('next.people.invitation.expired'), tone: 'warn'};
    case 'revoked': return {title: t('next.people.invitation.revoked'), tone: 'bad'};
    default: return {title: invitation.state, tone: 'quiet'};
  }
}

/** The drawn Invitations table: newest first, used and expired kept until revoked,
 * with the invitation's own link and revoke action. An empty library selection is
 * how the server spells "every library", so the Access cell says that rather than
 * counting a list it cannot see. */
export function PeopleInvitations({row, bindings}: {row: SettingsRow; bindings: PeopleBindings}) {
  const source = bindings.invitations;
  const [query, setQuery] = useState<TableQuery>({search: '', filters: {status: 'all'}, sort: {by: 'sent', direction: 'desc'}});
  const [state, setState] = useState<Snapshot>({phase: 'idle', rows: [], more: false, matched: 0, busy: ''});
  const [confirming, setConfirming] = useState<string>();
  const capture = useRef(new AbortController());
  const alive = useRef(true);

  useEffect(() => () => {alive.current = false; capture.current.abort();}, []);
  const current = () => alive.current && bindings.authority.isCurrent();
  const publish = (patch: Partial<Snapshot>) => {if (current()) setState(old => ({...old, ...patch}));};

  async function read(cursor: string, append: boolean) {
    if (!source || !current()) return;
    capture.current.abort();
    const controller = new AbortController();
    capture.current = controller;
    publish({phase: 'loading'});
    try {
      const page = await source.read(query, cursor, {signal: controller.signal, isCurrent: current});
      if (!current() || controller.signal.aborted) return;
      const rows = append ? [...state.rows, ...page.items] : page.items;
      publish({phase: 'ready', rows, cursor: page.nextCursor, more: rows.length < page.matchedCount, matched: page.matchedCount});
    } catch {
      if (current() && !controller.signal.aborted) publish({phase: 'error'});
    }
  }
  useEffect(() => {void read('', false); /* the query is the only input */}, [query.search, query.filters.status, query.sort.by, query.sort.direction]); // eslint-disable-line react-hooks/exhaustive-deps

  async function copy(invitation: ServerInvitation) {
    if (!source || !current() || state.busy) return;
    publish({busy: invitation.id, result: undefined});
    try {
      const link = await source.link(invitation, {signal: capture.current.signal, isCurrent: current});
      if (!current()) return;
      await navigator.clipboard.writeText(link);
      publish({busy: '', result: t('next.people.invitation.copied')});
    } catch {if (current()) publish({busy: '', result: t('next.people.invitation.copyFailed')});}
  }

  async function revoke(invitation: ServerInvitation) {
    if (!source || !current() || state.busy) return;
    publish({busy: invitation.id, result: undefined});
    try {
      // One operation id per attempt: fully determined by this invitation and its
      // revision, so a retry of the same revoke replays its receipt.
      const done = await source.revoke(invitation, `${invitation.id}:${invitation.revision}`, {signal: capture.current.signal, isCurrent: current});
      if (!current()) return;
      setConfirming(undefined);
      publish({busy: '', rows: state.rows.map(item => item.id === done.id ? done : item), result: t('next.people.invitation.revoked')});
    } catch {if (current()) publish({busy: '', result: t('next.people.invitation.revokeFailed')});}
  }

  if (!source) return null;
  const control = row.control;
  const rows: readonly SettingsTableRow[] = state.rows.map(invitation => ({
    id: invitation.id, label: invited(invitation), cells: {
      invite: {title: invited(invitation)},
      role: {title: invitation.role === 'admin' ? t('next.people.invitation.roleAdmin') : t('next.people.invitation.roleMember'), tone: invitation.role === 'admin' ? 'accent' : 'quiet'},
      access: invitation.allowedLibraries.length === 0 ? t('next.people.invitation.allLibraries')
        : t('next.people.invitation.someLibraries', {count: invitation.allowedLibraries.length}),
      sent: defaultI18n.date(invitation.createdAt),
      status: statusOf(invitation),
    }}));

  return <>
    <SettingsTable label={t(row.label)} rows={rows} columns={control.columns ?? []} query={query} onQueryChange={setQuery} filters={control.filters}
      searchLabel={t(control.search ?? row.label)} countLabel={t('next.people.invitation.count', {count: state.matched})}
      emptyLabel={t('next.people.invitation.empty')} loading={state.phase === 'loading'} height={control.height}
      rowActions={[
        {id: 'copy', label: t('next.people.invitation.copy'), onAction: tableRow => {const invitation = state.rows.find(item => item.id === tableRow.id); if (invitation) void copy(invitation);}},
        {id: 'revoke', label: t('next.people.invitation.revoke'), destructive: true, onAction: tableRow => {if (state.rows.find(item => item.id === tableRow.id)?.state === 'pending') setConfirming(tableRow.id);}},
      ]} />
    {confirming && <div role="alertdialog" aria-label={t('next.people.invitation.confirmRevokeTitle')}>
      <p>{t('next.people.invitation.confirmRevoke')}</p>
      <SettingsAction appearance="danger" loading={state.busy === confirming} onClick={() => {const invitation = state.rows.find(item => item.id === confirming); if (invitation) void revoke(invitation);}}>{t('next.people.invitation.revoke')}</SettingsAction>
      <SettingsAction onClick={() => setConfirming(undefined)}>{t('action.cancel')}</SettingsAction>
    </div>}
    {state.more && <SettingsAction loading={state.phase === 'loading'} onClick={() => {void read(state.cursor ?? '', true);}}>{t('next.people.invitation.more')}</SettingsAction>}
    {state.result && <p role="status">{state.result}</p>}
  </>;
}
