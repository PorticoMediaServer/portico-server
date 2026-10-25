import {useEffect, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import {listPersonDevices, type PersonDevice} from '@core/person-devices.ts';
import type {SettingsPeopleApi} from '@core/settings-people.ts';
import {SettingsAction} from '../../kit/settings';

const t = defaultI18n.t;

/** One opening of this row: the exact person it shows. Every read and action
 * carries it, so a row that has been replaced, closed or unmounted cannot send
 * anything against the person it used to show. */
type Ticket = Readonly<{account: string; opening: number}>;

type Snapshot = Readonly<{phase: 'idle' | 'loading' | 'ready' | 'error'; devices: readonly PersonDevice[]; cursor?: string;
  busy: string; result?: string; error?: 'read' | 'action'}>;

/** The drawn per-person Devices row: one person's signed-in devices from the
 * owner's own device producer, filtered to their account. No cache, no second
 * fetch, and every request keeps the opening it belongs to. */
export function PersonDevices({account, api, isCurrent, onSignOut}: {account: string; api: SettingsPeopleApi; isCurrent: () => boolean;
  /** Sign one device of this person out through the existing revocation path. */
  onSignOut?: (device: PersonDevice) => Promise<void>}) {
  const [state, setState] = useState<Snapshot>({phase: 'idle', devices: [], busy: ''});
  const [confirming, setConfirming] = useState<Readonly<{deviceId: string; ticket: Ticket}> | null>(null);
  const ticket = useRef<Ticket>({account, opening: 0});
  const reads = useRef(new Set<AbortController>());
  const flight = useRef<string>('');
  const alive = useRef(true);

  const current = (opened: Ticket) => alive.current && isCurrent() && opened.account === ticket.current.account && opened.opening === ticket.current.opening;
  const publish = (opened: Ticket, patch: Partial<Snapshot>) => {if (current(opened)) setState(old => ({...old, ...patch}));};
  const retire = () => {
    for (const controller of reads.current) controller.abort();
    reads.current.clear();
    flight.current = '';
    setConfirming(null);
  };

  // A new person, or this row going away, retires every captured read at once
  // and hides the previous person's devices before anything else is drawn.
  useEffect(() => {
    alive.current = true;
    ticket.current = {account, opening: ticket.current.opening + 1};
    retire();
    setState({phase: account ? 'loading' : 'idle', devices: [], busy: ''});
    return () => {alive.current = false; retire();};
  }, [account]);

  const read = (opened: Ticket, cursor: string | undefined, append: boolean) => {
    if (!account || !current(opened) || flight.current === 'read') return;
    if (append && (state.phase !== 'ready' || !state.cursor)) return;
    const controller = new AbortController();
    reads.current.add(controller);
    flight.current = 'read';
    publish(opened, append ? {phase: 'loading'} : {phase: 'loading', devices: [], cursor: undefined});
    void listPersonDevices(api, {account, ...(cursor ? {cursor} : {}), limit: 50}, controller.signal)
      .then(page => {
        if (!current(opened) || controller.signal.aborted) return;
        setState(old => ({...old, phase: 'ready', error: undefined,
          devices: append ? [...old.devices.filter(item => !page.items.some(next => next.id === item.id)), ...page.items] : page.items,
          cursor: page.nextCursor}));
      })
      .catch(() => {if (current(opened) && !controller.signal.aborted) publish(opened, {phase: 'error', error: 'read', devices: append ? state.devices : []});})
      .finally(() => {reads.current.delete(controller); if (flight.current === 'read') flight.current = '';});
  };

  useEffect(() => {
    const opened = ticket.current;
    read(opened, undefined, false);
    return () => {for (const controller of reads.current) controller.abort();};
  }, [account]); // eslint-disable-line react-hooks/exhaustive-deps

  async function signOut(device: PersonDevice) {
    const opened = ticket.current, confirmed = confirming;
    // The confirmation is a ticket for this exact device on this opening, and the
    // device still has to be in the page that is on screen right now.
    if (!onSignOut || !current(opened) || !confirmed || confirmed.ticket !== opened || confirmed.deviceId !== device.id ||
      !state.devices.some(item => item.id === device.id) || flight.current) return;
    flight.current = 'action';
    publish(opened, {busy: device.id, result: undefined, error: undefined});
    try {
      await onSignOut(device);
      if (!current(opened)) return;
      setConfirming(null);
      setState(old => ({...old, busy: '', devices: old.devices.filter(item => item.id !== device.id), result: t('next.people.device.signedOut')}));
    } catch {
      if (current(opened)) publish(opened, {busy: '', error: 'action', result: t('next.people.device.signOutFailed')});
    } finally {if (flight.current === 'action') flight.current = '';}
  }

  if (!account) return null;
  if (state.phase === 'loading' && !state.devices.length) return <p role="status">{t('next.settings.loading')}</p>;
  if (state.error === 'read' && !state.devices.length) return <p role="alert">{t('next.people.device.failed')}</p>;
  if (!state.devices.length) return <p>{t('next.people.device.empty')}</p>;
  return <div>
    <ul aria-label={t('next.people.device.list')}>
      {state.devices.map(device => <li key={device.id}>
        <b>{device.name}</b>
        <span>{device.platform} · {device.app} {device.appVersion}</span>
        <span>{device.profileName ?? ''}</span>
        <span>{defaultI18n.relativeTime(Date.parse(device.lastSeenAt), Date.now())}</span>
        {onSignOut && (confirming?.deviceId === device.id && confirming.ticket === ticket.current
          ? <><span>{t('next.people.device.confirmSignOut')}</span>
            <SettingsAction appearance="danger" loading={state.busy === device.id} onClick={() => {void signOut(device);}}>{t('next.people.device.signOut')}</SettingsAction>
            <SettingsAction onClick={() => setConfirming(null)}>{t('action.cancel')}</SettingsAction></>
          : <SettingsAction appearance="danger" disabled={!!state.busy || !!flight.current} onClick={() => setConfirming({deviceId: device.id, ticket: ticket.current})}>{t('next.people.device.signOut')}</SettingsAction>)}
      </li>)}
    </ul>
    {state.cursor && <SettingsAction loading={state.phase === 'loading'} disabled={!!flight.current} onClick={() => {const opened = ticket.current; read(opened, state.cursor, true);}}>{t('next.people.invitation.more')}</SettingsAction>}
    {state.result && <p role="status">{state.result}</p>}
  </div>;
}

/** Sign out every device of one person, through the same revocation path. The
 * confirmation is bound to the opening and to the person it names. */
export function PersonSignOutAll({account, isCurrent, onSignOut}: {account: string; isCurrent: () => boolean; onSignOut?: (account: string) => Promise<void>}) {
  const [busy, setBusy] = useState(false), [confirming, setConfirming] = useState(false), [result, setResult] = useState<string>();
  const ticket = useRef<string>(''), flight = useRef(false), alive = useRef(true);
  useEffect(() => {alive.current = true; ticket.current = account; setConfirming(false); setResult(undefined); flight.current = false;
    return () => {alive.current = false; ticket.current = '';};}, [account]);
  const current = () => alive.current && isCurrent() && ticket.current === account;
  if (!onSignOut || !account) return null;
  return <div>
    {confirming ? <>
      <p>{t('next.people.device.confirmSignOutAll')}</p>
      <SettingsAction appearance="danger" loading={busy} onClick={() => {
        if (!current() || flight.current || !confirming) return;
        flight.current = true;
        setBusy(true); setResult(undefined);
        void onSignOut(account).then(() => {if (current()) {setConfirming(false); setResult(t('next.people.device.signedOutAll'));}})
          .catch(() => {if (current()) setResult(t('next.people.device.signOutAllFailed'));})
          .finally(() => {flight.current = false; if (current()) setBusy(false);});
      }}>{t('next.people.device.signOutAll')}</SettingsAction>
      <SettingsAction onClick={() => setConfirming(false)}>{t('action.cancel')}</SettingsAction>
    </> : <SettingsAction appearance="danger" onClick={() => {if (current()) setConfirming(true);}}>{t('next.people.device.signOutAll')}</SettingsAction>}
    {result && <p role="status">{result}</p>}
  </div>;
}
