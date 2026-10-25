import {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {DirectSnapshot} from '@core/profile-management.ts';
import type {IdentityApi} from '@core/identity-client.ts';
import type {AccountEntryState} from '../../platform/account-entry';
import {AuthError, AuthStage, ServerProfiles, ServerProfilePIN} from '../../kit/accounts';
import {ProfilePINRecovery} from '../../kit/accounts/ProfilePINRecovery';
import {Button} from '../../kit/controls';

type RecoveryPhase = Parameters<typeof ProfilePINRecovery>[0]['phase'];
/** Structural view seam for the real Accounts-owned recovery controller.
 * No temporary proof, family or viewer credential is a view-model field. */
export type ProfileRecoveryView = Readonly<{
  subscribe: (listener: () => void) => () => void;
  getSnapshot: () => Readonly<{phase: RecoveryPhase; busy?: boolean; error?: unknown}>;
  password: (value: string) => Promise<void>;
  portico: (api: IdentityApi, origin: string, isCurrent: () => boolean) => Promise<void>;
  code: (value: string) => Promise<void>;
  reset: (value: string) => Promise<void>;
  dispose: () => void;
}>;
export type ProfilePickerViewBinding = Readonly<{
  subscribe: (listener: () => void) => () => void;
  getSnapshot: () => AccountEntryState;
  loadProfiles: () => Promise<void>;
  chooseProfile: (id: string, pin: string, remember: boolean) => Promise<void>;
  profileRecovery: (id: string) => ProfileRecoveryView;
  porticoRecovery: (flow: ProfileRecoveryView) => Promise<void>;
}>;

/** auth13/14/18. The route supplies the actual captured entry/domain helpers,
 * retains their authority and owns their disposal. No renderer creates auth. */
export function AccountProfilesView({binding, onSwitchServer}: {binding: ProfilePickerViewBinding; onSwitchServer: () => void}) {
  const state = useSyncExternalStore(binding.subscribe, binding.getSnapshot);
  const [selectedId, setSelectedId] = useState<string>(), [remember, setRemember] = useState(false), [recovery, setRecovery] = useState<ProfileRecoveryView>();
  const [lock, setLock] = useState<Readonly<{error: unknown; until: number}>>(), [now, setNow] = useState(Date.now);
  const activeRecovery = useRef<ProfileRecoveryView>(undefined);
  useEffect(() => {
    setSelectedId(undefined); setRemember(false); setRecovery(undefined);
    return () => {activeRecovery.current?.dispose(); activeRecovery.current = undefined;};
  }, [binding, state.snapshot?.serverId, state.snapshot?.account.id, state.phase]);
  useEffect(() => {
    const failure = state.error as {code?: unknown; retryAfterSeconds?: unknown} | undefined;
    const seconds = failure?.retryAfterSeconds;
    if (failure?.code !== 'profile_pin_locked' || typeof seconds !== 'number' || !Number.isFinite(seconds) || seconds <= 0) {setLock(undefined); return;}
    const start = Date.now(); setNow(start); setLock({error: state.error, until: start + seconds * 1000});
  }, [state.error]);
  useEffect(() => {
    if (!lock) return;
    const timer = setInterval(() => {const current = Date.now(); setNow(current); if (current >= lock.until) clearInterval(timer);}, 1000);
    return () => clearInterval(timer);
  }, [lock]);
  const snapshot = state.snapshot, server = state.server;
  // DirectSnapshot is this proved account's profile list. Library access is
  // evaluated inside each profile, never used to disable its picker tile.
  const profiles = snapshot?.profiles.map(profile => ({...profile, eligible: true}));
  const selected = profiles?.find(profile => profile.id === selectedId);
  const closeRecovery = () => {activeRecovery.current?.dispose(); activeRecovery.current = undefined; setRecovery(undefined);};
  if (!snapshot || !server) return <AuthStage legal={false}>{!state.error && <p role="status">{defaultI18n.t('auth.loadingProfiles')}</p>}<AuthError error={state.error} />{Boolean(state.error) && <Button onClick={() => void binding.loadProfiles()}>{defaultI18n.t('action.tryAgain')}</Button>}</AuthStage>;
  // Required password/two-step pages are a different account-only continuation.
  // This picker cannot offer profiles until the actual controller clears it.
  if (state.phase !== 'profiles' || snapshot.twoStepSetupRequired) return null;
  const pinError = Boolean(state.error && ['profile_pin_invalid', 'profile_pin'].includes(String((state.error as {code?: unknown}).code)));
  const seconds = lock && lock.error === state.error ? Math.max(0, Math.ceil((lock.until - now) / 1000)) : 0;
  const otherProfiles = () => {closeRecovery(); setSelectedId(undefined); setRemember(false); void binding.loadProfiles();};
  return <>
    {selected ? <ServerProfilePIN key={JSON.stringify([selected.id, selected.pinRevision])} serverName={server.name} profile={selected} remember={remember} onRememberChange={setRemember}
      busy={state.busy || seconds > 0} error={pinError} errorText={seconds ? defaultI18n.t('accounts.profiles.pinLocked', {seconds}) : undefined}
      failure={!pinError && !seconds ? state.error : undefined} onComplete={pin => void binding.chooseProfile(selected.id, pin, remember)}
      onOtherProfiles={otherProfiles} onSwitchServer={onSwitchServer} onForgotPIN={() => {
        closeRecovery(); const flow = binding.profileRecovery(selected.id); activeRecovery.current = flow; setRecovery(flow);
      }} /> : <ServerProfiles serverName={server.name} profiles={profiles!} error={state.error} onSwitchServer={onSwitchServer}
        onSelect={profile => {setRemember(false); if (profile.pinRequired) setSelectedId(profile.id); else void binding.chooseProfile(profile.id, '', false);}} />}
    {selected && recovery && <Recovery binding={binding} flow={recovery} profileName={selected.name} username={snapshot.account.username}
      onClose={closeRecovery} onOpen={pin => {closeRecovery(); void binding.loadProfiles().then(() => binding.chooseProfile(selected.id, pin, false));}} />}
  </>;
}

function Recovery({binding, flow, profileName, username, onClose, onOpen}: {
  binding: ProfilePickerViewBinding; flow: ProfileRecoveryView; profileName: string; username: DirectSnapshot['account']['username'];
  onClose: () => void; onOpen: (pin: string) => void;
}) {
  const state = useSyncExternalStore(flow.subscribe, flow.getSnapshot), [busy, setBusy] = useState(false), [error, setError] = useState<unknown>();
  const lifetime = useRef(true);
  useEffect(() => {lifetime.current = true; return () => {lifetime.current = false;};}, []);
  const portico = async () => {
    if (busy) return; setBusy(true); setError(undefined);
    try {await binding.porticoRecovery(flow);} catch (failure) {if (lifetime.current) setError(failure);} finally {if (lifetime.current) setBusy(false);}
  };
  return <ProfilePINRecovery profileName={profileName} username={username} phase={state.phase} busy={state.busy || busy} error={state.error ?? error}
    onPassword={password => void flow.password(password)} onPortico={() => void portico()} onCode={code => void flow.code(code)} onPIN={pin => void flow.reset(pin)} onClose={onClose} onOpen={onOpen} />;
}
