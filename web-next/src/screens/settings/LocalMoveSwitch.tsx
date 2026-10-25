import {useLayoutEffect, useRef, useState, useSyncExternalStore} from 'react';
import type {AccountMove} from '@core/account-move.ts';
import {passwordStrength} from '@core/password-strength.ts';
import {defaultI18n} from '@i18n';
import {Button} from '../../kit/controls/Button';
import {TextField} from '../../kit/controls/Fields';
import {Dialog} from '../../kit/overlays/Dialog';
import {router} from '../../router';
import {cancelLocalMoveHandoff, offerLocalMove, type LocalMoveHandoff} from '../../platform/local-move-continuation';
import {ownerSwitchContextAllowed} from '../../platform/secure-pending';
import type {SettingsOwnerSwitch} from './owner-switch';
import styles from './owner-signin-switch.module.css';

const t = defaultI18n.t;
const panel = (startOver: boolean) => startOver
  ? 'next.settingsSpec.server.signin.panels.startOverLocal'
  : 'next.settingsSpec.server.signin.panels.switchToLocal';

/** One rendered opening, bound to the exact controller preview that produced it.
 * Its phase is the claim: only an `open` attempt can confirm, only the confirming
 * attempt can hand off, and it can never be revived after close or unmount. */
type Attempt = {
  readonly id: number;
  readonly preview: AccountMove;
  readonly startOver: boolean;
  readonly source: SettingsOwnerSwitch;
  phase: 'open' | 'confirming' | 'handed' | 'closed';
};

/** The drawn Change group's move to local. The settings row opens the existing
 * owner-move preview; confirmation is the reviewed controller, and its one-shot
 * custody is paired with one private sign-in entry instance so that screen renders
 * its own challenge, profile and ready states. This surface never adopts or stores
 * a session and never carries a grant in a URL or storage. */
