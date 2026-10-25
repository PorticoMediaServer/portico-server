import {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {passwordStrength} from '@core/password-strength.ts';
import {Button} from '../../kit/controls/Button';
import {TextField} from '../../kit/controls/Fields';
import {Dialog} from '../../kit/overlays/Dialog';
import {router} from '../../router';
import {discardLocalMove, handOffLocalMove} from '../../platform/local-move';
import {disposeLocalMoveContinuation, startLocalMoveContinuation} from '../../platform/local-move-continuation';
import {ownerSwitchContextAllowed} from '../../platform/secure-pending';
import type {SettingsOwnerSwitch} from './owner-switch';
import styles from './owner-signin-switch.module.css';

const t = defaultI18n.t;
const panel = (startOver: boolean) => startOver
  ? 'next.settingsSpec.server.signin.panels.startOverLocal'
  : 'next.settingsSpec.server.signin.panels.switchToLocal';

/** The drawn Change group's move to local. The settings row opens the existing
 * owner-move preview; confirmation is the reviewed controller, and its one-shot
 * custody is handed to one private sign-in entry instance so that screen renders
 * its own challenge, profile and ready states. This surface never adopts or
 * stores a session and never carries a grant in a URL or storage. */
export function LocalMoveSwitch({source, isCurrent, onError, startOver = false}: {
  startOver?: boolean; source: SettingsOwnerSwitch; isCurrent: () => boolean; onError: (error: unknown) => void;
}) {
  const state = useSyncExternalStore(source.subscribe, source.getSnapshot);
  const [username, setUsername] = useState(''), [password, setPassword] = useState(''), [confirmation, setConfirmation] = useState('');
  const [match, setMatch] = useState(''), [open, setOpen] = useState(false), [secure, setSecure] = useState(false);
  const pending = useRef(false), alive = useRef(true);
  useEffect(() => {alive.current = true; return () => {alive.current = false;};}, []);
  const current = () => alive.current && isCurrent();
  const run = async (work: () => Promise<void>) => {
    if (!alive.current || pending.current) return;
    pending.current = true;
    try {await work();} catch (error) {if (alive.current) onError(error);} finally {pending.current = false;}
  };
  const start = () => {void run(async () => {
    if (!current()) return;
    // The captured controller guards the secure context; refuse before a preview.
    if (!ownerSwitchContextAllowed()) {setSecure(true);return;}
    setConfirmation(''); setPassword(''); setMatch('');
    await source.previewDirection('local', startOver);
    if (alive.current) setOpen(true);
  });};
  const close = () => {void run(async () => {
    if (!current() || state.busy) return;
    await source.cancelPreview();
    setPassword(''); setMatch(''); setConfirmation(''); setUsername('');
    if (alive.current) setOpen(false);
  });};
  const proceed = () => {void run(async () => {
    const preview = state.preview;
    if (!current() || !preview || preview.to !== 'local' || preview.startOver !== startOver || state.busy) return;
    if (!username.trim() || !password || password !== match || startOver && confirmation !== preview.serverName) return;
    const custody = await source.confirmLocal({username, password}, startOver ? confirmation : '');
    setPassword(''); setMatch(''); setUsername(''); setConfirmation('');
    // A successful confirmation already switched the server, so current() may now
    // report the new mode; only a gone view prevents the handover. Admission still
    // rechecks the selection revision and retires the grant if anything changed.
    if (!alive.current) {await custody.retire();return;}
    handOffLocalMove(custody);
    startLocalMoveContinuation();
    try {await router.navigate({to: '/sign-in'});}
    catch (error) {disposeLocalMoveContinuation(); discardLocalMove(); throw error;}
  });};
  const busy = state.busy || pending.current, preview = state.preview, keys = panel(startOver);
  const mismatch = Boolean(match) && password !== match;
  const ready = Boolean(preview && preview.to === 'local' && preview.startOver === startOver && username.trim() && password && !mismatch
    && (!startOver || confirmation === preview.serverName) && passwordStrength(password).acceptable);
  return <>
    <Button className={startOver ? styles.destructive : undefined} variant="glass" size="small" disabled={state.busy || !!state.preview || state.confirmed}
      onClick={start}>{t(`${startOver ? 'next.settingsSpec.server.signin.groups.1.rows.1' : 'next.settingsSpec.server.signin.groups.1.rows.0'}.control.label`)}</Button>
    <Dialog children={null} open={secure} onOpenChange={value => {if (!value) setSecure(false);}} title={t('accounts.ownerSwitch.secureTitle')}
      description={t('accounts.ownerSwitch.secureBody', {localUrl: 'http://localhost:32500'})}
      footer={<Button variant="glass" onClick={() => setSecure(false)}>{t('action.cancel')}</Button>}/>
    <Dialog open={open && !!preview} onOpenChange={value => {if (!value) close();}} width={600}
      title={t(`${keys}.title`)} dismissOnBack={!busy} dismissOnOutsidePress={!busy} showCloseButton={!busy}
      footer={<><Button variant="glass" disabled={busy} onClick={close}>{t(`${keys}.footer.secondary`)}</Button>
        <Button className={startOver ? styles.destructive : undefined} disabled={busy || !ready} onClick={proceed}>{t(`${keys}.footer.primary`)}</Button></>}>
      {preview && <div className={styles.preview}>
        <p>{preview.serverName}</p>
        {startOver ? <>
          <h3>{t(`${keys}.groups.0.title`)}</h3>
          <dl>
            <dt>{t(`${keys}.groups.0.rows.0.label`)}</dt><dd>{t(`${keys}.groups.0.rows.0.help`)}</dd>
            <dt>{t(`${keys}.groups.0.rows.1.label`)}</dt><dd>{t(`${keys}.groups.0.rows.1.help`)}</dd>
            <dt>{t(`${keys}.groups.0.rows.2.label`)}</dt><dd>{t(`${keys}.groups.0.rows.2.help`)}</dd>
            <dt>{t(`${keys}.groups.0.rows.3.label`)}</dt><dd/>
            <dt>{t(`${keys}.groups.0.rows.4.label`)}</dt><dd>{t(`${keys}.groups.0.rows.4.help`)}</dd>
            <dt>{t(`${keys}.groups.0.rows.5.label`)}</dt><dd>{t(`${keys}.groups.0.rows.5.help`)}</dd>
          </dl>
          <h3>{t(`${keys}.groups.1.title`)}</h3>
          <dl>
            <dt>{t(`${keys}.groups.1.rows.0.label`)}</dt><dd>{t(`${keys}.groups.1.rows.0.help`)}</dd>
            <dt>{t(`${keys}.groups.1.rows.1.label`)}</dt><dd>{t(`${keys}.groups.1.rows.1.help`)}</dd>
          </dl>
        </> : <>
          <h3>{t(`${keys}.groups.0.title`)}</h3>
          <dl>
            <dt>{t(`${keys}.groups.0.rows.0.label`)}</dt><dd>{defaultI18n.number(preview.people)}<p>{t(`${keys}.groups.0.rows.0.help`)}</p></dd>
            <dt>{t(`${keys}.groups.0.rows.1.label`)}</dt><dd>{preview.devices !== undefined && defaultI18n.number(preview.devices)}</dd>
            <dt>{t(`${keys}.groups.0.rows.2.label`)}</dt><dd>{t(`${keys}.groups.0.rows.2.help`)}</dd>
            <dt>{t(`${keys}.groups.0.rows.3.label`)}</dt><dd>{t(`${keys}.groups.0.rows.3.help`)}</dd>
            <dt>{t(`${keys}.groups.0.rows.4.label`)}</dt><dd>{t(`${keys}.groups.0.rows.4.help`)}</dd>
            <dt>{t(`${keys}.groups.0.rows.5.label`)}</dt><dd>{defaultI18n.number(preview.invitations)}<p>{t(`${keys}.groups.0.rows.5.help`)}</p></dd>
            <dt>{t(`${keys}.groups.0.rows.6.label`)}</dt><dd>{t(`${keys}.groups.0.rows.6.help`)}</dd>
          </dl>
          <h3>{t(`${keys}.groups.1.title`)}</h3>
          <dl>
            <dt>{t(`${keys}.groups.1.rows.0.label`)}</dt><dd/>
            <dt>{t(`${keys}.groups.1.rows.1.label`)}</dt><dd>{defaultI18n.number(preview.profiles)}<p>{t(`${keys}.groups.1.rows.1.help`)}</p></dd>
          </dl>
        </>}
        <h3>{t(`${keys}.groups.2.title`)}</h3>
        <TextField label={t(`${keys}.groups.2.rows.0.label`)} description={t(`${keys}.groups.2.rows.0.help`)}
          placeholder={t(`${keys}.groups.2.rows.0.control.placeholder`)} autoComplete="username" value={username} disabled={busy}
          onChange={event => setUsername(event.currentTarget.value)}/>
        <TextField type="password" label={t(`${keys}.groups.2.rows.1.label`)}
          placeholder={t(`${keys}.groups.2.rows.1.control.placeholder`)} autoComplete="new-password" value={password} disabled={busy}
          onChange={event => setPassword(event.currentTarget.value)}/>
        <TextField type="password" label={t(`${keys}.groups.2.rows.2.label`)} error={mismatch ? t('web.reset.mismatch') : undefined}
          placeholder={t(`${keys}.groups.2.rows.2.control.placeholder`)} autoComplete="new-password" value={match} disabled={busy}
          onChange={event => setMatch(event.currentTarget.value)}/>
        <dl><dt>{t(`${keys}.groups.2.rows.3.label`)}</dt><dd>{t(`${keys}.groups.2.rows.3.help`)}</dd></dl>
        {startOver && <>
          <h3>{t(`${keys}.groups.3.title`)}</h3>
          <TextField label={t(`${keys}.groups.3.rows.0.label`)} placeholder={t(`${keys}.groups.3.rows.0.control.placeholder`)}
            value={confirmation} disabled={busy} onChange={event => setConfirmation(event.currentTarget.value)}/>
        </>}
      </div>}
    </Dialog>
  </>;
}
