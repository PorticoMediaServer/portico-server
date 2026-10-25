import {useEffect, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import {listPersonDevices, type PersonDevice} from '@core/person-devices.ts';
import type {SettingsPeopleApi} from '@core/settings-people.ts';
import {SettingsAction} from '../../kit/settings';

const t = defaultI18n.t;

type Snapshot = Readonly<{phase: 'idle' | 'loading' | 'ready' | 'error'; devices: readonly PersonDevice[]; cursor?: string;
  busy: string; result?: string}>;

/** The drawn per-person Devices row: this person's signed-in devices, from the
 * owner's own device producer filtered to their account. Nothing is cached
 * between reads and no account-wide list is relabelled as theirs. */
export function PersonDevices({account, api, isCurrent, onSignOut}: {account: string; api: SettingsPeopleApi; isCurrent: () => boolean;
  /** Sign out one device of this person, through the existing revocation path. */
  onSignOut?: (device: PersonDevice) => Promise<void>}) {
  const [state, setState] = useState<Snapshot>({phase: 'idle', devices: [], busy: ''});
  const [confirming, setConfirming] = useState<string>();
  const capture = useRef(new AbortController());
  const alive = useRef(true);
  const current = () => alive.current && isCurrent();
  const publish = (patch: Partial<Snapshot>) => {if (current()) setState(old => ({...old, ...patch}));};

  useEffect(() => () => {alive.current = false; capture.current.abort();}, []);
  useEffect(() => {
    if (!account || !current()) return;
    capture.current.abort();
    const controller = new AbortController();
    capture.current = controller;
    publish({phase: 'loading'});
    void listPersonDevices(api, {account, limit: 50}, controller.signal)
      .then(page => {if (current() && !controller.signal.aborted) publish({phase: 'ready', devices: page.items, cursor: page.nextCursor});})
      .catch(() => {if (current() && !controller.signal.aborted) publish({phase: 'error'});});
    return () => controller.abort();
  }, [account]); // eslint-disable-line react-hooks/exhaustive-deps

  async function signOut(device: PersonDevice) {
    if (!onSignOut || !current() || state.busy) return;
    publish({busy: device.id, result: undefined});
    try {
      await onSignOut(device);
      if (!current()) return;
      setConfirming(undefined);
      publish({busy: '', devices: state.devices.filter(item => item.id !== device.id), result: t('next.people.device.signedOut')});
    } catch {if (current()) publish({busy: '', result: t('next.people.device.signOutFailed')});}
  }

  if (!account) return null;
  if (state.phase === 'loading' && !state.devices.length) return <p role="status">{t('next.settings.loading')}</p>;
  if (state.phase === 'error' && !state.devices.length) return <p role="alert">{t('next.people.device.failed')}</p>;
  if (!state.devices.length) return <p>{t('next.people.device.empty')}</p>;
  return <div>
    <ul aria-label={t('next.people.device.list')}>
      {state.devices.map(device => <li key={device.id}>
        <b>{device.name}</b>
        <span>{device.platform} · {device.app} {device.appVersion}</span>
        <span>{device.profileName ?? ''}</span>
        <span>{defaultI18n.relativeTime(Date.parse(device.lastSeenAt), Date.now())}</span>
        {onSignOut && (confirming === device.id
          ? <><span>{t('next.people.device.confirmSignOut')}</span>
            <SettingsAction appearance="danger" loading={state.busy === device.id} onClick={() => {void signOut(device);}}>{t('next.people.device.signOut')}</SettingsAction>
            <SettingsAction onClick={() => setConfirming(undefined)}>{t('action.cancel')}</SettingsAction></>
          : <SettingsAction appearance="danger" disabled={!!state.busy} onClick={() => setConfirming(device.id)}>{t('next.people.device.signOut')}</SettingsAction>)}
      </li>)}
    </ul>
    {state.cursor && <SettingsAction loading={state.phase === 'loading'} onClick={() => {
      const controller = new AbortController();
      capture.current = controller;
      publish({phase: 'loading'});
      void listPersonDevices(api, {account, cursor: state.cursor, limit: 50}, controller.signal)
        .then(page => {if (current() && !controller.signal.aborted) publish({phase: 'ready', devices: [...state.devices, ...page.items], cursor: page.nextCursor});})
        .catch(() => {if (current() && !controller.signal.aborted) publish({phase: 'error'});});
    }}>{t('next.people.invitation.more')}</SettingsAction>}
    {state.result && <p role="status">{state.result}</p>}
  </div>;
}