export function LocalMoveSwitch({source, isCurrent, onError, startOver = false}: {
  startOver?: boolean; source: SettingsOwnerSwitch; isCurrent: () => boolean; onError: (error: unknown) => void;
}) {
  const state = useSyncExternalStore(source.subscribe, source.getSnapshot);
  const [username, setUsername] = useState(''), [password, setPassword] = useState(''), [confirmation, setConfirmation] = useState('');
  const [match, setMatch] = useState(''), [opening, setOpening] = useState<Attempt>(), [secure, setSecure] = useState(false);
  const active = useRef<Attempt>(), sourceRef = useRef(source), ticket = useRef<{retired: boolean}>();
  const sequence = useRef(0), starting = useRef(0);
  const cancelPreview = (attempt: Attempt) => {
    try {if (attempt.source.getSnapshot().preview === attempt.preview) void attempt.source.cancelPreview().catch(() => {});} catch { /* the controller already ended */ }
  };
  // A new controller replaces the old one; the old opening is retired as its own
  // instance. Layout cleanup also runs synchronously on unmount, before a late
  // continuation can publish for a successor.
  useLayoutEffect(() => {
    const owned = {retired: false};
    ticket.current = owned;sourceRef.current = source;
    return () => {
      owned.retired = true;starting.current = 0;
      const attempt = active.current;
      if (!attempt) return;
      active.current = undefined;
      attempt.phase = 'closed';
      cancelPreview(attempt);
    };
  }, [source]);
  const clearDraft = () => {setUsername('');setPassword('');setMatch('');setConfirmation('');};
  const start = () => {
    const owned = ticket.current, owner = source;
    if (!owned || owned.retired || owner !== sourceRef.current || starting.current || active.current) return;
    if (!isCurrent() || state.busy || state.preview || state.confirmed) return;
    // The captured controller checks this again; refuse before creating a preview.
    if (!ownerSwitchContextAllowed()) {setSecure(true);return;}
    const id = ++sequence.current;starting.current = id;
    clearDraft();
    void owner.previewDirection('local', startOver).then(preview => {
      if (owned.retired || owner !== sourceRef.current || id !== sequence.current) {
        // This start no longer owns the preview it created; retire exactly that one.
        try {if (owner.getSnapshot().preview === preview) void owner.cancelPreview().catch(() => {});} catch { /* the controller already ended */ }
        return;
      }
      const attempt: Attempt = {id, preview, startOver, source: owner, phase: 'open'};
      active.current = attempt;setOpening(attempt);
    }).catch(error => {
      if (!owned.retired && owner === sourceRef.current && id === sequence.current) onError(error);
    }).finally(() => {if (starting.current === id) starting.current = 0;});
  };
  const close = (attempt: Attempt) => {
    if (active.current !== attempt || attempt.phase !== 'open') return;
    attempt.phase = 'closed';active.current = undefined;setOpening(undefined);clearDraft();cancelPreview(attempt);
  };
  const proceed = (attempt: Attempt) => {
    if (active.current !== attempt || attempt.phase !== 'open') return;
    const preview = attempt.preview;
    if (preview.to !== 'local' || preview.startOver !== attempt.startOver) return;
    if (!username.trim() || !password || password !== match || !passwordStrength(password).acceptable) return;
    if (attempt.startOver && confirmation !== preview.serverName) return;
    if (attempt.source.getSnapshot().preview !== preview) return;
    const owned = ticket.current;
    if (!owned || owned.retired || attempt.source !== sourceRef.current) return;
    // Claim this exact opening and freeze its draft before any await.
    const submission = Object.freeze({username: username.trim(), password, confirmation: attempt.startOver ? confirmation : ''});
    attempt.phase = 'confirming';clearDraft();
    void (async () => {
      try {
        const custody = await attempt.source.confirmLocal({username: submission.username, password: submission.password}, submission.confirmation);
        // The view, controller or ticket may have been replaced while confirming;
        // the fresh grant is then retired instead of being handed to a successor.
        if (active.current !== attempt || attempt.phase !== 'confirming' || !ticket.current || ticket.current.retired || sourceRef.current !== attempt.source) {
          attempt.phase = 'closed';await custody.retire();return;
        }
        let handoff: LocalMoveHandoff;
        try {handoff = offerLocalMove(custody);}
        catch (error) {attempt.phase = 'closed';throw error;}
        attempt.phase = 'handed';
        try {await router.navigate({to: '/sign-in'});}
        catch (error) {cancelLocalMoveHandoff(handoff);throw error;}
      } catch (error) {
        attempt.phase = 'closed';
        if (active.current === attempt) active.current = undefined;
        if (!ticket.current?.retired) onError(error);
      }
    })();
  };
  const current = opening && active.current === opening ? opening : undefined;
  const preview = current?.preview, busy = state.busy || current?.phase === 'confirming' || current?.phase === 'handed';
  const mismatch = Boolean(match) && password !== match, keys = panel(startOver);
  const ready = Boolean(current && current.phase === 'open' && preview && preview.to === 'local' && preview.startOver === startOver
    && username.trim() && password && !mismatch && (!startOver || confirmation === preview.serverName) && passwordStrength(password).acceptable);
  return <>
    <Button className={startOver ? styles.destructive : undefined} variant="glass" size="small" disabled={state.busy || !!state.preview || state.confirmed || !!active.current}
      onClick={start}>{t(`${startOver ? 'next.settingsSpec.server.signin.groups.1.rows.1' : 'next.settingsSpec.server.signin.groups.1.rows.0'}.control.label`)}</Button>
    <Dialog children={null} open={secure} onOpenChange={value => {if (!value) setSecure(false);}} title={t('accounts.ownerSwitch.secureTitle')}
      description={t('accounts.ownerSwitch.secureBody', {localUrl: 'http://localhost:32500'})}
      footer={<Button variant="glass" onClick={() => setSecure(false)}>{t('action.cancel')}</Button>}/>
    <Dialog open={!!current && !!preview && state.preview === preview} onOpenChange={value => {if (!value && current) close(current);}} width={600}
      title={t(`${keys}.title`)} dismissOnBack={!busy} dismissOnOutsidePress={!busy} showCloseButton={!busy}
      footer={<><Button variant="glass" disabled={busy} onClick={() => {if (current) close(current);}}>{t(`${keys}.footer.secondary`)}</Button>
        <Button className={startOver ? styles.destructive : undefined} disabled={busy || !ready} onClick={() => {if (current) proceed(current);}}>{t(`${keys}.footer.primary`)}</Button></>}>
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
