import {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {SettingsRow} from '@core/presentation/settings-spec.ts';
import type {ServerInvitation} from '@core/direct-account.ts';
import {SettingsTable, type SettingsTableRow} from '../../kit/settings/SettingsTable';
import {nextTableSort, type TableQuery} from '../../kit/settings/table-model';
import {SettingsAction} from '../../kit/settings';
import type {SettingsInvitations} from './invitations';
import type {SettingsPeopleBindings} from './People';

const t = defaultI18n.t;

/** One captured People invitations view: the producer filters, counts and pages;
 * this keeps only the current page and the query the owner chose. No code is
 * cached and no second fetch is opened. */
type Snapshot = Readonly<{phase: 'idle' | 'loading' | 'ready' | 'error'; rows: readonly ServerInvitation[]; cursor?: string; more: boolean;
  matched: number; total: number; busy: string; result?: string}>;

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
 * with the invitation's own link and the revoke action. */
export function PeopleInvitations({section, bindings}: {section: SettingsRow; bindings: SettingsPeopleBindings}) {
  const source = bindings.invitations;
  const [query, setQuery] = useState<TableQuery>({search: '', filters: {status: 'all'}, sort: {by: 'sent', direction: 'desc'}});
  const [state, setState] = useState<Snapshot>({phase: 'idle', rows: [], more: false, matched: 0, total: 0, busy: ''});
  const capture = useRef(new AbortController());
  const alive = useRef(true);
  const [confirming, setConfirming] = useState<string>();

  useEffect(() => () => {alive.current = false; capture.current.abort();}, []);
  const current = () => alive.current && bindings.authority.isCurrent();

  function publish(patch: Partial<Snapshot>) {if (current()) setState(old => ({...old, ...patch}));}

  async function read(cursor: string, append: boolean) {
    if (!source || !current()) return;
    capture.current.abort();
    const controller = new AbortController();
    capture.current = controller;
    publish({phase: 'loading'});
    try {
      const page = await source.read(query, cursor, {signal: controller.signal, isCurrent: current});
      if (!current() || controller.signal.aborted) return;
      publish({phase: 'ready', rows: append ? [...state.rows, ...page.items] : page.items, cursor: page.nextCursor,
        more: page.matchedCount > (append ? state.rows.length : 0) + page.items.length, matched: page.matchedCount, total: page.totalCount});
    } catch {
      if (current() && !controller.signal.aborted) publish({phase: 'error'});
    }
  }

  useEffect(() => {void read('', false); /* eslint-disable-line react-hooks/exhaustive-deps */}, [query.search, query.filters.status, query.sort.by, query.sort.direction]);

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
      // One operation id per attempt: a retry of this exact revoke replays its
      // receipt, a new attempt mints a new one.
      await source.revoke(invitation, `${invitation.id}:${invitation.revision}`, {signal: capture.current.signal, isCurrent: current});
      if (!current()) return;
      setConfirming(undefined);
      publish({busy: '', result: t('next.people.invitation.revoked')});
      await read('', false);
    } catch {if (current()) publish({busy: '', result: t('next.people.invitation.revokeFailed')});}
  }

  const control = section.control;
  const rows: readonly SettingsTableRow[] = state.rows.map(invitation => {
    const status = statusOf(invitation);
    return {id: invitation.id, label: invited(invitation), cells: {
      invite: {title: invited(invitation)},
      role: {title: invitation.role === 'admin' ? t('next.people.role.admin') : t('next.people.role.member'), tone: invitation.role === 'admin' ? 'accent' : 'quiet'},
      access: invitation.allowedLibraries.length === 0 ? t('next.people.access.all') : t('next.people.access.selected', {count: invitation.allowedLibraries.length}),
      sent: defaultI18n.date(invitation.createdAt),
      status,
    }};
  });

  if (!source) return null;
  return <>
    <SettingsTable label={t(section.label)} rows={rows} columns={control.columns ?? []} query={query} onQueryChange={setQuery}
      filters={control.filters} searchLabel={t(control.search ?? 'next.people.invitation.search')} countLabel={t('next.people.invitation.count', {count: state.matched})}
      emptyLabel={t('next.people.invitation.empty')} loading={state.phase === 'loading'} height={control.height ?? 360}
      rowAction={{label: t('next.people.invitation.copy'), onAction: row => {const invitation = state.rows.find(item => item.id === row.id); if (invitation) void copy(invitation);}}} />
    {state.rows.length > 0 && <div className={styles.actionsWrap}>
      {state.rows.filter(invitation => invitation.state === 'pending').map(invitation => <div key={invitation.id} className={styles.actionsRow}>
        {confirming === invitation.id
          ? <><span>{t('next.people.invitation.confirmRevoke')}</span>
            <SettingsAction appearance="danger" loading={state.busy === invitation.id} onClick={() => {void revoke(invitation);}}>{t('next.people.invitation.revoke')}</SettingsAction>
            <SettingsAction onClick={() => setConfirming(undefined)}>{t('action.cancel')}</SettingsAction></>
          : <><span className={styles.actionsName}>{invited(invitation)}</span>
            <SettingsAction disabled={!!state.busy} onClick={() => {void copy(invitation);}}>{t('next.people.invitation.copy')}</SettingsAction>
            <SettingsAction appearance="danger" disabled={!!state.busy} onClick={() => setConfirming(invitation.id)}>{t('next.people.invitation.revoke')}</SettingsAction></>}
      </div>)}
    </div>}
    {state.more && <div className={styles.actionsRow}><SettingsAction loading={state.phase === 'loading'} onClick={() => {void read(state.cursor ?? '', true);}}>{t('next.people.invitation.more')}</SettingsAction></div>}
    {state.result && <p role="status">{state.result}</p>}
  </>;
}
