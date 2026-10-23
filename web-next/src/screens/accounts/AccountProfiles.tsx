import {useEffect, useLayoutEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {ManagedProfile} from '@core/profile-management.ts';
import type {ProfilePickerProfile} from '../../kit/artwork/ProfilePicker';
import {AuthError, AuthStage, ServerProfiles, ServerProfilePIN} from '../../kit/accounts';
import {ProfilePINRecovery} from '../../kit/accounts/ProfilePINRecovery';
import {Button} from '../../kit/controls/Button';
import {createBrowserAccountEntry} from '../../platform/account-entry';
import {browserAccount} from '../../platform/account';
import type {BrowserProfileRecovery} from '../../platform/profile-recovery';
import {useAccountBootstrap} from '../../shell/BootstrapState';
import {router} from '../../router';

const t = defaultI18n.t;
/** DirectSnapshot contains only this account's profiles on the proven server.
 * Selection remains a live server decision; no cross-server profile is added. */
const pickerProfile = (profile: ManagedProfile): ProfilePickerProfile => ({...profile, eligible: true});
export function AccountProfilesScreen() {
  const entry = useMemo(createBrowserAccountEntry, []), state = useSyncExternalStore(entry.subscribe, entry.getSnapshot);
  const bootstrap = useAccountBootstrap(), suspend = bootstrap?.suspendForAccountEntry, resume = bootstrap?.resumeAfterAccountEntry ?? bootstrap?.retry;
  const [selected, setSelected] = useState<ProfilePickerProfile>(), [remember, setRemember] = useState(false), [recovery, setRecovery] = useState<BrowserProfileRecovery>();
  const [lockedUntil, setLockedUntil] = useState(0), [now, setNow] = useState(Date.now);
  useLayoutEffect(() => {suspend?.();}, [suspend]);
  useEffect(() => {void entry.loadProfiles(); return () => entry.dispose();}, [entry]);
  useEffect(() => () => recovery?.dispose(), [recovery]);
  useEffect(() => {
    const error = state.error as {code?: string; retryAfterSeconds?: number} | undefined;
    if (error?.code !== 'profile_pin_locked') {setLockedUntil(0); return;}
    const seconds = error.retryAfterSeconds;
    if (typeof seconds === 'number' && Number.isFinite(seconds) && seconds > 0) {const time = Date.now(); setNow(time); setLockedUntil(time + seconds * 1000);}
  }, [state.error]);
  useEffect(() => {
    if (!lockedUntil) return;
    const timer = setInterval(() => {const time = Date.now(); setNow(time); if (time >= lockedUntil) {clearInterval(timer); setLockedUntil(0);}}, 1000);
    return () => clearInterval(timer);
  }, [lockedUntil]);
  useEffect(() => {
    if (state.phase !== 'ready') return;
    let current = true;
    void Promise.resolve(resume?.()).then(() => {if (current) void router.navigate({to: '/'});});
    return () => {current = false;};
  }, [state.phase, resume]);
  const switchServer = () => {void router.navigate({to: '/servers'});};
  const snapshot = state.snapshot, server = state.server;
  if (!snapshot || !server) return <AuthStage legal={false}><AuthError error={state.error} />{Boolean(state.error) && <Button onClick={() => void entry.loadProfiles()}>{t('action.tryAgain')}</Button>}</AuthStage>;
  const profiles = snapshot.profiles.map(pickerProfile);
  const choose = (profile: ProfilePickerProfile) => {setRemember(false); if (profile.pinRequired) setSelected(profile); else void entry.chooseProfile(profile.id);};
  const pinError = state.error && ['profile_pin_invalid', 'profile_pin'].includes(String((state.error as {code?: unknown}).code));
  const seconds = Math.max(0, Math.ceil((lockedUntil - now) / 1000));
  return <>
    {selected ? <ServerProfilePIN key={selected.id} serverName={server.name} profile={selected} remember={remember} onRememberChange={setRemember}
      busy={state.busy || seconds > 0} error={Boolean(pinError)} errorText={seconds > 0 ? t('accounts.profiles.pinLocked', {seconds}) : undefined}
      failure={!pinError && !seconds ? state.error : undefined} onComplete={pin => void entry.chooseProfile(selected.id, pin, remember)}
      onOtherProfiles={() => {setSelected(undefined); void entry.loadProfiles();}} onSwitchServer={switchServer}
      onForgotPIN={() => setRecovery(entry.profileRecovery(selected.id))} /> :
      <ServerProfiles serverName={server.name} profiles={profiles} onSelect={choose} onSwitchServer={switchServer} error={state.error} />}
    {selected && recovery && <Recovery flow={recovery} profile={selected} username={snapshot.account.username} onClose={() => {setRecovery(undefined); void entry.loadProfiles();}}
      onOpen={pin => {setRecovery(undefined); void entry.loadProfiles().then(() => entry.chooseProfile(selected.id, pin));}} />}
  </>;
}
function Recovery({flow, profile, username, onClose, onOpen}: {flow: BrowserProfileRecovery; profile: ProfilePickerProfile; username: string; onClose: () => void; onOpen: (pin: string) => void}) {
  const state = useSyncExternalStore(flow.subscribe, flow.getSnapshot), [busy, setBusy] = useState(false), [error, setError] = useState<unknown>();
  const portico = async () => {
    setBusy(true); setError(undefined);
    try {
      const central = browserAccount(); await central.service.synchronize(); const session = await central.service.accessSession();
      const current = () => {const live = central.service.getSnapshot().session; return live?.account.id === session.account.id && live.familyId === session.familyId;};
      await flow.portico({request: (path, method = 'GET', body, signal) => central.api.send(path, method, body, signal, session.accessToken)}, central.api.origin, current);
    } catch (failure) {setError(failure);} finally {setBusy(false);}
  };
  return <ProfilePINRecovery profileName={profile.name} username={username} phase={state.phase} busy={state.busy || busy} error={state.error ?? error}
    onPassword={password => void flow.password(password)} onPortico={() => void portico()} onCode={code => void flow.code(code)} onPIN={pin => void flow.reset(pin)} onClose={onClose} onOpen={onOpen} />;
}
