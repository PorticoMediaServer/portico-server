import {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {ManagedProfile} from '@core/profile-management.ts';
import {AuthCodeBoxes} from '../../../kit/accounts/AuthStage';
import {Button, Switch} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays/Dialog';
import type {ProfileSettingsSource} from './source';
import styles from './profile.module.css';
const t = defaultI18n.t;

/** The managing profile's checked set/change/remove flow. Forgot PIN retains
 * the separate reauthentication journey; it is never implemented by this control. */
export function ProfilePINControl({source, isCurrent, onFailure, action = 'toggle'}: {
  source: ProfileSettingsSource; isCurrent: () => boolean; onFailure: () => void; action?: 'toggle' | 'change';
}) {
  const state = useSyncExternalStore(source.subscribe, source.getSnapshot, source.getSnapshot);
  const [dialog, setDialog] = useState<{kind: 'set' | 'remove' | 'done'; target: ManagedProfile}>();
  const [pin, setPIN] = useState(''), [repeat, setRepeat] = useState(''), [mismatch, setMismatch] = useState(false);
  const request = useRef(false), live = useRef(true), trigger = useRef<HTMLElement | null>(null);
  useEffect(() => {live.current = true;return () => {live.current = false;};}, []);
  const selected = source.selected();
  const permitted = isCurrent() && state.document?.canManage === true && state.document.capabilities.manageAccount;
  const clear = () => {setPIN('');setRepeat('');setMismatch(false);};
  const close = () => {if (!request.current) {clear();setDialog(undefined);}};
  const open = (kind: 'set' | 'remove') => {
    if (!permitted || !selected || state.busy || request.current) return;
    trigger.current = document.activeElement as HTMLElement;clear();setDialog({kind, target: selected});
  };
  const save = async () => {
    if (!dialog || dialog.kind === 'done' || !permitted || request.current || state.busy) return;
    if (dialog.kind === 'set' && (!/^\d{4}$/.test(pin) || !/^\d{4}$/.test(repeat))) return;
    if (dialog.kind === 'set' && pin !== repeat) {setMismatch(true);return;}
    const value = dialog.kind === 'remove' ? '' : pin, target = dialog.target, removed = dialog.kind === 'remove';
    request.current = true;clear();
    try {
      await source.setPIN(target, value);
      if (live.current && isCurrent()) setDialog(removed ? undefined : {kind: 'done', target});
    } catch {
      if (live.current && isCurrent()) {setDialog(undefined);onFailure();}
    } finally {request.current = false;}
  };
  if (!permitted || !selected) return null;
  return <>
    {action === 'toggle' ? <Switch aria-label={t('next.settingsSpec.account.profile.groups.2.rows.0.label')} checked={selected.pinRequired} disabled={state.busy} onCheckedChange={value => open(value ? 'set' : 'remove')} />
      : <Button size="small" variant="glass" disabled={state.busy || !selected.pinRequired} onClick={() => open('set')}>{t('next.settingsSpec.account.profile.groups.2.rows.0.children.0.control.label')}</Button>}
    {dialog && <Dialog open width={420} returnFocusRef={trigger} onOpenChange={value => {if (!value) close();}}
      dismissOnOutsidePress={!state.busy} dismissOnBack={!state.busy} showCloseButton={!state.busy}
      title={t(dialog.kind === 'done' ? 'profile.pinChanged' : dialog.kind === 'remove' ? 'profile.removePinConfirm.title' : 'profile.pinTitle', {name: dialog.target.name})}
      description={t(dialog.kind === 'done' ? 'profile.pinSavedBody' : dialog.kind === 'remove' ? 'profile.removePinConfirm.body' : 'profile.pinChoose', {name: dialog.target.name})}
      footer={dialog.kind === 'done' ? <Button variant="primary" size="small" onClick={close}>{t('action.done')}</Button> : <>
        <Button size="small" disabled={state.busy} onClick={close}>{t('action.cancel')}</Button>
        <Button variant={dialog.kind === 'remove' ? 'glass' : 'primary'} className={dialog.kind === 'remove' ? styles.danger : undefined} size="small" loading={state.busy} disabled={state.busy || dialog.kind === 'set' && (pin.length !== 4 || repeat.length !== 4)} onClick={() => {void save();}}>{t(dialog.kind === 'remove' ? 'profile.removePin' : 'action.save')}</Button>
      </>}>
      {dialog.kind === 'set' && <form className={styles.fields} onSubmit={event => {event.preventDefault();void save();}}>
        <label>{t('web.profile.newPin')}<AuthCodeBoxes length={4} value={pin} label={t('web.profile.newPin')} disabled={state.busy} autoFocus onChange={value => {setPIN(value);setMismatch(false);}} onComplete={setPIN} /></label>
        <label>{t('web.profile.repeatPin')}<AuthCodeBoxes length={4} value={repeat} label={t('web.profile.repeatPin')} disabled={state.busy} error={mismatch ? t('profile.pinMismatch') : undefined} onChange={value => {setRepeat(value);setMismatch(false);}} onComplete={setRepeat} /></label>
      </form>}
    </Dialog>}
  </>;
}
