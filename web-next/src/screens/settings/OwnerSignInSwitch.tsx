import {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {Button} from '../../kit/controls/Button';
import {Dialog} from '../../kit/overlays/Dialog';
import {ownerSwitchContextAllowed} from '../../platform/secure-pending';
import type {SettingsOwnerSwitch} from './owner-switch';
import styles from './owner-signin-switch.module.css';

const t = defaultI18n.t;
/** This surface presents only the existing captured owner-move controller. */
export function OwnerSignInSwitch({source, isCurrent, onError}: {
  source: SettingsOwnerSwitch; isCurrent: () => boolean; onError: (error: unknown) => void;
}) {
  const state = useSyncExternalStore(source.subscribe, source.getSnapshot);
  const [open, setOpen] = useState(false), [secure, setSecure] = useState(false);
  const pending = useRef(false), alive = useRef(true);
  useEffect(() => {alive.current = true; return () => {alive.current = false;};}, []);
  const current = () => alive.current && isCurrent();
  const run = async (work: () => Promise<void>) => {
    if (!current() || pending.current) return;
    pending.current = true;
    try {await work();} catch (error) {if (current()) onError(error);} finally {pending.current = false;}
  };
  const start = () => {void run(async () => {
    // Refuse before preview/claim creation; the controller checks this again.
    if (!ownerSwitchContextAllowed()) {setSecure(true);return;}
    await source.preview(false);
    if (current()) setOpen(true);
  });};
  const close = () => {void run(async () => {
    if (state.busy || state.confirmed) return;
    await source.cancelPreview();
    if (current()) setOpen(false);
  });};
  const proceed = () => {void run(async () => {
    if (!state.preview || state.busy) return;
    const url = state.confirmed ? await source.retryPreparation() : await source.confirmAndPrepare('');
    if (current()) window.location.assign(url);
  });};
  const preview = state.preview;
  const busy = state.busy || pending.current;
  return <>
    <Button variant="glass" size="small" disabled={state.busy || state.confirmed} onClick={start}>{t('next.settingsSpec.server.signin.groups.1.rows.2.control.label')}</Button>
    <Dialog open={secure} onOpenChange={value => {if (!value) setSecure(false);}} title={t('accounts.ownerSwitch.secureTitle')}
      description={t('accounts.ownerSwitch.secureBody', {localUrl: 'http://localhost:32500'})}
      footer={<Button variant="glass" onClick={() => setSecure(false)}>{t('action.cancel')}</Button>}/>
    <Dialog open={open && !!preview} onOpenChange={value => {if (!value) close();}} width={600}
      title={t('next.settingsSpec.server.signin.panels.switchToPortico.title')} dismissOnBack={!busy && !state.confirmed}
      dismissOnOutsidePress={!busy && !state.confirmed} showCloseButton={!busy && !state.confirmed}
      footer={<><Button variant="glass" disabled={busy || state.confirmed} onClick={close}>{t('action.cancel')}</Button>
        <Button disabled={busy} onClick={proceed}>{t('next.settingsSpec.server.signin.panels.switchToPortico.footer.primary')}</Button></>}>
      {preview && <div className={styles.preview}>
        <p>{preview.serverName}</p>
        <h3>{t('next.settingsSpec.server.signin.panels.switchToPortico.groups.0.title')}</h3>
        <p>{t('next.settingsSpec.server.signin.panels.switchToPortico.groups.0.rows.0.help')}</p>
        <h3>{t('next.settingsSpec.server.signin.panels.switchToPortico.groups.1.title')}</h3>
        <dl>
          <dt>{t('next.settingsSpec.server.signin.panels.switchToPortico.groups.1.rows.0.label')}</dt><dd>{t('next.settingsSpec.server.signin.panels.switchToPortico.groups.1.rows.0.help')}</dd>
          <dt>{t('next.settingsSpec.server.signin.panels.switchToPortico.groups.1.rows.1.label')}</dt><dd>{defaultI18n.number(preview.people)}<p>{t('next.settingsSpec.server.signin.panels.switchToPortico.groups.1.rows.1.help')}</p></dd>
          <dt>{t('next.settingsSpec.server.signin.panels.switchToPortico.groups.1.rows.2.label')}</dt><dd>{t('next.settingsSpec.server.signin.panels.switchToPortico.groups.1.rows.2.help')}</dd>
          {preview.devices !== undefined && <><dt>{t('next.settingsSpec.server.signin.panels.switchToPortico.groups.1.rows.3.label')}</dt><dd>{defaultI18n.number(preview.devices)}</dd></>}
          <dt>{t('next.settingsSpec.server.signin.panels.switchToPortico.groups.1.rows.4.label')}</dt><dd>{t('next.settingsSpec.server.signin.panels.switchToPortico.groups.1.rows.4.help')}</dd>
          <dt>{t('next.settingsSpec.server.signin.panels.switchToPortico.groups.1.rows.5.label')}</dt><dd>{t('next.settingsSpec.server.signin.panels.switchToPortico.groups.1.rows.5.help')}</dd>
          <dt>{t('next.settingsSpec.server.signin.panels.switchToPortico.groups.1.rows.6.label')}</dt><dd>{defaultI18n.number(preview.invitations)}<p>{t('next.settingsSpec.server.signin.panels.switchToPortico.groups.1.rows.6.help')}</p></dd>
        </dl>
        <h3>{t('next.settingsSpec.server.signin.panels.switchToPortico.groups.2.title')}</h3>
        <dl><dt>{t('next.settingsSpec.server.signin.panels.switchToPortico.groups.2.rows.0.label')}</dt><dd/>
          <dt>{t('next.settingsSpec.server.signin.panels.switchToPortico.groups.2.rows.1.label')}</dt><dd>{defaultI18n.number(preview.profiles)}<p>{t('next.settingsSpec.server.signin.panels.switchToPortico.groups.2.rows.1.help')}</p></dd></dl>
      </div>}
    </Dialog>
  </>;
}
