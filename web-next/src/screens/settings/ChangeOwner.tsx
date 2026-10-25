import {useEffect, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import {Button} from '../../kit/controls/Button';
import {Dialog} from '../../kit/overlays/Dialog';
import {ownerSwitchContextAllowed} from '../../platform/secure-pending';
import type {SettingsOwnerSwitch} from './owner-switch';

const t = defaultI18n.t;
const LABEL = 'next.settingsSpec.server.signin.groups.0.rows.4.control.label';
const CONFIRM = 'next.settingsSpec.server.signin.groups.0.rows.4.control.confirm';

/** The portico-account Change owner row. The current owner confirms here, then
 * approves a new Portico Account on Hosted through the existing owner claim
 * handoff. This surface never prepares a move or a local owner transfer. */
export function ChangeOwner({source, isCurrent, onError}: {
  source: SettingsOwnerSwitch; isCurrent: () => boolean; onError: (error: unknown) => void;
}) {
  const [confirming, setConfirming] = useState(false), [secure, setSecure] = useState(false), [busy, setBusy] = useState(false);
  const opener = useRef<HTMLButtonElement>(null), cancel = useRef<HTMLButtonElement>(null), alive = useRef(true);
  useEffect(() => {alive.current = true; return () => {alive.current = false;};}, []);
  const label = t(LABEL), verb = label.replace(/…$/, '');
  const open = () => {
    if (busy || !alive.current || !isCurrent()) return;
    // The claim producer requires a secure context and encrypted pending storage.
    if (!ownerSwitchContextAllowed()) {setSecure(true);return;}
    setConfirming(true);
  };
  const run = async () => {
    if (busy || !alive.current || !isCurrent()) return;
    setBusy(true);
    try {
      const url = await source.prepareChangeOwner();
      if (alive.current && isCurrent()) window.location.assign(url);
    } catch (error) {if (alive.current && isCurrent()) onError(error);}
    finally {if (alive.current && isCurrent()) setBusy(false);}
  };
  return <>
    <Button ref={opener} variant="glass" size="small" loading={busy} disabled={!isCurrent()} onClick={open}>{label}</Button>
    <Dialog children={null} open={secure} onOpenChange={value => {if (!value) setSecure(false);}} title={t('accounts.ownerSwitch.secureTitle')}
      description={t('accounts.ownerSwitch.secureBody', {localUrl: 'http://localhost:32500'})}
      footer={<Button variant="glass" onClick={() => setSecure(false)}>{t('action.cancel')}</Button>}/>
    <Dialog open={confirming} onOpenChange={value => {if (!busy) setConfirming(value);}} title={`${verb}?`} description={t(CONFIRM)}
      initialFocusRef={cancel} returnFocusRef={opener} footer={<>
        <Button ref={cancel} size="small" disabled={busy} onClick={() => setConfirming(false)}>{t('action.cancel')}</Button>
        <Button variant="primary" size="small" loading={busy} onClick={() => {void run();}}>{verb}</Button></>}>
      <span />
    </Dialog>
  </>;
}
