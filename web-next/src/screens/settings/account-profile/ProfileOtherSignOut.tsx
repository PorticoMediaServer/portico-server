import {useEffect, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import {Button} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays/Dialog';
import type {ProfileOtherDeviceSignOut} from './sign-out-others';
const t = defaultI18n.t;

export function ProfileOtherSignOut({command, isCurrent, onFailure, disabled}: {
  command: ProfileOtherDeviceSignOut; isCurrent: () => boolean; onFailure: () => void; disabled?: boolean;
}) {
  const [open, setOpen] = useState(false), [busy, setBusy] = useState(false);
  const pending = useRef(false), live = useRef(true), trigger = useRef<HTMLButtonElement>(null);
  useEffect(() => {live.current = true;return () => {live.current = false;};}, []);
  const run = async () => {
    if (pending.current || disabled || !isCurrent()) return;
    pending.current = true;setBusy(true);
    try {await command.signOut();if (live.current && isCurrent()) setOpen(false);}
    catch {if (live.current && isCurrent()) {setOpen(false);onFailure();}}
    finally {pending.current = false;if (live.current && isCurrent()) setBusy(false);}
  };
  if (!isCurrent()) return null;
  return <>
    <Button ref={trigger} size="small" variant="glass" disabled={disabled || busy} onClick={() => {if (isCurrent()) setOpen(true);}}>{t('next.settingsSpec.account.profile.groups.8.rows.0.control.label')}</Button>
    <Dialog open={open} size="sm" returnFocusRef={trigger} title={t('next.settingsSpec.account.profile.groups.8.rows.0.label')}
      description={t('next.settingsSpec.account.profile.groups.8.rows.0.control.confirm')}
      dismissOnOutsidePress={!busy} dismissOnBack={!busy} showCloseButton={!busy} onOpenChange={value => {if (!pending.current && isCurrent()) setOpen(value);}}
      footer={<><Button size="small" disabled={busy} onClick={() => setOpen(false)}>{t('action.cancel')}</Button>
        <Button size="small" variant="primary" loading={busy} onClick={() => {void run();}}>{t('next.settingsSpec.account.profile.groups.8.rows.0.control.label')}</Button></>} />
  </>;
}
