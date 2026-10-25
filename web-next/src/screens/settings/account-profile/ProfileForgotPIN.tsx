import {useEffect, useRef, useState, useSyncExternalStore, type RefObject} from 'react';
import {defaultI18n} from '@i18n';
import type {HttpLocalApi} from '@core/index.ts';
import {BrowserProfileRecovery} from '../../../platform/profile-recovery';
import {AuthCodeBoxes, AuthError, AuthPasswordField} from '../../../kit/accounts/AuthStage';
import {Button} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays/Dialog';
import type {ProfileSettingsSource} from './source';
import styles from './profile.module.css';
const t = defaultI18n.t;

/** Local-account recovery uses the same temporary sign-in controller as the
 * picker. Its grant never becomes the installed viewer or a profile switch. */
export function ProfileForgotPIN({source, api, isCurrent}: {source: ProfileSettingsSource; api: HttpLocalApi; isCurrent: () => boolean}) {
  const state = useSyncExternalStore(source.subscribe, source.getSnapshot, source.getSnapshot);
  const [flow, setFlow] = useState<BrowserProfileRecovery>();
  const active = useRef<BrowserProfileRecovery | undefined>(undefined), trigger = useRef<HTMLButtonElement>(null);
  useEffect(() => () => {active.current?.dispose();active.current = undefined;}, [source]);
  const selected = source.selected(), account = state.document;
  const close = (captured: BrowserProfileRecovery) => {
    if (active.current !== captured) return;
    active.current = undefined;captured.dispose();setFlow(undefined);
  };
  if (!isCurrent() || !account || account.canManage || !selected?.pinRequired) return null;
  return <>
    <Button ref={trigger} size="small" variant="glass" disabled={state.busy || !!active.current} onClick={() => {
      const document = source.getSnapshot().document, profile = source.selected();
      if (!isCurrent() || active.current || !document || document.canManage || !profile?.pinRequired) return;
      const captured = new BrowserProfileRecovery(api, document, profile.id, 'local', () => isCurrent() && !!source.getSnapshot().document && source.selected()?.id === profile.id);
      active.current = captured;setFlow(captured);
    }}>{t('next.settingsSpec.account.profile.groups.2.rows.1.control.label')}</Button>
    {flow && <RecoveryForm key={flow.profileId} flow={flow} current={() => active.current === flow && isCurrent() && !!source.getSnapshot().document}
      profileName={selected.name} username={account.account.username} close={() => close(flow)} returnFocusRef={trigger} />}
  </>;
}

function RecoveryForm({flow, current, profileName, username, close, returnFocusRef}: {
  flow: BrowserProfileRecovery; current: () => boolean; profileName: string; username: string; close: () => void;
  returnFocusRef: RefObject<HTMLElement | null>;
}) {
  const state = useSyncExternalStore(flow.subscribe, flow.getSnapshot, flow.getSnapshot);
  const [password, setPassword] = useState(''), [pin, setPIN] = useState(''), [code, setCode] = useState('');
  const live = useRef(true), passwordInput = useRef<HTMLInputElement>(null);
  useEffect(() => {live.current = true;return () => {live.current = false;};}, []);
  const admitted = () => live.current && current();
  const dismiss = () => {setPassword('');setPIN('');setCode('');close();};
  const title = state.phase === 'done' ? t('profile.pinChanged') : state.phase === 'pin' ? t('profile.pinRecoveryTitle', {name: profileName}) : t('accounts.profiles.forgotTitle', {profile: profileName});
  if (!current()) return null;
  return <Dialog open width={420} title={title} returnFocusRef={returnFocusRef} initialFocusRef={state.phase === 'password' ? passwordInput : undefined}
    onOpenChange={open => {if (!open) dismiss();}}>
    <div className={styles.fields}>
      <AuthError error={state.error} />
      {state.phase === 'password' && <form className={styles.fields} onSubmit={event => {
        event.preventDefault();if (!admitted() || state.busy || !password) return;const value = password;setPassword('');void flow.password(value);
      }}>
        <AuthPasswordField ref={passwordInput} label={t('accounts.profiles.accountPassword', {username})} autoComplete="current-password" value={password} disabled={state.busy} onChange={event => setPassword(event.currentTarget.value)} />
        <Button size="small" variant="primary" type="submit" loading={state.busy} disabled={!password}>{t('accounts.signIn.continue')}</Button>
      </form>}
      {state.phase === 'challenge' && <><AuthCodeBoxes length={6} value={code} label={t('profile.pinRecoveryCode')} disabled={state.busy} onChange={setCode} onComplete={value => {if (admitted() && !state.busy) {setCode('');void flow.code(value);}}} /></>}
      {state.phase === 'pin' && <><AuthCodeBoxes length={4} value={pin} label={t('profile.pinChoose')} disabled={state.busy} onChange={setPIN} onComplete={setPIN} />
        <Button size="small" variant="primary" loading={state.busy} disabled={pin.length !== 4} onClick={() => {if (admitted() && !state.busy) {const value = pin;setPIN('');void flow.reset(value);}}}>{t('profile.pinRecoveryAction')}</Button></>}
      {state.phase === 'done' && <p className={styles.limit}>{t('profile.pinSavedBody', {name: profileName})}</p>}
      <Button size="small" onClick={dismiss}>{t(state.phase === 'done' ? 'action.done' : 'action.cancel')}</Button>
    </div>
  </Dialog>;
}
